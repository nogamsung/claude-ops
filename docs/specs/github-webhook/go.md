# github-webhook — Go 구현 가이드

상위 PRD: [`../github-webhook.md`](../github-webhook.md)

> 이 문서는 PRD 의 내용을 반복하지 않습니다. 단계·수락 기준·체크리스트만 담습니다. 의사결정·근거는 PRD 본문을 참조하세요.

---

## 0. 사전 확인

### CLAUDE.md 핵심 규칙 발췌
- **DI**: 생성자 파라미터 주입만 — 전역 `var db *gorm.DB` 금지
- **context**: 모든 레이어 `ctx context.Context` 첫 인자, 핸들러는 `c.Request.Context()` (주의: webhook handler 내부 `gin.Context` 가 raw body 를 한 번만 읽을 수 있다는 점)
- **domain import 금지**: `internal/domain/` 은 GORM/gin/외부 패키지 import 금지
- **sqlc**: 조건/페이징/조인은 sqlc — 단, 본 기능은 신규 쿼리 거의 없음 (in-memory dedup 우선)
- **swag**: 모든 신규 핸들러에 `@Summary @Tags @Router @Success @Failure` godoc 필수
- **Response DTO**: `example:"..."` json 태그 필수
- **NEVER**: 시크릿 평문 로그, `panic()`, `domain/` 외부 import, swag 없는 endpoint, raw SQL 하드코딩
- **커버리지 게이트**: ≥80% (`internal/api/webhook_*` · `internal/github/webhook` 패키지는 ≥85%)

### 영향받는 기존 파일 (수정)
- `cmd/scheduled-dev-agent/main.go` — DI 조립 (webhook handler / dedupCache / signature verifier 와이어업, env 전달)
- `internal/api/server.go` — `POST /github/webhook` 라우트 등록 (Slack 라우트 옆)
- `internal/api/dto.go` — `WebhookResponse` DTO 추가
- `internal/api/errors.go` — 변경 없음 (signature/format 에러는 핸들러 내 status 직매핑)
- `internal/config/config.go` — `Env.GitHubWebhookSecret` 필드 + `LoadEnv` 환경변수 매핑 (`GITHUB_WEBHOOK_SECRET`)
- `internal/config/validate.go` — webhook secret 미설정 시 `slog.Warn("webhook disabled: GITHUB_WEBHOOK_SECRET not set")` (기능 비활성, 검증 통과)
- `internal/github/poller.go` — `hasAllLabels`, `detectTaskType` 를 패키지 export 또는 webhook 코드를 같은 `internal/github` 패키지에 두기 (후자 권장 — DRY · 패키지 간 의존 최소)

### 신규 생성 파일
- `internal/github/webhook_verifier.go` — HMAC-SHA256 검증 (`internal/slack/verify.go` 패턴 차용, GitHub 헤더 형식)
- `internal/github/webhook_dedup.go` — `sync.Map` 기반 in-memory dedup cache (5분 TTL, GC goroutine)
- `internal/github/webhook_handler.go` — Gin 핸들러 (라우팅·검증·dedup·enqueue 오케스트레이션)
- `internal/github/webhook_handler_test.go` — handler httptest 단위 테스트
- `internal/github/webhook_verifier_test.go` — 시그니처 검증 단위 테스트 (table-driven)
- `internal/github/webhook_dedup_test.go` — dedup TTL / 동시성 테스트

> Migration 불필요. 신규 sqlc 쿼리 불필요 (기존 `ExistsByRepoAndIssue` · `Create` 재사용).

---

## 1. 단계별 작업

### Step 1 — Migration
**스킵.** in-memory dedup 만 사용 (PRD §7, OI-3 결정 — AppState 영속은 v1 보류).

### Step 2 — Domain
변경 없음. PRD §7 의 신규 컬럼 / 테이블 / EventKind 추가 모두 v1 보류.

> 단, `TaskEvent.payload_json` 의 `kind=started` payload 에 `source: "webhook"|"polling"` 키를 추가하는 것은 payload 가 free-form JSON 이므로 도메인 변경 없음. enqueue 시 payload 에 키 하나 더 채우기만 하면 됨.

### Step 3 — Repository
변경 없음. 기존 `TaskRepository.ExistsByRepoAndIssue(ctx, repo, issue)` · `Create(ctx, task)` 사용.

### Step 4 — UseCase
**최소 변경.** 기존 `TaskUseCase.EnqueueFromIssue` (또는 polling 이 호출하는 동등 함수) 의 시그니처에 `source string` 인자를 1개 추가하여 webhook 과 polling 이 동일 경로로 enqueue 하도록 통일. payload 에 `source` 만 다르게 기록.

**작업 내용**
1. `internal/usecase/task_usecase.go` — `EnqueueFromIssue(ctx, issue, source)` 또는 매개변수 풍부화. `source` 는 enum 문자열 `"webhook"|"polling"`.
2. polling 호출부 `internal/github/poller.go` 도 `source="polling"` 으로 동일 호출 경로 사용 (회귀 0건).

**수락 기준**
- polling 이 enqueue 한 task 의 TaskEvent (kind=started) payload 에 `"source":"polling"` 포함
- webhook 이 enqueue 한 task 는 `"source":"webhout"` 포함 (오타 주의: `webhook`)
- 두 경로가 같은 (repo, issue_number) 를 동시에 처리해도 `ExistsByRepoAndIssue` 가차이로 1건만 INSERT

**Sub-agent**: `go-modifier`

### Step 5 — Handler (verifier · dedup · gin handler)

#### Step 5a. `internal/github/webhook_verifier.go`
**작업 내용**
- `type WebhookVerifier struct { secret []byte }`, `NewWebhookVerifier(secret string) *WebhookVerifier`
- `Verify(rawBody []byte, signatureHeader string) error` — `sha256=<hex>` prefix 파싱 → `hmac.Equal` 비교
- `secret` 비어 있으면 `ErrWebhookDisabled` 반환 (config 미설정 시 endpoint 자체 비활성)

**수락 기준**
- table-driven 테스트: `valid` / `prefix mismatch` / `hex decode error` / `wrong secret` / `missing header` 5케이스 모두 expected error 일치
- `hmac.Equal` 사용 (timing attack 방지) — golangci-lint `gosec` 통과
- 테스트 커버리지 ≥ 95%

**Sub-agent**: `go-generator`

#### Step 5b. `internal/github/webhook_dedup.go`
**작업 내용**
- `type DedupCache interface { CheckAndAdd(deliveryID string) bool /* true=accepted, false=duplicate */ }`
- 구현: `sync.Map[string]time.Time`, TTL 5분
- 백그라운드 GC goroutine (`time.Ticker(60s)` 마다 만료 항목 제거) — `Stop()` 메서드로 중지 가능
- 주입: `cmd/main.go` 가 `NewMemDedupCache(ctx, ttl)` 로 생성 후 `ctx.Done()` 시 자동 종료

**수락 기준**
- 동일 `deliveryID` 재호출 시 false 반환
- 5분 + 1초 후 동일 ID 재호출 시 true 반환 (TTL 동작 확인 — `clock` 인터페이스 주입으로 테스트 결정성 확보)
- 동시성 테스트: 1000 goroutine × 같은 ID → 정확히 1개만 true (race detector `-race` 통과)
- 커버리지 ≥ 90%

**Sub-agent**: `go-generator`

#### Step 5c. `internal/github/webhook_handler.go`
**작업 내용 — 처리 순서 (PRD §6.1 와 1:1)**
1. `ctx := c.Request.Context()`
2. `body, err := io.ReadAll(c.Request.Body)` — body 1MB cap (`http.MaxBytesReader`)
3. `event := c.GetHeader("X-GitHub-Event")` — 누락 시 400
4. `delivery := c.GetHeader("X-GitHub-Delivery")` — 누락 시 400
5. `sig := c.GetHeader("X-Hub-Signature-256")` — 누락 시 401
6. `verifier.Verify(body, sig)` 실패 시 401 (slog.Warn 만, body 미로깅)
7. `event == "ping"` → 200 `{accepted:true, reason:"ping"}` 즉시 반환
8. `event != "issues"` → 200 `{accepted:false, reason:"ignored:event_not_supported"}` 반환
9. `dedupCache.CheckAndAdd(delivery)` false 면 200 `{accepted:false, reason:"duplicate"}`
10. JSON unmarshal `IssueEvent` (action, issue, repository, label) — 실패 시 400
11. `event.Issue.PullRequest != nil` → 200 `{accepted:false, reason:"ignored:pr_event"}`
12. action ∉ `{"opened", "labeled"}` → 200 `{accepted:false, reason:"ignored:action"}`
13. allowlist 체크: `repository.full_name ∈ config.GitHub.Repos[].Name` — 미매치 시 200 `{accepted:false, reason:"ignored:not_in_allowlist"}`
14. 라벨 매치: `hasAllLabels(issue.Labels, repoCfg.Labels)` — false 시 200 `{accepted:false, reason:"ignored:label_mismatch"}`
15. `taskRepo.ExistsByRepoAndIssue(ctx, repo, issueNumber)` true 면 200 `{accepted:false, reason:"duplicate"}`
16. `taskUC.EnqueueFromIssue(ctx, issue, "webhook")` 호출
17. INSERT 실패 시 500 (GitHub 가 재시도 → dedup 으로 자연 해소)
18. 200 `{accepted:true, reason:"queued", task_id:<uuid>}` + `slog.Info("webhook accepted", event, action, delivery, repo, issue, latency_ms)`

**swag godoc**
```go
// HandleWebhook ingests GitHub issue webhooks.
//
// @Summary  Receive GitHub webhook
// @Tags     github
// @Accept   json
// @Param    X-Hub-Signature-256 header string true "HMAC-SHA256 signature"
// @Param    X-GitHub-Delivery   header string true "Delivery UUID"
// @Param    X-GitHub-Event      header string true "Event type (issues, ping)"
// @Success  200 {object} api.WebhookResponse
// @Failure  400 {object} api.ErrorResponse
// @Failure  401 {object} api.ErrorResponse
// @Failure  500 {object} api.ErrorResponse
// @Router   /github/webhook [post]
```

**수락 기준 (httptest 단위)**
- 모든 PRD §6.2 예외 경로 케이스에 대해 status code · response body 일치하는 table-driven 테스트
- 검증 실패 / dedup hit / allowlist 외 케이스에서 `taskUC.EnqueueFromIssue` 호출 0회 (mockery 검증)
- happy path 에서 `EnqueueFromIssue` 정확히 1회 호출, payload `source="webhook"`
- e2e 풍 테스트: window 밖에서 webhook 수신 → enqueue 됨 → claude exec 호출 0회 (scheduler mock — US-7)
- 커버리지 ≥ 85%

**Sub-agent**: `go-generator` + `go-tester`

### Step 6 — DI 조립 (`cmd/scheduled-dev-agent/main.go`)
**작업 내용**
1. `cfg.Env.GitHubWebhookSecret` 로드 → `verifier := github.NewWebhookVerifier(secret)`. 비어 있으면 verifier `nil` (handler 내 nil 가드 → 503 또는 라우트 미등록)
2. `dedup := github.NewMemDedupCache(rootCtx, 5*time.Minute)`
3. `webhookHandler := github.NewWebhookHandler(verifier, dedup, taskUC, taskRepo, &cfg.GitHub, slog)`
4. `server.go` 의 `RegisterRoutes` 에 `r.POST("/github/webhook", webhookHandler.Handle)` 추가
5. body size limit middleware: `r.POST("/github/webhook", maxBytesMiddleware(1<<20), webhookHandler.Handle)` (1MB)

**수락 기준**
- `go build ./...` 통과
- `go test ./cmd/...` 통과
- secret 미설정 부팅 → `slog.Warn("github webhook disabled")` 1회, polling 은 정상 동작

**Sub-agent**: `go-modifier`

### Step 7 — 테스트
- **handler**: `webhook_handler_test.go` — httptest + mockery (`TaskUseCase`, `TaskRepository`)
- **verifier**: `webhook_verifier_test.go` — table-driven, GitHub 공식 예제 시그니처 fixture 1개 포함
- **dedup**: `webhook_dedup_test.go` — `clock` 주입, `-race` 동시성 테스트
- **e2e (선택)**: `internal/api/server_test.go` 에 `POST /github/webhook` 라우트 등록 검증 (404 아님)

**커버리지 게이트**
```bash
go test -race -coverprofile=coverage.out ./internal/github/...
go tool cover -func=coverage.out | grep -E '(webhook|verifier|dedup)' # ≥85%
```

**Sub-agent**: `go-tester`

### Step 8 — 검증
```bash
go build ./...
go test -race ./...
go vet ./...
golangci-lint run ./...
swag init -g cmd/scheduled-dev-agent/main.go -o docs   # /swagger 에 POST /github/webhook 노출 확인
```

`mockery --name=TaskUseCase --dir=internal/usecase --output=mocks` (시그니처 변경 시).

**수락 기준 (전체 종료 게이트)**
- 모든 명령 0 exit
- `/swagger/index.html` 에서 `POST /github/webhook` 보임
- `.claude/hooks/pre-push.sh` 통과 (커버리지 ≥80%)

---

## 2. PRD 수락 기준 매핑

| PRD Goal | 검증 방법 |
|----------|-----------|
| G1. 라벨 → INSERT p95 < 2s | handler 단위 테스트 latency < 200ms (network 제외) + 로컬 e2e 측정 |
| G2. dedup (delivery + repo/issue) | webhook_dedup_test + handler test (`ExistsByRepoAndIssue=true` mock) |
| G3. 401/무시 시 INSERT 0건 | handler test 의 모든 reject 케이스에서 `EnqueueFromIssue` mock 호출 횟수 0 검증 |
| G4. polling fallback 유지 | 기존 poller_test 회귀 통과, source="polling" payload 검증 |
| G5. window 무관 enqueue | scheduler mock + handler e2e — window 밖 입력 시 INSERT 1건 / dispatch 0건 |

| PRD User Story | 검증 |
|----------------|------|
| US-1 (labeled) | handler test action="labeled" |
| US-2 (opened + labels) | handler test action="opened" + labels 매치 |
| US-3 (가짜 호출) | verifier test + handler 401/400 |
| US-4 (재전송 dedup) | dedup test 5분 TTL |
| US-5 (polling fallback) | poller 통합 회귀 |
| US-6 (로그) | handler test 의 slog capture 검증 — body PII 미포함 |
| US-7 (window 게이트) | scheduler mock e2e |

---

## 3. CLAUDE.md NEVER 체크리스트

- [ ] `domain/` 패키지가 외부 패키지 (gin, gorm, github SDK) import 안 함 — 본 기능은 `internal/github/` 에 모두 두므로 자연 통과
- [ ] raw SQL 하드코딩 없음 (sqlc 또는 기존 GORM 메서드만)
- [ ] 전역 `var db` 없음, 모든 의존은 `cmd/main.go` 에서 생성자 주입
- [ ] handler 안에서 `context.Background()` 직접 사용 금지 — `c.Request.Context()` 만
- [ ] swag 주석 없는 endpoint 추가 금지 — `HandleWebhook` godoc 필수
- [ ] 기존 migration 수정 금지 — 본 기능은 migration 없음
- [ ] 테스트 없는 UseCase 메서드 추가 금지 — `EnqueueFromIssue(source)` 시그니처 변경 시 테스트 동시 갱신
- [ ] webhook secret · raw body 를 로그에 남기지 않음 (slog Warn 도 secret 비포함)
- [ ] `panic()` 금지 — 모든 에러는 status code 매핑

---

## 4. 후속 수동 작업

운영자 가이드 (README 또는 docs 보강 — 본 PRD 범위 외이지만 인지 필요):
1. GitHub repo Settings → Webhooks → Add webhook
2. Payload URL: `https://<host>/github/webhook`, Content type: `application/json`
3. Secret: `GITHUB_WEBHOOK_SECRET` 와 동일값
4. Events: "Let me select" → Issues 만 체크
5. reverse proxy (cloudflared / nginx / Caddy) 로 외부 노출 — OI-1 결정 후 가이드 업데이트
