package clickup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.clickup.com/api/v2"

const defaultHTTPTimeout = 30 * time.Second

// Client は ClickUp REST API v2 のクライアント。
// リスト内タスクの取得・個別タスクの取得・ステータス更新を提供する。
type Client struct {
	apiToken   string
	listID     string
	baseURL    string
	httpClient *http.Client
}

// NewClient は新しいClickUp APIクライアントを生成する
func NewClient(apiToken, listID string) *Client {
	return &Client{
		apiToken:   apiToken,
		listID:     listID,
		baseURL:    defaultBaseURL,
		httpClient: &http.Client{Timeout: defaultHTTPTimeout},
	}
}

// apiTaskStatus はClickUp APIのステータスフィールド
type apiTaskStatus struct {
	Status string `json:"status"`
}

// apiCustomField はClickUp APIのカスタムフィールド
type apiCustomField struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

// apiTask はClickUp APIのタスクレスポンス
type apiTask struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	Status       apiTaskStatus    `json:"status"`
	CustomFields []apiCustomField `json:"custom_fields"`
	DateCreated  string           `json:"date_created"`
	DateUpdated  string           `json:"date_updated"`
}

// apiTasksResponse はGetTasksのレスポンス
type apiTasksResponse struct {
	Tasks []apiTask `json:"tasks"`
}

func (t *apiTask) toTask() Task {
	cf := make(map[string]string)
	for _, f := range t.CustomFields {
		key := strings.ToLower(strings.ReplaceAll(f.Name, " ", "_"))
		if v, ok := f.Value.(string); ok {
			cf[key] = v
		}
	}
	return Task{
		ID:           t.ID,
		Name:         t.Name,
		Description:  t.Description,
		Status:       strings.ToLower(t.Status.Status),
		CustomFields: cf,
		DateCreated:  t.DateCreated,
		DateUpdated:  t.DateUpdated,
	}
}

func (c *Client) doRequest(ctx context.Context, method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", c.apiToken)
	req.Header.Set("Content-Type", "application/json")
	return c.httpClient.Do(req)
}

// GetTasks はリスト内の全タスクを取得する
func (c *Client) GetTasks(ctx context.Context) ([]Task, error) {
	url := fmt.Sprintf("%s/list/%s/task", c.baseURL, c.listID)
	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("fetching tasks: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result apiTasksResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding tasks response: %w", err)
	}

	tasks := make([]Task, len(result.Tasks))
	for i, t := range result.Tasks {
		tasks[i] = t.toTask()
	}
	return tasks, nil
}

// GetTask は単一タスクを取得する
func (c *Client) GetTask(ctx context.Context, taskID string) (*Task, error) {
	url := fmt.Sprintf("%s/task/%s", c.baseURL, taskID)
	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("fetching task: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var t apiTask
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return nil, fmt.Errorf("decoding task response: %w", err)
	}

	task := t.toTask()
	return &task, nil
}

// UpdateTaskStatus はタスクのステータスを更新する
func (c *Client) UpdateTaskStatus(ctx context.Context, taskID string, status string) error {
	url := fmt.Sprintf("%s/task/%s", c.baseURL, taskID)
	payload := struct {
		Status string `json:"status"`
	}{Status: status}
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling status payload: %w", err)
	}
	resp, err := c.doRequest(ctx, http.MethodPut, url, strings.NewReader(string(jsonBody)))
	if err != nil {
		return fmt.Errorf("updating task status: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}
