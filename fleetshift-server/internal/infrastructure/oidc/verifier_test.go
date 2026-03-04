package oidc_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/infrastructure/oidc"
)

// setupTestJWKSServer creates an RSA key pair, serves the public JWKS from an HTTP server,
// and returns the server, JWKS URI, and private key for signing. Caller must defer server.Close().
func setupTestJWKSServer(t *testing.T) (*httptest.Server, string, *rsa.PrivateKey) {
	t.Helper()

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}

	pubKey, err := jwk.Import(privKey.PublicKey)
	if err != nil {
		t.Fatalf("import public key: %v", err)
	}
	if err := pubKey.Set(jwk.KeyIDKey, "test-kid"); err != nil {
		t.Fatalf("set key id: %v", err)
	}
	if err := pubKey.Set(jwk.AlgorithmKey, jwa.RS256()); err != nil {
		t.Fatalf("set algorithm: %v", err)
	}

	set := jwk.NewSet()
	set.AddKey(pubKey)

	jwksJSON, err := json.Marshal(set)
	if err != nil {
		t.Fatalf("marshal JWKS: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwksJSON)
	})

	server := httptest.NewServer(mux)
	jwksURI := server.URL + "/jwks.json"
	return server, jwksURI, privKey
}

func signToken(t *testing.T, privKey *rsa.PrivateKey, issuer, audience, sub string, exp time.Time) string {
	t.Helper()

	token, err := jwt.NewBuilder().
		Subject(sub).
		Issuer(issuer).
		Audience([]string{audience}).
		IssuedAt(time.Now()).
		Expiration(exp).
		Build()
	if err != nil {
		t.Fatalf("build token: %v", err)
	}

	jwkPriv, err := jwk.Import(privKey)
	if err != nil {
		t.Fatalf("import private key: %v", err)
	}
	if err := jwkPriv.Set(jwk.KeyIDKey, "test-kid"); err != nil {
		t.Fatalf("set key id: %v", err)
	}

	signed, err := jwt.Sign(token, jwt.WithKey(jwa.RS256(), jwkPriv))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return string(signed)
}

func TestVerifier_ValidToken(t *testing.T) {
	ctx := context.Background()

	server, jwksURI, privKey := setupTestJWKSServer(t)
	defer server.Close()

	verifier, err := oidc.NewVerifier(ctx)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	if err := verifier.RegisterKeySet(ctx, jwksURI); err != nil {
		t.Fatalf("RegisterKeySet: %v", err)
	}

	issuer := "https://test-issuer"
	audience := "test-audience"
	sub := "user-123"
	rawToken := signToken(t, privKey, issuer, audience, sub, time.Now().Add(time.Hour))

	config := domain.OIDCConfig{
		IssuerURL: issuer,
		Audience:  audience,
		JWKSURI:   jwksURI,
	}

	claims, err := verifier.Verify(ctx, config, rawToken)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if claims.ID != domain.SubjectID(sub) {
		t.Errorf("ID: got %q, want %q", claims.ID, sub)
	}
	if claims.Issuer != issuer {
		t.Errorf("Issuer: got %q, want %q", claims.Issuer, issuer)
	}
}

func TestVerifier_ExpiredToken(t *testing.T) {
	ctx := context.Background()

	server, jwksURI, privKey := setupTestJWKSServer(t)
	defer server.Close()

	verifier, err := oidc.NewVerifier(ctx)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	if err := verifier.RegisterKeySet(ctx, jwksURI); err != nil {
		t.Fatalf("RegisterKeySet: %v", err)
	}

	issuer := "https://test-issuer"
	audience := "test-audience"
	sub := "user-123"
	rawToken := signToken(t, privKey, issuer, audience, sub, time.Now().Add(-time.Hour))

	config := domain.OIDCConfig{
		IssuerURL: issuer,
		Audience:  audience,
		JWKSURI:   jwksURI,
	}

	_, err = verifier.Verify(ctx, config, rawToken)
	if err == nil {
		t.Fatal("Verify: expected error for expired token, got nil")
	}
}

func TestVerifier_WrongIssuer(t *testing.T) {
	ctx := context.Background()

	server, jwksURI, privKey := setupTestJWKSServer(t)
	defer server.Close()

	verifier, err := oidc.NewVerifier(ctx)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	if err := verifier.RegisterKeySet(ctx, jwksURI); err != nil {
		t.Fatalf("RegisterKeySet: %v", err)
	}

	issuer := "https://test-issuer"
	audience := "test-audience"
	sub := "user-123"
	rawToken := signToken(t, privKey, "https://wrong-issuer", audience, sub, time.Now().Add(time.Hour))

	config := domain.OIDCConfig{
		IssuerURL: issuer,
		Audience:  audience,
		JWKSURI:   jwksURI,
	}

	_, err = verifier.Verify(ctx, config, rawToken)
	if err == nil {
		t.Fatal("Verify: expected error for wrong issuer, got nil")
	}
}

func TestVerifier_WrongAudience(t *testing.T) {
	ctx := context.Background()

	server, jwksURI, privKey := setupTestJWKSServer(t)
	defer server.Close()

	verifier, err := oidc.NewVerifier(ctx)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	if err := verifier.RegisterKeySet(ctx, jwksURI); err != nil {
		t.Fatalf("RegisterKeySet: %v", err)
	}

	issuer := "https://test-issuer"
	audience := "test-audience"
	sub := "user-123"
	rawToken := signToken(t, privKey, issuer, "wrong-audience", sub, time.Now().Add(time.Hour))

	config := domain.OIDCConfig{
		IssuerURL: issuer,
		Audience:  audience,
		JWKSURI:   jwksURI,
	}

	_, err = verifier.Verify(ctx, config, rawToken)
	if err == nil {
		t.Fatal("Verify: expected error for wrong audience, got nil")
	}
}
