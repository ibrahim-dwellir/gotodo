package api

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"sync"
	"time"

	"gotodo/internal/auth"
	"gotodo/internal/store"
)

const (
	minPasswordLen = 8
	maxPasswordLen = 72 // bcrypt only uses the first 72 bytes
	maxNameLen     = 100
)

type registerRequest struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Password  string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type updateMeRequest struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type authResponse struct {
	Token     string      `json:"token"`
	ExpiresAt time.Time   `json:"expires_at"`
	User      *store.User `json:"user"`
}

var dummyHash = sync.OnceValue(func() string {
	h, err := auth.HashPassword("not-a-real-password")
	if err != nil {
		panic(err)
	}
	return h
})

func normalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func validateName(field, v string) string {
	switch {
	case v == "":
		return field + " is required"
	case len(v) > maxNameLen:
		return field + " must be at most 100 characters"
	}
	return ""
}

func validateEmail(email string) string {
	if email == "" {
		return "email is required"
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return "email is not a valid address"
	}
	return ""
}

func validatePassword(p string) string {
	switch {
	case len(p) < minPasswordLen:
		return "password must be at least 8 characters"
	case len(p) > maxPasswordLen:
		return "password must be at most 72 bytes"
	}
	return ""
}

func firstProblem(problems ...string) string {
	for _, p := range problems {
		if p != "" {
			return p
		}
	}
	return ""
}

func (a *API) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.FirstName = strings.TrimSpace(req.FirstName)
	req.LastName = strings.TrimSpace(req.LastName)
	req.Email = normalizeEmail(req.Email)

	if msg := firstProblem(
		validateName("first_name", req.FirstName),
		validateName("last_name", req.LastName),
		validateEmail(req.Email),
		validatePassword(req.Password),
	); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	user, err := a.store.CreateUser(r.Context(), req.FirstName, req.LastName, req.Email, hash)
	if errors.Is(err, store.ErrEmailTaken) {
		a.metrics.registrations.WithLabelValues("email_taken").Inc()
		writeError(w, http.StatusConflict, "email is already registered")
		return
	}
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	a.metrics.registrations.WithLabelValues("success").Inc()
	writeJSON(w, http.StatusCreated, user)
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Email = normalizeEmail(req.Email)
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	user, err := a.store.GetUserByEmail(r.Context(), req.Email)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		writeInternal(w, r, err)
		return
	}
	if user == nil {
		// Spend the same bcrypt time as a real check so response timing
		// does not reveal which emails are registered.
		auth.CheckPassword(dummyHash(), req.Password)
		a.metrics.logins.WithLabelValues("invalid_credentials").Inc()
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	if !auth.CheckPassword(user.PasswordHash, req.Password) {
		a.metrics.logins.WithLabelValues("invalid_credentials").Inc()
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	user, err = a.store.RecordLogin(r.Context(), user.ID)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	token, exp, err := a.tokens.Issue(user.ID)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	a.metrics.logins.WithLabelValues("success").Inc()
	writeJSON(w, http.StatusOK, authResponse{Token: token, ExpiresAt: exp.UTC(), User: user})
}

func (a *API) getMe(w http.ResponseWriter, r *http.Request) {
	user, err := a.store.GetUserByID(r.Context(), userIDFrom(r.Context()))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "user no longer exists")
		return
	}
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (a *API) updateMe(w http.ResponseWriter, r *http.Request) {
	var req updateMeRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.FirstName = strings.TrimSpace(req.FirstName)
	req.LastName = strings.TrimSpace(req.LastName)
	if msg := firstProblem(
		validateName("first_name", req.FirstName),
		validateName("last_name", req.LastName),
	); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	user, err := a.store.UpdateUserName(r.Context(), userIDFrom(r.Context()), req.FirstName, req.LastName)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "user no longer exists")
		return
	}
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}
