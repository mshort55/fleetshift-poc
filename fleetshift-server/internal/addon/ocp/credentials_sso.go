package ocp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

const (
	defaultProvisionSTSDuration = 2 * time.Hour
	defaultDestroySTSDuration   = 1 * time.Hour
	rhSSOTokenEndpoint          = "https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/token"
	rhSSODeviceEndpoint         = "https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/auth/device"
	rhPullSecretEndpoint        = "https://api.openshift.com/api/accounts_mgmt/v1/access_token"
	rhSSOClientID               = "ocm-cli"
)

// SSOCredentialProvider exchanges caller OIDC tokens for temporary AWS
// credentials via AssumeRoleWithWebIdentity and acquires pull secrets via
// Red Hat SSO device code flow.
type SSOCredentialProvider struct {
	STSDuration time.Duration // override default STS session duration (0 = use default)
	HTTPClient  *http.Client  // override for testing
}

// DeviceCodeResponse represents the response from Red Hat SSO device code endpoint.
type DeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// ResolveAWS exchanges the caller's OIDC token for temporary AWS credentials
// via STS AssumeRoleWithWebIdentity.
func (p *SSOCredentialProvider) ResolveAWS(ctx context.Context, req AWSCredentialRequest) (*AWSCredentials, error) {
	if req.Auth.Token == "" {
		return nil, fmt.Errorf("auth token is required")
	}
	if req.RoleARN == "" {
		return nil, fmt.Errorf("role ARN is required")
	}

	// Determine session duration
	duration := p.STSDuration
	if duration == 0 {
		duration = defaultProvisionSTSDuration
	}

	// Load AWS config for the specified region
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(req.Region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create STS client
	stsClient := sts.NewFromConfig(cfg)

	// Assume role with web identity
	result, err := stsClient.AssumeRoleWithWebIdentity(ctx, &sts.AssumeRoleWithWebIdentityInput{
		RoleArn:          aws.String(req.RoleARN),
		RoleSessionName:  aws.String(fmt.Sprintf("fleetshift-provision-%d", time.Now().Unix())),
		WebIdentityToken: aws.String(string(req.Auth.Token)),
		DurationSeconds:  aws.Int32(int32(duration.Seconds())),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to assume role with web identity: %w", err)
	}

	if result.Credentials == nil {
		return nil, fmt.Errorf("STS returned nil credentials")
	}

	return &AWSCredentials{
		AccessKeyID:     aws.ToString(result.Credentials.AccessKeyId),
		SecretAccessKey: aws.ToString(result.Credentials.SecretAccessKey),
		SessionToken:    aws.ToString(result.Credentials.SessionToken),
	}, nil
}

// ResolvePullSecret acquires an OpenShift pull secret from Red Hat API
// using a bearer token.
func (p *SSOCredentialProvider) ResolvePullSecret(ctx context.Context, req PullSecretRequest) ([]byte, error) {
	if req.Auth.Token == "" {
		return nil, fmt.Errorf("auth token is required")
	}

	client := p.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	// Create POST request with empty JSON body
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, rhPullSecretEndpoint, strings.NewReader("{}"))
	if err != nil {
		return nil, fmt.Errorf("failed to create pull secret request: %w", err)
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", req.Auth.Token))
	httpReq.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pull secret: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read pull secret response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pull secret request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Validate response has auths field
	var pullSecret map[string]interface{}
	if err := json.Unmarshal(body, &pullSecret); err != nil {
		return nil, fmt.Errorf("failed to parse pull secret response: %w", err)
	}

	if _, ok := pullSecret["auths"]; !ok {
		return nil, fmt.Errorf("pull secret response missing 'auths' field")
	}

	return body, nil
}

// InitiateDeviceCodeFlow starts the Red Hat SSO device code flow for CLI authentication.
func (p *SSOCredentialProvider) InitiateDeviceCodeFlow(ctx context.Context) (*DeviceCodeResponse, error) {
	client := p.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	// Prepare form data
	data := url.Values{}
	data.Set("client_id", rhSSOClientID)
	data.Set("scope", "openid")

	// Create POST request
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, rhSSODeviceEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create device code request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Execute request
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to initiate device code flow: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read device code response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device code request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var deviceResp DeviceCodeResponse
	if err := json.Unmarshal(body, &deviceResp); err != nil {
		return nil, fmt.Errorf("failed to parse device code response: %w", err)
	}

	return &deviceResp, nil
}

// PollForToken polls the Red Hat SSO token endpoint until the user completes
// authentication or the device code expires.
func (p *SSOCredentialProvider) PollForToken(ctx context.Context, deviceCode string, interval int) (string, error) {
	client := p.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("polling cancelled: %w", ctx.Err())
		case <-ticker.C:
			// Prepare form data
			data := url.Values{}
			data.Set("client_id", rhSSOClientID)
			data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
			data.Set("device_code", deviceCode)

			// Create POST request
			httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, rhSSOTokenEndpoint, strings.NewReader(data.Encode()))
			if err != nil {
				return "", fmt.Errorf("failed to create token request: %w", err)
			}

			httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			// Execute request
			resp, err := client.Do(httpReq)
			if err != nil {
				return "", fmt.Errorf("failed to poll for token: %w", err)
			}

			// Read response body
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				return "", fmt.Errorf("failed to read token response: %w", err)
			}

			// Parse response
			var tokenResp map[string]interface{}
			if err := json.Unmarshal(body, &tokenResp); err != nil {
				return "", fmt.Errorf("failed to parse token response: %w", err)
			}

			// Check for error
			if errCode, ok := tokenResp["error"].(string); ok {
				switch errCode {
				case "authorization_pending":
					// User hasn't completed auth yet, continue polling
					continue
				case "slow_down":
					// Increase polling interval as requested by server
					ticker.Reset(time.Duration(interval+5) * time.Second)
					continue
				default:
					// Other errors are terminal
					errDesc := tokenResp["error_description"]
					return "", fmt.Errorf("token request failed: %s - %v", errCode, errDesc)
				}
			}

			// Success - extract access token
			if accessToken, ok := tokenResp["access_token"].(string); ok {
				return accessToken, nil
			}

			return "", fmt.Errorf("token response missing access_token field")
		}
	}
}
