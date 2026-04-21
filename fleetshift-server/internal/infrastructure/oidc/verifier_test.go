package oidc_test

import (
	"context"
	"testing"
	"time"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/infrastructure/oidc"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/infrastructure/oidc/oidctest"
)

func TestVerifier_ValidToken(t *testing.T) {
	ctx := context.Background()
	idp := oidctest.Start(t, oidctest.WithAudience("test-audience"))

	verifier, err := oidc.NewVerifier(ctx, oidc.WithHTTPClient(idp.HTTPClient()))
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	rawToken := idp.IssueToken(t, oidctest.TokenClaims{Subject: "user-123"})

	claims, err := verifier.Verify(ctx, idp.OIDCConfig(), rawToken)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if claims.Subject != "user-123" {
		t.Errorf("Subject: got %q, want %q", claims.Subject, "user-123")
	}
	if claims.Issuer != idp.IssuerURL() {
		t.Errorf("Issuer: got %q, want %q", claims.Issuer, idp.IssuerURL())
	}
}

func TestVerifier_ExpiredToken(t *testing.T) {
	ctx := context.Background()
	idp := oidctest.Start(t, oidctest.WithAudience("test-audience"))

	verifier, err := oidc.NewVerifier(ctx, oidc.WithHTTPClient(idp.HTTPClient()))
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	rawToken := idp.IssueToken(t, oidctest.TokenClaims{
		Subject: "user-123",
		Expiry:  -time.Hour,
	})

	_, err = verifier.Verify(ctx, idp.OIDCConfig(), rawToken)
	if err == nil {
		t.Fatal("Verify: expected error for expired token, got nil")
	}
}

func TestVerifier_WrongIssuer(t *testing.T) {
	ctx := context.Background()
	idp := oidctest.Start(t, oidctest.WithAudience("test-audience"))

	verifier, err := oidc.NewVerifier(ctx, oidc.WithHTTPClient(idp.HTTPClient()))
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	// Issue a token, then verify against a config with a different issuer.
	rawToken := idp.IssueToken(t, oidctest.TokenClaims{Subject: "user-123"})

	config := idp.OIDCConfig()
	config.IssuerURL = "https://wrong-issuer"

	_, err = verifier.Verify(ctx, config, rawToken)
	if err == nil {
		t.Fatal("Verify: expected error for wrong issuer, got nil")
	}
}

func TestVerifier_WrongAudience(t *testing.T) {
	ctx := context.Background()
	idp := oidctest.Start(t, oidctest.WithAudience("test-audience"))

	verifier, err := oidc.NewVerifier(ctx, oidc.WithHTTPClient(idp.HTTPClient()))
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	rawToken := idp.IssueToken(t, oidctest.TokenClaims{Subject: "user-123"})

	config := domain.OIDCConfig{
		IssuerURL: idp.IssuerURL(),
		Audience:  "wrong-audience",
		JWKSURI:   idp.OIDCConfig().JWKSURI,
	}

	_, err = verifier.Verify(ctx, config, rawToken)
	if err == nil {
		t.Fatal("Verify: expected error for wrong audience, got nil")
	}
}

func TestVerifier_ContainerHostRewrite(t *testing.T) {
	ctx := context.Background()
	idp := oidctest.Start(t, oidctest.WithAudience("test-audience"))

	// Build a verifier with ContainerHost that rewrites "localhost"
	// to the actual listen address (127.0.0.1).
	verifier, err := oidc.NewVerifier(ctx,
		oidc.WithHTTPClient(idp.HTTPClient()),
		oidc.WithContainerHost("127.0.0.1"),
	)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	// Register a JWKS URI using "localhost" — the rewrite should make
	// it reachable via 127.0.0.1.
	localhostJWKS := domain.EndpointURL("https://localhost:" + idp.Port() + "/jwks")
	if err := verifier.RegisterKeySet(ctx, localhostJWKS); err != nil {
		t.Fatalf("RegisterKeySet with localhost URI: %v", err)
	}

	// Verify a token using a config with the localhost JWKS URI.
	// The verifier should look up the rewritten cache key internally.
	rawToken := idp.IssueToken(t, oidctest.TokenClaims{Subject: "user-rewrite"})
	config := idp.OIDCConfig()
	config.JWKSURI = localhostJWKS

	claims, err := verifier.Verify(ctx, config, rawToken)
	if err != nil {
		t.Fatalf("Verify with rewritten JWKS: %v", err)
	}
	if claims.Subject != "user-rewrite" {
		t.Errorf("Subject = %q, want %q", claims.Subject, "user-rewrite")
	}
}
