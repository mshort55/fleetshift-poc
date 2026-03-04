package auth

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "fleetctl"
	keyringUser    = "tokens"
)

// KeyringTokenStore persists tokens in the OS secure keychain.
type KeyringTokenStore struct{}

func (KeyringTokenStore) Save(_ context.Context, tokens Tokens) error {
	data, err := json.Marshal(tokens)
	if err != nil {
		return fmt.Errorf("marshal tokens: %w", err)
	}
	if err := keyring.Set(keyringService, keyringUser, string(data)); err != nil {
		return fmt.Errorf("save to keyring: %w", err)
	}
	return nil
}

func (KeyringTokenStore) Load(_ context.Context) (Tokens, error) {
	data, err := keyring.Get(keyringService, keyringUser)
	if err != nil {
		return Tokens{}, fmt.Errorf("load from keyring: %w", err)
	}
	var tokens Tokens
	if err := json.Unmarshal([]byte(data), &tokens); err != nil {
		return Tokens{}, fmt.Errorf("parse tokens: %w", err)
	}
	return tokens, nil
}

func (KeyringTokenStore) Clear(_ context.Context) error {
	if err := keyring.Delete(keyringService, keyringUser); err != nil {
		return fmt.Errorf("clear keyring: %w", err)
	}
	return nil
}
