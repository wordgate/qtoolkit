package aws

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// PresignRequest represents a presigned URL request
type PresignRequest struct {
	Filename   string `json:"filename" binding:"required"`
	Expiration int    `json:"expiration,omitempty"` // minutes, default 15
}

// PresignResponse represents a presigned URL response
type PresignResponse struct {
	URL        string            `json:"url"`
	Method     string            `json:"method"`
	Headers    map[string]string `json:"headers,omitempty"`
	FormData   map[string]string `json:"form_data,omitempty"`
	UploadType string            `json:"upload_type"` // "PUT" or "POST"
}

// HandlePresignedURL generates presigned URLs for client-side uploads
func HandlePresignedURL() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req PresignRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "invalid request: " + err.Error()})
			return
		}

		// Default expiration: 15 minutes
		expiration := 15
		if req.Expiration > 0 && req.Expiration <= 60 {
			expiration = req.Expiration
		}

		duration := time.Duration(expiration) * time.Minute
		
		// Generate PUT presigned URL (simpler for client)
		url, err := S3GeneratePresignedURL(req.Filename, duration)
		if err != nil {
			c.JSON(500, gin.H{"error": "failed to generate presigned URL: " + err.Error()})
			return
		}

		response := PresignResponse{
			URL:        url,
			Method:     "PUT",
			UploadType: "PUT",
			Headers: map[string]string{
				"Content-Type": "application/octet-stream",
			},
		}

		c.JSON(200, response)
	}
}

// HandlePresignedPOSTURL generates presigned POST URLs with form data
func HandlePresignedPOSTURL() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req PresignRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "invalid request: " + err.Error()})
			return
		}

		expiration := 15
		if req.Expiration > 0 && req.Expiration <= 60 {
			expiration = req.Expiration
		}

		duration := time.Duration(expiration) * time.Minute
		
		presignedPost, err := S3GeneratePresignedPOSTURL(req.Filename, duration)
		if err != nil {
			c.JSON(500, gin.H{"error": "failed to generate presigned POST URL: " + err.Error()})
			return
		}

		response := PresignResponse{
			URL:        presignedPost.URL,
			Method:     "PUT", // Changed to PUT since we're using PUT presigned URL
			UploadType: "PUT",
			FormData:   presignedPost.Fields,
		}

		c.JSON(200, response)
	}
}

// SimplePresignedURLHandler provides a simple HTTP handler for presigned URLs
// Usage: http://localhost:8080/presign?filename=test.jpg&expiration=30
func SimplePresignedURLHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filename := r.URL.Query().Get("filename")
		if filename == "" {
			http.Error(w, "filename parameter required", http.StatusBadRequest)
			return
		}

		// Default: 15 minutes
		expiration := 15 * time.Minute
		if exp := r.URL.Query().Get("expiration"); exp != "" {
			if minutes, err := time.ParseDuration(exp + "m"); err == nil {
				if minutes <= 60*time.Minute { // Max 60 minutes
					expiration = minutes
				}
			}
		}

		url, err := S3GeneratePresignedURL(filename, expiration)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to generate presigned URL: %v", err), http.StatusInternalServerError)
			return
		}

		response := map[string]string{
			"url":    url,
			"method": "PUT",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}