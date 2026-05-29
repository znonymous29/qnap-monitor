package api

import (
	"net/http"
	"strconv"
	"time"
)

// GET /api/disks/temps?hd_no=0:1&from=&to=&bucket=
func (s *Server) handleDiskTemps(w http.ResponseWriter, r *http.Request) {
	hdNo := r.URL.Query().Get("hd_no")
	if hdNo == "" {
		writeErr(w, http.StatusBadRequest, nil)
		return
	}
	from, to, bucket := parseTimeRange(r)
	bucketSec := bucketSeconds(bucket)

	rows, err := s.Store.QueryDiskTemps(r.Context(), hdNo, from, to, bucketSec)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"hdNo":   hdNo,
		"from":   from,
		"to":     to,
		"bucket": bucket,
		"points": rows,
	})
}

// GET /api/volumes/usage?vol_no=3&from=&to=&bucket=
func (s *Server) handleVolumeUsage(w http.ResponseWriter, r *http.Request) {
	volNoStr := r.URL.Query().Get("vol_no")
	if volNoStr == "" {
		writeErr(w, http.StatusBadRequest, nil)
		return
	}
	volNo, err := strconv.Atoi(volNoStr)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	from, to, bucket := parseTimeRange(r)
	bucketSec := bucketSeconds(bucket)

	rows, err := s.Store.QueryVolumeUsage(r.Context(), volNo, from, to, bucketSec)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"volNo":  volNo,
		"from":   from,
		"to":     to,
		"bucket": bucket,
		"points": rows,
	})
}

func parseTimeRange(r *http.Request) (from, to int64, bucket string) {
	q := r.URL.Query()
	now := time.Now().Unix()
	from = parseInt64Default(q.Get("from"), now-3600)
	to = parseInt64Default(q.Get("to"), now)
	bucket = q.Get("bucket")
	if bucket == "" {
		bucket = autoBucket(to - from)
	}
	return
}

func parseInt64Default(s string, def int64) int64 {
	if s == "" {
		return def
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return def
	}
	return v
}
