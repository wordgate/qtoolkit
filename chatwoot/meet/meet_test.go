package meet

import (
	"testing"
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
