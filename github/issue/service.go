package issue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/wordgate/qtoolkit/redis"
)

// ========== Cache Helpers (fail-safe) ==========

// cacheEnabled checks if Redis is configured
var cacheEnabled = true

// DisableCache disables caching (for testing)
func DisableCache() {
	cacheEnabled = false
}

// EnableCache enables caching
func EnableCache() {
	cacheEnabled = true
}

func cacheGet(key string, val any) bool {
	if !cacheEnabled {
		return false
	}
	defer func() { recover() }() // Ignore Redis panic
	exist, _ := redis.CacheGet(key, val)
	return exist
}

func cacheSet(key string, val any, ttl int) {
	if !cacheEnabled {
		return
	}
	defer func() { recover() }()
	redis.CacheSet(key, val, ttl)
}

func cacheDel(key string) {
	if !cacheEnabled {
		return
	}
	defer func() { recover() }()
	redis.CacheDel(key)
}

func cacheDelPattern(pattern string) {
	if !cacheEnabled {
		return
	}
	defer func() { recover() }()
	ctx := context.Background()
	keys, err := redis.Client().Keys(ctx, pattern).Result()
	if err != nil {
		return
	}
	for _, k := range keys {
		redis.CacheDel(k)
	}
}

// ========== GitHub API Types (internal) ==========

type ghUser struct {
	Login string `json:"login"`
}

type ghLabel struct {
	Name string `json:"name"`
}

type ghIssue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	Labels    []ghLabel `json:"labels"`
	Comments  int       `json:"comments"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ghComment struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	User      ghUser    `json:"user"`
	CreatedAt time.Time `json:"created_at"`
}

// ========== HTTP Client ==========

var (
	apiBaseURL = "https://api.github.com"
	httpClient *http.Client
	clientOnce sync.Once
	clientMux  sync.RWMutex
)

// SetAPIBaseURL sets the base URL for GitHub API (for testing).
func SetAPIBaseURL(url string) {
	clientMux.Lock()
	defer clientMux.Unlock()
	apiBaseURL = url
}

func getAPIBaseURL() string {
	clientMux.RLock()
	defer clientMux.RUnlock()
	return apiBaseURL
}

func resetClient() {
	clientMux.Lock()
	defer clientMux.Unlock()
	clientOnce = sync.Once{}
	globalConfig = nil
	configOnce = sync.Once{}
}

func getHTTPClient() *http.Client {
	clientOnce.Do(func() {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	})
	return httpClient
}

func doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	cfg := getConfig()

	url := fmt.Sprintf("%s%s", getAPIBaseURL(), path)

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cfg.Token))
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return getHTTPClient().Do(req)
}

// ========== Service Functions ==========

// ListIssues returns paginated issues list (cache-first).
func ListIssues(ctx context.Context, page, perPage int) (*ListIssuesResponse, error) {
	cfg := getConfig()

	// Try cache first
	cacheKey := fmt.Sprintf("github:issues:list:p%d:n%d", page, perPage)
	var cached ListIssuesResponse
	if cacheGet(cacheKey, &cached) {
		return &cached, nil
	}

	// Fetch from GitHub
	path := fmt.Sprintf("/repos/%s/%s/issues?page=%d&per_page=%d&state=all",
		cfg.Owner, cfg.Repo, page, perPage)

	resp, err := doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("github api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github api: status %d, body: %s", resp.StatusCode, body)
	}

	var ghIssues []ghIssue
	if err := json.NewDecoder(resp.Body).Decode(&ghIssues); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Transform to DTOs
	issues := make([]Issue, len(ghIssues))
	for i, gh := range ghIssues {
		issues[i] = *transformToIssue(&gh)
	}

	result := &ListIssuesResponse{
		Issues:  issues,
		Page:    page,
		PerPage: perPage,
		HasMore: len(ghIssues) == perPage,
	}

	// Cache result
	cacheSet(cacheKey, result, cfg.CacheTTL)

	return result, nil
}

// GetIssue returns issue detail with comments (cache-first).
func GetIssue(ctx context.Context, number int) (*IssueDetail, error) {
	cfg := getConfig()

	// Try cache first
	cacheKey := fmt.Sprintf("github:issues:%d", number)
	var cached IssueDetail
	if cacheGet(cacheKey, &cached) {
		return &cached, nil
	}

	// Fetch issue
	path := fmt.Sprintf("/repos/%s/%s/issues/%d", cfg.Owner, cfg.Repo, number)
	resp, err := doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("github api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github api: status %d, body: %s", resp.StatusCode, body)
	}

	var ghIssue ghIssue
	if err := json.NewDecoder(resp.Body).Decode(&ghIssue); err != nil {
		return nil, fmt.Errorf("decode issue: %w", err)
	}

	// Fetch comments
	commentsPath := fmt.Sprintf("/repos/%s/%s/issues/%d/comments", cfg.Owner, cfg.Repo, number)
	commentsResp, err := doRequest(ctx, "GET", commentsPath, nil)
	if err != nil {
		return nil, fmt.Errorf("github api comments: %w", err)
	}
	defer commentsResp.Body.Close()

	if commentsResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(commentsResp.Body)
		return nil, fmt.Errorf("github api comments: status %d, body: %s", commentsResp.StatusCode, body)
	}

	var ghComments []ghComment
	if err := json.NewDecoder(commentsResp.Body).Decode(&ghComments); err != nil {
		return nil, fmt.Errorf("decode comments: %w", err)
	}

	// Transform to DTOs
	comments := make([]Comment, len(ghComments))
	for i, gh := range ghComments {
		comments[i] = *transformToComment(&gh)
	}

	result := &IssueDetail{
		Issue:    *transformToIssue(&ghIssue),
		Comments: comments,
	}

	// Cache result
	cacheSet(cacheKey, result, cfg.CacheTTL)

	return result, nil
}

// CreateIssue creates a new issue on GitHub (invalidates cache).
func CreateIssue(ctx context.Context, req *CreateIssueRequest, appUserID string) (*Issue, error) {
	cfg := getConfig()

	// Inject user metadata
	bodyWithMeta := injectMetadata(req.Body, appUserID)

	payload := map[string]string{
		"title": req.Title,
		"body":  bodyWithMeta,
	}

	path := fmt.Sprintf("/repos/%s/%s/issues", cfg.Owner, cfg.Repo)
	resp, err := doRequest(ctx, "POST", path, payload)
	if err != nil {
		return nil, fmt.Errorf("github api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github api: status %d, body: %s", resp.StatusCode, body)
	}

	var ghIssue ghIssue
	if err := json.NewDecoder(resp.Body).Decode(&ghIssue); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Invalidate list cache
	invalidateListCache()

	return transformToIssue(&ghIssue), nil
}

// CreateComment creates a new comment on an issue (invalidates cache).
func CreateComment(ctx context.Context, number int, req *CreateCommentRequest, appUserID string) (*Comment, error) {
	cfg := getConfig()

	// Inject user metadata
	bodyWithMeta := injectMetadata(req.Body, appUserID)

	payload := map[string]string{
		"body": bodyWithMeta,
	}

	path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments", cfg.Owner, cfg.Repo, number)
	resp, err := doRequest(ctx, "POST", path, payload)
	if err != nil {
		return nil, fmt.Errorf("github api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github api: status %d, body: %s", resp.StatusCode, body)
	}

	var ghComment ghComment
	if err := json.NewDecoder(resp.Body).Decode(&ghComment); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Invalidate issue cache
	cacheDel(fmt.Sprintf("github:issues:%d", number))

	return transformToComment(&ghComment), nil
}

// ========== Transform Functions ==========

func transformToIssue(gh *ghIssue) *Issue {
	cfg := getConfig()

	labels := make([]string, len(gh.Labels))
	hasOfficial := false

	for i, l := range gh.Labels {
		labels[i] = l.Name
		if l.Name == cfg.OfficialLabel {
			hasOfficial = true
		}
	}

	return &Issue{
		Number:       gh.Number,
		Title:        gh.Title,
		Body:         stripMetadata(gh.Body),
		State:        gh.State,
		Labels:       labels,
		HasOfficial:  hasOfficial,
		CommentCount: gh.Comments,
		CreatedAt:    gh.CreatedAt,
		UpdatedAt:    gh.UpdatedAt,
	}
}

func transformToComment(gh *ghComment) *Comment {
	officialUsers := viper.GetStringSlice("github.official_users")
	isOfficial := slices.Contains(officialUsers, gh.User.Login)

	return &Comment{
		ID:         gh.ID,
		Body:       stripMetadata(gh.Body),
		IsOfficial: isOfficial,
		CreatedAt:  gh.CreatedAt,
	}
}

// ========== Cache Invalidation ==========

func invalidateListCache() {
	cacheDelPattern("github:issues:list:*")
}
