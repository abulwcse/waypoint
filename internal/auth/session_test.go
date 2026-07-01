package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionCookieRoundTrip(t *testing.T) {
	secret := []byte("test-secret")
	want := Session{Email: "user@example.com", Name: "Test User", Picture: "https://example.com/pic.jpg"}

	rec := httptest.NewRecorder()
	if err := SetSessionCookie(rec, secret, want, time.Hour, false); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}

	got, err := ReadSession(req, secret)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Email != want.Email || got.Name != want.Name || got.Picture != want.Picture {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestReadSessionRejectsTamperedCookie(t *testing.T) {
	secret := []byte("test-secret")
	rec := httptest.NewRecorder()
	if err := SetSessionCookie(rec, secret, Session{Email: "user@example.com"}, time.Hour, false); err != nil {
		t.Fatal(err)
	}

	cookies := rec.Result().Cookies()
	cookies[0].Value = cookies[0].Value + "tampered"

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookies[0])

	if _, err := ReadSession(req, secret); err == nil {
		t.Fatal("expected an error for a tampered session cookie")
	}
}

func TestReadSessionRejectsWrongSecret(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := SetSessionCookie(rec, []byte("secret-a"), Session{Email: "user@example.com"}, time.Hour, false); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}

	if _, err := ReadSession(req, []byte("secret-b")); err == nil {
		t.Fatal("expected an error when verifying with a different secret")
	}
}

func TestReadSessionRejectsExpired(t *testing.T) {
	secret := []byte("test-secret")
	rec := httptest.NewRecorder()
	if err := SetSessionCookie(rec, secret, Session{Email: "user@example.com"}, -time.Hour, false); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}

	if _, err := ReadSession(req, secret); err == nil {
		t.Fatal("expected an error for an expired session")
	}
}
