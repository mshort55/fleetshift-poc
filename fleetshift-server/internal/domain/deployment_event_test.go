package domain_test

import (
	"sort"
	"testing"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

func ids(pool []domain.TargetInfo) []string {
	out := make([]string, len(pool))
	for i, t := range pool {
		out[i] = string(t.ID)
	}
	sort.Strings(out)
	return out
}

func assertIDs(t *testing.T, pool []domain.TargetInfo, want ...string) {
	t.Helper()
	sort.Strings(want)
	got := ids(pool)
	if len(got) != len(want) {
		t.Fatalf("pool IDs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pool IDs = %v, want %v", got, want)
		}
	}
}

func TestApplyPoolChange_Set(t *testing.T) {
	pool := []domain.TargetInfo{{ID: "a"}, {ID: "b"}}
	result := domain.ApplyPoolChange(pool, domain.PoolChange{
		Set: []domain.TargetInfo{{ID: "x"}, {ID: "y"}, {ID: "z"}},
	})
	assertIDs(t, result, "x", "y", "z")
}

func TestApplyPoolChange_Added(t *testing.T) {
	pool := []domain.TargetInfo{{ID: "a"}, {ID: "b"}}
	result := domain.ApplyPoolChange(pool, domain.PoolChange{
		Added: []domain.TargetInfo{{ID: "c"}},
	})
	assertIDs(t, result, "a", "b", "c")
}

func TestApplyPoolChange_Removed(t *testing.T) {
	pool := []domain.TargetInfo{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	result := domain.ApplyPoolChange(pool, domain.PoolChange{
		Removed: []domain.TargetID{"b"},
	})
	assertIDs(t, result, "a", "c")
}

func TestApplyPoolChange_Updated(t *testing.T) {
	pool := []domain.TargetInfo{
		{ID: "a", Labels: map[string]string{"env": "dev"}},
		{ID: "b", Labels: map[string]string{"env": "dev"}},
	}
	result := domain.ApplyPoolChange(pool, domain.PoolChange{
		Updated: []domain.TargetInfo{{ID: "b", Labels: map[string]string{"env": "prod"}}},
	})
	assertIDs(t, result, "a", "b")
	for _, t := range result {
		if t.ID == "b" && t.Labels["env"] != "prod" {
			t.Labels["env"] = "should-be-prod"
		}
	}
}

func TestApplyPoolChange_Combined(t *testing.T) {
	pool := []domain.TargetInfo{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	result := domain.ApplyPoolChange(pool, domain.PoolChange{
		Added:   []domain.TargetInfo{{ID: "d"}},
		Removed: []domain.TargetID{"a"},
		Updated: []domain.TargetInfo{{ID: "b", Name: "updated-b"}},
	})
	assertIDs(t, result, "b", "c", "d")
	for _, ti := range result {
		if ti.ID == "b" && ti.Name != "updated-b" {
			t.Errorf("target b Name = %q, want %q", ti.Name, "updated-b")
		}
	}
}

func TestApplyPoolChange_EmptyPool_Added(t *testing.T) {
	result := domain.ApplyPoolChange(nil, domain.PoolChange{
		Added: []domain.TargetInfo{{ID: "a"}, {ID: "b"}},
	})
	assertIDs(t, result, "a", "b")
}

func TestApplyPoolChange_SetNil_NoReplace(t *testing.T) {
	pool := []domain.TargetInfo{{ID: "a"}}
	result := domain.ApplyPoolChange(pool, domain.PoolChange{})
	assertIDs(t, result, "a")
}

func TestTargetInfosByID(t *testing.T) {
	pool := []domain.TargetInfo{
		{ID: "a", Name: "A"},
		{ID: "b", Name: "B"},
		{ID: "c", Name: "C"},
	}
	result := domain.TargetInfosByID([]domain.TargetID{"b", "c", "missing"}, pool)
	assertIDs(t, result, "b", "c")
}
