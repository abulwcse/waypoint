package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const sessionCookieName = "wp_session"

// Session is the identity carried in the app's own signed session cookie,
// set once a Google ID token has been verified.
type Session struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
	Exp     int64  `json:"exp"`
}

// SetSessionCookie signs sess with secret and sets it as an HTTP-only cookie.
// secure should be true whenever the app is served over HTTPS (it's left off
// for plain-HTTP local development, where the Secure flag would otherwise
// make browsers silently drop the cookie).
func SetSessionCookie(w http.ResponseWriter, secret []byte, sess Session, ttl time.Duration, secure bool) error {
	sess.Exp = time.Now().Add(ttl).Unix()
	payload, err := json.Marshal(sess)
	if err != nil {
		return err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    encoded + "." + sign(secret, encoded),
		Path:     "/",
		Expires:  time.Unix(sess.Exp, 0),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// ReadSession validates and decodes the session cookie on r, if present.
func ReadSession(r *http.Request, secret []byte) (*Session, error) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil, err
	}
	encoded, sig, ok := strings.Cut(c.Value, ".")
	if !ok {
		return nil, fmt.Errorf("malformed session cookie")
	}
	if !hmac.Equal([]byte(sig), []byte(sign(secret, encoded))) {
		return nil, fmt.Errorf("invalid session signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("malformed session cookie: %w", err)
	}
	var sess Session
	if err := json.Unmarshal(payload, &sess); err != nil {
		return nil, fmt.Errorf("malformed session cookie: %w", err)
	}
	if time.Now().Unix() > sess.Exp {
		return nil, fmt.Errorf("session expired")
	}
	return &sess, nil
}

// ClearSessionCookie removes the session cookie (used on sign-out).
func ClearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func sign(secret []byte, data string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
