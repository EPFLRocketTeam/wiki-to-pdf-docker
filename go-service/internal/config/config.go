package config

import (
	"errors"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr        string
	RedisAddr         string
	RedisDB           int
	AllowedOrigins    []string
	RequestBodyLimit  int64
	ToolTimeout       time.Duration
	PandocBinary      string
	LuaLatexBinary    string
	LuaFilterPath     string
	BaseTemplatePath  string
	AssetsTemplateDir string
	ERTWikiRoot       string
	LogLevel          slog.Level
}

func Load() (Config, error) {
	cfg := Config{
		ListenAddr:        envOrDefault("LISTEN_ADDR", ":8000"),
		RedisAddr:         envOrDefault("REDIS_ADDR", "127.0.0.1:6379"),
		RedisDB:           envIntOrDefault("REDIS_DB", 0),
		AllowedOrigins:    splitCSV(envOrDefault("CORS_ALLOWED_ORIGINS", "https://rocket-team.epfl.ch")),
		RequestBodyLimit:  envInt64OrDefault("REQUEST_BODY_LIMIT_BYTES", 100*1024*1024),
		ToolTimeout:       envDurationOrDefault("TOOL_TIMEOUT", 90*time.Second),
		PandocBinary:      envOrDefault("PANDOC_BINARY", "pandoc"),
		LuaLatexBinary:    envOrDefault("LUALATEX_BINARY", "lualatex"),
		LuaFilterPath:     envOrDefault("LUA_FILTER_PATH", "/app/ImageLuaFilter.lua"),
		BaseTemplatePath:  envOrDefault("BASE_TEMPLATE_PATH", "/app/latex_templates/base.tex"),
		AssetsTemplateDir: envOrDefault("ASSETS_TEMPLATE_DIR", "/app/latex_templates/template_images"),
		ERTWikiRoot:       envOrDefault("ERT_WIKI_ROOT", "/app/ert_wiki"),
		LogLevel:          parseLogLevel(envOrDefault("LOG_LEVEL", "info")),
	}

	if cfg.ListenAddr == "" {
		return Config{}, errors.New("LISTEN_ADDR must not be empty")
	}

	return cfg, nil
}

func envOrDefault(k, fallback string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return fallback
	}
	return v
}

func envIntOrDefault(k string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envInt64OrDefault(k string, fallback int64) int64 {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

func envDurationOrDefault(k string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return []string{"*"}
	}
	return out
}

func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
