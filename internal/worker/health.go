package worker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// Healthy reports whether the worker is consuming (ready) and the AMQP connection is open.
func (a *App) Healthy() bool {
	if a == nil || a.conn == nil || a.conn.IsClosed() {
		return false
	}
	return a.ready.Load()
}

func (a *App) serveHealth(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", a.handleHealth)

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := srv.Shutdown(shutdownCtx)
		if err != nil {
			a.log.ErrorContext(ctx, "worker health server shutdown", "err", err)
		}
	}()

	err := srv.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (a *App) handleHealth(w http.ResponseWriter, _ *http.Request) {
	st := map[string]string{"status": "ok"}
	code := http.StatusOK
	if !a.Healthy() {
		st["status"] = "degraded"
		code = http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	err := json.NewEncoder(w).Encode(st)
	if err != nil {
		a.log.Error("worker health response encode", "err", err)
	}
}
