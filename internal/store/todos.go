package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Status is the workflow state of a todo.
type Status string

const (
	StatusNotStarted Status = "not_started"
	StatusInProgress Status = "in_progress"
	StatusInReview   Status = "in_review"
	StatusCompleted  Status = "completed"
)

// Statuses lists every valid status in workflow order.
var Statuses = []Status{StatusNotStarted, StatusInProgress, StatusInReview, StatusCompleted}

// Valid reports whether s is one of the known statuses.
func (s Status) Valid() bool {
	for _, v := range Statuses {
		if s == v {
			return true
		}
	}
	return false
}

// Todo is a task owned by a single user.
type Todo struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"user_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      Status    `json:"status"`
	DueDate     *string   `json:"due_date"` // YYYY-MM-DD or null
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TodoInput holds the writable fields of a todo.
type TodoInput struct {
	Title       string
	Description string
	Status      Status
	DueDate     *string
}

const todoColumns = `id, user_id, title, description, status, due_date, created_at, updated_at`

func scanTodo(row rowScanner) (*Todo, error) {
	var (
		t   Todo
		due sql.NullString
	)
	err := row.Scan(&t.ID, &t.UserID, &t.Title, &t.Description, &t.Status, &due, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if due.Valid {
		d := due.String
		t.DueDate = &d
	}
	return &t, nil
}

// ListTodos returns the user's todos, newest first, optionally filtered by status.
func (s *Store) ListTodos(ctx context.Context, userID int64, status *Status) ([]Todo, error) {
	query := `SELECT ` + todoColumns + ` FROM todos WHERE user_id = ?`
	args := []any{userID}
	if status != nil {
		query += ` AND status = ?`
		args = append(args, *status)
	}
	query += ` ORDER BY created_at DESC, id DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list todos: %w", err)
	}
	defer rows.Close()

	todos := []Todo{}
	for rows.Next() {
		t, err := scanTodo(rows)
		if err != nil {
			return nil, fmt.Errorf("scan todo: %w", err)
		}
		todos = append(todos, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate todos: %w", err)
	}
	return todos, nil
}

// GetTodo returns the todo if it exists and belongs to userID.
func (s *Store) GetTodo(ctx context.Context, userID, id int64) (*Todo, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+todoColumns+` FROM todos WHERE id = ? AND user_id = ?`, id, userID)
	return scanTodo(row)
}

// CreateTodo inserts a todo owned by userID.
func (s *Store) CreateTodo(ctx context.Context, userID int64, in TodoInput) (*Todo, error) {
	now := s.now()
	row := s.db.QueryRowContext(ctx,
		`INSERT INTO todos (user_id, title, description, status, due_date, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 RETURNING `+todoColumns,
		userID, in.Title, in.Description, in.Status, in.DueDate, now, now)
	t, err := scanTodo(row)
	if err != nil {
		return nil, fmt.Errorf("create todo: %w", err)
	}
	return t, nil
}

// UpdateTodo replaces the writable fields of a todo owned by userID.
func (s *Store) UpdateTodo(ctx context.Context, userID, id int64, in TodoInput) (*Todo, error) {
	row := s.db.QueryRowContext(ctx,
		`UPDATE todos
		 SET title = ?, description = ?, status = ?, due_date = ?, updated_at = ?
		 WHERE id = ? AND user_id = ?
		 RETURNING `+todoColumns,
		in.Title, in.Description, in.Status, in.DueDate, s.now(), id, userID)
	return scanTodo(row)
}

// DeleteTodo removes a todo owned by userID.
func (s *Store) DeleteTodo(ctx context.Context, userID, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM todos WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("delete todo: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete todo: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
