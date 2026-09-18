package state

import (
	"testing"
	"time"
)

func sampleEvent(hash string) InputEvent {
	return InputEvent{
		ServiceHash: hash,
		RouteName:   "default",
		DedupKey:    "k-" + hash,
		RuleName:    "cpu",
		Entity:      "host-a",
		RawCount:    1,
	}
}

func TestApplyIngest_DoesNotRecoverMissing(t *testing.T) {
	st := New()
	now := time.Unix(1_700_000_000, 0)
	res := st.ApplyIngest(now, []InputEvent{sampleEvent("h1")})
	if res.ActiveTotal != 1 || len(res.NewActives) != 1 {
		t.Fatalf("ingest create: active=%d new=%d", res.ActiveTotal, len(res.NewActives))
	}

	res = st.ApplyIngest(now.Add(time.Minute), nil)
	rec, ok := st.Get("h1")
	if !ok {
		t.Fatal("expected record to remain")
	}
	if rec.Status != StatusActive {
		t.Fatalf("push ingest must not recover absences, status=%s miss=%d", rec.Status, rec.MissCount)
	}
	if rec.MissCount != 0 {
		t.Fatalf("ingest should reset/keep miss_count=0, got %d", rec.MissCount)
	}
	if len(res.NewRecovereds) != 0 {
		t.Fatalf("ingest should not emit recovereds, got %d", len(res.NewRecovereds))
	}
}

func TestApplyPull_RecoversOnlyAfterMissCount(t *testing.T) {
	st := New()
	now := time.Unix(1_700_000_000, 0)
	opt := ApplyOptions{RecoverMissCount: 2, RetainRecoveredSeconds: 86400}

	st.ApplyPull(now, []InputEvent{sampleEvent("h1")}, opt)

	res := st.ApplyPull(now.Add(time.Minute), nil, opt)
	rec, ok := st.Get("h1")
	if !ok {
		t.Fatal("missing after first miss")
	}
	if rec.Status != StatusActive || rec.MissCount != 1 {
		t.Fatalf("first miss should stay active, status=%s miss=%d", rec.Status, rec.MissCount)
	}
	if len(res.NewRecovereds) != 0 {
		t.Fatalf("recovered too early: %+v", res.NewRecovereds)
	}

	res = st.ApplyPull(now.Add(2*time.Minute), nil, opt)
	rec, ok = st.Get("h1")
	if !ok {
		t.Fatal("missing after recover")
	}
	if rec.Status != StatusRecovered {
		t.Fatalf("expected recovered, got %s", rec.Status)
	}
	if len(res.NewRecovereds) != 1 {
		t.Fatalf("expected 1 new recovered, got %d", len(res.NewRecovereds))
	}
}

func TestApplyPull_SuccessfulEmptyListRecovers(t *testing.T) {
	st := New()
	now := time.Unix(1_700_000_000, 0)
	opt := ApplyOptions{RecoverMissCount: 1, RetainRecoveredSeconds: 86400}
	st.ApplyPull(now, []InputEvent{sampleEvent("h1")}, opt)

	res := st.ApplyPull(now.Add(time.Minute), []InputEvent{}, opt)
	rec, ok := st.Get("h1")
	if !ok || rec.Status != StatusRecovered {
		t.Fatalf("empty successful pull should recover, ok=%v status=%v", ok, rec.Status)
	}
	if len(res.NewRecovereds) != 1 {
		t.Fatalf("expected recovered event, got %d", len(res.NewRecovereds))
	}
}

func TestApplyIngest_ReactivatesRecovered(t *testing.T) {
	st := New()
	now := time.Unix(1_700_000_000, 0)
	opt := ApplyOptions{RecoverMissCount: 1, RetainRecoveredSeconds: 86400}
	st.ApplyPull(now, []InputEvent{sampleEvent("h1")}, opt)
	st.ApplyPull(now.Add(time.Minute), nil, opt)

	res := st.ApplyIngest(now.Add(2*time.Minute), []InputEvent{sampleEvent("h1")})
	rec, ok := st.Get("h1")
	if !ok || rec.Status != StatusActive {
		t.Fatalf("ingest should reactivate, ok=%v status=%v", ok, rec.Status)
	}
	if len(res.NewActives) != 1 {
		t.Fatalf("expected new active, got %d", len(res.NewActives))
	}
}

func TestApplyPull_PurgesOldRecovered(t *testing.T) {
	st := New()
	now := time.Unix(1_700_000_000, 0)
	opt := ApplyOptions{RecoverMissCount: 1, RetainRecoveredSeconds: 60}
	st.ApplyPull(now, []InputEvent{sampleEvent("h1")}, opt)
	st.ApplyPull(now.Add(time.Second), nil, opt)

	res := st.ApplyPull(now.Add(2*time.Minute), nil, opt)
	if _, ok := st.Get("h1"); ok {
		t.Fatal("expected purged recovered record")
	}
	if res.PurgedRecovered != 1 {
		t.Fatalf("purged=%d", res.PurgedRecovered)
	}
}
