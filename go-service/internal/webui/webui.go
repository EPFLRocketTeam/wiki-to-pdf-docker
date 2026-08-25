package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed assets/*
var embeddedAssets embed.FS

type UI struct {
	assetFS fs.FS
}

func New() (*UI, error) {
	sub, err := fs.Sub(embeddedAssets, "assets")
	if err != nil {
		return nil, err
	}
	return &UI{assetFS: sub}, nil
}

func (u *UI) ServeFile(w http.ResponseWriter, name, contentType string) {
	data, err := fs.ReadFile(u.assetFS, name)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (u *UI) StaticHandler() http.Handler {
	return http.FileServer(http.FS(u.assetFS))
}
