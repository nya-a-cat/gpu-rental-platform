package clusterstate

import (
	"errors"
	"time"
)

const (
	DefaultHeartbeatInterval = 15 * time.Second
	DefaultDegradedAfter     = 45 * time.Second
	DefaultOfflineAfter      = 90 * time.Second
)

type Thresholds struct {
	HeartbeatInterval time.Duration
	DegradedAfter     time.Duration
	OfflineAfter      time.Duration
}

func DefaultThresholds() Thresholds {
	return Thresholds{
		HeartbeatInterval: DefaultHeartbeatInterval,
		DegradedAfter:     DefaultDegradedAfter,
		OfflineAfter:      DefaultOfflineAfter,
	}
}

func (thresholds Thresholds) Validate() error {
	if thresholds.HeartbeatInterval <= 0 {
		return errors.New("agent heartbeat interval must be greater than zero")
	}
	if thresholds.DegradedAfter <= thresholds.HeartbeatInterval {
		return errors.New("agent degraded threshold must be greater than heartbeat interval")
	}
	if thresholds.OfflineAfter <= thresholds.DegradedAfter {
		return errors.New("agent offline threshold must be greater than degraded threshold")
	}
	return nil
}

type ConnectionState string

const (
	ConnectionConnected ConnectionState = "connected"
	ConnectionDegraded  ConnectionState = "degraded"
	ConnectionOffline   ConnectionState = "offline"
)

type Signals struct {
	Now                 time.Time
	LastHeartbeatAt     time.Time
	LastInventoryAt     time.Time
	ManuallySchedulable bool
	ExecutionHealthy    bool
	Fenced              bool
}

type Status struct {
	ConnectionState  ConnectionState
	Connected        bool
	Schedulable      bool
	InventoryFresh   bool
	ExecutionHealthy bool
}

func Evaluate(signals Signals, thresholds Thresholds) (Status, error) {
	if err := thresholds.Validate(); err != nil {
		return Status{}, err
	}
	if signals.Now.IsZero() {
		return Status{}, errors.New("evaluation time is required")
	}

	connectionState := connectionState(
		elapsed(signals.Now, signals.LastHeartbeatAt),
		signals.LastHeartbeatAt.IsZero(),
		thresholds,
	)
	inventoryFresh := !signals.LastInventoryAt.IsZero() &&
		elapsed(signals.Now, signals.LastInventoryAt) <= thresholds.DegradedAfter
	connected := connectionState == ConnectionConnected

	return Status{
		ConnectionState:  connectionState,
		Connected:        connected,
		Schedulable:      signals.ManuallySchedulable && connected && inventoryFresh && signals.ExecutionHealthy && !signals.Fenced,
		InventoryFresh:   inventoryFresh,
		ExecutionHealthy: signals.ExecutionHealthy,
	}, nil
}

func connectionState(age time.Duration, missing bool, thresholds Thresholds) ConnectionState {
	if missing || age > thresholds.OfflineAfter {
		return ConnectionOffline
	}
	if age > thresholds.DegradedAfter {
		return ConnectionDegraded
	}
	return ConnectionConnected
}

func elapsed(now, observedAt time.Time) time.Duration {
	age := now.Sub(observedAt)
	if age < 0 {
		return 0
	}
	return age
}
