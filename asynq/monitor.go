package asynq

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynqmon"
)

// Mount mounts the asynqmon web UI to the gin router at the specified path.
// Automatically starts the worker if handlers are registered.
//
// Example:
//
//	// Basic usage
//	asynq.Mount(r, "/asynq")
//
//	// With authentication middleware
//	asynq.Mount(r.Group("/admin", authMiddleware), "/tasks")
func Mount(r gin.IRouter, path string) {
	// Ensure path starts with /
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// Calculate full base path for asynqmon
	basePath := path
	if rg, ok := r.(*gin.RouterGroup); ok && rg.BasePath() != "/" {
		basePath = rg.BasePath() + path
	}

	// Auto-start worker
	ensureWorkerStarted()

	cfg := loadConfig()
	h := asynqmon.New(asynqmon.Options{
		RootPath:     basePath,
		RedisConnOpt: getRedisOpt(),
		ReadOnly:     cfg.Monitor.ReadOnly,
	})

	// Register both exact path and wildcard to handle trailing slash redirects
	r.Any(path, gin.WrapH(h))
	r.Any(path+"/*any", gin.WrapH(h))
}

// MonitorHandler returns a gin.HandlerFunc for the asynqmon web UI.
// Use Mount() for simpler setup. This is for advanced cases where you need
// full control over routing.
//
// Example:
//
//	r.Any("/asynq/*any", asynq.MonitorHandler("/asynq"))
func MonitorHandler(basePath string) gin.HandlerFunc {
	// Auto-start worker when monitor is mounted
	ensureWorkerStarted()

	cfg := loadConfig()

	h := asynqmon.New(asynqmon.Options{
		RootPath:     basePath,
		RedisConnOpt: getRedisOpt(),
		ReadOnly:     cfg.Monitor.ReadOnly,
	})

	return gin.WrapH(h)
}
