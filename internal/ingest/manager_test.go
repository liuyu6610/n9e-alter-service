package ingest

import (
	"context"
	"errors"
	"testing"
	"time"

	"n9e-alter-service/internal/config"
	"n9e-alter-service/internal/n9e"
	"n9e-alter-service/internal/state"
)

func TestEnqueue_FailsClosedWhenEnabledWithoutStart(t *testing.T) {
	m := New(config.PushConfig{Enabled: true, QueueSize: 4, WorkerCount: 1, EnqueueTimeoutMilli: 50}, config.StateConfig{}, nil, state.New(), nil, nil)
	err := m.Enqueue(context.Background(), []n9e.CurEvent{{ID: 1, Hash: "h"}})
	if !errors.Is(err, ErrNoWorkers) {
		t.Fatalf("expected ErrNoWorkers, got %v", err)
	}
	if m.RunningWorkers() != 0 {
		t.Fatalf("workers=%d want 0", m.RunningWorkers())
	}
}

func TestHotEnable_StartsWorkersAndAcceptsEnqueue(t *testing.T) {
	st := state.New()
	m := New(config.PushConfig{Enabled: false, QueueSize: 8, WorkerCount: 1, EnqueueTimeoutMilli: 200}, config.StateConfig{}, nil, st, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)

	err := m.Enqueue(context.Background(), []n9e.CurEvent{{ID: 1, Hash: "h"}})
	if err == nil {
		t.Fatalf("expected disabled error before hot-enable")
	}
	if m.RunningWorkers() != 0 {
		t.Fatalf("workers=%d want 0 while disabled", m.RunningWorkers())
	}

	m.SetConfig(config.PushConfig{Enabled: true, QueueSize: 8, WorkerCount: 3, EnqueueTimeoutMilli: 200}, config.StateConfig{})
	if m.RunningWorkers() != 3 {
		t.Fatalf("workers=%d want 3 after raising worker_count", m.RunningWorkers())
	}
	err = m.Enqueue(context.Background(), []n9e.CurEvent{{ID: 1, Hash: "h"}})
	if err != nil {
		t.Fatalf("enqueue after hot-enable: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(m.queue) == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("queued batch was not consumed")
}

func TestStart_EnabledSpawnsConfiguredWorkers(t *testing.T) {
	m := New(config.PushConfig{Enabled: true, QueueSize: 4, WorkerCount: 2, EnqueueTimeoutMilli: 50}, config.StateConfig{}, nil, state.New(), nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)
	if m.RunningWorkers() != 2 {
		t.Fatalf("workers=%d want 2", m.RunningWorkers())
	}
	m.Start(ctx)
	if m.RunningWorkers() != 2 {
		t.Fatalf("second Start should be idempotent, workers=%d", m.RunningWorkers())
	}
}
