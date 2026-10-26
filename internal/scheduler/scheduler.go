package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"botteleabsensi/internal/config"
	"botteleabsensi/internal/service"
)

// Scheduler menjalankan pekerjaan terjadwal (Monev dan rangkuman harian)
type Scheduler struct {
	cfg        *config.Config
	monevSvc   *service.MonevService
	convSvc    *service.ConversationService
	logger     *slog.Logger
	stopCh     chan struct{}
	wg         sync.WaitGroup
	tickerSecs time.Duration
}

// NewScheduler membuat instance Scheduler baru
func NewScheduler(
	cfg *config.Config,
	monevSvc *service.MonevService,
	convSvc *service.ConversationService,
	logger *slog.Logger,
) *Scheduler {
	return &Scheduler{
		cfg:        cfg,
		monevSvc:   monevSvc,
		convSvc:    convSvc,
		logger:     logger,
		stopCh:     make(chan struct{}),
		tickerSecs: 30 * time.Second,
	}
}

// Start menjalankan loop penjadwal secara asinkron
func (s *Scheduler) Start(ctx context.Context) {
	s.wg.Add(1)
	go s.run(ctx)
}

// Stop menghentikan loop penjadwal secara aman (graceful)
func (s *Scheduler) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("Scheduler berhasil dihentikan")
}

func (s *Scheduler) run(ctx context.Context) {
	defer s.wg.Done()

	s.logger.Info("Scheduler otomatis dimulai dengan zona waktu", "timezone", s.cfg.TimezoneName)

	// 1. Eksekusi pemulihan jadwal jika server baru menyala setelah restart
	if err := s.monevSvc.RecoverMissedReminders(ctx, time.Now()); err != nil {
		s.logger.Error("Gagal menjalankan pemulihan jadwal Monev setelah restart", "error", err)
	}

	ticker := time.NewTicker(s.tickerSecs)
	defer ticker.Stop()

	var lastCheckedMinute int = -1

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case t := <-ticker.C:
			nowInLoc := t.In(s.cfg.Location)
			currentMinute := nowInLoc.Minute()

			// Pastikan pengecekan hanya dieksekusi sekali per menit kalender
			if currentMinute == lastCheckedMinute {
				continue
			}
			lastCheckedMinute = currentMinute

			// 2. Evaluasi pengingat Monev (15:00 dan 20:00 WIB, Senin-Sabtu)
			if err := s.monevSvc.CheckAndRunScheduledReminders(ctx, nowInLoc); err != nil {
				s.logger.Error("Error saat mengevaluasi pengingat Monev terjadwal", "error", err)
			}

			// 3. Evaluasi rangkuman harian grup otomatis
			if nowInLoc.Hour() == s.cfg.DailySummaryHour && nowInLoc.Minute() == s.cfg.DailySummaryMinute {
				targetID := s.monevSvc.GetTargetGroupID()
				if targetID != 0 && s.convSvc != nil {
					s.logger.Info("Memicu pembuatan rangkuman harian otomatis...", "group_id", targetID)
					go func() {
						summaryCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
						defer cancel()
						if err := s.convSvc.GenerateAndSendDailySummary(summaryCtx, targetID); err != nil {
							s.logger.Error("Gagal mengirim rangkuman harian otomatis", "error", err)
						}
					}()
				}
			}
		}
	}
}
