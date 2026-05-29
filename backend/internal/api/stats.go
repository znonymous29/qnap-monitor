package api

import (
	"net/http"
	"time"
)

type statsResp struct {
	Period  string      `json:"period"`
	Entries []statsEntry `json:"entries"`
}

type statsEntry struct {
	PeriodStart int64   `json:"periodStart"`
	CPUAvg      float64 `json:"cpuAvg"`
	CPUMax      float64 `json:"cpuMax"`
	MemAvg      float64 `json:"memAvg"`
	MemMax      float64 `json:"memMax"`
	TempAvg     float64 `json:"tempAvg"`
	TempMax     float64 `json:"tempMax"`
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "day"
	}

	now := time.Now()
	var from int64
	var bucketSec int
	var entries int

	switch period {
	case "day":
		// Last 24h, hourly buckets
		from = now.Add(-24 * time.Hour).Unix()
		bucketSec = 3600
		entries = 24
	case "week":
		// Last 7 days, daily buckets
		from = now.Add(-7 * 24 * time.Hour).Unix()
		bucketSec = 86400
		entries = 7
	case "month":
		// Last 30 days, daily buckets
		from = now.Add(-30 * 24 * time.Hour).Unix()
		bucketSec = 86400
		entries = 30
	default:
		writeErr(w, http.StatusBadRequest, nil)
		return
	}

	to := now.Unix()
	rows, err := s.Store.QueryMetrics(r.Context(), from, to, bucketSec)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	result := make([]statsEntry, 0, entries)
	for _, row := range rows {
		result = append(result, statsEntry{
			PeriodStart: row.TS,
			CPUAvg:      row.CPUUsage,
			CPUMax:      row.CPUUsage, // QueryMetrics already aggregates with AVG/MAX
			MemAvg:      row.MemUsage,
			MemMax:      row.MemUsage,
			TempAvg:     row.SysTempC,
			TempMax:     row.SysTempC,
		})
	}

	writeJSON(w, http.StatusOK, statsResp{Period: period, Entries: result})
}
