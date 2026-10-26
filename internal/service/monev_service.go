package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"botteleabsensi/internal/config"
	"botteleabsensi/internal/repository"
	"botteleabsensi/internal/telegram"
)

const (
	Slot1500 = "15:00"
	Slot2000 = "20:00"
)

// MonevStatusInfo berisi ringkasan status layanan Monev
type MonevStatusInfo struct {
	TargetGroupID    int64     `json:"target_group_id"`
	TargetGroupTitle string    `json:"target_group_title"`
	Timezone         string    `json:"timezone"`
	NextSlot         string    `json:"next_slot"`
	NextScheduledAt  time.Time `json:"next_scheduled_at"`
	Slot1SentToday   bool      `json:"slot_1_sent_today"`
	Slot2SentToday   bool      `json:"slot_2_sent_today"`
}

// MonevService mengelola pengiriman dan siklus pengingat Monev ke grup Telegram
type MonevService struct {
	cfg        *config.Config
	monevRepo  repository.MonevRepository
	groupRepo  repository.GroupRepository
	teleClient telegram.Client
	logger     *slog.Logger
	mu         sync.RWMutex
	targetID   int64
}

// NewMonevService membuat instance baru MonevService
func NewMonevService(
	cfg *config.Config,
	monevRepo repository.MonevRepository,
	groupRepo repository.GroupRepository,
	teleClient telegram.Client,
	logger *slog.Logger,
) *MonevService {
	return &MonevService{
		cfg:        cfg,
		monevRepo:  monevRepo,
		groupRepo:  groupRepo,
		teleClient: teleClient,
		logger:     logger,
		targetID:   cfg.MonevTargetGroupID,
	}
}

// SetTargetGroup memperbarui grup target pengingat Monev
func (s *MonevService) SetTargetGroup(ctx context.Context, groupID int64, title string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.targetID = groupID
	s.cfg.MonevTargetGroupID = groupID

	// Simpan ke database grup
	if err := s.groupRepo.SetGroupAllowed(ctx, groupID, title, true); err != nil {
		return fmt.Errorf("gagal memperbarui izin grup di database: %w", err)
	}

	s.logger.Info("Target grup pengingat Monev berhasil diperbarui", "group_id", groupID, "title", title)
	return nil
}

// GetTargetGroupID mengembalikan ID grup target saat ini
func (s *MonevService) GetTargetGroupID() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.targetID
}

// FormatReminderMessage membuat teks pengingat resmi berdasarkan slot waktu
func (s *MonevService) FormatReminderMessage(slot string) string {
	if slot == Slot1500 {
		return "🔔 **Pengingat Absensi Monev — Pukul 15.00 WIB**\n\n" +
			"Jangan lupa mengisi absensi Monev hari ini.\n" +
			"Lengkapi uraian aktivitas, pembelajaran yang diperoleh, dan kendala yang dialami.\n" +
			"Pastikan laporan sesuai dengan kegiatan yang benar-benar dilakukan."
	}
	return "🔔 **Pengingat Kedua Absensi Monev — Pukul 20.00 WIB**\n\n" +
		"Sudah mengisi absensi Monev hari ini?\n" +
		"Periksa kembali uraian aktivitas, pembelajaran yang diperoleh, dan kendala yang dialami.\n" +
		"Pastikan laporan sudah tersimpan sebelum hari berakhir."
}

// SendReminder mengeksekusi pengiriman pengingat Monev untuk waktu sekarang
func (s *MonevService) SendReminder(ctx context.Context, slot string) error {
	return s.SendReminderAt(ctx, time.Now(), slot)
}

// SendReminderAt mengeksekusi pengiriman pengingat Monev pada waktu spesifik
func (s *MonevService) SendReminderAt(ctx context.Context, now time.Time, slot string) error {
	targetID := s.GetTargetGroupID()
	if targetID == 0 {
		err := fmt.Errorf("target group ID untuk Monev belum dikonfigurasi. Atur TELEGRAM_MONEV_TARGET_GROUP_ID di .env atau jalankan /setgroup di grup target")
		s.logger.Error("Pengiriman pengingat Monev dibatalkan", "error", err)
		return err
	}

	nowInLoc := now.In(s.cfg.Location)
	todayStr := nowInLoc.Format("2006-01-02")

	// Pengecekan hari Minggu
	if nowInLoc.Weekday() == time.Sunday {
		s.logger.Info("Hari ini adalah hari Minggu, pengingat Monev tidak dikirim", "date", todayStr)
		return nil
	}

	// 1. Coba peroleh kunci/lock idempoten di database
	locked, err := s.monevRepo.TryLockReminderLog(ctx, targetID, todayStr, slot)
	if err != nil {
		s.logger.Error("Gagal mengambil lock pengingat Monev di database", "error", err, "group_id", targetID, "slot", slot)
		return fmt.Errorf("gagal memperoleh lock pengingat Monev: %w", err)
	}

	if !locked {
		s.logger.Info("Pengingat Monev sudah pernah terkirim atau dilewati untuk slot ini (idempoten)",
			"group_id", targetID, "date", todayStr, "slot", slot)
		return nil
	}

	// 2. Kirim pesan ke grup Telegram dengan format Markdown
	msgText := s.FormatReminderMessage(slot)
	s.logger.Info("Mengirim pesan pengingat Monev ke grup...", "group_id", targetID, "slot", slot, "date", todayStr)

	sendErr := s.teleClient.SendMessageWithMode(ctx, targetID, msgText, "Markdown")
	if sendErr != nil {
		s.logger.Error("Gagal mengirim pengingat Monev melalui Telegram API",
			"group_id", targetID, "slot", slot, "error", sendErr)
		// Catat kegagalan ke database
		_ = s.monevRepo.MarkFailed(ctx, targetID, todayStr, slot, sendErr.Error())
		return fmt.Errorf("gagal mengirim pesan pengingat ke grup: %w", sendErr)
	}

	// 3. Catat keberhasilan ke database HANYA setelah Telegram API berhasil
	if err := s.monevRepo.MarkSuccess(ctx, targetID, todayStr, slot, time.Now()); err != nil {
		s.logger.Error("Gagal menandai status sukses di database", "error", err)
		return fmt.Errorf("pesan terkirim tetapi gagal update status database: %w", err)
	}

	s.logger.Info("Pengingat Monev BERHASIL dikirim ke grup Telegram", "group_id", targetID, "slot", slot, "date", todayStr)
	return nil
}

// CheckAndRunScheduledReminders memeriksa waktu saat ini dan memicu pengiriman jika tepat pada jadwal
func (s *MonevService) CheckAndRunScheduledReminders(ctx context.Context, now time.Time) error {
	nowInLoc := now.In(s.cfg.Location)

	// Hari Minggu libur
	if nowInLoc.Weekday() == time.Sunday {
		return nil
	}

	hour := nowInLoc.Hour()
	minute := nowInLoc.Minute()

	if hour == s.cfg.MonevReminderHour1 && minute == s.cfg.MonevReminderMinute1 {
		return s.SendReminderAt(ctx, nowInLoc, Slot1500)
	}

	if hour == s.cfg.MonevReminderHour2 && minute == s.cfg.MonevReminderMinute2 {
		return s.SendReminderAt(ctx, nowInLoc, Slot2000)
	}

	return nil
}

// RecoverMissedReminders memulihkan jadwal jika server baru menyala setelah restart
func (s *MonevService) RecoverMissedReminders(ctx context.Context, now time.Time) error {
	targetID := s.GetTargetGroupID()
	if targetID == 0 {
		return nil
	}

	nowInLoc := now.In(s.cfg.Location)
	if nowInLoc.Weekday() == time.Sunday {
		return nil
	}

	todayStr := nowInLoc.Format("2006-01-02")
	grace := time.Duration(s.cfg.MonevRecoveryGraceMinutes) * time.Minute

	// Waktu slot 1 hari ini
	slot1Time := time.Date(nowInLoc.Year(), nowInLoc.Month(), nowInLoc.Day(),
		s.cfg.MonevReminderHour1, s.cfg.MonevReminderMinute1, 0, 0, s.cfg.Location)

	// Waktu slot 2 hari ini
	slot2Time := time.Date(nowInLoc.Year(), nowInLoc.Month(), nowInLoc.Day(),
		s.cfg.MonevReminderHour2, s.cfg.MonevReminderMinute2, 0, 0, s.cfg.Location)

	// Evaluasi Slot 1 (15:00)
	if nowInLoc.After(slot1Time) {
		alreadySent, _ := s.monevRepo.IsAlreadySent(ctx, targetID, todayStr, Slot1500)
		if !alreadySent {
			if nowInLoc.Before(slot1Time.Add(grace)) {
				// Masih dalam batas toleransi (grace period), kirim pengingat yang tertunda
				s.logger.Info("Pemulihan restart: Mengirim pengingat slot 15:00 yang tertunda (dalam batas toleransi)", "group_id", targetID)
				_ = s.SendReminderAt(ctx, nowInLoc, Slot1500)
			} else if nowInLoc.Before(slot2Time) {
				// Sudah melewati toleransi slot 1, tandai missed agar tidak mengirim pesan basi
				s.logger.Info("Pemulihan restart: Melewati slot 15:00 karena melebihi toleransi restart, menunggu slot 20:00", "group_id", targetID)
				_ = s.monevRepo.MarkMissed(ctx, targetID, todayStr, Slot1500, "Melebihi toleransi waktu pemulihan restart")
			}
		}
	}

	// Evaluasi Slot 2 (20:00)
	if nowInLoc.After(slot2Time) {
		alreadySent, _ := s.monevRepo.IsAlreadySent(ctx, targetID, todayStr, Slot2000)
		if !alreadySent {
			if nowInLoc.Before(slot2Time.Add(grace)) {
				// Masih dalam batas toleransi (grace period)
				s.logger.Info("Pemulihan restart: Mengirim pengingat slot 20:00 yang tertunda (dalam batas toleransi)", "group_id", targetID)
				_ = s.SendReminderAt(ctx, nowInLoc, Slot2000)
			} else {
				// Sudah melewati toleransi slot 2
				s.logger.Info("Pemulihan restart: Melewati slot 20:00 karena hari sudah berakhir", "group_id", targetID)
				_ = s.monevRepo.MarkMissed(ctx, targetID, todayStr, Slot2000, "Melebihi toleransi waktu pemulihan restart")
			}
		}
	}

	return nil
}

// CalculateNextSlot menghitung waktu dan slot jadwal Monev berikutnya
func (s *MonevService) CalculateNextSlot(now time.Time) (string, time.Time) {
	nowInLoc := now.In(s.cfg.Location)

	// Buat kandidat waktu hari ini
	todaySlot1 := time.Date(nowInLoc.Year(), nowInLoc.Month(), nowInLoc.Day(),
		s.cfg.MonevReminderHour1, s.cfg.MonevReminderMinute1, 0, 0, s.cfg.Location)
	todaySlot2 := time.Date(nowInLoc.Year(), nowInLoc.Month(), nowInLoc.Day(),
		s.cfg.MonevReminderHour2, s.cfg.MonevReminderMinute2, 0, 0, s.cfg.Location)

	if nowInLoc.Weekday() != time.Sunday {
		if nowInLoc.Before(todaySlot1) {
			return Slot1500, todaySlot1
		}
		if nowInLoc.Before(todaySlot2) {
			return Slot2000, todaySlot2
		}
	}

	// Cari hari berikutnya yang bukan hari Minggu
	candidate := nowInLoc.Add(24 * time.Hour)
	for candidate.Weekday() == time.Sunday {
		candidate = candidate.Add(24 * time.Hour)
	}

	nextTime := time.Date(candidate.Year(), candidate.Month(), candidate.Day(),
		s.cfg.MonevReminderHour1, s.cfg.MonevReminderMinute1, 0, 0, s.cfg.Location)

	return Slot1500, nextTime
}

// GetStatus mengambil status terkini dari penjadwal Monev
func (s *MonevService) GetStatus(ctx context.Context) (*MonevStatusInfo, error) {
	targetID := s.GetTargetGroupID()
	now := time.Now().In(s.cfg.Location)
	todayStr := now.Format("2006-01-02")

	var title string
	if targetID != 0 {
		g, _ := s.groupRepo.GetGroup(ctx, targetID)
		if g != nil {
			title = g.Title
		}
	}

	nextSlot, nextTime := s.CalculateNextSlot(now)

	s1, _ := s.monevRepo.IsAlreadySent(ctx, targetID, todayStr, Slot1500)
	s2, _ := s.monevRepo.IsAlreadySent(ctx, targetID, todayStr, Slot2000)

	return &MonevStatusInfo{
		TargetGroupID:    targetID,
		TargetGroupTitle: title,
		Timezone:         s.cfg.TimezoneName,
		NextSlot:         nextSlot,
		NextScheduledAt:  nextTime,
		Slot1SentToday:   s1,
		Slot2SentToday:   s2,
	}, nil
}
