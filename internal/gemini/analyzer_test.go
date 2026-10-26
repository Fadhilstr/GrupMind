package gemini_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"botteleabsensi/internal/gemini"
	"botteleabsensi/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStructuredOutput_Unmarshal(t *testing.T) {
	sampleJSON := `{
		"topic": "Implementasi Ekspor PDF",
		"summary": "Tim membahas pembagian tugas modul laporan dan jadwal penyelesaian.",
		"decisions": [
			{
				"topic": "Format Dokumen",
				"decision_text": "Laporan disetujui diekspor dalam format PDF standar.",
				"status": "final",
				"source_message_id": 105
			}
		],
		"tasks": [
			{
				"title": "Mengerjakan Ekspor PDF",
				"description": "Modul laporan diekspor ke PDF dengan tata letak rapi",
				"assignee": "Ahmad",
				"due_date_str": "2026-10-30T17:00:00",
				"certainty": "pasti",
				"status": "belum_dikerjakan",
				"source_message_id": 102
			}
		],
		"schedules": [
			{
				"event_name": "Review Progres",
				"event_time_str": "2026-10-29T10:00:00",
				"location": "Google Meet",
				"source_message_id": 104
			}
		],
		"unresolved_questions": [
			"Apakah perlu menyertakan tanda tangan digital?"
		],
		"cancelled_or_modified_items": [
			{
				"item_type": "decision",
				"description": "Pembatalan opsi format Word .docx",
				"change_reason": "Lebih disukai PDF konsisten"
			}
		]
	}`

	var res gemini.AnalysisResult
	err := json.Unmarshal([]byte(sampleJSON), &res)
	require.NoError(t, err)

	assert.Equal(t, "Implementasi Ekspor PDF", res.Topic)
	require.Len(t, res.Decisions, 1)
	assert.Equal(t, "final", res.Decisions[0].Status)
	assert.Equal(t, int64(105), res.Decisions[0].SourceMessageID)

	require.Len(t, res.Tasks, 1)
	assert.Equal(t, "Ahmad", res.Tasks[0].Assignee)
	assert.Equal(t, "2026-10-30T17:00:00", res.Tasks[0].DueDateStr)

	require.Len(t, res.Schedules, 1)
	assert.Equal(t, "Google Meet", res.Schedules[0].Location)

	require.Len(t, res.UnresolvedQuestions, 1)
	assert.Contains(t, res.UnresolvedQuestions[0], "tanda tangan digital")
}

func TestMockAnalyzer_ConversationAnalysis(t *testing.T) {
	ctx := context.Background()
	analyzer := gemini.NewMockAnalyzer()

	refTime := time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)
	msgs := []*model.SourceMessage{
		{
			MessageID:  101,
			ChatID:     -100123,
			SenderName: "Budi",
			Text:       "Kita kerjakan modul laporan hari ini.",
			SentAt:     refTime,
		},
		{
			MessageID:  102,
			ChatID:     -100123,
			SenderName: "Fadhil",
			Text:       "Bagian export PDF saya kerjakan.",
			SentAt:     refTime.Add(time.Minute),
		},
		{
			MessageID:  103,
			ChatID:     -100123,
			SenderName: "Siti",
			Text:       "Deadline sepakat hari Jumat sore ya.",
			SentAt:     refTime.Add(2 * time.Minute),
		},
		{
			MessageID:  104,
			ChatID:     -100123,
			SenderName: "Budi",
			Text:       "Bagaimana kalau ada data kosong?",
			SentAt:     refTime.Add(3 * time.Minute),
		},
	}

	res, err := analyzer.AnalyzeConversation(ctx, msgs, refTime)
	require.NoError(t, err)
	assert.NotEmpty(t, res.Topic)
	assert.NotEmpty(t, res.Tasks)
	assert.NotEmpty(t, res.Decisions)
	assert.NotEmpty(t, res.UnresolvedQuestions)
}

