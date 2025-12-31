package state

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisSnapshotKey 返回用于存储快照的 redis key。
func RedisSnapshotKey(prefix string) string {
	p := strings.TrimSpace(prefix)
	if p == "" {
		p = "n9e_alter"
	}
	return p + ":state:snapshot"
}

// LoadFromRedis 从 redis 读取快照并恢复到内存状态。
// - key 不存在时视为无状态，返回 nil
// - 该函数仅做数据恢复，不会改变 snapshot 文件逻辑
func (s *Store) LoadFromRedis(ctx context.Context, rdb *redis.Client, key string) (bool, error) {
	if s == nil {
		return false, nil
	}
	if rdb == nil {
		return false, nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return false, nil
	}

	b, err := rdb.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return false, nil
		}
		return false, err
	}
	if len(b) == 0 {
		return false, nil
	}

	var snap Snapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return false, fmt.Errorf("unmarshal snapshot: %w", err)
	}

	s.mu.Lock()
	s.records = map[string]*Record{}
	for k, v := range snap.Records {
		vv := v
		s.records[k] = &vv
	}
	s.rev = 0
	s.dirty = false
	s.mu.Unlock()
	return true, nil
}

// SaveToRedis 将内存快照写入 redis，并设置 TTL。
// - 仅在 Store.dirty=true 时写入，避免频繁写 redis
// - 写入失败仅返回 error，上层可选择记录日志并继续运行
func (s *Store) SaveToRedis(ctx context.Context, rdb *redis.Client, key string, ttl time.Duration) error {
	if s == nil {
		return nil
	}
	if rdb == nil {
		return nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}

	s.mu.RLock()
	if !s.dirty {
		s.mu.RUnlock()
		return nil
	}
	rev := s.rev
	recs := make(map[string]Record, len(s.records))
	for k, v := range s.records {
		if v == nil {
			continue
		}
		recs[k] = *v
	}
	s.mu.RUnlock()

	snap := Snapshot{
		Version:   1,
		UpdatedAt: time.Now().Unix(),
		Records:   recs,
	}
	b, err := json.Marshal(snap)
	if err != nil {
		return err
	}

	pipe := rdb.Pipeline()
	pipe.Set(ctx, key, b, ttl)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	if s.rev == rev {
		s.dirty = false
	}
	s.mu.Unlock()
	return nil
}
