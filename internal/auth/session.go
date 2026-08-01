package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const (
	SessionCookieName = "kinops_admin_session"
	SessionTTL        = 12 * time.Hour
)

type session struct {
	expiresAt time.Time
	csrfToken string
}

type Manager struct {
	username string
	password passwordVerifier
	secure   bool
	now      func() time.Time

	mu       sync.Mutex
	sessions map[[sha256.Size]byte]session
}

func NewManager(username, passwordHash string, secure bool) (*Manager, error) {
	return newManager(username, passwordHash, secure, time.Now)
}

func newManager(username, passwordHash string, secure bool, now func() time.Time) (*Manager, error) {
	if username == "" {
		return nil, errors.New("admin username is required")
	}
	password, err := parsePasswordHash(passwordHash)
	if err != nil {
		return nil, err
	}
	return &Manager{username: username, password: password, secure: secure, now: now, sessions: make(map[[sha256.Size]byte]session)}, nil
}

func (m *Manager) Authenticate(username, password string) bool {
	wantUsername := sha256.Sum256([]byte(m.username))
	gotUsername := sha256.Sum256([]byte(username))
	usernameOK := subtle.ConstantTimeCompare(gotUsername[:], wantUsername[:]) == 1
	passwordOK := m.password.Verify(password)
	return usernameOK && passwordOK
}

func (m *Manager) CreateSession(w http.ResponseWriter) (string, error) {
	token, err := randomToken(32)
	if err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	csrfToken, err := randomToken(32)
	if err != nil {
		return "", fmt.Errorf("generate CSRF token: %w", err)
	}
	now := m.now()
	m.mu.Lock()
	m.removeExpiredLocked(now)
	m.sessions[sha256.Sum256([]byte(token))] = session{expiresAt: now.Add(SessionTTL), csrfToken: csrfToken}
	m.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookieName, Value: token, Path: "/admin", HttpOnly: true,
		Secure: m.secure, SameSite: http.SameSiteStrictMode, MaxAge: int(SessionTTL.Seconds()),
		Expires: now.Add(SessionTTL),
	})
	return csrfToken, nil
}

func (m *Manager) DestroySession(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(SessionCookieName); err == nil {
		m.mu.Lock()
		delete(m.sessions, sha256.Sum256([]byte(cookie.Value)))
		m.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookieName, Value: "", Path: "/admin", HttpOnly: true,
		Secure: m.secure, SameSite: http.SameSiteStrictMode, MaxAge: -1,
		Expires: time.Unix(1, 0),
	})
}

func (m *Manager) CSRFToken(r *http.Request) (string, bool) {
	value, ok := m.lookup(r)
	return value.csrfToken, ok
}

func (m *Manager) VerifyCSRF(r *http.Request, submitted string) bool {
	value, ok := m.lookup(r)
	if !ok || submitted == "" {
		return false
	}
	want := sha256.Sum256([]byte(value.csrfToken))
	got := sha256.Sum256([]byte(submitted))
	return subtle.ConstantTimeCompare(got[:], want[:]) == 1
}

func (m *Manager) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if _, ok := m.lookup(r); !ok {
			http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (m *Manager) lookup(r *http.Request) (session, bool) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		return session{}, false
	}
	now := m.now()
	key := sha256.Sum256([]byte(cookie.Value))
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.sessions[key]
	if !ok {
		return session{}, false
	}
	if !value.expiresAt.After(now) {
		delete(m.sessions, key)
		return session{}, false
	}
	return value, true
}

func (m *Manager) removeExpiredLocked(now time.Time) {
	for key, value := range m.sessions {
		if !value.expiresAt.After(now) {
			delete(m.sessions, key)
		}
	}
}

func randomToken(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
