package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type compareRecord struct {
	Timestamp       string `json:"timestamp"`
	RequestID       string `json:"request_id"`
	Method          string `json:"method"`
	Path            string `json:"path"`
	PythonStatus    int    `json:"python_status"`
	GoStatus        int    `json:"go_status"`
	PythonLatencyMS int64  `json:"python_latency_ms"`
	GoLatencyMS     int64  `json:"go_latency_ms"`
	StatusMismatch  bool   `json:"status_mismatch"`
	LatencyGapMS    int64  `json:"latency_gap_ms"`
	Discrepancy     bool   `json:"discrepancy"`
	Reason          string `json:"reason,omitempty"`
	RequestBody     string `json:"request_body,omitempty"`
	RequestTrunc    bool   `json:"request_body_truncated,omitempty"`
	PythonError     string `json:"python_error,omitempty"`
	GoError         string `json:"go_error,omitempty"`
}

type sink struct {
	mu       sync.RWMutex
	records  []compareRecord
	max      int
	filePath string
}

func newSink(path string, max int) *sink {
	if max <= 0 {
		max = 1000
	}
	return &sink{records: make([]compareRecord, 0, max), max: max, filePath: path}
}

func (s *sink) add(rec compareRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.records) >= s.max {
		copy(s.records, s.records[1:])
		s.records = s.records[:s.max-1]
	}
	s.records = append(s.records, rec)

	if s.filePath != "" {
		f, err := os.OpenFile(s.filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			enc := json.NewEncoder(f)
			_ = enc.Encode(rec)
			_ = f.Close()
		}
	}
}

func (s *sink) latest(limit int, discrepanciesOnly bool) []compareRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}
	if limit > s.max {
		limit = s.max
	}

	out := make([]compareRecord, 0, limit)
	for i := len(s.records) - 1; i >= 0 && len(out) < limit; i-- {
		rec := s.records[i]
		if discrepanciesOnly && !rec.Discrepancy {
			continue
		}
		out = append(out, rec)
	}
	return out
}

type proxyServer struct {
	pythonBase *url.URL
	goBase     *url.URL
	client     *http.Client
	sink       *sink

	bodyCaptureLimit int
	latencyGapMS     int64
}

func main() {
	listenAddr := envOr("COMPARE_LISTEN_ADDR", ":8080")
	pythonURL := envOr("PYTHON_BACKEND_URL", "http://web:8000")
	goURL := envOr("GO_BACKEND_URL", "http://web-go:8000")
	sinkFile := envOr("COMPARE_SINK_FILE", "/tmp/compare-results.jsonl")
	maxRecords := envIntOr("COMPARE_MAX_RECORDS", 3000)
	bodyCaptureLimit := envIntOr("COMPARE_CAPTURE_BODY_LIMIT", 65536)
	latencyGap := int64(envIntOr("COMPARE_LATENCY_GAP_MS", 500))

	py, err := url.Parse(pythonURL)
	if err != nil {
		slog.Error("invalid PYTHON_BACKEND_URL", "error", err)
		os.Exit(1)
	}
	goBackend, err := url.Parse(goURL)
	if err != nil {
		slog.Error("invalid GO_BACKEND_URL", "error", err)
		os.Exit(1)
	}

	s := &proxyServer{
		pythonBase: py,
		goBase:     goBackend,
		client: &http.Client{
			Timeout: 180 * time.Second,
		},
		sink:             newSink(sinkFile, maxRecords),
		bodyCaptureLimit: bodyCaptureLimit,
		latencyGapMS:     latencyGap,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /_compare/results", s.getResults)
	mux.HandleFunc("GET /_compare/discrepancies", s.getDiscrepancies)
	mux.HandleFunc("/", s.handleProxy)

	httpServer := &http.Server{
		Addr:         listenAddr,
		Handler:      withLogging(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 180 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("compare proxy started", "addr", listenAddr, "python", pythonURL, "go", goURL)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("compare proxy failed", "error", err)
			os.Exit(1)
		}
	}()

	<-context.Background().Done()
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("proxy request", "method", r.Method, "path", r.URL.Path, "duration_ms", time.Since(start).Milliseconds())
	})
}

func (s *proxyServer) handleProxy(w http.ResponseWriter, r *http.Request) {
	requestID := strings.TrimSpace(r.Header.Get("X-Request-Id"))
	if requestID == "" {
		requestID = randomID()
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	_ = r.Body.Close()

	pyResp, pyLatency, pyErr := s.forward(r.Context(), s.pythonBase, r, body, requestID)
	if pyErr != nil {
		http.Error(w, "python backend error: "+pyErr.Error(), http.StatusBadGateway)
		go s.captureShadowAsync(requestID, r, body, pyLatency, nil, pyErr)
		return
	}

	go s.captureShadowAsync(requestID, r, body, pyLatency, pyResp, nil)

	copyHeaders(w.Header(), pyResp.headers)
	w.Header().Set("X-Request-Id", requestID)
	w.WriteHeader(pyResp.status)
	_, _ = w.Write(pyResp.body)
}

func (s *proxyServer) captureShadowAsync(requestID string, r *http.Request, body []byte, pyLatency time.Duration, pyResp *forwardResp, pyErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	goResp, goLatency, goErr := s.forward(ctx, s.goBase, r, body, requestID)

	rec := compareRecord{
		Timestamp:       time.Now().UTC().Format(time.RFC3339Nano),
		RequestID:       requestID,
		Method:          r.Method,
		Path:            r.URL.RequestURI(),
		PythonLatencyMS: pyLatency.Milliseconds(),
		GoLatencyMS:     goLatency.Milliseconds(),
	}

	if pyResp != nil {
		rec.PythonStatus = pyResp.status
	}
	if goResp != nil {
		rec.GoStatus = goResp.status
	}
	if pyErr != nil {
		rec.PythonError = pyErr.Error()
	}
	if goErr != nil {
		rec.GoError = goErr.Error()
	}

	bodyText, trunc := safeBodyString(body, s.bodyCaptureLimit)
	rec.RequestBody = bodyText
	rec.RequestTrunc = trunc

	rec.LatencyGapMS = abs64(rec.PythonLatencyMS - rec.GoLatencyMS)
	rec.StatusMismatch = rec.PythonStatus != rec.GoStatus

	if rec.PythonError != "" || rec.GoError != "" {
		rec.Discrepancy = true
		rec.Reason = "backend_error"
	} else if rec.StatusMismatch {
		rec.Discrepancy = true
		rec.Reason = "status_mismatch"
	} else if rec.LatencyGapMS > s.latencyGapMS {
		rec.Discrepancy = true
		rec.Reason = "latency_gap"
	}

	s.sink.add(rec)
}

type forwardResp struct {
	status  int
	headers http.Header
	body    []byte
}

func (s *proxyServer) forward(ctx context.Context, base *url.URL, orig *http.Request, body []byte, requestID string) (*forwardResp, time.Duration, error) {
	start := time.Now()

	target := *base
	target.Path = strings.TrimSuffix(base.Path, "/") + orig.URL.Path
	target.RawQuery = orig.URL.RawQuery

	req, err := http.NewRequestWithContext(ctx, orig.Method, target.String(), bytesReader(body))
	if err != nil {
		return nil, time.Since(start), err
	}
	copyHeaders(req.Header, orig.Header)
	req.Header.Set("X-Request-Id", requestID)
	req.Host = base.Host

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, time.Since(start), err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, time.Since(start), err
	}

	return &forwardResp{status: resp.StatusCode, headers: resp.Header.Clone(), body: respBody}, time.Since(start), nil
}

func (s *proxyServer) getResults(w http.ResponseWriter, r *http.Request) {
	limit := envIntOrFromQuery(r, "limit", 100)
	writeJSON(w, http.StatusOK, map[string]any{"items": s.sink.latest(limit, false)})
}

func (s *proxyServer) getDiscrepancies(w http.ResponseWriter, r *http.Request) {
	limit := envIntOrFromQuery(r, "limit", 100)
	writeJSON(w, http.StatusOK, map[string]any{"items": s.sink.latest(limit, true)})
}

func copyHeaders(dst, src http.Header) {
	for k := range dst {
		dst.Del(k)
	}
	for k, values := range src {
		if strings.EqualFold(k, "Content-Length") {
			continue
		}
		for _, v := range values {
			dst.Add(k, v)
		}
	}
}

func bytesReader(body []byte) io.Reader {
	if len(body) == 0 {
		return nil
	}
	return strings.NewReader(string(body))
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func envOr(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func envIntOr(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envIntOrFromQuery(r *http.Request, key string, fallback int) int {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b)
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func safeBodyString(b []byte, limit int) (string, bool) {
	if len(b) <= limit {
		return string(b), false
	}
	return string(b[:limit]), true
}
