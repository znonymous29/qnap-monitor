package api

import (
	"encoding/json"
	"net/http"
)

type statusResp struct {
	Configured bool            `json:"configured"`
	LastError  string          `json:"lastError,omitempty"`
	Metric     *metricView     `json:"metric,omitempty"`
	SystemInfo *systemInfoView `json:"systemInfo,omitempty"`
	Disks      []diskView      `json:"disks,omitempty"`
	Volumes    []volumeView    `json:"volumes,omitempty"`
	Alert      alertState      `json:"alert"`
}

type systemInfoView struct {
	Model         string `json:"model"`
	SerialNumber  string `json:"serialNumber"`
	Firmware      string `json:"firmware"`
	UptimeSeconds int64  `json:"uptimeSeconds"`
}

type metricView struct {
	TS               int64   `json:"ts"`
	CPUUsage         float64 `json:"cpuUsage"`
	MemUsage         float64 `json:"memUsage"`
	SysTempC         float64 `json:"sysTempC"`
	CPUTempC         float64 `json:"cpuTempC"`
	FanRPM           int     `json:"fanRpm"`
	VolumeTotalBytes int64   `json:"volumeTotalBytes"`
	VolumeUsedBytes  int64   `json:"volumeUsedBytes"`
	VolumeUsagePct   float64 `json:"volumeUsagePct"`
}

type diskView struct {
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
	TempC             int    `json:"tempC"`
	PowerOnHours      int64  `json:"powerOnHours"`
	ReallocatedSectors int64  `json:"reallocatedSectors"`
}

type volumeView struct {
	VolNo         int     `json:"volNo"`
	Label         string  `json:"label"`
	CapacityBytes int64   `json:"capacityBytes"`
	UsedBytes     int64   `json:"usedBytes"`
	FreeBytes     int64   `json:"freeBytes"`
	UsedPct       float64 `json:"usedPct"`
	Filesystem    string  `json:"filesystem"`
	RaidLevel     int     `json:"raidLevel"`
	HDList        string  `json:"hdList"`
}

type alertState struct {
	InAlert              bool        `json:"inAlert"`
	Threshold            float64     `json:"threshold"`
	DiskTempThreshold    float64     `json:"diskTempThreshold"`
	CPUTempThreshold     float64     `json:"cpuTempThreshold"`
	Event                interface{} `json:"event,omitempty"`
	DiskHealthAlerts     []diskHealthAlert `json:"diskHealthAlerts,omitempty"`
}

type diskHealthAlert struct {
	HDNo   string `json:"hdNo"`
	Health string `json:"health"`
}

func (s *Server) handleStatusCurrent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cfg := s.Config.Current()
	resp := statusResp{
		Configured: cfg.QNAPURL != "" && cfg.QNAPUser != "" && cfg.QNAPPassword != "",
		LastError:  s.Collector.LastError(),
	}

	metric, err := s.Store.LatestMetric(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if metric != nil {
		resp.Metric = &metricView{
			TS:               metric.TS,
			CPUUsage:         metric.CPUUsage,
			MemUsage:         metric.MemUsage,
			SysTempC:         metric.SysTempC,
			CPUTempC:         metric.CPUTempC,
			FanRPM:           metric.FanRPM,
			VolumeTotalBytes: metric.VolumeTotalBytes,
			VolumeUsedBytes:  metric.VolumeUsedBytes,
			VolumeUsagePct:   metric.VolumeUsagePct,
		}
	}

	// System info
	sysInfo, err := s.Store.GetSystemInfo(ctx)
	if err == nil && sysInfo != nil {
		resp.SystemInfo = &systemInfoView{
			Model:         sysInfo.Model,
			SerialNumber:  sysInfo.SerialNumber,
			Firmware:      sysInfo.Firmware,
			UptimeSeconds: sysInfo.UptimeSeconds,
		}
	}

	// Disks with latest temperature
	disks, err := s.Store.ListDisks(ctx)
	if err == nil {
		temps, _ := s.Store.GetLatestDiskTemps(ctx)
		for _, d := range disks {
			dv := diskView{
				HDNo: d.HDNo, Alias: d.Alias, Model: d.Model, Serial: d.Serial,
				Firmware: d.Firmware, Vendor: d.Vendor, Capacity: d.Capacity, CapacityBytes: d.CapacityBytes,
				Health: d.Health, IsSSD: d.IsSSD, TempC: temps[d.HDNo],
				PowerOnHours: d.PowerOnHours, ReallocatedSectors: d.ReallocatedSectors,
			}
			resp.Disks = append(resp.Disks, dv)
		}
	}

	// Volumes
	volumes, err := s.Store.LatestVolumeDetails(ctx)
	if err == nil {
		for _, v := range volumes {
			resp.Volumes = append(resp.Volumes, volumeView{
				VolNo: v.VolNo, Label: v.Label, CapacityBytes: v.CapacityBytes,
				UsedBytes: v.UsedBytes, FreeBytes: v.FreeBytes, UsedPct: v.UsedPct,
				Filesystem: v.Filesystem, RaidLevel: v.RaidLevel, HDList: v.HDList,
			})
		}
	}

	inAlert, threshold, diskTempThreshold, cpuTempThreshold, _ := s.Alerts.State()
	resp.Alert.InAlert = inAlert
	resp.Alert.Threshold = threshold
	resp.Alert.DiskTempThreshold = diskTempThreshold
	resp.Alert.CPUTempThreshold = cpuTempThreshold
	if ev := s.Alerts.ConsumeLastEvent(); ev != nil {
		resp.Alert.Event = ev
	}

	// Disk health alerts
	unhealthy := s.Alerts.UnhealthyDisks()
	if len(unhealthy) > 0 {
		healthMap := make(map[string]string)
		for _, d := range resp.Disks {
			healthMap[d.HDNo] = d.Health
		}
		for hdNo := range unhealthy {
			resp.Alert.DiskHealthAlerts = append(resp.Alert.DiskHealthAlerts, diskHealthAlert{
				HDNo:   hdNo,
				Health: healthMap[hdNo],
			})
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}
