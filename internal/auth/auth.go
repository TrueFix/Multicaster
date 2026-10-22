package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/truefix/multicaster/internal/config"
	"github.com/truefix/multicaster/internal/engine"
	"golang.org/x/crypto/bcrypt"
)

var ErrLimited = errors.New("Please wait a moment before trying again")
var ErrInvalid = errors.New("Password or setup code was not accepted")

const cookieName = "multicaster_session"
const lifetime = 16 * time.Hour

type Manager struct {
	eng      *engine.Engine
	mu       sync.Mutex
	sessions map[[32]byte]time.Time
	busy     bool
	next     time.Time
}

func New(e *engine.Engine) *Manager { return &Manager{eng: e, sessions: map[[32]byte]time.Time{}} }
func (m *Manager) NeedsSetup() bool { return m.eng.Config().Auth.PasswordHash == "" }
func equal(a, b string) bool {
	return a != "" && b != "" && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
func (m *Manager) Authorized(r *http.Request) bool {
	a := m.eng.Config().Auth
	if equal(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), a.AdminToken) && strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		return true
	}
	if equal(r.Header.Get("X-Admin-Token"), a.AdminToken) {
		return true
	}
	c, err := r.Cookie(cookieName)
	if err != nil || len(c.Value) > 128 {
		return false
	}
	key := sha256.Sum256([]byte(c.Value))
	m.mu.Lock()
	defer m.mu.Unlock()
	until, ok := m.sessions[key]
	if !ok {
		return false
	}
	if time.Now().After(until) {
		delete(m.sessions, key)
		return false
	}
	return true
}
func (m *Manager) begin() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.busy || time.Now().Before(m.next) {
		return ErrLimited
	}
	m.busy = true
	m.next = time.Now().Add(time.Second)
	return nil
}
func (m *Manager) end() { m.mu.Lock(); m.busy = false; m.mu.Unlock() }
func (m *Manager) Login(password string) (string, error) {
	if err := m.begin(); err != nil {
		return "", err
	}
	defer m.end()
	a := m.eng.Config().Auth
	if len(password) > 72 || a.PasswordHash == "" || bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(password)) != nil {
		return "", ErrInvalid
	}
	return m.newSession()
}
func (m *Manager) Setup(code, password string) (string, error) {
	if err := m.begin(); err != nil {
		return "", err
	}
	defer m.end()
	a := m.eng.Config().Auth
	if a.PasswordHash != "" || (!equal(code, a.SetupToken) && !equal(code, a.AdminToken)) {
		return "", ErrInvalid
	}
	if utf8.RuneCountInString(password) < 8 || len(password) > 72 {
		return "", errors.New("Use at least 8 characters and no more than 72 UTF-8 bytes")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	a.PasswordHash = string(hash)
	a.SetupToken = ""
	if err = m.eng.UpdateAuth(a); err != nil {
		return "", err
	}
	return m.newSession()
}
func (m *Manager) newSession() (string, error) {
	token, err := config.NewSecret()
	if err != nil {
		return "", err
	}
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, until := range m.sessions {
		if now.After(until) {
			delete(m.sessions, key)
		}
	}
	if len(m.sessions) >= 128 {
		for key := range m.sessions {
			delete(m.sessions, key)
			break
		}
	}
	m.sessions[sha256.Sum256([]byte(token))] = now.Add(lifetime)
	return token, nil
}
func (m *Manager) Reset() (string, error) {
	m.mu.Lock()
	if m.busy {
		m.mu.Unlock()
		return "", ErrLimited
	}
	m.busy = true
	m.mu.Unlock()
	defer m.end()
	a := m.eng.Config().Auth
	token, err := config.NewSecret()
	if err != nil {
		return "", err
	}
	a.PasswordHash = ""
	a.SetupToken = token
	if err = m.eng.UpdateAuth(a); err != nil {
		return "", err
	}
	m.mu.Lock()
	clear(m.sessions)
	m.next = time.Time{}
	m.mu.Unlock()
	return token, nil
}
func (m *Manager) Logout(r *http.Request) {
	if c, e := r.Cookie(cookieName); e == nil {
		m.mu.Lock()
		delete(m.sessions, sha256.Sum256([]byte(c.Value)))
		m.mu.Unlock()
	}
}
func SetCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: int(lifetime.Seconds())})
}
func ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
}
