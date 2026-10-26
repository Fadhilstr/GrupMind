package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"botteleabsensi/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository mengimplementasikan seluruh antarmuka repositori menggunakan PostgreSQL
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository membuat instance baru PostgresRepository
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// ============================================================================
// GROUP REPOSITORY
// ============================================================================

func (r *PostgresRepository) UpsertGroup(ctx context.Context, g *model.Group) error {
	query := `
		INSERT INTO groups (id, title, is_allowed, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (id) DO UPDATE
		SET title = EXCLUDED.title, updated_at = NOW();
	`
	_, err := r.pool.Exec(ctx, query, g.ID, g.Title, g.IsAllowed)
	return err
}

func (r *PostgresRepository) GetGroup(ctx context.Context, id int64) (*model.Group, error) {
	query := `SELECT id, title, is_allowed, created_at, updated_at FROM groups WHERE id = $1;`
	row := r.pool.QueryRow(ctx, query, id)
	var g model.Group
	err := row.Scan(&g.ID, &g.Title, &g.IsAllowed, &g.CreatedAt, &g.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func (r *PostgresRepository) SetGroupAllowed(ctx context.Context, id int64, title string, allowed bool) error {
	query := `
		INSERT INTO groups (id, title, is_allowed, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (id) DO UPDATE
		SET is_allowed = EXCLUDED.is_allowed,
		    title = CASE WHEN EXCLUDED.title <> '' THEN EXCLUDED.title ELSE groups.title END,
		    updated_at = NOW();
	`
	_, err := r.pool.Exec(ctx, query, id, title, allowed)
	return err
}

func (r *PostgresRepository) ListAllowedGroups(ctx context.Context) ([]*model.Group, error) {
	query := `SELECT id, title, is_allowed, created_at, updated_at FROM groups WHERE is_allowed = TRUE ORDER BY title ASC;`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []*model.Group
	for rows.Next() {
		var g model.Group
		if err := rows.Scan(&g.ID, &g.Title, &g.IsAllowed, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, &g)
	}
	return groups, rows.Err()
}

// ============================================================================
// USER REPOSITORY
// ============================================================================

func (r *PostgresRepository) UpsertUser(ctx context.Context, u *model.User) error {
	query := `
		INSERT INTO users (id, username, first_name, last_name, role, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (id) DO UPDATE
		SET username = EXCLUDED.username,
		    first_name = EXCLUDED.first_name,
		    last_name = EXCLUDED.last_name;
	`
	_, err := r.pool.Exec(ctx, query, u.ID, u.Username, u.FirstName, u.LastName, u.Role)
	return err
}

func (r *PostgresRepository) GetUser(ctx context.Context, id int64) (*model.User, error) {
	query := `SELECT id, username, first_name, last_name, role, created_at FROM users WHERE id = $1;`
	row := r.pool.QueryRow(ctx, query, id)
	var u model.User
	err := row.Scan(&u.ID, &u.Username, &u.FirstName, &u.LastName, &u.Role, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// ============================================================================
// MESSAGE REPOSITORY
// ============================================================================

func (r *PostgresRepository) SaveMessage(ctx context.Context, msg *model.SourceMessage) error {
	query := `
		INSERT INTO source_messages (message_id, chat_id, sender_id, sender_name, text, sent_at, processed_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (chat_id, message_id) DO NOTHING
		RETURNING id;
	`
	var id int64
	err := r.pool.QueryRow(ctx, query, msg.MessageID, msg.ChatID, msg.SenderID, msg.SenderName, msg.Text, msg.SentAt).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Pesan duplikat telah diabaikan
			return nil
		}
		return err
	}
	msg.ID = id
	return nil
}

func (r *PostgresRepository) GetRecentMessages(ctx context.Context, chatID int64, limit int) ([]*model.SourceMessage, error) {
	query := `
		SELECT id, message_id, chat_id, sender_id, sender_name, text, sent_at, processed_at
		FROM source_messages
		WHERE chat_id = $1
		ORDER BY sent_at DESC
		LIMIT $2;
	`
	rows, err := r.pool.Query(ctx, query, chatID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []*model.SourceMessage
	for rows.Next() {
		var m model.SourceMessage
		if err := rows.Scan(&m.ID, &m.MessageID, &m.ChatID, &m.SenderID, &m.SenderName, &m.Text, &m.SentAt, &m.ProcessedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, &m)
	}
	// Balik urutan agar dari terlama ke terbaru (kronologis)
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, rows.Err()
}

func (r *PostgresRepository) GetMessagesBetween(ctx context.Context, chatID int64, start, end time.Time) ([]*model.SourceMessage, error) {
	query := `
		SELECT id, message_id, chat_id, sender_id, sender_name, text, sent_at, processed_at
		FROM source_messages
		WHERE chat_id = $1 AND sent_at >= $2 AND sent_at <= $3
		ORDER BY sent_at ASC;
	`
	rows, err := r.pool.Query(ctx, query, chatID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []*model.SourceMessage
	for rows.Next() {
		var m model.SourceMessage
		if err := rows.Scan(&m.ID, &m.MessageID, &m.ChatID, &m.SenderID, &m.SenderName, &m.Text, &m.SentAt, &m.ProcessedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, &m)
	}
	return msgs, rows.Err()
}

// ============================================================================
// TASK REPOSITORY
// ============================================================================

func (r *PostgresRepository) CreateTask(ctx context.Context, t *model.Task) (int64, error) {
	query := `
		INSERT INTO tasks (group_id, source_message_id, title, description, assignee, due_date, status, certainty, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		RETURNING id;
	`
	var id int64
	err := r.pool.QueryRow(ctx, query,
		t.GroupID, t.SourceMessageID, t.Title, t.Description, t.Assignee, t.DueDate, t.Status, t.Certainty,
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	t.ID = id
	return id, nil
}

func (r *PostgresRepository) GetTaskByID(ctx context.Context, id int64) (*model.Task, error) {
	query := `
		SELECT id, group_id, source_message_id, title, description, assignee, due_date, status, certainty, created_at, updated_at
		FROM tasks
		WHERE id = $1;
	`
	row := r.pool.QueryRow(ctx, query, id)
	var t model.Task
	err := row.Scan(&t.ID, &t.GroupID, &t.SourceMessageID, &t.Title, &t.Description, &t.Assignee, &t.DueDate, &t.Status, &t.Certainty, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *PostgresRepository) ListTasks(ctx context.Context, groupID int64, status *model.TaskStatus) ([]*model.Task, error) {
	var query string
	var args []any

	if status != nil {
		query = `
			SELECT id, group_id, source_message_id, title, description, assignee, due_date, status, certainty, created_at, updated_at
			FROM tasks
			WHERE group_id = $1 AND status = $2
			ORDER BY id DESC;
		`
		args = []any{groupID, string(*status)}
	} else {
		query = `
			SELECT id, group_id, source_message_id, title, description, assignee, due_date, status, certainty, created_at, updated_at
			FROM tasks
			WHERE group_id = $1
			ORDER BY id DESC;
		`
		args = []any{groupID}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*model.Task
	for rows.Next() {
		var t model.Task
		if err := rows.Scan(&t.ID, &t.GroupID, &t.SourceMessageID, &t.Title, &t.Description, &t.Assignee, &t.DueDate, &t.Status, &t.Certainty, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, &t)
	}
	return tasks, rows.Err()
}

func (r *PostgresRepository) ListTodayTasks(ctx context.Context, groupID int64, loc *time.Location) ([]*model.Task, error) {
	now := time.Now().In(loc)
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	endOfDay := startOfDay.Add(24 * time.Hour)

	query := `
		SELECT id, group_id, source_message_id, title, description, assignee, due_date, status, certainty, created_at, updated_at
		FROM tasks
		WHERE group_id = $1 AND due_date >= $2 AND due_date < $3
		ORDER BY due_date ASC;
	`
	rows, err := r.pool.Query(ctx, query, groupID, startOfDay, endOfDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*model.Task
	for rows.Next() {
		var t model.Task
		if err := rows.Scan(&t.ID, &t.GroupID, &t.SourceMessageID, &t.Title, &t.Description, &t.Assignee, &t.DueDate, &t.Status, &t.Certainty, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, &t)
	}
	return tasks, rows.Err()
}

func (r *PostgresRepository) ListUpcomingTasks(ctx context.Context, groupID int64, loc *time.Location) ([]*model.Task, error) {
	now := time.Now().In(loc)
	query := `
		SELECT id, group_id, source_message_id, title, description, assignee, due_date, status, certainty, created_at, updated_at
		FROM tasks
		WHERE group_id = $1 AND due_date >= $2 AND status <> 'selesai' AND status <> 'dibatalkan'
		ORDER BY due_date ASC;
	`
	rows, err := r.pool.Query(ctx, query, groupID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*model.Task
	for rows.Next() {
		var t model.Task
		if err := rows.Scan(&t.ID, &t.GroupID, &t.SourceMessageID, &t.Title, &t.Description, &t.Assignee, &t.DueDate, &t.Status, &t.Certainty, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, &t)
	}
	return tasks, rows.Err()
}

func (r *PostgresRepository) UpdateTaskStatus(ctx context.Context, taskID int64, newStatus model.TaskStatus, changedBy, reason string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Ambil status lama
	var oldStatus string
	var oldDueDate *time.Time
	err = tx.QueryRow(ctx, "SELECT status, due_date FROM tasks WHERE id = $1 FOR UPDATE;", taskID).Scan(&oldStatus, &oldDueDate)
	if err != nil {
		return fmt.Errorf("tugas dengan ID %d tidak ditemukan: %w", taskID, err)
	}

	// Update tugas
	_, err = tx.Exec(ctx, "UPDATE tasks SET status = $1, updated_at = NOW() WHERE id = $2;", string(newStatus), taskID)
	if err != nil {
		return fmt.Errorf("gagal memperbarui status tugas: %w", err)
	}

	// Catat riwayat
	historyQuery := `
		INSERT INTO task_histories (task_id, changed_by, old_status, new_status, old_due_date, new_due_date, change_reason, changed_at)
		VALUES ($1, $2, $3, $4, $5, $5, $6, NOW());
	`
	_, err = tx.Exec(ctx, historyQuery, taskID, changedBy, oldStatus, string(newStatus), oldDueDate, reason)
	if err != nil {
		return fmt.Errorf("gagal mencatat riwayat perubahan tugas: %w", err)
	}

	return tx.Commit(ctx)
}

func (r *PostgresRepository) UpdateTaskDueDate(ctx context.Context, taskID int64, newDueDate *time.Time, changedBy, reason string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var currentStatus string
	var oldDueDate *time.Time
	err = tx.QueryRow(ctx, "SELECT status, due_date FROM tasks WHERE id = $1 FOR UPDATE;", taskID).Scan(&currentStatus, &oldDueDate)
	if err != nil {
		return fmt.Errorf("tugas dengan ID %d tidak ditemukan: %w", taskID, err)
	}

	_, err = tx.Exec(ctx, "UPDATE tasks SET due_date = $1, updated_at = NOW() WHERE id = $2;", newDueDate, taskID)
	if err != nil {
		return fmt.Errorf("gagal memperbarui tenggat tugas: %w", err)
	}

	historyQuery := `
		INSERT INTO task_histories (task_id, changed_by, old_status, new_status, old_due_date, new_due_date, change_reason, changed_at)
		VALUES ($1, $2, $3, $3, $4, $5, $6, NOW());
	`
	_, err = tx.Exec(ctx, historyQuery, taskID, changedBy, currentStatus, oldDueDate, newDueDate, reason)
	if err != nil {
		return fmt.Errorf("gagal mencatat riwayat perubahan tenggat: %w", err)
	}

	return tx.Commit(ctx)
}

func (r *PostgresRepository) GetTaskHistories(ctx context.Context, taskID int64) ([]*model.TaskHistory, error) {
	query := `
		SELECT id, task_id, changed_by, old_status, new_status, old_due_date, new_due_date, change_reason, changed_at
		FROM task_histories
		WHERE task_id = $1
		ORDER BY changed_at DESC;
	`
	rows, err := r.pool.Query(ctx, query, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var histories []*model.TaskHistory
	for rows.Next() {
		var h model.TaskHistory
		if err := rows.Scan(&h.ID, &h.TaskID, &h.ChangedBy, &h.OldStatus, &h.NewStatus, &h.OldDueDate, &h.NewDueDate, &h.ChangeReason, &h.ChangedAt); err != nil {
			return nil, err
		}
		histories = append(histories, &h)
	}
	return histories, rows.Err()
}

// ============================================================================
// DECISION REPOSITORY
// ============================================================================

func (r *PostgresRepository) CreateDecision(ctx context.Context, d *model.Decision) (int64, error) {
	query := `
		INSERT INTO decisions (group_id, source_message_id, topic, decision_text, status, decided_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		RETURNING id;
	`
	var id int64
	err := r.pool.QueryRow(ctx, query,
		d.GroupID, d.SourceMessageID, d.Topic, d.DecisionText, d.Status, d.DecidedAt,
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	d.ID = id
	return id, nil
}

func (r *PostgresRepository) ListDecisions(ctx context.Context, groupID int64) ([]*model.Decision, error) {
	query := `
		SELECT id, group_id, source_message_id, topic, decision_text, status, decided_at, created_at
		FROM decisions
		WHERE group_id = $1
		ORDER BY id DESC;
	`
	rows, err := r.pool.Query(ctx, query, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var decisions []*model.Decision
	for rows.Next() {
		var d model.Decision
		if err := rows.Scan(&d.ID, &d.GroupID, &d.SourceMessageID, &d.Topic, &d.DecisionText, &d.Status, &d.DecidedAt, &d.CreatedAt); err != nil {
			return nil, err
		}
		decisions = append(decisions, &d)
	}
	return decisions, rows.Err()
}

func (r *PostgresRepository) UpdateDecisionStatus(ctx context.Context, id int64, status model.DecisionStatus) error {
	query := `UPDATE decisions SET status = $1 WHERE id = $2;`
	_, err := r.pool.Exec(ctx, query, string(status), id)
	return err
}

// ============================================================================
// SCHEDULE REPOSITORY
// ============================================================================

func (r *PostgresRepository) CreateSchedule(ctx context.Context, s *model.Schedule) (int64, error) {
	query := `
		INSERT INTO schedules (group_id, source_message_id, event_name, event_time, location, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		RETURNING id;
	`
	var id int64
	err := r.pool.QueryRow(ctx, query,
		s.GroupID, s.SourceMessageID, s.EventName, s.EventTime, s.Location, s.Status,
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	s.ID = id
	return id, nil
}

func (r *PostgresRepository) ListUpcomingSchedules(ctx context.Context, groupID int64, from time.Time) ([]*model.Schedule, error) {
	query := `
		SELECT id, group_id, source_message_id, event_name, event_time, location, status, created_at
		FROM schedules
		WHERE group_id = $1 AND event_time >= $2
		ORDER BY event_time ASC;
	`
	rows, err := r.pool.Query(ctx, query, groupID, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []*model.Schedule
	for rows.Next() {
		var s model.Schedule
		if err := rows.Scan(&s.ID, &s.GroupID, &s.SourceMessageID, &s.EventName, &s.EventTime, &s.Location, &s.Status, &s.CreatedAt); err != nil {
			return nil, err
		}
		schedules = append(schedules, &s)
	}
	return schedules, rows.Err()
}

// ============================================================================
// SUMMARY REPOSITORY
// ============================================================================

func (r *PostgresRepository) SaveDailySummary(ctx context.Context, s *model.DailySummary) error {
	query := `
		INSERT INTO daily_summaries (group_id, summary_date, content, created_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (group_id, summary_date) DO UPDATE
		SET content = EXCLUDED.content, created_at = NOW();
	`
	_, err := r.pool.Exec(ctx, query, s.GroupID, s.SummaryDate, s.Content)
	return err
}

func (r *PostgresRepository) GetDailySummary(ctx context.Context, groupID int64, date string) (*model.DailySummary, error) {
	query := `SELECT id, group_id, summary_date, content, created_at FROM daily_summaries WHERE group_id = $1 AND summary_date = $2;`
	row := r.pool.QueryRow(ctx, query, groupID, date)
	var s model.DailySummary
	err := row.Scan(&s.ID, &s.GroupID, &s.SummaryDate, &s.Content, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *PostgresRepository) GetLatestDailySummary(ctx context.Context, groupID int64) (*model.DailySummary, error) {
	query := `SELECT id, group_id, summary_date, content, created_at FROM daily_summaries WHERE group_id = $1 ORDER BY summary_date DESC LIMIT 1;`
	row := r.pool.QueryRow(ctx, query, groupID)
	var s model.DailySummary
	err := row.Scan(&s.ID, &s.GroupID, &s.SummaryDate, &s.Content, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ============================================================================
// MONEV REMINDER REPOSITORY (IDEMPOTEN & ATOMIK)
// ============================================================================

func (r *PostgresRepository) TryLockReminderLog(ctx context.Context, groupID int64, date, slot string) (bool, error) {
	// Memeriksa status saat ini
	checkQuery := `SELECT status FROM monev_reminder_logs WHERE group_id = $1 AND reminder_date = $2 AND slot = $3;`
	var currentStatus string
	err := r.pool.QueryRow(ctx, checkQuery, groupID, date, slot).Scan(&currentStatus)
	if err == nil {
		// Log sudah ada
		if currentStatus == string(model.MonevStatusSuccess) || currentStatus == string(model.MonevStatusMissed) {
			// Sudah sukses atau dilewati secara resmi, jangan kirim ulang!
			return false, nil
		}
		// Status failed atau pending: coba ambil lock kembali dengan menambah attempts
		updateQuery := `
			UPDATE monev_reminder_logs
			SET status = 'pending', attempts = attempts + 1, updated_at = NOW()
			WHERE group_id = $1 AND reminder_date = $2 AND slot = $3 AND status <> 'success' AND status <> 'missed';
		`
		tag, err := r.pool.Exec(ctx, updateQuery, groupID, date, slot)
		if err != nil {
			return false, err
		}
		return tag.RowsAffected() > 0, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}

	// Belum ada log, insert awal dengan status 'pending'
	insertQuery := `
		INSERT INTO monev_reminder_logs (group_id, reminder_date, slot, status, attempts, created_at, updated_at)
		VALUES ($1, $2, $3, 'pending', 1, NOW(), NOW())
		ON CONFLICT (group_id, reminder_date, slot) DO NOTHING;
	`
	tag, err := r.pool.Exec(ctx, insertQuery, groupID, date, slot)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *PostgresRepository) MarkSuccess(ctx context.Context, groupID int64, date, slot string, sentAt time.Time) error {
	query := `
		UPDATE monev_reminder_logs
		SET status = 'success', sent_at = $4, last_error = '', updated_at = NOW()
		WHERE group_id = $1 AND reminder_date = $2 AND slot = $3;
	`
	_, err := r.pool.Exec(ctx, query, groupID, date, slot, sentAt)
	return err
}

func (r *PostgresRepository) MarkFailed(ctx context.Context, groupID int64, date, slot string, lastErr string) error {
	query := `
		UPDATE monev_reminder_logs
		SET status = 'failed', last_error = $4, updated_at = NOW()
		WHERE group_id = $1 AND reminder_date = $2 AND slot = $3;
	`
	_, err := r.pool.Exec(ctx, query, groupID, date, slot, lastErr)
	return err
}

func (r *PostgresRepository) MarkMissed(ctx context.Context, groupID int64, date, slot string, reason string) error {
	query := `
		INSERT INTO monev_reminder_logs (group_id, reminder_date, slot, status, last_error, created_at, updated_at)
		VALUES ($1, $2, $3, 'missed', $4, NOW(), NOW())
		ON CONFLICT (group_id, reminder_date, slot) DO UPDATE
		SET status = CASE WHEN monev_reminder_logs.status = 'success' THEN 'success' ELSE 'missed' END,
		    last_error = EXCLUDED.last_error,
		    updated_at = NOW();
	`
	_, err := r.pool.Exec(ctx, query, groupID, date, slot, reason)
	return err
}

func (r *PostgresRepository) IsAlreadySent(ctx context.Context, groupID int64, date, slot string) (bool, error) {
	query := `SELECT EXISTS (
		SELECT 1 FROM monev_reminder_logs
		WHERE group_id = $1 AND reminder_date = $2 AND slot = $3 AND status = 'success'
	);`
	var exists bool
	err := r.pool.QueryRow(ctx, query, groupID, date, slot).Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) GetLog(ctx context.Context, groupID int64, date, slot string) (*model.MonevReminderLog, error) {
	query := `
		SELECT id, group_id, reminder_date, slot, status, attempts, last_error, sent_at, created_at, updated_at
		FROM monev_reminder_logs
		WHERE group_id = $1 AND reminder_date = $2 AND slot = $3;
	`
	row := r.pool.QueryRow(ctx, query, groupID, date, slot)
	var log model.MonevReminderLog
	var dateVal time.Time
	err := row.Scan(&log.ID, &log.GroupID, &dateVal, &log.Slot, &log.Status, &log.Attempts, &log.LastError, &log.SentAt, &log.CreatedAt, &log.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	log.ReminderDate = dateVal.Format("2006-01-02")
	return &log, nil
}

