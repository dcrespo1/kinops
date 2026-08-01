-- +goose Up

CREATE TABLE household_events (
    id INTEGER PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    location TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL DEFAULT '',
    all_day INTEGER NOT NULL DEFAULT 1 CHECK (all_day IN (0, 1)),
    start_date TEXT NOT NULL,
    end_date TEXT NOT NULL,
    start_time TEXT,
    end_time TEXT,
    recurrence_type TEXT NOT NULL CHECK (
        recurrence_type IN (
            'one_off', 'daily', 'every_n_days', 'weekly_days',
            'monthly_day', 'annual'
        )
    ),
    recurrence_params TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(recurrence_params)),
    recurrence_end_date TEXT,
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),

    CHECK (length(trim(title)) > 0),
    CHECK (
        (all_day = 1 AND end_date > start_date)
        OR (all_day = 0 AND end_date >= start_date)
    ),
    CHECK (recurrence_end_date IS NULL OR recurrence_end_date >= start_date),
    CHECK (
        (all_day = 1 AND start_time IS NULL AND end_time IS NULL)
        OR
        (all_day = 0 AND start_time IS NOT NULL AND end_time IS NOT NULL)
    )
);

CREATE TABLE event_audiences (
    event_id INTEGER NOT NULL,
    person_id INTEGER NOT NULL,
    PRIMARY KEY (event_id, person_id),
    FOREIGN KEY (event_id) REFERENCES household_events (id) ON DELETE CASCADE,
    FOREIGN KEY (person_id) REFERENCES people (id) ON DELETE RESTRICT
);

CREATE TABLE event_occurrences (
    id INTEGER PRIMARY KEY,
    event_id INTEGER NOT NULL,
    occurrence_key TEXT NOT NULL,
    start_date TEXT NOT NULL,
    end_date TEXT NOT NULL,
    start_at TEXT,
    end_at TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),

    FOREIGN KEY (event_id) REFERENCES household_events (id) ON DELETE CASCADE,
    UNIQUE (event_id, occurrence_key),
    CHECK (end_date > start_date),
    CHECK (
        (start_at IS NULL AND end_at IS NULL)
        OR
        (start_at IS NOT NULL AND end_at IS NOT NULL AND end_at > start_at)
    )
);

CREATE INDEX event_occurrences_date_idx
    ON event_occurrences (start_date, end_date);

CREATE INDEX event_audiences_person_idx
    ON event_audiences (person_id, event_id);

-- +goose Down

DROP TABLE IF EXISTS event_occurrences;
DROP TABLE IF EXISTS event_audiences;
DROP TABLE IF EXISTS household_events;
