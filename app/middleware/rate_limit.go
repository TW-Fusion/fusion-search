package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/TW-Fusion/fusion-search/app/config"
	"github.com/TW-Fusion/fusion-search/app/logging"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

// RateLimiter implements rate limiting using Redis
type RateLimiter struct {
	redisClient *redis.Client
	enabled     bool
	logger      *zap.SugaredLogger
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(cfg *config.AppConfig) *RateLimiter {
	logger := logging.GetLogger()

	if !cfg.RateLimit.Enabled {
		logger.Info("rate_limiting_disabled")
		return &RateLimiter{enabled: false, logger: logger}
	}

	// Parse Redis URL
	redisURL := cfg.Cache.RedisURL
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		logger.Errorw("redis_url_parse_failed", "error", err)
		return &RateLimiter{enabled: false, logger: logger}
	}

	client := redis.NewClient(opts)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		logger.Errorw("redis_connection_failed", "error", err)
		return &RateLimiter{enabled: false, logger: logger}
	}

	logger.Info("rate_limiter_initialized", "redis_url", redisURL)
	return &RateLimiter{
		redisClient: client,
		enabled:     true,
		logger:      logger,
	}
}

// parseRate parses rate limit string like "30/minute" to count and window
func parseRate(rateStr string) (int, time.Duration, error) {
	parts := strings.Split(rateStr, "/")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid rate format: %s", rateStr)
	}

	count, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid count in rate: %s", rateStr)
	}

	var window time.Duration
	switch parts[1] {
	case "second":
		window = time.Second
	case "minute":
		window = time.Minute
	case "hour":
		window = time.Hour
	case "day":
		window = 24 * time.Hour
	default:
		return 0, 0, fmt.Errorf("invalid time window in rate: %s", rateStr)
	}

	return count, window, nil
}

// Limit returns a gin middleware that enforces rate limiting
func (rl *RateLimiter) Limit(rateStr string) gin.HandlerFunc {
	if !rl.enabled {
		return func(c *gin.Context) {
			c.Next()
		}
	}

	maxRequests, window, err := parseRate(rateStr)
	if err != nil {
		rl.logger.Errorw("rate_limit_config_error", "error", err)
		return func(c *gin.Context) {
			c.Next()
		}
	}

	return func(c *gin.Context) {
		// Get client identifier (API key or IP)
		clientKey := c.ClientIP()
		if apiKey, exists := c.Get("api_key"); exists {
			clientKey = apiKey.(string)
		}

		// Create Redis key
		key := fmt.Sprintf("rate_limit:%s:%d", clientKey, window.Seconds())

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Use Redis to implement sliding window rate limiting
		pipe := rl.redisClient.TxPipeline()

		// Remove old entries
		pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", time.Now().Add(-window).UnixNano()))

		// Count current requests
		countCmd := pipe.ZCard(ctx, key)

		// Add current request
		pipe.ZAdd(ctx, key, &redis.Z{
			Score:  float64(time.Now().UnixNano()),
			Member: fmt.Sprintf("%d:%d", time.Now().UnixNano(), time.Now().UnixNano()),
		})

		// Set expiry on key
		pipe.Expire(ctx, key, window)

		_, err := pipe.Exec(ctx)
		if err != nil {
			rl.logger.Errorw("rate_limit_redis_error", "error", err)
			c.Next()
			return
		}

		count, err := countCmd.Result()
		if err != nil {
			rl.logger.Errorw("rate_limit_count_error", "error", err)
			c.Next()
			return
		}

		if int(count) > maxRequests {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "Rate limit exceeded",
			})
			return
		}

		c.Next()
	}
}

// Close closes the Redis client
func (rl *RateLimiter) Close() error {
	if rl.redisClient != nil {
		return rl.redisClient.Close()
	}
	return nil
}
