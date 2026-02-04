package issue

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers GitHub issue routes to the given router group.
// Usage: issue.RegisterRoutes(r.Group("/api/issues"))
func RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", handleListIssues)
	rg.GET("/:number", handleGetIssue)
	rg.POST("", handleCreateIssue)
	rg.POST("/:number/comments", handleCreateComment)
}

func handleListIssues(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	resp, err := ListIssues(c.Request.Context(), page, perPage)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func handleGetIssue(c *gin.Context) {
	number, err := strconv.Atoi(c.Param("number"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid issue number"})
		return
	}

	detail, err := GetIssue(c.Request.Context(), number)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, detail)
}

func handleCreateIssue(c *gin.Context) {
	var req CreateIssueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get App UserID from header (should be set by auth middleware)
	userID := c.GetHeader("X-App-User-ID")
	if userID == "" {
		userID = "anonymous"
	}

	issue, err := CreateIssue(c.Request.Context(), &req, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, issue)
}

func handleCreateComment(c *gin.Context) {
	number, err := strconv.Atoi(c.Param("number"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid issue number"})
		return
	}

	var req CreateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := c.GetHeader("X-App-User-ID")
	if userID == "" {
		userID = "anonymous"
	}

	comment, err := CreateComment(c.Request.Context(), number, &req, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, comment)
}
