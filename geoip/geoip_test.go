package geoip

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func resetState() {
	if stopCh != nil {
		close(stopCh)
		stopCh = nil
	}
	readerMux.Lock()
	if globalReader != nil {
		globalReader.Close()
		globalReader = nil
	}
	readerMux.Unlock()
	initOnce = sync.Once{}
	initErr = nil
	downloadURL = ""
	dbPath = ""
}

// testMMDBPath returns the path to a real GeoLite2-Country.mmdb for testing.
// Downloads one from the default mirror if not cached.
func testMMDBPath(t *testing.T) string {
	t.Helper()
	cacheDir := t.TempDir()
	path := filepath.Join(cacheDir, dbFileName)

	// Download a real .mmdb for testing
	if err := downloadDB(defaultDownloadURL, path); err != nil {
		t.Skipf("cannot download test database (network unavailable): %v", err)
	}
	return path
}

func TestCountry_Success(t *testing.T) {
	resetState()

	mmdbPath := testMMDBPath(t)

	// Serve the .mmdb from a local HTTP server
	data, err := os.ReadFile(mmdbPath)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	SetConfig(server.URL, filepath.Join(tmpDir, dbFileName))

	got, err := Country("8.8.8.8")
	if err != nil {
		t.Fatalf("Country(8.8.8.8) error: %v", err)
	}
	if got != "US" {
		t.Errorf("Country(8.8.8.8) = %q, want US", got)
	}

	// Verify another well-known public IP
	got2, err := Country("139.130.4.5")
	if err != nil {
		t.Fatalf("Country(139.130.4.5) error: %v", err)
	}
	if got2 != "AU" {
		t.Errorf("Country(139.130.4.5) = %q, want AU", got2)
	}
}

func TestCountry_PrivateIP(t *testing.T) {
	resetState()

	mmdbPath := testMMDBPath(t)
	data, err := os.ReadFile(mmdbPath)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	SetConfig(server.URL, filepath.Join(tmpDir, dbFileName))

	got, err := Country("192.168.1.1")
	if err != nil {
		t.Errorf("Country(192.168.1.1) error: %v", err)
	}
	if got != "" {
		t.Errorf("Country(192.168.1.1) = %q, want empty string", got)
	}
}

func TestCountry_InvalidIP(t *testing.T) {
	resetState()

	mmdbPath := testMMDBPath(t)
	data, err := os.ReadFile(mmdbPath)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	SetConfig(server.URL, filepath.Join(tmpDir, dbFileName))

	_, err = Country("not-an-ip")
	if err == nil {
		t.Error("expected error for invalid IP")
	}
}

func TestCountry_DownloadFailed(t *testing.T) {
	resetState()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	SetConfig(server.URL, filepath.Join(tmpDir, dbFileName))

	_, err := Country("8.8.8.8")
	if err == nil {
		t.Error("expected error when download fails")
	}
}

func TestCountry_CachedDB(t *testing.T) {
	resetState()

	mmdbPath := testMMDBPath(t)

	// Copy the .mmdb to a temp location to simulate a cached DB
	data, err := os.ReadFile(mmdbPath)
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	cachedPath := filepath.Join(tmpDir, dbFileName)
	if err := os.WriteFile(cachedPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	// Server that should NOT be called (DB is already cached)
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	SetConfig(server.URL, cachedPath)

	got, err := Country("8.8.8.8")
	if err != nil {
		t.Fatalf("Country(8.8.8.8) error: %v", err)
	}
	if got != "US" {
		t.Errorf("Country(8.8.8.8) = %q, want US", got)
	}
	if called {
		t.Error("server was called despite cached DB existing")
	}
}

func TestClose(t *testing.T) {
	resetState()

	mmdbPath := testMMDBPath(t)
	data, err := os.ReadFile(mmdbPath)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	SetConfig(server.URL, filepath.Join(tmpDir, dbFileName))

	_, err = Country("8.8.8.8")
	if err != nil {
		t.Fatalf("Country error: %v", err)
	}

	if err := Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}
}
