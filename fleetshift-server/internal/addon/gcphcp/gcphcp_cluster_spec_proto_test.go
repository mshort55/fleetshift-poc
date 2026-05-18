package gcphcp

import (
	"os"
	"strings"
	"testing"
)

func TestGCPHCPClusterSpecProto_ValidatesPositiveNodepoolNumbers(t *testing.T) {
	data, err := os.ReadFile("gcphcp_cluster_spec.proto")
	if err != nil {
		t.Fatalf("read proto: %v", err)
	}

	protoText := string(data)
	for _, want := range []string{
		`int32 replicas = 2 [(buf.validate.field).int32.gt = 0];`,
		`int32 root_volume_size = 4 [(buf.validate.field).int32.gt = 0];`,
	} {
		if !strings.Contains(protoText, want) {
			t.Fatalf("expected proto to contain %q", want)
		}
	}
}
