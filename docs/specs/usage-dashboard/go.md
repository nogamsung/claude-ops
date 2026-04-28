# usage-dashboard — Go 구현 가이드

상위 PRD: [`../usage-dashboard.md`](../usage-dashboard.md)

> 이 문서는 PRD 의 결정·근거를 반복하지 않습니다. 단계·체크리스트·수락 기준만 담습니다. 모순 시 PRD 가 우선.

---

## 0. 사전 확인

### CLAUDE.md 핵심 규칙
- DI 생성자 주입 / 전역 db 금지
- 모든 레이어 `ctx context.Context` 첫 인자
- `domain/` 외부 패키지 import 금지
- 동적·집계·페이징·조인 쿼리는 sqlc 필수 — 본 기능의 `SumUsageByBucket`, `SumUsageByModel`, `SumUsageDaily/Weekly` 모두 sqlc
- 모든 신규 핸들러 swag godoc 필수, Response DTO 의 모든 필드 `example:"..."` 태그
- 기존 migration 수정 금지 — `000004_*` 신규 추가만
- 테스트 없이 UseCase 메서드 추가 금지
- 커버리지 ≥80% (`internal/usecase/usage_*` 와 sqlc 매핑·gap-fill·warn 게이트는 ≥85%)

### 영향받는 기존 파일 (수정)
- `internal/domain/task.go` — Task 구조체에 6개 필드 추가 (`CostUSD float64`, `TotalInputTokens int64`, `TotalOutputTokens int64`, `CacheCreationInputTokens int64`, `CacheReadInputTokens int64`, `ModelUsageJSON string`)
- `internal/domain/repository.go` — 기존 `TaskRepository` 변경 없음. 신규 `UsageRepository` 인터페이스 추가
- `internal/repository/task_repository.go` — `gormTask` 매핑 6개 필드 추가
- `internal/usecase/budget_usecase.go` — `cost_warn_state` AppState 영속 + 80%/100% 임계 평가 메서드 (`EvaluateCostWarn`)
- `internal/scheduler/worker.go` — `applyRunUsage` (또는 동등 함수) 확장 → `RunResult` 의 cost / modelUsage / 4개 토큰 컬럼을 Task 에 적용 + cost warn 트리거
- `internal/claude/runner.go` — `RunResult` 에 `ModelUsage` 필드 추가 (이미 `total_cost_usd` / `Usage` 는 파싱 중이지만 worker 까지 전달되는지 확인)
- `internal/claude/stream/parser.go` — `Signal.Result.ModelUsage` 가 worker 까지 통과되도록 매핑 추가 (소량 변경)
- `internal/api/dto.go` — `UsageResponse`, `UsageBucket`, `UsageTotals`, `ModelUsageResponse`, `ModelUsageItem`, `UsageLimitsResponse` DTO 추가
- `internal/api/server.go` — 3개 라우트 등록
- `internal/config/config.go` — `Limits.DailyMaxCostUSD float64`, `Limits.WeeklyMaxCostUSD float64` 추가
- `internal/config/validate.go` — 음수 금지, 0 허용 (미설정 의미)
- `internal/slack/client.go` — `NotifyCostWarning(ctx, scope, percent, current, max)` 추가
- `cmd/scheduled-dev-agent/main.go` — DI 조립
- `config.example.yaml` — `limits.daily_max_cost_usd`, `weekly_max_cost_usd` 추가

### 신규 생성 파일
- `migrations/000004_add_cost_columns_to_tasks.up.sql`
- `migrations/000004_add_cost_columns_to_tasks.down.sql`
- `db/query/usage.sql` — sqlc 집계 쿼리
- `internal/repository/usage_repository.go` — sqlc-backed 구현체
- `internal/usecase/usage_usecase.go` — gap-fill / by-model / limits / from-to 검증
- `internal/api/usage_handler.go` — 3개 핸들러
- 테스트: `internal/usecase/usage_usecase_test.go`, `internal/repository/usage_repository_test.go`, `internal/api/usage_handler_test.go`, `internal/usecase/budget_usecase_cost_test.go`

---

## 1. 단계별 작업

### Step 1 — Migration

**파일**: `migrations/000004_add_cost_columns_to_tasks.up.sql`
```sql
ALTER TABLE tasks ADD COLUMN cost_usd                       REAL    NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN total_input_tokens             INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN total_output_tokens            INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN cache_creation_input_tokens    INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN cache_read_input_tokens        INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN model_usage_json               TEXT    NOT NULL DEFAULT '{}';
```

`000004_*.down.sql`:
```sql
ALTER TABLE tasks DROP COLUMN model_usage_json;
ALTER TABLE tasks DROP COLUMN cache_read_input_tokens;
ALTER TABLE tasks DROP COLUMN cache_creation_input_tokens;
ALTER TABLE tasks DROP COLUMN total_output_tokens;
ALTER TABLE tasks DROP COLUMN total_input_tokens;
ALTER TABLE tasks DROP COLUMN cost_usd;
```

> 인덱스 추가는 v1 보류 (10k 기준 OK). PRD §7 — 100k 임계 시 `idx_tasks_status_finished_at` 검토.

> 기존 `estimated_input_tokens` / `estimated_output_tokens` 컬럼은 **유지**. 신규 컬럼이 권위있는 값.

**수락 기준**
- migrate up/down 정상 적용
- 기존 task row 들이 영향 없이 cost_usd=0, model_usage_json='{}' 로 초기화

**Sub-agent**: `go-generator`

### Step 2 — Domain

**작업 내용**
1. `internal/domain/task.go`:
   ```go
   type Task struct {
       // ... 기존 ...
       CostUSD                  float64
       TotalInputTokens         int64
       TotalOutputTokens        int64
       CacheCreationInputTokens int64
       CacheReadInputTokens     int64
       ModelUsageJSON           string  // 원본 JSON, 빈 값은 "{}"
   }
   ```
2. `internal/domain/repository.go`:
   ```go
   type UsageRepository interface {
       SumByBucket(ctx context.Context, from, to time.Time, bucket BucketKind) ([]UsageBucketRow, error)
       SumByModel(ctx context.Context, from, to time.Time) ([]UsageModelRow, error)
       SumDailyCost(ctx context.Context, dayKey string) (float64, error)
       SumWeeklyCost(ctx context.Context, weekKey string) (float64, error)
   }

   type BucketKind string
   const (
       BucketDay   BucketKind = "day"
       BucketWeek  BucketKind = "week"
       BucketMonth BucketKind = "month"
   )

   type UsageBucketRow struct {
       Bucket               string
       TaskCount            int64
       CostUSD              float64
       InputTokens          int64
       OutputTokens         int64
       CacheReadTokens      int64
       CacheCreationTokens  int64
       FailedCostUSD        float64
   }

   type UsageModelRow struct {
       ModelID              string
       TaskCount            int64
       CostUSD              float64
       InputTokens          int64
       OutputTokens         int64
       CacheReadTokens      int64
       CacheCreationTokens  int64
   }
   ```

**수락 기준**
- domain 패키지가 외부 패키지 import 안 함
- `go vet` 통과

**Sub-agent**: `go-modifier`

### Step 3 — Repository (sqlc)

**작업 내용**
1. `db/query/usage.sql`:
   ```sql
   -- name: SumUsageByDay :many
   SELECT
       date(COALESCE(finished_at, created_at)) AS bucket,
       COUNT(*) FILTER (WHERE status = 'done')          AS task_count,
       COALESCE(SUM(cost_usd) FILTER (WHERE status = 'done'), 0) AS cost_usd,
       COALESCE(SUM(total_input_tokens) FILTER (WHERE status = 'done'), 0)             AS input_tokens,
       COALESCE(SUM(total_output_tokens) FILTER (WHERE status = 'done'), 0)            AS output_tokens,
       COALESCE(SUM(cache_read_input_tokens) FILTER (WHERE status = 'done'), 0)        AS cache_read_tokens,
       COALESCE(SUM(cache_creation_input_tokens) FILTER (WHERE status = 'done'), 0)    AS cache_creation_tokens,
       COALESCE(SUM(cost_usd) FILTER (WHERE status IN ('failed','cancelled')), 0)      AS failed_cost_usd
   FROM tasks
   WHERE COALESCE(finished_at, created_at) >= ?
     AND COALESCE(finished_at, created_at) <  ?
   GROUP BY bucket
   ORDER BY bucket;

   -- name: SumUsageByWeek  :many   (strftime('%Y-W%W', ...))
   -- name: SumUsageByMonth :many   (strftime('%Y-%m',  ...))

   -- name: SumUsageByModel :many
   --   model_usage_json 을 application-side scan 으로 합산 (PRD §10 R2 — v1 application-side OK)
   --   sqlc 는 SELECT 만 하고 application 에서 JSON 펼치기 (이 쿼리는 SELECT cost_usd, model_usage_json 만)
   SELECT cost_usd, model_usage_json,
          total_input_tokens, total_output_tokens,
          cache_read_input_tokens, cache_creation_input_tokens
     FROM tasks
    WHERE status = 'done'
      AND COALESCE(finished_at, created_at) >= ?
      AND COALESCE(finished_at, created_at) <  ?;

   -- name: SumDailyCost  :one  (status='done' AND day key)
   -- name: SumWeeklyCost :one
   ```
2. `sqlc generate` → `db/sqlc/` 자동 갱신
3. `internal/repository/usage_repository.go`:
   - `SumByBucket(BucketDay)` → `SumUsageByDay` 호출 후 `[]UsageBucketRow` 매핑
   - `SumByBucket(BucketWeek)` → `SumUsageByWeek`
   - `SumByBucket(BucketMonth)` → `SumUsageByMonth`
   - `SumByModel` → `SumUsageByModel` 결과 row 마다 `model_usage_json` 을 `map[string]ModelUsageEntry` 로 unmarshal → application-side group-by 로 모델별 합산
4. `internal/repository/task_repository.go` 의 `gormTask` 에 6개 컬럼 매핑 추가

**수락 기준**
- sqlc generate 무에러
- 통합 테스트 (sqlite seed):
  - 같은 일자 done task 3건 → bucket 1개, task_count=3, cost_usd 합 일치 (±0.001)
  - failed task 의 cost 는 `failed_cost_usd` 로만 합산
  - 빈 기간 → 빈 슬라이스 (gap-fill 은 usecase 책임)
  - by-model: model 2개 task 5건 → cost_usd 내림차순, modelID="" → "unknown" 으로 합산
- 커버리지 ≥ 85%

**Sub-agent**: `go-generator` + `go-tester`

### Step 4 — UseCase

**작업 내용**
1. `internal/usecase/usage_usecase.go`:
   ```go
   type UsageUseCase interface {
       Aggregate(ctx, from, to time.Time, bucket BucketKind) (UsageResult, error)
       ByModel(ctx, from, to time.Time) ([]UsageModelRow, error)
       Limits(ctx, now time.Time) (LimitsSnapshot, error)
   }
   ```
2. `Aggregate`:
   - `from > to` → `ErrInvalidRange`
   - 기간 > 365일 → `ErrRangeTooLarge`
   - `BucketKind` enum 검증 → `ErrInvalidBucket`
   - 기본값: `to = now date`, `from = to - 30d`
   - repo 호출 → **gap-fill**: 빈 일/주/월도 0 으로 채워 반환 (timezone = config.Schedule.ResetTZ 기준 — `BudgetCounters.DailyKey/WeeklyKey/MonthlyKey` 재사용)
   - `totals` 합계 계산
3. `Limits`:
   - `dayKey := budgetCounters.DailyKey(now)`, `weekKey := budgetCounters.WeeklyKey(now)`
   - `repo.SumDailyCost(dayKey)`, `repo.SumWeeklyCost(weekKey)`
   - `max=0 → percent=null`, 그 외 `percent = round(cur/max*100, 1)`
4. `internal/usecase/budget_usecase.go` 확장:
   - `EvaluateCostWarn(ctx, now)` → cost_warn_state 영속 (AppState `cost_warn_state` 키 — daily/weekly_key, daily/weekly_warned_80/100)
   - 키가 바뀌면 (`now` 가 새 일/주에 진입) 플래그 리셋
   - `[80, 100)` 진입 + `daily_warned_80=false` → Slack `:warning:` 발송 + 플래그 set (발송 실패해도 set — R3)
   - `>= 100` + `daily_warned_100=false` → Slack `:rotating_light:` 발송 + 플래그 set
   - `daily_max_cost_usd == 0` → no-op

**수락 기준**
- mockery `UsageRepository` mock 후 단위 테스트:
  - happy path day/week/month 각 케이스
  - gap-fill: 비어있는 중간 일자 0 으로 채워짐 (`2026-W17` 빠짐 없이 출력)
  - `from > to` → 400 에러
  - 365일 초과 → 400
  - by-model: cost_usd 내림차순, modelID 빈 값 → "unknown" 합산
  - limits: max=0 → percent=null
  - cost warn:
    - 70% → 발송 0회
    - 70 → 85 진입 → :warning: 1회
    - 같은 날 다시 평가 → 발송 0회 (daily_warned_80=true)
    - 85 → 105 진입 → :rotating_light: 1회 (warned_80 은 이미 true 라 추가 발송 없음)
    - 다음 날 (dayKey 변경) → 플래그 리셋, 다시 80% 진입 시 발송
- 커버리지 ≥ 85%

**Sub-agent**: `go-generator` + `go-tester`

### Step 5 — Worker 통합 (cost / modelUsage 영속화 + warn 트리거)

**작업 내용**
1. `internal/claude/stream/parser.go` — `ResultEvent` 가 이미 `TotalCostUSD`, `Usage`, `ModelUsage` 파싱 중인지 확인 → 미흡 시 보강 (Signal.Result 에 ModelUsage map 통과)
2. `internal/claude/runner.go` — `RunResult` 에 `ModelUsage map[string]ModelUsageEntry` (혹은 raw JSON string) 필드 추가, parser → runner → worker 까지 전달
3. `internal/scheduler/worker.go` `applyRunUsage` (또는 동등):
   - 기존: `EstimatedInputTokens` / `EstimatedOutputTokens` 만 set
   - 추가: `task.CostUSD = result.TotalCostUSD`, `task.TotalInputTokens = result.Usage.InputTokens`, `Output...`, `CacheCreation/Read...`, `task.ModelUsageJSON = json.Marshal(result.ModelUsage)` (빈 map 이면 `"{}"`)
   - `result` 미수신 (crash 등) → 0 / "{}" 유지 (Update 호출 안 하면 default 유지)
4. markDone 직전 `Update(task)` → 영속화
5. markDone 이후 `budgetUC.EvaluateCostWarn(ctx, now)` 호출 — Slack 발송 트리거

**수락 기준**
- worker 단위 테스트 (mock Runner / mock Repo):
  - result.TotalCostUSD=0.5 → task.CostUSD=0.5 로 Update 호출
  - result.ModelUsage 비어있음 → task.ModelUsageJSON="{}"
  - result 미수신 (Run 에러) → CostUSD=0 유지, status=failed
  - cost warn 80% 진입 케이스에서 `slackClient.NotifyCostWarning` 1회 호출
- 회귀: 기존 worker happy path 테스트 모두 통과 (estimated tokens 도 여전히 set)
- 커버리지 ≥ 85%

**Sub-agent**: `go-modifier` + `go-tester`

### Step 6 — Handler (Gin + swag godoc)

**작업 내용**
1. `internal/api/usage_handler.go`:
   ```go
   // GetUsage returns a time-series usage aggregation.
   //
   // @Summary  Aggregated usage by bucket
   // @Tags     usage
   // @Param    from     query string false "ISO date YYYY-MM-DD"
   // @Param    to       query string false "ISO date YYYY-MM-DD"
   // @Param    group_by query string false "day|week|month" Enums(day, week, month)
   // @Success  200 {object} api.UsageResponse
   // @Failure  400 {object} api.ErrorResponse
   // @Router   /usage [get]
   func (h *UsageHandler) GetUsage(c *gin.Context) { ... }

   // GetUsageByModel returns per-model usage aggregation.
   // @Router /usage/by-model [get]
   func (h *UsageHandler) GetUsageByModel(c *gin.Context) { ... }

   // GetUsageLimits returns current vs configured cost limits.
   // @Router /usage/limits [get]
   func (h *UsageHandler) GetUsageLimits(c *gin.Context) { ... }
   ```
2. `internal/api/dto.go` — DTO + 모든 필드에 `example:"..."` 태그 (PRD §8 응답 예시 그대로)
3. `internal/api/server.go` — 3개 라우트 등록
4. domain error → HTTP status 매핑:
   - `ErrInvalidRange`, `ErrInvalidBucket`, `ErrRangeTooLarge` → 400
   - 그 외 → 500 (slog.Error)

**수락 기준**
- httptest:
  - `GET /usage` 누락 query → 기본값 (to=today, from=to-30d, group_by=day) 응답 200
  - `GET /usage?from=invalid` → 400
  - `GET /usage?group_by=year` → 400
  - `GET /usage?group_by=week&from=2026-01-01&to=2026-04-27` → 200, buckets[].bucket 이 ISO week 형식
  - `GET /usage/by-model` → cost_usd 내림차순
  - `GET /usage/limits` → max=0 일 때 percent=null (json 출력 검증)
- swag init 후 `/swagger/index.html` 에 3개 라우트 + 모든 DTO 예시 노출
- p95 < 200ms 검증 (10k seed task 로 로컬 측정 — 별도 벤치 권장)
- 커버리지 ≥ 85%

**Sub-agent**: `go-generator` + `go-tester`

### Step 7 — DI 조립 (`cmd/scheduled-dev-agent/main.go`)

**작업 내용**
- `usageRepo := repository.NewUsageRepository(db, sqlcQueries)`
- `usageUC := usecase.NewUsageUseCase(usageRepo, &cfg.Schedule, &cfg.Limits)`
- `budgetUC` 에 `usageRepo` + `slackClient` 주입 (cost warn 평가용)
- `usageHandler := api.NewUsageHandler(usageUC)`
- `server.go` 에서 라우트 등록

**수락 기준**
- `go build ./...` 통과
- `limits.daily_max_cost_usd=0` 부팅 → cost warn 비활성화 (worker 가 평가 호출은 하되 no-op)

**Sub-agent**: `go-modifier`

### Step 8 — 검증

```bash
sqlc generate
mockery --name=UsageRepository --dir=internal/domain --output=mocks
mockery --name=SlackClient     --dir=internal/slack  --output=mocks
go build ./...
go test -race -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | tail -1                          # ≥80%
go tool cover -func=coverage.out | grep -E '(usage_usecase|usage_repository|budget_usecase)'  # ≥85%
go vet ./...
golangci-lint run ./...
swag init -g cmd/scheduled-dev-agent/main.go -o docs
```

**수락 기준 (전체 게이트)**
- 모든 명령 0 exit
- `.claude/hooks/pre-push.sh` 통과
- swagger 에 `/usage`, `/usage/by-model`, `/usage/limits` + 모든 신규 DTO 노출
- 회귀: 기존 `/tasks`, `/modes/*` 응답 스키마 변경 0건 (필드 추가 없음 — usage-dashboard 는 별도 라우트)

---

## 2. PRD 수락 기준 매핑

| PRD Goal | 검증 |
|----------|------|
| G1. result 이벤트 → 6개 필드 100% 영속 | worker_test 의 모든 happy path 케이스 |
| G2. /usage p95 < 200ms (10k 기준) | sqlite seed 10k → 로컬 측정 + 단위 테스트 latency 가드 |
| G3. by-model 분해 | usage_handler_test + by-model 응답 검증 |
| G4. 80%/100% Slack warn 1회만 | budget_usecase_test 의 멱등 시나리오 |
| G5. 기존 게이트 회귀 0건 | scheduler/budget_gate 회귀 테스트 통과 |

| PRD User Story | 검증 |
|----------------|------|
| US-1 (영속화) | worker_test |
| US-2 (시계열 집계 + gap-fill) | usage_usecase_test gap-fill 케이스 |
| US-3 (by-model) | by-model 단위 + 핸들러 |
| US-4 (limits) | limits 단위 + max=0 케이스 |
| US-5 (Slack 경고) | budget_usecase_test 80/100% 멱등 |
| US-6 (Swagger) | swag init 산출물 검증 |

---

## 3. CLAUDE.md NEVER 체크리스트

- [ ] `domain/` 외부 패키지 import 금지 — Task 의 신규 필드는 모두 primitive
- [ ] raw SQL 금지 — `db/query/usage.sql` + sqlc generate
- [ ] 전역 var db 금지 — `cmd/main.go` DI 만
- [ ] `context.Background()` 핸들러 직접 사용 금지 — 모든 핸들러 `c.Request.Context()`
- [ ] swag 주석 없는 endpoint 추가 금지 — 3개 모두 godoc
- [ ] 기존 migration 수정 금지 — 000004 신규만
- [ ] 테스트 없이 UseCase 메서드 추가 금지 — Aggregate / ByModel / Limits / EvaluateCostWarn 모두 단위 테스트 동시 추가
- [ ] `db/sqlc/` 수동 수정 금지 — sqlc generate 만
- [ ] `panic()` 금지 — 모든 경로 에러 반환
- [ ] secret/PII 로그 금지 — cost·token 수치는 PII 아니지만 model_usage_json 원본은 로그 무덤프

---

## 4. OI / 후속 결정 (PRD §12 와 동기화)

- **OI-1**: `COALESCE(finished_at, created_at)` 채택 (sqlc 쿼리에 반영) — PRD v1 결정값
- **OI-2**: 정규화 시점은 100k 임계 도달 시 별도 PRD. v1 은 application-side scan 유지
- **OI-3**: `/usage/by-model` 별도 엔드포인트 채택. `?breakdown=model` 쿼리 옵션은 v1 미구현
- **OI-4**: 80%/100% 임계 고정. config 외부화는 v2
- **OI-5**: ISO week 통일 (`2026-W17` 형식, `BudgetCounters.WeeklyKey` 재사용)
- **OI-6**: dispatch 차단은 v2 — 본 PRD 는 경고만

---

## 5. 후속 수동 작업

```bash
# 마이그레이션 적용
go run ./cmd/scheduled-dev-agent migrate up   # 또는 사용 중인 migrate 명령

# config.example.yaml 에 limits 추가
limits:
  daily_max_cost_usd:  1.00
  weekly_max_cost_usd: 5.00

# README "Cost & Usage" 섹션 추가 — `/usage`, `/usage/by-model`, `/usage/limits` 사용 예시 포함
```
