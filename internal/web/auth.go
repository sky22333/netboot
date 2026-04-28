package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/argon2"
)

const (
	loginMaxFailures = 10
	loginWindow      = 10 * time.Minute
	loginLockout     = 10 * time.Minute
	loginMaxEntries  = 2048
)

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9._@-]{3,32}$`)

type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]string
}

type LoginLimiter struct {
	mu       sync.Mutex
	attempts map[string]loginAttempt
}

type loginAttempt struct {
	Failures    int
	LockedUntil time.Time
	UpdatedAt   time.Time
}

func NewSessionManager() *SessionManager {
	return &SessionManager{sessions: map[string]string{}}
}

func NewLoginLimiter() *LoginLimiter {
	return &LoginLimiter{attempts: map[string]loginAttempt{}}
}

func (l *LoginLimiter) Allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pruneLocked(now)
	item := l.attempts[key]
	if now.After(item.LockedUntil) && now.Sub(item.UpdatedAt) > loginWindow {
		delete(l.attempts, key)
		return true
	}
	return now.After(item.LockedUntil)
}

func (l *LoginLimiter) Fail(key string) {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	item := l.attempts[key]
	if now.Sub(item.UpdatedAt) > loginWindow {
		item.Failures = 0
	}
	item.Failures++
	item.UpdatedAt = now
	if item.Failures >= loginMaxFailures {
		item.LockedUntil = now.Add(loginLockout)
	}
	l.attempts[key] = item
	l.pruneLocked(now)
}

func (l *LoginLimiter) Success(key string) {
	l.mu.Lock()
	delete(l.attempts, key)
	l.mu.Unlock()
}

func (l *LoginLimiter) pruneLocked(now time.Time) {
	if len(l.attempts) <= loginMaxEntries {
		return
	}
	for key, item := range l.attempts {
		if now.After(item.LockedUntil) && now.Sub(item.UpdatedAt) > loginWindow {
			delete(l.attempts, key)
		}
	}
}

func (s *SessionManager) Create(username string) string {
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	token := base64.RawURLEncoding.EncodeToString(buf)
	s.mu.Lock()
	s.sessions[token] = username
	s.mu.Unlock()
	return token
}

func (s *SessionManager) Valid(token string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[token] != ""
}

func (h *Handler) requireAuth(c *gin.Context) {
	settings, _ := h.app.Storage().GetSettings(c.Request.Context())
	if !settings.Security.AdminAuthEnabled {
		c.Next()
		return
	}
	token, err := c.Cookie("pxe_session")
	if err == nil && h.sessions.Valid(token) {
		c.Next()
		return
	}
	Fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "请先登录")
	c.Abort()
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	return fmt.Sprintf("argon2id$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

func validateUsername(username string) error {
	if !usernamePattern.MatchString(username) {
		return fmt.Errorf("用户名需为 3-32 位，只能包含字母、数字、点、下划线、短横线或 @")
	}
	return nil
}

func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 3 || parts[0] != "argon2id" {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[1])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[2])
	if err1 != nil || err2 != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}

func (h *Handler) hasUsers(ctx context.Context) bool {
	var count int
	_ = h.app.Storage().RawDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE enabled=1`).Scan(&count)
	return count > 0
}

func (h *Handler) createUser(ctx context.Context, username, password string) error {
	username = strings.TrimSpace(username)
	if err := validateUsername(username); err != nil {
		return err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = h.app.Storage().RawDB().ExecContext(ctx, `INSERT INTO users(username,password_hash,role,enabled,created_at,updated_at) VALUES(?,?,?,?,?,?)`, username, hash, "admin", 1, now, now)
	return err
}

func (h *Handler) createUserWithRole(ctx context.Context, username, password, role string) error {
	username = strings.TrimSpace(username)
	if err := validateUsername(username); err != nil {
		return err
	}
	if role == "" {
		role = "admin"
	}
	if role != "admin" {
		return fmt.Errorf("不支持的用户角色")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = h.app.Storage().RawDB().ExecContext(ctx, `INSERT INTO users(username,password_hash,role,enabled,created_at,updated_at) VALUES(?,?,?,?,?,?)`, username, hash, role, 1, now, now)
	return err
}

func (h *Handler) changePassword(ctx context.Context, id int64, password string) error {
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	_, err = h.app.Storage().RawDB().ExecContext(ctx, `UPDATE users SET password_hash=?,updated_at=? WHERE id=?`, hash, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

func (h *Handler) checkLogin(ctx context.Context, username, password string) bool {
	var hash string
	err := h.app.Storage().RawDB().QueryRowContext(ctx, `SELECT password_hash FROM users WHERE username=? AND enabled=1`, username).Scan(&hash)
	if err == sql.ErrNoRows || err != nil {
		return false
	}
	return verifyPassword(hash, password)
}
