package github

import "net/http"

// rewriteTransport は全てのリクエストをモックサーバーに転送するカスタムトランスポート
type rewriteTransport struct {
	base    http.RoundTripper
	baseURL string
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// 元のパスを保持しつつ、ホストをモックサーバーに変更
	req.URL.Scheme = "http"
	req.URL.Host = t.baseURL[len("http://"):]
	return t.base.RoundTrip(req)
}
