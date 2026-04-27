# parallel-tasks — Go 구현 프롬프트

> 이 파일은 `/planner` 가 생성한 **단일 스택(Go) 구현 지시서** 입니다.
> 대응 PRD: [`../parallel-tasks.md`](../parallel-tasks.md)
> 스택: **Go (Gin + GORM + sqlc + golangci-lint + swaggo)**
> 모 PRD: [`../scheduled-dev-agent.md`](../scheduled-dev-agent.md) §3 비목표 / §10 R3 / §12 OI-2 해소

---

## 맥락 (꼭 읽을 것)

- **PRD 본문**: `docs/specs/parallel-tasks.md`
- **모 PRD**: `docs/specs/scheduled-dev-agent.md` (전체 시스템 흐름)
- **스택 규칙 (최우선)**: 프로젝트 루트 `CLAUDE.md` — Go Gin 기반, MUST/NEVER, sqlc, golangci-lint, swag, 커버리지 80%
- **관련 패턴 스킬**: `.claude/skills/go-patterns.md`, `.claude/skills/api-design-patterns.md` (있으면)
- **외부 도구**: `claude` CLI (병렬 실행 충돌 spike 대상), `gh` CLI, `git`

**스택의 절대 규칙 (CLAUDE.md 요약 — 충돌 시 CLAUDE.md 우선)**
- `domain/` 패키지는 외부 패키지 import 금지 (GORM, gin 불가)
- 레이어 방향: `handler → usecase → domain ← repository`
- 복잡 쿼리 / 조건 검색은 **sqlc 필수**. 단순 CRUD 만 GORM.
- 모든 Handler 에 swag 주석 필수. Request/Response DTO 에 `example:"..."` 태그 필수.
- `context.Context` 전 레이어 전파. 요청 핸들러는 `c.Request.Context()` 사용.
- 전역 DB 변수 금지. DI 는 생성자 파라미터로만.
- 기존 migration 수정 금지 — 새 파일만.
- 토큰 / PAT / Slack secret / PII 로그 금지.
- 복구 가능한 에러에 `panic()` 금지 (단, 본 PRD 에서는 worker goroutine 의 top-level `recover()` 가 격리 보장 목적이라 허용).

---

## 이 프로젝트의 책임 범위 (전체)

단일 스택이므로 PRD 전체를 이 프롬프트가 커버합니다.

- **포함**:
  - `internal/scheduler` 의 dispatch 로직을 N 슬롯으로 확장 (sem 크기 변경 + repo lock)
  - per-repo in-memory mutex (`RepoLocks`) 신규 패키지
  - sqlc 쿼리 2개 추가 (`PickNextDispatchable`, `ClaimTaskAtomic`) + Repository 메서드
  - `ConcurrencyUseCase` (AppState 기반 runtime override + clamp 정책)
  - `/modes/concurrency` GET/PATCH 핸들러 + DTO + swag 주석
  - `/healthz` 응답 확장 (`busy_slots`, `max_parallel_tasks`)
  - config 확장: `concurrency.max_parallel_tasks` (기본 1, 최대 3 clamp)
  - 단위 / e2e 테스트 (race 시뮬레이션, repo lock 격리, 격리 검증)
  - kill switch 환경변수 (`PARALLEL_TASKS_DISABLED=1`)

- **제외 (PRD §3 비목표)**:
  - 같은 repo 내 task 동시 처리
  - 분산 / 멀티 머신
  - hot-reload (config 변경 즉시 반영)
  - claude CLI HOME 격리 자동화 — spike 결과에 따라 별도 PRD 로 분기
  - metric 노출 (Prometheus/OTel)

---

## 변경할/생성할 파일 (체크리스트)

> 모든 경로는 프로젝트 루트 기준. 단계별로 PRD §6.5 의 phase 0 (max=1, v1 동작 동일) → phase 1+ (병렬) 가 한 release 안에 들어갑니다.

### 1. Config

- [ ] `internal/config/config.go` — `ConcurrencyConfig{ MaxParallelTasks int }` 추가, `Config.Concurrency` 필드, viper default = `1`
- [ ] `internal/config/validate.go` — `MaxParallelTasks < 0` reject, `> 3` 일 때 `3` 으로 clamp + `slog.Warn`
- [ ] `internal/config/config_test.go` — 음수 / 0 / 1 / 2 / 3 / 4 (clamp 검증) / 누락 (default=1)
- [ ] `config.example.yaml` 에 `concurrency.max_parallel_tasks: 1` 섹션 추가 + 주석 (권장 최대 3)

### 2. Domain

- [ ] `internal/domain/repository.go` — `TaskRepository` 인터페이스에 다음 추가:
  ```go
  // PickNextDispatchable returns the oldest queued task whose repo is NOT in
  // excludedRepos. Returns ErrNotFound when no candidate exists.
  PickNextDispatchable(ctx context.Context, excludedRepos []string) (*Task, error)

  // ClaimTask atomically transitions a task from 'queued' to 'running'.
  // Returns true iff RowsAffected == 1 (this caller won the race).
  ClaimTask(ctx context.Context, id string) (bool, error)
  ```
- [ ] `domain/` 외부 import 금지 규칙 준수 — 위 메서드 시그니처는 표준 `context` 외 의존 없음

### 3. Migrations

본 PRD 는 **신규 컬럼 추가가 없음**. AppState 의 새 키 (`concurrency_override`) 만 사용.

- [ ] (스킵) 마이그레이션 파일 생성하지 않음 — 기존 `app_states` 테이블에 key 만 INSERT
- [ ] (대안) 만약 기존 task 가 `status='running'` 상태로 orphan 되어 있을 때 starvation 가능성을 막기 위해 startup 시 한 번만 `UPDATE tasks SET status='failed' WHERE status='running' AND started_at < (now - 1h)` 보정 — 이미 모 PRD §6.2 의 orphan 처리에 포함됨 → 추가 작업 없음

### 4. Repository (sqlc + GORM)

- [ ] `db/query/task.sql` — 다음 두 쿼리 추가 (raw SQL 금지 규칙 준수, sqlc generate 필수):
  ```sql
  -- name: PickNextDispatchable :one
  SELECT id, repo_full_name, issue_number, issue_title, task_type, status,
         prompt_template, worktree_path, pr_url, pr_number,
         started_at, finished_at,
         estimated_input_tokens, estimated_output_tokens,
         exit_code, stderr_tail, created_at, updated_at
  FROM tasks
  WHERE status = 'queued'
    AND repo_full_name NOT IN (sqlc.slice('excluded_repos'))
  ORDER BY created_at ASC
  LIMIT 1;

  -- name: ClaimTask :execrows
  UPDATE tasks
  SET status = 'running', updated_at = datetime('now')
  WHERE id = sqlc.arg('id') AND status = 'queued';
  ```
  - **주의**: sqlc 의 `sqlc.slice` 는 SQLite 에서 빈 배열 처리 시 placeholder 가 0개라 syntax error 가능 — caller 가 빈 slice 를 전달할 경우 `repo_full_name NOT IN ('')` 가 되도록 dummy `""` 를 항상 1개 prepend 하거나, 두 가지 쿼리 (`PickNextDispatchableAll` / `PickNextDispatchableExcluding`) 로 분기
- [ ] `sqlc.yaml` 변경 없음 (이미 `db/query/`, `db/sqlc/` 설정)
- [ ] **`sqlc generate` 필수 실행** — `db/sqlc/` 코드 자동 생성. 수동 수정 금지
- [ ] `internal/repository/task_repository.go`
  - `PickNextDispatchable(ctx, excludedRepos []string) (*domain.Task, error)` 구현
    - sqlc 호출 → `gormTask` 동등 변환 → `toDomainTask`
    - 결과 없음 → `domain.ErrNotFound`
  - `ClaimTask(ctx, id string) (bool, error)` 구현
    - sqlc `:execrows` 결과 → `rows == 1`
- [ ] `internal/repository/sqlite_test.go` 또는 신규 `task_repository_dispatch_test.go`
  - 두 goroutine 이 같은 `id` 로 동시 `ClaimTask` → 정확히 1개만 `true` (race-free 검증)
  - `PickNextDispatchable(ctx, []string{"a/b"})` → repo 'a/b' task 가 있으면 그 다음 task 반환
  - 빈 slice 전달 시 정상 동작 (sqlc.slice 빈 처리 검증)

### 5. RepoLocks (신규 패키지)

- [ ] `internal/scheduler/repolocks.go`
  ```go
  // RepoLocks tracks per-repo mutexes for serialising same-repo task dispatch.
  // The zero value is ready to use.
  type RepoLocks struct {
      m sync.Map  // string -> *sync.Mutex
  }

  // TryLock attempts to acquire the lock for repo without blocking.
  // Returns release fn (always non-nil) and ok=false if the lock is held.
  func (r *RepoLocks) TryLock(repo string) (release func(), ok bool) { ... }

  // HeldRepos returns a snapshot of repos currently locked. For observability
  // (e.g. /modes/concurrency response). Order is not guaranteed.
  func (r *RepoLocks) HeldRepos() []string { ... }
  ```
  - 구현: `sync.Map.LoadOrStore` 로 mutex 포인터 확보 → `mu.TryLock()` (Go 1.18+) → release 는 `mu.Unlock()`
  - `HeldRepos` 는 `sync.Map.Range` 로 `mu.TryLock` 시도해 잡혀 있던 것만 수집 후 즉시 해제 (관측용 — 정확성보다 가시성 우선)
- [ ] `internal/scheduler/repolocks_test.go`
  - 같은 repo 두 번 `TryLock` → 두 번째 `ok=false`
  - release 후 다시 `TryLock` → 성공
  - 다른 repo 두 개 → 둘 다 성공 (격리)
  - 100 goroutine 동시 `TryLock` 같은 repo → 정확히 1개 성공

### 6. Scheduler 확장

- [ ] `internal/scheduler/scheduler.go`
  - `Config` 에 `MaxParallelTasks int` 추가
  - `Scheduler.sem` 크기를 `MaxParallelTasks` 로 (clamp: `0/1 → 1`, `>3 → 3`)
  - `Scheduler.repoLocks *RepoLocks` 필드 추가
  - `Scheduler.concurrencyOverride func() int` 필드 (런타임 override 조회 — `ConcurrencyUseCase.EffectiveMax` 주입)
  - `tick()` 에서 dispatch 루프 N 회:
    ```
    for slot := 0; slot < effectiveMax; slot++ {
        if !s.tryAcquireSlot() { break }
        if !s.dispatch(ctx) { s.releaseSlot(); break }
    }
    ```
  - `dispatch(ctx)` 변경:
    - `repoLocks.HeldRepos()` 호출해 `excludedRepos` 산출
    - `taskRepo.PickNextDispatchable(ctx, excludedRepos)` 호출
    - `repoLocks.TryLock(task.RepoFullName)` 시도. 실패하면 false return (다음 slot 도 시도 가능)
    - `taskRepo.ClaimTask(ctx, task.ID)` 호출. `false` (이미 다른 슬롯이 가져갔거나 status 가 queued 가 아님) 면 release lock + false return
    - 성공 시 `cancelMap[task.ID] = cancel`, `wg.Add(1)` 후 worker goroutine spawn
    - worker goroutine 의 `defer` 체인에서 sem 반납 + repo lock release + cancelMap 삭제
  - `cancelMap` 보호: 기존 `sync.Mutex` 그대로 유지 — N 슬롯에서도 안전
  - **`recover()` 추가**: worker goroutine 의 top-level 에서 panic 격리:
    ```go
    defer func() {
        if r := recover(); r != nil {
            slog.Error("scheduler: worker panic", "task_id", task.ID, "panic", r, "stack", string(debug.Stack()))
        }
    }()
    ```
- [ ] **kill switch**: `os.Getenv("PARALLEL_TASKS_DISABLED") == "1"` 이면 `effectiveMax = 1` 강제 (config / override 무시)
- [ ] `internal/scheduler/scheduler_test.go`
  - max=1 일 때 sem 크기 1, dispatch 루프 1회 (v1 회귀)
  - max=2 일 때 두 다른 repo task → 동시 dispatch 검증 (두 worker 의 RunTask 가 동시에 호출됨 — `time.Now()` 차이 < 200ms)
  - max=2 일 때 같은 repo task 두 개 → 한 번에 한 개만 dispatch
  - max=3 일 때 worker panic → 다른 슬롯 영향 없음 (recover 검증)
  - max=4 입력 → 3 으로 clamp + warn 로그
  - `PARALLEL_TASKS_DISABLED=1` → max=1 강제

### 7. Worker (변경 최소)

- [ ] `internal/scheduler/worker.go` — **변경 없음** 이 원칙. 단:
  - `RunTask` 내부에서 budget gate 의 `CheckAndIncrementReason` 가 race-free 인지 재확인 (이미 mutex 보호)
  - `provisionWorktree` 의 worktree 경로가 task ID 기반 (`.worktrees/task-{id}`) 이라 N 슬롯에서도 충돌 없음 (검증만)
- [ ] worker_test.go 에 e2e 시나리오 추가:
  - 두 task 가 RunTask 를 동시에 실행해도 Slack notify, TaskEvent 기록, BudgetUseCase increment 가 모두 race-free

### 8. ConcurrencyUseCase (신규)

- [ ] `internal/usecase/concurrency_usecase.go`
  ```go
  type ConcurrencyUseCase struct {
      appStateRepo domain.AppStateRepository
      configMax    int
      mu           sync.Mutex
  }

  // EffectiveMax returns the runtime cap, applying override > config > default
  // and the [0..3] clamp. Reads from AppState every call (cheap — single SQL).
  func (uc *ConcurrencyUseCase) EffectiveMax(ctx context.Context) int { ... }

  // SetOverride persists a runtime override. max=0 falls back to config.
  func (uc *ConcurrencyUseCase) SetOverride(ctx context.Context, max int) (int, error) { ... }

  // Snapshot returns config + override + effective for /modes/concurrency.
  func (uc *ConcurrencyUseCase) Snapshot(ctx context.Context) ConcurrencySnapshot { ... }
  ```
- [ ] AppState key: `"concurrency_override"`, value JSON: `{"max": int, "updated_at": unix}`
- [ ] clamp 정책 한 곳에 — `clamp(n int) int { if n < 0 { return 0 }; if n > 3 { return 3 }; return n }`
- [ ] `internal/usecase/concurrency_usecase_test.go`
  - override 없으면 config 값 반환
  - override=2, config=1 → 2
  - override=4 → 3 (clamp + warn 로그 검증)
  - override=0 → config 폴백
  - `PARALLEL_TASKS_DISABLED=1` 우선 (선택 — env 검사를 UseCase 가 아닌 Scheduler 단에서만 한다면 스킵)

### 9. API Handler

- [ ] `internal/api/dto.go` 추가:
  ```go
  // ConcurrencyResponse is the response for GET /modes/concurrency.
  type ConcurrencyResponse struct {
      Max          int      `json:"max" example:"2"`
      ConfigMax    int      `json:"config_max" example:"1"`
      OverrideMax  int      `json:"override_max" example:"2"`
      BusySlots    int      `json:"busy_slots" example:"1"`
      RunningRepos []string `json:"running_repos" example:"owner/repo"`
  }

  // ConcurrencyPatchRequest is the body for PATCH /modes/concurrency.
  // max=0 falls back to the config value. Values >3 are clamped to 3.
  type ConcurrencyPatchRequest struct {
      Max int `json:"max" example:"2"`
  }
  ```
- [ ] `internal/api/dto.go` `HealthResponse` 확장:
  ```go
  BusySlots         int `json:"busy_slots" example:"1"`
  MaxParallelTasks  int `json:"max_parallel_tasks" example:"2"`
  ```
- [ ] `internal/api/handlers_concurrency.go` (신규)
  - `GET /modes/concurrency` — swag `@Tags modes`, `@Router /modes/concurrency [get]`, `@Success 200 {object} ConcurrencyResponse`
  - `PATCH /modes/concurrency` — `@Failure 400 {object} ErrorResponse`, body validation (음수 reject)
- [ ] `internal/api/server.go` (또는 router 등록 위치) — 위 두 라우트 추가
- [ ] healthz 핸들러에 `busy_slots`, `max_parallel_tasks` 채우는 의존성 주입 (`Scheduler.Stats() (busy, max int)` 같은 메서드 추가 검토)
- [ ] `internal/api/handler_test.go` 에 두 엔드포인트 테스트 (성공 / clamp / override=0 폴백 / 음수 reject)

### 10. main.go DI

- [ ] `cmd/scheduled-dev-agent/main.go`
  - `cfg.Concurrency.MaxParallelTasks` 읽어 `scheduler.Config.MaxParallelTasks` 로 전달
  - `ConcurrencyUseCase` 생성 → Scheduler 의 `concurrencyOverride` 함수 어댑터로 주입
  - kill switch 환경변수 검사를 main 또는 Scheduler 생성자 한 곳에서
  - swag 주석 갱신 — 새 endpoint 가 자동 픽업되는지 `swag init -g cmd/scheduled-dev-agent/main.go -o docs` 실행

### 11. Tests

#### 단위 테스트 (커버리지 ≥ 85% — scheduler 패키지)

- [ ] `RepoLocks` — 100 goroutine race
- [ ] `ClaimTask` — 두 동시 호출 → 1개만 true (sqlite_test.go 에 추가)
- [ ] `PickNextDispatchable` — excluded_repos 빈 / 1개 / 3개 케이스
- [ ] `ConcurrencyUseCase.EffectiveMax` — 모든 분기
- [ ] config validate — `MaxParallelTasks` 음수 / 0 / 1 / 4 / clamp

#### 통합 / e2e

- [ ] `internal/scheduler/scheduler_test.go` (또는 별도 `parallel_e2e_test.go`)
  - **시나리오 1 — 다른 repo 동시 dispatch**: max=2, queue=[task-A(repoX), task-B(repoY)] → 200ms 안에 두 worker 모두 RunTask 진입
  - **시나리오 2 — 같은 repo 직렬화**: max=2, queue=[task-A(repoX), task-B(repoX), task-C(repoY)] → A + C 동시, B 는 A 완료 후
  - **시나리오 3 — 격리 (panic)**: max=2 worker 1 패닉 → worker 2 정상 완료 + scheduler 살아있음
  - **시나리오 4 — budget race**: max=2, daily_cap=1 (마지막 1슬롯) → 두 worker 가 동시 CheckAndIncrement → 1개 성공, 1개 cancel
  - **시나리오 5 — graceful shutdown**: max=3, 3 worker 진행 중 Stop() → 모두 SIGTERM, 30s 안에 wg.Wait 완료
  - **시나리오 6 — kill switch**: `PARALLEL_TASKS_DISABLED=1` 환경에서 max=3 config → 실제 sem 크기 1
- [ ] `mocks/` — `mockery --name=TaskRepository --dir=internal/domain --output=mocks` 재생성

#### 회귀 (v1 동작 동일)

- [ ] max=1 일 때 기존 e2e suite 100% 통과 (PRD G6)

### 12. Swag / Lint / Coverage

- [ ] `swag init -g cmd/scheduled-dev-agent/main.go -o docs` 재실행 — `/modes/concurrency` 가 Swagger UI 에 노출되는지 확인
- [ ] `golangci-lint run ./...` 통과 (모든 신규 파일에 `// Package ...` doc comment, exported 식별자 doc comment 필수)
- [ ] `go test ./... -coverprofile=cover.out` → 전체 80% 이상, `internal/scheduler` ≥ 85%
- [ ] `.claude/hooks/pre-push.sh` 통과 (커버리지 + lint 게이트)

### 13. Docs / Config / README

- [ ] `config.example.yaml`
  ```yaml
  concurrency:
    # 동시 실행 슬롯 수. 기본 1 (직렬 — v1.0 동작과 동일).
    # 권장 최대 3. 4 이상은 자동으로 3 으로 clamp 됩니다.
    # PRD docs/specs/parallel-tasks.md §6.5 의 단계적 ramp-up 절차 참조.
    max_parallel_tasks: 1
  ```
- [ ] `README.md` 또는 `docs/operations.md` — kill switch (`PARALLEL_TASKS_DISABLED=1`), 단계적 ramp-up 절차, 롤백 절차 (PRD §6.6) 명시
- [ ] CHANGELOG / release notes (이 repo 가 `release-please` 사용 — `feat(scheduler): support N parallel tasks` 형태 commit)

---

## 구현 제약 (스택 CLAUDE.md 준수)

이 역할의 `CLAUDE.md` 규칙이 본 프롬프트보다 우선합니다. 다음 항목을 특히 주의:

- [ ] **DI**: 생성자 파라미터 주입만. `ConcurrencyUseCase` 와 `Scheduler` 모두 `New(...)` 에서 의존성 받음. 전역 변수 금지
- [ ] **에러**: `fmt.Errorf("...: %w", err)` 로 wrap. `gorm.ErrRecordNotFound` → `domain.ErrNotFound` 변환은 기존 패턴 그대로
- [ ] **context**: `PickNextDispatchable`, `ClaimTask` 모두 `ctx` 첫 인자. handler 는 `c.Request.Context()` 사용
- [ ] **Handler 응답**: domain error → HTTP status (404 / 400 / 500). 내부 에러 메시지 노출 금지
- [ ] **sqlc**: 신규 쿼리 두 개는 sqlc 로 작성. raw SQL 문자열 (GORM `Raw`) 금지
- [ ] **swag**: 새 endpoint 모두 `@Summary @Tags @Router @Success @Failure` 작성. DTO 에 `example:"..."` 태그
- [ ] **domain/ 외부 import 금지**: `TaskRepository` 인터페이스에 `context` 외 의존 없음. mutex / sync.Map 같은 구현 디테일은 repository 레이어
- [ ] **panic 금지** — 단, worker goroutine top-level `defer recover()` 는 격리 보장 목적이라 허용. 그 외 호출 경로에서는 절대 panic 사용 X
- [ ] **기존 migration 수정 금지** — 본 PRD 는 마이그레이션 추가 없음 (AppState key 만 사용)
- [ ] **`db/sqlc/` 수동 수정 금지** — `sqlc generate` 로 재생성

---

## 다른 시스템과의 계약 (Interface)

- **Scheduler ↔ ConcurrencyUseCase**:
  - Scheduler 는 매 tick 마다 `concurrencyOverride()` 호출 → 효과적 max 반환
  - 변경 시 Scheduler 는 다음 tick 부터 새 값 사용 (현재 진행 중 슬롯은 그대로 drain)
- **Scheduler ↔ TaskRepository**:
  - `PickNextDispatchable(ctx, excludedRepos)` → 후보 1개 또는 `ErrNotFound`
  - `ClaimTask(ctx, id)` → atomic queued→running 전이. `false` 는 race 패배
- **API ↔ ConcurrencyUseCase**:
  - GET `/modes/concurrency` → `Snapshot()` 호출
  - PATCH `/modes/concurrency` → `SetOverride(ctx, max)` 호출. Scheduler 는 polling 방식이라 즉시 반영

계약 변경 시 PRD §8 의 "API 계약" 섹션을 먼저 갱신.

---

## 실행 지시

이 프롬프트를 받은 agent 는 아래 순서로 진행:

1. 프로젝트 루트 `CLAUDE.md` 와 `docs/specs/parallel-tasks.md`, `docs/specs/scheduled-dev-agent.md` 를 먼저 읽기
2. 기존 코드 탐색 — `internal/scheduler/scheduler.go`, `internal/scheduler/worker.go`, `internal/repository/task_repository.go`, `internal/usecase/budget_usecase.go`, `internal/api/dto.go` 패턴 파악
3. 위 체크리스트의 **필요한 항목만** 생성. 마이그레이션은 추가하지 않음
4. **파일 생성 순서** (의존성 그래프 따라):
   1. `db/query/task.sql` 쿼리 추가 → `sqlc generate`
   2. `internal/domain/repository.go` 인터페이스 확장
   3. `internal/repository/task_repository.go` 구현
   4. `internal/scheduler/repolocks.go` 신규
   5. `internal/usecase/concurrency_usecase.go` 신규
   6. `internal/scheduler/scheduler.go` 확장
   7. `internal/api/dto.go` 확장 + `handlers_concurrency.go` 신규 + `server.go` 라우트 등록
   8. `internal/config/config.go`, `validate.go`, `config.example.yaml`
   9. `cmd/scheduled-dev-agent/main.go` DI 조립
   10. 모든 단위 / e2e 테스트
   11. `swag init`, `golangci-lint run`, 커버리지 측정
5. 생성 완료 후 요약 리포트:
   - 생성된 파일 목록
   - 변경된 기존 파일 목록
   - 추가된 sqlc 쿼리 (재생성된 `db/sqlc/` 파일 목록 포함)
   - swag init 결과 (Swagger UI 의 새 endpoint 확인)
   - 커버리지 % (전체 + scheduler 패키지)
   - **단계적 ramp-up 안내**: 운영자가 첫 release 후 `max_parallel_tasks=1` 로 1주 운영 → phase 1 spike 전환 절차 (PRD §6.5)

---

## 성공 기준

- [ ] 위 체크리스트 모든 항목 체크됨 (마이그레이션 항목은 명시적 스킵 OK)
- [ ] `go test ./...` 전체 통과
- [ ] 커버리지: 전체 ≥ 80%, `internal/scheduler` ≥ 85%
- [ ] `golangci-lint run ./...` 통과
- [ ] `swag init` 성공, Swagger UI 에 `/modes/concurrency` GET/PATCH + 확장된 `/healthz` 노출
- [ ] **회귀 검증**: `max_parallel_tasks=1` 로 설정 시 기존 e2e suite 100% 통과 (v1.0 동작 바이트 동일)
- [ ] **race 검증**: e2e 시나리오 1~6 통과
- [ ] CLAUDE.md 의 MUST / NEVER 위반 0건
- [ ] kill switch (`PARALLEL_TASKS_DISABLED=1`) 동작 확인
- [ ] PRD §6.5 의 phase 0 acceptance 통과 (max=1 1주 운영 가능 상태)

---

> 이 프롬프트는 `/planner` 가 자동 생성했습니다. PRD 변경이 발생하면 이 파일도 함께 갱신하세요.
