package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type pullRequest struct {
	MergedAt *string `json:"merged_at"`
}

// GitHubPRChecker は GitHub API を使って PR のマージ状態を確認する
type GitHubPRChecker struct {
	auth       Authenticator
	owner      string
	repo       string
	httpClient *http.Client
}

// NewPRChecker は新しい GitHubPRChecker を生成する
func NewPRChecker(auth Authenticator, owner, repo string) *GitHubPRChecker {
	return &GitHubPRChecker{
		auth:       auth,
		owner:      owner,
		repo:       repo,
		httpClient: &http.Client{Timeout: defaultHTTPTimeout},
	}
}

// IsPRMerged はブランチ名規約 feature/clickup-{taskID} を利用して対応する PR を検索し、
// マージ済みの PR が存在する場合に true を返す。
// PR が存在しない場合や全て未マージの場合は false, nil を返す。
func (c *GitHubPRChecker) IsPRMerged(ctx context.Context, taskID string) (bool, error) {
	branch := fmt.Sprintf("feature/clickup-%s", taskID)
	url := fmt.Sprintf("%s/repos/%s/%s/pulls?head=%s:%s&state=all",
		githubAPIBase, c.owner, c.repo, c.owner, branch)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("creating request: %w", err)
	}

	if err := c.auth.SetAuth(req); err != nil {
		return false, fmt.Errorf("setting auth: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("fetching PRs: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var prs []pullRequest
	if err := json.NewDecoder(resp.Body).Decode(&prs); err != nil {
		return false, fmt.Errorf("decoding response: %w", err)
	}

	for _, pr := range prs {
		if pr.MergedAt != nil {
			return true, nil
		}
	}

	return false, nil
}
