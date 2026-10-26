-- Migrasi Inisialisasi Skema Database Bot Telegram & Monev AI Agent

-- 1. Tabel Grup Telegram
CREATE TABLE IF NOT EXISTS groups (
    id BIGINT PRIMARY KEY,
    title TEXT NOT NULL DEFAULT '',
    is_allowed BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 2. Tabel Pengguna Telegram
CREATE TABLE IF NOT EXISTS users (
    id BIGINT PRIMARY KEY,
    username TEXT NOT NULL DEFAULT '',
    first_name TEXT NOT NULL DEFAULT '',
    last_name TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT 'member',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 3. Tabel Pesan Sumber Percakapan (Audit Trail)
CREATE TABLE IF NOT EXISTS source_messages (
    id BIGSERIAL PRIMARY KEY,
    message_id BIGINT NOT NULL,
    chat_id BIGINT NOT NULL,
    sender_id BIGINT NOT NULL,
    sender_name TEXT NOT NULL DEFAULT '',
    text TEXT NOT NULL DEFAULT '',
    sent_at TIMESTAMPTZ NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_source_messages_chat_msg UNIQUE (chat_id, message_id)
);

CREATE INDEX IF NOT EXISTS idx_source_messages_chat_sent ON source_messages (chat_id, sent_at DESC);

-- 4. Tabel Tugas (Tasks)
CREATE TABLE IF NOT EXISTS tasks (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL,
    source_message_id BIGINT,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    assignee TEXT NOT NULL DEFAULT '',
    due_date TIMESTAMPTZ,
    status VARCHAR(30) NOT NULL DEFAULT 'belum_dikerjakan',
    certainty VARCHAR(30) NOT NULL DEFAULT 'pasti',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_tasks_source_message FOREIGN KEY (source_message_id) REFERENCES source_messages(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_tasks_group_status ON tasks (group_id, status);
CREATE INDEX IF NOT EXISTS idx_tasks_group_due_date ON tasks (group_id, due_date);

-- 5. Tabel Riwayat Perubahan Tugas (Task Histories)
CREATE TABLE IF NOT EXISTS task_histories (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL,
    changed_by TEXT NOT NULL DEFAULT '',
    old_status VARCHAR(30) NOT NULL DEFAULT '',
    new_status VARCHAR(30) NOT NULL DEFAULT '',
    old_due_date TIMESTAMPTZ,
    new_due_date TIMESTAMPTZ,
    change_reason TEXT NOT NULL DEFAULT '',
    changed_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_task_histories_task FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_task_histories_task_id ON task_histories (task_id, changed_at DESC);

-- 6. Tabel Keputusan dan Kesimpulan (Decisions)
CREATE TABLE IF NOT EXISTS decisions (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL,
    source_message_id BIGINT,
    topic TEXT NOT NULL,
    decision_text TEXT NOT NULL,
    status VARCHAR(30) NOT NULL DEFAULT 'final',
    decided_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_decisions_source_message FOREIGN KEY (source_message_id) REFERENCES source_messages(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_decisions_group_status ON decisions (group_id, status);

-- 7. Tabel Jadwal & Agenda (Schedules)
CREATE TABLE IF NOT EXISTS schedules (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL,
    source_message_id BIGINT,
    event_name TEXT NOT NULL,
    event_time TIMESTAMPTZ NOT NULL,
    location TEXT NOT NULL DEFAULT '',
    status VARCHAR(30) NOT NULL DEFAULT 'aktif',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_schedules_source_message FOREIGN KEY (source_message_id) REFERENCES source_messages(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_schedules_group_time ON schedules (group_id, event_time);

-- 8. Tabel Rangkuman Harian (Daily Summaries)
CREATE TABLE IF NOT EXISTS daily_summaries (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL,
    summary_date DATE NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_daily_summaries_group_date UNIQUE (group_id, summary_date)
);

-- 9. Tabel Log Pengingat Monev (Monev Reminder Logs)
-- Memastikan pengiriman idempoten, mencegah pengiriman ganda per (group_id, reminder_date, slot)
CREATE TABLE IF NOT EXISTS monev_reminder_logs (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL,
    reminder_date DATE NOT NULL,
    slot VARCHAR(10) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    attempts INT NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_monev_logs_group_date_slot UNIQUE (group_id, reminder_date, slot)
);

CREATE INDEX IF NOT EXISTS idx_monev_logs_status ON monev_reminder_logs (status, reminder_date);

