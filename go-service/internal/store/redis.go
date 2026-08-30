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
	PutEditorImageSource(ctx context.Context, sessionID string, payload any, ttl time.Duration) error
	GetEditorImageSource(ctx context.Context, sessionID string) (json.RawMessage, error)
	PutZipData(ctx context.Context, sessionID string, data []byte, ttl time.Duration) error
	GetZipData(ctx context.Context, sessionID string) ([]byte, error)
	DeleteZipData(ctx context.Context, sessionID string) error
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

func (s *RedisStore) PutEditorImageSource(ctx context.Context, sessionID string, payload any, ttl time.Duration) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, fmt.Sprintf("editor_image_source:%s", sessionID), b, ttl).Err()
}

func (s *RedisStore) GetEditorImageSource(ctx context.Context, sessionID string) (json.RawMessage, error) {
	val, err := s.rdb.Get(ctx, fmt.Sprintf("editor_image_source:%s", sessionID)).Result()
	if err != nil {
		return nil, err
	}
	return json.RawMessage(val), nil
}

func (s *RedisStore) PutZipData(ctx context.Context, sessionID string, data []byte, ttl time.Duration) error {
	key := fmt.Sprintf("zip_project:%s", sessionID)
	return s.rdb.Set(ctx, key, data, ttl).Err()
}

func (s *RedisStore) GetZipData(ctx context.Context, sessionID string) ([]byte, error) {
	key := fmt.Sprintf("zip_project:%s", sessionID)
	return s.rdb.Get(ctx, key).Bytes()
}

func (s *RedisStore) DeleteZipData(ctx context.Context, sessionID string) error {
	key := fmt.Sprintf("zip_project:%s", sessionID)
	return s.rdb.Del(ctx, key).Err()
}
