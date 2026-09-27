package localapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"print-agent/internal/config"
	"print-agent/internal/infrastructure/printer"
)

const version = "0.1.0"

type Server struct {
	cfg     *config.Config
	logger  *slog.Logger
	printer printer.Printer
}

func NewServer(cfg *config.Config, logger *slog.Logger, p printer.Printer) *Server {
	return &Server{cfg: cfg, logger: logger, printer: p}
}

func (s *Server) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /info", s.handleInfo)
	mux.HandleFunc("POST /test-print", s.handleTestPrint)

	addr := fmt.Sprintf("%s:%d", s.cfg.LocalAPI.Host, s.cfg.LocalAPI.Port)
	srv := &http.Server{Addr: addr, Handler: mux}

	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()

	s.logger.Info("local api server started", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		s.logger.Error("local api server error", "err", err)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"agent_id": s.cfg.Agent.ID,
		"version":  version,
		"backend":  s.cfg.Backend.BaseURL,
	})
}

func (s *Server) handleTestPrint(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "not implemented"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
