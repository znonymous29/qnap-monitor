package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/qnap-monitor/backend/internal/config"
	"github.com/qnap-monitor/backend/internal/qnap"
)

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	v := s.Config.Current().View()
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	var u config.Update
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	cfg, err := s.Config.Apply(r.Context(), u)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, cfg.View())
}

type testReq struct {
	QNAPURL      string `json:"qnapUrl"`
	QNAPUser     string `json:"qnapUser"`
	QNAPPassword string `json:"qnapPassword"`
}

type testResp struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// handleTestConfig attempts a login + single fetch with submitted creds without persisting them.
// If password is empty, falls back to the saved password (so the user can re-test without retyping).
func (s *Server) handleTestConfig(w http.ResponseWriter, r *http.Request) {
	var req testReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	pw := req.QNAPPassword
	if pw == "" {
		pw = s.Config.Current().QNAPPassword
	}
	cli := qnap.New(req.QNAPURL, req.QNAPUser, pw)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if _, err := cli.FetchAll(ctx); err != nil {
		writeJSON(w, http.StatusOK, testResp{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, testResp{OK: true})
}
