# ci-fix-loop — Go 구현 가이드

상위 PRD: [`../ci-fix-loop.md`](../ci-fix-loop.md)

> 이 문서는 PRD 의 결정·근거를 반복하지 않습니다. 단계·체크리스트·수락 기준만 담습니다. 모순 시 PRD 가 우선.

---

## 0. 사전 확인

### CLAUDE.md 핵심 규칙
- DI 생성자 주입 / 전역 db 금지
- 모든 레이어 `ctx context.Context` 첫 인자, 핸들러는 `c.Request.Context()`
- `domain/` 외부 패키지 import 금지
- 동적·집계·페이징 쿼리는 sqlc 필수 (raw SQL 하드코딩 금지) — 본 기능에서 `ListWatching`, `FindFixChildByParentAndHeadSHA`, `UpdateCIStatus` 모두 sqlc
- 모든 신규 핸들러 swag godoc 필수
- 기존 migration 수정 금지 — `000004_*` / 필요 시 `000005_*` 신규 추가만
- 테스트 없이 UseCase 메서드 추가 금지
- 로그에 secret 노출 금지 — `gh run view --log-failed` 출력에 secret 패턴 마스킹 필수
- 커버리지 ≥80% (`internal/ci` 패키지는 ≥85%)

### 영향받는 기존 파일 (수정)
- `internal/domain/task.go` — Task 구조체에 `ParentTaskID`, `FixAttemptCount`, `CIStatus`, `HeadSHA`, `CILastPolledAt` 5개 필드 + `TaskTypeCIFix` 상수
- `internal/domain/event.go` — `EventKindCICheckPolled` / `CIFailureDetected` / `CIFixEnqueued` / `CIFixExhausted` 4개 상수
- `internal/domain/repository.go` — `TaskRepository` 인터페이스에 `ListWatching(ctx) ([]*Task, error)`, `FindFixChildByParentAndHeadSHA(ctx, parentID, sha)`, `UpdateCIStatus(ctx, taskID, status, lastPolledAt, headSHA)` 추가
- `internal/repository/task_repository.go` — GORM struct (`gormTask`) 컬럼 매핑, sqlc 생성 쿼리 호출 위임
- `internal/scheduler/worker.go` — `markDone` 직후 ci-watcher hook (PR 번호 존재 시 ci_status='watching' 으로 전이, worktree 보존), worktree cleanup 분기
- `internal/scheduler/budget_gate.go` — fix task 도 daily/weekly cap 카운트 (변경 없을 가능성 — 모든 task 이미 카운트면 회귀 검증만)
- `internal/api/dto.go` — `TaskResponse` 에 `ci_status`, `parent_task_id`, `fix_attempt_count`, `head_sha`, `ci_last_polled_at` 5개 필드 추가
- `internal/api/handler.go` (또는 task_handler.go) — `GET /tasks?ci_status=` 필터 파싱
- `internal/api/server.go` — `/modes/ci-fix` 라우트 등록
- `internal/config/config.go` — `CIFix CIFixConfig` 추가 (`Enabled`, `MaxAttempts`, `PollInterval`, `PollTimeout`, `CommentOnExhaustion`)
- `internal/config/validate.go` — CIFixConfig 검증 (`MaxAttempts >= 1`, interval > 0)
- `internal/github/client.go` — `GhPRChecks(ctx, repo, prNumber)`, `GhRunLogFailed(ctx, runID)`, `GhPRView(ctx, repo, pr)`, `GhPRComment(ctx, repo, pr, body)` 메서드 추가
- `cmd/scheduled-dev-agent/main.go` — DI 조립
- `config.example.yaml` — `ci_fix:` 블록 추가

### 신규 생성 파일
- `migrations/000004_add_ci_fix_columns_to_tasks.up.sql`
- `migrations/000004_add_ci_fix_columns_to_tasks.down.sql`
- (필요 시) `migrations/000005_extend_task_type_check.up.sql` — SQLite CHECK 제약 변경 위해 표 재생성 필요한 경우 (단순 ALTER 로 안 되면 분리 마이그레이션)
- `db/query/task_ci.sql` — sqlc 쿼리 (`ListWatching`, `FindFixChildByParentAndHeadSHA`, `UpdateCIStatus`) → `db/sqlc/` 자동 생성
- `internal/ci/watcher.go` — `CIWatcher` (Tick · gh pr checks polling 오케스트레이션)
- `internal/ci/checks.go` — `gh pr checks --json` 파싱 + conclusion 평가 (`AllSuccess`, `AnyFailure`, `AnyActionRequired`)
- `internal/ci/log_extractor.go` — `gh run view --log-failed` 추출 + tail 200줄 cap
- `internal/ci/secret_mask.go` — secret 패턴 마스킹 (regex)
- `internal/ci/prompt_builder.go` — `prompts/ci-fix.tmpl` 렌더 (FailedStep, LogTail, PRNumber, PreviousAttempts)
- `internal/api/mode_ci_fix_handler.go` — `GET/PATCH /modes/ci-fix`
- `internal/usecase/ci_fix_usecase.go` — `EnqueueFixTask`, `ListByCIStatus`, `MarkCIPassed/Failed/Exhausted`
- `prompts/ci-fix.tmpl`
- 테스트: `internal/ci/*_test.go`, `internal/scheduler/worker_ci_test.go`, `internal/api/mode_ci_fix_handler_test.go`

---

## 1. 단계별 작업

### Step 1 — Migration

**파일**: `migrations/000004_add_ci_fix_columns_to_tasks.up.sql`
```sql
ALTER TABLE tasks ADD COLUMN parent_task_id    TEXT REFERENCES tasks(id) ON DELETE SET NULL;
ALTER TABLE tasks ADD COLUMN fix_attempt_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN ci_status         TEXT    NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN head_sha          TEXT    NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN ci_last_polled_at DATETIME;

CREATE INDEX idx_tasks_parent      ON tasks(parent_task_id);
CREATE INDEX idx_tasks_ci_status   ON tasks(ci_status);
CREATE UNIQUE INDEX uniq_tasks_parent_headsha
    ON tasks(parent_task_id, head_sha)
    WHERE task_type = 'ci-fix';
```

`000004_*.down.sql`:
```sql
DROP INDEX IF EXISTS uniq_tasks_parent_headsha;
DROP INDEX IF EXISTS idx_tasks_ci_status;
DROP INDEX IF EXISTS idx_tasks_parent;
ALTER TABLE tasks DROP COLUMN ci_last_polled_at;
ALTER TABLE tasks DROP COLUMN head_sha;
ALTER TABLE tasks DROP COLUMN ci_status;
ALTER TABLE tasks DROP COLUMN fix_attempt_count;
ALTER TABLE tasks DROP COLUMN parent_task_id;
```

**task_type CHECK 확장 (`'ci-fix'` 추가)**: SQLite 의 CHECK 제약은 ALTER 가 안 되므로 **테이블 재생성** 이 필요할 수 있음. 우선 기존 `000001_create_tasks_table.up.sql` 의 task_type CHECK 가 어떻게 정의되어 있는지 확인:
- enum CHECK 가 있다면 → `000005_extend_task_type_check.{up,down}.sql` 로 표 rebuild (`CREATE TABLE tasks_new` → `INSERT SELECT` → `DROP tasks` → `ALTER RENAME`). PRD §10 "기존 migration 수정 금지" 준수
- enum CHECK 가 없다면 (자유 TEXT) → 000005 불필요

**수락 기준**
- `golang-migrate` 로 up/down 정상 적용 (테스트: `make migrate-test` 또는 sqlite 임시 파일)
- 기존 task row 들의 `ci_status=''` 로 초기화되어 polling 영향 없음 (US-7)
- 동일 (parent_task_id, head_sha) 두 번 INSERT 시 UNIQUE 위반 (PRD §7 마지막 줄)

**Sub-agent**: `go-generator`

### Step 2 — Domain

**작업 내용**
1. `internal/domain/task.go`:
   ```go
   type Task struct {
       // ... 기존 ...
       ParentTaskID     *string
       FixAttemptCount  int
       CIStatus         CIStatus  // 신규 타입
       HeadSHA          string
       CILastPolledAt   *time.Time
   }

   type CIStatus string
   const (
       CIStatusEmpty      CIStatus = ""
       CIStatusPending    CIStatus = "pending"
       CIStatusWatching   CIStatus = "watching"
       CIStatusPassed     CIStatus = "passed"
       CIStatusFailed     CIStatus = "failed"
       CIStatusExhausted  CIStatus = "exhausted"
       CIStatusTimeout    CIStatus = "timeout"
       CIStatusStuck      CIStatus = "stuck"
       CIStatusClosed     CIStatus = "closed"
   )

   const TaskTypeCIFix TaskType = "ci-fix"
   ```
2. `internal/domain/event.go` — 4개 EventKind 상수 추가
3. `internal/domain/repository.go` — `TaskRepository` 에 3개 메서드 추가
4. `internal/domain/error.go` — `ErrFixChildAlreadyExists`, `ErrCIFixExhausted` sentinel error (필요 시)

**수락 기준**
- `go vet ./internal/domain/...` 통과
- domain 패키지가 gorm/gh CLI/외부 패키지 import 안 함 (CLAUDE.md NEVER)
- 모든 신규 enum 값에 대한 `String()` 또는 직접 비교 가능

**Sub-agent**: `go-modifier`

### Step 3 — Repository (GORM + sqlc)

**작업 내용**
1. `internal/repository/task_repository.go` 의 `gormTask` 에 5개 컬럼 매핑 추가 (struct tag `gorm:"column:..."`)
2. `db/query/task_ci.sql`:
   ```sql
   -- name: ListWatchingTasks :many
   SELECT * FROM tasks WHERE ci_status = 'watching' ORDER BY ci_last_polled_at NULLS FIRST;

   -- name: FindFixChildByParentAndHeadSHA :one
   SELECT * FROM tasks
   WHERE task_type = 'ci-fix' AND parent_task_id = ? AND head_sha = ?
   LIMIT 1;

   -- name: UpdateCIStatus :exec
   UPDATE tasks SET ci_status = ?, ci_last_polled_at = ?, head_sha = ? WHERE id = ?;
   ```
3. `sqlc generate` 실행 — `db/sqlc/` 자동 갱신 (수동 수정 금지)
4. Repository 구현체에서 sqlc 생성 함수 호출 → domain Task 매핑

**수락 기준**
- `sqlc generate` 무에러
- Repository 통합 테스트 (sqlite 임시 DB) 4개 케이스 모두 통과:
  - watching task 0건 → 빈 슬라이스
  - watching task 2건 → ci_last_polled_at NULL 우선 정렬
  - FindFixChild 동일 (parent, sha) 존재 → row 반환, 없으면 `ErrNotFound`
  - UpdateCIStatus 후 SELECT 시 컬럼 일치
- 커버리지 ≥ 85% (`internal/repository/`)

**Sub-agent**: `go-generator` + `go-tester`

### Step 4 — UseCase

**작업 내용**
1. `internal/usecase/ci_fix_usecase.go`:
   ```go
   type CIFixUseCase interface {
       EnqueueFixTask(ctx, parentTaskID, headSHA, failedStep, logTail string) (*domain.Task, error)
       ListWatching(ctx) ([]*domain.Task, error)
       MarkCIStatus(ctx, taskID, status, headSHA) error
       IsExhausted(ctx, parentTaskID) (bool, error)  // fix_attempt_count >= max_attempts
   }
   ```
2. `EnqueueFixTask`:
   - `IsExhausted` true → `ErrCIFixExhausted` 반환 (호출자가 Slack `:warning:` 발송 + (옵션) PR 코멘트)
   - `FindFixChildByParentAndHeadSHA` hit → 기존 child 반환 (no-op, 멱등)
   - 부모 fetch → `attempt = parent.FixAttemptCount + 1` (부모가 ci-fix 면 그대로 +1, 아니면 1)
   - INSERT 새 Task (`task_type=ci-fix`, `parent_task_id`, `fix_attempt_count=attempt`, `head_sha`, `status=queued`, `ci_status=''`)
   - TaskEvent `kind=ci_fix_enqueued` 1건
3. `MarkCIStatus`:
   - status enum 검증
   - UpdateCIStatus 호출 + TaskEvent 기록 (kind 자동 매핑: passed→ci_check_polled+passed, failed→ci_failure_detected, exhausted→ci_fix_exhausted)
4. `internal/usecase/task_usecase.go` 의 `ListTasks` 에 `ci_status` 필터 추가 (`GET /tasks?ci_status=watching`)

**수락 기준**
- mockery 로 `TaskRepository` mock 후 단위 테스트:
  - 정상 enqueue → 1건 INSERT, 1건 EventKind=ci_fix_enqueued
  - 중복 (FindFixChild hit) → 기존 child 반환, INSERT 0건
  - 한도 도달 → `ErrCIFixExhausted` + 1건 EventKind=ci_fix_exhausted
- 동일 head_sha 변경 시 attempt 카운터가 새로 시작 (별개 row)
- 커버리지 ≥ 85%

**Sub-agent**: `go-generator` + `go-tester`

### Step 5 — CIWatcher (`internal/ci`)

**작업 내용**
1. `internal/ci/watcher.go`:
   - `type Watcher struct { gh GitHubClient; uc CIFixUseCase; cfg CIFixConfig; clock Clock; slack SlackNotifier }`
   - `Tick(ctx)` — `ListWatching()` → 각 task 에 대해:
     - `gh pr view <pr> --json state,headRefOid,commits.author` 호출
     - PR closed/merged → `MarkCIStatus(closed)` + worktree cleanup 신호
     - last human push 감지 (`commits[-1].author != bot`) → US-2 R4 — watching 중단
     - `gh pr checks <pr> --json conclusion,name,detailsUrl,status` 호출
     - 모두 `status=completed` 가 아니면 다음 tick 까지 대기 (no-op + Debug 로그)
     - 모두 success → `MarkCIStatus(passed)` + Slack `:white_check_mark:`
     - any failure → `gh run view <id> --log-failed` 추출 → secret mask → `EnqueueFixTask`
       - `ErrCIFixExhausted` → `MarkCIStatus(exhausted)` + Slack `:warning:` + (옵션) PR 코멘트
       - 정상 → `MarkCIStatus(failed)` (자식 task 가 새 watching 시작은 다음 사이클)
   - poll_timeout 초과 (`now - first_polled_at > 30m`) → `MarkCIStatus(timeout)`
2. `internal/ci/checks.go` — `gh pr checks --json` 출력 → `[]CheckRun`, `func (c CheckSet) Status() ConclusionVerdict { allSuccess | anyFailure | anyActionRequired | inProgress }`
3. `internal/ci/log_extractor.go` — `gh run view <id> --log-failed` stdout 캡처, 16KB cap, 끝 200줄, `[truncated]` 마커
4. `internal/ci/secret_mask.go`:
   - regex: `(?i)(token|secret|password|api[_-]?key)\s*[:=]\s*\S+` → group1 + `=***`
   - `\$\{\{\s*secrets\.[A-Z0-9_]+\s*\}\}` → `***`
   - 단위 테스트: 12+ 케이스 (실측 GitHub Actions 로그 fixture 포함)
5. `internal/ci/prompt_builder.go` — `prompts/ci-fix.tmpl` 렌더 (`FailedStep`, `LogTail`, `PRNumber`, `PreviousAttempts`, `BasePrompt`)

**Scheduler 통합** (`internal/scheduler/scheduler.go` 또는 `worker.go`):
- 기존 tick loop 안에 `if cfg.CIFix.Enabled { ciWatcher.Tick(ctx) }` 호출 (직렬, inline — OI-4 결정 v1)
- `Worker.markDone` 완료 후 PR 번호 존재 시 `MarkCIStatus(watching)` + worktree 보존
- `MarkCIStatus(passed/exhausted/timeout/stuck/closed)` 호출 시 worktree cleanup 트리거 (orphan 방어)

**수락 기준**
- mockery `GitHubClient` 로 다음 시나리오 테이블 테스트:
  - all success → passed
  - 1 failure → fix enqueued (`EnqueueFixTask` 호출 1회, with masked log)
  - in_progress 혼재 → no-op (`MarkCIStatus` 호출 0회)
  - max_attempts 도달 후 failure → exhausted
  - PR closed → closed
  - poll_timeout 초과 → timeout
  - human push 감지 → watching 중단 (status='' 로 복귀 또는 closed 마킹 결정 — PRD §10 R4 따라 `closed` 권장)
- secret mask 테스트 12+ 케이스 통과
- log tail 16KB 초과 시 정확히 200줄로 자르고 `[truncated]` 마커 포함
- 커버리지 ≥ 85% (`internal/ci/`)

**Sub-agent**: `go-generator` + `go-tester`

### Step 6 — Handler (Gin + swag godoc)

**작업 내용**
1. `internal/api/mode_ci_fix_handler.go`:
   ```go
   // GetCIFixMode reports the current ci-fix mode.
   // @Summary Get ci-fix mode
   // @Tags    modes
   // @Success 200 {object} api.ModeResponse
   // @Router  /modes/ci-fix [get]

   // PatchCIFixMode toggles ci-fix mode at runtime.
   // @Summary Toggle ci-fix mode
   // @Tags    modes
   // @Accept  json
   // @Param   body body api.ModeToggleRequest true "..."
   // @Success 200 {object} api.ModeResponse
   // @Failure 400 {object} api.ErrorResponse
   // @Router  /modes/ci-fix [patch]
   ```
   - AppState 키 `ci_fix_mode` 영속 (`{enabled: bool, updated_at}`)
2. `internal/api/task_handler.go` (또는 동등) `ListTasks` 에 `ci_status` query param 파싱 추가, `TaskResponse` DTO 에 5개 신규 필드 매핑
3. `internal/api/dto.go`:
   ```go
   type TaskResponse struct {
       // ... 기존 ...
       CIStatus         string  `json:"ci_status" example:"watching"`
       ParentTaskID     *string `json:"parent_task_id,omitempty" example:"abc-123"`
       FixAttemptCount  int     `json:"fix_attempt_count" example:"1"`
       HeadSHA          string  `json:"head_sha" example:"f00ba2"`
       CILastPolledAt   *string `json:"ci_last_polled_at,omitempty" example:"2026-04-27T10:00:00Z"`
   }
   ```

**수락 기준**
- httptest:
  - `GET /tasks?ci_status=watching` → ci_status='watching' task 만 반환
  - `GET /modes/ci-fix` → `{enabled:true}` (기본)
  - `PATCH /modes/ci-fix {enabled:false}` → 200, AppState 갱신, 후속 GET 결과 일치
- swag 재생성 후 `/swagger/index.html` 에 신규 라우트 + enum 노출
- 커버리지 ≥ 85% (handler)

**Sub-agent**: `go-generator` + `go-tester`

### Step 7 — DI 조립 (`cmd/scheduled-dev-agent/main.go`)

**작업 내용**
- `cifg := cfg.CIFix`
- `ciFixUC := usecase.NewCIFixUseCase(taskRepo, eventRepo, cifg)`
- `ciWatcher := ci.NewWatcher(ghClient, ciFixUC, cifg, slackClient, clock)`
- scheduler 에 `ciWatcher` 주입 → tick loop 에서 호출
- API handler 에 `ciFixUC` 주입
- AppState repo 에 `ci_fix_mode` 키 초기화 (없으면 `{enabled: cifg.Enabled}` 로 seed)

**수락 기준**
- `go build ./...`, `go test ./cmd/...` 통과
- ci_fix.enabled=false 부팅 → watcher tick 자체 스킵 (`slog.Debug("ci-fix disabled")`)

**Sub-agent**: `go-modifier`

### Step 8 — 테스트 + 검증

```bash
sqlc generate
mockery --name=TaskRepository --dir=internal/domain --output=mocks
mockery --name=GitHubClient   --dir=internal/github --output=mocks
mockery --name=CIFixUseCase   --dir=internal/usecase --output=mocks
go build ./...
go test -race -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | tail -1   # ≥80%
go tool cover -func=coverage.out | grep -E '(internal/ci|internal/usecase/ci_fix)' # ≥85%
go vet ./...
golangci-lint run ./...
swag init -g cmd/scheduled-dev-agent/main.go -o docs
```

**수락 기준 (전체)**
- 전체 명령 0 exit
- `.claude/hooks/pre-push.sh` 통과
- swagger 에 `GET/PATCH /modes/ci-fix`, `TaskResponse` 의 신규 필드 노출
- 회귀: 기존 `Worker.markDone` 단위 테스트 통과 (워크트리 cleanup 분기 변경 검증)

**Sub-agent**: `go-tester`

---

## 2. PRD 수락 기준 매핑

| PRD Goal | 검증 방법 |
|----------|-----------|
| G1. 실패 → enqueue 2분 이내 | poll_interval=60s 기본, watcher tick 단위 테스트 (mock clock) |
| G2. max 2회 후 escalate | usecase test 의 `IsExhausted` 분기, exhausted 시 Slack 호출 mock |
| G3. 동일 (parent, sha) 0/1 INSERT | repository 통합 테스트 (UNIQUE INDEX) + usecase test (FindFixChild hit) |
| G4. window/budget/rate gate enforced | scheduler 통합 테스트 — fix task 도 dispatch 시 gate 통과 |
| G5. 프롬프트에 step + log tail 포함 | prompt_builder 단위 테스트 (template 출력 검증) |
| G6. worker pipeline 변경 최소화 | markDone hook 회귀 테스트 (기존 happy path 변경 0건) |

| PRD User Story | 검증 |
|----------------|------|
| US-1 (자동 fix 커밋) | watcher e2e + usecase + repository |
| US-2 (max attempts) | usecase IsExhausted 분기 |
| US-3 (FailedStep + LogTail) | log_extractor + prompt_builder + secret_mask 테스트 |
| US-4 (게이트 enforced) | scheduler test (window 밖 fix task = queued 상태 유지) |
| US-5 (중복 방지) | UNIQUE INDEX + usecase mock test |
| US-6 (조회 API) | mode/ci-fix + task list `ci_status=` 필터 httptest |
| US-7 (글로벌 토글) | `PATCH /modes/ci-fix` httptest + watcher tick 스킵 검증 |

---

## 3. CLAUDE.md NEVER 체크리스트

- [ ] `domain/` 외부 패키지 import 금지 — `Task` struct 에 time.Time / string / int 만, gh-go SDK import 없음
- [ ] raw SQL 금지 — `db/query/task_ci.sql` + `sqlc generate` 만, GORM raw `db.Exec` 금지
- [ ] 전역 var db 금지 — `cmd/main.go` 에서 모든 의존성 주입
- [ ] `context.Background()` 핸들러 직접 사용 금지 — `c.Request.Context()`. watcher tick 은 scheduler 가 전달한 ctx 사용
- [ ] swag 주석 없는 endpoint 추가 금지 — `/modes/ci-fix` GET/PATCH 둘 다 godoc
- [ ] 기존 migration 파일 수정 금지 — 000004 / (필요 시 000005) 신규만
- [ ] 테스트 없이 UseCase 메서드 추가 금지 — `EnqueueFixTask`, `MarkCIStatus`, `IsExhausted`, `ListWatching` 모두 단위 테스트 동시 추가
- [ ] secret 로그 노출 금지 — log_extractor 출력은 secret_mask 거친 후에만 prompt 에 삽입, slog 출력에도 raw log 직접 미노출
- [ ] `panic()` 금지 — gh CLI 실패는 retry 후 degrade

---

## 4. OI / 후속 결정 필요 (PRD §12 와 동기화)

- OI-1 secret 마스킹 패턴 보강은 실측 fixture 누적 후 `internal/ci/secret_mask.go` 에 추가 (테스트 케이스 형태로 누적)
- OI-2 human push 감지 룰은 `internal/ci/watcher.go` 의 `isHumanPush()` 헬퍼로 격리 — bot 화이트리스트 (`github-actions[bot]`, `dependabot[bot]`, 운영자 PAT user) config 노출 검토
- OI-3 `comment_on_exhaustion` 토글은 config 에 이미 반영. usecase 의 exhausted 분기에서 토글 체크
- OI-4 inline polling 으로 v1 구현. watching 누적 cap (예: 50) 초과 시 `slog.Warn` 후 가장 오래된 것부터 `closed` 강제 — v1.1 검토
- OI-5 `prompts/ci-fix.tmpl` 텍스트는 모 PRD §12 OI-7 prompt 정책과 일관되게 작성, 별도 리뷰 필요

---

## 5. 후속 수동 작업

```bash
# 첫 실행 전
sqlc generate
mockery --all --dir=internal/domain --output=mocks
swag init -g cmd/scheduled-dev-agent/main.go -o docs

# config.example.yaml 에 ci_fix 블록 추가 + README "CI Fix Loop" 섹션 보강
```
