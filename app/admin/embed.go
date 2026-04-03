package admin

import (
	"embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed static/*
var StaticFiles embed.FS

// ServeStaticFile serves a static file from embedded filesystem
func ServeStaticFile(c *gin.Context, filepath string) {
	data, err := StaticFiles.ReadFile("static/" + filepath)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", data)
}
