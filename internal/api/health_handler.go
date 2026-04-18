package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gs97ahn/scheduled-dev-agent/internal/usecase"
)

// HealthHandler handles health check requests.
type HealthHandler struct {
	modeUC *usecase.ModeUseCase
}

// NewHealthHandler creates a HealthHandler.
func NewHealthHandler(modeUC *usecase.ModeUseCase) *HealthHandler {
	return &HealthHandler{modeUC: modeUC}
}

// Health godoc
//
//	@Summary      Health check
//	@Tags         health
//	@Produce      json
//	@Success      200  {object}  HealthResponse
//	@Router       /healthz [get]
func (h *HealthHandler) Health(c *gin.Context) {
	fullMode := false
	if h.modeUC != nil {
		state, err := h.modeUC.GetFullMode(c.Request.Context())
		if err == nil {
			fullMode = state.Enabled
		}
	}
	c.JSON(http.StatusOK, HealthResponse{
		Status:   "ok",
		FullMode: fullMode,
	})
}
