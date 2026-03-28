package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// ServicePinger は依存サービスへの疎通確認インターフェース。
type ServicePinger interface {
	Ping(ctx context.Context) error
}

// ProjectPingers は1プロジェクト分の ServicePinger ペアを保持する。
type ProjectPingers struct {
	Name    string // "owner/repo" 形式のプロジェクト識別子
	ClickUp ServicePinger
	GitHub  ServicePinger
}

// Handler はヘルスチェックエンドポイントのハンドラ。
type Handler struct {
	projects []ProjectPingers
}

// NewHandler は新しい Handler を生成する。
func NewHandler(projects []ProjectPingers) *Handler {
	return &Handler{projects: projects}
}

type serviceStatus struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type projectStatus struct {
	Services map[string]serviceStatus `json:"services"`
}

type healthResponse struct {
	Status   string                   `json:"status"`
	Projects map[string]projectStatus `json:"projects"`
}

type result struct {
	project string
	service string
	err     error
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	total := len(h.projects) * 2
	results := make(chan result, total)

	for _, proj := range h.projects {
		go func(name string, p ServicePinger) {
			pingCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			results <- result{project: name, service: "clickup", err: p.Ping(pingCtx)}
		}(proj.Name, proj.ClickUp)
		go func(name string, p ServicePinger) {
			pingCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			results <- result{project: name, service: "github", err: p.Ping(pingCtx)}
		}(proj.Name, proj.GitHub)
	}

	projects := make(map[string]projectStatus, len(h.projects))
	degraded := false
	for range total {
		res := <-results
		ps, ok := projects[res.project]
		if !ok {
			ps = projectStatus{Services: make(map[string]serviceStatus)}
		}
		if res.err != nil {
			degraded = true
			ps.Services[res.service] = serviceStatus{Status: "error", Message: res.err.Error()}
		} else {
			ps.Services[res.service] = serviceStatus{Status: "ok"}
		}
		projects[res.project] = ps
	}

	resp := healthResponse{Projects: projects}
	statusCode := http.StatusOK
	if degraded {
		resp.Status = "degraded"
		statusCode = http.StatusServiceUnavailable
	} else {
		resp.Status = "ok"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(resp)
}
