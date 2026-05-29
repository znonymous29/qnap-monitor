package config

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Config is the runtime configuration of the monitor.
type Config struct {
	QNAPURL                   string  `json:"qnapUrl"`
	QNAPUser                  string  `json:"qnapUser"`
	QNAPPassword              string  `json:"-"` // decrypted in-memory only
	CollectIntervalSeconds    int     `json:"collectIntervalSeconds"`
	TempThresholdCelsius      float64 `json:"tempThresholdCelsius"`
	DiskTempThresholdCelsius  float64 `json:"diskTempThresholdCelsius"`
	CPUTempThresholdCelsius   float64 `json:"cpuTempThresholdCelsius"`
	RetentionDays             int     `json:"retentionDays"`
	WeComWebhookURL           string  `json:"wecomWebhookUrl"`
	UpdatedAt                 int64   `json:"updatedAt"`
}

// View is the safe-for-JSON shape returned to the frontend (no password).
type View struct {
	QNAPURL                   string  `json:"qnapUrl"`
	QNAPUser                  string  `json:"qnapUser"`
	PasswordSet               bool    `json:"passwordSet"`
	CollectIntervalSeconds    int     `json:"collectIntervalSeconds"`
	TempThresholdCelsius      float64 `json:"tempThresholdCelsius"`
	DiskTempThresholdCelsius  float64 `json:"diskTempThresholdCelsius"`
	CPUTempThresholdCelsius   float64 `json:"cpuTempThresholdCelsius"`
	RetentionDays             int     `json:"retentionDays"`
	WeComWebhookURL           string  `json:"wecomWebhookUrl"`
	UpdatedAt                 int64   `json:"updatedAt"`
}

func (c Config) View() View {
	return View{
		QNAPURL:                  c.QNAPURL,
		QNAPUser:                 c.QNAPUser,
		PasswordSet:              c.QNAPPassword != "",
		CollectIntervalSeconds:   c.CollectIntervalSeconds,
		TempThresholdCelsius:     c.TempThresholdCelsius,
		DiskTempThresholdCelsius: c.DiskTempThresholdCelsius,
		CPUTempThresholdCelsius:  c.CPUTempThresholdCelsius,
		RetentionDays:            c.RetentionDays,
		WeComWebhookURL:          c.WeComWebhookURL,
		UpdatedAt:                c.UpdatedAt,
	}
}

// Manager owns the in-memory copy of Config and writes through to SQLite.
// Subscribers are notified on every Save() so the collector can re-tune its ticker.
type Manager struct {
	db  *sql.DB
	key []byte

	mu          sync.RWMutex
	cur         Config
	subscribers []chan struct{}
}

func NewManager(db *sql.DB, key []byte) (*Manager, error) {
	m := &Manager{db: db, key: key}
	if err := m.load(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) load() error {
	row := m.db.QueryRow(`
		SELECT qnap_url, qnap_user, qnap_password_enc, collect_interval_seconds,
		       temp_threshold_celsius, disk_temp_threshold_celsius, cpu_temp_threshold_celsius,
		       retention_days, wecom_webhook_url, updated_at
		FROM config WHERE id = 1`)
	var enc []byte
	var c Config
	if err := row.Scan(&c.QNAPURL, &c.QNAPUser, &enc, &c.CollectIntervalSeconds,
		&c.TempThresholdCelsius, &c.DiskTempThresholdCelsius, &c.CPUTempThresholdCelsius,
		&c.RetentionDays, &c.WeComWebhookURL, &c.UpdatedAt); err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if len(enc) > 0 {
		pw, err := Decrypt(m.key, enc)
		if err != nil {
			return fmt.Errorf("decrypt password: %w", err)
		}
		c.QNAPPassword = string(pw)
	}
	m.cur = c
	return nil
}

func (m *Manager) Current() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cur
}

// Update merges patch into the current config. If patch.QNAPPassword is empty,
// the existing password is preserved.
type Update struct {
	QNAPURL                   *string  `json:"qnapUrl"`
	QNAPUser                  *string  `json:"qnapUser"`
	QNAPPassword              *string  `json:"qnapPassword"` // nil/empty = unchanged
	CollectIntervalSeconds    *int     `json:"collectIntervalSeconds"`
	TempThresholdCelsius      *float64 `json:"tempThresholdCelsius"`
	DiskTempThresholdCelsius  *float64 `json:"diskTempThresholdCelsius"`
	CPUTempThresholdCelsius   *float64 `json:"cpuTempThresholdCelsius"`
	RetentionDays             *int     `json:"retentionDays"`
	WeComWebhookURL           *string  `json:"wecomWebhookUrl"`
}

func (m *Manager) Apply(ctx context.Context, u Update) (Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := m.cur
	if u.QNAPURL != nil {
		next.QNAPURL = *u.QNAPURL
	}
	if u.QNAPUser != nil {
		next.QNAPUser = *u.QNAPUser
	}
	if u.QNAPPassword != nil && *u.QNAPPassword != "" {
		next.QNAPPassword = *u.QNAPPassword
	}
	if u.CollectIntervalSeconds != nil {
		if *u.CollectIntervalSeconds < 1 {
			return Config{}, errors.New("collectIntervalSeconds must be >= 1")
		}
		next.CollectIntervalSeconds = *u.CollectIntervalSeconds
	}
	if u.TempThresholdCelsius != nil {
		next.TempThresholdCelsius = *u.TempThresholdCelsius
	}
	if u.DiskTempThresholdCelsius != nil {
		next.DiskTempThresholdCelsius = *u.DiskTempThresholdCelsius
	}
	if u.CPUTempThresholdCelsius != nil {
		next.CPUTempThresholdCelsius = *u.CPUTempThresholdCelsius
	}
	if u.RetentionDays != nil {
		if *u.RetentionDays < 1 {
			return Config{}, errors.New("retentionDays must be >= 1")
		}
		next.RetentionDays = *u.RetentionDays
	}
	if u.WeComWebhookURL != nil {
		next.WeComWebhookURL = *u.WeComWebhookURL
	}
	next.UpdatedAt = time.Now().Unix()

	enc, err := Encrypt(m.key, []byte(next.QNAPPassword))
	if err != nil {
		return Config{}, err
	}
	_, err = m.db.ExecContext(ctx, `
		UPDATE config SET qnap_url=?, qnap_user=?, qnap_password_enc=?,
		  collect_interval_seconds=?, temp_threshold_celsius=?, disk_temp_threshold_celsius=?,
		  cpu_temp_threshold_celsius=?,
		  retention_days=?, wecom_webhook_url=?, updated_at=?
		WHERE id = 1`,
		next.QNAPURL, next.QNAPUser, enc,
		next.CollectIntervalSeconds, next.TempThresholdCelsius, next.DiskTempThresholdCelsius,
		next.CPUTempThresholdCelsius,
		next.RetentionDays, next.WeComWebhookURL, next.UpdatedAt)
	if err != nil {
		return Config{}, err
	}
	m.cur = next
	m.notify()
	return next, nil
}

// Subscribe returns a channel that receives a signal every time the config is updated.
func (m *Manager) Subscribe() <-chan struct{} {
	ch := make(chan struct{}, 1)
	m.mu.Lock()
	m.subscribers = append(m.subscribers, ch)
	m.mu.Unlock()
	return ch
}

func (m *Manager) notify() {
	for _, ch := range m.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
