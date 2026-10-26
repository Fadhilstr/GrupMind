package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"botteleabsensi/internal/config"
	"botteleabsensi/internal/database"
	"botteleabsensi/internal/gemini"
	"botteleabsensi/internal/handler"
	"botteleabsensi/internal/repository"
	"botteleabsensi/internal/scheduler"
	"botteleabsensi/internal/service"
	"botteleabsensi/internal/telegram"
)

func main() {
	// 1. Inisialisasi Logger Terstruktur
	logHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger := slog.New(logHandler)
	slog.SetDefault(logger)

	logger.Info("=========================================================")
	logger.Info("Memulai AI Agent Bot Telegram & Pengingat Absensi Monev")
	logger.Info("=========================================================")

	// 2. Memuat Konfigurasi
	cfg, err := config.Load()
	if err != nil {
		logger.Error("Gagal memuat konfigurasi aplikasi", "error", err)
		os.Exit(1)
	}
	logger.Info("Konfigurasi berhasil dimuat",
		"timezone", cfg.TimezoneName,
		"target_group_id", cfg.MonevTargetGroupID,
		"model_gemini", cfg.GeminiModel,
	)

	// Context utama aplikasi dengan pembatalan graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 3. Menghubungkan ke Database PostgreSQL
	dbPool, err := database.ConnectPostgres(ctx, cfg.GetDSN(), logger)
	if err != nil {
		logger.Error("Gagal terhubung ke database PostgreSQL", "error", err)
		logger.Warn("Aplikasi membutuhkan PostgreSQL untuk beroperasi. Periksa koneksi atau jalankan Docker Compose.")
		os.Exit(1)
	}
	defer dbPool.Close()

	// 4. Menjalankan Migrasi Skema Database
	if err := database.RunMigrations(ctx, dbPool, logger); err != nil {
		logger.Error("Gagal mengeksekusi migrasi skema database", "error", err)
		os.Exit(1)
	}

	// 5. Inisialisasi Repositori
	repo := repository.NewPostgresRepository(dbPool)

	// Daftarkan grup dari konfigurasi TELEGRAM_ALLOWED_GROUPS ke database
	for _, gid := range cfg.AllowedGroups {
		_ = repo.SetGroupAllowed(ctx, gid, "", true)
	}
	if cfg.MonevTargetGroupID != 0 {
		_ = repo.SetGroupAllowed(ctx, cfg.MonevTargetGroupID, "", true)
	}

	// 6. Inisialisasi Telegram Client
	if cfg.TelegramBotToken == "" {
		logger.Warn("TELEGRAM_BOT_TOKEN belum diatur! Bot tidak dapat terhubung ke Telegram API.")
		logger.Warn("Silakan buat bot melalui @BotFather dan masukkan token ke dalam file .env")
	}
	teleClient := telegram.NewHTTPClient(cfg.TelegramBotToken, logger)

	// Verifikasi token jika tersedia
	if cfg.TelegramBotToken != "" {
		meCtx, meCancel := context.WithTimeout(ctx, 10*time.Second)
		botInfo, err := teleClient.GetMe(meCtx)
		meCancel()
		if err != nil {
			logger.Warn("Tidak dapat memverifikasi token bot Telegram (apakah jaringan / token valid?)", "error", err)
		} else {
			logger.Info("Bot Telegram terverifikasi", "bot_id", botInfo.ID, "first_name", botInfo.FirstName, "username", botInfo.Username)
		}
	}

	// 7. Inisialisasi Gemini Analyzer
	var analyzer gemini.Analyzer
	if cfg.GeminiAPIKey != "" {
		gAnalyzer, err := gemini.NewGeminiAnalyzer(ctx, cfg.GeminiAPIKey, cfg.GeminiModel, cfg.Location, logger)
		if err != nil {
			logger.Error("Gagal inisialisasi Gemini Analyzer, beralih ke Mock Analyzer", "error", err)
			analyzer = gemini.NewMockAnalyzer()
		} else {
			analyzer = gAnalyzer
			logger.Info("Gemini AI Analyzer aktif", "model", cfg.GeminiModel)
		}
	} else {
		logger.Warn("GEMINI_API_KEY belum diisi! Menggunakan Mock Analyzer untuk pemrosesan lokal.")
		analyzer = gemini.NewMockAnalyzer()
	}

	// 8. Inisialisasi Business Services
	monevSvc := service.NewMonevService(cfg, repo, repo, teleClient, logger)
	convSvc := service.NewConversationService(cfg, repo, repo, repo, repo, repo, analyzer, teleClient, logger)

	// 9. Inisialisasi dan Jalankan Scheduler Terjadwal
	sched := scheduler.NewScheduler(cfg, monevSvc, convSvc, logger)
	sched.Start(ctx)

	// 10. Inisialisasi Handler Pesan & Perintah Telegram
	botHandler := handler.NewHandler(cfg, teleClient, monevSvc, convSvc, repo, repo, repo, repo, repo, repo, logger)

	// 11. Jalankan Telegram Long-Polling Worker
	if cfg.TelegramBotToken != "" {
		go startPollingWorker(ctx, teleClient, botHandler, logger)
	}

	// 12. HTTP Server untuk Health Check & Observability
	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		statusInfo, _ := monevSvc.GetStatus(r.Context())
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":   "ok",
			"time":     time.Now().In(cfg.Location).Format(time.RFC3339),
			"timezone": cfg.TimezoneName,
			"monev":    statusInfo,
		})
	})

	server := &http.Server{
		Addr:         ":" + cfg.AppPort,
		Handler:      httpMux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("HTTP Health Check server berjalan", "port", cfg.AppPort)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server error", "error", err)
		}
	}()

	// 13. Graceful Shutdown
	quitCh := make(chan os.Signal, 1)
	signal.Notify(quitCh, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	<-quitCh
	logger.Info("Menerima sinyal terminasi, memulai shutdown aman...")

	// Hentikan scheduler
	sched.Stop()

	// Matikan HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)

	// Batalkan context aplikasi
	cancel()

	logger.Info("AI Agent Telegram & Monev berhasil dimatikan secara aman.")
}

// startPollingWorker menjalankan long-polling Telegram secara terus menerus
func startPollingWorker(ctx context.Context, client telegram.Client, botHandler *handler.Handler, logger *slog.Logger) {
	logger.Info("Telegram long-polling worker aktif menerima pesan...")

	var offset int64 = 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
			updates, err := client.GetUpdates(ctx, offset, 100, 20)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				logger.Warn("Gagal mengambil update Telegram, jeda 3 detik...", "error", err)
				time.Sleep(3 * time.Second)
				continue
			}

			for _, update := range updates {
				if update.UpdateID >= offset {
					offset = update.UpdateID + 1
				}

				// Proses update secara aman
				func(u telegram.Update) {
					defer func() {
						if r := recover(); r != nil {
							logger.Error("Panic tertangkap saat memproses update", "panic", r, "update_id", u.UpdateID)
						}
					}()
					procCtx, procCancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer procCancel()
					if err := botHandler.ProcessUpdate(procCtx, &u); err != nil {
						logger.Error("Gagal memproses update", "update_id", u.UpdateID, "error", err)
					}
				}(update)
			}
		}
	}
}
