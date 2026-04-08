package internal

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/TW-Fusion/fusion-search/app/admin"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

// Admin session storage
type AdminSession struct {
	Token     string
	CreatedAt time.Time
}

var (
	adminSessions = make(map[string]AdminSession)
	sessionsMu    sync.RWMutex

	// Admin password hash (default: "admin123")
	adminPasswordHash = "240be518fabd2724ddb6f04eeb1da5967448d7e831c08c8fa822809f74c720a9" // SHA256 of "admin123"

	// Token expiration
	tokenExpiration = 24 * time.Hour

	// Config file path (set during setup)
	configFilePath string
)

// Admin login request
type AdminLoginRequest struct {
	Password string `json:"password" binding:"required"`
}

// Password change request
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required"`
}

// SetupAdminRoutes registers admin routes
func SetupAdminRoutes(r *gin.Engine, configPath string) {
	configFilePath = configPath

	// Load password hash from config file
	loadPasswordHashFromConfig()

	// Serve admin static files from embedded filesystem
	r.GET("/admin", func(c *gin.Context) {
		admin.ServeStaticFile(c, "admin.html")
	})
	r.GET("/admin/login", func(c *gin.Context) {
		admin.ServeStaticFile(c, "admin.html")
	})
	r.GET("/admin/config", func(c *gin.Context) {
		admin.ServeStaticFile(c, "admin.html")
	})
	r.GET("/admin/password", func(c *gin.Context) {
		admin.ServeStaticFile(c, "admin.html")
	})

	// Admin API routes
	adminGroup := r.Group("/admin/api")
	{
		adminGroup.POST("/login", handleAdminLogin)
		adminGroup.GET("/config", handleGetConfig(configPath))
		adminGroup.PUT("/config", handleUpdateConfig(configPath))
		adminGroup.PUT("/password", handleChangePassword)
	}
}

// Admin auth middleware
func AdminAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		token := authHeader[len("Bearer "):]
		sessionsMu.RLock()
		session, exists := adminSessions[token]
		sessionsMu.RUnlock()

		if !exists || time.Since(session.CreatedAt) > tokenExpiration {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// Handle admin login
func handleAdminLogin(c *gin.Context) {
	var req AdminLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Hash the provided password
	hash := sha256.Sum256([]byte(req.Password))
	passwordHash := hex.EncodeToString(hash[:])

	// Compare with stored hash
	if passwordHash != adminPasswordHash {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid password"})
		return
	}

	// Generate token
	token := generateToken()

	// Store session
	sessionsMu.Lock()
	adminSessions[token] = AdminSession{
		Token:     token,
		CreatedAt: time.Now(),
	}
	sessionsMu.Unlock()

	// Clean up expired sessions periodically
	go cleanupExpiredSessions()

	c.JSON(http.StatusOK, gin.H{
		"token":   token,
		"message": "Login successful",
	})
}

// Handle get config
func handleGetConfig(configPath string) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, err := os.ReadFile(configPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read config file"})
			return
		}

		var config map[string]interface{}
		if err := yaml.Unmarshal(data, &config); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse config file"})
			return
		}

		c.JSON(http.StatusOK, config)
	}
}

// Handle update config
func handleUpdateConfig(configPath string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var incoming map[string]interface{}
		if err := c.ShouldBindJSON(&incoming); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}

		// Read existing config so unknown fields are preserved.
		existingData, err := os.ReadFile(configPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read config file"})
			return
		}

		var existing map[string]interface{}
		if err := yaml.Unmarshal(existingData, &existing); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse config file"})
			return
		}

		merged := mergeConfigMaps(existing, incoming)

		// Marshal to YAML
		yamlData, err := yaml.Marshal(&merged)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to marshal config"})
			return
		}

		// Write to file
		if err := os.WriteFile(configPath, yamlData, 0644); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write config file"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Configuration updated successfully",
		})
	}
}

func mergeConfigMaps(base, updates map[string]interface{}) map[string]interface{} {
	if base == nil {
		base = map[string]interface{}{}
	}
	for k, v := range updates {
		if vmap, ok := v.(map[string]interface{}); ok {
			if bmap, ok := base[k].(map[string]interface{}); ok {
				base[k] = mergeConfigMaps(bmap, vmap)
			} else {
				base[k] = mergeConfigMaps(map[string]interface{}{}, vmap)
			}
			continue
		}
		base[k] = v
	}
	return base
}

// Handle change password
func handleChangePassword(c *gin.Context) {
	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Verify current password
	currentHash := sha256.Sum256([]byte(req.CurrentPassword))
	currentHashStr := hex.EncodeToString(currentHash[:])

	if currentHashStr != adminPasswordHash {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Current password is incorrect"})
		return
	}

	// Validate new password
	if len(req.NewPassword) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "New password must be at least 6 characters"})
		return
	}

	// Update password hash
	newHash := sha256.Sum256([]byte(req.NewPassword))
	newHashStr := hex.EncodeToString(newHash[:])

	// Save to config file
	if err := savePasswordHashToConfig(newHashStr); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save password to config file"})
		return
	}

	// Update in-memory hash
	adminPasswordHash = newHashStr

	c.JSON(http.StatusOK, gin.H{
		"message": "Password updated successfully",
	})
}

// Generate a random token
func generateToken() string {
	data := fmt.Sprintf("%d-%d", time.Now().UnixNano(), os.Getpid())
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// Clean up expired sessions
func cleanupExpiredSessions() {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	now := time.Now()
	for token, session := range adminSessions {
		if now.Sub(session.CreatedAt) > tokenExpiration {
			delete(adminSessions, token)
		}
	}
}

// Load password hash from config file
func loadPasswordHashFromConfig() {
	if configFilePath == "" {
		return
	}

	data, err := os.ReadFile(configFilePath)
	if err != nil {
		return
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return
	}

	if admin, ok := config["admin"].(map[string]interface{}); ok {
		if hash, ok := admin["password_hash"].(string); ok && hash != "" {
			adminPasswordHash = hash
		}
	}
}

// Save password hash to config file
func savePasswordHashToConfig(newHash string) error {
	if configFilePath == "" {
		return fmt.Errorf("config file path not set")
	}

	// Read current config
	data, err := os.ReadFile(configFilePath)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Update password hash
	if admin, ok := config["admin"].(map[string]interface{}); ok {
		admin["password_hash"] = newHash
	} else {
		config["admin"] = map[string]interface{}{
			"password_hash": newHash,
		}
	}

	// Marshal to YAML
	yamlData, err := yaml.Marshal(&config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file
	if err := os.WriteFile(configFilePath, yamlData, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}
