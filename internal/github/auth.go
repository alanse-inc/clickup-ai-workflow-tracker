package github

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Authenticator は GitHub API リクエストに認証情報を付与するインターフェース。
// PAT や GitHub App (Installation Token) など、異なる認証方式を差し替え可能にする。
type Authenticator interface {
	SetAuth(req *http.Request) error
}

// PATAuthenticator は Personal Access Token による認証を行う
type PATAuthenticator struct {
	token string
}

// NewPATAuthenticator は新しい PATAuthenticator を生成する
func NewPATAuthenticator(token string) *PATAuthenticator {
	return &PATAuthenticator{token: token}
}

// SetAuth はリクエストに Bearer トークンを設定する
func (a *PATAuthenticator) SetAuth(req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+a.token)
	return nil
}

const (
	githubAPIBase       = "https://api.github.com"
	tokenRefreshMargin  = 5 * time.Minute
	jwtExpiration       = 10 * time.Minute
	installTokenTimeout = 30 * time.Second
)

// GitHubAppAuthenticator は GitHub App (Installation Token) による認証を行う
type GitHubAppAuthenticator struct {
	appID          int64
	installationID int64
	privateKey     *rsa.PrivateKey
	baseURL        string // テスト用にオーバーライド可能

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

type installationTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// NewGitHubAppAuthenticator は新しい GitHubAppAuthenticator を生成する。
// PKCS#1 ("RSA PRIVATE KEY") と PKCS#8 ("PRIVATE KEY") の両方をサポートする。
func NewGitHubAppAuthenticator(appID, installationID int64, privateKeyPEM []byte) (*GitHubAppAuthenticator, error) {
	block, _ := pem.Decode(privateKeyPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	key, err := parseRSAPrivateKey(block)
	if err != nil {
		return nil, err
	}

	return &GitHubAppAuthenticator{
		appID:          appID,
		installationID: installationID,
		privateKey:     key,
		baseURL:        githubAPIBase,
	}, nil
}

func parseRSAPrivateKey(block *pem.Block) (*rsa.PrivateKey, error) {
	switch block.Type {
	case "RSA PRIVATE KEY":
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse PKCS#1 private key: %w", err)
		}
		return key, nil
	case "PRIVATE KEY":
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse PKCS#8 private key: %w", err)
		}
		key, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("PKCS#8 key is not RSA")
		}
		return key, nil
	default:
		return nil, fmt.Errorf("unsupported PEM block type %q", block.Type)
	}
}

// SetAuth はキャッシュされた installation token を使い Bearer ヘッダーを設定する。
// トークンが未取得または期限切れの場合は自動で取得・リフレッシュする。
// ロックはリフレッシュ中も保持する。トークン更新は1時間に1回程度であり、
// オーケストレータは単一ポーリングループのため並行ブロックの影響は軽微。
func (a *GitHubAppAuthenticator) SetAuth(req *http.Request) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.token == "" || time.Now().After(a.expiresAt.Add(-tokenRefreshMargin)) {
		cachedToken := a.token
		cachedExpiry := a.expiresAt
		if err := a.refreshToken(req.Context()); err != nil {
			// リフレッシュ失敗でも既存トークンがまだ有効ならフォールバック
			if cachedToken != "" && time.Now().Before(cachedExpiry) {
				req.Header.Set("Authorization", "Bearer "+cachedToken)
				return nil
			}
			return fmt.Errorf("failed to set auth: %w", err)
		}
	}

	req.Header.Set("Authorization", "Bearer "+a.token)
	return nil
}

func (a *GitHubAppAuthenticator) refreshToken(parentCtx context.Context) error {
	jwtToken, err := a.generateJWT()
	if err != nil {
		return fmt.Errorf("generating JWT: %w", err)
	}

	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", a.baseURL, a.installationID)
	ctx, cancel := context.WithTimeout(parentCtx, installTokenTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(respBody))
	}

	var tokenResp installationTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	a.token = tokenResp.Token
	a.expiresAt = tokenResp.ExpiresAt
	return nil
}

func (a *GitHubAppAuthenticator) generateJWT() (string, error) {
	now := time.Now()
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	payload := map[string]any{
		"iat": now.Add(-60 * time.Second).Unix(),
		"exp": now.Add(jwtExpiration).Unix(),
		"iss": a.appID,
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("marshaling header: %w", err)
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshaling payload: %w", err)
	}

	enc := base64.RawURLEncoding
	signingInput := enc.EncodeToString(headerJSON) + "." + enc.EncodeToString(payloadJSON)

	hash := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, a.privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("signing JWT: %w", err)
	}

	return signingInput + "." + enc.EncodeToString(sig), nil
}
