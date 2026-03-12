package kind

import (
	"fmt"
	"os"
	"strings"
)

// OIDCSpec configures the K8s API server's OIDC authentication on a
// kind cluster. When set on a [ClusterSpec], the agent generates the
// kind cluster config with the appropriate kubeadmConfigPatches and
// CA cert extraMount.
type OIDCSpec struct {
	IssuerURL     string `json:"issuerURL"`
	ClientID      string `json:"clientID"`
	UsernameClaim string `json:"usernameClaim,omitempty"` // default: "sub"
	GroupsClaim   string `json:"groupsClaim,omitempty"`   // default: "groups"
	CABundle      []byte `json:"caBundle,omitempty"`      // PEM-encoded CA cert
}

func (s *OIDCSpec) validate() error {
	if s.IssuerURL == "" {
		return fmt.Errorf("oidc: issuerURL is required")
	}
	if s.ClientID == "" {
		return fmt.Errorf("oidc: clientID is required")
	}
	return nil
}

func (s *OIDCSpec) usernameClaim() string {
	if s.UsernameClaim != "" {
		return s.UsernameClaim
	}
	return "sub"
}

func (s *OIDCSpec) groupsClaim() string {
	if s.GroupsClaim != "" {
		return s.GroupsClaim
	}
	return "groups"
}

const oidcCACertContainerPath = "/etc/kubernetes/pki/oidc-ca.pem"

// BuildKindOIDCConfig generates a kind cluster config YAML with OIDC
// API server flags and (optionally) a CA cert mount. caCertHostPath is
// the host-side path to the CA certificate file; it may be empty if no
// CABundle is configured.
func BuildKindOIDCConfig(spec *OIDCSpec, caCertHostPath string) ([]byte, error) {
	if err := spec.validate(); err != nil {
		return nil, err
	}

	var extraArgs strings.Builder
	fmt.Fprintf(&extraArgs, "        oidc-issuer-url: %q\n", spec.IssuerURL)
	fmt.Fprintf(&extraArgs, "        oidc-client-id: %q\n", spec.ClientID)
	fmt.Fprintf(&extraArgs, "        oidc-username-claim: %q\n", spec.usernameClaim())
	fmt.Fprintf(&extraArgs, "        oidc-groups-claim: %q\n", spec.groupsClaim())
	if caCertHostPath != "" {
		fmt.Fprintf(&extraArgs, "        oidc-ca-file: %q\n", oidcCACertContainerPath)
	}

	var extraMounts string
	if caCertHostPath != "" {
		extraMounts = fmt.Sprintf(`    extraMounts:
      - hostPath: %s
        containerPath: %s
        readOnly: true
`, caCertHostPath, oidcCACertContainerPath)
	}

	config := fmt.Sprintf(`kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
%skubeadmConfigPatches:
  - |
    kind: ClusterConfiguration
    apiServer:
      extraArgs:
%s`, extraMounts, extraArgs.String())

	return []byte(config), nil
}

// writeCABundle writes the CA bundle to a temp file in dir and returns
// the path. If dir is empty, [os.TempDir] is used. The caller is
// responsible for cleanup.
func writeCABundle(caBundle []byte, dir string) (string, error) {
	f, err := os.CreateTemp(dir, "kind-oidc-ca-*.pem")
	if err != nil {
		return "", fmt.Errorf("create CA bundle temp file: %w", err)
	}
	if _, err := f.Write(caBundle); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", fmt.Errorf("write CA bundle: %w", err)
	}
	f.Close()
	return f.Name(), nil
}
