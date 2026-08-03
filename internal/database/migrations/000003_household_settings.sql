-- +goose Up

CREATE TABLE household_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    household_event_color TEXT NOT NULL DEFAULT '#a78bfa',
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),

    CHECK (
        length(household_event_color) = 7
        AND household_event_color GLOB '#[0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f]'
    )
);

INSERT INTO household_settings (id, household_event_color)
VALUES (1, '#a78bfa');

-- +goose Down

DROP TABLE IF EXISTS household_settings;
