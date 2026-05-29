package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/qnap-monitor/backend/internal/store"
)

const (
	TypeTempHigh     = "temperature_high"
	TypeCPUTempHigh  = "cpu_temperature_high"
	TypeDiskTempHigh = "disk_temperature_high"
	TypeDiskHealth   = "disk_health_warning"
)

// Manager evaluates system temp, disk temps, and disk health against thresholds.
// Alerts are consolidated: a single row tracks the full duration of an incident
// (from first detection to recovery). Peak values are updated while active.
type Manager struct {
	store *store.Store

	mu                sync.Mutex
	threshold         float64
	diskTempThreshold float64
	cpuTempThreshold  float64
	webhookURL        string

	// System temp alert state
	sysAlertID  int64
	sysAlertVal float64

	// CPU temp alert state
	cpuAlertID  int64
	cpuAlertVal float64

	// Per-disk temp alert state: hdNo → (alertID, peakValue)
	diskTempAlerts map[string]diskAlertState

	// Per-disk health alert state: hdNo → alertID
	diskHealthAlerts map[string]int64

	lastEvent *Event
}

type diskAlertState struct {
	alertID int64
	peak    float64
}

type Event struct {
	ID        int64   `json:"id"`
	TS        int64   `json:"ts"`
	Type      string  `json:"type"`
	HDNo      string  `json:"hdNo"`
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
}

func NewManager(s *store.Store, threshold, diskTempThreshold, cpuTempThreshold float64, webhookURL string) *Manager {
	return &Manager{
		store:             s,
		threshold:         threshold,
		diskTempThreshold: diskTempThreshold,
		cpuTempThreshold:  cpuTempThreshold,
		webhookURL:        webhookURL,
		diskTempAlerts:    make(map[string]diskAlertState),
		diskHealthAlerts:  make(map[string]int64),
	}
}

// RestoreFromDB loads open alerts from the database and restores in-memory state.
// This ensures alerts that were active when the app was interrupted can be
// properly closed on recovery.
func (m *Manager) RestoreFromDB(ctx context.Context) error {
	open, err := m.store.ListOpenAlerts(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range open {
		peak := a.Value
		if a.PeakValue != nil {
			peak = *a.PeakValue
		}
		switch a.Type {
		case TypeTempHigh:
			m.sysAlertID = a.ID
			m.sysAlertVal = peak
		case TypeCPUTempHigh:
			m.cpuAlertID = a.ID
			m.cpuAlertVal = peak
		case TypeDiskTempHigh:
			m.diskTempAlerts[a.HDNo] = diskAlertState{alertID: a.ID, peak: peak}
		case TypeDiskHealth:
			m.diskHealthAlerts[a.HDNo] = a.ID
		}
	}
	return nil
}

func (m *Manager) UpdateConfig(threshold, diskTempThreshold, cpuTempThreshold float64, webhookURL string) {
	m.mu.Lock()
	m.threshold = threshold
	m.diskTempThreshold = diskTempThreshold
	m.cpuTempThreshold = cpuTempThreshold
	m.webhookURL = webhookURL
	m.mu.Unlock()
}

func (m *Manager) State() (inAlert bool, threshold float64, diskTempThreshold float64, cpuTempThreshold float64, lastEvent *Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sysAlertID != 0 || m.cpuAlertID != 0, m.threshold, m.diskTempThreshold, m.cpuTempThreshold, m.lastEvent
}

func (m *Manager) ConsumeLastEvent() *Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.lastEvent
	m.lastEvent = nil
	return e
}

// UnhealthyDisks returns hdNo set for disks currently in health alert.
func (m *Manager) UnhealthyDisks() map[string]bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]bool, len(m.diskHealthAlerts))
	for k := range m.diskHealthAlerts {
		out[k] = true
	}
	return out
}

// Evaluate checks system temperature against threshold.
func (m *Manager) Evaluate(ctx context.Context, tempC float64) {
	m.mu.Lock()
	webhook := m.webhookURL
	threshold := m.threshold
	activeID := m.sysAlertID
	m.mu.Unlock()

	over := tempC > threshold

	if over && activeID == 0 {
		// New alert
		now := time.Now().Unix()
		a := &store.Alert{TS: now, Type: TypeTempHigh, Value: tempC, Threshold: threshold}
		id, err := m.store.InsertAlert(ctx, a)
		if err != nil {
			log.Printf("alert: insert failed: %v", err)
			return
		}
		m.mu.Lock()
		m.sysAlertID = id
		m.sysAlertVal = tempC
		m.lastEvent = &Event{ID: id, TS: now, Type: TypeTempHigh, Value: tempC, Threshold: threshold}
		m.mu.Unlock()

		if webhook != "" {
			go m.sendWeCom(ctx, id, TypeTempHigh, tempC, threshold, "", webhook)
		}
	} else if over && activeID != 0 {
		// Still over — update peak
		_ = m.store.UpdateAlertPeak(ctx, activeID, tempC)
		m.mu.Lock()
		if tempC > m.sysAlertVal {
			m.sysAlertVal = tempC
		}
		m.mu.Unlock()
	} else if !over && activeID != 0 {
		// Recovered — close the alert
		now := time.Now().Unix()
		m.mu.Lock()
		peak := m.sysAlertVal
		m.sysAlertID = 0
		m.sysAlertVal = 0
		m.mu.Unlock()

		_ = m.store.CloseAlert(ctx, activeID, now, peak)
		if webhook != "" {
			go m.sendWeCom(ctx, activeID, TypeTempHigh, peak, threshold, "", webhook)
		}
	}
}

// EvaluateCPUTemp checks CPU temperature against the CPU temp threshold.
func (m *Manager) EvaluateCPUTemp(ctx context.Context, cpuTempC float64) {
	if cpuTempC <= 0 {
		return // not available
	}
	m.mu.Lock()
	webhook := m.webhookURL
	threshold := m.cpuTempThreshold
	activeID := m.cpuAlertID
	m.mu.Unlock()

	over := cpuTempC > threshold

	if over && activeID == 0 {
		now := time.Now().Unix()
		a := &store.Alert{TS: now, Type: TypeCPUTempHigh, Value: cpuTempC, Threshold: threshold}
		id, err := m.store.InsertAlert(ctx, a)
		if err != nil {
			log.Printf("alert: insert cpu temp failed: %v", err)
			return
		}
		m.mu.Lock()
		m.cpuAlertID = id
		m.cpuAlertVal = cpuTempC
		m.lastEvent = &Event{ID: id, TS: now, Type: TypeCPUTempHigh, Value: cpuTempC, Threshold: threshold}
		m.mu.Unlock()

		if webhook != "" {
			go m.sendCPUTempWeCom(ctx, id, cpuTempC, threshold, true, webhook)
		}
	} else if over && activeID != 0 {
		_ = m.store.UpdateAlertPeak(ctx, activeID, cpuTempC)
		m.mu.Lock()
		if cpuTempC > m.cpuAlertVal {
			m.cpuAlertVal = cpuTempC
		}
		m.mu.Unlock()
	} else if !over && activeID != 0 {
		now := time.Now().Unix()
		m.mu.Lock()
		peak := m.cpuAlertVal
		m.cpuAlertID = 0
		m.cpuAlertVal = 0
		m.mu.Unlock()

		_ = m.store.CloseAlert(ctx, activeID, now, peak)
		if webhook != "" {
			go m.sendCPUTempWeCom(ctx, activeID, peak, threshold, false, webhook)
		}
	}
}

func (m *Manager) sendCPUTempWeCom(_ context.Context, alertID int64, value, threshold float64, isHigh bool, webhook string) {
	var title, emoji string
	if isHigh {
		title = "QNAP CPU 温度告警"
		emoji = "🔥"
	} else {
		title = "QNAP CPU 温度恢复"
		emoji = "✅"
	}
	content := fmt.Sprintf("## %s %s\n> 当前温度：**%.1f°C**\n> 阈值：%.1f°C\n> 时间：%s",
		emoji, title, value, threshold, time.Now().Format("2006-01-02 15:04:05"))

	body, _ := json.Marshal(map[string]any{
		"msgtype":  "markdown",
		"markdown": map[string]string{"content": content},
	})

	cctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodPost, webhook, bytes.NewReader(body))
	if err != nil {
		log.Printf("alert: build cpu temp webhook request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("alert: cpu temp webhook send failed: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_ = m.store.MarkAlertWebhookSent(cctx, alertID)
	} else {
		log.Printf("alert: cpu temp webhook returned status %d", resp.StatusCode)
	}
}

// EvaluateDiskTemps checks per-disk temperatures against the disk temp threshold.
// diskAliases maps hdNo to display alias (e.g. "0:1" → "3.5\" SATA HDD 1").
func (m *Manager) EvaluateDiskTemps(ctx context.Context, diskTemps map[string]int, diskAliases map[string]string) {
	m.mu.Lock()
	threshold := m.diskTempThreshold
	webhook := m.webhookURL
	// Copy current state
	active := make(map[string]diskAlertState, len(m.diskTempAlerts))
	for k, v := range m.diskTempAlerts {
		active[k] = v
	}
	m.mu.Unlock()

	for hdNo, tempC := range diskTemps {
		over := float64(tempC) > threshold
		st, hasActive := active[hdNo]

		if over && !hasActive {
			// New alert
			now := time.Now().Unix()
			a := &store.Alert{TS: now, Type: TypeDiskTempHigh, HDNo: hdNo, Value: float64(tempC), Threshold: threshold}
			id, err := m.store.InsertAlert(ctx, a)
			if err != nil {
				log.Printf("alert: insert disk temp alert failed: %v", err)
				continue
			}
			m.mu.Lock()
			m.diskTempAlerts[hdNo] = diskAlertState{alertID: id, peak: float64(tempC)}
			m.lastEvent = &Event{ID: id, TS: now, Type: TypeDiskTempHigh, HDNo: hdNo, Value: float64(tempC), Threshold: threshold}
			m.mu.Unlock()

			if webhook != "" {
				go m.sendWeCom(ctx, id, TypeDiskTempHigh, float64(tempC), threshold, diskAliases[hdNo], webhook)
			}
		} else if over && hasActive {
			// Still over — update peak
			_ = m.store.UpdateAlertPeak(ctx, st.alertID, float64(tempC))
			if float64(tempC) > st.peak {
				st.peak = float64(tempC)
				m.mu.Lock()
				m.diskTempAlerts[hdNo] = st
				m.mu.Unlock()
			}
		} else if !over && hasActive {
			// Recovered — close the alert
			now := time.Now().Unix()
			peak := st.peak
			_ = m.store.CloseAlert(ctx, st.alertID, now, peak)
			m.mu.Lock()
			delete(m.diskTempAlerts, hdNo)
			m.mu.Unlock()

			if webhook != "" {
				go m.sendWeCom(ctx, st.alertID, TypeDiskTempHigh, peak, threshold, diskAliases[hdNo], webhook)
			}
		}
	}
}

// EvaluateDiskHealth checks each disk's health and fires alerts on state transitions.
// diskAliases maps hdNo to display alias.
func (m *Manager) EvaluateDiskHealth(ctx context.Context, disks map[string]string, diskAliases map[string]string) {
	m.mu.Lock()
	webhook := m.webhookURL
	active := make(map[string]int64, len(m.diskHealthAlerts))
	for k, v := range m.diskHealthAlerts {
		active[k] = v
	}
	m.mu.Unlock()

	for hdNo, health := range disks {
		bad := health != "OK"
		_, hasActive := active[hdNo]

		if bad && !hasActive {
			now := time.Now().Unix()
			a := &store.Alert{TS: now, Type: TypeDiskHealth, HDNo: hdNo, Value: 0, Threshold: 0}
			id, err := m.store.InsertAlert(ctx, a)
			if err != nil {
				log.Printf("alert: insert disk health alert failed: %v", err)
				continue
			}
			m.mu.Lock()
			m.diskHealthAlerts[hdNo] = id
			m.lastEvent = &Event{ID: id, TS: now, Type: TypeDiskHealth, HDNo: hdNo, Value: 0, Threshold: 0}
			m.mu.Unlock()

			if webhook != "" {
				go m.sendDiskHealthWeCom(ctx, id, diskAliases[hdNo], health, true, webhook)
			}
		} else if !bad && hasActive {
			now := time.Now().Unix()
			alertID := active[hdNo]
			_ = m.store.CloseAlert(ctx, alertID, now, 0)
			m.mu.Lock()
			delete(m.diskHealthAlerts, hdNo)
			m.mu.Unlock()

			if webhook != "" {
				go m.sendDiskHealthWeCom(ctx, alertID, diskAliases[hdNo], health, false, webhook)
			}
		}
	}
}

func (m *Manager) sendWeCom(_ context.Context, alertID int64, alertType string, value, threshold float64, diskName, webhook string) {
	var title, emoji, content string
	if alertType == TypeTempHigh && diskName == "" {
		title = "QNAP 系统温度告警"
		emoji = "🔥"
		content = fmt.Sprintf("## %s %s\n> 当前温度：**%.1f°C**\n> 阈值：%.1f°C\n> 时间：%s",
			emoji, title, value, threshold, time.Now().Format("2006-01-02 15:04:05"))
	} else {
		title = "QNAP 硬盘温度告警"
		emoji = "🌡️"
		content = fmt.Sprintf("## %s %s\n> 硬盘：**%s**\n> 温度：**%.1f°C**\n> 阈值：%.1f°C\n> 时间：%s",
			emoji, title, diskName, value, threshold, time.Now().Format("2006-01-02 15:04:05"))
	}

	body, _ := json.Marshal(map[string]any{
		"msgtype":  "markdown",
		"markdown": map[string]string{"content": content},
	})

	cctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodPost, webhook, bytes.NewReader(body))
	if err != nil {
		log.Printf("alert: build webhook request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("alert: webhook send failed: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_ = m.store.MarkAlertWebhookSent(cctx, alertID)
	} else {
		log.Printf("alert: webhook returned status %d", resp.StatusCode)
	}
}

func (m *Manager) sendDiskHealthWeCom(_ context.Context, alertID int64, diskName, health string, isBad bool, webhook string) {
	var title, emoji string
	if isBad {
		title = "QNAP 硬盘健康告警"
		emoji = "⚠️"
	} else {
		title = "QNAP 硬盘健康恢复"
		emoji = "✅"
	}
	content := fmt.Sprintf("## %s %s\n> 硬盘：**%s**\n> 状态：**%s**\n> 时间：%s",
		emoji, title, diskName, health, time.Now().Format("2006-01-02 15:04:05"))

	body, _ := json.Marshal(map[string]any{
		"msgtype":  "markdown",
		"markdown": map[string]string{"content": content},
	})

	cctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodPost, webhook, bytes.NewReader(body))
	if err != nil {
		log.Printf("alert: build disk health webhook request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("alert: disk health webhook send failed: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_ = m.store.MarkAlertWebhookSent(cctx, alertID)
	} else {
		log.Printf("alert: disk health webhook returned status %d", resp.StatusCode)
	}
}
