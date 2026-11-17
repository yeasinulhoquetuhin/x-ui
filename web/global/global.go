// Package global provides global variables and interfaces for accessing web and subscription servers.
package global

import (
	"context"
	"sync"
	_ "unsafe"

	"github.com/robfig/cron/v3"
)

var (
	webServer   WebServer
	subServer   SubServer
	subServerMu sync.RWMutex
)

// WebServer interface defines methods for accessing the web server instance.
type WebServer interface {
	GetCron() *cron.Cron     // Get the cron scheduler
	GetCtx() context.Context // Get the server context
	GetWSHub() any           // Get the WebSocket hub (using any to avoid circular dependency)
}

// SubServer interface defines methods for accessing the subscription server instance.
type SubServer interface {
	GetCtx() context.Context // Get the server context
}

// SetWebServer sets the global web server instance.
func SetWebServer(s WebServer) {
	webServer = s
}

// GetWebServer returns the global web server instance.
func GetWebServer() WebServer {
	return webServer
}

// SetSubServer sets the global subscription server instance.
func SetSubServer(s SubServer) {
	subServerMu.Lock()
	defer subServerMu.Unlock()
	subServer = s
}

// GetSubServer returns the global subscription server instance.
func GetSubServer() SubServer {
	subServerMu.RLock()
	defer subServerMu.RUnlock()
	return subServer
}
