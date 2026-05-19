package gcphcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// HypershiftWorkspace holds the tempdir-backed assets required by a hypershift subprocess.
type HypershiftWorkspace struct {
	Env      []string
	JWKSPath string
	tempDir  string
}

// Cleanup removes the temporary workspace directory and every credential file in it.
func (w *HypershiftWorkspace) Cleanup() error {
	if w == nil || w.tempDir == "" {
		return nil
	}
	tempDir := w.tempDir
	w.tempDir = ""
	w.Env = nil
	w.JWKSPath = ""
	return os.RemoveAll(tempDir)
}

// PrepareCreateHypershiftWorkspace builds a tempdir-backed workspace for the
// create-path hypershift calls, including the public JWKS payload.
func PrepareCreateHypershiftWorkspace(
	callerToken string,
	target TargetConfig,
	jwksJSON []byte,
) (_ *HypershiftWorkspace, retErr error) {
	return prepareHypershiftWorkspace(callerToken, target, jwksJSON)
}

// PrepareDestroyHypershiftWorkspace builds a tempdir-backed workspace for the
// destroy-path hypershift calls.
func PrepareDestroyHypershiftWorkspace(
	callerToken string,
	target TargetConfig,
) (_ *HypershiftWorkspace, retErr error) {
	return prepareHypershiftWorkspace(callerToken, target, nil)
}

func prepareHypershiftWorkspace(
	callerToken string,
	target TargetConfig,
	jwksJSON []byte,
) (_ *HypershiftWorkspace, retErr error) {
	tempDir, err := os.MkdirTemp("", "gcphcp-hypershift-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}

	workspace := &HypershiftWorkspace{tempDir: tempDir}
	defer func() {
		if retErr == nil {
			return
		}
		if cleanupErr := workspace.Cleanup(); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
			retErr = errors.Join(retErr, fmt.Errorf("cleanup hypershift workspace: %w", cleanupErr))
		}
	}()

	subjectTokenPath := filepath.Join(tempDir, "subject_token.txt")
	if err := os.WriteFile(subjectTokenPath, []byte(callerToken), 0600); err != nil {
		return nil, fmt.Errorf("write subject token: %w", err)
	}

	credConfigJSON, err := buildWorkforceCredentialConfig(target, subjectTokenPath)
	if err != nil {
		return nil, fmt.Errorf("build credential config: %w", err)
	}

	credConfigPath := filepath.Join(tempDir, "workforce-cred.json")
	if err := os.WriteFile(credConfigPath, credConfigJSON, 0600); err != nil {
		return nil, fmt.Errorf("write credential config: %w", err)
	}

	homeDir := filepath.Join(tempDir, "home")
	cloudSDKDir := filepath.Join(tempDir, "cloudsdk")
	xdgConfigDir := filepath.Join(tempDir, "xdg")
	for _, dir := range []string{homeDir, cloudSDKDir, xdgConfigDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	workspace.Env = buildHypershiftEnvironment(credConfigPath, homeDir, cloudSDKDir, xdgConfigDir)

	if len(jwksJSON) == 0 {
		return workspace, nil
	}

	jwksPath := filepath.Join(tempDir, "jwks.json")
	if err := os.WriteFile(jwksPath, jwksJSON, 0600); err != nil {
		return nil, fmt.Errorf("write JWKS file: %w", err)
	}
	workspace.JWKSPath = jwksPath
	return workspace, nil
}

func buildWorkforceCredentialConfig(target TargetConfig, subjectTokenPath string) ([]byte, error) {
	audience := fmt.Sprintf("//iam.googleapis.com/locations/global/workforcePools/%s/providers/%s",
		target.WorkforcePool, target.WorkforceProvider)

	credConfig := map[string]any{
		"type":               "external_account",
		"audience":           audience,
		"subject_token_type": "urn:ietf:params:oauth:token-type:jwt",
		"token_url":          "https://sts.googleapis.com/v1/token",
		"credential_source": map[string]any{
			"file": subjectTokenPath,
		},
		"workforce_pool_user_project": target.GCPProject,
	}

	credConfigJSON, err := json.MarshalIndent(credConfig, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal credential config: %w", err)
	}
	return credConfigJSON, nil
}

func buildHypershiftEnvironment(
	credentialConfigPath, homeDir, cloudSDKDir, xdgConfigDir string,
) []string {
	env := []string{
		fmt.Sprintf("GOOGLE_APPLICATION_CREDENTIALS=%s", credentialConfigPath),
		fmt.Sprintf("HOME=%s", homeDir),
		fmt.Sprintf("CLOUDSDK_CONFIG=%s", cloudSDKDir),
		fmt.Sprintf("XDG_CONFIG_HOME=%s", xdgConfigDir),
	}

	for _, key := range []string{"PATH", "USER", "LOGNAME"} {
		if val := os.Getenv(key); val != "" {
			env = append(env, fmt.Sprintf("%s=%s", key, val))
		}
	}

	return env
}
