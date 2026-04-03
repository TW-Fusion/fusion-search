package routers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HealthRouter registers the health check endpoint
func HealthRouter(r *gin.Engine) {
	r.GET("/health", healthHandler)
}

// healthHandler godoc
// @Summary Health check endpoint
// @Description Returns the health status of the service
// @Tags Health
// @Produce json
// @Success 200 {object} map[string]string "Service is healthy"
// @Router /health [get]
func healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "fusion-search",
	})
}
