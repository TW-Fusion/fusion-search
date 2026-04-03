//go:build dev

package main

import (
	_ "github.com/TW-Fusion/fusion-search/app/docs"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/swaggo/swag"
)

// setupSwagger registers Swagger UI endpoints (development only)
func setupSwagger(r *gin.Engine) {
	r.GET("/swagger/*any", func(c *gin.Context) {
		if c.Param("any") == "/doc.json" {
			doc, err := swag.ReadDoc("swagger")
			if err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
			c.Header("Content-Type", "application/json; charset=utf-8")
			c.String(200, doc)
			return
		}
		ginSwagger.WrapHandler(swaggerFiles.Handler,
			ginSwagger.InstanceName("swagger"),
			ginSwagger.DocExpansion("list"),
		)(c)
	})
}
