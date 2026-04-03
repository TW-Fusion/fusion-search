package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/TW-Fusion/fusion-search/app/config"
	"github.com/gin-gonic/gin"
)

// AuthMiddleware validates API keys
func AuthMiddleware(cfg *config.AppConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !cfg.Auth.Enabled {
			c.Next()
			return
		}

		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Missing API key. Provide Authorization: Bearer <key>",
			})
			return
		}

		// Extract Bearer token
		if len(authHeader) < 7 || authHeader[:7] != "Bearer " {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid authorization format. Use: Bearer <key>",
			})
			return
		}

		apiKey := authHeader[7:]

		// Check if API key is valid
		valid := false
		for _, key := range cfg.Auth.APIKeys {
			if subtle.ConstantTimeCompare([]byte(apiKey), []byte(key)) == 1 {
				valid = true
				break
			}
		}

		if !valid {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "Invalid API key",
			})
			return
		}

		c.Set("api_key", apiKey)
		c.Next()
	}
}
