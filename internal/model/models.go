package model

import (
	"time"
)

// Status Tugas
type TaskStatus string

const (
	TaskStatusPerluKonfirmasi  TaskStatus = "perlu_konfirmasi"
	TaskStatusBelumDikerjakan  TaskStatus = "belum_dikerjakan"
	TaskStatusSedangDikerjakan TaskStatus = "sedang_dikerjakan"
	TaskStatusSelesai          TaskStatus = "selesai"
	TaskStatusDibatalkan       TaskStatus = "dibatalkan"
)

// Status Keputusan
type DecisionStatus string

const (
	DecisionStatusFinal           DecisionStatus = "final"
	DecisionStatusUsulan          DecisionStatus = "usulan"
	DecisionStatusPerluKonfirmasi DecisionStatus = "perlu_konfirmasi"
	DecisionStatusDibatalkan      DecisionStatus = "dibatalkan"
)

// Status Pengingat Monev
type MonevReminderStatus string

const (
	MonevStatusPending MonevReminderStatus = "pending"
	MonevStatusSuccess MonevReminderStatus = "success"
	MonevStatusFailed  MonevReminderStatus = "failed"
	MonevStatusMissed  MonevReminderStatus = "missed"
)

// Group menyimpan data grup Telegram yang terdaftar
type Group struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	IsAllowed bool      `json:"is_allowed"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// User menyimpan profil pengguna Telegram
type User struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Role      string    `json:"role"` // admin, member
	CreatedAt time.Time `json:"created_at"`
}

// SourceMessage mencatat setiap pesan Telegram yang diterima untuk audit trail
type SourceMessage struct {
	ID          int64     `json:"id"`
	MessageID   int64     `json:"message_id"`
	ChatID      int64     `json:"chat_id"`
	SenderID    int64     `json:"sender_id"`
	SenderName  string    `json:"sender_name"`
	Text        string    `json:"text"`
	SentAt      time.Time `json:"sent_at"`
	ProcessedAt time.Time `json:"processed_at"`
}

// Task menyimpan data tugas hasil ekstraksi dari percakapan
type Task struct {
	ID              int64      `json:"id"`
	GroupID         int64      `json:"group_id"`
	SourceMessageID *int64     `json:"source_message_id,omitempty"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	Assignee        string     `json:"assignee"`
	DueDate         *time.Time `json:"due_date,omitempty"`
	Status          TaskStatus `json:"status"`
	Certainty       string     `json:"certainty"` // pasti, perlu_konfirmasi, usulan
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// TaskHistory mencatat riwayat perubahan status atau tenggat tugas
type TaskHistory struct {
	ID           int64      `json:"id"`
	TaskID       int64      `json:"task_id"`
	ChangedBy    string     `json:"changed_by"`
	OldStatus    string     `json:"old_status"`
	NewStatus    string     `json:"new_status"`
	OldDueDate   *time.Time `json:"old_due_date,omitempty"`
	NewDueDate   *time.Time `json:"new_due_date,omitempty"`
	ChangeReason string     `json:"change_reason"`
	ChangedAt    time.Time  `json:"changed_at"`
}

// Decision mencatat kesimpulan dan keputusan hasil diskusi di grup
type Decision struct {
	ID              int64          `json:"id"`
	GroupID         int64          `json:"group_id"`
	SourceMessageID *int64         `json:"source_message_id,omitempty"`
	Topic           string         `json:"topic"`
	DecisionText    string         `json:"decision_text"`
	Status          DecisionStatus `json:"status"` // final, usulan, perlu_konfirmasi, dibatalkan
	DecidedAt       time.Time      `json:"decided_at"`
	CreatedAt       time.Time      `json:"created_at"`
}

// Schedule mencatat jadwal agenda atau kegiatan
type Schedule struct {
	ID              int64     `json:"id"`
	GroupID         int64     `json:"group_id"`
	SourceMessageID *int64    `json:"source_message_id,omitempty"`
	EventName       string    `json:"event_name"`
	EventTime       time.Time `json:"event_time"`
	Location        string    `json:"location"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}

// DailySummary menyimpan rangkuman percakapan harian
type DailySummary struct {
	ID          int64     `json:"id"`
	GroupID     int64     `json:"group_id"`
	SummaryDate string    `json:"summary_date"` // YYYY-MM-DD
	Content     string    `json:"content"`
	CreatedAt   time.Time `json:"created_at"`
}

// MonevReminderLog memastikan pengingat absensi bersifat idempoten dan tidak terkirim ganda
type MonevReminderLog struct {
	ID           int64               `json:"id"`
	GroupID      int64               `json:"group_id"`
	ReminderDate string              `json:"reminder_date"` // YYYY-MM-DD
	Slot         string              `json:"slot"`          // 15:00 atau 20:00
	Status       MonevReminderStatus `json:"status"`        // pending, success, failed, missed
	Attempts     int                 `json:"attempts"`
	LastError    string              `json:"last_error"`
	SentAt       *time.Time          `json:"sent_at,omitempty"`
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
}

