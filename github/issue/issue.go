// Package issue provides GitHub Issues integration for App feedback system.
//
// Usage:
//
//	issues, err := issue.ListIssues(ctx, 1, 20)
//	detail, err := issue.GetIssue(ctx, 42)
//	newIssue, err := issue.CreateIssue(ctx, &issue.CreateIssueRequest{...}, "user123")
package issue

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
)

// ========== Config ==========

// Config holds GitHub module configuration.
type Config struct {
	Owner         string `yaml:"owner"`          // Repository owner
	Repo          string `yaml:"repo"`           // Repository name
	Token         string `yaml:"token"`          // GitHub PAT
	OfficialLabel string `yaml:"official_label"` // Label for official replies
	CacheTTL      int    `yaml:"cache_ttl"`      // Cache TTL in seconds
}

var (
	globalConfig *Config
	configOnce   sync.Once
	configMux    sync.RWMutex
)

func loadConfigFromViper() *Config {
	cfg := &Config{}

	cfg.Owner = viper.GetString("github.owner")
	cfg.Repo = viper.GetString("github.repo")
	cfg.Token = viper.GetString("github.token")
	cfg.OfficialLabel = viper.GetString("github.official_label")
	cfg.CacheTTL = viper.GetInt("github.cache_ttl")

	// Defaults
	if cfg.OfficialLabel == "" {
		cfg.OfficialLabel = "official-reply"
	}
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = 300
	}

	return cfg
}

func initialize() {
	configMux.RLock()
	cfg := globalConfig
	configMux.RUnlock()

	if cfg == nil {
		cfg = loadConfigFromViper()
		configMux.Lock()
		globalConfig = cfg
		configMux.Unlock()
	}
}

func ensureInitialized() {
	configOnce.Do(initialize)
}

func getConfig() *Config {
	ensureInitialized()
	configMux.RLock()
	defer configMux.RUnlock()
	return globalConfig
}

// SetConfig sets configuration manually (for testing).
func SetConfig(cfg *Config) {
	configMux.Lock()
	defer configMux.Unlock()
	globalConfig = cfg
}

// ========== Response DTOs ==========

// Issue is the sanitized issue returned to App clients.
type Issue struct {
	Number       int       `json:"number"`
	Title        string    `json:"title"`
	Body         string    `json:"body"`
	State        string    `json:"state"`
	Labels       []string  `json:"labels"`
	HasOfficial  bool      `json:"has_official"`
	CommentCount int       `json:"comment_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Comment is the sanitized comment returned to App clients.
type Comment struct {
	ID         int64     `json:"id"`
	Body       string    `json:"body"`
	IsOfficial bool      `json:"is_official"`
	CreatedAt  time.Time `json:"created_at"`
}

// IssueDetail includes issue with its comments.
type IssueDetail struct {
	Issue
	Comments []Comment `json:"comments"`
}

// ListIssuesResponse is the paginated list response.
type ListIssuesResponse struct {
	Issues  []Issue `json:"issues"`
	Page    int     `json:"page"`
	PerPage int     `json:"per_page"`
	HasMore bool    `json:"has_more"`
}

// ========== Request DTOs ==========

// CreateIssueRequest is the request to create a new issue.
type CreateIssueRequest struct {
	Title string `json:"title" binding:"required,min=5,max=200"`
	Body  string `json:"body" binding:"required,min=10,max=10000"`
}

// CreateCommentRequest is the request to create a new comment.
type CreateCommentRequest struct {
	Body string `json:"body" binding:"required,min=1,max=5000"`
}

// ========== Utility Functions ==========

var metadataRegex = regexp.MustCompile(`\n\n<!-- app_user_id: [^>]+ -->$`)

// stripMetadata removes the embedded user metadata from body.
func stripMetadata(body string) string {
	return strings.TrimSpace(metadataRegex.ReplaceAllString(body, ""))
}

// injectMetadata adds user metadata to body as invisible HTML comment.
func injectMetadata(body, userID string) string {
	return fmt.Sprintf("%s\n\n<!-- app_user_id: %s -->", body, userID)
}
