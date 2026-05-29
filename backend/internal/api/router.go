package api

import (
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/qnap-monitor/backend/internal/alert"
	"github.com/qnap-monitor/backend/internal/collector"
	"github.com/qnap-monitor/backend/internal/config"
	"github.com/qnap-monitor/backend/internal/store"
)

type Server struct {
	Store     *store.Store
	Config    *config.Manager
	Alerts    *alert.Manager
	Collector *collector.Collector
	StaticFS  fs.FS // optional: embedded frontend dist
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)

	r.Route("/api", func(r chi.Router) {
		r.Get("/status/current", s.handleStatusCurrent)
		r.Get("/metrics", s.handleMetrics)
		r.Get("/stats", s.handleStats)
		r.Get("/disks/temps", s.handleDiskTemps)
		r.Get("/volumes/usage", s.handleVolumeUsage)
		r.Get("/config", s.handleGetConfig)
		r.Put("/config", s.handlePutConfig)
		r.Post("/config/test", s.handleTestConfig)
		r.Get("/alerts", s.handleListAlerts)
		r.Post("/alerts/{id}/ack", s.handleAckAlert)
	})

	if s.StaticFS != nil {
		fileServer := http.FileServer(http.FS(s.StaticFS))
		// SPA fallback: any non-API path that doesn't match a real file → index.html
		r.Handle("/*", spaHandler(s.StaticFS, fileServer))
	}

	return r
}

// spaHandler serves files when they exist, otherwise rewrites the path to /index.html
// (so React Router routes like /history work on hard refresh).
func spaHandler(root fs.FS, fileServer http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}
		clean := path[1:]
		if f, err := root.Open(clean); err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		// fall back to index.html
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	}
}
