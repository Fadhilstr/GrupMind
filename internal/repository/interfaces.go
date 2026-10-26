package repository

import (
	"context"
	"time"

	"botteleabsensi/internal/model"
)

// GroupRepository mengelola operasi database untuk grup Telegram
type GroupRepository interface {
	UpsertGroup(ctx context.Context, group *model.Group) error
	GetGroup(ctx context.Context, id int64) (*model.Group, error)
	SetGroupAllowed(ctx context.Context, id int64, title string, allowed bool) error
	ListAllowedGroups(ctx context.Context) ([]*model.Group, error)
}

// UserRepository mengelola data pengguna Telegram
type UserRepository interface {
	UpsertUser(ctx context.Context, user *model.User) error
	GetUser(ctx context.Context, id int64) (*model.User, error)
}

// MessageRepository mencatat audit trail pesan percakapan
type MessageRepository interface {
	SaveMessage(ctx context.Context, msg *model.SourceMessage) error
	GetRecentMessages(ctx context.Context, chatID int64, limit int) ([]*model.SourceMessage, error)
	GetMessagesBetween(ctx context.Context, chatID int64, start, end time.Time) ([]*model.SourceMessage, error)
}

// TaskRepository mengelola data tugas dan riwayat perubahan
type TaskRepository interface {
	CreateTask(ctx context.Context, task *model.Task) (int64, error)
	GetTaskByID(ctx context.Context, id int64) (*model.Task, error)
	ListTasks(ctx context.Context, groupID int64, status *model.TaskStatus) ([]*model.Task, error)
	ListTodayTasks(ctx context.Context, groupID int64, loc *time.Location) ([]*model.Task, error)
	ListUpcomingTasks(ctx context.Context, groupID int64, loc *time.Location) ([]*model.Task, error)
	UpdateTaskStatus(ctx context.Context, taskID int64, newStatus model.TaskStatus, changedBy, reason string) error
	UpdateTaskDueDate(ctx context.Context, taskID int64, newDueDate *time.Time, changedBy, reason string) error
	GetTaskHistories(ctx context.Context, taskID int64) ([]*model.TaskHistory, error)
}

// DecisionRepository mengelola keputusan dan kesimpulan diskusi
type DecisionRepository interface {
	CreateDecision(ctx context.Context, decision *model.Decision) (int64, error)
	ListDecisions(ctx context.Context, groupID int64) ([]*model.Decision, error)
	UpdateDecisionStatus(ctx context.Context, id int64, status model.DecisionStatus) error
}

// ScheduleRepository mengelola jadwal agenda kegiatan
type ScheduleRepository interface {
	CreateSchedule(ctx context.Context, schedule *model.Schedule) (int64, error)
	ListUpcomingSchedules(ctx context.Context, groupID int64, from time.Time) ([]*model.Schedule, error)
}

// SummaryRepository menyimpan rangkuman harian percakapan
type SummaryRepository interface {
	SaveDailySummary(ctx context.Context, summary *model.DailySummary) error
	GetDailySummary(ctx context.Context, groupID int64, date string) (*model.DailySummary, error)
	GetLatestDailySummary(ctx context.Context, groupID int64) (*model.DailySummary, error)
}

// MonevRepository mengelola log status pengiriman pengingat Monev
type MonevRepository interface {
	// TryLockReminderLog mencoba mencatat log pengingat pending. Mengembalikan true jika berhasil lock (belum pernah success/pending aktif)
	TryLockReminderLog(ctx context.Context, groupID int64, date, slot string) (bool, error)
	MarkSuccess(ctx context.Context, groupID int64, date, slot string, sentAt time.Time) error
	MarkFailed(ctx context.Context, groupID int64, date, slot string, lastErr string) error
	MarkMissed(ctx context.Context, groupID int64, date, slot string, reason string) error
	IsAlreadySent(ctx context.Context, groupID int64, date, slot string) (bool, error)
	GetLog(ctx context.Context, groupID int64, date, slot string) (*model.MonevReminderLog, error)
}

