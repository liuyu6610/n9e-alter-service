package workers

import (
	"testing"
	"time"

	"n9e-alter-service/internal/config"
	"n9e-alter-service/internal/state"
)

func TestPickEscalation_SelectHighestAfterNotExceedingAge(t *testing.T) {
	now := time.Now().Unix()
	rec := state.Record{FirstSeenAt: now - 400}

	es := []config.EscalationConfig{
		{AfterSeconds: 60, RepeatIntervalSeconds: 3600},
		{AfterSeconds: 300, RepeatIntervalSeconds: 3600},
		{AfterSeconds: 600, RepeatIntervalSeconds: 3600},
	}

	idx, _, ok := pickEscalation(es, rec, now)
	if !ok {
		t.Fatalf("expected ok")
	}
	if idx != 1 {
		t.Fatalf("expected idx=1, got %d", idx)
	}
}

func TestPickEscalation_NotTriggeredWhenTooYoung(t *testing.T) {
	now := time.Now().Unix()
	rec := state.Record{FirstSeenAt: now - 10}

	es := []config.EscalationConfig{{AfterSeconds: 60}}
	_, _, ok := pickEscalation(es, rec, now)
	if ok {
		t.Fatalf("expected not ok")
	}
}
