package ccoctl

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateAllArgs(t *testing.T) {
	got := CreateAllArgs("test-cluster", "us-east-1", "/path/to/credrequests", "/path/to/output")
	want := []string{
		"aws",
		"create-all",
		"--name", "test-cluster",
		"--region", "us-east-1",
		"--credentials-requests-dir", "/path/to/credrequests",
		"--output-dir", "/path/to/output",
	}

	if len(got) != len(want) {
		t.Fatalf("CreateAllArgs() length = %d, want %d", len(got), len(want))
	}

	for i, arg := range got {
		if arg != want[i] {
			t.Errorf("CreateAllArgs()[%d] = %q, want %q", i, arg, want[i])
		}
	}
}

func TestDeleteArgs(t *testing.T) {
	got := DeleteArgs("test-cluster", "us-east-1")
	want := []string{
		"aws",
		"delete",
		"--name", "test-cluster",
		"--region", "us-east-1",
	}

	if len(got) != len(want) {
		t.Fatalf("DeleteArgs() length = %d, want %d", len(got), len(want))
	}

	for i, arg := range got {
		if arg != want[i] {
			t.Errorf("DeleteArgs()[%d] = %q, want %q", i, arg, want[i])
		}
	}
}

func TestExtractBinaryArgs(t *testing.T) {
	got := ExtractBinaryArgs("/work/dir", "/path/to/pull-secret.json", "quay.io/openshift-release-dev/ocp-release:4.14.0")

	// Must contain these key arguments
	requiredArgs := []string{"--command=ccoctl", "--to", "/work/dir", "--registry-config", "/path/to/pull-secret.json"}

	for _, required := range requiredArgs {
		found := false
		for _, arg := range got {
			if arg == required {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ExtractBinaryArgs() missing required arg %q, got %v", required, got)
		}
	}

	// Should also contain the release image
	found := false
	for _, arg := range got {
		if arg == "quay.io/openshift-release-dev/ocp-release:4.14.0" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ExtractBinaryArgs() missing release image in args: %v", got)
	}
}

func TestExtractCredReqArgs(t *testing.T) {
	got := ExtractCredReqArgs("/path/to/credrequests", "/path/to/pull-secret.json", "quay.io/openshift-release-dev/ocp-release:4.14.0")

	// Must contain these key arguments
	requiredArgs := []string{"--credentials-requests", "--cloud=aws", "--to", "/path/to/credrequests", "--registry-config", "/path/to/pull-secret.json"}

	for _, required := range requiredArgs {
		found := false
		for _, arg := range got {
			if arg == required {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ExtractCredReqArgs() missing required arg %q, got %v", required, got)
		}
	}

	// Should also contain the release image
	found := false
	for _, arg := range got {
		if arg == "quay.io/openshift-release-dev/ocp-release:4.14.0" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ExtractCredReqArgs() missing release image in args: %v", got)
	}
}

func TestBinaryPath(t *testing.T) {
	got := BinaryPath("/work/dir")
	want := filepath.Join("/work/dir", "ccoctl")

	if got != want {
		t.Errorf("BinaryPath() = %q, want %q", got, want)
	}
}

func TestCredReqDir(t *testing.T) {
	got := CredReqDir("/work/dir")
	want := filepath.Join("/work/dir", "credrequests")

	if got != want {
		t.Errorf("CredReqDir() = %q, want %q", got, want)
	}
}

func TestOutputDir(t *testing.T) {
	got := OutputDir("/work/dir")
	want := filepath.Join("/work/dir", "ccoctl-output")

	if got != want {
		t.Errorf("OutputDir() = %q, want %q", got, want)
	}
}

func TestInjectManifests(t *testing.T) {
	// Create temporary directories
	tmpDir := t.TempDir()
	ccoctlOutputDir := filepath.Join(tmpDir, "ccoctl-output")
	installerDir := filepath.Join(tmpDir, "installer")

	// Create source structure with manifests and tls
	manifestsDir := filepath.Join(ccoctlOutputDir, "manifests")
	tlsDir := filepath.Join(ccoctlOutputDir, "tls")
	if err := os.MkdirAll(manifestsDir, 0755); err != nil {
		t.Fatalf("failed to create manifests dir: %v", err)
	}
	if err := os.MkdirAll(tlsDir, 0755); err != nil {
		t.Fatalf("failed to create tls dir: %v", err)
	}

	// Create test files
	manifestFile := filepath.Join(manifestsDir, "role.yaml")
	if err := os.WriteFile(manifestFile, []byte("apiVersion: v1\nkind: Role"), 0644); err != nil {
		t.Fatalf("failed to write manifest file: %v", err)
	}

	tlsFile := filepath.Join(tlsDir, "key.key")
	if err := os.WriteFile(tlsFile, []byte("-----BEGIN PRIVATE KEY-----"), 0644); err != nil {
		t.Fatalf("failed to write tls file: %v", err)
	}

	// Create installer directory
	if err := os.MkdirAll(installerDir, 0755); err != nil {
		t.Fatalf("failed to create installer dir: %v", err)
	}

	// Call InjectManifests
	if err := InjectManifests(ccoctlOutputDir, installerDir); err != nil {
		t.Fatalf("InjectManifests() error = %v", err)
	}

	// Verify files were copied
	destManifestFile := filepath.Join(installerDir, "manifests", "role.yaml")
	if _, err := os.Stat(destManifestFile); os.IsNotExist(err) {
		t.Errorf("manifest file was not copied to %q", destManifestFile)
	}

	destTlsFile := filepath.Join(installerDir, "tls", "key.key")
	if _, err := os.Stat(destTlsFile); os.IsNotExist(err) {
		t.Errorf("tls file was not copied to %q", destTlsFile)
	}

	// Verify content
	manifestContent, err := os.ReadFile(destManifestFile)
	if err != nil {
		t.Fatalf("failed to read copied manifest: %v", err)
	}
	if string(manifestContent) != "apiVersion: v1\nkind: Role" {
		t.Errorf("manifest content = %q, want %q", string(manifestContent), "apiVersion: v1\nkind: Role")
	}

	tlsContent, err := os.ReadFile(destTlsFile)
	if err != nil {
		t.Fatalf("failed to read copied tls file: %v", err)
	}
	if string(tlsContent) != "-----BEGIN PRIVATE KEY-----" {
		t.Errorf("tls content = %q, want %q", string(tlsContent), "-----BEGIN PRIVATE KEY-----")
	}
}

func TestInjectManifests_MissingSourceDir(t *testing.T) {
	tmpDir := t.TempDir()
	ccoctlOutputDir := filepath.Join(tmpDir, "nonexistent")
	installerDir := filepath.Join(tmpDir, "installer")

	// Create installer directory
	if err := os.MkdirAll(installerDir, 0755); err != nil {
		t.Fatalf("failed to create installer dir: %v", err)
	}

	// Should not error when source doesn't exist
	err := InjectManifests(ccoctlOutputDir, installerDir)
	if err != nil {
		t.Errorf("InjectManifests() with missing source dir should return nil, got error = %v", err)
	}
}
