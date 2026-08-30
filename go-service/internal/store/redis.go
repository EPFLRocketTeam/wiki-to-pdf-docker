package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type SessionStore interface {
	Ping(ctx context.Context) error
	PutJSON(ctx context.Context, payload any, ttl time.Duration) (string, error)
	GetJSON(ctx context.Context, sessionID string) (json.RawMessage, error)
	PutZipPath(ctx context.Context, sessionID, path string, ttl time.Duration) error
	GetZipPath(ctx context.Context, sessionID string) (string, error)
	DeleteZipPath(ctx context.Context, sessionID string) error
}

type RedisStore struct {
	rdb *redis.Client
}

func NewRedisStore(rdb *redis.Client) *RedisStore {
	return &RedisStore{rdb: rdb}
}

func (s *RedisStore) Ping(ctx context.Context) error {
	return s.rdb.Ping(ctx).Err()
}

func (s *RedisStore) PutJSON(ctx context.Context, payload any, ttl time.Duration) (string, error) {
	id := uuid.NewString()
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	if err := s.rdb.Set(ctx, id, b, ttl).Err(); err != nil {
		return "", err
	}
	return id, nil
}

func (s *RedisStore) GetJSON(ctx context.Context, sessionID string) (json.RawMessage, error) {
	val, err := s.rdb.Get(ctx, sessionID).Result()
	if err != nil {
		return nil, err
	}
	return json.RawMessage(val), nil
}

func (s *RedisStore) PutZipPath(ctx context.Context, sessionID, path string, ttl time.Duration) error {
	key := fmt.Sprintf("zip_project:%s", sessionID)
	return s.rdb.Set(ctx, key, path, ttl).Err()
}

func (s *RedisStore) GetZipPath(ctx context.Context, sessionID string) (string, error) {
	key := fmt.Sprintf("zip_project:%s", sessionID)
	return s.rdb.Get(ctx, key).Result()
}

func (s *RedisStore) DeleteZipPath(ctx context.Context, sessionID string) error {
	key := fmt.Sprintf("zip_project:%s", sessionID)
	return s.rdb.Del(ctx, key).Err()
}
