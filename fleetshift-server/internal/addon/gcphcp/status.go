package gcphcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fleetshift/fleetshift-poc/fleetshift-server/internal/domain"
)

// ParseClusterPhase extracts the status.phase field from cluster data.
// Returns "Unknown" if the field is not found or not a string.
func ParseClusterPhase(clusterData map[string]any) string {
	status, ok := clusterData["status"].(map[string]any)
	if !ok {
		return "Unknown"
	}
	phase, ok := status["phase"].(string)
	if !ok {
		return "Unknown"
	}
	return phase
}

// ResolveGuestAPIEndpoint resolves the guest API endpoint from cluster status data.
// It first scans controller_status[].conditions[] for a condition with type="APIServer"
// whose message starts with "https://". If not found, it falls back to the api_endpoint field.
// Returns an error if neither source provides a valid endpoint.
func ResolveGuestAPIEndpoint(statusData map[string]any) (string, error) {
	// Primary: scan controller_status for APIServer condition
	if controllerStatus, ok := statusData["controller_status"].([]any); ok {
		for _, ctrl := range controllerStatus {
			ctrlMap, ok := ctrl.(map[string]any)
			if !ok {
				continue
			}
			conditions, ok := ctrlMap["conditions"].([]any)
			if !ok {
				continue
			}
			for _, cond := range conditions {
				condMap, ok := cond.(map[string]any)
				if !ok {
					continue
				}
				condType, _ := condMap["type"].(string)
				if condType != "APIServer" {
					continue
				}
				message, _ := condMap["message"].(string)
				if strings.HasPrefix(message, "https://") {
					return message, nil
				}
			}
		}
	}

	// Fallback: check api_endpoint field
	if apiEndpoint, ok := statusData["api_endpoint"].(string); ok && apiEndpoint != "" {
		return apiEndpoint, nil
	}

	return "", fmt.Errorf("guest API endpoint not found in cluster status")
}

// PollClusterReady polls the cluster status until it reaches "Ready" phase or fails.
// It polls every 15 seconds for up to 20 minutes.
// Returns nil when phase="Ready", error when phase="Failed", or error on timeout.
func PollClusterReady(ctx context.Context, client *CLSClient, clusterID string, signaler *domain.DeliverySignaler) error {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	timeout := time.After(20 * time.Minute)

	// Check immediately first
	clusterData, err := client.GetCluster(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("get cluster: %w", err)
	}

	phase := ParseClusterPhase(clusterData)
	signaler.Emit(ctx, domain.DeliveryEvent{
		Timestamp: time.Now(),
		Kind:      domain.DeliveryEventProgress,
		Message:   fmt.Sprintf("Cluster phase: %s", phase),
	})

	if phase == "Ready" {
		return nil
	}
	if phase == "Failed" {
		statusMsg := ""
		if status, ok := clusterData["status"].(map[string]any); ok {
			if msg, ok := status["message"].(string); ok {
				statusMsg = msg
			}
		}
		return fmt.Errorf("cluster provisioning failed: %s", statusMsg)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for cluster to become ready (phase=%s)", phase)
		case <-ticker.C:
			clusterData, err := client.GetCluster(ctx, clusterID)
			if err != nil {
				return fmt.Errorf("get cluster: %w", err)
			}

			phase = ParseClusterPhase(clusterData)
			signaler.Emit(ctx, domain.DeliveryEvent{
				Timestamp: time.Now(),
				Kind:      domain.DeliveryEventProgress,
				Message:   fmt.Sprintf("Cluster phase: %s", phase),
			})

			if phase == "Ready" {
				return nil
			}
			if phase == "Failed" {
				statusMsg := ""
				if status, ok := clusterData["status"].(map[string]any); ok {
					if msg, ok := status["message"].(string); ok {
						statusMsg = msg
					}
				}
				return fmt.Errorf("cluster provisioning failed: %s", statusMsg)
			}
		}
	}
}

// PollClusterDeleted polls until the cluster is deleted (404 response).
// It polls every 15 seconds for up to 20 minutes.
// Returns nil when cluster returns 404, error on timeout.
func PollClusterDeleted(ctx context.Context, client *CLSClient, clusterID string, signaler *domain.DeliverySignaler) error {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	timeout := time.After(20 * time.Minute)

	// Check immediately first
	_, err := client.GetCluster(ctx, clusterID)
	if err != nil && strings.Contains(err.Error(), "404") {
		signaler.Emit(ctx, domain.DeliveryEvent{
			Timestamp: time.Now(),
			Kind:      domain.DeliveryEventProgress,
			Message:   "Cluster deleted",
		})
		return nil
	}

	signaler.Emit(ctx, domain.DeliveryEvent{
		Timestamp: time.Now(),
		Kind:      domain.DeliveryEventProgress,
		Message:   "Waiting for cluster deletion",
	})

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for cluster deletion")
		case <-ticker.C:
			_, err := client.GetCluster(ctx, clusterID)
			if err != nil && strings.Contains(err.Error(), "404") {
				signaler.Emit(ctx, domain.DeliveryEvent{
					Timestamp: time.Now(),
					Kind:      domain.DeliveryEventProgress,
					Message:   "Cluster deleted",
				})
				return nil
			}

			signaler.Emit(ctx, domain.DeliveryEvent{
				Timestamp: time.Now(),
				Kind:      domain.DeliveryEventProgress,
				Message:   "Waiting for cluster deletion",
			})
		}
	}
}
