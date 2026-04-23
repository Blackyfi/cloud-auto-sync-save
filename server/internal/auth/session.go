package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	sessionCookieName = "cass_session"
	sessionTTL        = 24 * time.Hour
)

type SessionStore struct {
	secret []byte
}

func NewSessionStore(dataDir string) (*SessionStore, error) {
	keyPath := filepath.Join(dataDir, "session.key")
	if b, err := os.ReadFile(keyPath); err == nil && len(b) == 32 {
		return &SessionStore{secret: b}, nil
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, secret, 0o600); err != nil {
		return nil, err
	}
	return &SessionStore{secret: secret}, nil
}

type Session struct {
	UserID  int64
	Expires time.Time
}

func (s *SessionStore) sign(userID int64, ttl time.Duration) string {
	expires := time.Now().Add(ttl).Unix()
	payload := fmt.Sprintf("%d|%d", userID, expires)
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString([]byte(payload + "|" + sig))
}

func (s *SessionStore) verify(token string) (*Session, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 3 {
		return nil, errors.New("malformed session")
	}
	payload := parts[0] + "|" + parts[1]
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payload))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(parts[2])) {
		return nil, errors.New("bad signature")
	}
	uid, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return nil, err
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, err
	}
	if time.Now().Unix() > exp {
		return nil, errors.New("session expired")
	}
	return &Session{UserID: uid, Expires: time.Unix(exp, 0)}, nil
}

func (s *SessionStore) SetCookie(w http.ResponseWriter, userID int64) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    s.sign(userID, sessionTTL),
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(sessionTTL),
	})
}

func (s *SessionStore) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (s *SessionStore) Read(r *http.Request) (*Session, error) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil, err
	}
	return s.verify(c.Value)
}
