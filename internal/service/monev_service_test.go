package service_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"botteleabsensi/internal/config"
	"botteleabsensi/internal/model"
	"botteleabsensi/internal/repository"
	"botteleabsensi/internal/service"
	"botteleabsensi/internal/telegram"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockTelegramClient struct {
	mu           sync.Mutex
	sentMessages []telegram.SendMessageRequest
	failSend     bool
}

func newMockTelegramClient() *mockTelegramClient {
	return &mockTelegramClient{}
}

func (m *mockTelegramClient) SendMessage(ctx context.Context, chatID int64, text string) error {
	return m.SendMessageWithMode(ctx, chatID, text, "")
}

func (m *mockTelegramClient) SendMessageWithMode(ctx context.Context, chatID int64, text, parseMode string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failSend {
		return fmt.Errorf("telegram API network timeout error")
	}
	m.sentMessages = append(m.sentMessages, telegram.SendMessageRequest{
		ChatID:    chatID,
		Text:      text,
		ParseMode: parseMode,
	})
	return nil
}

func (m *mockTelegramClient) GetUpdates(ctx context.Context, offset int64, limit int, timeout int) ([]telegram.Update, error) {
	return nil, nil
}

func (m *mockTelegramClient) GetMe(ctx context.Context) (*telegram.User, error) {
	return &telegram.User{ID: 12345, FirstName: "TestBot"}, nil
}

func (m *mockTelegramClient) IsChatAdmin(ctx context.Context, chatID, userID int64) (bool, error) {
	return true, nil
}

func setupMonevTest(t *testing.T) (*service.MonevService, *repository.MockRepository, *mockTelegramClient, *config.Config) {
	loc, err := time.LoadLocation("Asia/Jakarta")
	require.NoError(t, err)

	cfg := &config.Config{
		Location:                  loc,
		TimezoneName:              "Asia/Jakarta",
		MonevTargetGroupID:        -1009999999,
		MonevReminderHour1:        15,
		MonevReminderMinute1:      0,
		MonevReminderHour2:        20,
		MonevReminderMinute2:      0,
		MonevRecoveryGraceMinutes: 30,
	}

	repo := repository.NewMockRepository()
	tele := newMockTelegramClient()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	monevSvc := service.NewMonevService(cfg, repo, repo, tele, logger)
	return monevSvc, repo, tele, cfg
}

func TestMonevService_SendReminder_Success(t *testing.T) {
	ctx := context.Background()
	svc, repo, tele, _ := setupMonevTest(t)

	// Uji slot 15:00
	err := svc.SendReminder(ctx, service.Slot1500)
	require.NoError(t, err)

	require.Len(t, tele.sentMessages, 1)
	assert.Equal(t, int64(-1009999999), tele.sentMessages[0].ChatID)
	assert.Contains(t, tele.sentMessages[0].Text, "Pengingat Absensi Monev — Pukul 15.00 WIB")

	// Pastikan tercatat sukses di repository
	todayStr := time.Now().In(time.FixedZone("WIB", 7*3600)).Format("2006-01-02")
	sent, err := repo.IsAlreadySent(ctx, -1009999999, todayStr, service.Slot1500)
	require.NoError(t, err)
	assert.True(t, sent)

	// Uji Idempotensi: Kirim ulang slot yang sama di hari yang sama
	err = svc.SendReminder(ctx, service.Slot1500)
	require.NoError(t, err)
	assert.Len(t, tele.sentMessages, 1, "Pesan tidak boleh dikirim dua kali untuk slot dan tanggal yang sama")
}

func TestMonevService_SundayExclusion(t *testing.T) {
	ctx := context.Background()
	svc, _, tele, cfg := setupMonevTest(t)

	// Buat waktu hari Minggu (misal 2026-11-01 adalah hari Minggu)
	sundayTime := time.Date(2026, 11, 1, 15, 0, 0, 0, cfg.Location)
	require.Equal(t, time.Sunday, sundayTime.Weekday())

	err := svc.CheckAndRunScheduledReminders(ctx, sundayTime)
	require.NoError(t, err)
	assert.Empty(t, tele.sentMessages, "Tidak boleh ada pengingat yang dikirim pada hari Minggu")
}

func TestMonevService_UnconfiguredTargetGroup(t *testing.T) {
	ctx := context.Background()
	svc, _, tele, cfg := setupMonevTest(t)
	cfg.MonevTargetGroupID = 0
	_ = svc.SetTargetGroup(ctx, 0, "")

	err := svc.SendReminder(ctx, service.Slot1500)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "belum dikonfigurasi")
	assert.Empty(t, tele.sentMessages)
}

func TestMonevService_TelegramAPIFailure(t *testing.T) {
	ctx := context.Background()
	svc, repo, tele, _ := setupMonevTest(t)
	tele.failSend = true

	err := svc.SendReminder(ctx, service.Slot1500)
	require.Error(t, err)

	// Pastikan TIDAK dicatat sebagai sukses di database
	todayStr := time.Now().In(time.FixedZone("WIB", 7*3600)).Format("2006-01-02")
	sent, err := repo.IsAlreadySent(ctx, -1009999999, todayStr, service.Slot1500)
	require.NoError(t, err)
	assert.False(t, sent, "Kegagalan Telegram API tidak boleh dicatat sebagai sukses")

	log, err := repo.GetLog(ctx, -1009999999, todayStr, service.Slot1500)
	require.NoError(t, err)
	require.NotNil(t, log)
	assert.Equal(t, model.MonevStatusFailed, log.Status)
	assert.Contains(t, log.LastError, "network timeout")
}

func TestMonevService_RestartRecovery_WithinGracePeriod(t *testing.T) {
	ctx := context.Background()
	svc, _, tele, cfg := setupMonevTest(t)

	// Asumsikan aplikasi restart pada pukul 15:15 WIB (dalam grace period 30 menit dari 15:00)
	// Pilih hari kerja (misal Senin)
	restartTime := time.Date(2026, 10, 26, 15, 15, 0, 0, cfg.Location)
	require.Equal(t, time.Monday, restartTime.Weekday())

	err := svc.RecoverMissedReminders(ctx, restartTime)
	require.NoError(t, err)

	// Pengingat slot 15:00 harus dipulihkan dan dikirim
	require.Len(t, tele.sentMessages, 1)
	assert.Contains(t, tele.sentMessages[0].Text, "Pukul 15.00 WIB")
}

func TestMonevService_RestartRecovery_OutsideGracePeriod(t *testing.T) {
	ctx := context.Background()
	svc, repo, tele, cfg := setupMonevTest(t)

	// Asumsikan server mati dari pukul 14:00 sampai 17:00 (sudah lewat grace period 30 menit)
	restartTime := time.Date(2026, 10, 26, 17, 0, 0, 0, cfg.Location)

	err := svc.RecoverMissedReminders(ctx, restartTime)
	require.NoError(t, err)

	// Pesan 15:00 yang kadaluarsa TIDAK boleh dikirim (mencegah spam pesan basi)
	assert.Empty(t, tele.sentMessages)

	// Status harus ditandai 'missed'
	todayStr := restartTime.Format("2006-01-02")
	log, err := repo.GetLog(ctx, -1009999999, todayStr, service.Slot1500)
	require.NoError(t, err)
	require.NotNil(t, log)
	assert.Equal(t, model.MonevStatusMissed, log.Status)
}

