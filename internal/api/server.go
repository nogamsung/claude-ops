package api

import (
	"context" // ADDED
	"net/http"

	"github.com/gin-gonic/gin"
)

// GitHubWebhookHandler is the interface for the GitHub webhook handler.
type GitHubWebhookHandler interface {
	HandleWebhook(c *gin.Context)
}

// NewRouter creates and configures a Gin router with all handlers.
func NewRouter(
	healthHandler *HealthHandler,
	taskHandler *TaskHandler,
	modeHandler *ModeHandler,
	limitsHandler *LimitsHandler,
	slackHandler *SlackHandler,
	webhookHandler GitHubWebhookHandler,
) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())
	r.Use(requestIDMiddleware())

	// Swagger UI
	r.Static("/swagger", "./docs/swagger")

	// Health
	r.GET("/healthz", healthHandler.Health)

	// Tasks
	r.GET("/tasks", taskHandler.ListTasks)
	r.GET("/tasks/:id", taskHandler.GetTask)
	r.POST("/tasks", taskHandler.EnqueueTask)
	r.POST("/tasks/:id/stop", taskHandler.StopTask)

	// Full mode
	r.GET("/modes/full", modeHandler.GetFullMode)
	r.POST("/modes/full", modeHandler.SetFullMode)

	// Task budget limits
	r.GET("/modes/limits", limitsHandler.GetLimits)
	r.PATCH("/modes/limits", limitsHandler.PatchLimits)

	// Slack
	r.POST("/slack/interactions", slackHandler.HandleInteractions)

	// GitHub webhook — registered only when handler is provided (secret configured).
	if webhookHandler != nil {
		r.POST("/github/webhook", webhookHandler.HandleWebhook)
	}

	return r
}

// Server wraps the http.Server for graceful lifecycle management.
type Server struct {
	srv *http.Server
}

// NewServer creates a Server bound to addr.
func NewServer(addr string, handler http.Handler) *Server {
	return &Server{
		srv: &http.Server{
			Addr:    addr,
			Handler: handler,
		},
	}
}

// ListenAndServe starts the HTTP server (blocking).
func (s *Server) ListenAndServe() error {
	return s.srv.ListenAndServe()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error { // MODIFIED: was interface{Done()<-chan struct{}} no-op
	return s.srv.Shutdown(ctx) // MODIFIED: was return nil
}

func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-Id")
		if id == "" {
			id = "none"
		}
		c.Set("request_id", id)
		c.Header("X-Request-Id", id)
		c.Next()
	}
}
