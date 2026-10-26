package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config menyimpan seluruh konfigurasi aplikasi
type Config struct {
	AppPort                   string
	AppEnv                    string
	TelegramBotToken          string
	AllowedGroups             []int64
	AdminUserIDs              []int64
	MonevTargetGroupID        int64
	GeminiAPIKey              string
	GeminiModel               string
	DBHost                    string
	DBPort                    string
	DBUser                    string
	DBPassword                string
	DBName                    string
	DBSSLMode                 string
	DatabaseURL               string
	TimezoneName              string
	Location                  *time.Location
	MonevReminderHour1        int
	MonevReminderMinute1      int
	MonevReminderHour2        int
	MonevReminderMinute2      int
	MonevRecoveryGraceMinutes int
	DailySummaryHour          int
	DailySummaryMinute        int
}

// Load membaca konfigurasi dari file .env (jika ada) dan environment variables
func Load() (*Config, error) {
	// Muat file .env jika tersedia, abaikan jika tidak ada (misalnya saat di Docker)
	_ = godotenv.Load()

	cfg := &Config{
		AppPort:                   getEnv("APP_PORT", "8080"),
		AppEnv:                    getEnv("APP_ENV", "development"),
		TelegramBotToken:          getEnv("TELEGRAM_BOT_TOKEN", ""),
		AllowedGroups:             parseInt64Slice(getEnv("TELEGRAM_ALLOWED_GROUPS", "")),
		AdminUserIDs:              parseInt64Slice(getEnv("TELEGRAM_ADMIN_USER_IDS", "")),
		MonevTargetGroupID:        parseInt64(getEnv("TELEGRAM_MONEV_TARGET_GROUP_ID", "0")),
		GeminiAPIKey:              getEnv("GEMINI_API_KEY", ""),
		GeminiModel:               getEnv("GEMINI_MODEL", "gemini-2.5-flash"),
		DBHost:                    getEnv("DATABASE_HOST", "localhost"),
		DBPort:                    getEnv("DATABASE_PORT", "5432"),
		DBUser:                    getEnv("DATABASE_USER", "bottele"),
		DBPassword:                getEnv("DATABASE_PASSWORD", "secretpassword"),
		DBName:                    getEnv("DATABASE_NAME", "botteleabsensi"),
		DBSSLMode:                 getEnv("DATABASE_SSLMODE", "disable"),
		DatabaseURL:               getEnv("DATABASE_URL", ""),
		TimezoneName:              getEnv("TIMEZONE", "Asia/Jakarta"),
		MonevRecoveryGraceMinutes: parseInt(getEnv("MONEV_RECOVERY_GRACE_MINUTES", "30")),
	}

	// Validasi & load zona waktu (Wajib Asia/Jakarta)
	loc, err := time.LoadLocation(cfg.TimezoneName)
	if err != nil {
		return nil, fmt.Errorf("zona waktu '%s' tidak valid: %w", cfg.TimezoneName, err)
	}
	cfg.Location = loc

	// Parse jadwal pengingat Monev slot 1 (default 15:00)
	h1, m1, err := parseTimeHM(getEnv("MONEV_REMINDER_TIME_1", "15:00"))
	if err != nil {
		return nil, fmt.Errorf("format MONEV_REMINDER_TIME_1 tidak valid: %w", err)
	}
	cfg.MonevReminderHour1 = h1
	cfg.MonevReminderMinute1 = m1

	// Parse jadwal pengingat Monev slot 2 (default 20:00)
	h2, m2, err := parseTimeHM(getEnv("MONEV_REMINDER_TIME_2", "20:00"))
	if err != nil {
		return nil, fmt.Errorf("format MONEV_REMINDER_TIME_2 tidak valid: %w", err)
	}
	cfg.MonevReminderHour2 = h2
	cfg.MonevReminderMinute2 = m2

	// Parse jadwal rangkuman harian (default 21:00)
	sh, sm, err := parseTimeHM(getEnv("DAILY_SUMMARY_TIME", "21:00"))
	if err != nil {
		return nil, fmt.Errorf("format DAILY_SUMMARY_TIME tidak valid: %w", err)
	}
	cfg.DailySummaryHour = sh
	cfg.DailySummaryMinute = sm

	return cfg, nil
}

// IsGroupAllowed memeriksa apakah sebuah group ID terdaftar dalam konfigurasi grup yang diizinkan
func (c *Config) IsGroupAllowed(groupID int64) bool {
	// Jika MonevTargetGroupID diatur dan cocok, izinkan
	if c.MonevTargetGroupID != 0 && c.MonevTargetGroupID == groupID {
		return true
	}
	for _, id := range c.AllowedGroups {
		if id == groupID {
			return true
		}
	}
	return false
}

// IsAdmin memeriksa apakah user ID terdaftar sebagai admin
func (c *Config) IsAdmin(userID int64) bool {
	if len(c.AdminUserIDs) == 0 {
		return false
	}
	for _, id := range c.AdminUserIDs {
		if id == userID {
			return true
		}
	}
	return false
}

// GetDSN mengembalikan Data Source Name untuk koneksi PostgreSQL
func (c *Config) GetDSN() string {
	if c.DatabaseURL != "" {
		return c.DatabaseURL
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, c.DBSSLMode,
	)
}

func getEnv(key, defaultVal string) string {
	val, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(val) == "" {
		return defaultVal
	}
	return strings.TrimSpace(val)
}

func parseInt64(s string) int64 {
	val, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return val
}

func parseInt(s string) int {
	val, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return val
}

func parseInt64Slice(s string) []int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var result []int64
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if val, err := strconv.ParseInt(p, 10, 64); err == nil {
			result = append(result, val)
		}
	}
	return result
}

func parseTimeHM(s string) (int, int, error) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("format harus HH:MM (contoh: 15:00)")
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil || h < 0 || h > 23 {
		return 0, 0, fmt.Errorf("jam harus antara 00-23")
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("menit harus antara 00-59")
	}
	return h, m, nil
}

