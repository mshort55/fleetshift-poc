package application

import (
	"context"
	"fmt"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

// AuthMethodService manages authentication methods at runtime.
type AuthMethodService struct {
	Methods   domain.AuthMethodRepository
	Discovery domain.OIDCDiscoveryClient
}

// Create validates the auth method, resolves OIDC discovery metadata,
// and persists it.
func (s *AuthMethodService) Create(ctx context.Context, id domain.AuthMethodID, method domain.AuthMethod) (domain.AuthMethod, error) {
	if id == "" {
		return domain.AuthMethod{}, fmt.Errorf("%w: auth method ID is required", domain.ErrInvalidArgument)
	}

	method.ID = id
	if err := method.Validate(); err != nil {
		return domain.AuthMethod{}, fmt.Errorf("%w: %v", domain.ErrInvalidArgument, err)
	}

	switch method.Type {
	case domain.AuthMethodTypeOIDC:
		if method.OIDC.IssuerURL == "" {
			return domain.AuthMethod{}, fmt.Errorf("%w: issuer_url is required", domain.ErrInvalidArgument)
		}
		meta, err := s.Discovery.FetchMetadata(ctx, method.OIDC.IssuerURL)
		if err != nil {
			return domain.AuthMethod{}, fmt.Errorf("fetch OIDC discovery: %w", err)
		}
		method.OIDC.JWKSURI = meta.JWKSURI
		method.OIDC.AuthorizationEndpoint = meta.AuthorizationEndpoint
		method.OIDC.TokenEndpoint = meta.TokenEndpoint
	}

	if err := s.Methods.Save(ctx, method); err != nil {
		return domain.AuthMethod{}, fmt.Errorf("save auth method: %w", err)
	}
	return method, nil
}

// Get retrieves an auth method by ID.
func (s *AuthMethodService) Get(ctx context.Context, id domain.AuthMethodID) (domain.AuthMethod, error) {
	return s.Methods.Get(ctx, id)
}

// List returns all configured auth methods.
func (s *AuthMethodService) List(ctx context.Context) ([]domain.AuthMethod, error) {
	return s.Methods.List(ctx)
}
