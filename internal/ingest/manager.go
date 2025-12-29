package ingest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"n9e-alter-service/internal/config"
	"n9e-alter-service/internal/engine"
	"n9e-alter-service/internal/n9e"
	"n9e-alter-service/internal/state"
)

var ErrQueueFull = errors.New("ingest queue full")

type Manager struct {
	cfg config.PushConfig
	eng *engine.Engine
	st  *state.Store

	queue chan []n9e.CurEvent
}

func New(cfg config.PushConfig, eng *engine.Engine, st *state.Store) *Manager {
	qsize := cfg.QueueSize
	if qsize <= 0 {
		qsize = 20000
	}
	return &Manager{cfg: cfg, eng: eng, st: st, queue: make(chan []n9e.CurEvent, qsize)}
}

func (m *Manager) Enabled() bool {
	return m != nil && m.cfg.Enabled
}

func (m *Manager) Token() string {
	if m == nil {
		return ""
	}
	return m.cfg.Token
}

func (m *Manager) Start(ctx context.Context) {
	if m == nil || !m.cfg.Enabled {
		return
	}
	wc := m.cfg.WorkerCount
	if wc <= 0 {
		wc = 8
	}
	for i := 0; i < wc; i++ {
		go m.worker(ctx)
	}
}

func (m *Manager) Enqueue(ctx context.Context, batch []n9e.CurEvent) error {
	if m == nil || !m.cfg.Enabled {
		return fmt.Errorf("push ingest disabled")
	}
	if len(batch) == 0 {
		return nil
	}

	tmoMs := m.cfg.EnqueueTimeoutMilli
	if tmoMs < 0 {
		tmoMs = 0
	}
	if tmoMs == 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case m.queue <- batch:
			return nil
		}
	}

	tmo := time.Duration(tmoMs) * time.Millisecond
	t := time.NewTimer(tmo)
	defer func() {
		if !t.Stop() {
			select {
			case <-t.C:
			default:
			}
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return ErrQueueFull
	case m.queue <- batch:
		return nil
	}
}

func (m *Manager) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case batch := <-m.queue:
			if len(batch) == 0 {
				continue
			}
			inputs := m.eng.BuildInputEvents(batch)
			if len(inputs) == 0 {
				continue
			}
			_ = m.st.ApplyIngest(time.Now(), inputs)
		}
	}
}
