package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gotodo/internal/store"
)

const (
	maxTitleLen       = 200
	maxDescriptionLen = 5000
	dateLayout        = "2006-01-02"
)

// todoRequest is the body for creating (POST) and replacing (PUT) a todo.
type todoRequest struct {
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Status      store.Status `json:"status"`
	DueDate     *string      `json:"due_date"`
}

// toInput validates the request. When requireStatus is false an empty status
// defaults to not_started (POST); PUT replaces the whole todo and needs it.
func (req todoRequest) toInput(requireStatus bool) (store.TodoInput, string) {
	in := store.TodoInput{
		Title:       strings.TrimSpace(req.Title),
		Description: strings.TrimSpace(req.Description),
		Status:      req.Status,
	}

	switch {
	case in.Title == "":
		return in, "title is required"
	case len(in.Title) > maxTitleLen:
		return in, "title must be at most 200 characters"
	case len(in.Description) > maxDescriptionLen:
		return in, "description must be at most 5000 characters"
	}

	if in.Status == "" {
		if requireStatus {
			return in, "status is required"
		}
		in.Status = store.StatusNotStarted
	}
	if !in.Status.Valid() {
		return in, "status must be one of: not_started, in_progress, in_review, completed"
	}

	if req.DueDate != nil {
		d := strings.TrimSpace(*req.DueDate)
		if d != "" {
			if _, err := time.Parse(dateLayout, d); err != nil {
				return in, "due_date must be a date in YYYY-MM-DD format"
			}
			in.DueDate = &d
		}
	}
	return in, ""
}

func (a *API) listTodos(w http.ResponseWriter, r *http.Request) {
	var filter *store.Status
	if q := r.URL.Query().Get("status"); q != "" {
		s := store.Status(q)
		if !s.Valid() {
			writeError(w, http.StatusBadRequest, "status must be one of: not_started, in_progress, in_review, completed")
			return
		}
		filter = &s
	}
	todos, err := a.store.ListTodos(r.Context(), userIDFrom(r.Context()), filter)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, todos)
}

func (a *API) createTodo(w http.ResponseWriter, r *http.Request) {
	var req todoRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	in, msg := req.toInput(false)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	todo, err := a.store.CreateTodo(r.Context(), userIDFrom(r.Context()), in)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/todos/"+strconv.FormatInt(todo.ID, 10))
	writeJSON(w, http.StatusCreated, todo)
}

func (a *API) getTodo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "todo not found")
		return
	}
	todo, err := a.store.GetTodo(r.Context(), userIDFrom(r.Context()), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "todo not found")
		return
	}
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, todo)
}

func (a *API) updateTodo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "todo not found")
		return
	}
	var req todoRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	in, msg := req.toInput(true)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	todo, err := a.store.UpdateTodo(r.Context(), userIDFrom(r.Context()), id, in)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "todo not found")
		return
	}
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, todo)
}

func (a *API) deleteTodo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "todo not found")
		return
	}
	err := a.store.DeleteTodo(r.Context(), userIDFrom(r.Context()), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "todo not found")
		return
	}
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
