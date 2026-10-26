package test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"botteleabsensi/internal/config"
	"botteleabsensi/internal/gemini"
	"botteleabsensi/internal/handler"
	"botteleabsensi/internal/model"
	"botteleabsensi/internal/repository"
	"botteleabsensi/internal/service"
	"botteleabsensi/internal/telegram"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordedMessage struct {
	ChatID    int64
	Text      string
	ParseMode string
}

type integrationTeleClient struct {
	mu           sync.Mutex
	sentMessages []recordedMessage
	failSend     bool
}

func newIntegrationTeleClient() *integrationTeleClient {
	return &integrationTeleClient{}
}

func (c *integrationTeleClient) SendMessage(ctx context.Context, chatID int64, text string) error {
	return c.SendMessageWithMode(ctx, chatID, text, "")
}

func (c *integrationTeleClient) SendMessageWithMode(ctx context.Context, chatID int64, text, parseMode string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failSend {
		return fmt.Errorf("telegram API 500 error simulasi")
	}
	c.sentMessages = append(c.sentMessages, recordedMessage{
		ChatID:    chatID,
		Text:      text,
		ParseMode: parseMode,
	})
	return nil
}

func (c *integrationTeleClient) GetUpdates(ctx context.Context, offset int64, limit int, timeout int) ([]telegram.Update, error) {
	return nil, nil
}

func (c *integrationTeleClient) GetMe(ctx context.Context) (*telegram.User, error) {
	return &telegram.User{ID: 99999, FirstName: "MonevAgentBot", Username: "monev_agent_bot"}, nil
}

func (c *integrationTeleClient) IsChatAdmin(ctx context.Context, chatID, userID int64) (bool, error) {
	return userID == 100, nil // User ID 100 diakui sebagai admin
}

func (c *integrationTeleClient) getSentMessages() []recordedMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	res := make([]recordedMessage, len(c.sentMessages))
	copy(res, c.sentMessages)
	return res
}

func (c *integrationTeleClient) clearSentMessages() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sentMessages = nil
}

func TestEndToEnd_CompleteWorkflow(t *testing.T) {
	ctx := context.Background()

	// 1. Inisialisasi Konfigurasi & Zona Waktu
	loc, err := time.LoadLocation("Asia/Jakarta")
	require.NoError(t, err)

	targetGroupID := int64(-100888999)
	adminUserID := int64(100)
	normalUserID := int64(200)

	cfg := &config.Config{
		Location:                  loc,
		TimezoneName:              "Asia/Jakarta",
		AllowedGroups:             []int64{targetGroupID},
		AdminUserIDs:              []int64{adminUserID},
		MonevTargetGroupID:        0, // Mulai dari 0 untuk menguji aktivasi via /setgroup
		MonevReminderHour1:        15,
		MonevReminderMinute1:      0,
		MonevReminderHour2:        20,
		MonevReminderMinute2:      0,
		MonevRecoveryGraceMinutes: 30,
	}

	repo := repository.NewMockRepository()
	tele := newIntegrationTeleClient()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	analyzer := gemini.NewMockAnalyzer()

	monevSvc := service.NewMonevService(cfg, repo, repo, tele, logger)
	convSvc := service.NewConversationService(cfg, repo, repo, repo, repo, repo, analyzer, tele, logger)
	botHandler := handler.NewHandler(cfg, tele, monevSvc, convSvc, repo, repo, repo, repo, repo, repo, logger)

	// =========================================================================
	// LANGKAH 1: Pengguna bukan admin mencoba /setgroup -> HARUS DITOLAK
	// =========================================================================
	err = botHandler.ProcessUpdate(ctx, &telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 1,
			Date:      time.Now().Unix(),
			Chat:      telegram.Chat{ID: targetGroupID, Type: "supergroup", Title: "Grup Divisi IT"},
			From:      &telegram.User{ID: normalUserID, FirstName: "Budi"},
			Text:      "/setgroup",
		},
	})
	require.NoError(t, err)
	msgs := tele.getSentMessages()
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Text, "tidak memiliki wewenang")
	assert.Equal(t, int64(0), monevSvc.GetTargetGroupID())
	tele.clearSentMessages()

	// =========================================================================
	// LANGKAH 2: Admin menjalankan /setgroup -> BERHASIL
	// =========================================================================
	err = botHandler.ProcessUpdate(ctx, &telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID: 2,
			Date:      time.Now().Unix(),
			Chat:      telegram.Chat{ID: targetGroupID, Type: "supergroup", Title: "Grup Divisi IT"},
			From:      &telegram.User{ID: adminUserID, FirstName: "Pak Admin"},
			Text:      "/setgroup",
		},
	})
	require.NoError(t, err)
	msgs = tele.getSentMessages()
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Text, "Target Pengingat Monev Berhasil Ditetapkan")
	assert.Equal(t, targetGroupID, monevSvc.GetTargetGroupID())
	tele.clearSentMessages()

	// =========================================================================
	// LANGKAH 3: Simulasi Percakapan Grup & Ekstraksi AI
	// =========================================================================
	// Rangkaian percakapan multi-pesan
	refTime := time.Date(2026, 10, 26, 10, 0, 0, 0, loc)
	convMessages := []*model.SourceMessage{
		{
			MessageID:  10,
			ChatID:     targetGroupID,
			SenderID:   normalUserID,
			SenderName: "Budi",
			Text:       "Kita kerjakan modul laporan hari Kamis.",
			SentAt:     refTime,
		},
		{
			MessageID:  11,
			ChatID:     targetGroupID,
			SenderID:   adminUserID,
			SenderName: "Fadhil",
			Text:       "Bagian export PDF saya kerjakan.",
			SentAt:     refTime.Add(time.Minute),
		},
		{
			MessageID:  12,
			ChatID:     targetGroupID,
			SenderID:   normalUserID,
			SenderName: "Budi",
			Text:       "Deadline-nya Jumat sore saja ya sudah sepakat.",
			SentAt:     refTime.Add(2 * time.Minute),
		},
	}

	for _, m := range convMessages {
		err = repo.SaveMessage(ctx, m)
		require.NoError(t, err)
	}

	// Atur hasil ekstraksi analisis percakapan secara spesifik
	analyzer.CustomResult = &gemini.AnalysisResult{
		Topic:   "Pengerjaan Modul Laporan dan Absensi",
		Summary: "Tim menyepakati pembagian tugas dan tenggat waktu laporan.",
		Decisions: []gemini.ExtractedDecision{
			{
				Topic:           "Tenggat Waktu",
				DecisionText:    "Deadline disepakati Jumat sore",
				Status:          "final",
				SourceMessageID: 12,
			},
		},
		Tasks: []gemini.ExtractedTask{
			{
				Title:           "Pengerjaan Modul Export PDF",
				Description:     "Mengerjakan bagian export PDF modul laporan",
				Assignee:        "Fadhil",
				DueDateStr:      "2026-10-30T17:00:00",
				Certainty:       "pasti",
				Status:          "belum_dikerjakan",
				SourceMessageID: 11,
			},
		},
	}

	// Jalankan sinkronisasi analisis dan simpan ke database
	err = convSvc.AnalyzeAndApply(ctx, targetGroupID, convMessages)
	require.NoError(t, err)

	// Verifikasi Tugas Tersimpan
	tasks, err := repo.ListTasks(ctx, targetGroupID, nil)
	require.NoError(t, err)
	require.NotEmpty(t, tasks)
	var foundTask *model.Task
	for _, t := range tasks {
		if t.Assignee == "Fadhil" {
			foundTask = t
			break
		}
	}
	require.NotNil(t, foundTask, "Tugas dengan penanggung jawab 'Fadhil' harus diekstrak")
	assert.Contains(t, foundTask.Title, "Export PDF")

	// Verifikasi Keputusan Tersimpan
	decisions, err := repo.ListDecisions(ctx, targetGroupID)
	require.NoError(t, err)
	require.NotEmpty(t, decisions)
	assert.Equal(t, model.DecisionStatusFinal, decisions[0].Status)

	// =========================================================================
	// LANGKAH 4: Perintah /tasks dan /done
	// =========================================================================
	err = botHandler.ProcessUpdate(ctx, &telegram.Update{
		UpdateID: 3,
		Message: &telegram.Message{
			MessageID: 15,
			Date:      time.Now().Unix(),
			Chat:      telegram.Chat{ID: targetGroupID, Type: "supergroup"},
			From:      &telegram.User{ID: normalUserID, FirstName: "Budi"},
			Text:      "/tasks",
		},
	})
	require.NoError(t, err)
	msgs = tele.getSentMessages()
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Text, "Export PDF")
	tele.clearSentMessages()

	// Selesaikan tugas dengan /done
	doneCmd := fmt.Sprintf("/done %d", foundTask.ID)
	err = botHandler.ProcessUpdate(ctx, &telegram.Update{
		UpdateID: 4,
		Message: &telegram.Message{
			MessageID: 16,
			Date:      time.Now().Unix(),
			Chat:      telegram.Chat{ID: targetGroupID, Type: "supergroup"},
			From:      &telegram.User{ID: adminUserID, FirstName: "Fadhil"},
			Text:      doneCmd,
		},
	})
	require.NoError(t, err)
	msgs = tele.getSentMessages()
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Text, "Tugas Berhasil Diselesaikan")
	tele.clearSentMessages()

	// Pastikan status di database berubah menjadi 'selesai' dan ada riwayatnya
	completedTask, err := repo.GetTaskByID(ctx, foundTask.ID)
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSelesai, completedTask.Status)

	histories, err := repo.GetTaskHistories(ctx, foundTask.ID)
	require.NoError(t, err)
	require.Len(t, histories, 1)
	assert.Equal(t, "selesai", histories[0].NewStatus)

	// =========================================================================
	// LANGKAH 5: Pengingat Monev Pukul 15.00 WIB (Senin-Sabtu)
	// =========================================================================
	// Hari Senin pukul 15.00 WIB
	monday1500 := time.Date(2026, 10, 26, 15, 0, 0, 0, loc)
	require.Equal(t, time.Monday, monday1500.Weekday())

	err = monevSvc.CheckAndRunScheduledReminders(ctx, monday1500)
	require.NoError(t, err)

	msgs = tele.getSentMessages()
	require.Len(t, msgs, 1)
	assert.Equal(t, targetGroupID, msgs[0].ChatID, "Pengingat Monev HARUS dikirim ke grup target")
	assert.Contains(t, msgs[0].Text, "Pengingat Absensi Monev — Pukul 15.00 WIB")
	tele.clearSentMessages()

	// =========================================================================
	// LANGKAH 6: Pengingat Monev Pukul 20.00 WIB (Senin-Sabtu)
	// =========================================================================
	monday2000 := time.Date(2026, 10, 26, 20, 0, 0, 0, loc)
	err = monevSvc.CheckAndRunScheduledReminders(ctx, monday2000)
	require.NoError(t, err)

	msgs = tele.getSentMessages()
	require.Len(t, msgs, 1)
	assert.Equal(t, targetGroupID, msgs[0].ChatID)
	assert.Contains(t, msgs[0].Text, "Pengingat Kedua Absensi Monev — Pukul 20.00 WIB")
	tele.clearSentMessages()

	// =========================================================================
	// LANGKAH 7: Pengecualian Hari Minggu — TIDAK BOLEH ADA PENGINGAT
	// =========================================================================
	sunday1500 := time.Date(2026, 11, 1, 15, 0, 0, 0, loc)
	require.Equal(t, time.Sunday, sunday1500.Weekday())

	err = monevSvc.CheckAndRunScheduledReminders(ctx, sunday1500)
	require.NoError(t, err)

	sunday2000 := time.Date(2026, 11, 1, 20, 0, 0, 0, loc)
	err = monevSvc.CheckAndRunScheduledReminders(ctx, sunday2000)
	require.NoError(t, err)

	assert.Empty(t, tele.getSentMessages(), "Hari Minggu mutlak tidak boleh mengirim pesan pengingat")

	// =========================================================================
	// LANGKAH 8: Pencegahan Pengiriman Ganda (Idempotensi)
	// =========================================================================
	// Picu kembali pengingat 15:00 di hari Senin yang sama
	err = monevSvc.SendReminder(ctx, service.Slot1500)
	require.NoError(t, err)
	assert.Empty(t, tele.getSentMessages(), "Slot 15:00 yang sudah terkirim hari ini tidak boleh dikirim ulang")

	// =========================================================================
	// LANGKAH 9: Pemulihan Setelah Restart (Recovery)
	// =========================================================================
	// Kasus A: Server menyala pada hari Selasa pukul 15:10 WIB (dalam batas grace period 30 menit)
	tuesday1510 := time.Date(2026, 10, 27, 15, 10, 0, 0, loc)
	require.Equal(t, time.Tuesday, tuesday1510.Weekday())

	err = monevSvc.RecoverMissedReminders(ctx, tuesday1510)
	require.NoError(t, err)

	msgs = tele.getSentMessages()
	require.Len(t, msgs, 1, "Pengingat dalam toleransi waktu harus dikirim")
	assert.Contains(t, msgs[0].Text, "Pukul 15.00 WIB")
	tele.clearSentMessages()

	// Kasus B: Server mati dari jam 14:00 sampai 17:00 (melebihi batas grace period 30 menit)
	// Pada hari Rabu pukul 17:00
	wednesday1700 := time.Date(2026, 10, 28, 17, 0, 0, 0, loc)
	err = monevSvc.RecoverMissedReminders(ctx, wednesday1700)
	require.NoError(t, err)

	assert.Empty(t, tele.getSentMessages(), "Pengingat yang sudah basi tidak boleh dikirim")
	log15, err := repo.GetLog(ctx, targetGroupID, "2026-10-28", service.Slot1500)
	require.NoError(t, err)
	require.NotNil(t, log15)
	assert.Equal(t, model.MonevStatusMissed, log15.Status)

	// =========================================================================
	// LANGKAH 10: Penanganan Kegagalan Telegram API
	// =========================================================================
	tele.failSend = true
	thursday2000 := time.Date(2026, 10, 29, 20, 0, 0, 0, loc)
	err = monevSvc.SendReminderAt(ctx, thursday2000, service.Slot2000)
	require.Error(t, err)

	// Pastikan status di database adalah 'failed', BUKAN 'success'
	thursdayStr := thursday2000.Format("2006-01-02")
	sent, err := repo.IsAlreadySent(ctx, targetGroupID, thursdayStr, service.Slot2000)
	require.NoError(t, err)
	assert.False(t, sent, "Kegagalan pengiriman tidak boleh ditandai sebagai sukses")

	tele.failSend = false
}
