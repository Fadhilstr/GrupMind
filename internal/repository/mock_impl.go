package repository

import (
	"context"
	"fmt"
	"sync"
	"time"

	"botteleabsensi/internal/model"
)

// MockRepository menyediakan implementasi repository berbasis in-memory untuk unit testing
type MockRepository struct {
	mu             sync.RWMutex
	groups         map[int64]*model.Group
	users          map[int64]*model.User
	messages       []*model.SourceMessage
	tasks          map[int64]*model.Task
	taskHistories  map[int64][]*model.TaskHistory
	decisions      map[int64]*model.Decision
	schedules      map[int64]*model.Schedule
	dailySummaries map[string]*model.DailySummary
	monevLogs      map[string]*model.MonevReminderLog

	nextTaskID     int64
	nextDecisionID int64
	nextScheduleID int64
}

// NewMockRepository membuat instance MockRepository
func NewMockRepository() *MockRepository {
	return &MockRepository{
		groups:         make(map[int64]*model.Group),
		users:          make(map[int64]*model.User),
		tasks:          make(map[int64]*model.Task),
		taskHistories:  make(map[int64][]*model.TaskHistory),
		decisions:      make(map[int64]*model.Decision),
		schedules:      make(map[int64]*model.Schedule),
		dailySummaries: make(map[string]*model.DailySummary),
		monevLogs:      make(map[string]*model.MonevReminderLog),
		nextTaskID:     1,
		nextDecisionID: 1,
		nextScheduleID: 1,
	}
}

// Group
func (m *MockRepository) UpsertGroup(ctx context.Context, g *model.Group) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.groups[g.ID]; ok {
		existing.Title = g.Title
		existing.UpdatedAt = time.Now()
	} else {
		m.groups[g.ID] = &model.Group{
			ID:        g.ID,
			Title:     g.Title,
			IsAllowed: g.IsAllowed,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
	}
	return nil
}

func (m *MockRepository) GetGroup(ctx context.Context, id int64) (*model.Group, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if g, ok := m.groups[id]; ok {
		return g, nil
	}
	return nil, nil
}

func (m *MockRepository) SetGroupAllowed(ctx context.Context, id int64, title string, allowed bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.groups[id]; ok {
		existing.IsAllowed = allowed
		if title != "" {
			existing.Title = title
		}
		existing.UpdatedAt = time.Now()
	} else {
		m.groups[id] = &model.Group{
			ID:        id,
			Title:     title,
			IsAllowed: allowed,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
	}
	return nil
}

func (m *MockRepository) ListAllowedGroups(ctx context.Context) ([]*model.Group, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*model.Group
	for _, g := range m.groups {
		if g.IsAllowed {
			list = append(list, g)
		}
	}
	return list, nil
}

// User
func (m *MockRepository) UpsertUser(ctx context.Context, u *model.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[u.ID] = u
	return nil
}

func (m *MockRepository) GetUser(ctx context.Context, id int64) (*model.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if u, ok := m.users[id]; ok {
		return u, nil
	}
	return nil, nil
}

// Message
func (m *MockRepository) SaveMessage(ctx context.Context, msg *model.SourceMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.messages {
		if existing.ChatID == msg.ChatID && existing.MessageID == msg.MessageID {
			return nil // Duplikat
		}
	}
	msg.ID = int64(len(m.messages) + 1)
	m.messages = append(m.messages, msg)
	return nil
}

func (m *MockRepository) GetRecentMessages(ctx context.Context, chatID int64, limit int) ([]*model.SourceMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var msgs []*model.SourceMessage
	for _, msg := range m.messages {
		if msg.ChatID == chatID {
			msgs = append(msgs, msg)
		}
	}
	if len(msgs) > limit {
		msgs = msgs[len(msgs)-limit:]
	}
	return msgs, nil
}

func (m *MockRepository) GetMessagesBetween(ctx context.Context, chatID int64, start, end time.Time) ([]*model.SourceMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var msgs []*model.SourceMessage
	for _, msg := range m.messages {
		if msg.ChatID == chatID && (msg.SentAt.Equal(start) || msg.SentAt.After(start)) && (msg.SentAt.Equal(end) || msg.SentAt.Before(end)) {
			msgs = append(msgs, msg)
		}
	}
	return msgs, nil
}

// Task
func (m *MockRepository) CreateTask(ctx context.Context, t *model.Task) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t.ID = m.nextTaskID
	m.nextTaskID++
	t.CreatedAt = time.Now()
	t.UpdatedAt = time.Now()
	m.tasks[t.ID] = t
	return t.ID, nil
}

func (m *MockRepository) GetTaskByID(ctx context.Context, id int64) (*model.Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if t, ok := m.tasks[id]; ok {
		return t, nil
	}
	return nil, nil
}

func (m *MockRepository) ListTasks(ctx context.Context, groupID int64, status *model.TaskStatus) ([]*model.Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*model.Task
	for _, t := range m.tasks {
		if t.GroupID == groupID {
			if status == nil || t.Status == *status {
				list = append(list, t)
			}
		}
	}
	return list, nil
}

func (m *MockRepository) ListTodayTasks(ctx context.Context, groupID int64, loc *time.Location) ([]*model.Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	now := time.Now().In(loc)
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	endOfDay := startOfDay.Add(24 * time.Hour)

	var list []*model.Task
	for _, t := range m.tasks {
		if t.GroupID == groupID && t.DueDate != nil {
			due := t.DueDate.In(loc)
			if (due.Equal(startOfDay) || due.After(startOfDay)) && due.Before(endOfDay) {
				list = append(list, t)
			}
		}
	}
	return list, nil
}

func (m *MockRepository) ListUpcomingTasks(ctx context.Context, groupID int64, loc *time.Location) ([]*model.Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	now := time.Now().In(loc)
	var list []*model.Task
	for _, t := range m.tasks {
		if t.GroupID == groupID && t.DueDate != nil && (t.DueDate.Equal(now) || t.DueDate.After(now)) && t.Status != model.TaskStatusSelesai && t.Status != model.TaskStatusDibatalkan {
			list = append(list, t)
		}
	}
	return list, nil
}

func (m *MockRepository) UpdateTaskStatus(ctx context.Context, taskID int64, newStatus model.TaskStatus, changedBy, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[taskID]
	if !ok {
		return fmt.Errorf("tugas dengan ID %d tidak ditemukan", taskID)
	}
	oldStatus := string(t.Status)
	t.Status = newStatus
	t.UpdatedAt = time.Now()

	m.taskHistories[taskID] = append(m.taskHistories[taskID], &model.TaskHistory{
		ID:           int64(len(m.taskHistories[taskID]) + 1),
		TaskID:       taskID,
		ChangedBy:    changedBy,
		OldStatus:    oldStatus,
		NewStatus:    string(newStatus),
		OldDueDate:   t.DueDate,
		NewDueDate:   t.DueDate,
		ChangeReason: reason,
		ChangedAt:    time.Now(),
	})
	return nil
}

func (m *MockRepository) UpdateTaskDueDate(ctx context.Context, taskID int64, newDueDate *time.Time, changedBy, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[taskID]
	if !ok {
		return fmt.Errorf("tugas dengan ID %d tidak ditemukan", taskID)
	}
	oldDue := t.DueDate
	t.DueDate = newDueDate
	t.UpdatedAt = time.Now()

	m.taskHistories[taskID] = append(m.taskHistories[taskID], &model.TaskHistory{
		ID:           int64(len(m.taskHistories[taskID]) + 1),
		TaskID:       taskID,
		ChangedBy:    changedBy,
		OldStatus:    string(t.Status),
		NewStatus:    string(t.Status),
		OldDueDate:   oldDue,
		NewDueDate:   newDueDate,
		ChangeReason: reason,
		ChangedAt:    time.Now(),
	})
	return nil
}

func (m *MockRepository) GetTaskHistories(ctx context.Context, taskID int64) ([]*model.TaskHistory, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.taskHistories[taskID], nil
}

// Decision
func (m *MockRepository) CreateDecision(ctx context.Context, d *model.Decision) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d.ID = m.nextDecisionID
	m.nextDecisionID++
	d.CreatedAt = time.Now()
	m.decisions[d.ID] = d
	return d.ID, nil
}

func (m *MockRepository) ListDecisions(ctx context.Context, groupID int64) ([]*model.Decision, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*model.Decision
	for _, d := range m.decisions {
		if d.GroupID == groupID {
			list = append(list, d)
		}
	}
	return list, nil
}

func (m *MockRepository) UpdateDecisionStatus(ctx context.Context, id int64, status model.DecisionStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.decisions[id]
	if !ok {
		return fmt.Errorf("keputusan ID %d tidak ditemukan", id)
	}
	d.Status = status
	return nil
}

// Schedule
func (m *MockRepository) CreateSchedule(ctx context.Context, s *model.Schedule) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s.ID = m.nextScheduleID
	m.nextScheduleID++
	s.CreatedAt = time.Now()
	m.schedules[s.ID] = s
	return s.ID, nil
}

func (m *MockRepository) ListUpcomingSchedules(ctx context.Context, groupID int64, from time.Time) ([]*model.Schedule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*model.Schedule
	for _, s := range m.schedules {
		if s.GroupID == groupID && (s.EventTime.Equal(from) || s.EventTime.After(from)) {
			list = append(list, s)
		}
	}
	return list, nil
}

// Summary
func (m *MockRepository) SaveDailySummary(ctx context.Context, s *model.DailySummary) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%d_%s", s.GroupID, s.SummaryDate)
	s.CreatedAt = time.Now()
	m.dailySummaries[key] = s
	return nil
}

func (m *MockRepository) GetDailySummary(ctx context.Context, groupID int64, date string) (*model.DailySummary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%d_%s", groupID, date)
	return m.dailySummaries[key], nil
}

func (m *MockRepository) GetLatestDailySummary(ctx context.Context, groupID int64) (*model.DailySummary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var latest *model.DailySummary
	for _, s := range m.dailySummaries {
		if s.GroupID == groupID {
			if latest == nil || s.SummaryDate > latest.SummaryDate {
				latest = s
			}
		}
	}
	return latest, nil
}

// Monev
func (m *MockRepository) TryLockReminderLog(ctx context.Context, groupID int64, date, slot string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%d_%s_%s", groupID, date, slot)
	log, exists := m.monevLogs[key]
	if exists {
		if log.Status == model.MonevStatusSuccess || log.Status == model.MonevStatusMissed {
			return false, nil
		}
		log.Status = model.MonevStatusPending
		log.Attempts++
		log.UpdatedAt = time.Now()
		return true, nil
	}
	m.monevLogs[key] = &model.MonevReminderLog{
		GroupID:      groupID,
		ReminderDate: date,
		Slot:         slot,
		Status:       model.MonevStatusPending,
		Attempts:     1,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	return true, nil
}

func (m *MockRepository) MarkSuccess(ctx context.Context, groupID int64, date, slot string, sentAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%d_%s_%s", groupID, date, slot)
	if log, ok := m.monevLogs[key]; ok {
		log.Status = model.MonevStatusSuccess
		log.SentAt = &sentAt
		log.LastError = ""
		log.UpdatedAt = time.Now()
	}
	return nil
}

func (m *MockRepository) MarkFailed(ctx context.Context, groupID int64, date, slot string, lastErr string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%d_%s_%s", groupID, date, slot)
	if log, ok := m.monevLogs[key]; ok {
		log.Status = model.MonevStatusFailed
		log.LastError = lastErr
		log.UpdatedAt = time.Now()
	}
	return nil
}

func (m *MockRepository) MarkMissed(ctx context.Context, groupID int64, date, slot string, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%d_%s_%s", groupID, date, slot)
	log, ok := m.monevLogs[key]
	if !ok {
		log = &model.MonevReminderLog{
			GroupID:      groupID,
			ReminderDate: date,
			Slot:         slot,
			Status:       model.MonevStatusMissed,
			Attempts:     0,
			LastError:    reason,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		m.monevLogs[key] = log
	} else if log.Status != model.MonevStatusSuccess {
		log.Status = model.MonevStatusMissed
		log.LastError = reason
		log.UpdatedAt = time.Now()
	}
	return nil
}

func (m *MockRepository) IsAlreadySent(ctx context.Context, groupID int64, date, slot string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%d_%s_%s", groupID, date, slot)
	if log, ok := m.monevLogs[key]; ok {
		return log.Status == model.MonevStatusSuccess, nil
	}
	return false, nil
}

func (m *MockRepository) GetLog(ctx context.Context, groupID int64, date, slot string) (*model.MonevReminderLog, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%d_%s_%s", groupID, date, slot)
	if log, ok := m.monevLogs[key]; ok {
		return log, nil
	}
	return nil, nil
}

