package middleware

import (
	"time"

	"github.com/TW-Fusion/fusion-search/app/logging"
	"github.com/gin-gonic/gin"
)

// TimingMiddleware logs request duration
func TimingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start)

		logger := logging.GetLogger()
		logger.Infow("request_completed",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status_code", c.Writer.Status(),
			"duration_ms", float64(duration.Microseconds())/1000.0,
		)
	}
}
