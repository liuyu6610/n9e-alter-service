package telemetry

import (
	"sync/atomic"
	"time"
)

type Stats struct {
	StartedAtUnix int64

	IngestBatches uint64
	IngestEvents  uint64
	BuildInputs   uint64

	RedisBucketWrites uint64
	RedisBucketErrors uint64

	RedisDedupChecks  uint64
	RedisDedupAllowed uint64
	RedisDedupBlocked uint64
	RedisDedupErrors  uint64

	NotifySendOK    uint64
	NotifySendError uint64

	HTTPRequests        uint64
	HTTP4xx             uint64
	HTTP5xx             uint64
	HTTPDurationMsTotal uint64

	BucketReset uint64
	BucketDel   uint64
}

func New() *Stats {
	return &Stats{StartedAtUnix: time.Now().Unix()}
}

func (s *Stats) IncIngestBatches(n uint64) { atomic.AddUint64(&s.IngestBatches, n) }
func (s *Stats) IncIngestEvents(n uint64)  { atomic.AddUint64(&s.IngestEvents, n) }
func (s *Stats) IncBuildInputs(n uint64)   { atomic.AddUint64(&s.BuildInputs, n) }

func (s *Stats) IncRedisBucketWrites(n uint64) { atomic.AddUint64(&s.RedisBucketWrites, n) }
func (s *Stats) IncRedisBucketErrors(n uint64) { atomic.AddUint64(&s.RedisBucketErrors, n) }

func (s *Stats) IncRedisDedupChecks(n uint64)  { atomic.AddUint64(&s.RedisDedupChecks, n) }
func (s *Stats) IncRedisDedupAllowed(n uint64) { atomic.AddUint64(&s.RedisDedupAllowed, n) }
func (s *Stats) IncRedisDedupBlocked(n uint64) { atomic.AddUint64(&s.RedisDedupBlocked, n) }
func (s *Stats) IncRedisDedupErrors(n uint64)  { atomic.AddUint64(&s.RedisDedupErrors, n) }

func (s *Stats) IncNotifySendOK(n uint64)    { atomic.AddUint64(&s.NotifySendOK, n) }
func (s *Stats) IncNotifySendError(n uint64) { atomic.AddUint64(&s.NotifySendError, n) }

func (s *Stats) ObserveHTTP(status int, dur time.Duration) {
	if s == nil {
		return
	}
	atomic.AddUint64(&s.HTTPRequests, 1)
	if status >= 500 {
		atomic.AddUint64(&s.HTTP5xx, 1)
	} else if status >= 400 {
		atomic.AddUint64(&s.HTTP4xx, 1)
	}
	ms := dur.Milliseconds()
	if ms < 0 {
		ms = 0
	}
	atomic.AddUint64(&s.HTTPDurationMsTotal, uint64(ms))
}

func (s *Stats) IncBucketReset(n uint64) { atomic.AddUint64(&s.BucketReset, n) }
func (s *Stats) IncBucketDel(n uint64)   { atomic.AddUint64(&s.BucketDel, n) }

type Snapshot struct {
	Time                int64  `json:"time"`
	StartedAtUnix       int64  `json:"started_at_unix"`
	UptimeSeconds       int64  `json:"uptime_seconds"`
	IngestBatches       uint64 `json:"ingest_batches"`
	IngestEvents        uint64 `json:"ingest_events"`
	BuildInputs         uint64 `json:"build_inputs"`
	RedisBucketWrites   uint64 `json:"redis_bucket_writes"`
	RedisBucketErrors   uint64 `json:"redis_bucket_errors"`
	RedisDedupChecks    uint64 `json:"redis_dedup_checks"`
	RedisDedupAllowed   uint64 `json:"redis_dedup_allowed"`
	RedisDedupBlocked   uint64 `json:"redis_dedup_blocked"`
	RedisDedupErrors    uint64 `json:"redis_dedup_errors"`
	NotifySendOK        uint64 `json:"notify_send_ok"`
	NotifySendError     uint64 `json:"notify_send_error"`
	HTTPRequests        uint64 `json:"http_requests"`
	HTTP4xx             uint64 `json:"http_4xx"`
	HTTP5xx             uint64 `json:"http_5xx"`
	HTTPDurationMsTotal uint64 `json:"http_duration_ms_total"`
	BucketReset         uint64 `json:"bucket_reset"`
	BucketDel           uint64 `json:"bucket_del"`
}

func (s *Stats) Snapshot() Snapshot {
	if s == nil {
		now := time.Now().Unix()
		return Snapshot{Time: now, StartedAtUnix: now, UptimeSeconds: 0}
	}
	now := time.Now().Unix()
	started := atomic.LoadInt64(&s.StartedAtUnix)
	up := now - started
	if up < 0 {
		up = 0
	}
	return Snapshot{
		Time:                now,
		StartedAtUnix:       started,
		UptimeSeconds:       up,
		IngestBatches:       atomic.LoadUint64(&s.IngestBatches),
		IngestEvents:        atomic.LoadUint64(&s.IngestEvents),
		BuildInputs:         atomic.LoadUint64(&s.BuildInputs),
		RedisBucketWrites:   atomic.LoadUint64(&s.RedisBucketWrites),
		RedisBucketErrors:   atomic.LoadUint64(&s.RedisBucketErrors),
		RedisDedupChecks:    atomic.LoadUint64(&s.RedisDedupChecks),
		RedisDedupAllowed:   atomic.LoadUint64(&s.RedisDedupAllowed),
		RedisDedupBlocked:   atomic.LoadUint64(&s.RedisDedupBlocked),
		RedisDedupErrors:    atomic.LoadUint64(&s.RedisDedupErrors),
		NotifySendOK:        atomic.LoadUint64(&s.NotifySendOK),
		NotifySendError:     atomic.LoadUint64(&s.NotifySendError),
		HTTPRequests:        atomic.LoadUint64(&s.HTTPRequests),
		HTTP4xx:             atomic.LoadUint64(&s.HTTP4xx),
		HTTP5xx:             atomic.LoadUint64(&s.HTTP5xx),
		HTTPDurationMsTotal: atomic.LoadUint64(&s.HTTPDurationMsTotal),
		BucketReset:         atomic.LoadUint64(&s.BucketReset),
		BucketDel:           atomic.LoadUint64(&s.BucketDel),
	}
}
