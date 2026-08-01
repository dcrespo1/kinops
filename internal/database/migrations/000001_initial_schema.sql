-- +goose Up

PRAGMA foreign_keys = ON;

CREATE TABLE people (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    color TEXT NOT NULL,
    calendar_token TEXT NOT NULL UNIQUE,
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    created_at TEXT NOT NULL DEFAULT (
        strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
    ),
    updated_at TEXT NOT NULL DEFAULT (
        strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
    ),

    CHECK (length(trim(name)) > 0),
    CHECK (length(trim(color)) > 0),
    CHECK (length(trim(calendar_token)) >= 32)
);

CREATE TABLE chores (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL DEFAULT '',
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    created_at TEXT NOT NULL DEFAULT (
        strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
    ),
    updated_at TEXT NOT NULL DEFAULT (
        strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
    ),

    CHECK (length(trim(name)) > 0)
);

CREATE TABLE schedules (
    id INTEGER PRIMARY KEY,
    chore_id INTEGER NOT NULL,

    rule_type TEXT NOT NULL CHECK (
        rule_type IN (
            'daily',
            'every_n_days',
            'weekly_days',
            'monthly_day',
            'one_off'
        )
    ),

    rule_params TEXT NOT NULL DEFAULT '{}'
        CHECK (json_valid(rule_params)),

    start_date TEXT NOT NULL,
    end_date TEXT,

    assignment_mode TEXT NOT NULL CHECK (
        assignment_mode IN ('fixed', 'rotate')
    ),

    fixed_person_id INTEGER,
    rotation_start_person_id INTEGER,

    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),

    created_at TEXT NOT NULL DEFAULT (
        strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
    ),
    updated_at TEXT NOT NULL DEFAULT (
        strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
    ),

    FOREIGN KEY (chore_id)
        REFERENCES chores (id)
        ON DELETE CASCADE,

    FOREIGN KEY (fixed_person_id)
        REFERENCES people (id)
        ON DELETE RESTRICT,

    FOREIGN KEY (rotation_start_person_id)
        REFERENCES people (id)
        ON DELETE RESTRICT,

    CHECK (
        (
            assignment_mode = 'fixed'
            AND fixed_person_id IS NOT NULL
            AND rotation_start_person_id IS NULL
        )
        OR
        (
            assignment_mode = 'rotate'
            AND fixed_person_id IS NULL
            AND rotation_start_person_id IS NOT NULL
        )
    ),

    CHECK (
        end_date IS NULL
        OR end_date >= start_date
    )
);

CREATE TABLE chore_instances (
    id INTEGER PRIMARY KEY,
    chore_id INTEGER NOT NULL,
    schedule_id INTEGER NOT NULL,

    sequence_no INTEGER NOT NULL CHECK (sequence_no > 0),
    due_date TEXT NOT NULL,
    assigned_person_id INTEGER NOT NULL,

    status TEXT NOT NULL DEFAULT 'pending' CHECK (
        status IN ('pending', 'done', 'skipped')
    ),

    completed_at TEXT,

    created_at TEXT NOT NULL DEFAULT (
        strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
    ),
    updated_at TEXT NOT NULL DEFAULT (
        strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
    ),

    FOREIGN KEY (chore_id)
        REFERENCES chores (id)
        ON DELETE RESTRICT,

    FOREIGN KEY (schedule_id)
        REFERENCES schedules (id)
        ON DELETE CASCADE,

    FOREIGN KEY (assigned_person_id)
        REFERENCES people (id)
        ON DELETE RESTRICT,

    UNIQUE (schedule_id, due_date),
    UNIQUE (schedule_id, sequence_no),

    CHECK (
        (status = 'done' AND completed_at IS NOT NULL)
        OR
        (status != 'done' AND completed_at IS NULL)
    )
);

CREATE TABLE completion_logs (
    id INTEGER PRIMARY KEY,
    chore_instance_id INTEGER NOT NULL,
    person_id INTEGER NOT NULL,

    event_type TEXT NOT NULL CHECK (
        event_type IN (
            'completed',
            'reopened',
            'skipped',
            'unskipped'
        )
    ),

    occurred_at TEXT NOT NULL DEFAULT (
        strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
    ),

    FOREIGN KEY (chore_instance_id)
        REFERENCES chore_instances (id)
        ON DELETE CASCADE,

    FOREIGN KEY (person_id)
        REFERENCES people (id)
        ON DELETE RESTRICT
);

CREATE INDEX schedules_chore_id_idx
    ON schedules (chore_id);

CREATE INDEX chore_instances_due_date_idx
    ON chore_instances (due_date);

CREATE INDEX chore_instances_person_due_date_idx
    ON chore_instances (assigned_person_id, due_date);

CREATE INDEX chore_instances_status_due_date_idx
    ON chore_instances (status, due_date);

CREATE INDEX completion_logs_person_occurred_idx
    ON completion_logs (person_id, occurred_at);

-- +goose Down

DROP TABLE IF EXISTS completion_logs;
DROP TABLE IF EXISTS chore_instances;
DROP TABLE IF EXISTS schedules;
DROP TABLE IF EXISTS chores;
DROP TABLE IF EXISTS people;