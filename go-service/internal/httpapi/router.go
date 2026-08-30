package httpapi

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/epflrocketteam/wiki-to-pdf-go/internal/config"
)

func NewRouter(cfg config.Config, h *Handlers) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /", h.Index)
	mux.HandleFunc("GET /how-to-get-access-token", h.HowToGetAccessToken)
	mux.HandleFunc("GET /edit", h.Edit)
	mux.HandleFunc("GET /api/sessions/{session_id}", h.SessionByID)
	mux.Handle("GET /ui/", http.StripPrefix("/ui/", h.ui.StaticHandler()))
	mux.HandleFunc("GET /healthz", h.Healthz)
	mux.HandleFunc("GET /readyz", h.Readyz)
	mux.HandleFunc("POST /fetch", h.Fetch)
	mux.HandleFunc("POST /get-access-token", h.GetAccessToken)
	mux.HandleFunc("POST /convert", h.Convert)
	mux.HandleFunc("POST /generate-pdf", h.GeneratePDF)
	mux.HandleFunc("POST /store", h.Store)
	mux.HandleFunc("POST /editor-sessions", h.CreateEditorSession)
	mux.HandleFunc("GET /serve-zip-project/{session_id}", h.ServeZipProject)

	handler := recoverMiddleware(mux)
	handler = requestLoggerMiddleware(handler)
	handler = corsMiddleware(handler, cfg.AllowedOrigins)
	handler = maxBodyMiddleware(handler, cfg.RequestBodyLimit)

	return handler
}

func requestLoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(ww, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered", "panic", rec, "stack", string(debug.Stack()))
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func corsMiddleware(next http.Handler, allowed []string) http.Handler {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, a := range allowed {
		allowedSet[a] = struct{}{}
	}
	allowAny := len(allowed) == 1 && allowed[0] == "*"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if allowAny {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else if origin != "" {
			if _, ok := allowedSet[origin]; ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			}
		}

		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func maxBodyMiddleware(next http.Handler, limit int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(statusCode int) {
	w.status = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}
