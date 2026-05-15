package gcphcp_test

import (
	"testing"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/addon/gcphcp"
)

func TestParseClusterPhase(t *testing.T) {
	tests := []struct {
		name  string
		data  map[string]any
		want  string
	}{
		{
			name: "Ready phase",
			data: map[string]any{
				"status": map[string]any{
					"phase": "Ready",
				},
			},
			want: "Ready",
		},
		{
			name: "Progressing phase",
			data: map[string]any{
				"status": map[string]any{
					"phase": "Progressing",
				},
			},
			want: "Progressing",
		},
		{
			name: "Failed phase",
			data: map[string]any{
				"status": map[string]any{
					"phase": "Failed",
				},
			},
			want: "Failed",
		},
		{
			name: "empty status",
			data: map[string]any{},
			want: "Unknown",
		},
		{
			name: "status not a map",
			data: map[string]any{
				"status": "invalid",
			},
			want: "Unknown",
		},
		{
			name: "missing phase field",
			data: map[string]any{
				"status": map[string]any{},
			},
			want: "Unknown",
		},
		{
			name: "phase not a string",
			data: map[string]any{
				"status": map[string]any{
					"phase": 123,
				},
			},
			want: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gcphcp.ParseClusterPhase(tt.data)
			if got != tt.want {
				t.Errorf("ParseClusterPhase() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveGuestAPIEndpoint_FromControllerStatus(t *testing.T) {
	statusData := map[string]any{
		"controller_status": []any{
			map[string]any{
				"name": "cls-hypershift-client",
				"conditions": []any{
					map[string]any{
						"type":    "SomeOtherCondition",
						"message": "not an endpoint",
					},
					map[string]any{
						"type":    "APIServer",
						"message": "https://guest-api.example.com:6443",
					},
				},
			},
			map[string]any{
				"name": "another-controller",
				"conditions": []any{
					map[string]any{
						"type":    "APIServer",
						"message": "should not be picked",
					},
				},
			},
		},
	}

	endpoint, err := gcphcp.ResolveGuestAPIEndpoint(statusData)
	if err != nil {
		t.Fatalf("ResolveGuestAPIEndpoint() error = %v", err)
	}
	want := "https://guest-api.example.com:6443"
	if endpoint != want {
		t.Errorf("ResolveGuestAPIEndpoint() = %q, want %q", endpoint, want)
	}
}

func TestResolveGuestAPIEndpoint_FallbackToAPIEndpoint(t *testing.T) {
	statusData := map[string]any{
		"api_endpoint": "https://fallback-api.example.com:6443",
		"controller_status": []any{
			map[string]any{
				"name": "cls-hypershift-client",
				"conditions": []any{
					map[string]any{
						"type":    "SomeOtherCondition",
						"message": "not an endpoint",
					},
				},
			},
		},
	}

	endpoint, err := gcphcp.ResolveGuestAPIEndpoint(statusData)
	if err != nil {
		t.Fatalf("ResolveGuestAPIEndpoint() error = %v", err)
	}
	want := "https://fallback-api.example.com:6443"
	if endpoint != want {
		t.Errorf("ResolveGuestAPIEndpoint() = %q, want %q", endpoint, want)
	}
}

func TestResolveGuestAPIEndpoint_NotFound(t *testing.T) {
	tests := []struct {
		name       string
		statusData map[string]any
	}{
		{
			name:       "empty status",
			statusData: map[string]any{},
		},
		{
			name: "no controller_status and no api_endpoint",
			statusData: map[string]any{
				"phase": "Ready",
			},
		},
		{
			name: "controller_status without APIServer condition",
			statusData: map[string]any{
				"controller_status": []any{
					map[string]any{
						"name": "cls-hypershift-client",
						"conditions": []any{
							map[string]any{
								"type":    "SomeOtherCondition",
								"message": "not an endpoint",
							},
						},
					},
				},
			},
		},
		{
			name: "APIServer condition message does not start with https://",
			statusData: map[string]any{
				"controller_status": []any{
					map[string]any{
						"name": "cls-hypershift-client",
						"conditions": []any{
							map[string]any{
								"type":    "APIServer",
								"message": "not ready yet",
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, err := gcphcp.ResolveGuestAPIEndpoint(tt.statusData)
			if err == nil {
				t.Errorf("ResolveGuestAPIEndpoint() = %q, want error", endpoint)
			}
		})
	}
}
