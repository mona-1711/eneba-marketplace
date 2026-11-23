package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Session-cookie auth: an opaque random token in an HttpOnly cookie, stored server-side.
// Chosen over JWT because logout/revocation is trivial and the API and SPA share one origin.

const sessionCookie = "session"
const sessionTTL = 30 * 24 * time.Hour

type User struct {
	ID    int    `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type ctxKey struct{}

func newToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (a *App) startSession(w http.ResponseWriter, userID int) error {
	tok := newToken()
	exp := time.Now().Add(sessionTTL)
	if _, err := a.db.Exec(`INSERT INTO sessions(token,user_id,expires_at) VALUES(?,?,?)`, tok, userID, exp.UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: tok, Path: "/", Expires: exp,
		HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func (a *App) currentUser(r *http.Request) *User {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return nil
	}
	var u User
	var exp string
	err = a.db.QueryRow(`SELECT u.id, u.email, u.name, s.expires_at FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.token = ?`, c.Value).
		Scan(&u.ID, &u.Email, &u.Name, &exp)
	if err != nil {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, exp); err != nil || time.Now().After(t) {
		return nil
	}
	return &u
}

func (a *App) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := a.currentUser(r)
		if u == nil {
			writeErr(w, http.StatusUnauthorized, "sign in required")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	}
}

func userFrom(r *http.Request) *User { return r.Context().Value(ctxKey{}).(*User) }

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

func (a *App) handleRegister(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := decodeJSON(r, &c); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	c.Email = strings.ToLower(strings.TrimSpace(c.Email))
	c.Name = strings.TrimSpace(c.Name)
	if _, err := mail.ParseAddress(c.Email); err != nil || !strings.Contains(c.Email, ".") {
		writeErr(w, 400, "Please enter a valid email address.")
		return
	}
	if len(c.Password) < 8 {
		writeErr(w, 400, "Password must be at least 8 characters.")
		return
	}
	if c.Name == "" {
		c.Name = strings.Split(c.Email, "@")[0]
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(c.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, 500, "could not hash password")
		return
	}
	res, err := a.db.Exec(`INSERT INTO users(email,name,password_hash) VALUES(?,?,?)`, c.Email, c.Name, string(hash))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeErr(w, 409, "An account with this email already exists.")
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	id, _ := res.LastInsertId()
	if err := a.startSession(w, int(id)); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, User{ID: int(id), Email: c.Email, Name: c.Name})
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := decodeJSON(r, &c); err != nil {
		writeErr(w, 400, "invalid body")
		return
	}
	var u User
	var hash string
	err := a.db.QueryRow(`SELECT id, email, name, password_hash FROM users WHERE email = ?`, strings.ToLower(strings.TrimSpace(c.Email))).
		Scan(&u.ID, &u.Email, &u.Name, &hash)
	if err == sql.ErrNoRows || (err == nil && bcrypt.CompareHashAndPassword([]byte(hash), []byte(c.Password)) != nil) {
		writeErr(w, 401, "Wrong email or password.")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := a.startSession(w, u.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, u)
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		a.db.Exec(`DELETE FROM sessions WHERE token = ?`, c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode})
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	if u == nil {
		writeJSON(w, 200, map[string]any{"user": nil})
		return
	}
	writeJSON(w, 200, map[string]any{"user": u})
}
