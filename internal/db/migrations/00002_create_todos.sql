-- +goose Up
CREATE TABLE todos (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER  NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    title       TEXT     NOT NULL,
    description TEXT     NOT NULL DEFAULT '',
    status      TEXT     NOT NULL DEFAULT 'not_started'
        CHECK (status IN ('not_started', 'in_progress', 'in_review', 'completed')),
    due_date    TEXT,    -- calendar date, YYYY-MM-DD
    created_at  DATETIME NOT NULL,
    updated_at  DATETIME NOT NULL
);

CREATE INDEX idx_todos_user_id ON todos (user_id);

-- +goose Down
DROP INDEX idx_todos_user_id;
DROP TABLE todos;
