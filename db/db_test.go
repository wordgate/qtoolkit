package db

import (
	"testing"
)

func TestSetConfig(t *testing.T) {
	Reset() // Reset state for clean test

	cfg := &Config{
		DSN:   "test_dsn",
		Debug: true,
	}

	SetConfig(cfg)

	retrieved := GetConfig()
	if retrieved == nil {
		t.Error("Expected config to be set, got nil")
	}
	if retrieved.DSN != "test_dsn" {
		t.Errorf("Expected DSN 'test_dsn', got '%s'", retrieved.DSN)
	}
	if !retrieved.Debug {
		t.Error("Expected Debug to be true")
	}
}

func TestGetWithoutConfig(t *testing.T) {
	Reset() // Reset state for clean test

	// Don't set config
	db := Get()

	// Should return nil since config wasn't set
	if db != nil {
		t.Error("Expected nil when config not set")
	}

	// Check error
	if GetError() == nil {
		t.Error("Expected error when config not set")
	}
}

func TestMustGetWithoutConfig_ShouldPanic(t *testing.T) {
	Reset() // Reset state for clean test

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected MustGet to panic when config not set")
		}
	}()

	MustGet() // Should panic
}

func TestLazyLoadOnlyOnce(t *testing.T) {
	Reset() // Reset state for clean test

	// Set invalid config
	SetConfig(&Config{
		DSN: "", // Invalid DSN
	})

	// Call Get multiple times
	db1 := Get()
	db2 := Get()
	db3 := Get()

	// All calls should return the same instance (nil in this case)
	if db1 != db2 || db2 != db3 {
		t.Error("Expected Get to return the same instance on multiple calls")
	}

	// Error should be set once and remain the same
	err1 := GetError()
	err2 := GetError()
	if err1 == nil {
		t.Error("Expected error when DSN is empty")
	}
	if err1 != err2 {
		t.Error("Expected same error instance on multiple calls")
	}
}

func TestReset(t *testing.T) {
	Reset() // Reset state for clean test

	// Set config
	SetConfig(&Config{
		DSN:   "test_dsn",
		Debug: true,
	})

	// Reset
	Reset()

	// Config should be nil after reset
	if GetConfig() != nil {
		t.Error("Expected config to be nil after Reset")
	}

	// Error should be nil after reset
	if GetError() != nil {
		t.Error("Expected error to be nil after Reset")
	}
}

// Example of lazy load usage
func ExampleGet() {
	// Set configuration before first use
	SetConfig(&Config{
		DSN:   "user:pass@tcp(localhost:3306)/dbname?charset=utf8mb4",
		Debug: false,
	})

	// Database is automatically initialized on first Get() call
	db := Get()
	if db != nil {
		// Use database
		// db.Create(&user)
	}
}

// Example of must get usage
func ExampleMustGet() {
	// Set configuration
	SetConfig(&Config{
		DSN:   "user:pass@tcp(localhost:3306)/dbname?charset=utf8mb4",
		Debug: true,
	})

	// Database is automatically initialized, panics if failed
	db := MustGet()

	// Use database
	_ = db
	// db.Create(&user)
}
