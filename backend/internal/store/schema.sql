CREATE TABLE IF NOT EXISTS config (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  qnap_url TEXT NOT NULL DEFAULT '',
  qnap_user TEXT NOT NULL DEFAULT '',
  qnap_password_enc BLOB,
  collect_interval_seconds INTEGER NOT NULL DEFAULT 10,
  temp_threshold_celsius REAL NOT NULL DEFAULT 55,
  disk_temp_threshold_celsius REAL NOT NULL DEFAULT 55,
  cpu_temp_threshold_celsius REAL NOT NULL DEFAULT 75,
  retention_days INTEGER NOT NULL DEFAULT 30,
  wecom_webhook_url TEXT NOT NULL DEFAULT '',
  updated_at INTEGER NOT NULL DEFAULT 0
);

INSERT OR IGNORE INTO config (id, updated_at) VALUES (1, 0);

CREATE TABLE IF NOT EXISTS metrics (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  cpu_usage REAL,
  mem_usage REAL,
  sys_temp_c REAL,
  cpu_temp_c REAL,
  fan_rpm INTEGER,
  volume_total_bytes INTEGER,
  volume_used_bytes INTEGER,
  volume_usage_pct REAL
);
CREATE INDEX IF NOT EXISTS idx_metrics_ts ON metrics(ts);

-- Static NAS system info, upserted every collection cycle.
CREATE TABLE IF NOT EXISTS system_info (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  model TEXT NOT NULL DEFAULT '',
  serial_number TEXT NOT NULL DEFAULT '',
  firmware TEXT NOT NULL DEFAULT '',
  uptime_seconds INTEGER NOT NULL DEFAULT 0,
  updated_at INTEGER NOT NULL DEFAULT 0
);
INSERT OR IGNORE INTO system_info (id) VALUES (1);

CREATE TABLE IF NOT EXISTS alerts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  end_ts INTEGER,
  type TEXT NOT NULL,
  hd_no TEXT NOT NULL DEFAULT '',
  value REAL,
  peak_value REAL,
  threshold REAL,
  webhook_sent INTEGER NOT NULL DEFAULT 0,
  acknowledged INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_alerts_ts ON alerts(ts);

-- Static disk info, upserted every collection cycle (cheap: only ~6 rows).
CREATE TABLE IF NOT EXISTS disks (
  hd_no TEXT PRIMARY KEY,          -- e.g. "0:1"
  alias TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  serial TEXT NOT NULL DEFAULT '',
  firmware TEXT NOT NULL DEFAULT '',
  vendor TEXT NOT NULL DEFAULT '',
  capacity TEXT NOT NULL DEFAULT '',
  capacity_bytes INTEGER NOT NULL DEFAULT 0,
  health TEXT NOT NULL DEFAULT '',
  is_ssd INTEGER NOT NULL DEFAULT 0,
  disk_status INTEGER NOT NULL DEFAULT 0,
  power_on_hours INTEGER NOT NULL DEFAULT 0,
  reallocated_sectors INTEGER NOT NULL DEFAULT 0,
  updated_at INTEGER NOT NULL DEFAULT 0
);

-- Per-disk temperature time-series.
CREATE TABLE IF NOT EXISTS disk_temps (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  hd_no TEXT NOT NULL,
  temp_c INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_disk_temps_ts ON disk_temps(ts);
CREATE INDEX IF NOT EXISTS idx_disk_temps_hdno ON disk_temps(hd_no, ts);

-- Per-volume detail time-series.
CREATE TABLE IF NOT EXISTS volume_details (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  vol_no INTEGER NOT NULL,
  vol_label TEXT NOT NULL DEFAULT '',
  capacity_bytes INTEGER NOT NULL DEFAULT 0,
  used_bytes INTEGER NOT NULL DEFAULT 0,
  free_bytes INTEGER NOT NULL DEFAULT 0,
  used_pct REAL NOT NULL DEFAULT 0,
  filesystem TEXT NOT NULL DEFAULT '',
  raid_level INTEGER NOT NULL DEFAULT 0,
  mount_path TEXT NOT NULL DEFAULT '',
  hd_list TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_vol_details_ts ON volume_details(ts);
CREATE INDEX IF NOT EXISTS idx_vol_details_volno ON volume_details(vol_no, ts);
