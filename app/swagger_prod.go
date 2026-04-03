//go:build !dev

package main

import "github.com/gin-gonic/gin"

// setupSwagger is a no-op in production builds
func setupSwagger(r *gin.Engine) {
	// Swagger UI is disabled in production builds
	// Build with 'go build -tags dev' to enable Swagger UI
}
