package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestInsertAndPurge(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	now := time.Now().Unix()
	old := now - int64(40*24*3600) // 40 days ago — should be purged at retention=30
	recent := now - 60             // 1 minute ago — should be kept

	if err := s.InsertMetric(ctx, &Snapshot{TS: old, CPUUsage: 10, SysTempC: 40, VolumeTotalBytes: 100, VolumeUsedBytes: 20}); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertMetric(ctx, &Snapshot{TS: recent, CPUUsage: 20, SysTempC: 45, VolumeTotalBytes: 100, VolumeUsedBytes: 30}); err != nil {
		t.Fatal(err)
	}

	n, err := s.PurgeOldMetrics(ctx, 30)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1 row purged, got %d", n)
	}

	latest, err := s.LatestMetric(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if latest == nil || latest.TS != recent {
		t.Errorf("expected recent row to remain, got %+v", latest)
	}
}

func TestAggregateBucket(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	base := int64(1_700_000_040) // aligned to a 60-second boundary so offsets 0..50 stay in one bucket

	// 6 rows spread across 2 minute-buckets: temps 40,42,44 in bucket A; 50,55,52 in bucket B.
	// MAX(temp) for each bucket should be 44 and 55 respectively.
	rows := []Snapshot{
		{TS: base + 0, SysTempC: 40, CPUUsage: 10},
		{TS: base + 20, SysTempC: 42, CPUUsage: 12},
		{TS: base + 50, SysTempC: 44, CPUUsage: 14},
		{TS: base + 60, SysTempC: 50, CPUUsage: 20},
		{TS: base + 80, SysTempC: 55, CPUUsage: 25},
		{TS: base + 110, SysTempC: 52, CPUUsage: 22},
	}
	for i := range rows {
		if err := s.InsertMetric(ctx, &rows[i]); err != nil {
			t.Fatal(err)
		}
	}

	out, err := s.QueryMetrics(ctx, base, base+120, 60)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(out))
	}
	if out[0].SysTempC != 44 {
		t.Errorf("bucket0 temp expected 44 (MAX), got %v", out[0].SysTempC)
	}
	if out[1].SysTempC != 55 {
		t.Errorf("bucket1 temp expected 55 (MAX), got %v", out[1].SysTempC)
	}
	// CPU is AVG: bucket0 avg = (10+12+14)/3 = 12
	if out[0].CPUUsage < 11.9 || out[0].CPUUsage > 12.1 {
		t.Errorf("bucket0 cpu avg expected ~12, got %v", out[0].CPUUsage)
	}
}

func TestMigrateAddsNewColumns(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "m.db")

	// Create a DB with old schema (no new columns)
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	oldSchema := `
CREATE TABLE IF NOT EXISTS config (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  qnap_url TEXT NOT NULL DEFAULT '',
  qnap_user TEXT NOT NULL DEFAULT '',
  qnap_password_enc BLOB,
  collect_interval_seconds INTEGER NOT NULL DEFAULT 10,
  temp_threshold_celsius REAL NOT NULL DEFAULT 55,
  retention_days INTEGER NOT NULL DEFAULT 30,
  wecom_webhook_url TEXT NOT NULL DEFAULT '',
  updated_at INTEGER NOT NULL DEFAULT 0
);
INSERT OR IGNORE INTO config (id, updated_at) VALUES (1, 0);
CREATE TABLE IF NOT EXISTS alerts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  type TEXT NOT NULL,
  value REAL,
  threshold REAL,
  webhook_sent INTEGER NOT NULL DEFAULT 0,
  acknowledged INTEGER NOT NULL DEFAULT 0
);`
	if _, err := db.Exec(oldSchema); err != nil {
		t.Fatal(err)
	}
	// Insert a config row and an alert with old schema
	if _, err := db.Exec(`UPDATE config SET qnap_url='http://test', temp_threshold_celsius=60 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO alerts (ts, type, value, threshold) VALUES (1000, 'temperature_high', 65, 60)`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	// Open with new code — should run migration
	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	// Config should preserve old data
	cfg := s.db.QueryRow(`SELECT qnap_url, temp_threshold_celsius, disk_temp_threshold_celsius FROM config WHERE id=1`)
	var url string
	var tempThresh, diskThresh float64
	if err := cfg.Scan(&url, &tempThresh, &diskThresh); err != nil {
		t.Fatal(err)
	}
	if url != "http://test" {
		t.Errorf("expected url preserved, got %s", url)
	}
	if tempThresh != 60 {
		t.Errorf("expected temp threshold 60, got %v", tempThresh)
	}
	if diskThresh != 55 {
		t.Errorf("expected disk temp threshold default 55, got %v", diskThresh)
	}

	// Alert should preserve old data, new columns should have defaults
	alerts, err := s.ListAlerts(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	a := alerts[0]
	if a.Value != 65 {
		t.Errorf("expected value 65, got %v", a.Value)
	}
	if a.EndTS != nil {
		t.Errorf("expected endTs nil (old alert), got %v", *a.EndTS)
	}
	if a.HDNo != "" {
		t.Errorf("expected hdNo empty (old alert), got %s", a.HDNo)
	}

	// New alert with new columns should work
	id, err := s.InsertAlert(ctx, &Alert{TS: 2000, Type: "disk_temperature_high", HDNo: "0:1", Value: 55, Threshold: 50})
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("expected non-zero alert id")
	}
}
