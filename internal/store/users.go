package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// User is a registered account. PasswordHash is never serialised.
type User struct {
	ID           int64      `json:"id"`
	FirstName    string     `json:"first_name"`
	LastName     string     `json:"last_name"`
	Email        string     `json:"email"`
	PasswordHash string     `json:"-"`
	LastLoginAt  *time.Time `json:"last_login_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

const userColumns = `id, first_name, last_name, email, password_hash, last_login_at, created_at, updated_at`

func scanUser(row rowScanner) (*User, error) {
	var (
		u         User
		lastLogin sql.NullTime
	)
	err := row.Scan(&u.ID, &u.FirstName, &u.LastName, &u.Email, &u.PasswordHash, &lastLogin, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if lastLogin.Valid {
		t := lastLogin.Time
		u.LastLoginAt = &t
	}
	return &u, nil
}

// CreateUser inserts a new user. It returns ErrEmailTaken if the email is in use.
func (s *Store) CreateUser(ctx context.Context, firstName, lastName, email, passwordHash string) (*User, error) {
	now := s.now()
	row := s.db.QueryRowContext(ctx,
		`INSERT INTO users (first_name, last_name, email, password_hash, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 RETURNING `+userColumns,
		firstName, lastName, email, passwordHash, now, now)
	u, err := scanUser(row)
	if err != nil {
		var se *sqlite.Error
		if errors.As(err, &se) && se.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
			return nil, ErrEmailTaken
		}
		return nil, fmt.Errorf("create user: %w", err)
	}
	return u, nil
}

// GetUserByID returns the user with the given id.
func (s *Store) GetUserByID(ctx context.Context, id int64) (*User, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id)
	return scanUser(row)
}

// GetUserByEmail returns the user with the given email (case-insensitive).
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE email = ?`, email)
	return scanUser(row)
}

// RecordLogin stamps last_login_at for the user and returns the updated user.
// A login is not a change to the user's profile, so updated_at is left alone.
func (s *Store) RecordLogin(ctx context.Context, id int64) (*User, error) {
	row := s.db.QueryRowContext(ctx,
		`UPDATE users SET last_login_at = ? WHERE id = ? RETURNING `+userColumns,
		s.now(), id)
	return scanUser(row)
}

// UpdateUserName changes the user's first and last name and bumps updated_at.
func (s *Store) UpdateUserName(ctx context.Context, id int64, firstName, lastName string) (*User, error) {
	row := s.db.QueryRowContext(ctx,
		`UPDATE users SET first_name = ?, last_name = ?, updated_at = ? WHERE id = ? RETURNING `+userColumns,
		firstName, lastName, s.now(), id)
	return scanUser(row)
}
