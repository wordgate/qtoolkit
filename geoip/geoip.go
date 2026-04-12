// Package geoip provides country-level IP geolocation using MaxMind GeoLite2-Country.
//
// Zero configuration required. The database is automatically downloaded from a
// GitHub mirror and refreshed every 24 hours.
//
// Usage:
//
//	country, err := geoip.Country("8.8.8.8")   // "US"
//	country, err := geoip.Country("1.1.1.1")   // "AU"
package geoip

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/oschwald/geoip2-golang"
	"github.com/spf13/viper"
)

const (
	defaultDownloadURL = "https://raw.githubusercontent.com/Loyalsoldier/geoip/release/GeoLite2-Country.mmdb"
	dbFileName         = "GeoLite2-Country.mmdb"
	updateInterval     = 24 * time.Hour
	downloadTimeout    = 5 * time.Minute
)

// Errors
var (
	ErrNotInitialized = errors.New("geoip: not initialized")
	ErrInvalidIP      = errors.New("geoip: invalid IP address")
	ErrDownloadFailed = errors.New("geoip: database download failed")
)

var (
	globalReader *geoip2.Reader
	readerMux    sync.RWMutex
	initOnce     sync.Once
	initErr      error
	stopCh       chan struct{}
	downloadURL  string
	dbPath       string
)

// Country returns the ISO 3166-1 alpha-2 country code for an IP address.
// Returns "" if the IP is not found (e.g., private IPs).
func Country(ip string) (string, error) {
	initOnce.Do(initialize)
	if initErr != nil {
		return "", initErr
	}

	parsed := net.ParseIP(ip)
	if parsed == nil {
		return "", fmt.Errorf("%w: %s", ErrInvalidIP, ip)
	}

	readerMux.RLock()
	defer readerMux.RUnlock()

	record, err := globalReader.Country(parsed)
	if err != nil {
		return "", fmt.Errorf("geoip: lookup failed: %w", err)
	}

	return record.Country.IsoCode, nil
}

// Close stops the background updater and releases the database reader.
func Close() error {
	if stopCh != nil {
		close(stopCh)
	}

	readerMux.Lock()
	defer readerMux.Unlock()

	if globalReader != nil {
		err := globalReader.Close()
		globalReader = nil
		return err
	}
	return nil
}

// SetConfig sets the download URL and database path for testing.
func SetConfig(url, path string) {
	readerMux.Lock()
	defer readerMux.Unlock()
	downloadURL = url
	dbPath = path
	initOnce = sync.Once{}
	initErr = nil
	if globalReader != nil {
		globalReader.Close()
		globalReader = nil
	}
	if stopCh != nil {
		close(stopCh)
		stopCh = nil
	}
}

func initialize() {
	if downloadURL == "" {
		downloadURL = viper.GetString("geoip.download_url")
	}
	if downloadURL == "" {
		downloadURL = defaultDownloadURL
	}

	if dbPath == "" {
		path, err := resolveDBPath()
		if err != nil {
			initErr = fmt.Errorf("geoip: cannot determine cache directory: %w", err)
			return
		}
		dbPath = path
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		initErr = fmt.Errorf("geoip: cannot create cache directory: %w", err)
		return
	}

	// If cached DB exists, open it directly (non-blocking first call)
	if _, err := os.Stat(dbPath); err == nil {
		reader, err := geoip2.Open(dbPath)
		if err != nil {
			// Cached file is corrupted, re-download
			log.Printf("geoip: cached database corrupted, re-downloading: %v", err)
		} else {
			globalReader = reader
		}
	}

	// If no reader yet, download synchronously (blocking first call)
	if globalReader == nil {
		if err := downloadDB(downloadURL, dbPath); err != nil {
			initErr = fmt.Errorf("%w: %v", ErrDownloadFailed, err)
			return
		}
		reader, err := geoip2.Open(dbPath)
		if err != nil {
			initErr = fmt.Errorf("geoip: cannot open downloaded database: %w", err)
			return
		}
		globalReader = reader
	}

	stopCh = make(chan struct{})
	startUpdater()
}

func resolveDBPath() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "qtoolkit", "geoip", dbFileName), nil
}

func downloadDB(url, destPath string) error {
	client := &http.Client{Timeout: downloadTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	tmpPath := destPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}

	return os.Rename(tmpPath, destPath)
}

func refreshDB() {
	tmpPath := dbPath + ".new"
	if err := downloadDB(downloadURL, tmpPath); err != nil {
		log.Printf("geoip: update download failed: %v", err)
		return
	}

	newReader, err := geoip2.Open(tmpPath)
	if err != nil {
		log.Printf("geoip: updated database invalid: %v", err)
		os.Remove(tmpPath)
		return
	}

	readerMux.Lock()
	oldReader := globalReader
	globalReader = newReader
	readerMux.Unlock()

	if oldReader != nil {
		oldReader.Close()
	}

	// Replace the main DB file
	os.Rename(tmpPath, dbPath)
}

func startUpdater() {
	ticker := time.NewTicker(updateInterval)
	go func() {
		for {
			select {
			case <-ticker.C:
				refreshDB()
			case <-stopCh:
				ticker.Stop()
				return
			}
		}
	}()
}
