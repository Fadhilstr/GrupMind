package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"botteleabsensi/internal/config"
	"botteleabsensi/internal/gemini"
	"botteleabsensi/internal/model"
	"botteleabsensi/internal/repository"
	"botteleabsensi/internal/telegram"
)

// ConversationService mengelola penerimaan pesan grup, analisis AI, dan pembaruan tugas/keputusan
type ConversationService struct {
	cfg          *config.Config
	msgRepo      repository.MessageRepository
	taskRepo     repository.TaskRepository
	decisionRepo repository.DecisionRepository
	scheduleRepo repository.ScheduleRepository
	summaryRepo  repository.SummaryRepository
	analyzer     gemini.Analyzer
	teleClient   telegram.Client
	logger       *slog.Logger
}

// NewConversationService membuat instance ConversationService baru
func NewConversationService(
	cfg *config.Config,
	msgRepo repository.MessageRepository,
	taskRepo repository.TaskRepository,
	decisionRepo repository.DecisionRepository,
	scheduleRepo repository.ScheduleRepository,
	summaryRepo repository.SummaryRepository,
	analyzer gemini.Analyzer,
	teleClient telegram.Client,
	logger *slog.Logger,
) *ConversationService {
	return &ConversationService{
		cfg:          cfg,
		msgRepo:      msgRepo,
		taskRepo:     taskRepo,
		decisionRepo: decisionRepo,
		scheduleRepo: scheduleRepo,
		summaryRepo:  summaryRepo,
		analyzer:     analyzer,
		teleClient:   teleClient,
		logger:       logger,
	}
}

// HandleGroupMessage memproses pesan masuk dari grup yang diizinkan
func (s *ConversationService) HandleGroupMessage(ctx context.Context, msg *model.SourceMessage) error {
	// 1. Simpan pesan untuk audit trail
	if err := s.msgRepo.SaveMessage(ctx, msg); err != nil {
		s.logger.Error("Gagal menyimpan pesan sumber", "error", err, "message_id", msg.MessageID)
		return err
	}

	// 2. Jika pesan adalah teks biasa atau diskusi substantif, kumpulkan konteks pesan terakhir
	// dan analisis secara berkala / kontekstual
	recentMsgs, err := s.msgRepo.GetRecentMessages(ctx, msg.ChatID, 10)
	if err != nil {
		return fmt.Errorf("gagal mengambil pesan riwayat: %w", err)
	}

	// Hanya analisis jika ada setidaknya pesan atau kata kunci relevan untuk efisiensi
	if s.isRelevantDiscussion(msg.Text) || len(recentMsgs) >= 5 {
		go func() {
			// Jalankan analisis di background dengan context timeout terpisah
			analyzeCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			if err := s.AnalyzeAndApply(analyzeCtx, msg.ChatID, recentMsgs); err != nil {
				s.logger.Warn("Gagal menganalisis percakapan dengan Gemini", "chat_id", msg.ChatID, "error", err)
			}
		}()
	}

	return nil
}

// isRelevantDiscussion mendeteksi apakah pesan memuat indikasi tugas, keputusan, atau jadwal
func (s *ConversationService) isRelevantDiscussion(text string) bool {
	t := strings.ToLower(text)
	keywords := []string{
		"deadline", "tenggat", "tugas", "kerjakan", "selesaikan", "sepakat",
		"keputusan", "jadwal", "rapat", "besok", "lusa", "jumat", "senin",
		"laporan", "monev", "format", "revisi", "batal", "usul", "bagaimana kalau",
	}
	for _, kw := range keywords {
		if strings.Contains(t, kw) {
			return true
		}
	}
	return false
}

// AnalyzeAndApply mengeksekusi ekstraksi Gemini dan menyimpan tugas, keputusan, dan jadwal
func (s *ConversationService) AnalyzeAndApply(ctx context.Context, chatID int64, msgs []*model.SourceMessage) error {
	s.logger.Info("Menganalisis diskusi percakapan grup...", "chat_id", chatID, "message_count", len(msgs))

	now := time.Now().In(s.cfg.Location)
	result, err := s.analyzer.AnalyzeConversation(ctx, msgs, now)
	if err != nil {
		return err
	}

	// 1. Simpan atau perbarui Tugas (Tasks)
	for _, extTask := range result.Tasks {
		var dueDate *time.Time
		if extTask.DueDateStr != "" {
			parsedTime, parseErr := time.ParseInLocation("2006-01-02T15:04:05", extTask.DueDateStr, s.cfg.Location)
			if parseErr != nil {
				parsedDateOnly, err2 := time.ParseInLocation("2006-01-02", extTask.DueDateStr, s.cfg.Location)
				if err2 == nil {
					parsedTime = parsedDateOnly
					dueDate = &parsedTime
				}
			} else {
				dueDate = &parsedTime
			}
		}

		taskStatus := model.TaskStatusBelumDikerjakan
		if extTask.Status == "sedang_dikerjakan" {
			taskStatus = model.TaskStatusSedangDikerjakan
		} else if extTask.Status == "perlu_konfirmasi" || extTask.Certainty == "perlu_konfirmasi" {
			taskStatus = model.TaskStatusPerluKonfirmasi
		}

		// Cek duplikasi tugas aktif di grup
		existingTasks, _ := s.taskRepo.ListTasks(ctx, chatID, nil)
		var duplicate bool
		for _, et := range existingTasks {
			if strings.EqualFold(et.Title, extTask.Title) && et.Status != model.TaskStatusSelesai && et.Status != model.TaskStatusDibatalkan {
				duplicate = true
				// Jika ada pembaruan tenggat waktu
				if dueDate != nil && (et.DueDate == nil || !et.DueDate.Equal(*dueDate)) {
					_ = s.taskRepo.UpdateTaskDueDate(ctx, et.ID, dueDate, "AI Agent (Gemini)", "Pembaruan tenggat dari percakapan")
				}
				break
			}
		}

		if !duplicate {
			var srcMsgID *int64
			if extTask.SourceMessageID != 0 {
				srcMsgID = &extTask.SourceMessageID
			}
			_, _ = s.taskRepo.CreateTask(ctx, &model.Task{
				GroupID:         chatID,
				SourceMessageID: srcMsgID,
				Title:           extTask.Title,
				Description:     extTask.Description,
				Assignee:        extTask.Assignee,
				DueDate:         dueDate,
				Status:          taskStatus,
				Certainty:       extTask.Certainty,
			})
			s.logger.Info("Tugas baru dicatat dari percakapan", "title", extTask.Title, "assignee", extTask.Assignee)
		}
	}

	// 2. Simpan Keputusan (Decisions)
	for _, extDec := range result.Decisions {
		status := model.DecisionStatusFinal
		if extDec.Status == "usulan" {
			status = model.DecisionStatusUsulan
		} else if extDec.Status == "perlu_konfirmasi" {
			status = model.DecisionStatusPerluKonfirmasi
		}

		var srcMsgID *int64
		if extDec.SourceMessageID != 0 {
			srcMsgID = &extDec.SourceMessageID
		}

		_, _ = s.decisionRepo.CreateDecision(ctx, &model.Decision{
			GroupID:         chatID,
			SourceMessageID: srcMsgID,
			Topic:           extDec.Topic,
			DecisionText:    extDec.DecisionText,
			Status:          status,
			DecidedAt:       now,
		})
		s.logger.Info("Keputusan baru dicatat", "topic", extDec.Topic, "status", status)
	}

	// 3. Simpan Jadwal Kegiatan (Schedules)
	for _, extSch := range result.Schedules {
		if extSch.EventTimeStr != "" {
			eventTime, err := time.ParseInLocation("2006-01-02T15:04:05", extSch.EventTimeStr, s.cfg.Location)
			if err == nil {
				var srcMsgID *int64
				if extSch.SourceMessageID != 0 {
					srcMsgID = &extSch.SourceMessageID
				}
				_, _ = s.scheduleRepo.CreateSchedule(ctx, &model.Schedule{
					GroupID:         chatID,
					SourceMessageID: srcMsgID,
					EventName:       extSch.EventName,
					EventTime:       eventTime,
					Location:        extSch.Location,
					Status:          "aktif",
				})
			}
		}
	}

	return nil
}

// GenerateAndSendDailySummary membuat dan mempublikasikan rangkuman harian percakapan
func (s *ConversationService) GenerateAndSendDailySummary(ctx context.Context, groupID int64) error {
	now := time.Now().In(s.cfg.Location)
	todayStr := now.Format("2006-01-02")

	// Ambil pesan hari ini
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.cfg.Location)
	endOfDay := startOfDay.Add(24 * time.Hour)

	msgs, err := s.msgRepo.GetMessagesBetween(ctx, groupID, startOfDay, endOfDay)
	if err != nil {
		return fmt.Errorf("gagal mengambil pesan hari ini: %w", err)
	}

	if len(msgs) == 0 {
		s.logger.Info("Tidak ada pesan hari ini untuk dirangkum", "group_id", groupID, "date", todayStr)
		return nil
	}

	tasks, _ := s.taskRepo.ListTodayTasks(ctx, groupID, s.cfg.Location)
	decisions, _ := s.decisionRepo.ListDecisions(ctx, groupID)

	summaryText, err := s.analyzer.GenerateDailySummary(ctx, msgs, tasks, decisions, todayStr)
	if err != nil {
		return fmt.Errorf("gagal menghasilkan rangkuman harian dari AI: %w", err)
	}

	// Simpan ke database
	_ = s.summaryRepo.SaveDailySummary(ctx, &model.DailySummary{
		GroupID:     groupID,
		SummaryDate: todayStr,
		Content:     summaryText,
	})

	// Kirim rangkuman ke grup
	return s.teleClient.SendMessageWithMode(ctx, groupID, summaryText, "Markdown")
}

