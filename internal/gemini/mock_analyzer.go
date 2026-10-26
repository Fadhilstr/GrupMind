package gemini

import (
	"context"
	"fmt"
	"strings"
	"time"

	"botteleabsensi/internal/model"
)

// MockAnalyzer menyediakan implementasi analyzer tiruan untuk pengujian otomatis
type MockAnalyzer struct {
	CustomResult *AnalysisResult
	CustomErr    error
}

// NewMockAnalyzer membuat instance MockAnalyzer
func NewMockAnalyzer() *MockAnalyzer {
	return &MockAnalyzer{}
}

func (m *MockAnalyzer) AnalyzeConversation(ctx context.Context, messages []*model.SourceMessage, refTime time.Time) (*AnalysisResult, error) {
	if m.CustomErr != nil {
		return nil, m.CustomErr
	}
	if m.CustomResult != nil {
		return m.CustomResult, nil
	}

	// Default intelligent heuristic mock untuk testing multi-pesan
	res := &AnalysisResult{
		Topic:   "Pengembangan Modul Laporan dan Absensi",
		Summary: "Diskusi mengenai pembagian tugas laporan Monev dan export PDF.",
	}

	for _, msg := range messages {
		textLower := strings.ToLower(msg.Text)
		if strings.Contains(textLower, "laporan") || strings.Contains(textLower, "pdf") {
			dueDate := refTime.Add(24 * time.Hour).Format("2006-01-02T17:00:00")
			res.Tasks = append(res.Tasks, ExtractedTask{
				Title:           "Pengerjaan Modul Export PDF",
				Description:     "Mengerjakan bagian export PDF modul laporan",
				Assignee:        msg.SenderName,
				DueDateStr:      dueDate,
				Certainty:       "pasti",
				Status:          "belum_dikerjakan",
				SourceMessageID: msg.MessageID,
			})
		}
		if strings.Contains(textLower, "sepakat") || strings.Contains(textLower, "deadline") {
			res.Decisions = append(res.Decisions, ExtractedDecision{
				Topic:           "Tenggat Laporan",
				DecisionText:    "Deadline disepakati Jumat sore",
				Status:          "final",
				SourceMessageID: msg.MessageID,
			})
		}
		if strings.Contains(textLower, "bagaimana kalau") || strings.Contains(textLower, "?") {
			res.UnresolvedQuestions = append(res.UnresolvedQuestions, msg.Text)
		}
	}

	return res, nil
}

func (m *MockAnalyzer) GenerateDailySummary(ctx context.Context, messages []*model.SourceMessage, tasks []*model.Task, decisions []*model.Decision, date string) (string, error) {
	if m.CustomErr != nil {
		return "", m.CustomErr
	}
	return fmt.Sprintf("📋 Rangkuman Harian %s: Terdapat %d pesan, %d tugas, %d keputusan.", date, len(messages), len(tasks), len(decisions)), nil
}

