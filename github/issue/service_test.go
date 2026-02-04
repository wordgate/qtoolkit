package issue

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/spf13/viper"
)

// ========== Service Function Tests ==========

func TestListIssues(t *testing.T) {
	DisableCache() // Disable Redis for testing
	defer EnableCache()
	// Mock GitHub API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/test-owner/test-repo/issues" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer ghp_test123" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}

		// Return mock GitHub issues
		issues := []ghIssue{
			{
				Number:    1,
				Title:     "First Issue",
				Body:      "Body 1\n\n<!-- app_user_id: user1 -->",
				State:     "open",
				Labels:    []ghLabel{{Name: "bug"}},
				Comments:  2,
				CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			},
			{
				Number:    2,
				Title:     "Second Issue",
				Body:      "Body 2",
				State:     "closed",
				Labels:    []ghLabel{{Name: "official-reply"}},
				Comments:  0,
				CreatedAt: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(issues)
	}))
	defer server.Close()

	// Setup config
	viper.Reset()
	viper.Set("github.owner", "test-owner")
	viper.Set("github.repo", "test-repo")
	viper.Set("github.token", "ghp_test123")
	viper.Set("github.official_label", "official-reply")
	viper.Set("github.cache_ttl", 300)

	// Override API base URL for testing
	SetAPIBaseURL(server.URL)
	resetClient()

	resp, err := ListIssues(context.Background(), 1, 20)
	if err != nil {
		t.Fatalf("ListIssues failed: %v", err)
	}

	if len(resp.Issues) != 2 {
		t.Errorf("expected 2 issues, got %d", len(resp.Issues))
	}

	// Check first issue (metadata should be stripped)
	if resp.Issues[0].Body != "Body 1" {
		t.Errorf("expected body 'Body 1', got '%s'", resp.Issues[0].Body)
	}
	if resp.Issues[0].HasOfficial {
		t.Error("first issue should not have official reply")
	}

	// Check second issue (has official label)
	if !resp.Issues[1].HasOfficial {
		t.Error("second issue should have official reply")
	}
}

func TestGetIssue(t *testing.T) {
	DisableCache()
	defer EnableCache()

	// Mock GitHub API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/repos/test-owner/test-repo/issues/42":
			issue := ghIssue{
				Number:    42,
				Title:     "Test Issue",
				Body:      "Issue body\n\n<!-- app_user_id: user123 -->",
				State:     "open",
				Labels:    []ghLabel{{Name: "bug"}, {Name: "official-reply"}},
				Comments:  2,
				CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			}
			json.NewEncoder(w).Encode(issue)

		case "/repos/test-owner/test-repo/issues/42/comments":
			comments := []ghComment{
				{
					ID:        101,
					Body:      "Comment 1\n\n<!-- app_user_id: user456 -->",
					User:      ghUser{Login: "regular-user"},
					CreatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				},
				{
					ID:        102,
					Body:      "Official response",
					User:      ghUser{Login: "official-bot"},
					CreatedAt: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
				},
			}
			json.NewEncoder(w).Encode(comments)

		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	// Setup config
	viper.Reset()
	viper.Set("github.owner", "test-owner")
	viper.Set("github.repo", "test-repo")
	viper.Set("github.token", "ghp_test123")
	viper.Set("github.official_label", "official-reply")
	viper.Set("github.official_users", []string{"official-bot"})

	SetAPIBaseURL(server.URL)
	resetClient()

	detail, err := GetIssue(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetIssue failed: %v", err)
	}

	if detail.Number != 42 {
		t.Errorf("expected number 42, got %d", detail.Number)
	}
	if detail.Body != "Issue body" {
		t.Errorf("expected body 'Issue body', got '%s'", detail.Body)
	}
	if !detail.HasOfficial {
		t.Error("issue should have official reply")
	}

	if len(detail.Comments) != 2 {
		t.Errorf("expected 2 comments, got %d", len(detail.Comments))
	}

	// First comment metadata stripped
	if detail.Comments[0].Body != "Comment 1" {
		t.Errorf("expected comment body 'Comment 1', got '%s'", detail.Comments[0].Body)
	}
	if detail.Comments[0].IsOfficial {
		t.Error("first comment should not be official")
	}

	// Second comment is official
	if !detail.Comments[1].IsOfficial {
		t.Error("second comment should be official")
	}
}

func TestCreateIssue(t *testing.T) {
	DisableCache()
	defer EnableCache()

	var receivedBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/repos/test-owner/test-repo/issues" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		json.NewDecoder(r.Body).Decode(&receivedBody)

		// Return created issue
		issue := ghIssue{
			Number:    123,
			Title:     receivedBody["title"].(string),
			Body:      receivedBody["body"].(string),
			State:     "open",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(issue)
	}))
	defer server.Close()

	viper.Reset()
	viper.Set("github.owner", "test-owner")
	viper.Set("github.repo", "test-repo")
	viper.Set("github.token", "ghp_test123")

	SetAPIBaseURL(server.URL)
	resetClient()

	req := &CreateIssueRequest{
		Title: "Test Title",
		Body:  "Test body content",
	}

	issue, err := CreateIssue(context.Background(), req, "app-user-123")
	if err != nil {
		t.Fatalf("CreateIssue failed: %v", err)
	}

	if issue.Number != 123 {
		t.Errorf("expected number 123, got %d", issue.Number)
	}

	// Verify metadata was injected
	body := receivedBody["body"].(string)
	if body != "Test body content\n\n<!-- app_user_id: app-user-123 -->" {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestCreateComment(t *testing.T) {
	DisableCache()
	defer EnableCache()

	var receivedBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/repos/test-owner/test-repo/issues/42/comments" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		json.NewDecoder(r.Body).Decode(&receivedBody)

		comment := ghComment{
			ID:        456,
			Body:      receivedBody["body"].(string),
			User:      ghUser{Login: "test-user"},
			CreatedAt: time.Now(),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(comment)
	}))
	defer server.Close()

	viper.Reset()
	viper.Set("github.owner", "test-owner")
	viper.Set("github.repo", "test-repo")
	viper.Set("github.token", "ghp_test123")

	SetAPIBaseURL(server.URL)
	resetClient()

	req := &CreateCommentRequest{
		Body: "Test comment",
	}

	comment, err := CreateComment(context.Background(), 42, req, "app-user-456")
	if err != nil {
		t.Fatalf("CreateComment failed: %v", err)
	}

	if comment.ID != 456 {
		t.Errorf("expected id 456, got %d", comment.ID)
	}

	// Verify metadata was injected
	body := receivedBody["body"].(string)
	if body != "Test comment\n\n<!-- app_user_id: app-user-456 -->" {
		t.Errorf("unexpected body: %s", body)
	}
}

// ========== Transform Function Tests ==========

func TestTransformToIssue(t *testing.T) {
	gh := &ghIssue{
		Number:    42,
		Title:     "Test",
		Body:      "Body content\n\n<!-- app_user_id: user123 -->",
		State:     "open",
		Labels:    []ghLabel{{Name: "bug"}, {Name: "official-reply"}},
		Comments:  5,
		CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
	}

	viper.Reset()
	viper.Set("github.official_label", "official-reply")
	resetClient()

	issue := transformToIssue(gh)

	if issue.Number != 42 {
		t.Errorf("expected number 42, got %d", issue.Number)
	}
	if issue.Body != "Body content" {
		t.Errorf("expected body 'Body content', got '%s'", issue.Body)
	}
	if !issue.HasOfficial {
		t.Error("expected HasOfficial to be true")
	}
	if len(issue.Labels) != 2 {
		t.Errorf("expected 2 labels, got %d", len(issue.Labels))
	}
}

func TestTransformToComment(t *testing.T) {
	gh := &ghComment{
		ID:        101,
		Body:      "Comment body\n\n<!-- app_user_id: user123 -->",
		User:      ghUser{Login: "official-bot"},
		CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	viper.Reset()
	viper.Set("github.official_users", []string{"official-bot"})
	resetClient()

	comment := transformToComment(gh)

	if comment.ID != 101 {
		t.Errorf("expected id 101, got %d", comment.ID)
	}
	if comment.Body != "Comment body" {
		t.Errorf("expected body 'Comment body', got '%s'", comment.Body)
	}
	if !comment.IsOfficial {
		t.Error("expected IsOfficial to be true")
	}
}
