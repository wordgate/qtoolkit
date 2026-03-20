package meet

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	qtredis "github.com/wordgate/qtoolkit/redis"
)

func TestGenerateToken(t *testing.T) {
	token1, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken error: %v", err)
	}
	if len(token1) < 40 {
		t.Errorf("token length = %d, want >= 40", len(token1))
	}

	// Tokens should be unique
	token2, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken error: %v", err)
	}
	if token1 == token2 {
		t.Error("two generated tokens are identical")
	}
}

func TestGenerateID(t *testing.T) {
	id1, err := generateID()
	if err != nil {
		t.Fatalf("generateID error: %v", err)
	}
	if len(id1) != 12 {
		t.Errorf("id length = %d, want 12", len(id1))
	}

	id2, err := generateID()
	if err != nil {
		t.Fatalf("generateID error: %v", err)
	}
	if id1 == id2 {
		t.Error("two generated IDs are identical")
	}
}

func TestCreateParticipantToken(t *testing.T) {
	resetState()
	SetConfig(&Config{
		LiveKit: LiveKitConfig{
			URL:       "wss://test.livekit.cloud",
			APIKey:    "test-api-key",
			APISecret: "test-api-secret-that-is-long-enough-for-hmac",
		},
		TokenExpiry: 24 * time.Hour,
		RoomTimeout: 60 * time.Minute,
		BaseURL:     "https://example.com",
	})

	token, err := createParticipantToken("test-room", "user-123", "Test User")
	if err != nil {
		t.Fatalf("createParticipantToken error: %v", err)
	}
	if token == "" {
		t.Error("participant token is empty")
	}
}

func requireRedis(t *testing.T) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Skip("Redis not available, skipping integration test")
		}
	}()
	ctx := context.Background()
	if err := qtredis.Client().Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping integration test")
	}
}

func TestHandleMeetPage_InvalidToken(t *testing.T) {
	requireRedis(t)
	resetState()
	SetConfig(&Config{
		LiveKit: LiveKitConfig{
			URL:       "wss://test.livekit.cloud",
			APIKey:    "test-key",
			APISecret: "test-secret-long-enough-for-hmac-signing",
		},
		TokenExpiry: 24 * time.Hour,
		RoomTimeout: 60 * time.Minute,
		BaseURL:     "https://example.com",
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	Mount(router, "/meet", nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/meet/invalid-token-here", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "链接无效") {
		t.Error("response should contain error message for invalid token")
	}
}
