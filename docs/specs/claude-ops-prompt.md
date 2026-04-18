# Claude Ops — Go 단일 스택 구현 프롬프트

> 이 파일은 `/planner` 가 생성한 **단일 스택(Go) 구현 지시서** 입니다.
> 대응 PRD: [`./claude-ops.md`](./claude-ops.md)
> 스택: **Go (Gin + GORM + sqlc + golangci-lint + swaggo)**
> 실행 환경: 홈서버 / VPS · systemd 또는 Docker · Claude Code CLI local login 세션 필수

---

## 맥락 (꼭 읽을 것)

- **PRD 본문**: `docs/specs/claude-ops.md`
- **스택 규칙 (최우선)**: 프로젝트 루트의 `CLAUDE.md` (Go Gin 기반) — 아키텍처, MUST / NEVER, Swagger 주석, sqlc 사용, golangci-lint 게이트, 커버리지 80%
- **관련 패턴 스킬**: `.claude/skills/go-patterns.md` (있으면)
- **외부 도구**: `claude` CLI (필수, non-interactive `-p` 모드 + `--output-format stream-json`), `gh` CLI, `git` (>= 2.17)

**스택의 절대 규칙 (CLAUDE.md 에서 가져온 요약, 충돌 시 CLAUDE.md 우선)**
- `domain/` 패키지는 외부 패키지 import 금지 (GORM, gin 등 불가)
- 레이어 방향: `handler → usecase → domain ← repository`
- 복잡 쿼리 / 조건 검색 / 집계는 **sqlc 필수**. 단순 CRUD 만 GORM.
- 모든 Handler 에 swag 주석 필수. Request/Response DTO 에 example 태그 필수.
- `context.Context` 전 레이어 전파. `context.Background()` 를 요청 경로에서 직접 쓰지 말 것 (`c.Request.Context()` 사용)
- 전역 DB 변수 금지. DI 는 생성자 파라미터로만.
- 기존 migration 수정 금지 — 새 파일만.
- 토큰 · PAT · Slack secret · PII 는 로그 출력 금지.
- `panic()` 금지 (복구 가능한 에러에).

---

## 이 프로젝트의 책임 범위 (전체)

단일 스택이므로 PRD 의 **전체** 를 이 프롬프트가 커버합니다. 포함/제외는 아래 그대로.

- **포함**: config loader, SQLite store (Task / TaskEvent / AppState), scheduler (active-window + full-mode), GitHub poller + PR creator, Claude CLI runner (spawn + pgid kill + stream-json parser), Slack 메시지 빌더 + interactions webhook (signing secret 검증), Gin HTTP API + Swagger, systemd unit + Dockerfile, 단위·통합·e2e 테스트.
- **제외 (PRD Non-goals 에서 정의)**: 웹 대시보드, 다중 사용자, 다중 LLM, 다중 세션 병렬 실행, Anthropic API key 경로, 자동 이슈 생성.

---

## 변경/생성할 파일 (체크리스트)

> 모든 경로는 프로젝트 루트 기준. Go CLAUDE.md 의 표준 배치를 따릅니다.

### 0. 루트 / 설정
- [ ] `go.mod` (`go 1.22+`, module name: `github.com/gs97ahn/claude-ops` 권장 — 팀 규칙에 맞춰 조정)
- [ ] `.golangci.yml` (CLAUDE.md 예시 기반)
- [ ] `sqlc.yaml` (engine: `sqlite`, queries: `db/query/`, schema: `migrations/`, out: `db/sqlc`)
- [ ] `.gitignore` (`.worktrees/`, `data/`, `docs/swagger/`, `bin/`, `.env`)
- [ ] `Makefile` — `run`, `test`, `cover`, `lint`, `swag`, `sqlc`, `migrate-up`, `migrate-down`, `build`, `docker`
- [ ] `config.example.yaml` — 기본 예시 (active_windows, repos, slack, github, runtime)
- [ ] `.env.example` — `GITHUB_TOKEN`, `SLACK_BOT_TOKEN`, `SLACK_SIGNING_SECRET`, `HTTP_BIND_ADDR`, `DB_PATH`, `CONFIG_PATH`
- [ ] `README.md` — 설치·세션 준비·실행·배포 섹션 (PRD 링크 포함)

### 1. Entrypoint
- [ ] `cmd/claude-ops/main.go`
  - swag 전역 주석 (`@title claude-ops API`, `@BasePath /api/v1`, `@securityDefinitions.apikey`)
  - DI 조립만 담당 (config → store → github client → slack client → claude runner → scheduler → api server)
  - graceful shutdown: SIGINT/SIGTERM → scheduler stop → in-flight task SIGTERM → HTTP drain → DB close
  - `init()` 에서 `claude` · `gh` · `git` PATH 존재 확인, 없으면 fail fast

### 2. Config
- [ ] `internal/config/config.go` — 구조체 정의, `Load(path string) (*Config, error)` (viper 또는 yaml.v3 + godotenv)
- [ ] `internal/config/validate.go` — window 중첩 검증, repo allowlist 유효성, TZ 파싱, env 필수값 검사
- [ ] `internal/config/config_test.go` — 잘못된 설정 reject, TZ 오류, 중복 repo

**Config 구조 (YAML 예시)**:
```yaml
runtime:
  http_bind_addr: "127.0.0.1:8787"
  db_path: "data/agent.db"
  log_level: "info"
  tick_interval: "30s"
  worktree_root: ".worktrees"
  prompts_dir: "prompts"
scheduler:
  active_windows:
    - days: ["mon","tue","wed","thu","fri"]
      start: "09:00"
      end:   "18:00"
      tz:    "Asia/Seoul"
github:
  poll_interval: "60s"
  repos:
    - name: "gs97ahn/example"
      default_branch: "main"
      labels: ["claude-ops"]
      reviewers: ["gs97ahn"]
      checks: { security: true, perf: false }
slack:
  channel_id: "C0XXXXXXX"
  mention_user_id: "U0XXXXXXX"
```

### 3. Migrations (golang-migrate, SQLite)
- [ ] `migrations/000001_create_tasks_table.up.sql` + `.down.sql`
- [ ] `migrations/000002_create_task_events_table.up.sql` + `.down.sql`
- [ ] `migrations/000003_create_app_state_table.up.sql` + `.down.sql`

컬럼은 PRD §7 데이터 모델 따를 것. `status` / `kind` 는 CHECK 제약, `created_at`/`updated_at` 은 DEFAULT + 인덱스.

### 4. Domain (외부 import 금지)
- [ ] `internal/domain/task.go` — `Task`, `TaskStatus`, `TaskType` enum, `TaskEvent`
- [ ] `internal/domain/repository.go` — `TaskRepository`, `TaskEventRepository`, `AppStateRepository` interface
- [ ] `internal/domain/errors.go` — `ErrNotFound`, `ErrOutsideActiveWindow`, `ErrAlreadyRunning`, `ErrFullModeOff`, `ErrClaudeUsageExhausted`
- [ ] `internal/domain/window.go` — `ActiveWindow`, `Contains(t time.Time) bool` (순수 로직, time·timezone 만 사용)

### 5. Repository (GORM + sqlc)
- [ ] `internal/repository/sqlite.go` — `NewDB(path string) (*gorm.DB, error)` + WAL pragma
- [ ] `internal/repository/task_repository.go` — 단순 CRUD 는 GORM, `ListTasks`·`SearchTasks` 는 sqlc
- [ ] `internal/repository/task_event_repository.go`
- [ ] `internal/repository/app_state_repository.go` — key-value singleton
- [ ] `db/query/task.sql` — `ListTasks :many`, `CountTasksByStatus :many`
- [ ] `db/query/task_event.sql` — `ListEventsByTask :many`
- [ ] `db/sqlc/` (자동 생성, 수동 수정 금지)

### 6. UseCase (비즈니스 로직)
- [ ] `internal/usecase/task_usecase.go`
  - `EnqueueFromIssue(ctx, repo, issue) (*Task, error)`
  - `GetTask(ctx, id) (*TaskDetail, error)`
  - `ListTasks(ctx, filter) ([]*Task, error)`
  - `StopTask(ctx, id) error` — scheduler 의 `Canceller` 인터페이스 호출 위임
- [ ] `internal/usecase/mode_usecase.go` — full-mode 조회/토글, persist to AppState
- [ ] `internal/usecase/*_test.go` — 각 public 메서드 단위 테스트 (mockery 생성 mock 사용, 직접 작성 금지)

### 7. Scheduler
- [ ] `internal/scheduler/window_gate.go` — `AllowNow(now time.Time, fullMode bool, windows []ActiveWindow) bool` (순수 함수)
- [ ] `internal/scheduler/scheduler.go`
  - `Start(ctx)` / `Stop()` 루프, ticker 간격 = config
  - tick 마다: ① window gate 검사 → false 면 스킵 ② GitHub poller 실행 ③ 큐에 queued 있으면 worker 에 dispatch (세마포어 1)
  - `Canceller` 인터페이스 노출 — `Cancel(taskID) error`
- [ ] `internal/scheduler/worker.go`
  - task 1개 실행: worktree 생성 → prompt 렌더 → claude runner 호출 → 결과 반영 → PR 생성 → worktree 정리
  - 각 단계 실패 시 TaskEvent 기록 + Slack 알림
- [ ] `internal/scheduler/clock.go` — `Clock` 인터페이스 (테스트 주입용)
- [ ] `internal/scheduler/*_test.go` — fake clock 으로 윈도우 경계 / 중첩 / TZ 테스트, worker 단계별 실패 시나리오

### 8. GitHub Integration
- [ ] `internal/github/poller.go` — allowlist 레포에 대해 open + 라벨 AND 매치 이슈 fetch (go-github 라이브러리 권장; gh CLI shell-out 도 가능하지만 go-github 선호)
- [ ] `internal/github/pr_creator.go` — `git commit/push` 는 `git` 프로세스 직접 호출 (worktree 디렉토리에서), PR 생성은 `gh pr create` shell-out (인증 공유 간편)
- [ ] `internal/github/client.go` — token from env (config 에 저장 금지)
- [ ] `internal/github/*_test.go` — httptest 로 GitHub API mock, `gh` 호출 부분은 인터페이스 분리해서 fake 주입

### 9. Claude Runner (가장 핵심)
- [ ] `internal/claude/runner.go`
  - `Run(ctx, RunInput) (RunResult, error)` — `os/exec.CommandContext("claude", "-p", prompt, "--output-format", "stream-json")` + `SysProcAttr.Setpgid=true`
  - `cwd` = worktree path, `stdin` 은 빈 파이프, stdout pipe 라인 단위 읽기
  - **중요**: spawn 직전 `windowGate.AllowNow()` 재검사 (double-gate). window 밖이고 full-mode 꺼져 있으면 즉시 `ErrOutsideActiveWindow`.
  - 출력 실시간 파싱 → TaskEvent 저장 + Slack 중계 (옵션)
- [ ] `internal/claude/stream_parser.go` — `stream-json` 라인 JSON 파싱, `UsageSignal`, `RateLimitSignal`, `ErrorSignal`, `TextChunk` 분류. **미지 필드는 skip + debug 로그** (포맷 변경에 resilient)
- [ ] `internal/claude/canceller.go` — `Cancel(pgid int)`: SIGTERM → 5s 대기 → SIGKILL (`syscall.Kill(-pgid, ...)`)
- [ ] `internal/claude/prompt.go` — 프롬프트 템플릿 렌더 (`text/template`), 입력: 이슈 본문·레포·체크 옵션
- [ ] `prompts/feature.tmpl`, `prompts/security.tmpl`, `prompts/perf.tmpl` — v1 최소 템플릿 (이슈 body + "변경 후 PR 을 준비하되 실제 push/pr 은 에이전트가 외부에서 수행한다" 라는 지시)
- [ ] `internal/claude/*_test.go` — 파서 케이스 10+ (usage, rate_limit, error, text, unknown), fake exec 으로 runner 동작 검증, cancel signal 전파 테스트

### 10. Slack
- [ ] `internal/slack/client.go` — `PostStarted`, `PostDone`, `PostFailed`, `PostCancelled`, `PostModeChange` (Block Kit 빌더 내부에서 구성)
- [ ] `internal/slack/blocks.go` — Block Kit JSON 빌더 (Stop 버튼, View PR/Issue 링크)
- [ ] `internal/slack/verify.go` — `VerifySignature(timestamp, body, signature, secret)` (X-Slack-Request-Timestamp 5분 replay 방지)
- [ ] `internal/slack/interactions.go` — `HandleInteraction(payload)` → `stop_task` action_id 라우팅
- [ ] `internal/slack/*_test.go` — 서명 검증 (정상/만료/위조), 블록 스냅샷 테스트

### 11. HTTP API (Gin + swaggo)
- [ ] `internal/api/server.go` — Gin 구성, 미들웨어 (logger, recovery, request-id)
- [ ] `internal/api/health_handler.go` — `GET /healthz`
- [ ] `internal/api/task_handler.go` — `GET /tasks`, `GET /tasks/{id}`, `POST /tasks`, `POST /tasks/{id}/stop`
- [ ] `internal/api/mode_handler.go` — `GET /modes/full`, `POST /modes/full`
- [ ] `internal/api/slack_handler.go` — `POST /slack/interactions` (본문 raw 보존 — 서명 검증용)
- [ ] `internal/api/dto.go` — Request/Response DTO + swag example 태그
- [ ] `internal/api/errors.go` — domain error → HTTP status 매핑 (내부 메시지 누설 금지)
- [ ] `internal/api/*_test.go` — handler 테스트 전부 (httptest)

**API 계약 (Slack / 외부) — 요약**

| Method | Path | Request | Response (2xx) | 에러 |
|--------|------|---------|----------------|------|
| GET | `/healthz` | - | `{status, tick_at, full_mode}` | - |
| GET | `/tasks?status=&limit=&cursor=` | - | `{items: [...], next_cursor}` | - |
| GET | `/tasks/{id}` | - | `TaskDetailResponse` | 404 |
| POST | `/tasks` | `{repo, issue_number}` | `201 TaskResponse` | 409 (window 밖 + full off), 404 (repo 없음) |
| POST | `/tasks/{id}/stop` | - | `202 {accepted: true}` | 404, 409 (이미 종료) |
| GET | `/modes/full` | - | `{enabled, since}` | - |
| POST | `/modes/full` | `{enabled}` | `200 {enabled, since}` | 400 |
| POST | `/slack/interactions` | Slack payload (urlencoded) | `200` | 401 (서명 실패) |

Slack Block Kit 시작 메시지 예시는 PRD §8 참조. 종료 메시지는 `View PR` 버튼 (url) + diff 요약 section.

### 12. 외부 계약 (이 프로젝트가 제공/소비)

- **→ Slack 으로 전송**: `chat.postMessage` (bot token 사용). 응답 메시지 `ts` 는 TaskEvent 에 저장하여 후속 update 가능.
- **← Slack 에서 수신**: `POST /slack/interactions` — signing secret 검증 필수.
- **→ GitHub 호출**: issues list, PR create (via `gh`), repo push (via `git`).
- **→ Claude CLI 호출**: `claude -p <prompt> --output-format stream-json`. 세션은 머신의 `~/.claude` 에 의존.

계약이 변경되면 **PRD §8 API 계약** 을 먼저 갱신한 뒤 이 프롬프트 를 업데이트할 것.

### 13. 테스트 전략
- [ ] 단위 테스트: domain, config, scheduler/window_gate, claude/stream_parser, slack/verify, usecase 전부
- [ ] 통합 테스트: `internal/api/*_test.go` 에서 real SQLite (tempfile) + mock external (github/slack/claude) → end-to-end HTTP flow
- [ ] e2e 회귀: `testutil/e2e/` — fake clock 이 window 밖이면 `claude` exec 호출이 **0회** 임을 검증 (mock runner 에 카운터 부착)
- [ ] mock: `mockery` 설정 파일 (`.mockery.yaml`), `mocks/` 에 자동 생성
- [ ] 커버리지: 전체 80% 이상, `scheduler`·`claude`·`slack` 패키지는 **85% 이상**
- [ ] golangci-lint: `make lint` 통과
- [ ] swag: Handler 주석 변경 시 `swag init -g cmd/claude-ops/main.go -o docs`

### 14. 배포
- [ ] `deployments/claude-ops.service` (systemd unit)
  - `User=<운영자>` (Claude 세션 소유자), `WorkingDirectory=/srv/claude-ops`
  - `EnvironmentFile=/etc/claude-ops/.env`
  - `ExecStart=/usr/local/bin/claude-ops -config /etc/claude-ops/config.yaml`
  - `Restart=on-failure`, `RestartSec=5s`
  - `KillMode=mixed` (자식 `claude` 프로세스 그룹도 종료되도록)
- [ ] `deployments/Dockerfile` — multi-stage (build + alpine runtime), `claude`·`gh`·`git` 설치 지시 or docs 로 host-mount 방식 명시
- [ ] `deployments/docker-compose.yml` — volumes: `./data:/data`, `~/.claude:/root/.claude:ro` (세션 공유 주의), `./config.yaml`, `./.worktrees` (단, worktree 는 Claude 가 수정하므로 RW)
- [ ] `deployments/README.md` — systemd 경로·Docker 세션 공유 주의사항 (PRD §9 보안 요구사항 반영)

---

## 구현 순서 (Bottom-up, 권장)

1. **프로젝트 뼈대**: `go.mod`, `.golangci.yml`, `Makefile`, `sqlc.yaml`, `.gitignore`, `config.example.yaml`
2. **Migrations** → `migrations/000001..3`
3. **Domain** → `internal/domain/*` (테스트 먼저 — 순수 로직)
4. **Repository** → sqlite + GORM + sqlc 조합
5. **Config** → load + validate + 테스트
6. **Scheduler**
   1. `window_gate.go` + 단위 테스트 (fake clock, TZ, 중첩)
   2. `clock.go`
   3. `scheduler.go` + `worker.go` (mock runner 로 테스트)
7. **Claude Runner**
   1. `stream_parser.go` + 풍부한 파서 테스트
   2. `runner.go` — fake exec 으로 spawn/pgid/cancel 테스트
   3. `prompt.go` + `prompts/*.tmpl`
8. **GitHub** poller + pr_creator (httptest + gh shell-out 인터페이스 분리)
9. **Slack** verify (먼저 보안 테스트) → blocks → client → interactions
10. **UseCase** (task, mode) + 단위 테스트
11. **HTTP API** (Gin handler 전부) + swag 주석 + 통합 테스트
12. **Entrypoint** `cmd/claude-ops/main.go` — DI wiring, graceful shutdown
13. **Deployments** — systemd unit, Dockerfile, docker-compose, README
14. **E2E**: `testutil/e2e` 에 window 밖 실행 금지 / Stop 버튼 / full-mode 토글 시나리오

각 단계가 끝날 때마다 `make lint test` 가 통과해야 다음 단계로 넘어갑니다.

---

## 구현 제약 (CLAUDE.md 와 충돌하지 않을 것)

프로젝트 루트 `CLAUDE.md` (Go Gin) 가 최우선입니다. 이 프롬프트는 그 제약 안에서만 동작합니다. 특히:

- `domain/` 에서 외부 패키지 import 절대 금지 → `window.go` 는 `time` 만 사용.
- 복잡 쿼리 (`ListTasks` 필터, `CountTasksByStatus`) 는 반드시 **sqlc**. GORM 으로 raw 문자열 SQL 작성 금지.
- 모든 Handler 에 `swag` 주석 + `@Router` + `@Security BearerAuth` (관리 API 는 내부망 bind 이지만 향후 인증 추가 대비 플레이스홀더).
- 에러 응답은 domain error → HTTP status 매핑. 내부 에러 메시지 그대로 노출 금지.
- mock 은 mockery 자동 생성. 수동 작성 금지.
- Claude CLI 호출 시 **반드시 `-p` non-interactive 모드 + `--output-format stream-json`**. 대화형 spawn 금지.

---

## 실행 지시 (이 프롬프트를 받은 agent 용)

1. 프로젝트 루트 `CLAUDE.md` (Go Gin) 를 먼저 읽어 스택 규칙 숙지
2. `memory/MEMORY.md` 존재 시 과거 결정·교훈 스캔
3. 위 체크리스트를 **순서대로** 생성 (Migrations → Domain → … → API → 배포)
4. 각 섹션 끝날 때마다 `golangci-lint run ./...` + 관련 테스트 통과 확인
5. Handler 가 생길 때마다 swag 주석 추가, 마지막에 `swag init -g cmd/claude-ops/main.go -o docs` 실행
6. 완료 후 다음 리포트 반환:
   - 생성된 파일 목록 (경로)
   - 신규 외부 의존성 (go.mod diff 요약)
   - 배포 시 **사람이 수동으로 해야 하는 일**: ① `claude login` (대상 머신에서) ② `gh auth login` ③ `.env` 값 채우기 ④ Slack app 생성·권한 (chat:write, commands, actions) ⑤ Slack interactive endpoint URL 등록
   - 미결 PRD 오픈 이슈 중 구현하며 확정된 것 (예: stream-json 실제 스키마) → PRD 업데이트 제안

---

## 성공 기준 (Definition of Done)

- 모든 체크리스트 항목 체크됨
- `go test ./... -race -coverprofile=coverage.out` 통과, total coverage ≥ 80% (scheduler/claude/slack ≥ 85%)
- `golangci-lint run ./...` 무경고
- `swag init` 산출물 커밋, `/swagger/index.html` 정상 노출
- `make build` 로 단일 바이너리 생성, systemd unit / Docker compose 로 기동 확인
- e2e 검증: window 밖 시각에서 scheduler 가 `claude` 를 **0회** 호출 (mock runner 카운터 = 0)
- Slack Stop 버튼 → 5초 내 SIGTERM, 30초 내 SIGKILL, worktree 롤백 확인
- Full-mode on 상태에서 rate-limit 시그널 감지 시 자동으로 off + Slack 통지
- PRD §9 비기능 요구사항 전 항목 충족
- CLAUDE.md 의 MUST/NEVER 항목 위반 없음

---

## 13. Claude CLI 파서 계약 및 프롬프트 템플릿 (OI-1 / OI-7 해소 반영)

### 13.1 stream-json NDJSON 파서 계약

`internal/claude/stream/` 에 구현. CLI `v2.1.113` 실측 기반. NDJSON (line-delimited) 을 `bufio.Scanner` 로 한 줄씩 읽어 다음 단계로 처리:

```go
// internal/claude/stream/events.go
type Envelope struct {
    Type      string          `json:"type"`
    SessionID string          `json:"session_id"`
    UUID      string          `json:"uuid"`
    Raw       json.RawMessage `json:"-"` // 원본 보존 (미지 필드 로깅용)
}

// Type 별 구체 타입
type SystemInit struct {
    CWD             string   `json:"cwd"`
    Model           string   `json:"model"`
    PermissionMode  string   `json:"permissionMode"`
    APIKeySource    string   `json:"apiKeySource"` // "none" 이어야 구독 세션
    ClaudeCodeVer   string   `json:"claude_code_version"`
    OutputStyle     string   `json:"output_style"`
    // ... tools, mcp_servers, slash_commands 등은 필요 시 추가
}

type AssistantEvent struct {
    Message struct {
        Model   string `json:"model"`
        ID      string `json:"id"`
        Role    string `json:"role"`
        Content []struct {
            Type string          `json:"type"` // "text" | "tool_use" | ...
            Text string          `json:"text,omitempty"`
            Raw  json.RawMessage `json:"-"`
        } `json:"content"`
        Usage MessageUsage `json:"usage"`
    } `json:"message"`
}

type MessageUsage struct {
    InputTokens             int    `json:"input_tokens"`
    CacheCreationInputTokens int   `json:"cache_creation_input_tokens"`
    CacheReadInputTokens     int   `json:"cache_read_input_tokens"`
    OutputTokens            int    `json:"output_tokens"`
    ServiceTier             string `json:"service_tier"`
}

type RateLimitEvent struct {
    RateLimitInfo struct {
        Status                 string `json:"status"`          // "allowed" | "blocked" | ...
        ResetsAt               int64  `json:"resetsAt"`        // unix seconds
        RateLimitType          string `json:"rateLimitType"`   // "five_hour"
        OverageStatus          string `json:"overageStatus"`
        OverageDisabledReason  string `json:"overageDisabledReason,omitempty"`
        IsUsingOverage         bool   `json:"isUsingOverage"`
    } `json:"rate_limit_info"`
}

type ResultEvent struct {
    Subtype          string                 `json:"subtype"`          // "success" | "error" | "error_during_execution"
    IsError          bool                   `json:"is_error"`
    APIErrorStatus   *string                `json:"api_error_status"`
    DurationMS       int64                  `json:"duration_ms"`
    DurationAPIMS    int64                  `json:"duration_api_ms"`
    NumTurns         int                    `json:"num_turns"`
    Result           string                 `json:"result"`
    StopReason       string                 `json:"stop_reason"`       // "end_turn" | ...
    TotalCostUSD     float64                `json:"total_cost_usd"`
    Usage            ResultUsage            `json:"usage"`
    ModelUsage       map[string]ModelUsage  `json:"modelUsage"`
    PermissionDenials []PermissionDenial    `json:"permission_denials"`
    TerminalReason   string                 `json:"terminal_reason"`   // "completed" | "interrupted" | ...
}

type ModelUsage struct {
    InputTokens              int     `json:"inputTokens"`
    OutputTokens             int     `json:"outputTokens"`
    CacheReadInputTokens     int     `json:"cacheReadInputTokens"`
    CacheCreationInputTokens int     `json:"cacheCreationInputTokens"`
    WebSearchRequests        int     `json:"webSearchRequests"`
    CostUSD                  float64 `json:"costUSD"`
    ContextWindow            int     `json:"contextWindow"`
    MaxOutputTokens          int     `json:"maxOutputTokens"`
}
```

**핵심 규칙**:
- 알 수 없는 `type` 은 `slog.Debug("unknown stream event", "type", t, "raw", string(raw))` 후 무시 (CLI 상위호환).
- 알 수 없는 **필드** 는 Go JSON decoder 기본 동작으로 무시. 단 raw 는 보존.
- `result` 이벤트는 정확히 1회. 도착 시 파서 종료.
- **Rate limit 판정**: `event.Type == "rate_limit_event" && e.RateLimitInfo.Status != "allowed"` → `ErrRateLimited{ResetsAt, Type}` 반환.
- **세션 만료 판정**: `system.init` 수신 시 `APIKeySource != "none"` 이면 `ErrSessionMissing` 반환 + Slack 긴급 알림. stderr 에 "please login" / "not logged in" 포함 시도 동일.

**계약 테스트 fixture**: `internal/claude/stream/testdata/`
- `success.ndjson` — hook_*, init, assistant, result(success) 시퀀스
- `rate_limit.ndjson` — init, rate_limit_event(blocked), result(error)
- `session_missing.ndjson` — init(apiKeySource != "none")
- `cancelled.ndjson` — SIGTERM 으로 인한 프로세스 종료 (result 이벤트 없음)

### 13.2 작업 프롬프트 템플릿 (v1)

**공통 변수**:
```go
type PromptData struct {
    Repo         string   // "owner/name"
    Issue        IssueCtx // Number, Title, Body, Labels, Author, URL
    Branch       string   // ex: "claude/issue-123"
    BaseBranch   string   // ex: "main"
    Worktree     string   // 절대 경로
    TaskType     string   // "feature" | "security" | "performance"
}
```

**`prompts/feature.tmpl`** (기능 개발):
```text
당신은 `{{.Repo}}` 레포의 issue #{{.Issue.Number}} 를 해결하기 위해 호출되었습니다.

## 이슈
제목: {{.Issue.Title}}
작성자: @{{.Issue.Author}}
라벨: {{range .Issue.Labels}}{{.}} {{end}}
URL: {{.Issue.URL}}

본문:
---
{{.Issue.Body}}
---

## 작업 규칙 (반드시 준수)
1. 현재 작업 브랜치 `{{.Branch}}` 는 이미 체크아웃되어 있고, base 는 `{{.BaseBranch}}` 입니다.
2. 변경은 이 워크트리(`{{.Worktree}}`) 내에서만 수행하세요.
3. **금지**: `gh pr create` / `git push` / `git remote` 호출. PR 생성과 push 는 외부 오케스트레이터가 담당합니다.
4. **허용**: 파일 읽기·쓰기·편집, 로컬 명령 실행 (테스트·빌드·린트), `git add` / `git commit`.
5. 테스트 파일이 존재하면 관련 테스트를 실행해 통과 여부를 확인하세요.
6. 외부 네트워크 호출(`curl`, `wget`, HTTP API 등)은 금지. 패키지 매니저(`npm install`, `go mod tidy` 등) 는 꼭 필요할 때만.
7. 커밋 메시지는 Conventional Commits 형식 (`feat:`, `fix:`, `refactor:` 등).

## 완료 조건
- 이슈 본문의 요구사항을 구현
- 기존 테스트 전부 통과
- 변경에 대한 최소한의 테스트 추가 (신규 기능은 필수)
- 마지막 응답에 아래 형식의 요약을 포함:

```
CHANGES:
- <파일1>: <한 줄 설명>
- <파일2>: <한 줄 설명>
TESTS_RUN: <pass|fail|skipped>
NOTES: <리뷰어가 알아야 할 사항>
```

시작하세요.
```

**`prompts/security.tmpl`** (보안 점검) — feature.tmpl 에서 다음만 교체:
- 이슈 본문 아래에 "## 점검 체크리스트" 추가: 입력 검증 / 인증·인가 / 시크릿 하드코딩 / SQL·커맨드 인젝션 / XSS·CSRF / 의존성 CVE / 로깅에 민감정보 노출 / 권한 축소 원칙.
- 완료 조건에 "SECURITY_FINDINGS: severity=high|med|low 목록" 추가.
- 코드 변경 없이 리포트만 커밋하는 경우도 허용 (`docs/security/issue-{{.Issue.Number}}.md`).

**`prompts/performance.tmpl`** (성능 점검) — feature.tmpl 에서 다음만 교체:
- "## 점검 체크리스트": 핫패스 / N+1 쿼리 / 불필요한 allocation / goroutine leak / 캐시 적중률 / 인덱스 / 번들 크기 / 렌더링 블록.
- 완료 조건에 "BENCHMARK_RESULTS: before/after" 또는 "PROFILING: <결과 요약>" 추가.

### 13.3 Claude CLI 호출 명령

`internal/claude/runner.go` 가 다음 동등한 명령을 실행:

```
claude \
  -p "<PromptData 로 렌더된 템플릿>" \
  --output-format stream-json \
  --verbose \
  --permission-mode acceptEdits \
  --allowedTools "Bash,Edit,Read,Write,Glob,Grep" \
  --disallowedTools "WebFetch,WebSearch" \
  --session-id <uuid>   # Task 재개 필요 시 --resume 사용
```

- `--dangerously-skip-permissions` 는 **사용하지 않음** (보안). `acceptEdits` 로 파일 편집만 승인, 명령 실행은 allowlist 제약.
- `cmd.Dir = Worktree`, `cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}`.
- stdin 은 닫거나 `/dev/null`. 추가 입력이 필요한 경우 `--input-format stream-json` + NDJSON 주입 고려 (v1 에서는 단방향).
- stderr 는 별도 파이프로 캡처해 세션 만료·권한 거부 등 out-of-band 신호 파싱.

---

> 이 프롬프트는 `/planner` 가 자동 생성했고, 실측(`claude -p --output-format stream-json --verbose`, v2.1.113) 결과를 반영해 §13 을 보강했습니다. Agent 에게 전달 전 수동 검토·수정 가능합니다.
> PRD 변경 시 이 프롬프트도 동기화해야 합니다 — 계약 필드 (§13.1) 를 특히 주의.
