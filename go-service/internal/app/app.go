package app

import (
	"context"
	"net/http"
	"time"

	"github.com/epflrocketteam/wiki-to-pdf-go/internal/config"
	"github.com/epflrocketteam/wiki-to-pdf-go/internal/conversion"
	"github.com/epflrocketteam/wiki-to-pdf-go/internal/httpapi"
	"github.com/epflrocketteam/wiki-to-pdf-go/internal/store"
	"github.com/epflrocketteam/wiki-to-pdf-go/internal/webui"
	"github.com/epflrocketteam/wiki-to-pdf-go/internal/wiki"
	"github.com/redis/go-redis/v9"
)

type App struct {
	cfg    config.Config
	router http.Handler
	redis  *redis.Client
}

func New(cfg config.Config) (*App, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.RedisAddr,
		DB:   cfg.RedisDB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	sessionStore := store.NewRedisStore(rdb)
	wikiClient := wiki.NewClient(20 * time.Second)
	converter := conversion.NewConverter(cfg)
	ui, err := webui.New()
	if err != nil {
		return nil, err
	}

	h := httpapi.NewHandlers(cfg, sessionStore, wikiClient, converter, ui)
	router := httpapi.NewRouter(cfg, h)

	return &App{cfg: cfg, router: router, redis: rdb}, nil
}

func (a *App) Router() http.Handler {
	return a.router
}

func (a *App) Close() {
	if a.redis != nil {
		_ = a.redis.Close()
	}
}
