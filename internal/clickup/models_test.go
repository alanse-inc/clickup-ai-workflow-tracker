package clickup

import "testing"

func TestIsTriggerStatus(t *testing.T) {
	sm := DefaultStatusMapping()
	tests := []struct {
		status string
		want   bool
	}{
		{sm.ReadyForSpec, true},
		{sm.ReadyForCode, true},
		{"idea draft", false},
		{sm.GeneratingSpec, false},
		{sm.Implementing, false},
		{sm.Closed, false},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			if got := sm.IsTriggerStatus(tt.status); got != tt.want {
				t.Errorf("IsTriggerStatus(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestIsTriggerStatus_Custom(t *testing.T) {
	sm := StatusMapping{
		ReadyForSpec:   "custom ready spec",
		GeneratingSpec: "custom generating",
		SpecReview:     "custom review",
		ReadyForCode:   "custom ready code",
		Implementing:   "custom implementing",
		PRReview:       "custom pr review",
		Closed:         "custom closed",
	}
	if !sm.IsTriggerStatus("custom ready spec") {
		t.Error("expected custom ready spec to be trigger status")
	}
	if sm.IsTriggerStatus("ready for spec") {
		t.Error("expected default status not to match custom mapping")
	}
}

func TestIsProcessingStatus(t *testing.T) {
	sm := DefaultStatusMapping()
	tests := []struct {
		status string
		want   bool
	}{
		{sm.GeneratingSpec, true},
		{sm.Implementing, true},
		{sm.ReadyForSpec, false},
		{sm.Closed, false},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			if got := sm.IsProcessingStatus(tt.status); got != tt.want {
				t.Errorf("IsProcessingStatus(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestIsTerminalStatus(t *testing.T) {
	sm := DefaultStatusMapping()
	tests := []struct {
		status string
		want   bool
	}{
		{sm.Closed, true},
		{sm.ReadyForSpec, false},
		{sm.Implementing, false},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			if got := sm.IsTerminalStatus(tt.status); got != tt.want {
				t.Errorf("IsTerminalStatus(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestPhaseFromStatus(t *testing.T) {
	sm := DefaultStatusMapping()
	tests := []struct {
		status  string
		want    Phase
		wantErr bool
	}{
		{sm.ReadyForSpec, PhaseSpec, false},
		{sm.GeneratingSpec, PhaseSpec, false},
		{sm.SpecReview, PhaseSpec, false},
		{sm.ReadyForCode, PhaseCode, false},
		{sm.Implementing, PhaseCode, false},
		{sm.PRReview, PhaseCode, false},
		{"idea draft", "", true},
		{sm.Closed, "", true},
		{"unknown", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got, err := sm.PhaseFromStatus(tt.status)
			if (err != nil) != tt.wantErr {
				t.Errorf("PhaseFromStatus(%q) error = %v, wantErr %v", tt.status, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("PhaseFromStatus(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestProcessingStatusFor(t *testing.T) {
	sm := DefaultStatusMapping()
	tests := []struct {
		phase Phase
		want  string
	}{
		{PhaseSpec, sm.GeneratingSpec},
		{PhaseCode, sm.Implementing},
		{Phase("UNKNOWN"), ""},
	}
	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			if got := sm.ProcessingStatusFor(tt.phase); got != tt.want {
				t.Errorf("ProcessingStatusFor(%q) = %q, want %q", tt.phase, got, tt.want)
			}
		})
	}
}

func TestSuccessStatusFor(t *testing.T) {
	sm := DefaultStatusMapping()
	tests := []struct {
		phase Phase
		want  string
	}{
		{PhaseSpec, sm.SpecReview},
		{PhaseCode, sm.PRReview},
		{Phase("UNKNOWN"), ""},
	}
	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			if got := sm.SuccessStatusFor(tt.phase); got != tt.want {
				t.Errorf("SuccessStatusFor(%q) = %q, want %q", tt.phase, got, tt.want)
			}
		})
	}
}

func TestErrorStatusFor(t *testing.T) {
	sm := DefaultStatusMapping()
	tests := []struct {
		phase Phase
		want  string
	}{
		{PhaseSpec, sm.ReadyForSpec},
		{PhaseCode, sm.ReadyForCode},
		{Phase("UNKNOWN"), ""},
	}
	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			if got := sm.ErrorStatusFor(tt.phase); got != tt.want {
				t.Errorf("ErrorStatusFor(%q) = %q, want %q", tt.phase, got, tt.want)
			}
		})
	}
}

func TestAllStatuses(t *testing.T) {
	sm := DefaultStatusMapping()
	statuses := sm.AllStatuses()
	if len(statuses) != 7 {
		t.Fatalf("expected 7 statuses, got %d", len(statuses))
	}
	expected := []string{
		"ready for spec", "generating spec", "spec review",
		"ready for code", "implementing", "pr review", "closed",
	}
	for i, s := range expected {
		if statuses[i] != s {
			t.Errorf("AllStatuses()[%d] = %q, want %q", i, statuses[i], s)
		}
	}
}
