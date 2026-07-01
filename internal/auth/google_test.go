package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// signTestToken builds an RS256 JWT signed with priv, standing in for a
// Google-issued ID token so Verify's claim/signature checks can be exercised
// without a real Google account.
func signTestToken(t *testing.T, priv *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	header := map[string]string{"alg": "RS256", "kid": kid, "typ": "JWT"}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		t.Fatal(err)
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	signingInput := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
	hashed := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, hashed[:])
	if err != nil {
		t.Fatal(err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func cloneClaims(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func TestGoogleVerifierVerify(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	v := NewGoogleVerifier("test-client-id")
	v.fetchKey = func(ctx context.Context, kid string) (*rsa.PublicKey, error) {
		return &priv.PublicKey, nil
	}

	validClaims := map[string]any{
		"iss":            "accounts.google.com",
		"aud":            "test-client-id",
		"exp":            time.Now().Add(time.Hour).Unix(),
		"email":          "user@example.com",
		"email_verified": true,
		"name":           "Test User",
		"picture":        "https://example.com/pic.jpg",
	}

	t.Run("valid token", func(t *testing.T) {
		token := signTestToken(t, priv, "kid1", validClaims)
		user, err := v.Verify(context.Background(), token)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if user.Email != "user@example.com" || user.Name != "Test User" {
			t.Errorf("unexpected user: %+v", user)
		}
	})

	t.Run("wrong audience is rejected", func(t *testing.T) {
		claims := cloneClaims(validClaims)
		claims["aud"] = "someone-elses-app"
		token := signTestToken(t, priv, "kid1", claims)
		if _, err := v.Verify(context.Background(), token); err == nil {
			t.Fatal("expected an error for a token issued for a different app")
		}
	})

	t.Run("expired token is rejected", func(t *testing.T) {
		claims := cloneClaims(validClaims)
		claims["exp"] = time.Now().Add(-time.Hour).Unix()
		token := signTestToken(t, priv, "kid1", claims)
		if _, err := v.Verify(context.Background(), token); err == nil {
			t.Fatal("expected an error for an expired token")
		}
	})

	t.Run("unverified email is rejected", func(t *testing.T) {
		claims := cloneClaims(validClaims)
		claims["email_verified"] = false
		token := signTestToken(t, priv, "kid1", claims)
		if _, err := v.Verify(context.Background(), token); err == nil {
			t.Fatal("expected an error for an unverified email")
		}
	})

	t.Run("wrong issuer is rejected", func(t *testing.T) {
		claims := cloneClaims(validClaims)
		claims["iss"] = "https://evil.example.com"
		token := signTestToken(t, priv, "kid1", claims)
		if _, err := v.Verify(context.Background(), token); err == nil {
			t.Fatal("expected an error for a wrong issuer")
		}
	})

	t.Run("tampered payload fails signature check", func(t *testing.T) {
		token := signTestToken(t, priv, "kid1", validClaims)
		parts := strings.Split(token, ".")
		tampered := cloneClaims(validClaims)
		tampered["email"] = "attacker@example.com"
		tamperedJSON, err := json.Marshal(tampered)
		if err != nil {
			t.Fatal(err)
		}
		parts[1] = base64.RawURLEncoding.EncodeToString(tamperedJSON)
		if _, err := v.Verify(context.Background(), strings.Join(parts, ".")); err == nil {
			t.Fatal("expected a signature error for a tampered payload")
		}
	})
}
