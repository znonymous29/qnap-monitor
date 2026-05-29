package api

import (
	"net/http"
	"strconv"
	"time"
)

type metricsResp struct {
	From   int64        `json:"from"`
	To     int64        `json:"to"`
	Bucket string       `json:"bucket"`
	Points []metricView `json:"points"`
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	now := time.Now().Unix()
	to := parseInt64(q.Get("to"), now)
	from := parseInt64(q.Get("from"), now-3600) // default last hour
	bucket := q.Get("bucket")
	if bucket == "" {
		bucket = autoBucket(to - from)
	}
	bucketSec := bucketSeconds(bucket)

	rows, err := s.Store.QueryMetrics(r.Context(), from, to, bucketSec)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	points := make([]metricView, 0, len(rows))
	for _, row := range rows {
		points = append(points, metricView{
			TS:               row.TS,
			CPUUsage:         row.CPUUsage,
			MemUsage:         row.MemUsage,
			SysTempC:         row.SysTempC,
			CPUTempC:         row.CPUTempC,
			FanRPM:           row.FanRPM,
			VolumeTotalBytes: row.VolumeTotalBytes,
			VolumeUsedBytes:  row.VolumeUsedBytes,
			VolumeUsagePct:   row.VolumeUsagePct,
		})
	}
	writeJSON(w, http.StatusOK, metricsResp{From: from, To: to, Bucket: bucket, Points: points})
}

func parseInt64(s string, def int64) int64 {
	if s == "" {
		return def
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return def
	}
	return v
}

// autoBucket picks a sensible aggregation level given the requested time range.
// Goal: keep returned points roughly <= 600 to stay snappy on the wire.
func autoBucket(rangeSec int64) string {
	switch {
	case rangeSec <= 3600: // 1h
		return "raw"
	case rangeSec <= 6*3600: // 6h
		return "1m"
	case rangeSec <= 24*3600: // 1d
		return "5m"
	case rangeSec <= 7*24*3600: // 7d
		return "30m"
	default: // 30d
		return "1h"
	}
}

func bucketSeconds(bucket string) int {
	switch bucket {
	case "raw":
		return 0
	case "1m":
		return 60
	case "5m":
		return 300
	case "30m":
		return 1800
	case "1h":
		return 3600
	default:
		return 0
	}
}
