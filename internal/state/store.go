package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Status string

const (
	StatusActive    Status = "active"
	StatusRecovered Status = "recovered"
)

type Record struct {
	ServiceHash string `json:"service_hash"`
	RouteName   string `json:"route_name"`
	DedupKey    string `json:"dedup_key"`

	Status      Status `json:"status"`
	FirstSeenAt int64  `json:"first_seen_at"`
	LastSeenAt  int64  `json:"last_seen_at"`

	MissCount    int   `json:"miss_count"`
	RecoveredAt  int64 `json:"recovered_at"`
	LastNotified int64 `json:"last_notified"`

	N9EHash string `json:"n9e_hash"`
	N9EID   int64  `json:"n9e_id"`

	GroupID   int64  `json:"group_id"`
	GroupName string `json:"group_name"`
	RuleID    int64  `json:"rule_id"`
	RuleName  string `json:"rule_name"`
	Severity  int    `json:"severity"`
	Entity    string `json:"entity"`

	FirstTriggerTime int64 `json:"first_trigger_time"`
	LastTriggerTime  int64 `json:"last_trigger_time"`
	RawCount         int   `json:"raw_count"`
}

type InputEvent struct {
	ServiceHash string
	RouteName   string
	DedupKey    string

	N9EHash string
	N9EID   int64

	GroupID   int64
	GroupName string
	RuleID    int64
	RuleName  string
	Severity  int
	Entity    string

	FirstTriggerTime int64
	LastTriggerTime  int64
	RawCount         int
}

type Snapshot struct {
	Version   int               `json:"version"`
	UpdatedAt int64             `json:"updated_at"`
	Records   map[string]Record `json:"records"`
}

type ApplyOptions struct {
	RecoverMissCount       int
	RetainRecoveredSeconds int
}

type ApplyResult struct {
	NowUnix         int64
	Seen            int
	ActiveTotal     int
	RecoveredTotal  int
	NewActives      []Record
	NewRecovereds   []Record
	PurgedRecovered int
}

type Store struct {
	mu      sync.RWMutex
	records map[string]*Record
	rev     uint64
	dirty   bool
}

func New() *Store {
	return &Store{records: map[string]*Record{}}
}

func (s *Store) LoadFromFile(path string) error {
	if stringsTrim(path) == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var snap Snapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = map[string]*Record{}
	for k, v := range snap.Records {
		vv := v
		s.records[k] = &vv
	}
	s.rev = 0
	s.dirty = false
	return nil
}

func (s *Store) SaveToFile(path string) error {
	if stringsTrim(path) == "" {
		return nil
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
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}

	s.mu.Lock()
	if s.rev == rev {
		s.dirty = false
	}
	s.mu.Unlock()
	return nil
}

func (s *Store) ApplyPull(now time.Time, items []InputEvent, opt ApplyOptions) ApplyResult {
	nowUnix := now.Unix()

	recoverMiss := opt.RecoverMissCount
	if recoverMiss <= 0 {
		recoverMiss = 1
	}

	retainRecovered := opt.RetainRecoveredSeconds
	if retainRecovered <= 0 {
		retainRecovered = 86400
	}

	seen := make(map[string]InputEvent, len(items))
	for _, it := range items {
		if stringsTrim(it.ServiceHash) == "" {
			continue
		}
		seen[it.ServiceHash] = it
	}

	res := ApplyResult{NowUnix: nowUnix, Seen: len(seen)}

	s.mu.Lock()
	defer s.mu.Unlock()

	for h, it := range seen {
		r, ok := s.records[h]
		if !ok || r == nil {
			nr := &Record{
				ServiceHash:      h,
				RouteName:        it.RouteName,
				DedupKey:         it.DedupKey,
				Status:           StatusActive,
				FirstSeenAt:      nowUnix,
				LastSeenAt:       nowUnix,
				MissCount:        0,
				RecoveredAt:      0,
				LastNotified:     0,
				N9EHash:          it.N9EHash,
				N9EID:            it.N9EID,
				GroupID:          it.GroupID,
				GroupName:        it.GroupName,
				RuleID:           it.RuleID,
				RuleName:         it.RuleName,
				Severity:         it.Severity,
				Entity:           it.Entity,
				FirstTriggerTime: it.FirstTriggerTime,
				LastTriggerTime:  it.LastTriggerTime,
				RawCount:         it.RawCount,
			}
			s.records[h] = nr
			res.NewActives = append(res.NewActives, *nr)
			s.markDirtyLocked()
			continue
		}

		if r.Status == StatusRecovered {
			r.Status = StatusActive
			r.FirstSeenAt = nowUnix
			r.RecoveredAt = 0
			r.LastNotified = 0
			r.MissCount = 0
			res.NewActives = append(res.NewActives, *r)
			s.markDirtyLocked()
		}

		r.RouteName = it.RouteName
		r.DedupKey = it.DedupKey
		r.LastSeenAt = nowUnix
		r.MissCount = 0

		r.N9EHash = it.N9EHash
		r.N9EID = it.N9EID
		r.GroupID = it.GroupID
		r.GroupName = it.GroupName
		r.RuleID = it.RuleID
		r.RuleName = it.RuleName
		r.Severity = it.Severity
		r.Entity = it.Entity

		r.RawCount = it.RawCount
		if r.FirstTriggerTime == 0 || (it.FirstTriggerTime > 0 && it.FirstTriggerTime < r.FirstTriggerTime) {
			r.FirstTriggerTime = it.FirstTriggerTime
		}
		if it.LastTriggerTime > r.LastTriggerTime {
			r.LastTriggerTime = it.LastTriggerTime
		}

		s.markDirtyLocked()
	}

	cutoff := nowUnix - int64(retainRecovered)
	for h, r := range s.records {
		if r == nil {
			delete(s.records, h)
			s.markDirtyLocked()
			continue
		}

		_, ok := seen[h]
		if ok {
			continue
		}

		if r.Status == StatusActive {
			r.MissCount++
			if r.MissCount >= recoverMiss {
				r.Status = StatusRecovered
				r.RecoveredAt = nowUnix
				r.LastNotified = 0
				res.NewRecovereds = append(res.NewRecovereds, *r)
			}
			s.markDirtyLocked()
			continue
		}

		if r.Status == StatusRecovered {
			if r.RecoveredAt > 0 && r.RecoveredAt < cutoff {
				delete(s.records, h)
				res.PurgedRecovered++
				s.markDirtyLocked()
			}
		}
	}

	active := 0
	recovered := 0
	for _, r := range s.records {
		if r == nil {
			continue
		}
		if r.Status == StatusActive {
			active++
		} else if r.Status == StatusRecovered {
			recovered++
		}
	}

	res.ActiveTotal = active
	res.RecoveredTotal = recovered
	return res
}

func (s *Store) Summary() (active int, recovered int, total int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.records {
		if r == nil {
			continue
		}
		total++
		if r.Status == StatusActive {
			active++
		} else if r.Status == StatusRecovered {
			recovered++
		}
	}
	return
}

func (s *Store) Get(serviceHash string) (Record, bool) {
	serviceHash = stringsTrim(serviceHash)
	if serviceHash == "" {
		return Record{}, false
	}
	s.mu.RLock()
	r, ok := s.records[serviceHash]
	s.mu.RUnlock()
	if !ok || r == nil {
		return Record{}, false
	}
	return *r, true
}

func (s *Store) List(status Status, routeName string, offset int, limit int) ([]Record, int) {
	routeName = stringsTrim(routeName)
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 100000 {
		limit = 100000
	}

	s.mu.RLock()
	items := make([]Record, 0, len(s.records))
	for _, r := range s.records {
		if r == nil {
			continue
		}
		if status != "" && r.Status != status {
			continue
		}
		if routeName != "" && r.RouteName != routeName {
			continue
		}
		items = append(items, *r)
	}
	s.mu.RUnlock()

	sort.Slice(items, func(i, j int) bool {
		a := items[i]
		b := items[j]
		if a.Severity != b.Severity {
			return a.Severity < b.Severity
		}
		if a.RawCount != b.RawCount {
			return a.RawCount > b.RawCount
		}
		if a.LastTriggerTime != b.LastTriggerTime {
			return a.LastTriggerTime > b.LastTriggerTime
		}
		if a.RuleName != b.RuleName {
			return a.RuleName < b.RuleName
		}
		return a.Entity < b.Entity
	})

	total := len(items)
	if offset >= total {
		return []Record{}, total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return items[offset:end], total
}

func (s *Store) MarkNotified(serviceHash string, ts int64) bool {
	serviceHash = stringsTrim(serviceHash)
	if serviceHash == "" {
		return false
	}
	s.mu.Lock()
	r, ok := s.records[serviceHash]
	if !ok || r == nil {
		s.mu.Unlock()
		return false
	}
	r.LastNotified = ts
	s.markDirtyLocked()
	s.mu.Unlock()
	return true
}

func (s *Store) ResetRouteDaily(routeName string) int {
	routeName = stringsTrim(routeName)
	if routeName == "" {
		return 0
	}

	changed := 0
	s.mu.Lock()
	for _, r := range s.records {
		if r == nil {
			continue
		}
		if r.RouteName != routeName {
			continue
		}
		if r.Status != StatusActive {
			continue
		}
		if r.LastNotified != 0 {
			r.LastNotified = 0
			changed++
			s.markDirtyLocked()
		}
	}
	s.mu.Unlock()
	return changed
}

func (s *Store) PickNotifyActive(routeName string, nowUnix int64, observeSeconds int, repeatIntervalSeconds int, limit int) []Record {
	routeName = stringsTrim(routeName)
	if observeSeconds < 0 {
		observeSeconds = 0
	}
	if repeatIntervalSeconds <= 0 {
		repeatIntervalSeconds = 3600
	}
	if limit <= 0 {
		limit = 1000
	}

	items := make([]Record, 0, 64)
	s.mu.RLock()
	for _, r := range s.records {
		if r == nil {
			continue
		}
		if r.Status != StatusActive {
			continue
		}
		if routeName != "" && r.RouteName != routeName {
			continue
		}
		if observeSeconds > 0 && nowUnix-r.FirstSeenAt < int64(observeSeconds) {
			continue
		}
		if r.LastNotified > 0 && nowUnix-r.LastNotified < int64(repeatIntervalSeconds) {
			continue
		}
		items = append(items, *r)
		if len(items) >= limit {
			break
		}
	}
	s.mu.RUnlock()
	return items
}

func (s *Store) ClearRoute(routeName string) int {
	routeName = stringsTrim(routeName)
	if routeName == "" {
		return 0
	}

	removed := 0
	s.mu.Lock()
	for h, r := range s.records {
		if r == nil {
			delete(s.records, h)
			removed++
			continue
		}
		if r.RouteName == routeName {
			delete(s.records, h)
			removed++
		}
	}
	if removed > 0 {
		s.dirty = true
		s.rev++
	}
	s.mu.Unlock()
	return removed
}

func (s *Store) ClearAll() int {
	removed := 0
	s.mu.Lock()
	removed = len(s.records)
	s.records = map[string]*Record{}
	if removed > 0 {
		s.dirty = true
		s.rev++
	}
	s.mu.Unlock()
	return removed
}

func (s *Store) PickNotifyRecovered(routeName string, nowUnix int64, limit int) []Record {
	routeName = stringsTrim(routeName)
	if limit <= 0 {
		limit = 1000
	}

	items := make([]Record, 0, 64)
	s.mu.RLock()
	for _, r := range s.records {
		if r == nil {
			continue
		}
		if r.Status != StatusRecovered {
			continue
		}
		if routeName != "" && r.RouteName != routeName {
			continue
		}
		if r.LastNotified != 0 {
			continue
		}
		items = append(items, *r)
		if len(items) >= limit {
			break
		}
	}
	s.mu.RUnlock()
	return items
}

func stringsTrim(s string) string {
	return strings.TrimSpace(s)
}

func (s *Store) markDirtyLocked() {
	s.dirty = true
	s.rev++
}
