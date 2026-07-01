// Package auth verifies "Sign in with Google" ID tokens and issues the app's
// own session cookie, so the rest of the server never has to talk to Google
// directly. There's no client secret involved: the frontend uses Google
// Identity Services to get a signed ID token straight from Google, and this
// package checks that signature itself (against Google's public keys) rather
// than pulling in the full google.golang.org/api client.
package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// GoogleUser is the identity extracted from a verified Google ID token.
type GoogleUser struct {
	Email   string
	Name    string
	Picture string
}

// GoogleVerifier checks Google-issued ID tokens (RS256-signed JWTs) against
// Google's published public keys.
type GoogleVerifier struct {
	http     *http.Client
	certsURL string
	clientID string

	// fetchKey is overridable in tests so they can verify tokens signed with a
	// locally generated key instead of needing a real Google-signed token.
	fetchKey func(ctx context.Context, kid string) (*rsa.PublicKey, error)

	mu      sync.Mutex
	keys    map[string]*rsa.PublicKey
	fetched time.Time
}

// NewGoogleVerifier builds a verifier that only accepts tokens issued for the
// given OAuth 2.0 client ID (the app's GOOGLE_CLIENT_ID).
func NewGoogleVerifier(clientID string) *GoogleVerifier {
	v := &GoogleVerifier{
		http:     &http.Client{Timeout: 10 * time.Second},
		certsURL: envOr("GOOGLE_CERTS_URL", "https://www.googleapis.com/oauth2/v3/certs"),
		clientID: clientID,
		keys:     map[string]*rsa.PublicKey{},
	}
	v.fetchKey = v.fetchFromGoogle
	return v
}

// Verify checks idToken's signature, issuer, audience, expiry, and that the
// email is verified, returning the signed-in user's identity.
func (v *GoogleVerifier) Verify(ctx context.Context, idToken string) (*GoogleUser, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("malformed ID token")
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("malformed ID token header: %w", err)
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("malformed ID token payload: %w", err)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("malformed ID token signature: %w", err)
	}

	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, fmt.Errorf("malformed ID token header: %w", err)
	}
	if header.Alg != "RS256" {
		return nil, fmt.Errorf("unsupported ID token algorithm %q", header.Alg)
	}

	var claims struct {
		Iss           string `json:"iss"`
		Aud           string `json:"aud"`
		Exp           int64  `json:"exp"`
		Email         string `json:"email"`
		EmailVerified any    `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, fmt.Errorf("malformed ID token claims: %w", err)
	}
	if claims.Iss != "accounts.google.com" && claims.Iss != "https://accounts.google.com" {
		return nil, fmt.Errorf("unexpected ID token issuer %q", claims.Iss)
	}
	if claims.Aud != v.clientID {
		return nil, fmt.Errorf("ID token audience does not match this app")
	}
	if time.Now().Unix() > claims.Exp {
		return nil, fmt.Errorf("ID token expired")
	}
	if !truthy(claims.EmailVerified) {
		return nil, fmt.Errorf("Google account email is not verified")
	}

	key, err := v.fetchKey(ctx, header.Kid)
	if err != nil {
		return nil, fmt.Errorf("fetch Google signing key: %w", err)
	}
	hashed := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, hashed[:], sig); err != nil {
		return nil, fmt.Errorf("invalid ID token signature")
	}

	return &GoogleUser{Email: claims.Email, Name: claims.Name, Picture: claims.Picture}, nil
}

// fetchFromGoogle returns the public key for kid, fetching and caching
// Google's published JWK set (https://www.googleapis.com/oauth2/v3/certs) as
// needed. Google rotates these keys periodically, so a cache miss triggers a
// refetch rather than failing outright.
func (v *GoogleVerifier) fetchFromGoogle(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	key, ok := v.keys[kid]
	stale := time.Since(v.fetched) > time.Hour
	v.mu.Unlock()
	if ok && !stale {
		return key, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.certsURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := v.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var jwks struct {
		Keys []struct {
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, err
	}

	keys := make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, k := range jwks.Keys {
		pub, err := rsaPublicKey(k.N, k.E)
		if err != nil {
			continue
		}
		keys[k.Kid] = pub
	}

	v.mu.Lock()
	v.keys = keys
	v.fetched = time.Now()
	v.mu.Unlock()

	if pub, ok := keys[kid]; ok {
		return pub, nil
	}
	return nil, fmt.Errorf("no signing key found for kid %q", kid)
}

// rsaPublicKey decodes a JWK's base64url-encoded modulus (n) and exponent (e)
// into an *rsa.PublicKey.
func rsaPublicKey(n, e string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(n)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(e)
	if err != nil {
		return nil, err
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: int(new(big.Int).SetBytes(eBytes).Int64()),
	}, nil
}

// truthy handles email_verified arriving as either a JSON bool or string,
// which Google's tokens have done at different times.
func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true"
	default:
		return false
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
