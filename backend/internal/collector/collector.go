package collector

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qnap-monitor/backend/internal/alert"
	"github.com/qnap-monitor/backend/internal/config"
	"github.com/qnap-monitor/backend/internal/qnap"
	"github.com/qnap-monitor/backend/internal/store"
)

type Collector struct {
	store   *store.Store
	cfgMgr  *config.Manager
	alerts  *alert.Manager

	mu        sync.Mutex
	client    *qnap.Client
	clientKey string

	lastErr   atomic.Value
}

func New(s *store.Store, cm *config.Manager, am *alert.Manager) *Collector {
	c := &Collector{store: s, cfgMgr: cm, alerts: am}
	c.lastErr.Store("")
	return c
}

func (c *Collector) LastError() string {
	v, _ := c.lastErr.Load().(string)
	return v
}

func (c *Collector) Run(ctx context.Context) {
	updates := c.cfgMgr.Subscribe()

	cfg := c.cfgMgr.Current()
	interval := intervalFromCfg(cfg)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	cleanup := time.NewTicker(1 * time.Hour)
	defer cleanup.Stop()

	c.rebuildClient(cfg)
	c.alerts.UpdateConfig(cfg.TempThresholdCelsius, cfg.DiskTempThresholdCelsius, cfg.CPUTempThresholdCelsius, cfg.WeComWebhookURL)

	c.tickOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.tickOnce(ctx)
		case <-cleanup.C:
			cfg := c.cfgMgr.Current()
			if n, err := c.store.PurgeOldData(ctx, cfg.RetentionDays); err != nil {
				log.Printf("collector: purge failed: %v", err)
			} else if n > 0 {
				log.Printf("collector: purged %d old metric/disk rows", n)
			}
			if n, err := c.store.PurgeOldAlerts(ctx, 7); err != nil {
				log.Printf("collector: purge alerts failed: %v", err)
			} else if n > 0 {
				log.Printf("collector: purged %d old alerts", n)
			}
		case <-updates:
			cfg := c.cfgMgr.Current()
			newInterval := intervalFromCfg(cfg)
			if newInterval != interval {
				interval = newInterval
				ticker.Reset(interval)
				log.Printf("collector: interval updated to %s", interval)
			}
			c.rebuildClient(cfg)
			c.alerts.UpdateConfig(cfg.TempThresholdCelsius, cfg.DiskTempThresholdCelsius, cfg.CPUTempThresholdCelsius, cfg.WeComWebhookURL)
		}
	}
}

func intervalFromCfg(cfg config.Config) time.Duration {
	n := cfg.CollectIntervalSeconds
	if n < 1 {
		n = 10
	}
	return time.Duration(n) * time.Second
}

func (c *Collector) rebuildClient(cfg config.Config) {
	key := cfg.QNAPURL + "|" + cfg.QNAPUser + "|" + cfg.QNAPPassword
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.clientKey == key {
		return
	}
	c.clientKey = key
	if cfg.QNAPURL == "" || cfg.QNAPUser == "" || cfg.QNAPPassword == "" {
		c.client = nil
		return
	}
	c.client = qnap.New(cfg.QNAPURL, cfg.QNAPUser, cfg.QNAPPassword)
}

func (c *Collector) tickOnce(parentCtx context.Context) {
	c.mu.Lock()
	cli := c.client
	c.mu.Unlock()
	if cli == nil {
		c.lastErr.Store("QNAP 未配置：请在设置页填写 URL / 用户名 / 密码")
		return
	}

	ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
	defer cancel()

	result, err := cli.FetchAll(ctx)
	if err != nil {
		c.lastErr.Store(err.Error())
		log.Printf("collector: fetch failed: %v", err)
		return
	}
	c.lastErr.Store("")

	now := time.Now().Unix()
	snap := result.Snap

	// 1. Disk info (upsert static metadata + insert temperature)
	diskHealthMap := make(map[string]string)
	diskTempMap := make(map[string]int)
	diskAliasMap := make(map[string]string)
	for _, d := range result.Disks {
		if d.HDNo == "" || d.DiskStatus == -5 {
			continue // skip empty slots
		}
		diskHealthMap[d.HDNo] = d.Health
		diskAliasMap[d.HDNo] = d.Alias
		if d.TempC > 0 {
			diskTempMap[d.HDNo] = d.TempC
		}
		if err := c.store.UpsertDisk(ctx, &store.DiskRow{
			HDNo:              d.HDNo,
			Alias:             d.Alias,
			Model:             d.Model,
			Serial:            d.Serial,
			Firmware:          d.Firmware,
			Vendor:            d.Vendor,
			Capacity:          d.Capacity,
			CapacityBytes:     d.CapacityBytes,
			Health:            d.Health,
			IsSSD:             d.IsSSD,
			DiskStatus:        d.DiskStatus,
			PowerOnHours:      d.PowerOnHours,
			ReallocatedSectors: d.ReallocatedSectors,
			UpdatedAt:         now,
		}); err != nil {
			log.Printf("collector: upsert disk %s: %v", d.HDNo, err)
		}
		if d.TempC > 0 {
			if err := c.store.InsertDiskTemp(ctx, now, d.HDNo, d.TempC); err != nil {
				log.Printf("collector: insert disk temp %s: %v", d.HDNo, err)
			}
		}
	}

	// 2. Volume details
	for _, v := range result.Volumes {
		if err := c.store.InsertVolumeDetail(ctx, &store.VolumeDetailRow{
			TS:            now,
			VolNo:         v.VolNo,
			Label:         v.Label,
			CapacityBytes: v.CapacityBytes,
			UsedBytes:     v.UsedBytes,
			HDList:        v.HDList,
			FreeBytes:     v.FreeBytes,
			UsedPct:       v.UsedPct,
			Filesystem:    v.Filesystem,
			RaidLevel:     v.RaidLevel,
			MountPath:     v.MountPath,
		}); err != nil {
			log.Printf("collector: insert volume detail %d: %v", v.VolNo, err)
		}
		// Add to total for system-level metrics
		snap.VolumeTotalBytes += v.CapacityBytes
		snap.VolumeUsedBytes += v.UsedBytes
	}

	// 3. System metrics (after volumes so VolumeUsagePct is accurate)
	var volumeUsagePct float64
	if snap.VolumeTotalBytes > 0 {
		volumeUsagePct = float64(snap.VolumeUsedBytes) / float64(snap.VolumeTotalBytes) * 100
	}
	if err := c.store.InsertMetric(ctx, &store.Snapshot{
		TS:               now,
		CPUUsage:         snap.CPUUsage,
		MemUsage:         snap.MemUsage,
		SysTempC:         snap.SysTempC,
		CPUTempC:         snap.CPUTempC,
		FanRPM:           snap.FanRPM,
		VolumeTotalBytes: snap.VolumeTotalBytes,
		VolumeUsedBytes:  snap.VolumeUsedBytes,
		VolumeUsagePct:   volumeUsagePct,
	}); err != nil {
		log.Printf("collector: insert metric: %v", err)
	}

	// 4. System info (static NAS metadata)
	if snap.Model != "" || snap.SerialNumber != "" {
		if err := c.store.UpsertSystemInfo(ctx, &store.SystemInfo{
			Model:         snap.Model,
			SerialNumber:  snap.SerialNumber,
			Firmware:      snap.FirmwareVersion,
			UptimeSeconds: snap.UptimeSeconds,
			UpdatedAt:     now,
		}); err != nil {
			log.Printf("collector: upsert system info: %v", err)
		}
	}

	// 5. Alert evaluation (system temp + CPU temp + disk health + disk temp)
	c.alerts.Evaluate(ctx, snap.SysTempC)
	c.alerts.EvaluateCPUTemp(ctx, snap.CPUTempC)
	c.alerts.EvaluateDiskHealth(ctx, diskHealthMap, diskAliasMap)
	c.alerts.EvaluateDiskTemps(ctx, diskTempMap, diskAliasMap)
}
