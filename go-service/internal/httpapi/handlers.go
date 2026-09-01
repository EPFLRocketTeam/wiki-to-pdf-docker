package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
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

func (h *Handlers) EditorSessionTutorial(w http.ResponseWriter, _ *http.Request) {
	h.ui.ServeFile(w, "editor_session_tutorial.html", "text/html; charset=utf-8")
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
	if err := validateImages(req.Images); err != nil {
		writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}
	if strings.TrimSpace(req.ImageAuthToken) != "" && strings.TrimSpace(req.ImageBaseURL) == "" && strings.TrimSpace(req.EditorSessionID) == "" {
		writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "imageBaseUrl is required when imageAuthToken is provided"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.ToolTimeout)
	defer cancel()
	if err := h.applyEditorImageSource(ctx, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}

	result, err := h.converter.ConvertAndPackage(ctx, req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: err.Error()})
		return
	}

	zipData, err := os.ReadFile(result.ZipPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: fmt.Sprintf("failed reading conversion assets: %v", err)})
		return
	}
	defer os.Remove(result.ZipPath)
	if err := h.store.PutZipData(ctx, result.SessionID, zipData, 10*time.Minute); err != nil {
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

	var zipData []byte
	if req.AssetSessionID != "" {
		var err error
		zipData, err = h.store.GetZipData(ctx, req.AssetSessionID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, model.ErrorResponse{Error: "conversion assets not found or expired"})
			return
		}
	}

	pdfBytes, err := h.converter.GeneratePDF(ctx, req.LatexCode, zipData)
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
	w.Header().Set("Content-Length", strconv.Itoa(len(pdfBytes)))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename+".pdf"))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(pdfBytes); err != nil {
		slog.Error("failed writing PDF response", "error", err, "title", filename, "bytes", len(pdfBytes))
	}
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
	if err := validateImages(req.Images); err != nil {
		writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}
	if strings.TrimSpace(req.ImageAuthToken) != "" && strings.TrimSpace(req.ImageBaseURL) == "" {
		writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "imageBaseUrl is required when imageAuthToken is provided"})
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
			ImageBaseURL:       strings.TrimSpace(req.ImageBaseURL),
			ImageTokenSaved:    strings.TrimSpace(req.ImageAuthToken) != "",
			ImagePaths:         imagePaths(req.Images),
		},
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	id, err := h.store.PutJSON(ctx, session, 24*time.Hour)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: fmt.Sprintf("failed storing editor session: %v", err)})
		return
	}
	if strings.TrimSpace(req.ImageBaseURL) != "" || strings.TrimSpace(req.ImageAuthToken) != "" || len(req.Images) != 0 {
		imageSource := model.ImageSource{
			BaseURL:   strings.TrimSpace(req.ImageBaseURL),
			AuthToken: strings.TrimSpace(req.ImageAuthToken),
			Images:    req.Images,
		}
		if err := h.store.PutEditorImageSource(ctx, id, imageSource, 24*time.Hour); err != nil {
			writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: fmt.Sprintf("failed storing private image source: %v", err)})
			return
		}
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

	data, err := h.store.GetZipData(ctx, sessionID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, model.ErrorResponse{Error: "project zip file not found or expired"})
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=project.zip")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)

	_ = h.store.DeleteZipData(context.Background(), sessionID)
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

	var session model.EditorSession
	if err := json.Unmarshal(raw, &session); err != nil {
		writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: "invalid editor session"})
		return
	}
	if imageRaw, err := h.store.GetEditorImageSource(ctx, sessionID); err == nil {
		var imageSource model.ImageSource
		if err := json.Unmarshal(imageRaw, &imageSource); err != nil {
			writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: "invalid private image source"})
			return
		}
		if strings.TrimSpace(session.Settings.ImageBaseURL) == "" {
			session.Settings.ImageBaseURL = imageSource.BaseURL
		}
		session.Settings.ImageTokenSaved = strings.TrimSpace(imageSource.AuthToken) != ""
		session.Settings.ImagePaths = imagePaths(imageSource.Images)
	}
	writeJSON(w, http.StatusOK, session)
}

// SessionImage serves one image kept in the private image record for an
// editor session. The image data is never included in the session JSON.
func (h *Handlers) SessionImage(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.PathValue("session_id"))
	imagePath := strings.TrimSpace(r.URL.Query().Get("path"))
	if sessionID == "" || imagePath == "" {
		writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "session_id and image path are required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	raw, err := h.store.GetEditorImageSource(ctx, sessionID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, model.ErrorResponse{Error: "session image not found"})
		return
	}

	var source model.ImageSource
	if err := json.Unmarshal(raw, &source); err != nil {
		writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: "invalid private image source"})
		return
	}
	for _, image := range source.Images {
		if image.Path == imagePath {
			w.Header().Set("Content-Type", http.DetectContentType(image.Content))
			w.Header().Set("Cache-Control", "private, max-age=300")
			_, _ = w.Write(image.Content)
			return
		}
	}
	writeJSON(w, http.StatusNotFound, model.ErrorResponse{Error: "session image not found"})
}

func (h *Handlers) applyEditorImageSource(ctx context.Context, req *model.ConvertRequest) error {
	if req.EditorSessionID != "" {
		raw, err := h.store.GetEditorImageSource(ctx, req.EditorSessionID)
		if err == nil {
			var imageSource model.ImageSource
			if err := json.Unmarshal(raw, &imageSource); err != nil {
				return fmt.Errorf("invalid private image source")
			}
			if strings.TrimSpace(req.ImageBaseURL) == "" {
				req.ImageBaseURL = imageSource.BaseURL
			}
			if strings.TrimSpace(req.ImageAuthToken) == "" {
				req.ImageAuthToken = imageSource.AuthToken
			}
			if len(req.Images) == 0 {
				req.Images = imageSource.Images
			}
		}
	}
	if strings.TrimSpace(req.ImageAuthToken) != "" && strings.TrimSpace(req.ImageBaseURL) == "" {
		return fmt.Errorf("imageBaseUrl is required when imageAuthToken is provided")
	}
	return nil
}

func validateImages(images []model.ImageAsset) error {
	for _, image := range images {
		if strings.TrimSpace(image.Path) == "" {
			return fmt.Errorf("each image requires a path")
		}
		if len(image.Content) == 0 {
			return fmt.Errorf("image %q has no content", image.Path)
		}
	}
	return nil
}

func imagePaths(images []model.ImageAsset) []string {
	paths := make([]string, 0, len(images))
	seen := make(map[string]struct{}, len(images))
	for _, image := range images {
		path := strings.TrimSpace(image.Path)
		if path == "" {
			continue
		}
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	return paths
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
