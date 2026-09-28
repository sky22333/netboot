package web

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
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

type LoginLimiter struct {
	mu       sync.Mutex
	attempts map[string]loginAttempt
}

type loginAttempt struct {
	Failures    int
	LockedUntil time.Time
	UpdatedAt   time.Time
}

func NewLoginLimiter() *LoginLimiter {
	return &LoginLimiter{attempts: map[string]loginAttempt{}}
}

func (l *LoginLimiter) Allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pruneLocked(now)
	item, known := l.attempts[key]
	if !known && len(l.attempts) >= loginMaxEntries {
		return false
	}
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
	item, known := l.attempts[key]
	if !known && len(l.attempts) >= loginMaxEntries {
		return
	}
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
	for key, item := range l.attempts {
		if now.After(item.LockedUntil) && now.Sub(item.UpdatedAt) > loginWindow {
			delete(l.attempts, key)
		}
	}
}

const sessionLifetime = 24 * time.Hour

func sessionHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (h *Handler) sessionValid(ctx context.Context, token string) bool {
	if len(token) != 43 {
		return false
	}
	var id int64
	err := h.app.Storage().RawDB().QueryRowContext(ctx, `SELECT u.id FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND s.expires>unixepoch() AND u.enabled=1`, sessionHash(token)).Scan(&id)
	return err == nil
}
func (h *Handler) requireAuth(c *gin.Context) {
	token, _ := c.Cookie("pxe_session")
	if h.sessionValid(c.Request.Context(), token) {
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

func (h *Handler) hasUsers(ctx context.Context) (bool, error) {
	var count int
	err := h.app.Storage().RawDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	return count > 0, err
}

func (h *Handler) createUser(ctx context.Context, username, password string, initial bool) error {
	username = strings.TrimSpace(username)
	if err := validateUsername(username); err != nil {
		return err
	}
	if len(password) < 8 || len(password) > 1024 {
		return fmt.Errorf("密码长度需为 8-1024 字节")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	query := `INSERT INTO users(username,password_hash,role,enabled,created_at,updated_at) SELECT ?,?,'admin',1,?,?`
	if initial {
		query += ` WHERE NOT EXISTS(SELECT 1 FROM users)`
	}
	res, err := h.app.Storage().RawDB().ExecContext(ctx, query, username, hash, time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("初始化已经完成")
	}
	return nil
}

func (h *Handler) changePassword(ctx context.Context, id int64, password string) error {
	if len(password) < 8 || len(password) > 1024 {
		return fmt.Errorf("密码长度需为 8-1024 字节")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	tx, err := h.app.Storage().RawDB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE users SET password_hash=?,updated_at=? WHERE id=?`, hash, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("用户不存在")
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (h *Handler) loginSession(ctx context.Context, username, password string) (string, error) {
	var id int64
	var hash string
	if len(password) > 1024 {
		return "", fmt.Errorf("用户名或密码错误")
	}
	err := h.app.Storage().RawDB().QueryRowContext(ctx, `SELECT id,password_hash FROM users WHERE username=? AND enabled=1`, username).Scan(&id, &hash)
	if err != nil || !verifyPassword(hash, password) {
		return "", fmt.Errorf("用户名或密码错误")
	}
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	tx, err := h.app.Storage().RawDB().BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM sessions WHERE expires<=unixepoch()`); err != nil {
		return "", err
	}
	// The hash predicate prevents an in-flight login from surviving a password reset.
	res, err := tx.ExecContext(ctx, `INSERT INTO sessions(token_hash,user_id,expires) SELECT ?,id,? FROM users WHERE id=? AND password_hash=? AND enabled=1`, sessionHash(token), time.Now().Add(sessionLifetime).Unix(), id, hash)
	if err != nil {
		return "", err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return "", fmt.Errorf("账号已更改，请重新登录")
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=? AND token_hash NOT IN (SELECT token_hash FROM sessions WHERE user_id=? ORDER BY expires DESC LIMIT 20)`, id, id); err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return token, nil
}

func (h *Handler) limitAuthWork(c *gin.Context) {
	select {
	case h.authSlots <- struct{}{}:
		defer func() { <-h.authSlots }()
		c.Next()
	default:
		Fail(c, 429, "AUTH_BUSY", "登录请求繁忙，请稍后重试")
		c.Abort()
	}
}
