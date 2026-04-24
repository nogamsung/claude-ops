// Package main is the entrypoint for the claude-ops binary.
//
//	@title          Claude Ops API
//	@version        1.0
//	@description    GitHub issue to PR automation agent with active-window scheduling.
//	@BasePath       /
//	@contact.name   gs97ahn
//	@contact.email  gs97ahn@gmail.com
//
//	@securityDefinitions.apikey  BearerAuth
//	@in                          header
//	@name                        Authorization
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/gs97ahn/claude-ops/internal/api"
	"github.com/gs97ahn/claude-ops/internal/claude"
	"github.com/gs97ahn/claude-ops/internal/config"
	"github.com/gs97ahn/claude-ops/internal/domain"
	igithub "github.com/gs97ahn/claude-ops/internal/github"
	"github.com/gs97ahn/claude-ops/internal/metrics"
	"github.com/gs97ahn/claude-ops/internal/qualitygate"
	"github.com/gs97ahn/claude-ops/internal/repository"
	"github.com/gs97ahn/claude-ops/internal/scheduler"
	islack "github.com/gs97ahn/claude-ops/internal/slack"
	"github.com/gs97ahn/claude-ops/internal/usecase"

	"github.com/golang-migrate/migrate/v4"
	migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "config.example.yaml", "path to config file")
	flag.Parse()

	// Fail fast: check required CLI tools.
	checkCLI("claude")
	checkCLI("git")

	// Load config.
	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err = cfg.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}

	env, err := config.LoadEnv()
	if err != nil {
		return fmt.Errorf("load env: %w", err)
	}

	// Logging.
	level := slog.LevelInfo
	if cfg.Runtime.LogLevel == "debug" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	// Database.
	db, err := repository.NewDB(cfg.Runtime.DBPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get sql.DB: %w", err)
	}
	defer func() {
		if closeErr := sqlDB.Close(); closeErr != nil {
			slog.Error("close db", "err", closeErr)
		}
	}()

	// Run migrations.
	driver, err := migratesqlite.WithInstance(sqlDB, &migratesqlite.Config{})
	if err != nil {
		return fmt.Errorf("migrate driver: %w", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://migrations", "sqlite3", driver)
	if err != nil {
		return fmt.Errorf("migration init: %w", err)
	}
	if err = m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("run migrations: %w", err)
	}
	slog.Info("migrations applied")

	// Repositories.
	taskRepo := repository.NewSQLiteTaskRepository(db)
	eventRepo := repository.NewSQLiteTaskEventRepository(db)
	appStateRepo := repository.NewSQLiteAppStateRepository(db)

	// Active windows.
	windows, err := cfg.ActiveWindows()
	if err != nil {
		return fmt.Errorf("parse windows: %w", err)
	}

	// Mark orphan running tasks on startup.
	markOrphans(context.Background(), taskRepo)

	// Slack client.
	slackClient := islack.NewClient(env.SlackBotToken, cfg.Slack.ChannelID)

	// GitHub client + poller.
	ghClient := igithub.NewClient(env.GitHubToken)
	poller := igithub.NewPoller(ghClient.Issues, taskRepo, cfg.GitHub.Repos)

	// Shared real clock (injected into runner and task usecase for testability).
	sharedClock := scheduler.RealClock{} // ADDED

	// Claude runner.
	claudeRunner := claude.NewRunnerWithExecAndClock(exec.CommandContext, sharedClock) // MODIFIED: inject clock

	// PR creator.
	ghRunner := &igithub.RealGhRunner{}
	prCreator := igithub.NewPRCreator(ghRunner, cfg.GitHub.Repos)

	// Use cases.
	modeUC := usecase.NewModeUseCase(appStateRepo)

	// Resolve task-budget limits (config defaults + derived daily/weekly).
	dailyMax, weeklyMax, weekStart, resetTZ, err := cfg.Limits.ResolvedLimits()
	if err != nil {
		return fmt.Errorf("resolve limits: %w", err)
	}
	budgetUC := usecase.NewBudgetUseCase(appStateRepo, scheduler.BudgetLimits{
		DailyMax:     dailyMax,
		WeeklyMax:    weeklyMax,
		WeekStartsOn: weekStart,
		ResetTZ:      resetTZ,
	})

	// Metrics (Prometheus) — collector reads live from BudgetUseCase + windows.
	metricsRecorder := metrics.New(metrics.Options{
		Budget:   budgetUC,
		Window:   &metricsWindowAdapter{windows: windows},
		FullMode: &metricsFullModeAdapter{repo: appStateRepo},
		Clock:    metricsClockAdapter{clock: sharedClock},
	})

	// Quality gate (per-repo lint/test/build before PR).
	qgAdapter := qualitygate.NewAdapter(cfg.GitHub.Repos)

	// Worker.
	workerCfg := scheduler.WorkerConfig{
		TaskRepo:     taskRepo,
		EventRepo:    eventRepo,
		AppStateRepo: appStateRepo,
		Runner:       claudeRunner,
		Canceller:    claude.NewProcessCanceller(),
		Slack:        &schedulerSlackAdapter{client: slackClient},
		PRCreator:    &schedulerPRAdapter{inner: prCreator},
		Budget:       budgetUC,
		Metrics:      &schedulerMetricsAdapter{inner: metricsRecorder},
		QualityGate:  qgAdapter,
		Windows:      windows,
		WorktreeRoot: cfg.Runtime.WorktreeRoot,
		PromptsDir:   cfg.Runtime.PromptsDir,
		LogDir:       "data/logs",
	}
	worker := scheduler.NewWorker(workerCfg)

	// Scheduler.
	sched := scheduler.New(scheduler.Config{
		Windows:      windows,
		TaskRepo:     taskRepo,
		AppStateRepo: appStateRepo,
		Poller:       poller,
		Worker:       worker,
		BudgetGate:   budgetUC,
		TickInterval: cfg.Runtime.TickInterval,
	})

	// WindowGate adapter for TaskUseCase (C1 fix: inject window gate so EnqueueFromIssue checks correctly).
	ucWindowGate := &windowsGateAdapter{windows: windows} // ADDED
	taskUC := usecase.NewTaskUseCase(                     // MODIFIED
		taskRepo, eventRepo, appStateRepo, sched, cfg.GitHub.Repos, // MODIFIED
		usecase.WithWindowGate(ucWindowGate), // ADDED
		usecase.WithClock(sharedClock),       // ADDED
	) // MODIFIED

	// Maintenance use case + scheduler.
	maintenanceUC := usecase.NewMaintenanceUseCase(taskRepo, appStateRepo, budgetUC, scheduler.BudgetLimits{
		DailyMax:     dailyMax,
		WeeklyMax:    weeklyMax,
		WeekStartsOn: weekStart,
		ResetTZ:      resetTZ,
	})
	maintenanceSched := scheduler.NewMaintenanceScheduler(scheduler.MaintenanceSchedulerConfig{
		Tasks:    cfg.Scheduler.MaintenanceTasks,
		Windows:  windows,
		Enqueuer: maintenanceUC,
	})

	// HTTP server.
	healthH := api.NewHealthHandler(modeUC)
	taskH := api.NewTaskHandler(taskUC)
	modeH := api.NewModeHandler(modeUC)
	limitsH := api.NewLimitsHandler(budgetUC)
	slackH := api.NewSlackHandler(env.SlackSigningSecret, sched)
	router := api.NewRouter(healthH, taskH, modeH, limitsH, slackH)
	metrics.NewHandler(metricsRecorder).Register(router)

	srv := &http.Server{
		Addr:    cfg.Runtime.HTTPBindAddr,
		Handler: router,
	}

	// Graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	schedCtx, schedCancel := context.WithCancel(context.Background())
	go sched.Start(schedCtx)
	go maintenanceSched.Start(schedCtx)

	go func() {
		slog.Info("HTTP server listening", "addr", cfg.Runtime.HTTPBindAddr)
		if listenErr := srv.ListenAndServe(); !errors.Is(listenErr, http.ErrServerClosed) {
			slog.Error("HTTP server error", "err", listenErr)
		}
	}()

	<-sigCh
	slog.Info("shutdown signal received")

	schedCancel()
	sched.Stop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if shutdownErr := srv.Shutdown(shutdownCtx); shutdownErr != nil {
		slog.Error("HTTP server shutdown", "err", shutdownErr)
	}

	return nil
}

func checkCLI(name string) {
	if _, err := exec.LookPath(name); err != nil {
		slog.Warn("CLI tool not found in PATH", "tool", name)
	}
}

func markOrphans(ctx context.Context, repo *repository.SQLiteTaskRepository) {
	running, err := repo.GetRunning(ctx)
	if err != nil {
		slog.Error("mark orphans: list running", "err", err)
		return
	}
	for _, t := range running {
		t.Status = domain.TaskStatusFailed
		t.StderrTail = "service restarted while task was running (orphaned)"
		if updateErr := repo.Update(ctx, t); updateErr != nil {
			slog.Error("mark orphan", "task_id", t.ID, "err", updateErr)
		} else {
			slog.Warn("orphaned task marked failed", "task_id", t.ID)
		}
	}
}

// schedulerSlackAdapter adapts islack.Client to scheduler.SlackNotifier.
type schedulerSlackAdapter struct {
	client *islack.Client
}

func (s *schedulerSlackAdapter) NotifyStarted(ctx context.Context, task *domain.Task) error {
	return s.client.NotifyStarted(ctx, task)
}

func (s *schedulerSlackAdapter) NotifyDone(ctx context.Context, task *domain.Task) error {
	return s.client.NotifyDone(ctx, task)
}

func (s *schedulerSlackAdapter) NotifyFailed(ctx context.Context, task *domain.Task, errMsg string) error {
	return s.client.NotifyFailed(ctx, task, errMsg)
}

func (s *schedulerSlackAdapter) NotifyCancelled(ctx context.Context, task *domain.Task) error {
	return s.client.NotifyCancelled(ctx, task)
}

// schedulerPRAdapter adapts igithub.PRCreator to scheduler.PRCreator.
type schedulerPRAdapter struct {
	inner *igithub.PRCreator
}

func (p *schedulerPRAdapter) CreatePR(ctx context.Context, task *domain.Task) (string, int, error) {
	return p.inner.CreatePR(ctx, task)
}

// windowsGateAdapter adapts []*domain.ActiveWindow to usecase.WindowGate. // ADDED
type windowsGateAdapter struct { // ADDED
	windows []*domain.ActiveWindow // ADDED
} // ADDED

// AllowNow reports whether now is inside any configured active window. // ADDED
// fullMode=true is handled by the caller (TaskUseCase.EnqueueFromIssue). // ADDED
func (g *windowsGateAdapter) AllowNow(now time.Time, _ bool) bool { // ADDED
	for _, w := range g.windows { // ADDED
		if w.Contains(now) { // ADDED
			return true // ADDED
		} // ADDED
	} // ADDED
	return false // ADDED
} // ADDED

// schedulerMetricsAdapter forwards worker lifecycle signals to the metrics
// package. Lives here (not in scheduler/) because scheduler must not import
// a concrete metrics implementation — only the interface.
type schedulerMetricsAdapter struct {
	inner *metrics.Metrics
}

func (a *schedulerMetricsAdapter) RecordTaskFinished(repo, taskType, status string, startedAt, finishedAt time.Time) {
	a.inner.RecordTaskFinished(repo, taskType, status, startedAt, finishedAt)
}

func (a *schedulerMetricsAdapter) RecordBudgetBlock(reason scheduler.BudgetReason) {
	a.inner.RecordBudgetBlock(reason)
}

func (a *schedulerMetricsAdapter) RecordWindowClose() {
	a.inner.RecordWindowClose()
}

// metricsWindowAdapter exposes the current set of active windows to the
// metrics collector so scrape-time can compute the "window open" gauge. Full
// mode is handled by a sibling adapter — both together decide dispatchability.
type metricsWindowAdapter struct {
	windows []*domain.ActiveWindow
}

func (a *metricsWindowAdapter) IsOpen(t time.Time, fullMode bool) bool {
	if fullMode {
		return true
	}
	for _, w := range a.windows {
		if w.Contains(t) {
			return true
		}
	}
	return false
}

// metricsFullModeAdapter bridges the AppState-backed full_mode toggle to the
// metrics package without exposing the repository directly.
type metricsFullModeAdapter struct {
	repo domain.AppStateRepository
}

func (a *metricsFullModeAdapter) IsFullMode(ctx context.Context) bool {
	if a.repo == nil {
		return false
	}
	state, err := a.repo.Get(ctx, "full_mode")
	if err != nil || state == nil {
		return false
	}
	v := state.ValueJSON
	return v == "true" || v == "1" || v == `{"enabled":true}`
}

// metricsClockAdapter lets the metrics package observe the same clock the
// scheduler uses — important for tests that fake time, and for production
// consistency across the "would the scheduler dispatch now?" question.
type metricsClockAdapter struct {
	clock scheduler.Clock
}

func (a metricsClockAdapter) Now() time.Time { return a.clock.Now() }
