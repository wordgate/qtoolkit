package issue

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterRoutes(r.Group("/api/issues"))
	return r
}

func TestRouteListIssues(t *testing.T) {
	DisableCache()
	defer EnableCache()

	// Mock GitHub API
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issues := []ghIssue{
			{Number: 1, Title: "Issue 1", State: "open", CreatedAt: time.Now(), UpdatedAt: time.Now()},
			{Number: 2, Title: "Issue 2", State: "closed", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(issues)
	}))
	defer ghServer.Close()

	viper.Reset()
	viper.Set("github.owner", "test-owner")
	viper.Set("github.repo", "test-repo")
	viper.Set("github.token", "ghp_test")
	SetAPIBaseURL(ghServer.URL)
	resetClient()

	router := setupTestRouter()

	req := httptest.NewRequest("GET", "/api/issues?page=1&per_page=20", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp ListIssuesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(resp.Issues) != 2 {
		t.Errorf("expected 2 issues, got %d", len(resp.Issues))
	}
}

func TestRouteGetIssue(t *testing.T) {
	DisableCache()
	defer EnableCache()

	// Mock GitHub API
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/test-owner/test-repo/issues/42":
			issue := ghIssue{Number: 42, Title: "Test", State: "open", CreatedAt: time.Now(), UpdatedAt: time.Now()}
			json.NewEncoder(w).Encode(issue)
		case "/repos/test-owner/test-repo/issues/42/comments":
			json.NewEncoder(w).Encode([]ghComment{})
		}
	}))
	defer ghServer.Close()

	viper.Reset()
	viper.Set("github.owner", "test-owner")
	viper.Set("github.repo", "test-repo")
	viper.Set("github.token", "ghp_test")
	SetAPIBaseURL(ghServer.URL)
	resetClient()

	router := setupTestRouter()

	req := httptest.NewRequest("GET", "/api/issues/42", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp IssueDetail
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Number != 42 {
		t.Errorf("expected number 42, got %d", resp.Number)
	}
}

func TestRouteCreateIssue(t *testing.T) {
	DisableCache()
	defer EnableCache()

	// Mock GitHub API
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issue := ghIssue{Number: 123, Title: "New Issue", State: "open", CreatedAt: time.Now(), UpdatedAt: time.Now()}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(issue)
	}))
	defer ghServer.Close()

	viper.Reset()
	viper.Set("github.owner", "test-owner")
	viper.Set("github.repo", "test-repo")
	viper.Set("github.token", "ghp_test")
	SetAPIBaseURL(ghServer.URL)
	resetClient()

	router := setupTestRouter()

	body := `{"title":"New Issue","body":"This is a test issue body"}`
	req := httptest.NewRequest("POST", "/api/issues", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-App-User-ID", "user123") // Middleware should extract this
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp Issue
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Number != 123 {
		t.Errorf("expected number 123, got %d", resp.Number)
	}
}

func TestRouteCreateComment(t *testing.T) {
	DisableCache()
	defer EnableCache()

	// Mock GitHub API
	ghServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		comment := ghComment{ID: 456, Body: "Test comment", User: ghUser{Login: "user"}, CreatedAt: time.Now()}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(comment)
	}))
	defer ghServer.Close()

	viper.Reset()
	viper.Set("github.owner", "test-owner")
	viper.Set("github.repo", "test-repo")
	viper.Set("github.token", "ghp_test")
	SetAPIBaseURL(ghServer.URL)
	resetClient()

	router := setupTestRouter()

	body := `{"body":"This is a test comment"}`
	req := httptest.NewRequest("POST", "/api/issues/42/comments", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-App-User-ID", "user456")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp Comment
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.ID != 456 {
		t.Errorf("expected id 456, got %d", resp.ID)
	}
}

func TestRouteCreateIssueValidation(t *testing.T) {
	DisableCache()
	defer EnableCache()

	router := setupTestRouter()

	// Missing title
	body := `{"body":"This is a test"}`
	req := httptest.NewRequest("POST", "/api/issues", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}
