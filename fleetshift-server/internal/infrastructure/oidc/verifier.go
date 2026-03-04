package oidc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lestrrat-go/httprc/v3"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

// Verifier implements [domain.OIDCTokenVerifier] using lestrrat-go/jwx.
// It manages a [jwk.Cache] internally for JWKS auto-refresh.
type Verifier struct {
	cache *jwk.Cache

	mu      sync.RWMutex
	keySets map[string]jwk.Set // jwksURI -> cached set
}

// NewVerifier creates a verifier with a background JWKS cache.
func NewVerifier(ctx context.Context) (*Verifier, error) {
	client := httprc.NewClient()
	cache, err := jwk.NewCache(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("create JWK cache: %w", err)
	}
	return &Verifier{
		cache:   cache,
		keySets: make(map[string]jwk.Set),
	}, nil
}

// RegisterKeySet registers a JWKS URI with the background cache so keys
// are refreshed automatically. Call this on startup for persisted auth
// methods and after creating new ones.
func (v *Verifier) RegisterKeySet(ctx context.Context, jwksURI string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if _, ok := v.keySets[jwksURI]; ok {
		return nil
	}

	if err := v.cache.Register(ctx, jwksURI); err != nil {
		return fmt.Errorf("register JWKS URI %s: %w", jwksURI, err)
	}
	cached, err := v.cache.CachedSet(jwksURI)
	if err != nil {
		return fmt.Errorf("create cached set for %s: %w", jwksURI, err)
	}
	v.keySets[jwksURI] = cached
	return nil
}

// Verify validates a JWT against the OIDC configuration.
func (v *Verifier) Verify(ctx context.Context, config domain.OIDCConfig, rawToken string) (domain.SubjectClaims, error) {
	keySet, err := v.getKeySet(ctx, config.JWKSURI)
	if err != nil {
		return domain.SubjectClaims{}, fmt.Errorf("get key set: %w", err)
	}

	parseOpts := []jwt.ParseOption{
		jwt.WithKeySet(keySet),
		jwt.WithValidate(true),
		jwt.WithAcceptableSkew(30 * time.Second),
	}
	if config.IssuerURL != "" {
		parseOpts = append(parseOpts, jwt.WithIssuer(config.IssuerURL))
	}
	if config.Audience != "" {
		parseOpts = append(parseOpts, jwt.WithAudience(config.Audience))
	}

	tok, err := jwt.ParseString(rawToken, parseOpts...)
	if err != nil {
		return domain.SubjectClaims{}, fmt.Errorf("parse/verify token: %w", err)
	}

	sub, _ := tok.Subject()
	iss, _ := tok.Issuer()

	claims := domain.SubjectClaims{
		ID:     domain.SubjectID(sub),
		Issuer: iss,
		Extra:  make(map[string][]string),
	}

	var email string
	if err := tok.Get("email", &email); err == nil {
		claims.Extra["email"] = []string{email}
	}

	var groups []string
	if err := tok.Get("groups", &groups); err == nil {
		claims.Extra["groups"] = groups
	}

	var azp string
	if err := tok.Get("azp", &azp); err == nil {
		claims.Extra["azp"] = []string{azp}
	}

	return claims, nil
}

func (v *Verifier) getKeySet(ctx context.Context, jwksURI string) (jwk.Set, error) {
	v.mu.RLock()
	ks, ok := v.keySets[jwksURI]
	v.mu.RUnlock()
	if ok {
		return ks, nil
	}

	if err := v.RegisterKeySet(ctx, jwksURI); err != nil {
		return nil, err
	}

	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.keySets[jwksURI], nil
}
