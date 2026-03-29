package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type pullRequest struct {
	MergedAt *string `json:"merged_at"`
}

type searchResult struct {
	Items []searchItem `json:"items"`
}

type searchItem struct {
	PullRequest *searchPR `json:"pull_request"`
}

type searchPR struct {
	MergedAt *string `json:"merged_at"`
}

// mergeStatus はブランチ検索の結果を表す
type mergeStatus int

const (
	mergeStatusNotFound mergeStatus = iota // PR 未発見 or 全て未マージ
	mergeStatusMerged                      // マージ済み PR あり
	mergeStatusError                       // API エラー
)

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

// IsFeaturePRMerged はブランチ名規約 feature/clickup-{taskID} で PR を検索し、
// 見つからなければ PR 本文の "Closes CU-{taskID}" でフォールバック検索する。
func (c *GitHubPRChecker) IsFeaturePRMerged(ctx context.Context, taskID string) (bool, error) {
	status, _ := c.isBranchPRMerged(ctx, fmt.Sprintf("feature/clickup-%s", taskID))
	if status == mergeStatusMerged {
		return true, nil
	}
	return c.isBodySearchMerged(ctx, fmt.Sprintf("Closes CU-%s", taskID))
}

// IsSpecPRMerged はブランチ名規約 spec/clickup-{taskID} で PR を検索し、
// 見つからなければ PR 本文の "Refs CU-{taskID}" でフォールバック検索する。
func (c *GitHubPRChecker) IsSpecPRMerged(ctx context.Context, taskID string) (bool, error) {
	status, _ := c.isBranchPRMerged(ctx, fmt.Sprintf("spec/clickup-%s", taskID))
	if status == mergeStatusMerged {
		return true, nil
	}
	return c.isBodySearchMerged(ctx, fmt.Sprintf("Refs CU-%s", taskID))
}

// isBranchPRMerged は指定ブランチに対応する PR を検索し、マージ状態を返す。
func (c *GitHubPRChecker) isBranchPRMerged(ctx context.Context, branch string) (mergeStatus, error) {
	apiURL := fmt.Sprintf("%s/repos/%s/%s/pulls?head=%s:%s&state=all",
		githubAPIBase, c.owner, c.repo, c.owner, branch)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return mergeStatusError, fmt.Errorf("creating request: %w", err)
	}

	if err := c.auth.SetAuth(req); err != nil {
		return mergeStatusError, fmt.Errorf("setting auth: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return mergeStatusError, fmt.Errorf("fetching PRs: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return mergeStatusNotFound, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return mergeStatusError, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var prs []pullRequest
	if err := json.NewDecoder(resp.Body).Decode(&prs); err != nil {
		return mergeStatusError, fmt.Errorf("decoding response: %w", err)
	}

	for _, pr := range prs {
		if pr.MergedAt != nil {
			return mergeStatusMerged, nil
		}
	}

	return mergeStatusNotFound, nil
}

// isBodySearchMerged は GitHub Search API を使い、PR 本文に marker を含むマージ済み PR を検索する。
func (c *GitHubPRChecker) isBodySearchMerged(ctx context.Context, marker string) (bool, error) {
	q := fmt.Sprintf(`"%s" in:body type:pr repo:%s/%s is:merged`, marker, c.owner, c.repo)
	apiURL := fmt.Sprintf("%s/search/issues?q=%s", githubAPIBase, url.QueryEscape(q))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return false, fmt.Errorf("creating search request: %w", err)
	}

	if err := c.auth.SetAuth(req); err != nil {
		return false, fmt.Errorf("setting auth: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("search API request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("search API status %d: %s", resp.StatusCode, string(body))
	}

	var result searchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("decoding search response: %w", err)
	}

	for _, item := range result.Items {
		if item.PullRequest != nil && item.PullRequest.MergedAt != nil {
			return true, nil
		}
	}

	return false, nil
}
