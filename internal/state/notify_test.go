package state

import (
	"testing"
	"time"
)

func TestMarkChannelNotified_DoesNotMarkOtherChannels(t *testing.T) {
	s := New()
	now := time.Now()
	s.ApplyIngest(now, []InputEvent{{ServiceHash: "h1", RouteName: "default", DedupKey: "dk"}})

	ts := now.Unix()
	if !s.MarkChannelNotified("h1", ChannelWebhook, ts) {
		t.Fatalf("expected mark ok")
	}
	rec, ok := s.Get("h1")
	if !ok {
		t.Fatalf("missing record")
	}
	if rec.ChannelNotifiedAt(ChannelWebhook) != ts {
		t.Fatalf("webhook ts=%d want %d", rec.ChannelNotifiedAt(ChannelWebhook), ts)
	}
	if rec.ChannelNotifiedAt(ChannelRobot("r1")) != 0 {
		t.Fatalf("robot should still be due, got %d", rec.ChannelNotifiedAt(ChannelRobot("r1")))
	}
	if rec.LastNotified != ts {
		t.Fatalf("LastNotified=%d want %d", rec.LastNotified, ts)
	}
	if !rec.ChannelDue(ChannelRobot("r1"), ts+1, 3600, false) {
		t.Fatalf("failed robot channel should remain due")
	}
	if rec.ChannelDue(ChannelWebhook, ts+1, 3600, false) {
		t.Fatalf("successful webhook should not be due inside repeat window")
	}
}

func TestChannelDue_LegacyLastNotifiedAppliesToAllChannels(t *testing.T) {
	rec := Record{LastNotified: 1000}
	if rec.ChannelDue(ChannelWebhook, 1001, 3600, false) {
		t.Fatalf("legacy LastNotified should suppress all channels until repeat")
	}
	if rec.ChannelDue(ChannelRobot("r1"), 1001, 3600, false) {
		t.Fatalf("legacy LastNotified should suppress robot until repeat")
	}
	if !rec.ChannelDue(ChannelWebhook, 1000+3600, 3600, false) {
		t.Fatalf("legacy LastNotified should expire after repeat")
	}
}

func TestAnyChannelDue_PartialMapRetriesMissingDest(t *testing.T) {
	rec := Record{
		LastNotified:    50,
		ChannelNotified: map[string]int64{ChannelWebhook: 50},
	}
	dests := []string{ChannelWebhook, ChannelRobot("r1")}
	if !rec.AnyChannelDue(dests, 51, 3600, false) {
		t.Fatalf("missing robot dest should keep the event due")
	}
	if rec.ChannelDue(ChannelWebhook, 51, 3600, false) {
		t.Fatalf("webhook should not be re-sent")
	}
}

func TestResetRouteDaily_ClearsChannelMap(t *testing.T) {
	s := New()
	now := time.Now()
	s.ApplyIngest(now, []InputEvent{{ServiceHash: "h1", RouteName: "default", DedupKey: "dk"}})
	s.MarkChannelNotified("h1", ChannelWebhook, now.Unix())
	if n := s.ResetRouteDaily("default"); n != 1 {
		t.Fatalf("reset changed=%d want 1", n)
	}
	rec, _ := s.Get("h1")
	if rec.LastNotified != 0 || len(rec.ChannelNotified) != 0 {
		t.Fatalf("expected cleared notify state, got last=%d map=%v", rec.LastNotified, rec.ChannelNotified)
	}
}

func TestRecoveredReactivation_ClearsChannelMap(t *testing.T) {
	s := New()
	now := time.Now()
	s.ApplyIngest(now, []InputEvent{{ServiceHash: "h1", RouteName: "default", DedupKey: "dk"}})
	s.MarkChannelNotified("h1", ChannelWebhook, now.Unix())
	s.ApplyPull(now.Add(time.Second), nil, ApplyOptions{RecoverMissCount: 1, RetainRecoveredSeconds: 86400})
	rec, _ := s.Get("h1")
	if rec.Status != StatusRecovered {
		t.Fatalf("status=%s want recovered", rec.Status)
	}
	if rec.LastNotified != 0 || len(rec.ChannelNotified) != 0 {
		t.Fatalf("recovered should clear notify state, last=%d map=%v", rec.LastNotified, rec.ChannelNotified)
	}

	s.ApplyIngest(now.Add(2*time.Second), []InputEvent{{ServiceHash: "h1", RouteName: "default", DedupKey: "dk"}})
	rec, _ = s.Get("h1")
	if rec.Status != StatusActive {
		t.Fatalf("status=%s want active", rec.Status)
	}
	if rec.LastNotified != 0 || len(rec.ChannelNotified) != 0 {
		t.Fatalf("reactivated should clear notify state")
	}
}
