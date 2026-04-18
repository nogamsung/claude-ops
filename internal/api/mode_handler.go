package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gs97ahn/claude-ops/internal/usecase"
)

// ModeHandler handles full-mode toggle requests.
type ModeHandler struct {
	modeUC *usecase.ModeUseCase
}

// NewModeHandler creates a ModeHandler.
func NewModeHandler(modeUC *usecase.ModeUseCase) *ModeHandler {
	return &ModeHandler{modeUC: modeUC}
}

// GetFullMode godoc
//
//	@Summary      Get full usage mode status
//	@Tags         modes
//	@Produce      json
//	@Success      200  {object}  FullModeResponse
//	@Router       /modes/full [get]
func (h *ModeHandler) GetFullMode(c *gin.Context) {
	state, err := h.modeUC.GetFullMode(c.Request.Context())
	if err != nil {
		mapError(c, err)
		return
	}
	c.JSON(http.StatusOK, FullModeResponse{
		Enabled: state.Enabled,
		Since:   state.Since,
	})
}

// SetFullMode godoc
//
//	@Summary      Toggle full usage mode
//	@Tags         modes
//	@Accept       json
//	@Produce      json
//	@Param        request  body      FullModeRequest  true  "Enable or disable full mode"
//	@Success      200      {object}  FullModeResponse
//	@Failure      400      {object}  ErrorResponse
//	@Router       /modes/full [post]
func (h *ModeHandler) SetFullMode(c *gin.Context) {
	var req FullModeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
		return
	}

	state, err := h.modeUC.SetFullMode(c.Request.Context(), req.Enabled)
	if err != nil {
		mapError(c, err)
		return
	}

	c.JSON(http.StatusOK, FullModeResponse{
		Enabled: state.Enabled,
		Since:   state.Since,
	})
}
