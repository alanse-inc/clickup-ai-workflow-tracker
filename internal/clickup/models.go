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

// ステータス定数
// TODO: ステータス名を環境変数で設定可能にし、起動時に ClickUp ボード上に存在するか検証する
const (
	StatusIdeaDraft      = "idea draft"
	StatusReadyForSpec   = "ready for spec"
	StatusGeneratingSpec = "generating spec"
	StatusSpecReview     = "spec review"
	StatusReadyForCode   = "ready for code"
	StatusImplementing   = "implementing"
	StatusPRReview       = "pr review"
	StatusClosed         = "closed"
)

// IsTriggerStatus はトリガー対象のステータスかどうかを返す
func IsTriggerStatus(status string) bool {
	return status == StatusReadyForSpec || status == StatusReadyForCode
}

// IsProcessingStatus は処理中ステータスかどうかを返す
func IsProcessingStatus(status string) bool {
	return status == StatusGeneratingSpec || status == StatusImplementing
}

// IsTerminalStatus は終端ステータスかどうかを返す
func IsTerminalStatus(status string) bool {
	return status == StatusClosed
}

// PhaseFromStatus はステータスからフェーズを判定する
func PhaseFromStatus(status string) (Phase, error) {
	switch status {
	case StatusReadyForSpec, StatusGeneratingSpec, StatusSpecReview:
		return PhaseSpec, nil
	case StatusReadyForCode, StatusImplementing, StatusPRReview:
		return PhaseCode, nil
	default:
		return "", fmt.Errorf("cannot determine phase from status: %s", status)
	}
}

// ProcessingStatusFor はフェーズから処理中ステータスを返す
func ProcessingStatusFor(phase Phase) string {
	switch phase {
	case PhaseSpec:
		return StatusGeneratingSpec
	case PhaseCode:
		return StatusImplementing
	default:
		return ""
	}
}

// SuccessStatusFor はフェーズから成功時ステータスを返す
func SuccessStatusFor(phase Phase) string {
	switch phase {
	case PhaseSpec:
		return StatusSpecReview
	case PhaseCode:
		return StatusPRReview
	default:
		return ""
	}
}

// ErrorStatusFor はフェーズからエラー時戻しステータスを返す
func ErrorStatusFor(phase Phase) string {
	switch phase {
	case PhaseSpec:
		return StatusReadyForSpec
	case PhaseCode:
		return StatusReadyForCode
	default:
		return ""
	}
}
