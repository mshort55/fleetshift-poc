package kubernetes

import (
	"encoding/json"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

func TestExtractIdentityFields(t *testing.T) {
	ts := metav1.NewTime(time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC))
	r := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"uid":               "abc-123",
				"name":              "my-deploy",
				"namespace":         "default",
				"creationTimestamp": ts.Format(time.RFC3339),
				"labels": map[string]any{
					"app": "web",
				},
				"annotations": map[string]any{
					"note": "hello",
				},
				"ownerReferences": []any{
					map[string]any{
						"apiVersion": "apps/v1",
						"kind":       "ReplicaSet",
						"name":       "my-rs",
						"uid":        "owner-uid-1",
					},
					map[string]any{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"name":       "my-cm",
						"uid":        "owner-uid-2",
					},
				},
			},
		},
	}

	entry := SchemaEntry{
		GVR:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
		Kind: "Deployment",
	}

	item := ExtractObservedResource(r, entry, "target-1")

	if item.ID() != "target-1/abc-123" {
		t.Errorf("ID = %q, want %q", item.ID(), "target-1/abc-123")
	}
	if item.Type() != "apps/v1/Deployment" {
		t.Errorf("Type = %q, want %q", item.Type(), "apps/v1/Deployment")
	}
	if item.Name() != "my-deploy" {
		t.Errorf("Name = %q, want %q", item.Name(), "my-deploy")
	}
	if item.Labels()["app"] != "web" {
		t.Errorf("Labels[app] = %q, want %q", item.Labels()["app"], "web")
	}
}

func TestExtractIdentityFields_CoreAPIGroup(t *testing.T) {
	r := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]any{
				"uid":               string(types.UID("pod-uid")),
				"name":              "my-pod",
				"namespace":         "kube-system",
				"creationTimestamp": "2025-01-01T00:00:00Z",
			},
		},
	}

	entry := SchemaEntry{Kind: "Pod"}
	item := ExtractObservedResource(r, entry, "target-1")

	if item.Type() != "v1/Pod" {
		t.Errorf("Type = %q, want %q for core API", item.Type(), "v1/Pod")
	}
}

func TestExtractConditions_Enabled(t *testing.T) {
	r := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"uid":               "cond-uid",
				"name":              "cond-deploy",
				"creationTimestamp": "2025-01-01T00:00:00Z",
			},
			"status": map[string]any{
				"conditions": []any{
					map[string]any{
						"type":               "Available",
						"status":             "True",
						"reason":             "MinimumReplicasAvailable",
						"message":            "Deployment has minimum availability.",
						"lastTransitionTime": "2025-06-01T10:00:00Z",
					},
					map[string]any{
						"type":   "Progressing",
						"status": "True",
						"reason": "NewReplicaSetAvailable",
					},
				},
			},
		},
	}

	entry := SchemaEntry{Kind: "Deployment", ExtractConditions: true}
	item := ExtractObservedResource(r, entry, "target-1")

	conds := item.Conditions()
	if len(conds) != 2 {
		t.Fatalf("Conditions len = %d, want 2", len(conds))
	}

	c0 := conds[0]
	if c0.Type != "Available" {
		t.Errorf("Conditions[0].Type = %q, want %q", c0.Type, "Available")
	}
	if c0.Status != "True" {
		t.Errorf("Conditions[0].Status = %q, want %q", c0.Status, "True")
	}
	if c0.Reason != "MinimumReplicasAvailable" {
		t.Errorf("Conditions[0].Reason = %q, want %q", c0.Reason, "MinimumReplicasAvailable")
	}
	if c0.Message != "Deployment has minimum availability." {
		t.Errorf("Conditions[0].Message = %q, want %q", c0.Message, "Deployment has minimum availability.")
	}
	if c0.LastTransitionTime == nil {
		t.Error("Conditions[0].LastTransitionTime is nil")
	} else {
		want := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)
		if !c0.LastTransitionTime.Equal(want) {
			t.Errorf("Conditions[0].LastTransitionTime = %v, want %v", *c0.LastTransitionTime, want)
		}
	}

	c1 := conds[1]
	if c1.Type != "Progressing" {
		t.Errorf("Conditions[1].Type = %q, want %q", c1.Type, "Progressing")
	}
	if c1.LastTransitionTime != nil {
		t.Errorf("Conditions[1].LastTransitionTime should be nil when not present, got %v", *c1.LastTransitionTime)
	}
}

func TestExtractConditions_Disabled(t *testing.T) {
	r := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"uid":               "no-cond-uid",
				"name":              "no-cond-deploy",
				"creationTimestamp": "2025-01-01T00:00:00Z",
			},
			"status": map[string]any{
				"conditions": []any{
					map[string]any{
						"type":   "Available",
						"status": "True",
					},
				},
			},
		},
	}

	entry := SchemaEntry{Kind: "Deployment", ExtractConditions: false}
	item := ExtractObservedResource(r, entry, "target-1")

	if len(item.Conditions()) != 0 {
		t.Errorf("Conditions should be empty when ExtractConditions=false, got %d", len(item.Conditions()))
	}
}

func TestExtractObservedFields_NumberType(t *testing.T) {
	r := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"uid":               "fields-uid",
				"name":              "fields-deploy",
				"namespace":         "default",
				"creationTimestamp": "2025-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"replicas": int64(3),
			},
			"status": map[string]any{
				"readyReplicas": int64(2),
			},
		},
	}

	entry := SchemaEntry{
		Kind: "Deployment",
		Fields: []FieldExtraction{
			{Name: "replicas", JSONPath: ".spec.replicas", DataType: DataTypeNumber},
			{Name: "readyReplicas", JSONPath: ".status.readyReplicas", DataType: DataTypeNumber},
		},
	}

	item := ExtractObservedResource(r, entry, "target-1")

	observed := item.Observed()
	if observed == nil {
		t.Fatal("Observed is nil")
	}

	var fields map[string]any
	if err := json.Unmarshal(observed, &fields); err != nil {
		t.Fatalf("failed to unmarshal Observed: %v", err)
	}

	replVal, ok := fields["replicas"]
	if !ok {
		t.Fatal("Observed missing 'replicas'")
	}
	if replVal.(float64) != 3 {
		t.Errorf("replicas = %v, want 3", replVal)
	}

	readyVal, ok := fields["readyReplicas"]
	if !ok {
		t.Fatal("Observed missing 'readyReplicas'")
	}
	if readyVal.(float64) != 2 {
		t.Errorf("readyReplicas = %v, want 2", readyVal)
	}
}

func TestExtractObservedFields_BytesType(t *testing.T) {
	r := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Node",
			"metadata": map[string]any{
				"uid":               "node-uid",
				"name":              "worker-1",
				"creationTimestamp": "2025-01-01T00:00:00Z",
			},
			"status": map[string]any{
				"allocatable": map[string]any{
					"memory": "128Mi",
				},
			},
		},
	}

	entry := SchemaEntry{
		Kind: "Node",
		Fields: []FieldExtraction{
			{Name: "memoryAllocatable", JSONPath: ".status.allocatable.memory", DataType: DataTypeBytes},
		},
	}

	item := ExtractObservedResource(r, entry, "target-1")

	observed := item.Observed()
	if observed == nil {
		t.Fatal("Observed is nil")
	}

	var fields map[string]any
	if err := json.Unmarshal(observed, &fields); err != nil {
		t.Fatalf("failed to unmarshal Observed: %v", err)
	}

	memVal, ok := fields["memoryAllocatable"]
	if !ok {
		t.Fatal("Observed missing 'memoryAllocatable'")
	}
	// 128Mi = 128 * 1024 * 1024 = 134217728
	want := float64(134217728)
	if memVal.(float64) != want {
		t.Errorf("memoryAllocatable = %v, want %v", memVal, want)
	}
}

func TestExtractObservedFields_StringType(t *testing.T) {
	r := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]any{
				"uid":               "str-uid",
				"name":              "str-pod",
				"creationTimestamp": "2025-01-01T00:00:00Z",
			},
			"status": map[string]any{
				"phase": "Running",
			},
		},
	}

	entry := SchemaEntry{
		Kind: "Pod",
		Fields: []FieldExtraction{
			{Name: "phase", JSONPath: ".status.phase"},
		},
	}

	item := ExtractObservedResource(r, entry, "target-1")

	observed := item.Observed()
	if observed == nil {
		t.Fatal("Observed is nil")
	}

	var fields map[string]any
	if err := json.Unmarshal(observed, &fields); err != nil {
		t.Fatalf("failed to unmarshal Observed: %v", err)
	}

	v, ok := fields["phase"]
	if !ok {
		t.Fatal("Observed missing 'phase'")
	}
	if v.(string) != "Running" {
		t.Errorf("phase = %q, want %q", v, "Running")
	}
}

func TestExtractObservedFields_SliceType(t *testing.T) {
	r := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]any{
				"uid":               "slice-uid",
				"name":              "slice-pod",
				"creationTimestamp": "2025-01-01T00:00:00Z",
			},
			"status": map[string]any{
				"containerStatuses": []any{
					map[string]any{"image": "nginx:1.25"},
					map[string]any{"image": "sidecar:latest"},
				},
			},
		},
	}

	entry := SchemaEntry{
		Kind: "Pod",
		Fields: []FieldExtraction{
			{Name: "containerImages", JSONPath: ".status.containerStatuses[*].image", DataType: DataTypeSlice},
		},
	}

	item := ExtractObservedResource(r, entry, "target-1")

	observed := item.Observed()
	if observed == nil {
		t.Fatal("Observed is nil")
	}

	var fields map[string]any
	if err := json.Unmarshal(observed, &fields); err != nil {
		t.Fatalf("failed to unmarshal Observed: %v", err)
	}

	v, ok := fields["containerImages"]
	if !ok {
		t.Fatal("Observed missing 'containerImages'")
	}
	listVal, ok := v.([]any)
	if !ok {
		t.Fatal("containerImages is not a list")
	}
	if len(listVal) != 2 {
		t.Fatalf("containerImages len = %d, want 2", len(listVal))
	}
	if listVal[0].(string) != "nginx:1.25" {
		t.Errorf("containerImages[0] = %q, want %q", listVal[0], "nginx:1.25")
	}
	if listVal[1].(string) != "sidecar:latest" {
		t.Errorf("containerImages[1] = %q, want %q", listVal[1], "sidecar:latest")
	}
}

func TestExtractObservedFields_BoolNativeNotString(t *testing.T) {
	r := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]any{
				"uid":               "bool-uid",
				"name":              "bool-pod",
				"creationTimestamp": "2025-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"hostNetwork": true,
			},
		},
	}

	entry := SchemaEntry{
		Kind: "Pod",
		Fields: []FieldExtraction{
			{Name: "hostNetwork", JSONPath: ".spec.hostNetwork"},
		},
	}

	item := ExtractObservedResource(r, entry, "target-1")

	observed := item.Observed()
	if observed == nil {
		t.Fatal("Observed is nil")
	}

	var fields map[string]any
	if err := json.Unmarshal(observed, &fields); err != nil {
		t.Fatalf("failed to unmarshal Observed: %v", err)
	}

	v, ok := fields["hostNetwork"]
	if !ok {
		t.Fatal("Observed missing 'hostNetwork'")
	}
	if v.(bool) != true {
		t.Error("hostNetwork should be true (native bool), not a string")
	}
}

func TestExtractObservedFields_JSONPathNormalization(t *testing.T) {
	r := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]any{
				"uid":               "jp-uid",
				"name":              "jp-pod",
				"creationTimestamp": "2025-01-01T00:00:00Z",
			},
			"status": map[string]any{
				"phase": "Pending",
			},
		},
	}

	// JSONPath with braces should also work
	entry := SchemaEntry{
		Kind: "Pod",
		Fields: []FieldExtraction{
			{Name: "phase", JSONPath: "{.status.phase}"},
		},
	}

	item := ExtractObservedResource(r, entry, "target-1")

	observed := item.Observed()
	if observed == nil {
		t.Fatal("Observed is nil")
	}

	var fields map[string]any
	if err := json.Unmarshal(observed, &fields); err != nil {
		t.Fatalf("failed to unmarshal Observed: %v", err)
	}

	v, ok := fields["phase"]
	if !ok {
		t.Fatal("Observed missing 'phase'")
	}
	if v.(string) != "Pending" {
		t.Errorf("phase = %q, want %q", v, "Pending")
	}
}

func TestExtractObservedFields_MissingFieldIsSkipped(t *testing.T) {
	r := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"uid":               "miss-uid",
				"name":              "miss-deploy",
				"creationTimestamp": "2025-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"replicas": int64(3),
			},
			// No status at all
		},
	}

	entry := SchemaEntry{
		Kind: "Deployment",
		Fields: []FieldExtraction{
			{Name: "replicas", JSONPath: ".spec.replicas", DataType: DataTypeNumber},
			{Name: "readyReplicas", JSONPath: ".status.readyReplicas", DataType: DataTypeNumber},
		},
	}

	item := ExtractObservedResource(r, entry, "target-1")

	observed := item.Observed()
	if observed == nil {
		t.Fatal("Observed is nil")
	}

	var fields map[string]any
	if err := json.Unmarshal(observed, &fields); err != nil {
		t.Fatalf("failed to unmarshal Observed: %v", err)
	}

	if _, ok := fields["replicas"]; !ok {
		t.Error("should have 'replicas'")
	}
	if _, ok := fields["readyReplicas"]; ok {
		t.Error("should NOT have 'readyReplicas' when status is missing")
	}
}

func TestExtractAnnotations_StripsInternalKeys(t *testing.T) {
	// This test now just verifies that the extraction function does not
	// crash when annotations with internal keys are present. The domain
	// InventoryItem no longer stores annotations, but the extraction
	// should still succeed without error.
	r := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]any{
				"uid":               "strip-uid",
				"name":              "strip-pod",
				"creationTimestamp": "2025-01-01T00:00:00Z",
				"annotations": map[string]any{
					"keep": "yes",
					"apps.open-cluster-management.io/hosting-subscription": "sub/foo",
					"apps.open-cluster-management.io/hosting-deployable":   "dep/bar",
				},
			},
		},
	}

	entry := SchemaEntry{Kind: "Pod"}
	item := ExtractObservedResource(r, entry, "target-1")

	// The item should be created successfully
	if item.Name() != "strip-pod" {
		t.Errorf("Name = %q, want %q", item.Name(), "strip-pod")
	}
}

func TestExtractObservedFields_MapStringType(t *testing.T) {
	r := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]any{
				"uid":               "map-uid",
				"name":              "map-pod",
				"creationTimestamp": "2025-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"nodeSelector": map[string]any{
					"disktype": "ssd",
					"region":   "us-east",
				},
			},
		},
	}

	entry := SchemaEntry{
		Kind: "Pod",
		Fields: []FieldExtraction{
			{Name: "nodeSelector", JSONPath: ".spec.nodeSelector", DataType: DataTypeMapString},
		},
	}

	item := ExtractObservedResource(r, entry, "target-1")

	observed := item.Observed()
	if observed == nil {
		t.Fatal("Observed is nil")
	}

	var fields map[string]any
	if err := json.Unmarshal(observed, &fields); err != nil {
		t.Fatalf("failed to unmarshal Observed: %v", err)
	}

	v, ok := fields["nodeSelector"]
	if !ok {
		t.Fatal("Observed missing 'nodeSelector'")
	}
	sv, ok := v.(map[string]any)
	if !ok {
		t.Fatal("nodeSelector is not a map")
	}
	if sv["disktype"].(string) != "ssd" {
		t.Errorf("nodeSelector.disktype = %q, want %q", sv["disktype"], "ssd")
	}
	if sv["region"].(string) != "us-east" {
		t.Errorf("nodeSelector.region = %q, want %q", sv["region"], "us-east")
	}
}
