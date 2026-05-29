package store

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"log"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }
func (s *Store) DB() *sql.DB  { return s.db }

// --------------- Metrics (system-level) ---------------

type Snapshot struct {
	TS               int64
	CPUUsage         float64
	MemUsage         float64
	SysTempC         float64
	CPUTempC         float64
	FanRPM           int
	VolumeTotalBytes int64
	VolumeUsedBytes  int64
	VolumeUsagePct   float64
}

func (s *Store) InsertMetric(ctx context.Context, snap *Snapshot) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO metrics (ts, cpu_usage, mem_usage, sys_temp_c, cpu_temp_c, fan_rpm, volume_total_bytes, volume_used_bytes, volume_usage_pct)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snap.TS, snap.CPUUsage, snap.MemUsage, snap.SysTempC, snap.CPUTempC, snap.FanRPM,
		snap.VolumeTotalBytes, snap.VolumeUsedBytes, snap.VolumeUsagePct)
	return err
}

func (s *Store) LatestMetric(ctx context.Context) (*Snapshot, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT ts, cpu_usage, mem_usage, sys_temp_c, cpu_temp_c, fan_rpm, volume_total_bytes, volume_used_bytes, volume_usage_pct
		FROM metrics ORDER BY ts DESC LIMIT 1`)
	var snap Snapshot
	var cpuTemp sql.NullFloat64
	var fanRPM sql.NullInt64
	if err := row.Scan(&snap.TS, &snap.CPUUsage, &snap.MemUsage, &snap.SysTempC,
		&cpuTemp, &fanRPM, &snap.VolumeTotalBytes, &snap.VolumeUsedBytes, &snap.VolumeUsagePct); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if cpuTemp.Valid {
		snap.CPUTempC = cpuTemp.Float64
	}
	if fanRPM.Valid {
		snap.FanRPM = int(fanRPM.Int64)
	}
	return &snap, nil
}

func (s *Store) QueryMetrics(ctx context.Context, from, to int64, bucketSeconds int) ([]Snapshot, error) {
	var query string
	if bucketSeconds <= 0 {
		query = `SELECT ts, cpu_usage, mem_usage, sys_temp_c, cpu_temp_c, fan_rpm, volume_total_bytes, volume_used_bytes, volume_usage_pct
			FROM metrics WHERE ts BETWEEN ? AND ? ORDER BY ts ASC`
	} else {
		query = fmt.Sprintf(`SELECT
			(ts / %d) * %d AS bucket_ts,
			AVG(cpu_usage), AVG(mem_usage), MAX(sys_temp_c), MAX(cpu_temp_c), MAX(fan_rpm),
			AVG(volume_total_bytes), AVG(volume_used_bytes), AVG(volume_usage_pct)
			FROM metrics WHERE ts BETWEEN ? AND ?
			GROUP BY bucket_ts ORDER BY bucket_ts ASC`, bucketSeconds, bucketSeconds)
	}
	rows, err := s.db.QueryContext(ctx, query, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Snapshot{}
	for rows.Next() {
		var snap Snapshot
		var volTotal, volUsed sql.NullFloat64
		var cpuTemp sql.NullFloat64
		var fanRPM sql.NullInt64
		if bucketSeconds > 0 {
			if err := rows.Scan(&snap.TS, &snap.CPUUsage, &snap.MemUsage, &snap.SysTempC,
				&cpuTemp, &fanRPM, &volTotal, &volUsed, &snap.VolumeUsagePct); err != nil {
				return nil, err
			}
			snap.VolumeTotalBytes = int64(volTotal.Float64)
			snap.VolumeUsedBytes = int64(volUsed.Float64)
		} else {
			if err := rows.Scan(&snap.TS, &snap.CPUUsage, &snap.MemUsage, &snap.SysTempC,
				&cpuTemp, &fanRPM, &snap.VolumeTotalBytes, &snap.VolumeUsedBytes, &snap.VolumeUsagePct); err != nil {
				return nil, err
			}
		}
		if cpuTemp.Valid {
			snap.CPUTempC = cpuTemp.Float64
		}
		if fanRPM.Valid {
			snap.FanRPM = int(fanRPM.Int64)
		}
		out = append(out, snap)
	}
	return out, rows.Err()
}

// --------------- System info (static) ---------------

type SystemInfo struct {
	Model          string `json:"model"`
	SerialNumber   string `json:"serialNumber"`
	Firmware       string `json:"firmware"`
	UptimeSeconds  int64  `json:"uptimeSeconds"`
	UpdatedAt      int64  `json:"updatedAt"`
}

func (s *Store) UpsertSystemInfo(ctx context.Context, info *SystemInfo) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE system_info SET model=?, serial_number=?, firmware=?, uptime_seconds=?, updated_at=?
		WHERE id=1`,
		info.Model, info.SerialNumber, info.Firmware, info.UptimeSeconds, info.UpdatedAt)
	return err
}

func (s *Store) GetSystemInfo(ctx context.Context) (*SystemInfo, error) {
	row := s.db.QueryRowContext(ctx, `SELECT model, serial_number, firmware, uptime_seconds, updated_at FROM system_info WHERE id=1`)
	var info SystemInfo
	if err := row.Scan(&info.Model, &info.SerialNumber, &info.Firmware, &info.UptimeSeconds, &info.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &info, nil
}

// --------------- Disks (static metadata) ---------------

type DiskRow struct {
	HDNo              string `json:"hdNo"`
	Alias             string `json:"alias"`
	Model             string `json:"model"`
	Serial            string `json:"serial"`
	Firmware          string `json:"firmware"`
	Vendor            string `json:"vendor"`
	Capacity          string `json:"capacity"`
	CapacityBytes     int64  `json:"capacityBytes"`
	Health            string `json:"health"`
	IsSSD             bool   `json:"isSsd"`
	DiskStatus        int    `json:"diskStatus"`
	PowerOnHours      int64  `json:"powerOnHours"`
	ReallocatedSectors int64  `json:"reallocatedSectors"`
	TempC             int    `json:"tempC"` // latest temperature
	UpdatedAt         int64  `json:"updatedAt"`
}

// UpsertDisk inserts or updates a disk's static info. Also stores the latest temp.
func (s *Store) UpsertDisk(ctx context.Context, d *DiskRow) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO disks (hd_no, alias, model, serial, firmware, vendor, capacity, capacity_bytes, health, is_ssd, disk_status, power_on_hours, reallocated_sectors, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(hd_no) DO UPDATE SET
			alias=excluded.alias, model=excluded.model, serial=excluded.serial,
			firmware=excluded.firmware, vendor=excluded.vendor, capacity=excluded.capacity, capacity_bytes=excluded.capacity_bytes,
			health=excluded.health, is_ssd=excluded.is_ssd, disk_status=excluded.disk_status,
			power_on_hours=excluded.power_on_hours, reallocated_sectors=excluded.reallocated_sectors, updated_at=excluded.updated_at`,
		d.HDNo, d.Alias, d.Model, d.Serial, d.Firmware, d.Vendor, d.Capacity, d.CapacityBytes,
		d.Health, boolToInt(d.IsSSD), d.DiskStatus, d.PowerOnHours, d.ReallocatedSectors, d.UpdatedAt)
	return err
}

// ListDisks returns all known disks.
func (s *Store) ListDisks(ctx context.Context) ([]DiskRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT hd_no, alias, model, serial, firmware, vendor, capacity, capacity_bytes, health, is_ssd, disk_status, power_on_hours, reallocated_sectors, updated_at
		FROM disks ORDER BY hd_no`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DiskRow
	for rows.Next() {
		var d DiskRow
		var isSSD int
		if err := rows.Scan(&d.HDNo, &d.Alias, &d.Model, &d.Serial, &d.Firmware, &d.Vendor,
			&d.Capacity, &d.CapacityBytes, &d.Health, &isSSD, &d.DiskStatus,
			&d.PowerOnHours, &d.ReallocatedSectors, &d.UpdatedAt); err != nil {
			return nil, err
		}
		d.IsSSD = isSSD != 0
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetLatestDiskTemps returns the latest temperature for each disk.
func (s *Store) GetLatestDiskTemps(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT hd_no, temp_c FROM disk_temps
		WHERE id IN (SELECT MAX(id) FROM disk_temps GROUP BY hd_no)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string]int)
	for rows.Next() {
		var hdNo string
		var temp int
		if err := rows.Scan(&hdNo, &temp); err != nil {
			return nil, err
		}
		m[hdNo] = temp
	}
	return m, rows.Err()
}

// --------------- Disk temperatures (time-series) ---------------

type DiskTempRow struct {
	TS    int64  `json:"ts"`
	HDNo  string `json:"hdNo"`
	TempC int    `json:"tempC"`
}

func (s *Store) InsertDiskTemp(ctx context.Context, ts int64, hdNo string, tempC int) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO disk_temps (ts, hd_no, temp_c) VALUES (?, ?, ?)`, ts, hdNo, tempC)
	return err
}

// QueryDiskTemps returns temperature history for a specific disk.
func (s *Store) QueryDiskTemps(ctx context.Context, hdNo string, from, to int64, bucketSeconds int) ([]DiskTempRow, error) {
	var query string
	if bucketSeconds <= 0 {
		query = `SELECT ts, hd_no, temp_c FROM disk_temps WHERE hd_no = ? AND ts BETWEEN ? AND ? ORDER BY ts ASC`
	} else {
		query = fmt.Sprintf(`SELECT (ts / %d) * %d, hd_no, MAX(temp_c)
			FROM disk_temps WHERE hd_no = ? AND ts BETWEEN ? AND ?
			GROUP BY (ts / %d) ORDER BY (ts / %d) ASC`, bucketSeconds, bucketSeconds, bucketSeconds, bucketSeconds)
	}
	rows, err := s.db.QueryContext(ctx, query, hdNo, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DiskTempRow
	for rows.Next() {
		var r DiskTempRow
		if err := rows.Scan(&r.TS, &r.HDNo, &r.TempC); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// --------------- Volume details (time-series) ---------------

type VolumeDetailRow struct {
	TS           int64   `json:"ts"`
	VolNo        int     `json:"volNo"`
	Label        string  `json:"label"`
	CapacityBytes int64  `json:"capacityBytes"`
	UsedBytes    int64   `json:"usedBytes"`
	FreeBytes    int64   `json:"freeBytes"`
	UsedPct      float64 `json:"usedPct"`
	Filesystem   string  `json:"filesystem"`
	RaidLevel    int     `json:"raidLevel"`
	MountPath    string  `json:"mountPath"`
	HDList       string  `json:"hdList"`
}

func (s *Store) InsertVolumeDetail(ctx context.Context, v *VolumeDetailRow) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO volume_details (ts, vol_no, vol_label, capacity_bytes, used_bytes, free_bytes, used_pct, filesystem, raid_level, mount_path, hd_list)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.TS, v.VolNo, v.Label, v.CapacityBytes, v.UsedBytes, v.FreeBytes, v.UsedPct,
		v.Filesystem, v.RaidLevel, v.MountPath, v.HDList)
	return err
}

// LatestVolumeDetails returns the most recent snapshot of each volume.
func (s *Store) LatestVolumeDetails(ctx context.Context) ([]VolumeDetailRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT v.ts, v.vol_no, v.vol_label, v.capacity_bytes, v.used_bytes, v.free_bytes, v.used_pct, v.filesystem, v.raid_level, v.mount_path, v.hd_list
		FROM volume_details v
		INNER JOIN (SELECT MAX(id) as max_id FROM volume_details GROUP BY vol_no) m ON v.id = m.max_id
		ORDER BY v.vol_no`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VolumeDetailRow
	for rows.Next() {
		var v VolumeDetailRow
		if err := rows.Scan(&v.TS, &v.VolNo, &v.Label, &v.CapacityBytes, &v.UsedBytes, &v.FreeBytes,
			&v.UsedPct, &v.Filesystem, &v.RaidLevel, &v.MountPath, &v.HDList); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// QueryVolumeUsage returns usage history for a specific volume.
func (s *Store) QueryVolumeUsage(ctx context.Context, volNo int, from, to int64, bucketSeconds int) ([]VolumeDetailRow, error) {
	var query string
	if bucketSeconds <= 0 {
		query = `SELECT ts, vol_no, vol_label, capacity_bytes, used_bytes, free_bytes, used_pct, filesystem, raid_level, mount_path, hd_list
			FROM volume_details WHERE vol_no = ? AND ts BETWEEN ? AND ? ORDER BY ts ASC`
	} else {
		query = fmt.Sprintf(`SELECT (ts / %d) * %d, vol_no, '', MAX(capacity_bytes), CAST(AVG(used_bytes) AS INTEGER), CAST(AVG(free_bytes) AS INTEGER), AVG(used_pct), '', 0, '', ''
			FROM volume_details WHERE vol_no = ? AND ts BETWEEN ? AND ?
			GROUP BY (ts / %d) ORDER BY (ts / %d) ASC`, bucketSeconds, bucketSeconds, bucketSeconds, bucketSeconds)
	}
	rows, err := s.db.QueryContext(ctx, query, volNo, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VolumeDetailRow
	for rows.Next() {
		var v VolumeDetailRow
		if err := rows.Scan(&v.TS, &v.VolNo, &v.Label, &v.CapacityBytes, &v.UsedBytes, &v.FreeBytes,
			&v.UsedPct, &v.Filesystem, &v.RaidLevel, &v.MountPath, &v.HDList); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// --------------- Purge ---------------

func (s *Store) PurgeOldData(ctx context.Context, retentionDays int) (int64, error) {
	cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour).Unix()
	total := int64(0)
	for _, table := range []string{"metrics", "disk_temps", "volume_details"} {
		res, err := s.db.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE ts < ?`, table), cutoff)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}

// Backward-compatible alias used by existing code.
func (s *Store) PurgeOldMetrics(ctx context.Context, retentionDays int) (int64, error) {
	return s.PurgeOldData(ctx, retentionDays)
}

// PurgeOldAlerts deletes alerts older than the given number of days.
func (s *Store) PurgeOldAlerts(ctx context.Context, days int) (int64, error) {
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
	res, err := s.db.ExecContext(ctx, `DELETE FROM alerts WHERE ts < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// --------------- Alerts ---------------

type Alert struct {
	ID           int64   `json:"id"`
	TS           int64   `json:"ts"`
	EndTS        *int64  `json:"endTs"`
	Type         string  `json:"type"`
	HDNo         string  `json:"hdNo"`
	Value        float64 `json:"value"`
	PeakValue    *float64 `json:"peakValue"`
	Threshold    float64 `json:"threshold"`
	WebhookSent  bool    `json:"webhookSent"`
	Acknowledged bool    `json:"acknowledged"`
}

func (s *Store) InsertAlert(ctx context.Context, a *Alert) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO alerts (ts, type, hd_no, value, peak_value, threshold, webhook_sent, acknowledged)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		a.TS, a.Type, a.HDNo, a.Value, a.Value, a.Threshold, boolToInt(a.WebhookSent), boolToInt(a.Acknowledged))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) MarkAlertWebhookSent(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE alerts SET webhook_sent = 1 WHERE id = ?`, id)
	return err
}

func (s *Store) CloseAlert(ctx context.Context, id int64, endTS int64, peakValue float64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE alerts SET end_ts = ?, peak_value = ? WHERE id = ?`, endTS, peakValue, id)
	return err
}

func (s *Store) UpdateAlertPeak(ctx context.Context, id int64, currentValue float64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE alerts SET peak_value = MAX(peak_value, ?), value = ? WHERE id = ? AND end_ts IS NULL`, currentValue, currentValue, id)
	return err
}

func (s *Store) AckAlert(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE alerts SET acknowledged = 1 WHERE id = ?`, id)
	return err
}

// ListOpenAlerts returns all alerts that have not been closed (end_ts IS NULL).
func (s *Store) ListOpenAlerts(ctx context.Context) ([]Alert, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, ts, end_ts, type, hd_no, value, peak_value, threshold, webhook_sent, acknowledged
		FROM alerts WHERE end_ts IS NULL ORDER BY ts DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Alert
	for rows.Next() {
		var a Alert
		var ws, ack int
		var endTS sql.NullInt64
		var peakVal sql.NullFloat64
		if err := rows.Scan(&a.ID, &a.TS, &endTS, &a.Type, &a.HDNo, &a.Value, &peakVal, &a.Threshold, &ws, &ack); err != nil {
			return nil, err
		}
		a.WebhookSent = ws != 0
		a.Acknowledged = ack != 0
		if endTS.Valid {
			a.EndTS = &endTS.Int64
		}
		if peakVal.Valid {
			a.PeakValue = &peakVal.Float64
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CloseOpenAlerts closes all alerts that are still open (end_ts IS NULL).
func (s *Store) CloseOpenAlerts(ctx context.Context, endTS int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE alerts SET end_ts = ? WHERE end_ts IS NULL`, endTS)
	return err
}

func (s *Store) ListAlerts(ctx context.Context, limit int) ([]Alert, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, ts, end_ts, type, hd_no, value, peak_value, threshold, webhook_sent, acknowledged
		FROM alerts ORDER BY CASE WHEN end_ts IS NULL THEN 0 ELSE 1 END, ts DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Alert{}
	for rows.Next() {
		var a Alert
		var ws, ack int
		var endTS sql.NullInt64
		var peakVal sql.NullFloat64
		if err := rows.Scan(&a.ID, &a.TS, &endTS, &a.Type, &a.HDNo, &a.Value, &peakVal, &a.Threshold, &ws, &ack); err != nil {
			return nil, err
		}
		a.WebhookSent = ws != 0
		a.Acknowledged = ack != 0
		if endTS.Valid {
			a.EndTS = &endTS.Int64
		}
		if peakVal.Valid {
			a.PeakValue = &peakVal.Float64
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// columnMigrations lists columns that may need to be added to existing databases.
// Format: (table, column, column_definition).
var columnMigrations = []struct {
	table string
	col   string
	def   string
}{
	{"config", "disk_temp_threshold_celsius", "REAL NOT NULL DEFAULT 55"},
	{"alerts", "end_ts", "INTEGER"},
	{"alerts", "hd_no", "TEXT NOT NULL DEFAULT ''"},
	{"alerts", "peak_value", "REAL"},
	{"disks", "vendor", "TEXT NOT NULL DEFAULT ''"},
	{"config", "cpu_temp_threshold_celsius", "REAL NOT NULL DEFAULT 75"},
	{"disks", "power_on_hours", "INTEGER NOT NULL DEFAULT 0"},
	{"disks", "reallocated_sectors", "INTEGER NOT NULL DEFAULT 0"},
	{"metrics", "cpu_temp_c", "REAL"},
	{"metrics", "fan_rpm", "INTEGER"},
}

func migrate(db *sql.DB) error {
	for _, m := range columnMigrations {
		exists, err := columnExists(db, m.table, m.col)
		if err != nil {
			return err
		}
		if !exists {
			sql := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", m.table, m.col, m.def)
			log.Printf("migrate: %s", sql)
			if _, err := db.Exec(sql); err != nil {
				return fmt.Errorf("add %s.%s: %w", m.table, m.col, err)
			}
		}
	}
	return nil
}

func columnExists(db *sql.DB, table, col string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if strings.EqualFold(name, col) {
			return true, nil
		}
	}
	return false, rows.Err()
}
