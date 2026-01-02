package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"n9e-alter-service/internal/config"
	"n9e-alter-service/internal/engine"
	"n9e-alter-service/internal/n9e"
	"n9e-alter-service/internal/redismgr"
	"n9e-alter-service/internal/state"
	"n9e-alter-service/internal/telemetry"
)

var ErrQueueFull = errors.New("ingest queue full")

type Manager struct {
	cfgV      atomic.Value
	stateCfgV atomic.Value
	eng       atomic.Value
	st        *state.Store
	rm        *redismgr.Manager
	stt       *telemetry.Stats

	queue chan []n9e.CurEvent
}

func New(cfg config.PushConfig, stateCfg config.StateConfig, eng *engine.Engine, st *state.Store, rm *redismgr.Manager, stats *telemetry.Stats) *Manager {
	qsize := cfg.QueueSize
	if qsize <= 0 {
		qsize = 20000
	}
	m := &Manager{st: st, rm: rm, stt: stats, queue: make(chan []n9e.CurEvent, qsize)}
	m.cfgV.Store(cfg)
	m.stateCfgV.Store(stateCfg)
	m.eng.Store(eng)
	return m
}

func (m *Manager) SetEngine(eng *engine.Engine) {
	if m == nil {
		return
	}
	m.eng.Store(eng)
}

func (m *Manager) getEngine() *engine.Engine {
	if m == nil {
		return nil
	}
	v := m.eng.Load()
	if v == nil {
		return nil
	}
	return v.(*engine.Engine)
}

func (m *Manager) Enabled() bool {
	if m == nil {
		return false
	}
	cfg, _ := m.cfgV.Load().(config.PushConfig)
	return cfg.Enabled
}

func (m *Manager) Token() string {
	if m == nil {
		return ""
	}
	cfg, _ := m.cfgV.Load().(config.PushConfig)
	return cfg.Token
}

func (m *Manager) SetConfig(cfg config.PushConfig, stateCfg config.StateConfig) {
	if m == nil {
		return
	}
	m.cfgV.Store(cfg)
	m.stateCfgV.Store(stateCfg)
}

func (m *Manager) Start(ctx context.Context) {
	if m == nil {
		return
	}
	cfg, _ := m.cfgV.Load().(config.PushConfig)
	if !cfg.Enabled {
		return
	}
	wc := cfg.WorkerCount
	if wc <= 0 {
		wc = 8
	}
	for i := 0; i < wc; i++ {
		go m.worker(ctx)
	}
}

func (m *Manager) Enqueue(ctx context.Context, batch []n9e.CurEvent) error {
	if m == nil {
		return fmt.Errorf("push ingest disabled")
	}
	cfg, _ := m.cfgV.Load().(config.PushConfig)
	if !cfg.Enabled {
		return fmt.Errorf("push ingest disabled")
	}
	if len(batch) == 0 {
		return nil
	}
	if m.stt != nil {
		m.stt.IncIngestBatches(1)
		m.stt.IncIngestEvents(uint64(len(batch)))
	}

	tmoMs := cfg.EnqueueTimeoutMilli
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
			eng := m.getEngine()
			if eng == nil {
				continue
			}
			inputs := eng.BuildInputEvents(batch)
			if m.stt != nil {
				m.stt.IncBuildInputs(uint64(len(inputs)))
			}
			if len(inputs) == 0 {
				continue
			}
			stateCfg, _ := m.stateCfgV.Load().(config.StateConfig)
			rdb := (*redis.Client)(nil)
			if m.rm != nil {
				rdb = m.rm.Get()
			}
			if rdb != nil && stateCfg.Redis.Enabled {
				m.aggregateBuckets(ctx, rdb, stateCfg, inputs)
				inputs = m.dedupGate(ctx, rdb, stateCfg, inputs)
				if len(inputs) == 0 {
					continue
				}
			}
			_ = m.st.ApplyIngest(time.Now(), inputs)
		}
	}
}

func (m *Manager) aggregateBuckets(ctx context.Context, rdb *redis.Client, stateCfg config.StateConfig, inputs []state.InputEvent) {
	if m == nil || rdb == nil || len(inputs) == 0 {
		return
	}
	if strings.TrimSpace(stateCfg.Redis.Addr) == "" {
		return
	}

	prefix := strings.TrimSpace(stateCfg.Redis.KeyPrefix)
	if prefix == "" {
		prefix = "n9e_alter"
	}

	// 热状态窗口（默认 1 天）：dedup/bucket 的 TTL。
	ttlSec := stateCfg.Redis.HotTTLSeconds
	if ttlSec <= 0 {
		ttlSec = 86400
	}
	ttl := time.Duration(ttlSec) * time.Second

	pipe := rdb.Pipeline()
	writes := 0
	for i := range inputs {
		it := inputs[i]
		if strings.TrimSpace(it.RouteName) == "" || strings.TrimSpace(it.DedupKey) == "" {
			continue
		}
		k := prefix + ":bucket:" + it.RouteName + ":" + it.DedupKey
		writes++
		inc := int64(it.RawCount)
		if inc <= 0 {
			inc = 1
		}
		pipe.HIncrBy(ctx, k, "raw_count", inc)
		if it.FirstTriggerTime > 0 {
			pipe.HSetNX(ctx, k, "first_trigger_time", it.FirstTriggerTime)
		}
		if it.LastTriggerTime > 0 {
			pipe.HSet(ctx, k, "last_trigger_time", it.LastTriggerTime)
		}
		pipe.HSet(ctx, k, "last_seen", time.Now().Unix())
		// 存一份样本（只存第一次），用于后续排查/预览。
		if len(it.Tags) > 0 {
			if b, err := json.Marshal(it.Tags); err == nil {
				pipe.HSetNX(ctx, k, "sample_tags", string(b))
			}
		}
		pipe.Expire(ctx, k, ttl)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		if m.stt != nil {
			m.stt.IncRedisBucketErrors(1)
		}
		return
	}
	if m.stt != nil && writes > 0 {
		m.stt.IncRedisBucketWrites(uint64(writes))
	}
}

func (m *Manager) dedupGate(ctx context.Context, rdb *redis.Client, stateCfg config.StateConfig, inputs []state.InputEvent) []state.InputEvent {
	if m == nil || rdb == nil || len(inputs) == 0 {
		return inputs
	}
	if strings.TrimSpace(stateCfg.Redis.Addr) == "" {
		return inputs
	}
	// 热状态窗口（默认 1 天）：dedup/bucket 的 TTL。
	ttlSec := stateCfg.Redis.HotTTLSeconds
	if ttlSec <= 0 {
		ttlSec = 86400
	}
	ttl := time.Duration(ttlSec) * time.Second

	prefix := strings.TrimSpace(stateCfg.Redis.KeyPrefix)
	if prefix == "" {
		prefix = "n9e_alter"
	}

	// 单副本下，SET NX EX 足够；redis 异常走 fail-open（整批放行），避免影响可用性。
	pipe := rdb.Pipeline()
	cmds := make([]*redis.BoolCmd, 0, len(inputs))
	idx := make([]int, 0, len(inputs))
	out := make([]state.InputEvent, 0, len(inputs))

	for i := range inputs {
		it := inputs[i]
		if strings.TrimSpace(it.RouteName) == "" || strings.TrimSpace(it.DedupKey) == "" {
			out = append(out, it)
			continue
		}
		k := prefix + ":dedup:" + it.RouteName + ":" + it.DedupKey
		cmds = append(cmds, pipe.SetNX(ctx, k, "1", ttl))
		idx = append(idx, i)
	}

	if len(cmds) == 0 {
		return out
	}
	if m.stt != nil {
		m.stt.IncRedisDedupChecks(uint64(len(cmds)))
	}
	if _, err := pipe.Exec(ctx); err != nil {
		// 整批 fail-open
		if m.stt != nil {
			m.stt.IncRedisDedupErrors(1)
		}
		return inputs
	}

	allowed := 0
	blocked := 0
	for j := range cmds {
		ok, err := cmds[j].Result()
		if err != nil {
			out = append(out, inputs[idx[j]])
			if m.stt != nil {
				m.stt.IncRedisDedupErrors(1)
			}
			continue
		}
		if ok {
			out = append(out, inputs[idx[j]])
			allowed++
		} else {
			blocked++
		}
	}
	if m.stt != nil {
		if allowed > 0 {
			m.stt.IncRedisDedupAllowed(uint64(allowed))
		}
		if blocked > 0 {
			m.stt.IncRedisDedupBlocked(uint64(blocked))
		}
	}
	return out
}
