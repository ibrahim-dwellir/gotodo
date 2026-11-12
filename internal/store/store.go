// Package store implements persistence for users and todos on top of SQLite.
package store

import (
	"database/sql"
	"errors"
	"time"
)

var (
	// ErrNotFound is returned when a row does not exist or is not owned by the caller.
	ErrNotFound = errors.New("not found")
	// ErrEmailTaken is returned when registering an email that already exists.
	ErrEmailTaken = errors.New("email already registered")
)

// Store wraps the database handle.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// New returns a Store using the given database.
func New(db *sql.DB) *Store {
	return &Store{db: db, now: func() time.Time { return time.Now().UTC() }}
}

// rowScanner is satisfied by *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}
