package github

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Authenticator は GitHub API リクエストに認証情報を付与するインターフェース。
// PAT や GitHub App (Installation Token) など、異なる認証方式を差し替え可能にする。
type Authenticator interface {
	SetAuth(req *http.Request)
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
func (a *PATAuthenticator) SetAuth(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+a.token)
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

// NewGitHubAppAuthenticator は新しい GitHubAppAuthenticator を生成する
func NewGitHubAppAuthenticator(appID, installationID int64, privateKeyPEM []byte) (*GitHubAppAuthenticator, error) {
	block, _ := pem.Decode(privateKeyPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return &GitHubAppAuthenticator{
		appID:          appID,
		installationID: installationID,
		privateKey:     key,
		baseURL:        githubAPIBase,
	}, nil
}

// SetAuth はキャッシュされた installation token を使い Bearer ヘッダーを設定する。
// トークンが未取得または期限切れの場合は自動で取得・リフレッシュする。
func (a *GitHubAppAuthenticator) SetAuth(req *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.token == "" || time.Now().After(a.expiresAt.Add(-tokenRefreshMargin)) {
		if err := a.refreshToken(); err != nil {
			slog.Error("failed to refresh installation token", "error", err)
			return
		}
	}

	req.Header.Set("Authorization", "Bearer "+a.token)
}

func (a *GitHubAppAuthenticator) refreshToken() error {
	jwtToken, err := a.generateJWT()
	if err != nil {
		return fmt.Errorf("generating JWT: %w", err)
	}

	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", a.baseURL, a.installationID)
	ctx, cancel := context.WithTimeout(context.Background(), installTokenTimeout)
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
		return fmt.Errorf("unexpected status code %d", resp.StatusCode)
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
	sig, err := rsa.SignPKCS1v15(nil, a.privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("signing JWT: %w", err)
	}

	return signingInput + "." + enc.EncodeToString(sig), nil
}
