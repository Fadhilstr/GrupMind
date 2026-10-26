package handler_test

import (
	"context"
	"io"
	"log/slog"
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

type mockClientForHandler struct {
	sentMessages []telegram.SendMessageRequest
}

func (m *mockClientForHandler) SendMessage(ctx context.Context, chatID int64, text string) error {
	return m.SendMessageWithMode(ctx, chatID, text, "")
}

func (m *mockClientForHandler) SendMessageWithMode(ctx context.Context, chatID int64, text, parseMode string) error {
	m.sentMessages = append(m.sentMessages, telegram.SendMessageRequest{
		ChatID:    chatID,
		Text:      text,
		ParseMode: parseMode,
	})
	return nil
}

func (m *mockClientForHandler) GetUpdates(ctx context.Context, offset int64, limit int, timeout int) ([]telegram.Update, error) {
	return nil, nil
}

func (m *mockClientForHandler) GetMe(ctx context.Context) (*telegram.User, error) {
	return &telegram.User{ID: 999, FirstName: "Bot"}, nil
}

func (m *mockClientForHandler) IsChatAdmin(ctx context.Context, chatID, userID int64) (bool, error) {
	return userID == 100, nil // User 100 adalah admin
}

func setupHandlerTest(t *testing.T) (*handler.Handler, *mockClientForHandler, *repository.MockRepository, *config.Config) {
	loc, err := time.LoadLocation("Asia/Jakarta")
	require.NoError(t, err)

	cfg := &config.Config{
		Location:           loc,
		TimezoneName:       "Asia/Jakarta",
		AllowedGroups:      []int64{-100111},
		AdminUserIDs:       []int64{100},
		MonevTargetGroupID: -100111,
		MonevReminderHour1: 15,
		MonevReminderHour2: 20,
	}

	repo := repository.NewMockRepository()
	tele := &mockClientForHandler{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	analyzer := gemini.NewMockAnalyzer()

	monevSvc := service.NewMonevService(cfg, repo, repo, tele, logger)
	convSvc := service.NewConversationService(cfg, repo, repo, repo, repo, repo, analyzer, tele, logger)

	h := handler.NewHandler(cfg, tele, monevSvc, convSvc, repo, repo, repo, repo, repo, repo, logger)
	return h, tele, repo, cfg
}

func TestHandler_StartAndHelp(t *testing.T) {
	ctx := context.Background()
	h, tele, _, _ := setupHandlerTest(t)

	// Test /start
	err := h.ProcessUpdate(ctx, &telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 1,
			Date:      time.Now().Unix(),
			Chat:      telegram.Chat{ID: 12345, Type: "private"},
			From:      &telegram.User{ID: 12345, FirstName: "UserTest"},
			Text:      "/start",
		},
	})
	require.NoError(t, err)
	require.Len(t, tele.sentMessages, 1)
	assert.Contains(t, tele.sentMessages[0].Text, "Selamat datang di AI Agent Absensi Monev")

	// Test /help
	err = h.ProcessUpdate(ctx, &telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID: 2,
			Date:      time.Now().Unix(),
			Chat:      telegram.Chat{ID: 12345, Type: "private"},
			From:      &telegram.User{ID: 12345, FirstName: "UserTest"},
			Text:      "/help",
		},
	})
	require.NoError(t, err)
	require.Len(t, tele.sentMessages, 2)
	assert.Contains(t, tele.sentMessages[1].Text, "Panduan Penggunaan")
}

func TestHandler_SetGroup_AdminOnly(t *testing.T) {
	ctx := context.Background()
	h, tele, _, _ := setupHandlerTest(t)

	// User bukan admin (ID: 200) mencoba /setgroup
	err := h.ProcessUpdate(ctx, &telegram.Update{
		UpdateID: 3,
		Message: &telegram.Message{
			MessageID: 3,
			Date:      time.Now().Unix(),
			Chat:      telegram.Chat{ID: -100111, Type: "supergroup", Title: "Grup Proyek"},
			From:      &telegram.User{ID: 200, FirstName: "BukanAdmin"},
			Text:      "/setgroup",
		},
	})
	require.NoError(t, err)
	require.Len(t, tele.sentMessages, 1)
	assert.Contains(t, tele.sentMessages[0].Text, "Anda tidak memiliki wewenang")

	// User admin (ID: 100) menjalankan /setgroup
	err = h.ProcessUpdate(ctx, &telegram.Update{
		UpdateID: 4,
		Message: &telegram.Message{
			MessageID: 4,
			Date:      time.Now().Unix(),
			Chat:      telegram.Chat{ID: -100111, Type: "supergroup", Title: "Grup Proyek"},
			From:      &telegram.User{ID: 100, FirstName: "PakAdmin"},
			Text:      "/setgroup",
		},
	})
	require.NoError(t, err)
	require.Len(t, tele.sentMessages, 2)
	assert.Contains(t, tele.sentMessages[1].Text, "Target Pengingat Monev Berhasil Ditetapkan")
}

func TestHandler_TaskLifecycle_TasksAndDone(t *testing.T) {
	ctx := context.Background()
	h, tele, repo, _ := setupHandlerTest(t)

	// Tambahkan tugas ke repo
	due := time.Now().Add(48 * time.Hour)
	taskID, err := repo.CreateTask(ctx, &model.Task{
		GroupID:     -100111,
		Title:       "Menyusun Modul Absensi",
		Description: "Buat integrasi telegram",
		Assignee:    "Fadhil",
		DueDate:     &due,
		Status:      model.TaskStatusBelumDikerjakan,
		Certainty:   "pasti",
	})
	require.NoError(t, err)

	// Perintah /tasks
	err = h.ProcessUpdate(ctx, &telegram.Update{
		UpdateID: 5,
		Message: &telegram.Message{
			MessageID: 5,
			Date:      time.Now().Unix(),
			Chat:      telegram.Chat{ID: -100111, Type: "supergroup"},
			From:      &telegram.User{ID: 100, FirstName: "Fadhil"},
			Text:      "/tasks",
		},
	})
	require.NoError(t, err)
	require.Len(t, tele.sentMessages, 1)
	assert.Contains(t, tele.sentMessages[0].Text, "Menyusun Modul Absensi")

	// Perintah /done 1
	err = h.ProcessUpdate(ctx, &telegram.Update{
		UpdateID: 6,
		Message: &telegram.Message{
			MessageID: 6,
			Date:      time.Now().Unix(),
			Chat:      telegram.Chat{ID: -100111, Type: "supergroup"},
			From:      &telegram.User{ID: 100, FirstName: "Fadhil"},
			Text:      "/done 1",
		},
	})
	require.NoError(t, err)
	require.Len(t, tele.sentMessages, 2)
	assert.Contains(t, tele.sentMessages[1].Text, "Tugas Berhasil Diselesaikan")

	// Verifikasi status di DB
	updatedTask, err := repo.GetTaskByID(ctx, taskID)
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSelesai, updatedTask.Status)
}

func TestHandler_UnauthorizedGroup_Rejected(t *testing.T) {
	ctx := context.Background()
	h, tele, _, _ := setupHandlerTest(t)

	// Grup -100999 tidak ada di AllowedGroups dan bukan targetID
	err := h.ProcessUpdate(ctx, &telegram.Update{
		UpdateID: 7,
		Message: &telegram.Message{
			MessageID: 7,
			Date:      time.Now().Unix(),
			Chat:      telegram.Chat{ID: -100999, Type: "supergroup"},
			From:      &telegram.User{ID: 50, FirstName: "Stranger"},
			Text:      "/tasks",
		},
	})
	require.NoError(t, err)
	require.Len(t, tele.sentMessages, 1)
	assert.Contains(t, tele.sentMessages[0].Text, "Grup ini belum diizinkan")
}

