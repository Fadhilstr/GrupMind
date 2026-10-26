package scheduler_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"botteleabsensi/internal/config"
	"botteleabsensi/internal/gemini"
	"botteleabsensi/internal/repository"
	"botteleabsensi/internal/scheduler"
	"botteleabsensi/internal/service"
	"botteleabsensi/internal/telegram"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dummyTele struct{}

func (d *dummyTele) SendMessage(ctx context.Context, chatID int64, text string) error { return nil }
func (d *dummyTele) SendMessageWithMode(ctx context.Context, chatID int64, text, parseMode string) error {
	return nil
}
func (d *dummyTele) GetUpdates(ctx context.Context, offset int64, limit int, timeout int) ([]telegram.Update, error) {
	return nil, nil
}
func (d *dummyTele) GetMe(ctx context.Context) (*telegram.User, error) {
	return &telegram.User{ID: 1}, nil
}
func (d *dummyTele) IsChatAdmin(ctx context.Context, chatID, userID int64) (bool, error) {
	return true, nil
}

func TestScheduler_StartAndStop(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Jakarta")
	require.NoError(t, err)

	cfg := &config.Config{
		Location:                  loc,
		TimezoneName:              "Asia/Jakarta",
		MonevTargetGroupID:        -100123,
		MonevReminderHour1:        15,
		MonevReminderMinute1:      0,
		MonevReminderHour2:        20,
		MonevReminderMinute2:      0,
		MonevRecoveryGraceMinutes: 30,
	}

	repo := repository.NewMockRepository()
	tele := &dummyTele{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	analyzer := gemini.NewMockAnalyzer()

	monevSvc := service.NewMonevService(cfg, repo, repo, tele, logger)
	convSvc := service.NewConversationService(cfg, repo, repo, repo, repo, repo, analyzer, tele, logger)

	sched := scheduler.NewScheduler(cfg, monevSvc, convSvc, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sched.Start(ctx)

	// Beri jeda singkat agar scheduler loop berjalan
	time.Sleep(50 * time.Millisecond)

	// Pastikan berhenti secara graceful tanpa deadlock
	sched.Stop()
	assert.True(t, true)
}

