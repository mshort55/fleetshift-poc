package ocp

import (
	"context"
	"io"
	"net/http"
	"strings"
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

// roundTripFunc adapts a function into an http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestSSOProvider_ResolvePullSecret_ValidResponse(t *testing.T) {
	provider := &SSOCredentialProvider{
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				// Verify auth header was set
				if got := req.Header.Get("Authorization"); got != "Bearer test-token" {
					t.Errorf("Authorization = %q, want %q", got, "Bearer test-token")
				}
				body := `{"auths":{"registry.example.com":{"auth":"dGVzdA=="}}}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			}),
		},
	}

	result, err := provider.ResolvePullSecret(context.Background(), PullSecretRequest{
		Auth: domain.DeliveryAuth{Token: "test-token"},
	})
	if err != nil {
		t.Fatalf("ResolvePullSecret: %v", err)
	}
	if !strings.Contains(string(result), `"auths"`) {
		t.Errorf("result missing auths field: %s", result)
	}
}

func TestSSOProvider_ResolvePullSecret_MalformedJSON(t *testing.T) {
	provider := &SSOCredentialProvider{
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`not json`)),
				}, nil
			}),
		},
	}

	_, err := provider.ResolvePullSecret(context.Background(), PullSecretRequest{
		Auth: domain.DeliveryAuth{Token: "test-token"},
	})
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestSSOProvider_ResolvePullSecret_MissingAuths(t *testing.T) {
	provider := &SSOCredentialProvider{
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"valid":"json"}`)),
				}, nil
			}),
		},
	}

	_, err := provider.ResolvePullSecret(context.Background(), PullSecretRequest{
		Auth: domain.DeliveryAuth{Token: "test-token"},
	})
	if err == nil {
		t.Fatal("expected error for missing auths field, got nil")
	}
}

func TestSSOProvider_ResolvePullSecret_HTTPError(t *testing.T) {
	provider := &SSOCredentialProvider{
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusForbidden,
					Body:       io.NopCloser(strings.NewReader(`{"error":"forbidden"}`)),
				}, nil
			}),
		},
	}

	_, err := provider.ResolvePullSecret(context.Background(), PullSecretRequest{
		Auth: domain.DeliveryAuth{Token: "test-token"},
	})
	if err == nil {
		t.Fatal("expected error for HTTP 403, got nil")
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
