package issue

import (
	"testing"
	"time"

	"github.com/spf13/viper"
)

// ========== Config Tests ==========

func TestLoadConfigFromViper(t *testing.T) {
	viper.Reset()
	viper.Set("github.owner", "test-owner")
	viper.Set("github.repo", "test-repo")
	viper.Set("github.token", "ghp_test123")
	viper.Set("github.official_label", "official")
	viper.Set("github.cache_ttl", 600)

	cfg := loadConfigFromViper()

	if cfg.Owner != "test-owner" {
		t.Errorf("expected owner 'test-owner', got '%s'", cfg.Owner)
	}
	if cfg.Repo != "test-repo" {
		t.Errorf("expected repo 'test-repo', got '%s'", cfg.Repo)
	}
	if cfg.Token != "ghp_test123" {
		t.Errorf("expected token 'ghp_test123', got '%s'", cfg.Token)
	}
	if cfg.OfficialLabel != "official" {
		t.Errorf("expected official_label 'official', got '%s'", cfg.OfficialLabel)
	}
	if cfg.CacheTTL != 600 {
		t.Errorf("expected cache_ttl 600, got %d", cfg.CacheTTL)
	}
}

func TestLoadConfigFromViperDefaults(t *testing.T) {
	viper.Reset()
	viper.Set("github.owner", "test-owner")
	viper.Set("github.repo", "test-repo")
	viper.Set("github.token", "ghp_test123")
	// No official_label and cache_ttl set

	cfg := loadConfigFromViper()

	if cfg.OfficialLabel != "official-reply" {
		t.Errorf("expected default official_label 'official-reply', got '%s'", cfg.OfficialLabel)
	}
	if cfg.CacheTTL != 300 {
		t.Errorf("expected default cache_ttl 300, got %d", cfg.CacheTTL)
	}
}

// ========== DTO Tests ==========

func TestIssueDTO(t *testing.T) {
	issue := Issue{
		Number:       42,
		Title:        "Test Issue",
		Body:         "This is a test body",
		State:        "open",
		Labels:       []string{"bug", "official-reply"},
		HasOfficial:  true,
		CommentCount: 5,
		CreatedAt:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
	}

	if issue.Number != 42 {
		t.Errorf("expected number 42, got %d", issue.Number)
	}
	if issue.Title != "Test Issue" {
		t.Errorf("expected title 'Test Issue', got '%s'", issue.Title)
	}
	if !issue.HasOfficial {
		t.Error("expected HasOfficial to be true")
	}
}

func TestCommentDTO(t *testing.T) {
	comment := Comment{
		ID:         123,
		Body:       "This is a comment",
		IsOfficial: true,
		CreatedAt:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	if comment.ID != 123 {
		t.Errorf("expected id 123, got %d", comment.ID)
	}
	if comment.Body != "This is a comment" {
		t.Errorf("expected body 'This is a comment', got '%s'", comment.Body)
	}
	if !comment.IsOfficial {
		t.Error("expected IsOfficial to be true")
	}
}

func TestIssueDetailDTO(t *testing.T) {
	detail := IssueDetail{
		Issue: Issue{
			Number: 42,
			Title:  "Test Issue",
		},
		Comments: []Comment{
			{ID: 1, Body: "First comment"},
			{ID: 2, Body: "Second comment"},
		},
	}

	if detail.Number != 42 {
		t.Errorf("expected number 42, got %d", detail.Number)
	}
	if len(detail.Comments) != 2 {
		t.Errorf("expected 2 comments, got %d", len(detail.Comments))
	}
}

func TestListIssuesResponse(t *testing.T) {
	resp := ListIssuesResponse{
		Issues: []Issue{
			{Number: 1, Title: "Issue 1"},
			{Number: 2, Title: "Issue 2"},
		},
		Page:    1,
		PerPage: 20,
		HasMore: true,
	}

	if len(resp.Issues) != 2 {
		t.Errorf("expected 2 issues, got %d", len(resp.Issues))
	}
	if resp.Page != 1 {
		t.Errorf("expected page 1, got %d", resp.Page)
	}
	if !resp.HasMore {
		t.Error("expected HasMore to be true")
	}
}

// ========== Request DTO Validation Tests ==========

func TestCreateIssueRequest(t *testing.T) {
	req := CreateIssueRequest{
		Title: "Test Title",
		Body:  "Test body content",
	}

	if req.Title != "Test Title" {
		t.Errorf("expected title 'Test Title', got '%s'", req.Title)
	}
	if req.Body != "Test body content" {
		t.Errorf("expected body 'Test body content', got '%s'", req.Body)
	}
}

func TestCreateCommentRequest(t *testing.T) {
	req := CreateCommentRequest{
		Body: "Test comment",
	}

	if req.Body != "Test comment" {
		t.Errorf("expected body 'Test comment', got '%s'", req.Body)
	}
}

// ========== Utility Function Tests ==========

func TestStripMetadata(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "body with metadata",
			input:    "This is the body content\n\n<!-- app_user_id: user123 -->",
			expected: "This is the body content",
		},
		{
			name:     "body without metadata",
			input:    "This is the body content",
			expected: "This is the body content",
		},
		{
			name:     "body with metadata in middle (should not strip)",
			input:    "Start <!-- app_user_id: user123 --> End",
			expected: "Start <!-- app_user_id: user123 --> End",
		},
		{
			name:     "empty body",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripMetadata(tt.input)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestInjectMetadata(t *testing.T) {
	body := "This is the body"
	userID := "user123"

	result := injectMetadata(body, userID)

	expected := "This is the body\n\n<!-- app_user_id: user123 -->"
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}
