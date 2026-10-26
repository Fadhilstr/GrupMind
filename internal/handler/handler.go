package handler

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"botteleabsensi/internal/config"
	"botteleabsensi/internal/model"
	"botteleabsensi/internal/repository"
	"botteleabsensi/internal/service"
	"botteleabsensi/internal/telegram"
)

// Handler memproses setiap update Telegram masuk dan mengeksekusi routing perintah
type Handler struct {
	cfg          *config.Config
	client       telegram.Client
	monevSvc     *service.MonevService
	convSvc      *service.ConversationService
	taskRepo     repository.TaskRepository
	decisionRepo repository.DecisionRepository
	scheduleRepo repository.ScheduleRepository
	groupRepo    repository.GroupRepository
	userRepo     repository.UserRepository
	summaryRepo  repository.SummaryRepository
	logger       *slog.Logger
	startTime    time.Time
}

// NewHandler membuat instance baru Telegram Handler
func NewHandler(
	cfg *config.Config,
	client telegram.Client,
	monevSvc *service.MonevService,
	convSvc *service.ConversationService,
	taskRepo repository.TaskRepository,
	decisionRepo repository.DecisionRepository,
	scheduleRepo repository.ScheduleRepository,
	groupRepo repository.GroupRepository,
	userRepo repository.UserRepository,
	summaryRepo repository.SummaryRepository,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		cfg:          cfg,
		client:       client,
		monevSvc:     monevSvc,
		convSvc:      convSvc,
		taskRepo:     taskRepo,
		decisionRepo: decisionRepo,
		scheduleRepo: scheduleRepo,
		groupRepo:    groupRepo,
		userRepo:     userRepo,
		summaryRepo:  summaryRepo,
		logger:       logger,
		startTime:    time.Now(),
	}
}

// ProcessUpdate memproses satu update dari Telegram
func (h *Handler) ProcessUpdate(ctx context.Context, u *telegram.Update) error {
	msg := u.Message
	if msg == nil {
		msg = u.EditedMessage
	}
	if msg == nil {
		return nil
	}

	// Simpan data pengguna jika tersedia
	if msg.From != nil {
		_ = h.userRepo.UpsertUser(ctx, &model.User{
			ID:        msg.From.ID,
			Username:  msg.From.Username,
			FirstName: msg.From.FirstName,
			LastName:  msg.From.LastName,
			Role:      "member",
		})
	}

	text := strings.TrimSpace(msg.Text)
	chatID := msg.Chat.ID
	isPrivate := msg.Chat.Type == "private"

	// 1. Tangani Perintah (Commands diawali '/')
	if strings.HasPrefix(text, "/") {
		return h.handleCommand(ctx, msg, isPrivate)
	}

	// 2. Tangani Pesan Grup Biasa (Hanya jika grup diizinkan)
	if !isPrivate {
		if !h.cfg.IsGroupAllowed(chatID) && chatID != h.monevSvc.GetTargetGroupID() {
			// Grup tidak diizinkan, abaikan
			return nil
		}

		sourceMsg := &model.SourceMessage{
			MessageID:  msg.MessageID,
			ChatID:     chatID,
			SenderID:   msg.From.ID,
			SenderName: h.getSenderDisplayName(msg.From),
			Text:       text,
			SentAt:     time.Unix(msg.Date, 0),
		}

		return h.convSvc.HandleGroupMessage(ctx, sourceMsg)
	}

	// 3. Pesan non-command di chat pribadi
	return h.client.SendMessage(ctx, chatID, "Ketik /help untuk melihat daftar perintah yang tersedia.")
}

func (h *Handler) handleCommand(ctx context.Context, msg *telegram.Message, isPrivate bool) error {
	chatID := msg.Chat.ID
	text := strings.TrimSpace(msg.Text)
	parts := strings.Fields(text)
	cmd := strings.ToLower(parts[0])

	// Hapus postfix bot username misal "/start@botusername"
	if atIdx := strings.Index(cmd, "@"); atIdx != -1 {
		cmd = cmd[:atIdx]
	}

	// Logging perintah yang diterima
	h.logger.Info("Menerima perintah Telegram", "cmd", cmd, "chat_id", chatID, "from_user", msg.From.ID)

	// Perintah publik yang selalu diizinkan di grup maupun chat pribadi
	switch cmd {
	case "/start":
		return h.cmdStart(ctx, msg, isPrivate)
	case "/help":
		return h.cmdHelp(ctx, msg)
	case "/groupid":
		return h.cmdGroupID(ctx, msg, isPrivate)
	case "/status":
		return h.cmdStatus(ctx, msg)
	case "/monev":
		return h.cmdMonev(ctx, msg)
	}

	// Pengecekan otorisasi grup untuk perintah fungsional grup
	if !isPrivate && !h.cfg.IsGroupAllowed(chatID) && chatID != h.monevSvc.GetTargetGroupID() && cmd != "/setgroup" {
		h.logger.Warn("Menolak perintah dari grup yang belum diizinkan", "chat_id", chatID, "cmd", cmd)
		return h.client.SendMessage(ctx, chatID, "⛔ Grup ini belum diizinkan. Gunakan /groupid untuk melihat ID grup, lalu atur di konfigurasi bot.")
	}

	// Routing perintah operasional
	switch cmd {
	case "/setgroup":
		return h.cmdSetGroup(ctx, msg, parts, isPrivate)
	case "/testreminder":
		return h.cmdTestReminder(ctx, msg)
	case "/tasks":
		return h.cmdTasks(ctx, msg, isPrivate)
	case "/today":
		return h.cmdToday(ctx, msg, isPrivate)
	case "/upcoming":
		return h.cmdUpcoming(ctx, msg, isPrivate)
	case "/decisions":
		return h.cmdDecisions(ctx, msg, isPrivate)
	case "/done":
		return h.cmdDone(ctx, msg, parts)
	case "/summary":
		return h.cmdSummary(ctx, msg, isPrivate)
	default:
		return h.client.SendMessage(ctx, chatID, "Perintah tidak dikenali. Ketik /help untuk panduan penggunaan.")
	}
}

// ============================================================================
// IMPLEMENTASI MASING-MASING PERINTAH
// ============================================================================

func (h *Handler) cmdStart(ctx context.Context, msg *telegram.Message, isPrivate bool) error {
	reply := `👋 **Selamat datang di AI Agent Absensi Monev & Asisten Grup!**

Saya adalah bot otomatis berbasis Go & Gemini AI yang bertugas:
1. 🔔 **Pengingat Absensi Monev**: Mengirim pengingat otomatis ke grup Telegram setiap **Senin–Sabtu pukul 15.00 dan 20.00 WIB**.
2. 🧠 **Analisis Percakapan AI**: Memahami pembagian tugas, deadline, dan keputusan penting dari percakapan grup.
3. 📋 **Manajemen Tugas & Jadwal**: Memantau tenggat waktu dan agenda kerja tim.

💡 **Panduan Memulai:**
- Masukkan bot ke grup tim Anda.
- Ketik perintah ` + "`/groupid`" + ` di grup untuk melihat ID grup.
- Tetapkan grup sebagai target pengingat dengan ` + "`/setgroup`" + `.
- Ketik ` + "`/help`" + ` untuk melihat seluruh daftar perintah.`

	return h.client.SendMessageWithMode(ctx, msg.Chat.ID, reply, "Markdown")
}

func (h *Handler) cmdHelp(ctx context.Context, msg *telegram.Message) error {
	reply := `📖 **Panduan Penggunaan Perintah Bot:**

📌 **Pengingat & Status Monev:**
• ` + "`/groupid`" + ` — Tampilkan ID chat/grup saat ini
• ` + "`/setgroup`" + ` — Tetapkan grup saat ini sebagai target pengingat Monev (Admin)
• ` + "`/monev`" + ` — Tampilkan jadwal dan status pengingat Monev hari ini
• ` + "`/testreminder`" + ` — Uji pengiriman pengingat Monev ke grup secara langsung (Admin)
• ` + "`/status`" + ` — Tampilkan status koneksi, database, dan jadwal berikutnya

📋 **Tugas, Keputusan & Percakapan:**
• ` + "`/tasks`" + ` — Tampilkan seluruh tugas aktif di grup
• ` + "`/today`" + ` — Tampilkan tugas dan jadwal untuk hari ini
• ` + "`/upcoming`" + ` — Tampilkan tugas dan jadwal mendatang
• ` + "`/decisions`" + ` — Tampilkan keputusan dan kesimpulan diskusi penting
• ` + "`/done <ID>`" + ` — Tandai tugas dengan ID tertentu sebagai selesai
• ` + "`/summary`" + ` — Dapatkan rangkuman harian diskusi percakapan grup

ℹ️ *Catatan Privasi:* Agar bot dapat membaca pesan diskusi tim tanpa perlu di-mention, pastikan Group Privacy dinonaktifkan di @BotFather atau jadikan bot sebagai Administrator grup.`

	return h.client.SendMessageWithMode(ctx, msg.Chat.ID, reply, "Markdown")
}

func (h *Handler) cmdGroupID(ctx context.Context, msg *telegram.Message, isPrivate bool) error {
	chatID := msg.Chat.ID
	targetID := h.monevSvc.GetTargetGroupID()

	isTarget := chatID == targetID
	targetStatus := "Bukan target Monev"
	if isTarget {
		targetStatus = "✅ Terdaftar sebagai target pengingat Monev"
	}

	reply := fmt.Sprintf("🏷️ **Informasi Chat ID:**\n\n• **ID Chat Saat Ini:** `%d`\n• **Tipe:** `%s`\n• **Status Target:** %s",
		chatID, msg.Chat.Type, targetStatus)

	if !isPrivate && !isTarget {
		reply += "\n\n💡 *Untuk menjadikan grup ini sebagai target pengingat Monev, admin dapat mengetik:* `/setgroup`"
	}

	return h.client.SendMessageWithMode(ctx, chatID, reply, "Markdown")
}

func (h *Handler) cmdSetGroup(ctx context.Context, msg *telegram.Message, parts []string, isPrivate bool) error {
	userID := msg.From.ID
	var targetID int64
	var title string

	if len(parts) > 1 {
		parsed, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return h.client.SendMessage(ctx, msg.Chat.ID, "Format ID grup tidak valid. Contoh: /setgroup -1001234567890")
		}
		targetID = parsed
	} else {
		if isPrivate {
			return h.client.SendMessage(ctx, msg.Chat.ID, "Perintah /setgroup di chat pribadi harus menyertakan ID grup. Contoh: `/setgroup -1001234567890`")
		}
		targetID = msg.Chat.ID
		title = msg.Chat.Title
	}

	// Validasi wewenang admin
	isAdmin := h.cfg.IsAdmin(userID)
	if !isAdmin && !isPrivate {
		// Cek apakah admin di grup Telegram
		chatAdmin, err := h.client.IsChatAdmin(ctx, msg.Chat.ID, userID)
		if err == nil && chatAdmin {
			isAdmin = true
		}
	}

	if !isAdmin {
		h.logger.Warn("Akses ditolak untuk perintah /setgroup", "user_id", userID, "chat_id", msg.Chat.ID)
		return h.client.SendMessage(ctx, msg.Chat.ID, "⛔ Anda tidak memiliki wewenang untuk menetapkan target grup Monev. Hanya admin yang diizinkan.")
	}

	if err := h.monevSvc.SetTargetGroup(ctx, targetID, title); err != nil {
		return h.client.SendMessage(ctx, msg.Chat.ID, fmt.Sprintf("❌ Gagal menyimpan target grup: %s", err.Error()))
	}

	reply := fmt.Sprintf("✅ **Target Pengingat Monev Berhasil Ditetapkan!**\n\n• **ID Grup:** `%d`\n• **Jadwal Otomatis:** Senin–Sabtu pukul 15.00 & 20.00 WIB (Hari Minggu libur)\n\nPengingat akan dikirimkan langsung ke grup ini.", targetID)
	return h.client.SendMessageWithMode(ctx, msg.Chat.ID, reply, "Markdown")
}

func (h *Handler) cmdMonev(ctx context.Context, msg *telegram.Message) error {
	statusInfo, err := h.monevSvc.GetStatus(ctx)
	if err != nil {
		return h.client.SendMessage(ctx, msg.Chat.ID, fmt.Sprintf("Gagal mengambil status Monev: %s", err.Error()))
	}

	targetText := "Belum diatur (Gunakan /setgroup di grup target)"
	if statusInfo.TargetGroupID != 0 {
		targetText = fmt.Sprintf("`%d`", statusInfo.TargetGroupID)
		if statusInfo.TargetGroupTitle != "" {
			targetText += fmt.Sprintf(" (%s)", statusInfo.TargetGroupTitle)
		}
	}

	s1Status := "Menunggu jadwal"
	if statusInfo.Slot1SentToday {
		s1Status = "✅ Terkirim hari ini"
	}

	s2Status := "Menunggu jadwal"
	if statusInfo.Slot2SentToday {
		s2Status = "✅ Terkirim hari ini"
	}

	reply := fmt.Sprintf(`📊 **Konfigurasi & Status Pengingat Absensi Monev**

🎯 **Grup Tujuan:** %s
🌏 **Zona Waktu:** %s
⏰ **Jadwal Tetap:**
  • Pengingat 1: %02d:%02d WIB [%s]
  • Pengingat 2: %02d:%02d WIB [%s]
  • Hari Minggu: Libur (Tidak ada pengingat)

⏭️ **Jadwal Berikutnya:** %s pukul %s WIB`,
		targetText,
		statusInfo.Timezone,
		h.cfg.MonevReminderHour1, h.cfg.MonevReminderMinute1, s1Status,
		h.cfg.MonevReminderHour2, h.cfg.MonevReminderMinute2, s2Status,
		statusInfo.NextScheduledAt.Format("Monday, 02 Jan 2006"),
		statusInfo.NextSlot,
	)

	return h.client.SendMessageWithMode(ctx, msg.Chat.ID, reply, "Markdown")
}

func (h *Handler) cmdTestReminder(ctx context.Context, msg *telegram.Message) error {
	userID := msg.From.ID
	if !h.cfg.IsAdmin(userID) {
		chatAdmin, err := h.client.IsChatAdmin(ctx, msg.Chat.ID, userID)
		if err != nil || !chatAdmin {
			return h.client.SendMessage(ctx, msg.Chat.ID, "⛔ Perintah /testreminder hanya dapat dijalankan oleh Admin.")
		}
	}

	targetID := h.monevSvc.GetTargetGroupID()
	if targetID == 0 {
		return h.client.SendMessage(ctx, msg.Chat.ID, "❌ Target grup Monev belum ditentukan. Ketik /setgroup terlebih dahulu.")
	}

	_ = h.client.SendMessage(ctx, msg.Chat.ID, "⏳ Menguji pengiriman pengingat Monev ke grup target...")

	testMsg := "🧪 **[UJI COBA PENGINGAT MONEV]**\n\n" +
		h.monevSvc.FormatReminderMessage(service.Slot1500) +
		"\n\n*(Ini adalah pesan pengujian manual yang dipicu oleh admin)*"

	err := h.client.SendMessageWithMode(ctx, targetID, testMsg, "Markdown")
	if err != nil {
		return h.client.SendMessage(ctx, msg.Chat.ID, fmt.Sprintf("❌ Gagal mengirim ke grup target (`%d`): %s", targetID, err.Error()))
	}

	return h.client.SendMessage(ctx, msg.Chat.ID, fmt.Sprintf("✅ Berhasil mengirim pesan pengujian ke grup target (`%d`).", targetID))
}

func (h *Handler) cmdTasks(ctx context.Context, msg *telegram.Message, isPrivate bool) error {
	groupID := h.resolveGroupID(msg, isPrivate)
	if groupID == 0 {
		return h.client.SendMessage(ctx, msg.Chat.ID, "Tentukan grup terlebih dahulu atau gunakan perintah ini di dalam grup.")
	}

	tasks, err := h.taskRepo.ListTasks(ctx, groupID, nil)
	if err != nil {
		return h.client.SendMessage(ctx, msg.Chat.ID, fmt.Sprintf("Gagal mengambil daftar tugas: %s", err.Error()))
	}

	if len(tasks) == 0 {
		return h.client.SendMessage(ctx, msg.Chat.ID, "Belum ada tugas yang tercatat untuk grup ini.")
	}

	var sb strings.Builder
	sb.WriteString("📋 **Daftar Tugas Tim:**\n\n")

	for _, t := range tasks {
		dueStr := "Tidak ada tenggat"
		if t.DueDate != nil {
			dueStr = t.DueDate.In(h.cfg.Location).Format("02 Jan 15:04 WIB")
		}
		assigneeStr := "Belum ditentukan"
		if t.Assignee != "" {
			assigneeStr = t.Assignee
		}

		icon := "⏳"
		if t.Status == model.TaskStatusSelesai {
			icon = "✅"
		} else if t.Status == model.TaskStatusSedangDikerjakan {
			icon = "🔄"
		} else if t.Status == model.TaskStatusPerluKonfirmasi {
			icon = "❓"
		}

		sb.WriteString(fmt.Sprintf("%s **[ID: %d] %s**\n", icon, t.ID, t.Title))
		if t.Description != "" {
			sb.WriteString(fmt.Sprintf("   • Detail: %s\n", t.Description))
		}
		sb.WriteString(fmt.Sprintf("   • PIC: %s | Deadline: %s | Status: `%s`\n\n", assigneeStr, dueStr, t.Status))
	}

	sb.WriteString("💡 *Untuk menyelesaikan tugas: `/done <ID_TUGAS>`*")
	return h.client.SendMessageWithMode(ctx, msg.Chat.ID, sb.String(), "Markdown")
}

func (h *Handler) cmdToday(ctx context.Context, msg *telegram.Message, isPrivate bool) error {
	groupID := h.resolveGroupID(msg, isPrivate)
	if groupID == 0 {
		return h.client.SendMessage(ctx, msg.Chat.ID, "Grup target belum dikonfigurasi.")
	}

	tasks, _ := h.taskRepo.ListTodayTasks(ctx, groupID, h.cfg.Location)
	now := time.Now().In(h.cfg.Location)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📅 **Agenda & Tugas Hari Ini (%s):**\n\n", now.Format("02 January 2006")))

	if len(tasks) == 0 {
		sb.WriteString("Tidak ada tenggat tugas yang jatuh tempo hari ini.\n")
	} else {
		for _, t := range tasks {
			sb.WriteString(fmt.Sprintf("• **[ID: %d]** %s (PIC: %s, Status: `%s`)\n", t.ID, t.Title, t.Assignee, t.Status))
		}
	}

	return h.client.SendMessageWithMode(ctx, msg.Chat.ID, sb.String(), "Markdown")
}

func (h *Handler) cmdUpcoming(ctx context.Context, msg *telegram.Message, isPrivate bool) error {
	groupID := h.resolveGroupID(msg, isPrivate)
	if groupID == 0 {
		return h.client.SendMessage(ctx, msg.Chat.ID, "Grup target belum dikonfigurasi.")
	}

	tasks, _ := h.taskRepo.ListUpcomingTasks(ctx, groupID, h.cfg.Location)
	schedules, _ := h.scheduleRepo.ListUpcomingSchedules(ctx, groupID, time.Now())

	var sb strings.Builder
	sb.WriteString("🚀 **Tugas & Agenda Mendatang:**\n\n")

	if len(tasks) > 0 {
		sb.WriteString("📌 **Tugas Mendatang:**\n")
		for _, t := range tasks {
			dueStr := t.DueDate.In(h.cfg.Location).Format("02 Jan 15:04 WIB")
			sb.WriteString(fmt.Sprintf("• **[ID: %d]** %s — Deadline: %s (PIC: %s)\n", t.ID, t.Title, dueStr, t.Assignee))
		}
		sb.WriteString("\n")
	}

	if len(schedules) > 0 {
		sb.WriteString("📅 **Agenda Rapat / Jadwal:**\n")
		for _, s := range schedules {
			timeStr := s.EventTime.In(h.cfg.Location).Format("02 Jan 15:04 WIB")
			sb.WriteString(fmt.Sprintf("• %s — Waktu: %s (Lokasi: %s)\n", s.EventName, timeStr, s.Location))
		}
	}

	if len(tasks) == 0 && len(schedules) == 0 {
		sb.WriteString("Belum ada jadwal atau tugas mendatang.")
	}

	return h.client.SendMessageWithMode(ctx, msg.Chat.ID, sb.String(), "Markdown")
}

func (h *Handler) cmdDecisions(ctx context.Context, msg *telegram.Message, isPrivate bool) error {
	groupID := h.resolveGroupID(msg, isPrivate)
	if groupID == 0 {
		return h.client.SendMessage(ctx, msg.Chat.ID, "Grup target belum dikonfigurasi.")
	}

	decisions, err := h.decisionRepo.ListDecisions(ctx, groupID)
	if err != nil {
		return h.client.SendMessage(ctx, msg.Chat.ID, fmt.Sprintf("Gagal mengambil keputusan: %s", err.Error()))
	}

	if len(decisions) == 0 {
		return h.client.SendMessage(ctx, msg.Chat.ID, "Belum ada keputusan atau kesimpulan yang tercatat.")
	}

	var sb strings.Builder
	sb.WriteString("⚖️ **Daftar Keputusan & Kesepakatan Tim:**\n\n")

	for _, d := range decisions {
		dateStr := d.DecidedAt.In(h.cfg.Location).Format("02 Jan 2006")
		statusTag := "✅ Final"
		if d.Status == model.DecisionStatusUsulan {
			statusTag = "💡 Usulan"
		} else if d.Status == model.DecisionStatusPerluKonfirmasi {
			statusTag = "❓ Perlu Konfirmasi"
		}

		sb.WriteString(fmt.Sprintf("• **[%s] %s** (%s)\n  %s\n\n", statusTag, d.Topic, dateStr, d.DecisionText))
	}

	return h.client.SendMessageWithMode(ctx, msg.Chat.ID, sb.String(), "Markdown")
}

func (h *Handler) cmdDone(ctx context.Context, msg *telegram.Message, parts []string) error {
	if len(parts) < 2 {
		return h.client.SendMessage(ctx, msg.Chat.ID, "Gunakan format: `/done <ID_TUGAS>`\nContoh: `/done 1`")
	}

	taskID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return h.client.SendMessage(ctx, msg.Chat.ID, "ID tugas harus berupa angka numerik valid.")
	}

	task, err := h.taskRepo.GetTaskByID(ctx, taskID)
	if err != nil || task == nil {
		return h.client.SendMessage(ctx, msg.Chat.ID, fmt.Sprintf("❌ Tugas dengan ID %d tidak ditemukan.", taskID))
	}

	userName := h.getSenderDisplayName(msg.From)
	err = h.taskRepo.UpdateTaskStatus(ctx, taskID, model.TaskStatusSelesai, userName, "Ditandai selesai via /done")
	if err != nil {
		return h.client.SendMessage(ctx, msg.Chat.ID, fmt.Sprintf("❌ Gagal menandai tugas selesai: %s", err.Error()))
	}

	reply := fmt.Sprintf("🎉 **Tugas Berhasil Diselesaikan!**\n\n• **ID:** %d\n• **Judul:** %s\n• **Diselesaikan oleh:** %s",
		taskID, task.Title, userName)
	return h.client.SendMessageWithMode(ctx, msg.Chat.ID, reply, "Markdown")
}

func (h *Handler) cmdSummary(ctx context.Context, msg *telegram.Message, isPrivate bool) error {
	groupID := h.resolveGroupID(msg, isPrivate)
	if groupID == 0 {
		return h.client.SendMessage(ctx, msg.Chat.ID, "Grup target belum dikonfigurasi.")
	}

	todayStr := time.Now().In(h.cfg.Location).Format("2006-01-02")
	existing, err := h.summaryRepo.GetDailySummary(ctx, groupID, todayStr)
	if err == nil && existing != nil {
		return h.client.SendMessageWithMode(ctx, msg.Chat.ID, existing.Content, "Markdown")
	}

	_ = h.client.SendMessage(ctx, msg.Chat.ID, "⏳ Sedang menyusun rangkuman diskusi terbaru dari Gemini AI...")

	err = h.convSvc.GenerateAndSendDailySummary(ctx, groupID)
	if err != nil {
		return h.client.SendMessage(ctx, msg.Chat.ID, fmt.Sprintf("❌ Gagal membuat rangkuman: %s", err.Error()))
	}

	if isPrivate {
		latest, _ := h.summaryRepo.GetLatestDailySummary(ctx, groupID)
		if latest != nil {
			return h.client.SendMessageWithMode(ctx, msg.Chat.ID, latest.Content, "Markdown")
		}
	}

	return nil
}

func (h *Handler) cmdStatus(ctx context.Context, msg *telegram.Message) error {
	uptime := time.Since(h.startTime).Round(time.Second)
	monevStatus, _ := h.monevSvc.GetStatus(ctx)

	targetDesc := "Belum diatur"
	if monevStatus != nil && monevStatus.TargetGroupID != 0 {
		targetDesc = fmt.Sprintf("`%d`", monevStatus.TargetGroupID)
	}

	geminiStatus := "Terkonfigurasi"
	if h.cfg.GeminiAPIKey == "" {
		geminiStatus = "⚠️ Kunci API belum diisi"
	}

	reply := fmt.Sprintf(`🟢 **Status Layanan Bot Tele Monev & AI Agent**

⏱️ **Uptime:** %s
🌏 **Zona Waktu:** %s
🤖 **AI Engine:** %s (%s)
🎯 **Target Grup Monev:** %s
⏰ **Jadwal Pengingat Berikutnya:** %s (%s WIB)
📊 **Status Pengingat Hari Ini:**
  • Pukul 15:00 WIB: %v
  • Pukul 20:00 WIB: %v`,
		uptime.String(),
		h.cfg.TimezoneName,
		h.cfg.GeminiModel, geminiStatus,
		targetDesc,
		monevStatus.NextScheduledAt.Format("02 Jan 2006"), monevStatus.NextSlot,
		monevStatus.Slot1SentToday,
		monevStatus.Slot2SentToday,
	)

	return h.client.SendMessageWithMode(ctx, msg.Chat.ID, reply, "Markdown")
}

func (h *Handler) resolveGroupID(msg *telegram.Message, isPrivate bool) int64 {
	if !isPrivate {
		return msg.Chat.ID
	}
	return h.monevSvc.GetTargetGroupID()
}

func (h *Handler) getSenderDisplayName(u *telegram.User) string {
	if u == nil {
		return "Anonim"
	}
	if u.Username != "" {
		return "@" + u.Username
	}
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	if name != "" {
		return name
	}
	return fmt.Sprintf("User_%d", u.ID)
}

