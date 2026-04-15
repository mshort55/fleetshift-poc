package ccoctl

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ExtractBinaryArgs returns oc args for extracting the ccoctl binary from a release image.
// Returns: adm release extract --command=ccoctl --to <workDir> --registry-config <pullSecretFile> <releaseImage>
func ExtractBinaryArgs(workDir, pullSecretFile, releaseImage string) []string {
	return []string{
		"adm",
		"release",
		"extract",
		"--command=ccoctl",
		"--to", workDir,
		"--registry-config", pullSecretFile,
		releaseImage,
	}
}

// ExtractCredReqArgs returns oc args for extracting credentials requests from a release image.
// Returns: adm release extract --credentials-requests --cloud=aws --to <credReqDir> --registry-config <pullSecretFile> <releaseImage>
func ExtractCredReqArgs(credReqDir, pullSecretFile, releaseImage string) []string {
	return []string{
		"adm",
		"release",
		"extract",
		"--credentials-requests",
		"--cloud=aws",
		"--to", credReqDir,
		"--registry-config", pullSecretFile,
		releaseImage,
	}
}

// CreateAllArgs returns ccoctl args for creating all AWS resources for STS mode.
// Returns: aws create-all --name <name> --region <region> --credentials-requests-dir <dir> --output-dir <dir>
func CreateAllArgs(clusterName, region, credReqDir, outputDir string) []string {
	return []string{
		"aws",
		"create-all",
		"--name", clusterName,
		"--region", region,
		"--credentials-requests-dir", credReqDir,
		"--output-dir", outputDir,
	}
}

// DeleteArgs returns ccoctl args for deleting AWS resources for STS mode.
// Returns: aws delete --name <name> --region <region>
func DeleteArgs(clusterName, region string) []string {
	return []string{
		"aws",
		"delete",
		"--name", clusterName,
		"--region", region,
	}
}

// BinaryPath returns the path to the extracted ccoctl binary.
func BinaryPath(workDir string) string {
	return filepath.Join(workDir, "ccoctl")
}

// CredReqDir returns the path to the credentials requests directory.
func CredReqDir(workDir string) string {
	return filepath.Join(workDir, "credrequests")
}

// OutputDir returns the path to the ccoctl output directory.
func OutputDir(workDir string) string {
	return filepath.Join(workDir, "ccoctl-output")
}

// InjectManifests copies manifests/ and tls/ subdirectories from ccoctlOutputDir
// into installerDir. Silently skips if source directory doesn't exist (returns nil).
func InjectManifests(ccoctlOutputDir, installerDir string) error {
	// Check if source directory exists
	if _, err := os.Stat(ccoctlOutputDir); os.IsNotExist(err) {
		return nil // Silently skip if source doesn't exist
	}

	// Copy manifests/ subdirectory
	manifestsSrc := filepath.Join(ccoctlOutputDir, "manifests")
	if _, err := os.Stat(manifestsSrc); err == nil {
		manifestsDest := filepath.Join(installerDir, "manifests")
		if err := copyDir(manifestsSrc, manifestsDest); err != nil {
			return fmt.Errorf("failed to copy manifests: %w", err)
		}
	}

	// Copy tls/ subdirectory
	tlsSrc := filepath.Join(ccoctlOutputDir, "tls")
	if _, err := os.Stat(tlsSrc); err == nil {
		tlsDest := filepath.Join(installerDir, "tls")
		if err := copyDir(tlsSrc, tlsDest); err != nil {
			return fmt.Errorf("failed to copy tls: %w", err)
		}
	}

	return nil
}

// copyDir recursively copies a directory from src to dst.
func copyDir(src, dst string) error {
	// Get source directory info
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	// Create destination directory
	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	// Read directory contents
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	// Copy each entry
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			// Recursively copy subdirectory
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			// Copy file
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	// Open source file
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Get source file info for permissions
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	// Create destination file
	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	// Copy contents
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return nil
}
