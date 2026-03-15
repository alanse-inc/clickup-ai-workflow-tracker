package clickup

import "fmt"

// Task はClickUpタスクの正規化モデル
type Task struct {
	ID           string
	Name         string
	Description  string
	Status       string            // 小文字正規化済み
	CustomFields map[string]string // github_pr_url, agent_error
	DateCreated  string
	DateUpdated  string
}

// Phase はタスクの処理フェーズを表す
type Phase string

const (
	PhaseSpec Phase = "SPEC"
	PhaseCode Phase = "CODE"
)

// StatusMapping はClickUpステータス名のマッピングを保持する
type StatusMapping struct {
	ReadyForSpec   string
	GeneratingSpec string
	SpecReview     string
	ReadyForCode   string
	Implementing   string
	PRReview       string
	Closed         string
}

// DefaultStatusMapping はデフォルトのステータスマッピングを返す
func DefaultStatusMapping() StatusMapping {
	return StatusMapping{
		ReadyForSpec:   "ready for spec",
		GeneratingSpec: "generating spec",
		SpecReview:     "spec review",
		ReadyForCode:   "ready for code",
		Implementing:   "implementing",
		PRReview:       "pr review",
		Closed:         "closed",
	}
}

// AllStatuses はマッピング内の全ステータスをスライスで返す
func (sm StatusMapping) AllStatuses() []string {
	return []string{
		sm.ReadyForSpec,
		sm.GeneratingSpec,
		sm.SpecReview,
		sm.ReadyForCode,
		sm.Implementing,
		sm.PRReview,
		sm.Closed,
	}
}

// IsTriggerStatus はトリガー対象のステータスかどうかを返す
func (sm StatusMapping) IsTriggerStatus(status string) bool {
	return status == sm.ReadyForSpec || status == sm.ReadyForCode
}

// IsProcessingStatus は処理中ステータスかどうかを返す
func (sm StatusMapping) IsProcessingStatus(status string) bool {
	return status == sm.GeneratingSpec || status == sm.Implementing
}

// IsTerminalStatus は終端ステータスかどうかを返す
func (sm StatusMapping) IsTerminalStatus(status string) bool {
	return status == sm.Closed
}

// PhaseFromStatus はステータスからフェーズを判定する
func (sm StatusMapping) PhaseFromStatus(status string) (Phase, error) {
	switch status {
	case sm.ReadyForSpec, sm.GeneratingSpec, sm.SpecReview:
		return PhaseSpec, nil
	case sm.ReadyForCode, sm.Implementing, sm.PRReview:
		return PhaseCode, nil
	default:
		return "", fmt.Errorf("cannot determine phase from status: %s", status)
	}
}

// ProcessingStatusFor はフェーズから処理中ステータスを返す
func (sm StatusMapping) ProcessingStatusFor(phase Phase) string {
	switch phase {
	case PhaseSpec:
		return sm.GeneratingSpec
	case PhaseCode:
		return sm.Implementing
	default:
		return ""
	}
}

// SuccessStatusFor はフェーズから成功時ステータスを返す
func (sm StatusMapping) SuccessStatusFor(phase Phase) string {
	switch phase {
	case PhaseSpec:
		return sm.SpecReview
	case PhaseCode:
		return sm.PRReview
	default:
		return ""
	}
}

// ErrorStatusFor はフェーズからエラー時戻しステータスを返す
func (sm StatusMapping) ErrorStatusFor(phase Phase) string {
	switch phase {
	case PhaseSpec:
		return sm.ReadyForSpec
	case PhaseCode:
		return sm.ReadyForCode
	default:
		return ""
	}
}
