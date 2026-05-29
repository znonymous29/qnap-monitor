package alert

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/qnap-monitor/backend/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestAlertConsolidation(t *testing.T) {
	s := newStore(t)
	m := NewManager(s, 55.0, 55.0, 75.0, "")

	ctx := context.Background()

	// below threshold — no alert
	m.Evaluate(ctx, 50)
	if ev := m.ConsumeLastEvent(); ev != nil {
		t.Fatalf("expected no event below threshold, got %+v", ev)
	}

	// crosses up — creates alert
	m.Evaluate(ctx, 56)
	ev := m.ConsumeLastEvent()
	if ev == nil || ev.Type != TypeTempHigh {
		t.Fatalf("expected temperature_high event, got %+v", ev)
	}
	alertID := ev.ID

	// still above — updates peak, no new event
	m.Evaluate(ctx, 60)
	if ev := m.ConsumeLastEvent(); ev != nil {
		t.Fatalf("expected no new event while still above, got %+v", ev)
	}

	// drops back — closes the alert (no new event for recovery)
	m.Evaluate(ctx, 50)
	if ev := m.ConsumeLastEvent(); ev != nil {
		t.Fatalf("expected no event on recovery (consolidated), got %+v", ev)
	}

	// Verify: exactly 1 row, closed, with peak = 60
	rows, err := s.ListAlerts(ctx, 10)
	if err != nil {
		t.Fatalf("list alerts: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 consolidated alert row, got %d", len(rows))
	}
	a := rows[0]
	if a.ID != alertID {
		t.Fatalf("expected alert ID %d, got %d", alertID, a.ID)
	}
	if a.EndTS == nil {
		t.Fatal("expected alert to be closed (endTs != nil)")
	}
	if a.PeakValue == nil || *a.PeakValue != 60 {
		t.Fatalf("expected peak 60, got %v", a.PeakValue)
	}

	// Goes over again — creates a new alert
	m.Evaluate(ctx, 58)
	ev = m.ConsumeLastEvent()
	if ev == nil || ev.Type != TypeTempHigh {
		t.Fatalf("expected new temperature_high event, got %+v", ev)
	}

	rows, err = s.ListAlerts(ctx, 10)
	if err != nil {
		t.Fatalf("list alerts: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 alert rows, got %d", len(rows))
	}
}

func TestDiskTempConsolidation(t *testing.T) {
	s := newStore(t)
	m := NewManager(s, 55.0, 50.0, 75.0, "")
	ctx := context.Background()

	// Disk below threshold
	aliases := map[string]string{"0:1": "Test Disk 1"}
	m.EvaluateDiskTemps(ctx, map[string]int{"0:1": 40}, aliases)
	if ev := m.ConsumeLastEvent(); ev != nil {
		t.Fatalf("expected no event, got %+v", ev)
	}

	// Disk crosses threshold
	m.EvaluateDiskTemps(ctx, map[string]int{"0:1": 52}, aliases)
	ev := m.ConsumeLastEvent()
	if ev == nil || ev.Type != TypeDiskTempHigh || ev.HDNo != "0:1" {
		t.Fatalf("expected disk_temperature_high for 0:1, got %+v", ev)
	}

	// Still over — no new event
	m.EvaluateDiskTemps(ctx, map[string]int{"0:1": 55}, aliases)
	if ev := m.ConsumeLastEvent(); ev != nil {
		t.Fatalf("expected no new event, got %+v", ev)
	}

	// Recovered
	m.EvaluateDiskTemps(ctx, map[string]int{"0:1": 45}, aliases)
	if ev := m.ConsumeLastEvent(); ev != nil {
		t.Fatalf("expected no event on recovery, got %+v", ev)
	}

	rows, err := s.ListAlerts(ctx, 10)
	if err != nil {
		t.Fatalf("list alerts: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].HDNo != "0:1" {
		t.Fatalf("expected hdNo 0:1, got %s", rows[0].HDNo)
	}
	if rows[0].EndTS == nil {
		t.Fatal("expected closed alert")
	}
}

func TestDiskHealthConsolidation(t *testing.T) {
	s := newStore(t)
	m := NewManager(s, 55.0, 55.0, 75.0, "")
	ctx := context.Background()

	aliases := map[string]string{"0:1": "Test Disk 1"}
	// All OK
	m.EvaluateDiskHealth(ctx, map[string]string{"0:1": "OK"}, aliases)
	if ev := m.ConsumeLastEvent(); ev != nil {
		t.Fatalf("expected no event, got %+v", ev)
	}

	// Disk becomes bad
	m.EvaluateDiskHealth(ctx, map[string]string{"0:1": "Warning"}, aliases)
	ev := m.ConsumeLastEvent()
	if ev == nil || ev.Type != TypeDiskHealth || ev.HDNo != "0:1" {
		t.Fatalf("expected disk_health_warning for 0:1, got %+v", ev)
	}

	// Still bad — no new event
	m.EvaluateDiskHealth(ctx, map[string]string{"0:1": "Warning"}, aliases)
	if ev := m.ConsumeLastEvent(); ev != nil {
		t.Fatalf("expected no new event, got %+v", ev)
	}

	// Recovered
	m.EvaluateDiskHealth(ctx, map[string]string{"0:1": "OK"}, aliases)
	if ev := m.ConsumeLastEvent(); ev != nil {
		t.Fatalf("expected no event on recovery, got %+v", ev)
	}

	rows, err := s.ListAlerts(ctx, 10)
	if err != nil {
		t.Fatalf("list alerts: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].EndTS == nil {
		t.Fatal("expected closed alert")
	}
}
