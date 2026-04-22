package sqlite_test

import (
	"testing"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain/vaulttest"
	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/infrastructure/sqlite"
)

func TestVault(t *testing.T) {
	vaulttest.Run(t, func(t *testing.T) domain.Vault {
		return &sqlite.VaultStore{DB: sqlite.OpenTestDB(t)}
	})
}
