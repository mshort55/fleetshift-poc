package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDatabaseURLFile(t *testing.T) {
	tests := []struct {
		name            string
		databaseURL     string
		databaseURLFile string
		dbPath          string
		fileContent     string
		wantURL         string
		wantErr         string
	}{
		{
			name:            "reads URL from file",
			databaseURLFile: "url-file",
			fileContent:     "postgres://user:pass@host:5432/db?sslmode=disable\n",
			wantURL:         "postgres://user:pass@host:5432/db?sslmode=disable",
		},
		{
			name:            "trims whitespace",
			databaseURLFile: "url-file",
			fileContent:     "  postgres://user:pass@host:5432/db  \n",
			wantURL:         "postgres://user:pass@host:5432/db",
		},
		{
			name:            "mutual exclusivity with --database-url",
			databaseURL:     "postgres://inline",
			databaseURLFile: "url-file",
			fileContent:     "postgres://from-file",
			wantErr:         "--database-url-file and --database-url are mutually exclusive",
		},
		{
			name:            "mutual exclusivity with --db",
			databaseURLFile: "url-file",
			dbPath:          "custom.db",
			fileContent:     "postgres://from-file",
			wantErr:         "--database-url-file and --db are mutually exclusive",
		},
		{
			name:            "file not found",
			databaseURLFile: "nonexistent",
			wantErr:         "read database URL file",
		},
		{
			name:        "no file flag — databaseURL unchanged",
			databaseURL: "postgres://inline",
			wantURL:     "postgres://inline",
		},
		{
			name:    "neither flag set — empty URL",
			wantURL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &serveFlags{
				databaseURL:     tt.databaseURL,
				databaseURLFile: tt.databaseURLFile,
			}
			if tt.dbPath != "" {
				f.dbPath = tt.dbPath
			} else {
				f.dbPath = "fleetshift.db"
			}

			if tt.fileContent != "" {
				dir := t.TempDir()
				path := filepath.Join(dir, "url-file")
				if err := os.WriteFile(path, []byte(tt.fileContent), 0400); err != nil {
					t.Fatal(err)
				}
				f.databaseURLFile = path
			} else if tt.databaseURLFile == "nonexistent" {
				f.databaseURLFile = filepath.Join(t.TempDir(), "nonexistent")
			}

			err := resolveDatabaseURLFile(f)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if f.databaseURL != tt.wantURL {
				t.Errorf("databaseURL = %q, want %q", f.databaseURL, tt.wantURL)
			}
		})
	}
}
