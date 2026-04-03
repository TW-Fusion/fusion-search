package services

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/TW-Fusion/fusion-search/app/config"
	"github.com/TW-Fusion/fusion-search/app/logging"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

type CacheService struct {
	config      *config.AppConfig
	redisClient *redis.Client
	enabled     bool
	searchTTL   time.Duration
	extractTTL  time.Duration
	answerTTL   time.Duration
	logger      *zap.SugaredLogger
}

func NewCacheService(cfg *config.AppConfig) *CacheService {
	logger := logging.GetLogger()

	cacheService := &CacheService{
		config:     cfg,
		enabled:    cfg.Cache.Enabled,
		searchTTL:  time.Duration(cfg.Cache.SearchTTL) * time.Second,
		extractTTL: time.Duration(cfg.Cache.ExtractTTL) * time.Second,
		answerTTL:  time.Duration(cfg.LLM.AnswerTTL) * time.Second,
		logger:     logger,
	}

	return cacheService
}

func (cs *CacheService) Connect() error {
	if !cs.enabled {
		cs.logger.Info("cache_disabled")
		return nil
	}

	opts, err := redis.ParseURL(cs.config.Cache.RedisURL)
	if err != nil {
		cs.logger.Errorw("redis_url_parse_failed", "error", err)
		cs.enabled = false
		return nil
	}

	cs.redisClient = redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := cs.redisClient.Ping(ctx).Err(); err != nil {
		cs.logger.Errorw("redis_connection_failed", "error", err)
		cs.enabled = false
		cs.redisClient = nil
		return nil
	}

	cs.logger.Info("redis_connected", "url", cs.config.Cache.RedisURL)
	return nil
}

func (cs *CacheService) Close() error {
	if cs.redisClient != nil {
		return cs.redisClient.Close()
	}
	return nil
}

func (cs *CacheService) IsEnabled() bool {
	return cs.enabled
}

func (cs *CacheService) hashKey(prefix, value string) string {
	h := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%s:%x", prefix, h[:8])
}

// GetSearch retrieves cached search results
func (cs *CacheService) GetSearch(query, paramsHash string) (map[string]interface{}, error) {
	if !cs.enabled || cs.redisClient == nil {
		return nil, nil
	}

	key := cs.hashKey("search", query+"|"+paramsHash)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	data, err := cs.redisClient.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	} else if err != nil {
		cs.logger.Warnw("cache_read_error", "error", err)
		return nil, err
	}

	cs.logger.Debugw("cache_hit", "type", "search", "query", query)

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		return nil, err
	}

	return result, nil
}

// SetSearch caches search results
func (cs *CacheService) SetSearch(query, paramsHash string, data map[string]interface{}) error {
	if !cs.enabled || cs.redisClient == nil {
		return nil
	}

	key := cs.hashKey("search", query+"|"+paramsHash)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	if err := cs.redisClient.Set(ctx, key, jsonData, cs.searchTTL).Err(); err != nil {
		cs.logger.Warnw("cache_write_error", "error", err)
		return err
	}

	return nil
}

// GetExtract retrieves cached extraction content for a URL
func (cs *CacheService) GetExtract(url string) (*string, error) {
	if !cs.enabled || cs.redisClient == nil {
		return nil, nil
	}

	key := cs.hashKey("extract", url)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	data, err := cs.redisClient.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	} else if err != nil {
		cs.logger.Warnw("cache_read_error", "error", err)
		return nil, err
	}

	cs.logger.Debugw("cache_hit", "type", "extract", "url", url)
	return &data, nil
}

// SetExtract caches extraction content for a URL
func (cs *CacheService) SetExtract(url, content string) error {
	if !cs.enabled || cs.redisClient == nil {
		return nil
	}

	key := cs.hashKey("extract", url)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := cs.redisClient.Set(ctx, key, content, cs.extractTTL).Err(); err != nil {
		cs.logger.Warnw("cache_write_error", "error", err)
		return err
	}

	return nil
}

// GetExtractBatch retrieves cached extraction content for multiple URLs
func (cs *CacheService) GetExtractBatch(urls []string) (map[string]*string, error) {
	if !cs.enabled || cs.redisClient == nil {
		result := make(map[string]*string)
		for _, url := range urls {
			result[url] = nil
		}
		return result, nil
	}

	keys := make([]string, len(urls))
	for i, url := range urls {
		keys[i] = cs.hashKey("extract", url)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipe := cs.redisClient.TxPipeline()
	cmds := make([]*redis.StringCmd, len(keys))
	for i, key := range keys {
		cmds[i] = pipe.Get(ctx, key)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		cs.logger.Warnw("cache_batch_read_error", "error", err)
		// Return all nils on error
		result := make(map[string]*string)
		for _, url := range urls {
			result[url] = nil
		}
		return result, nil
	}

	result := make(map[string]*string)
	for i, url := range urls {
		val, err := cmds[i].Result()
		if err == redis.Nil {
			result[url] = nil
		} else if err != nil {
			result[url] = nil
		} else {
			result[url] = &val
		}
	}

	return result, nil
}

// SetExtractBatch caches extraction content for multiple URLs
func (cs *CacheService) SetExtractBatch(pairs [][2]string) error {
	if !cs.enabled || cs.redisClient == nil || len(pairs) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipe := cs.redisClient.TxPipeline()
	for _, pair := range pairs {
		url, content := pair[0], pair[1]
		key := cs.hashKey("extract", url)
		pipe.Set(ctx, key, content, cs.extractTTL)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		cs.logger.Warnw("cache_batch_write_error", "error", err)
		return err
	}

	return nil
}

// GetAnswer retrieves cached AI answer
func (cs *CacheService) GetAnswer(query, paramsHash string) (*string, error) {
	if !cs.enabled || cs.redisClient == nil {
		return nil, nil
	}

	key := cs.hashKey("answer", query+"|"+paramsHash)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	data, err := cs.redisClient.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	} else if err != nil {
		cs.logger.Warnw("cache_read_error", "error", err)
		return nil, err
	}

	cs.logger.Debugw("cache_hit", "type", "answer", "query", query)
	return &data, nil
}

// SetAnswer caches AI answer
func (cs *CacheService) SetAnswer(query, paramsHash, answer string) error {
	if !cs.enabled || cs.redisClient == nil {
		return nil
	}

	key := cs.hashKey("answer", query+"|"+paramsHash)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := cs.redisClient.Set(ctx, key, answer, cs.answerTTL).Err(); err != nil {
		cs.logger.Warnw("cache_write_error", "error", err)
		return err
	}

	return nil
}
