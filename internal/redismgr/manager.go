package redismgr

import (
	"strings"
	"sync/atomic"

	"github.com/redis/go-redis/v9"

	"n9e-alter-service/internal/config"
)

type Manager struct {
	cur atomic.Value
	cfg atomic.Value
}

type cfgKey struct {
	enabled  bool
	addr     string
	password string
	db       int
}

func New(initial config.RedisConfig) *Manager {
	m := &Manager{}
	m.Apply(initial)
	return m
}

func (m *Manager) Get() *redis.Client {
	if m == nil {
		return nil
	}
	v := m.cur.Load()
	if v == nil {
		return nil
	}
	if v == (*redis.Client)(nil) {
		return nil
	}
	return v.(*redis.Client)
}

func (m *Manager) Close() {
	if m == nil {
		return
	}
	old := m.swap(nil, cfgKey{})
	if old != nil {
		_ = old.Close()
	}
}

func (m *Manager) Apply(rc config.RedisConfig) {
	if m == nil {
		return
	}

	key := cfgKey{
		enabled:  rc.Enabled,
		addr:     strings.TrimSpace(rc.Addr),
		password: rc.Password,
		db:       rc.DB,
	}

	if !key.enabled || key.addr == "" {
		old := m.swap(nil, key)
		if old != nil {
			_ = old.Close()
		}
		return
	}

	prev, _ := m.cfg.Load().(cfgKey)
	if prev == key {
		return
	}

	cli := redis.NewClient(&redis.Options{Addr: key.addr, Password: key.password, DB: key.db})
	old := m.swap(cli, key)
	if old != nil {
		_ = old.Close()
	}
}

func (m *Manager) swap(next *redis.Client, key cfgKey) *redis.Client {
	var old *redis.Client
	if v := m.cur.Load(); v != nil {
		old = v.(*redis.Client)
	}
	m.cfg.Store(key)
	m.cur.Store(next)
	return old
}
