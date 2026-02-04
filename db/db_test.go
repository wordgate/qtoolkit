package db

import (
	"testing"

	"github.com/spf13/viper"
)

func TestGetWithoutConfig(t *testing.T) {
	Reset() // Reset state for clean test
	viper.Reset()

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
	viper.Reset()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected MustGet to panic when config not set")
		}
	}()

	MustGet() // Should panic
}

func TestLazyLoadOnlyOnce(t *testing.T) {
	Reset() // Reset state for clean test
	viper.Reset()

	// Set invalid config (empty DSN)
	viper.Set("database.dsn", "")

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
	viper.Reset()

	// Set config via viper
	viper.Set("database.dsn", "test_dsn")
	viper.Set("database.debug", true)

	// Reset
	Reset()

	// Error should be nil after reset
	if GetError() != nil {
		t.Error("Expected error to be nil after Reset")
	}
}

// Example of lazy load usage with viper configuration
func ExampleGet() {
	// Set configuration via viper before first use
	viper.Set("database.dsn", "user:pass@tcp(localhost:3306)/dbname?charset=utf8mb4")
	viper.Set("database.debug", false)

	// Database is automatically initialized on first Get() call
	db := Get()
	if db != nil {
		// Use database
		// db.Create(&user)
	}
}

// Example of must get usage
func ExampleMustGet() {
	// Set configuration via viper
	viper.Set("database.dsn", "user:pass@tcp(localhost:3306)/dbname?charset=utf8mb4")
	viper.Set("database.debug", true)

	// Database is automatically initialized, panics if failed
	db := MustGet()

	// Use database
	_ = db
	// db.Create(&user)
}
