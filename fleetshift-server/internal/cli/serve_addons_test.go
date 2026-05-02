package cli

import (
	"testing"
)

func TestParseAddons(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  map[string]bool
	}{
		{
			name:  "all addons",
			input: "kind,ocp,kubernetes",
			want:  map[string]bool{"kind": true, "ocp": true, "kubernetes": true},
		},
		{
			name:  "production subset",
			input: "ocp,kubernetes",
			want:  map[string]bool{"ocp": true, "kubernetes": true},
		},
		{
			name:  "single addon",
			input: "kind",
			want:  map[string]bool{"kind": true},
		},
		{
			name:  "whitespace trimmed",
			input: " kind , ocp ",
			want:  map[string]bool{"kind": true, "ocp": true},
		},
		{
			name:  "empty string",
			input: "",
			want:  map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseAddons(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("parseAddons(%q) returned %d entries, want %d", tt.input, len(got), len(tt.want))
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("parseAddons(%q)[%q] = %v, want %v", tt.input, k, got[k], v)
				}
			}
		})
	}
}
