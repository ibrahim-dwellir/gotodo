-- +goose Up
CREATE TABLE users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    first_name    TEXT     NOT NULL,
    last_name     TEXT     NOT NULL,
    email         TEXT     NOT NULL UNIQUE COLLATE NOCASE,
    password_hash TEXT     NOT NULL,
    last_login_at DATETIME,
    created_at    DATETIME NOT NULL,
    updated_at    DATETIME NOT NULL
);

-- +goose Down
DROP TABLE users;
