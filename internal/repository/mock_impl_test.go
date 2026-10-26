package repository_test

import (
	"context"
	"testing"
	"time"

	"botteleabsensi/internal/model"
	"botteleabsensi/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockRepository_GroupAndTaskWorkflow(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewMockRepository()

	// 1. Grup
	err := repo.SetGroupAllowed(ctx, -1001234567890, "Grip Tim Proyek", true)
	require.NoError(t, err)

	g, err := repo.GetGroup(ctx, -1001234567890)
	require.NoError(t, err)
	require.NotNil(t, g)
	assert.Equal(t, "Grip Tim Proyek", g.Title)
	assert.True(t, g.IsAllowed)

	// 2. Simpan Pesan Sumber
	msg := &model.SourceMessage{
		MessageID:  101,
		ChatID:     -1001234567890,
		SenderID:   999,
		SenderName: "Fadhil",
		Text:       "Kita selesaikan laporan Monev hari Kamis",
		SentAt:     time.Now(),
	}
	err = repo.SaveMessage(ctx, msg)
	require.NoError(t, err)

	// Uji pesan duplikat tidak menyebabkan error
	err = repo.SaveMessage(ctx, msg)
	require.NoError(t, err)

	// 3. Tambah Tugas
	due := time.Now().Add(24 * time.Hour)
	task := &model.Task{
		GroupID:         -1001234567890,
		SourceMessageID: &msg.ID,
		Title:           "Selesaikan laporan Monev",
		Description:     "Ekspor PDF laporan minggu ke-4",
		Assignee:        "Fadhil",
		DueDate:         &due,
		Status:          model.TaskStatusBelumDikerjakan,
		Certainty:       "pasti",
	}
	taskID, err := repo.CreateTask(ctx, task)
	require.NoError(t, err)
	assert.Equal(t, int64(1), taskID)

	// 4. Update Status Tugas & Verifikasi Riwayat
	err = repo.UpdateTaskStatus(ctx, taskID, model.TaskStatusSedangDikerjakan, "Fadhil", "Mulai pengerjaan export PDF")
	require.NoError(t, err)

	updatedTask, err := repo.GetTaskByID(ctx, taskID)
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSedangDikerjakan, updatedTask.Status)

	histories, err := repo.GetTaskHistories(ctx, taskID)
	require.NoError(t, err)
	require.Len(t, histories, 1)
	assert.Equal(t, "belum_dikerjakan", histories[0].OldStatus)
	assert.Equal(t, "sedang_dikerjakan", histories[0].NewStatus)
	assert.Equal(t, "Mulai pengerjaan export PDF", histories[0].ChangeReason)

	// 5. Tandai Selesai
	err = repo.UpdateTaskStatus(ctx, taskID, model.TaskStatusSelesai, "Fadhil", "Selesai diunggah")
	require.NoError(t, err)

	histories, err = repo.GetTaskHistories(ctx, taskID)
	require.NoError(t, err)
	assert.Len(t, histories, 2)
}

func TestMockRepository_MonevIdempotency(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewMockRepository()

	groupID := int64(-1001234567890)
	date := "2026-10-26"
	slot := "15:00"

	// Lock pertama kali harus berhasil
	locked, err := repo.TryLockReminderLog(ctx, groupID, date, slot)
	require.NoError(t, err)
	assert.True(t, locked)

	// Cek status saat ini belum sukses
	sent, err := repo.IsAlreadySent(ctx, groupID, date, slot)
	require.NoError(t, err)
	assert.False(t, sent)

	// Tandai Sukses
	now := time.Now()
	err = repo.MarkSuccess(ctx, groupID, date, slot, now)
	require.NoError(t, err)

	// Cek status harus sudah sukses
	sent, err = repo.IsAlreadySent(ctx, groupID, date, slot)
	require.NoError(t, err)
	assert.True(t, sent)

	// Percobaan lock kedua kalinya pada tanggal & slot yang sama harus DITOLAK
	lockedAgain, err := repo.TryLockReminderLog(ctx, groupID, date, slot)
	require.NoError(t, err)
	assert.False(t, lockedAgain, "Pengiriman kedua harus ditolak untuk mencegah pesan ganda")
}

