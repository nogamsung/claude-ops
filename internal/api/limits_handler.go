package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/gs97ahn/claude-ops/internal/usecase"
)

// LimitsHandler handles /modes/limits endpoints.
type LimitsHandler struct {
	budgetUC *usecase.BudgetUseCase
}

// NewLimitsHandler creates a LimitsHandler.
func NewLimitsHandler(budgetUC *usecase.BudgetUseCase) *LimitsHandler {
	return &LimitsHandler{budgetUC: budgetUC}
}

// GetLimits godoc
//
//	@Summary      Get current task limits and counters
//	@Description  Returns daily/weekly task counters, configured caps, and any active CLI rate-limit block.
//	@Tags         modes
//	@Produce      json
//	@Success      200  {object}  LimitsResponse
//	@Router       /modes/limits [get]
func (h *LimitsHandler) GetLimits(c *gin.Context) {
	snap, err := h.budgetUC.Snapshot(c.Request.Context(), time.Now())
	if err != nil {
		mapError(c, err)
		return
	}
	c.JSON(http.StatusOK, snapshotToResponse(snap))
}

// PatchLimits godoc
//
//	@Summary      Override the configured daily/weekly task caps
//	@Description  Persists a runtime override of the daily and/or weekly task cap. A value of 0 falls back to the config-derived default.
//	@Tags         modes
//	@Accept       json
//	@Produce      json
//	@Param        request  body      LimitsPatchRequest  true  "Override values"
//	@Success      200      {object}  LimitsResponse
//	@Failure      400      {object}  ErrorResponse
//	@Router       /modes/limits [patch]
func (h *LimitsHandler) PatchLimits(c *gin.Context) {
	var req LimitsPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
		return
	}
	daily := 0
	if req.DailyMax != nil {
		daily = *req.DailyMax
	}
	weekly := 0
	if req.WeeklyMax != nil {
		weekly = *req.WeeklyMax
	}
	if _, err := h.budgetUC.SetLimits(c.Request.Context(), daily, weekly); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	snap, err := h.budgetUC.Snapshot(c.Request.Context(), time.Now())
	if err != nil {
		mapError(c, err)
		return
	}
	c.JSON(http.StatusOK, snapshotToResponse(snap))
}

func snapshotToResponse(snap usecase.BudgetSnapshot) LimitsResponse {
	resp := LimitsResponse{
		Daily: LimitsDaily{
			Count: snap.Counters.DailyCount,
			Max:   snap.Limits.DailyMax,
			Date:  snap.Counters.DailyKey,
		},
		Weekly: LimitsWeekly{
			Count: snap.Counters.WeeklyCount,
			Max:   snap.Limits.WeeklyMax,
			Week:  snap.Counters.WeeklyKey,
		},
		Reason: string(snap.Reason),
	}
	if snap.Block.Active(time.Now()) {
		until := snap.Block.BlockedUntil
		resp.RateLimit = LimitsRateLimit{
			BlockedUntil:  &until,
			RateLimitType: snap.Block.RateLimitType,
		}
	}
	return resp
}
