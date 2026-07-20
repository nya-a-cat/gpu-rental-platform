package clusterstate

import (
	"strings"
	"testing"
	"time"
)

func TestEvaluateClusterSignals(t *testing.T) {
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()
	tests := []struct {
		name             string
		signals          Signals
		connectionState  ConnectionState
		connected        bool
		schedulable      bool
		inventoryFresh   bool
		executionHealthy bool
	}{
		{
			name: "healthy and schedulable",
			signals: Signals{
				Now:                 now,
				LastHeartbeatAt:     now.Add(-DefaultHeartbeatInterval),
				LastInventoryAt:     now.Add(-DefaultHeartbeatInterval),
				ManuallySchedulable: true,
				ExecutionHealthy:    true,
			},
			connectionState:  ConnectionConnected,
			connected:        true,
			schedulable:      true,
			inventoryFresh:   true,
			executionHealthy: true,
		},
		{
			name: "degraded heartbeat",
			signals: Signals{
				Now:                 now,
				LastHeartbeatAt:     now.Add(-DefaultDegradedAfter - time.Second),
				LastInventoryAt:     now,
				ManuallySchedulable: true,
				ExecutionHealthy:    true,
			},
			connectionState:  ConnectionDegraded,
			inventoryFresh:   true,
			executionHealthy: true,
		},
		{
			name: "offline heartbeat",
			signals: Signals{
				Now:                 now,
				LastHeartbeatAt:     now.Add(-DefaultOfflineAfter - time.Second),
				LastInventoryAt:     now,
				ManuallySchedulable: true,
				ExecutionHealthy:    true,
			},
			connectionState:  ConnectionOffline,
			inventoryFresh:   true,
			executionHealthy: true,
		},
		{
			name: "missing heartbeat",
			signals: Signals{
				Now:                 now,
				LastInventoryAt:     now,
				ManuallySchedulable: true,
				ExecutionHealthy:    true,
			},
			connectionState:  ConnectionOffline,
			inventoryFresh:   true,
			executionHealthy: true,
		},
		{
			name: "manual disable wins",
			signals: Signals{
				Now:              now,
				LastHeartbeatAt:  now,
				LastInventoryAt:  now,
				ExecutionHealthy: true,
			},
			connectionState:  ConnectionConnected,
			connected:        true,
			inventoryFresh:   true,
			executionHealthy: true,
		},
		{
			name: "stale inventory blocks scheduling",
			signals: Signals{
				Now:                 now,
				LastHeartbeatAt:     now,
				LastInventoryAt:     now.Add(-DefaultDegradedAfter - time.Second),
				ManuallySchedulable: true,
				ExecutionHealthy:    true,
			},
			connectionState:  ConnectionConnected,
			connected:        true,
			executionHealthy: true,
		},
		{
			name: "unhealthy execution blocks scheduling",
			signals: Signals{
				Now:                 now,
				LastHeartbeatAt:     now,
				LastInventoryAt:     now,
				ManuallySchedulable: true,
			},
			connectionState: ConnectionConnected,
			connected:       true,
			inventoryFresh:  true,
		},
		{
			name: "fencing blocks scheduling",
			signals: Signals{
				Now:                 now,
				LastHeartbeatAt:     now,
				LastInventoryAt:     now,
				ManuallySchedulable: true,
				ExecutionHealthy:    true,
				Fenced:              true,
			},
			connectionState:  ConnectionConnected,
			connected:        true,
			inventoryFresh:   true,
			executionHealthy: true,
		},
		{
			name: "future reports clamp to zero age",
			signals: Signals{
				Now:                 now,
				LastHeartbeatAt:     now.Add(time.Second),
				LastInventoryAt:     now.Add(time.Second),
				ManuallySchedulable: true,
				ExecutionHealthy:    true,
			},
			connectionState:  ConnectionConnected,
			connected:        true,
			schedulable:      true,
			inventoryFresh:   true,
			executionHealthy: true,
		},
		{
			name: "degraded boundary remains connected",
			signals: Signals{
				Now:                 now,
				LastHeartbeatAt:     now.Add(-DefaultDegradedAfter),
				LastInventoryAt:     now.Add(-DefaultDegradedAfter),
				ManuallySchedulable: true,
				ExecutionHealthy:    true,
			},
			connectionState:  ConnectionConnected,
			connected:        true,
			schedulable:      true,
			inventoryFresh:   true,
			executionHealthy: true,
		},
		{
			name: "offline boundary remains degraded",
			signals: Signals{
				Now:                 now,
				LastHeartbeatAt:     now.Add(-DefaultOfflineAfter),
				LastInventoryAt:     now,
				ManuallySchedulable: true,
				ExecutionHealthy:    true,
			},
			connectionState:  ConnectionDegraded,
			inventoryFresh:   true,
			executionHealthy: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, err := Evaluate(test.signals, thresholds)
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}
			if status.ConnectionState != test.connectionState ||
				status.Connected != test.connected ||
				status.Schedulable != test.schedulable ||
				status.InventoryFresh != test.inventoryFresh ||
				status.ExecutionHealthy != test.executionHealthy {
				t.Fatalf("status = %#v", status)
			}
		})
	}
}

func TestEvaluateRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name       string
		signals    Signals
		thresholds Thresholds
		want       string
	}{
		{
			name:       "evaluation time",
			signals:    Signals{},
			thresholds: DefaultThresholds(),
			want:       "evaluation time",
		},
		{
			name:    "heartbeat interval",
			signals: Signals{Now: time.Now()},
			thresholds: Thresholds{
				HeartbeatInterval: 0,
				DegradedAfter:     time.Second,
				OfflineAfter:      2 * time.Second,
			},
			want: "heartbeat interval",
		},
		{
			name:    "degraded threshold",
			signals: Signals{Now: time.Now()},
			thresholds: Thresholds{
				HeartbeatInterval: time.Second,
				DegradedAfter:     time.Second,
				OfflineAfter:      2 * time.Second,
			},
			want: "degraded threshold",
		},
		{
			name:    "offline threshold",
			signals: Signals{Now: time.Now()},
			thresholds: Thresholds{
				HeartbeatInterval: time.Second,
				DegradedAfter:     2 * time.Second,
				OfflineAfter:      2 * time.Second,
			},
			want: "offline threshold",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Evaluate(test.signals, test.thresholds)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Evaluate() error = %v, want text %q", err, test.want)
			}
		})
	}
}
