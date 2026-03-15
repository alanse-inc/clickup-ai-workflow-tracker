package github

import "net/http"

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
