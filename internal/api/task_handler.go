package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/gs97ahn/scheduled-dev-agent/internal/domain"
	"github.com/gs97ahn/scheduled-dev-agent/internal/usecase"
)

// TaskHandler handles task-related HTTP requests.
type TaskHandler struct {
	taskUC *usecase.TaskUseCase
}

// NewTaskHandler creates a TaskHandler.
func NewTaskHandler(taskUC *usecase.TaskUseCase) *TaskHandler {
	return &TaskHandler{taskUC: taskUC}
}

// ListTasks godoc
//
//	@Summary      List tasks
//	@Tags         tasks
//	@Produce      json
//	@Param        status  query     string  false  "Filter by status (queued|running|done|failed|cancelled)"  example("queued")
//	@Param        limit   query     int     false  "Maximum number of results"  example(50)
//	@Param        cursor  query     string  false  "Pagination cursor"
//	@Success      200     {object}  TaskListResponse
//	@Router       /tasks [get]
func (h *TaskHandler) ListTasks(c *gin.Context) {
	filter := domain.TaskFilter{}

	if s := c.Query("status"); s != "" {
		status := domain.TaskStatus(s)
		filter.Status = &status
	}
	if l := c.Query("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err == nil {
			filter.Limit = n
		}
	}
	filter.Cursor = c.Query("cursor")

	tasks, err := h.taskUC.ListTasks(c.Request.Context(), filter)
	if err != nil {
		mapError(c, err)
		return
	}

	items := make([]TaskResponse, len(tasks))
	for i, t := range tasks {
		items[i] = toTaskResponse(t)
	}

	var nextCursor string
	if len(items) > 0 {
		nextCursor = items[len(items)-1].ID
	}

	c.JSON(http.StatusOK, TaskListResponse{
		Items:      items,
		NextCursor: nextCursor,
	})
}

// GetTask godoc
//
//	@Summary      Get task details
//	@Tags         tasks
//	@Produce      json
//	@Param        id   path      string  true  "Task ID"  example("550e8400-e29b-41d4-a716-446655440000")
//	@Success      200  {object}  TaskDetailResponse
//	@Failure      404  {object}  ErrorResponse
//	@Router       /tasks/{id} [get]
func (h *TaskHandler) GetTask(c *gin.Context) {
	id := c.Param("id")
	detail, err := h.taskUC.GetTask(c.Request.Context(), id)
	if err != nil {
		mapError(c, err)
		return
	}

	resp := TaskDetailResponse{
		TaskResponse: toTaskResponse(detail.Task),
		StderrTail:   detail.Task.StderrTail,
	}
	for _, e := range detail.Events {
		resp.Events = append(resp.Events, TaskEventResponse{
			ID:          e.ID,
			Kind:        string(e.Kind),
			PayloadJSON: e.PayloadJSON,
			CreatedAt:   e.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, resp)
}

// EnqueueTask godoc
//
//	@Summary      Manually enqueue a task
//	@Tags         tasks
//	@Accept       json
//	@Produce      json
//	@Param        request  body      EnqueueRequest  true  "Issue to process"
//	@Success      201      {object}  TaskResponse
//	@Failure      400      {object}  ErrorResponse
//	@Failure      409      {object}  ErrorResponse
//	@Router       /tasks [post]
func (h *TaskHandler) EnqueueTask(c *gin.Context) {
	var req EnqueueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
		return
	}

	task, err := h.taskUC.EnqueueFromIssue(c.Request.Context(), usecase.EnqueueRequest{
		RepoFullName: req.RepoFullName,
		IssueNumber:  req.IssueNumber,
		IssueTitle:   req.IssueTitle,
	})
	if err != nil {
		mapError(c, err)
		return
	}

	c.JSON(http.StatusCreated, toTaskResponse(task))
}

// StopTask godoc
//
//	@Summary      Stop a running or queued task
//	@Tags         tasks
//	@Produce      json
//	@Param        id   path      string  true  "Task ID"  example("550e8400-e29b-41d4-a716-446655440000")
//	@Success      202  {object}  StopResponse
//	@Failure      404  {object}  ErrorResponse
//	@Failure      409  {object}  ErrorResponse
//	@Router       /tasks/{id}/stop [post]
func (h *TaskHandler) StopTask(c *gin.Context) {
	id := c.Param("id")
	if err := h.taskUC.StopTask(c.Request.Context(), id); err != nil {
		mapError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, StopResponse{Accepted: true})
}

func toTaskResponse(t *domain.Task) TaskResponse {
	return TaskResponse{
		ID:                    t.ID,
		RepoFullName:          t.RepoFullName,
		IssueNumber:           t.IssueNumber,
		IssueTitle:            t.IssueTitle,
		TaskType:              string(t.TaskType),
		Status:                string(t.Status),
		PRURL:                 t.PRURL,
		PRNumber:              t.PRNumber,
		StartedAt:             t.StartedAt,
		FinishedAt:            t.FinishedAt,
		EstimatedInputTokens:  t.EstimatedInputTokens,
		EstimatedOutputTokens: t.EstimatedOutputTokens,
		CreatedAt:             t.CreatedAt,
		UpdatedAt:             t.UpdatedAt,
	}
}
