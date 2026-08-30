package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/epflrocketteam/wiki-to-pdf-go/internal/config"
	"github.com/epflrocketteam/wiki-to-pdf-go/internal/conversion"
	"github.com/epflrocketteam/wiki-to-pdf-go/internal/model"
	"github.com/epflrocketteam/wiki-to-pdf-go/internal/store"
	"github.com/epflrocketteam/wiki-to-pdf-go/internal/webui"
	"github.com/epflrocketteam/wiki-to-pdf-go/internal/wiki"
)

type Handlers struct {
	cfg       config.Config
	store     store.SessionStore
	wiki      wiki.Client
	converter *conversion.Converter
	ui        *webui.UI
}

func NewHandlers(cfg config.Config, s store.SessionStore, w wiki.Client, c *conversion.Converter, ui *webui.UI) *Handlers {
	return &Handlers{cfg: cfg, store: s, wiki: w, converter: c, ui: ui}
}

func (h *Handlers) Index(w http.ResponseWriter, _ *http.Request) {
	h.ui.ServeFile(w, "index.html", "text/html; charset=utf-8")
}

func (h *Handlers) HowToGetAccessToken(w http.ResponseWriter, _ *http.Request) {
	h.ui.ServeFile(w, "how_to_get_access_token.html", "text/html; charset=utf-8")
}

func (h *Handlers) Edit(w http.ResponseWriter, _ *http.Request) {
	h.ui.ServeFile(w, "edit.html", "text/html; charset=utf-8")
}

func (h *Handlers) Healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) Readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := h.store.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, model.ErrorResponse{Error: "redis unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (h *Handlers) Fetch(w http.ResponseWriter, r *http.Request) {
	var req model.FetchRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}

	pages := wiki.ParseRocketURLs(req.URLs)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	contents, err := h.wiki.FetchContents(ctx, pages, req.GraphQLURL, req.Token)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: fmt.Sprintf("failed to fetch content: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, contents)
}

func (h *Handlers) GetAccessToken(w http.ResponseWriter, r *http.Request) {
	var req model.GetAccessTokenRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	token, err := h.wiki.GetAccessToken(ctx, req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

func (h *Handlers) Convert(w http.ResponseWriter, r *http.Request) {
	var req model.ConvertRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}
	if strings.TrimSpace(req.Markdown) == "" {
		writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "no markdown content provided"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.ToolTimeout)
	defer cancel()

	result, err := h.converter.ConvertAndPackage(ctx, req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: err.Error()})
		return
	}

	if err := h.store.PutZipPath(ctx, result.SessionID, result.ZipPath, 10*time.Minute); err != nil {
		writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: fmt.Sprintf("failed storing zip session: %v", err)})
		return
	}

	writeJSON(w, http.StatusOK, model.ConvertResponse{
		Latex:     result.Latex,
		Status:    "success",
		SessionID: result.SessionID,
	})
}

func (h *Handlers) GeneratePDF(w http.ResponseWriter, r *http.Request) {
	var req model.GeneratePDFRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}
	if strings.TrimSpace(req.LatexCode) == "" {
		writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "latex_code is required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.ToolTimeout)
	defer cancel()

	pdfBytes, err := h.converter.GeneratePDF(ctx, req.LatexCode)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: "failed to compile PDF", Message: err.Error()})
		return
	}

	filename := strings.TrimSpace(req.Title)
	if filename == "" {
		filename = "document"
	}
	filename = strings.ReplaceAll(filename, "\"", "")

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename+".pdf"))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdfBytes)
}

func (h *Handlers) Store(w http.ResponseWriter, r *http.Request) {
	var payload map[string]any
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	id, err := h.store.PutJSON(ctx, payload, 24*time.Hour)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, model.StoreResponse{SessionID: id})
}

func (h *Handlers) CreateEditorSession(w http.ResponseWriter, r *http.Request) {
	var req model.EditorSessionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}
	if strings.TrimSpace(req.Markdown) == "" {
		writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "markdown is required"})
		return
	}

	session := model.EditorSession{
		Page: model.EditorSessionPage{
			Content:    req.Markdown,
			Title:      req.Title,
			AuthorName: req.Author,
		},
		Settings: model.EditorSessionSettings{
			Template:           req.Template,
			Date:               req.Date,
			DocumentID:         req.DocumentID,
			FooterText:         req.FooterText,
			LineNumbersEnabled: req.LineNumbersEnabled,
		},
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	id, err := h.store.PutJSON(ctx, session, 24*time.Hour)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: fmt.Sprintf("failed storing editor session: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, model.EditorSessionResponse{
		SessionID: id,
		EditURL:   "/edit?session_id=" + id,
	})
}

func (h *Handlers) ServeZipProject(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.PathValue("session_id"))
	if sessionID == "" {
		writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "session_id is required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	zipPath, err := h.store.GetZipPath(ctx, sessionID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, model.ErrorResponse{Error: "project zip file not found or expired"})
		return
	}

	data, err := os.ReadFile(zipPath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, model.ErrorResponse{Error: "project zip file not found or expired"})
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=project.zip")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)

	_ = os.Remove(zipPath)
	_ = h.store.DeleteZipPath(context.Background(), sessionID)
}

func (h *Handlers) SessionByID(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.PathValue("session_id"))
	if sessionID == "" {
		writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "session_id is required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	raw, err := h.store.GetJSON(ctx, sessionID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, model.ErrorResponse{Error: "session not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

func decodeJSON(r *http.Request, out any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
