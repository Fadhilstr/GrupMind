package gemini

// ExtractedTask merepresentasikan tugas yang diekstraksi oleh AI dari percakapan
type ExtractedTask struct {
	Title           string `json:"title"`
	Description     string `json:"description"`
	Assignee        string `json:"assignee"`
	DueDateStr      string `json:"due_date_str"` // Format ISO: YYYY-MM-DDTHH:MM:SS atau YYYY-MM-DD
	Certainty       string `json:"certainty"`    // "pasti", "perlu_konfirmasi", "usulan"
	Status          string `json:"status"`       // "belum_dikerjakan", "sedang_dikerjakan", "selesai", "dibatalkan", "perlu_konfirmasi"
	SourceMessageID int64  `json:"source_message_id"`
}

// ExtractedDecision merepresentasikan keputusan atau kesimpulan diskusi
type ExtractedDecision struct {
	Topic           string `json:"topic"`
	DecisionText    string `json:"decision_text"`
	Status          string `json:"status"` // "final", "usulan", "perlu_konfirmasi", "dibatalkan"
	SourceMessageID int64  `json:"source_message_id"`
}

// ExtractedSchedule merepresentasikan agenda atau jadwal kegiatan
type ExtractedSchedule struct {
	EventName       string `json:"event_name"`
	EventTimeStr    string `json:"event_time_str"` // Format ISO: YYYY-MM-DDTHH:MM:SS
	Location        string `json:"location"`
	SourceMessageID int64  `json:"source_message_id"`
}

// ExtractedChange mencatat pembatalan atau perubahan kesepakatan sebelumnya
type ExtractedChange struct {
	ItemType     string `json:"item_type"` // "task", "decision", "schedule"
	Description  string `json:"description"`
	ChangeReason string `json:"change_reason"`
}

// AnalysisResult adalah struktur hasil ekstraksi percakapan dari Gemini
type AnalysisResult struct {
	Topic                    string              `json:"topic"`
	Summary                  string              `json:"summary"`
	Decisions                []ExtractedDecision `json:"decisions"`
	Tasks                    []ExtractedTask     `json:"tasks"`
	Schedules                []ExtractedSchedule `json:"schedules"`
	UnresolvedQuestions      []string            `json:"unresolved_questions"`
	CancelledOrModifiedItems []ExtractedChange   `json:"cancelled_or_modified_items"`
}

