package ocp

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

func TestSSOProvider_ResolveAWS_MissingToken(t *testing.T) {
	provider := &SSOCredentialProvider{}

	req := AWSCredentialRequest{
		Region:  "us-east-1",
		RoleARN: "arn:aws:iam::123456789012:role/test-role",
		Auth: domain.DeliveryAuth{
			Token: "", // empty token
		},
	}

	_, err := provider.ResolveAWS(context.Background(), req)
	if err == nil {
		t.Fatal("expected error when token is empty, got nil")
	}

	expectedMsg := "auth token is required"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message %q, got %q", expectedMsg, err.Error())
	}
}

func TestSSOProvider_ResolveAWS_MissingRoleARN(t *testing.T) {
	provider := &SSOCredentialProvider{}

	req := AWSCredentialRequest{
		Region:  "us-east-1",
		RoleARN: "", // empty role ARN
		Auth: domain.DeliveryAuth{
			Token: "test-token",
		},
	}

	_, err := provider.ResolveAWS(context.Background(), req)
	if err == nil {
		t.Fatal("expected error when role ARN is empty, got nil")
	}

	expectedMsg := "role ARN is required"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message %q, got %q", expectedMsg, err.Error())
	}
}

func TestSSOProvider_ResolvePullSecret_MissingToken(t *testing.T) {
	provider := &SSOCredentialProvider{}

	req := PullSecretRequest{
		Auth: domain.DeliveryAuth{
			Token: "", // empty token
		},
	}

	_, err := provider.ResolvePullSecret(context.Background(), req)
	if err == nil {
		t.Fatal("expected error when token is empty, got nil")
	}

	expectedMsg := "auth token is required"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message %q, got %q", expectedMsg, err.Error())
	}
}

func TestSSOProvider_DefaultHTTPClientTimeout(t *testing.T) {
	provider := &SSOCredentialProvider{}
	client := provider.httpClient()
	if client.Timeout != 30*time.Second {
		t.Errorf("default timeout = %v, want 30s", client.Timeout)
	}
}

func TestSSOProvider_CustomHTTPClient(t *testing.T) {
	custom := &http.Client{Timeout: 5 * time.Second}
	provider := &SSOCredentialProvider{HTTPClient: custom}
	client := provider.httpClient()
	if client != custom {
		t.Error("expected custom client to be returned")
	}
}
