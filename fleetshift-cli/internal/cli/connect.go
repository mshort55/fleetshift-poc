package cli

import (
	"context"
	"fmt"

	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/fleetshift/fleetshift-poc/fleetshift-cli/internal/auth"
)

func dial(ctx context.Context, addr string) (*grpc.ClientConn, error) {
	creds := &tokenCredentials{store: auth.KeyringTokenStore{}}
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(creds),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", addr, err)
	}
	return conn, nil
}

// tokenCredentials implements [credentials.PerRPCCredentials] by loading
// tokens from the token store and refreshing them if needed.
type tokenCredentials struct {
	store auth.TokenStore
}

func (t *tokenCredentials) GetRequestMetadata(ctx context.Context, _ ...string) (map[string]string, error) {
	cfg, err := auth.LoadConfig()
	if err != nil {
		return nil, nil
	}

	oauthCfg := &oauth2.Config{
		ClientID: cfg.ClientID,
		Endpoint: oauth2.Endpoint{
			AuthURL:   cfg.AuthorizationEndpoint,
			TokenURL:  cfg.TokenEndpoint,
			AuthStyle: oauth2.AuthStyleInParams,
		},
		Scopes: cfg.Scopes,
	}

	tokens, _, err := auth.RefreshIfNeeded(ctx, t.store, oauthCfg)
	if err != nil {
		return nil, nil
	}

	if tokens.AccessToken == "" {
		return nil, nil
	}

	return map[string]string{
		"authorization": "Bearer " + tokens.AccessToken,
	}, nil
}

func (t *tokenCredentials) RequireTransportSecurity() bool {
	return false
}
