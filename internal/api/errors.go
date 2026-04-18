package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gs97ahn/scheduled-dev-agent/internal/domain"
)

// mapError maps domain errors to HTTP status codes and writes the JSON error response.
// Internal error messages are never exposed directly.
func mapError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "not found"})
	case errors.Is(err, domain.ErrOutsideActiveWindow):
		c.JSON(http.StatusConflict, ErrorResponse{Error: "outside active window; enable full mode to override"})
	case errors.Is(err, domain.ErrAlreadyRunning):
		c.JSON(http.StatusConflict, ErrorResponse{Error: "task already queued or running for this issue"})
	case errors.Is(err, domain.ErrTaskNotCancellable):
		c.JSON(http.StatusConflict, ErrorResponse{Error: "task cannot be cancelled in its current state"})
	case errors.Is(err, domain.ErrClaudeUsageExhausted):
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "claude usage exhausted"})
	case errors.Is(err, domain.ErrSessionMissing):
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "claude session missing; please run 'claude login'"})
	default:
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
	}
}
