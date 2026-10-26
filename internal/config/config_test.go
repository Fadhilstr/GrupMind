package config_test

import (
	"os"
	"testing"

	"botteleabsensi/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_DefaultValues(t *testing.T) {
	// Kosongkan env yang mempengaruhi test
	os.Clearenv()

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "8080", cfg.AppPort)
	assert.Equal(t, "development", cfg.AppEnv)
	assert.Equal(t, "Asia/Jakarta", cfg.TimezoneName)
	assert.NotNil(t, cfg.Location)
	assert.Equal(t, 15, cfg.MonevReminderHour1)
	assert.Equal(t, 0, cfg.MonevReminderMinute1)
	assert.Equal(t, 20, cfg.MonevReminderHour2)
	assert.Equal(t, 0, cfg.MonevReminderMinute2)
	assert.Equal(t, 30, cfg.MonevRecoveryGraceMinutes)
	assert.Equal(t, 21, cfg.DailySummaryHour)
	assert.Equal(t, 0, cfg.DailySummaryMinute)
	assert.Equal(t, "postgres://bottele:secretpassword@localhost:5432/botteleabsensi?sslmode=disable", cfg.GetDSN())
}

func TestConfig_CustomValues(t *testing.T) {
	os.Clearenv()
	t.Setenv("APP_PORT", "9000")
	t.Setenv("TELEGRAM_ALLOWED_GROUPS", "-1001234567890,-1009876543210")
	t.Setenv("TELEGRAM_ADMIN_USER_IDS", "111222,333444")
	t.Setenv("TELEGRAM_MONEV_TARGET_GROUP_ID", "-1001234567890")
	t.Setenv("MONEV_REMINDER_TIME_1", "14:30")
	t.Setenv("MONEV_REMINDER_TIME_2", "19:45")
	t.Setenv("MONEV_RECOVERY_GRACE_MINUTES", "45")
	t.Setenv("DATABASE_URL", "postgres://custom:pass@remote:5432/mydb?sslmode=require")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "9000", cfg.AppPort)
	assert.Equal(t, []int64{-1001234567890, -1009876543210}, cfg.AllowedGroups)
	assert.Equal(t, []int64{111222, 333444}, cfg.AdminUserIDs)
	assert.Equal(t, int64(-1001234567890), cfg.MonevTargetGroupID)
	assert.Equal(t, 14, cfg.MonevReminderHour1)
	assert.Equal(t, 30, cfg.MonevReminderMinute1)
	assert.Equal(t, 19, cfg.MonevReminderHour2)
	assert.Equal(t, 45, cfg.MonevReminderMinute2)
	assert.Equal(t, 45, cfg.MonevRecoveryGraceMinutes)
	assert.Equal(t, "postgres://custom:pass@remote:5432/mydb?sslmode=require", cfg.GetDSN())

	// Test IsGroupAllowed
	assert.True(t, cfg.IsGroupAllowed(-1001234567890))
	assert.True(t, cfg.IsGroupAllowed(-1009876543210))
	assert.False(t, cfg.IsGroupAllowed(-999999999999))

	// Test IsAdmin
	assert.True(t, cfg.IsAdmin(111222))
	assert.True(t, cfg.IsAdmin(333444))
	assert.False(t, cfg.IsAdmin(999999))
}

func TestConfig_InvalidTimezone(t *testing.T) {
	os.Clearenv()
	t.Setenv("TIMEZONE", "Invalid/Timezone")

	_, err := config.Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "zona waktu")
}

func TestConfig_InvalidTimeFormat(t *testing.T) {
	os.Clearenv()
	t.Setenv("MONEV_REMINDER_TIME_1", "invalid_time")

	_, err := config.Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MONEV_REMINDER_TIME_1")
}

