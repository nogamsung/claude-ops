// Package main is the entrypoint for the scheduled-dev-agent binary.
//
//	@title          scheduled-dev-agent API
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

	"github.com/gs97ahn/scheduled-dev-agent/internal/api"
	"github.com/gs97ahn/scheduled-dev-agent/internal/claude"
	"github.com/gs97ahn/scheduled-dev-agent/internal/config"
	"github.com/gs97ahn/scheduled-dev-agent/internal/domain"
	igithub "github.com/gs97ahn/scheduled-dev-agent/internal/github"
	"github.com/gs97ahn/scheduled-dev-agent/internal/repository"
	"github.com/gs97ahn/scheduled-dev-agent/internal/scheduler"
	islack "github.com/gs97ahn/scheduled-dev-agent/internal/slack"
	"github.com/gs97ahn/scheduled-dev-agent/internal/usecase"

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

	// Worker.
	workerCfg := scheduler.WorkerConfig{
		TaskRepo:     taskRepo,
		EventRepo:    eventRepo,
		AppStateRepo: appStateRepo,
		Runner:       claudeRunner,
		Canceller:    claude.NewProcessCanceller(),
		Slack:        &schedulerSlackAdapter{client: slackClient},
		PRCreator:    &schedulerPRAdapter{inner: prCreator},
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
		TickInterval: cfg.Runtime.TickInterval,
	})

	// WindowGate adapter for TaskUseCase (C1 fix: inject window gate so EnqueueFromIssue checks correctly).
	ucWindowGate := &windowsGateAdapter{windows: windows} // ADDED
	taskUC := usecase.NewTaskUseCase(                     // MODIFIED
		taskRepo, eventRepo, appStateRepo, sched, cfg.GitHub.Repos, // MODIFIED
		usecase.WithWindowGate(ucWindowGate), // ADDED
		usecase.WithClock(sharedClock),       // ADDED
	) // MODIFIED

	// HTTP server.
	healthH := api.NewHealthHandler(modeUC)
	taskH := api.NewTaskHandler(taskUC)
	modeH := api.NewModeHandler(modeUC)
	slackH := api.NewSlackHandler(env.SlackSigningSecret, sched)
	router := api.NewRouter(healthH, taskH, modeH, slackH)

	srv := &http.Server{
		Addr:    cfg.Runtime.HTTPBindAddr,
		Handler: router,
	}

	// Graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	schedCtx, schedCancel := context.WithCancel(context.Background())
	go sched.Start(schedCtx)

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
