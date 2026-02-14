package web

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/libersuite-org/panel/database"
	"github.com/libersuite-org/panel/database/models"
)

// Embed HTML templates into the final binary
//go:embed templates/*
var f embed.FS

// StartServer launches the web server on the specified port
func StartServer(port int, adminUser, adminPass string) error {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// Template helper functions
	funcMap := template.FuncMap{
		"div": func(a, b float64) float64 {
			if b == 0 {
				return 0
			}
			return a / b
		},
		"float": func(v int64) float64 {
			return float64(v)
		},
	}

	// Load embedded templates with helper functions
	templ := template.Must(
		template.New("").
			Funcs(funcMap).
			ParseFS(f, "templates/*.html"),
	)

	r.SetHTMLTemplate(templ)

	// Basic authentication for admin panel
	authorized := r.Group("/", gin.BasicAuth(gin.Accounts{
		adminUser: adminPass,
	}))

	// -------------------- Routes --------------------

	// 1. List all clients
	authorized.GET("/", func(c *gin.Context) {
		var clients []models.Client

		if err := database.DB.
			Order("id desc").
			Find(&clients).Error; err != nil {

			c.String(http.StatusInternalServerError, "Failed to fetch clients")
			return
		}

		c.HTML(http.StatusOK, "index.html", gin.H{
			"Clients": clients,
			"User":    adminUser,
		})
	})

	// 2. Add new client
	authorized.POST("/client/add", func(c *gin.Context) {
		username := c.PostForm("username")
		password := c.PostForm("password")

		limitGB, err := strconv.ParseInt(c.PostForm("limit"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid traffic limit"})
			return
		}

		days, err := strconv.Atoi(c.PostForm("days"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid expiration days"})
			return
		}

		// Check for duplicate username
		var count int64
		if err := database.DB.
			Model(&models.Client{}).
			Where("username = ?", username).
			Count(&count).Error; err != nil {

			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}

		if count > 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Username already exists"})
			return
		}

		client := models.Client{
			Username:     username,
			Password:     password,
			TrafficLimit: limitGB * 1024 * 1024 * 1024, // Convert GB to bytes
			Enabled:      true,
		}

		// Set expiration date if provided
		if days > 0 {
			client.ExpiresAt = time.Now().AddDate(0, 0, days)
		}

		if err := database.DB.Create(&client).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.Redirect(http.StatusFound, "/")
	})

	// 3. Delete client
	authorized.POST("/client/delete/:id", func(c *gin.Context) {
		id := c.Param("id")

		if err := database.DB.
			Delete(&models.Client{}, id).Error; err != nil {

			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete client"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "deleted"})
	})

	// 4. Toggle client status (enable/disable)
	authorized.POST("/client/toggle/:id", func(c *gin.Context) {
		id := c.Param("id")
		var client models.Client

		if err := database.DB.First(&client, id).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Client not found"})
			return
		}

		client.Enabled = !client.Enabled

		if err := database.DB.Save(&client).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update client"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":  "toggled",
			"enabled": client.Enabled,
		})
	})

	// Start server
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	fmt.Printf("Web UI started at http://%s\n", addr)

	return r.Run(addr)
}
