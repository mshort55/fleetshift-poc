package oidc

import "testing"

func TestRewriteLocalhost(t *testing.T) {
	tests := []struct {
		name          string
		rawURL        string
		containerHost string
		want          string
	}{
		{
			name:          "rewrites localhost with port",
			rawURL:        "http://localhost:8180/auth/realms/fleetshift/.well-known/openid-configuration",
			containerHost: "keycloak",
			want:          "http://keycloak:8180/auth/realms/fleetshift/.well-known/openid-configuration",
		},
		{
			name:          "rewrites localhost without port",
			rawURL:        "http://localhost/auth/realms/fleetshift",
			containerHost: "keycloak",
			want:          "http://keycloak/auth/realms/fleetshift",
		},
		{
			name:          "rewrites 127.0.0.1 with port",
			rawURL:        "http://127.0.0.1:8180/auth/realms/fleetshift",
			containerHost: "keycloak",
			want:          "http://keycloak:8180/auth/realms/fleetshift",
		},
		{
			name:          "preserves https scheme",
			rawURL:        "https://localhost:8443/auth/realms/fleetshift",
			containerHost: "keycloak",
			want:          "https://keycloak:8443/auth/realms/fleetshift",
		},
		{
			name:          "no-op when containerHost is empty",
			rawURL:        "http://localhost:8180/auth/realms/fleetshift",
			containerHost: "",
			want:          "http://localhost:8180/auth/realms/fleetshift",
		},
		{
			name:          "no-op for non-localhost host",
			rawURL:        "https://login.example.com/auth/realms/fleetshift",
			containerHost: "keycloak",
			want:          "https://login.example.com/auth/realms/fleetshift",
		},
		{
			name:          "no-op for malformed URL",
			rawURL:        "://not-a-url",
			containerHost: "keycloak",
			want:          "://not-a-url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rewriteLocalhost(tt.rawURL, tt.containerHost)
			if got != tt.want {
				t.Errorf("rewriteLocalhost(%q, %q) = %q, want %q", tt.rawURL, tt.containerHost, got, tt.want)
			}
		})
	}
}
