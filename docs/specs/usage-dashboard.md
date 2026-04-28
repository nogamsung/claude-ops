# PRD — usage-dashboard

| 항목 | 값 |
|------|-----|
| 작성일 | 2026-04-27 |
| 상태 | draft |
| 스택 범위 | Go (단일 바이너리 — scheduled-dev-agent 확장) |
| 우선순위 | P1 |
| 작성자 | gs97ahn@gmail.com |
| 관련 PRD | [`./scheduled-dev-agent.md`](./scheduled-dev-agent.md) §7 데이터 모델, §10 R6, §12 OI-1 |

---

## 1. 배경 (Background)

`scheduled-dev-agent` 는 이미 Claude CLI 의 `stream-json` 출력에서 `result.total_cost_usd`, `result.usage.{input,output,cache_*}`, `result.modelUsage[model].{inputTokens,outputTokens,costUSD,...}` 까지 NDJSON 으로 파싱하고 있다 (§12 OI-1, `internal/claude/stream/events.go`). 하지만 worker 는 이 중 `EstimatedInputTokens` / `EstimatedOutputTokens` 두 컬럼만 Task 에 저장하고 비용 (`total_cost_usd`) 과 모델 단위 분해 (`modelUsage`) 는 휘발 중이다.

운영자는 "이번 주 plan 으로 얼마를 썼는지", "어떤 모델이 cost 의 대부분을 차지하는지", "남은 한도 대비 얼마나 위험한지" 를 알 수 없다. 현재 한도 게이트는 task **개수** 캡 (US-11) 과 CLI rate-limit 신호 (US-12) 만으로 동작하므로, 비용·토큰 단위의 사전 throttling 이 불가능하다.

이 기능은 (1) Task 테이블에 cost·model 분해를 영속화하고, (2) 일/주/월 집계 API 를 제공하며, (3) 비용 한도 임계치에 도달하면 Slack warn 을 발송한다. v1 은 **조회 + 경고** 까지이며 한도 도달 시 dispatch 차단은 후속 이터레이션에서 검토.

## 2. 목표 (Goals)

- G1. Claude CLI `result` 이벤트의 `total_cost_usd` 와 `modelUsage` 가 **task 종료 시점 100%** Task 행에 영속화 (성공·실패·취소 무관, 결과 이벤트 수신된 모든 task)
- G2. `GET /usage?from=&to=&group_by=day|week|month` 가 토큰·비용·task 수 집계를 반환 (p95 < 200ms, task 수 ≤ 10k 기준)
- G3. 모델 단위 분해 조회 지원 — `GET /usage/by-model` 또는 `group_by` 와 직교하는 `breakdown=model` 옵션
- G4. 일일/주간 cost 한도 (`limits.daily_max_cost_usd`, `limits.weekly_max_cost_usd`) 에 도달 시 Slack `:warning:` 메시지 1회 발송 (중복 발송 방지 — 동일 버킷에서 한 번만)
- G5. 기존 task 카운트 캡·rate-limit block 게이트 동작에 영향 없음 (회귀 0건)

## 3. 비목표 (Non-goals)

- 비용 한도 도달 시 dispatch **차단** (v1 은 경고만, v2 검토)
- 웹 UI 대시보드 (HTTP JSON 응답만)
- Slack 일일/주간 비용 요약 cron (`/usage` 호출로 갈음, v2 후보)
- Anthropic API 키 기반 호출 비용 (대상 외 — CLI 세션 비용만)
- 기존 task 의 retroactive backfill — migration 후 신규 task 부터 적용
- task 단위 개별 cost 수정 / 조정 API
- 다중 사용자 / per-user 분해 (단일 사용자 전제)

## 4. 대상 사용자 (Users)

| 페르소나 | 역할 | 목표 |
|----------|------|------|
| Solo developer (P1) | scheduled-dev-agent 운영자 | 이번 주 cost / 토큰 사용 추세 파악, plan 한도 도달 전 throttle 결정 |
| Small team lead (P2) | 1~3인 팀 리드 | 어떤 모델 (Sonnet vs Opus 등) 이 비용의 대부분을 차지하는지 파악해 프롬프트 튜닝 |

권한 모델: 기존과 동일 — 로컬 bind (`127.0.0.1:8787`) HTTP API. 인증 없음 (v1).

## 5. 유저 스토리 (User Stories)

| # | 스토리 | 수락 기준 |
|---|--------|-----------|
| US-1 | 운영자로서 task 가 끝나면 그 task 의 **총 비용·토큰·모델별 분해** 가 DB 에 남아있어야 한다 | 1) Task 테이블에 `cost_usd REAL`, `model_usage_json TEXT`, `total_input_tokens`, `total_output_tokens`, `cache_creation_input_tokens`, `cache_read_input_tokens` 컬럼이 추가됨 <br> 2) Worker 가 `result` 이벤트 수신 후 Task.Update 시 위 6개 필드를 채움 (없으면 0 / `{}`) <br> 3) `result` 이벤트 미수신 (crash 등) 시에는 0 / `{}` 유지하고 task 는 failed 처리 (기존 동작) |
| US-2 | 운영자로서 `GET /usage?from=&to=&group_by=day` 호출로 **일별 사용량 합계** 를 받고 싶다 | 1) `from`, `to` 는 ISO date (`YYYY-MM-DD`), 누락 시 기본값 `to=today`, `from=to-30d` <br> 2) `group_by` ∈ {`day`, `week`, `month`} — 누락 시 `day` <br> 3) 응답: `buckets[]` 각 항목에 `bucket`(예 `2026-04-27`), `task_count`, `cost_usd`, `input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_creation_tokens` <br> 4) status=done 인 task 만 집계 (failed/cancelled 는 제외, 단 cost 가 발생했으면 `usage.failed_cost_usd` 별도 필드로 합계 노출) <br> 5) 빈 버킷도 0 으로 채워 응답 (gap-fill — `2026-W17` 같은 키는 빠뜨리지 않음) |
| US-3 | 운영자로서 **어떤 모델에 얼마나 썼는지** 보고 싶다 | 1) `GET /usage/by-model?from=&to=` → `models[]` 각 항목에 `model_id`, `cost_usd`, `input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_creation_tokens`, `task_count` <br> 2) 또는 `GET /usage?breakdown=model` 도 동일 결과 반환 (선택 — 둘 중 한 형태로 구현) <br> 3) `cost_usd` 내림차순 정렬 <br> 4) `model_id` 가 비어있는 항목 (CLI 출력 누락) 은 `"unknown"` 으로 합산 |
| US-4 | 운영자로서 **현재 누적 cost** 와 **설정한 일/주 cost 한도 대비 사용률** 을 한 번에 보고 싶다 | 1) `GET /usage/limits` → `daily.{count_usd, max_usd, percent, date}`, `weekly.{count_usd, max_usd, percent, week}` <br> 2) `max_usd` 는 `config.yaml` 의 `limits.daily_max_cost_usd` / `weekly_max_cost_usd`. 미설정 (0) 이면 `max_usd: 0`, `percent: null` <br> 3) `percent` 는 `count_usd / max_usd * 100` (소수점 1자리), `>= 100` 이면 cap 표시 <br> 4) 카운터 리셋 키는 기존 task counter 와 동일 (`reset_tz` 자정, `week_starts_on` 0시) |
| US-5 | 운영자로서 cost 한도 직전에 **Slack 경고** 를 받고 싶다 | 1) 매 task 종료 시 (Worker 내) 누적 daily/weekly cost 와 한도 비교 <br> 2) `daily.percent` 가 `[80, 100)` 구간 진입 시 `:warning: Daily cost {X}% used (${cur}/${max})` Slack 발송, 동일 일자에서 1회만 <br> 3) `daily.percent >= 100` 시 `:rotating_light: Daily cost cap reached` 발송, 동일 일자에서 1회만 <br> 4) weekly 도 동일 (80%, 100% 두 단계) <br> 5) 발송 여부는 AppState `cost_warn_state` 에 영속 (재시작 후 재발송 안 함) <br> 6) 한도가 0 (미설정) 이면 경고 발송 안 함 |
| US-6 | 운영자로서 Swagger UI 에서 **새 엔드포인트** 를 즉시 호출해보고 싶다 | 1) `/usage`, `/usage/by-model`, `/usage/limits` 모두 swag godoc (`@Summary`, `@Tags`, `@Param`, `@Success`, `@Failure`, `@Router`) 작성 <br> 2) `swag init -g cmd/scheduled-dev-agent/main.go -o docs` 재생성 후 `/swagger/index.html` 에서 노출 <br> 3) 응답 DTO 의 모든 필드에 `example:"..."` 태그 |

## 6. 핵심 플로우 (Key Flows)

### 6.1 행복 경로 — task 종료 → 영속화 → 집계 조회

```
1. Worker.RunTask 가 Claude Runner 호출
2. Runner 가 result 이벤트의 TotalCostUSD, ModelUsage, Usage 를 RunResult 에 채워 반환
3. Worker.applyRunUsage 가 task 에 cost_usd / model_usage_json / 4개 토큰 컬럼 적용
4. TaskRepo.Update 로 영속화 (markDone 직전)
5. Worker.checkCostThreshold 가 BudgetUseCase.SnapshotCost 호출 → 80% / 100% 진입 시 Slack warn
6. 운영자가 GET /usage?group_by=week 호출
7. UsageUseCase 가 sqlc 의 SumUsageByBucket(from, to, bucket_kind) 실행
8. 결과를 UsageResponse DTO 로 매핑하여 반환
```

### 6.2 예외 경로

- **`result` 이벤트 미수신 (crash, kill, timeout)**: cost_usd=0, model_usage_json='{}' 로 task 저장. `/usage` 집계에서는 자동으로 0 기여.
- **`modelUsage` 누락 또는 비어있음**: `model_usage_json='{}'` 로 저장. `GET /usage/by-model` 에서 해당 task 는 합계에 영향 없음.
- **`from > to` 또는 잘못된 `group_by`**: 400 + `{error: "invalid group_by"}`.
- **`from`/`to` 가 너무 넓음** (>1년): 기본 limit 적용 후 `warning` 헤더로 안내. 또는 365일 초과 시 400.
- **Slack 경고 발송 실패**: `slog.Warn` 로깅만, task 종료 자체는 막지 않음. 중복 발송 방지 플래그는 발송 시도 후 set (실패해도 set — 재시도 폭증 방지).
- **한도 미설정** (`daily_max_cost_usd=0`): warn 비활성화, `/usage/limits` 의 `max_usd=0`, `percent=null`.
- **시계열 갭**: 빈 버킷도 0 으로 채움 (frontend gap-fill 부담 제거).

## 7. 데이터 모델 (요약)

기존 §7 (scheduled-dev-agent PRD) Task 스키마에 6개 컬럼 추가. 새 마이그레이션 파일로만 추가 (기존 migration 수정 금지 — CLAUDE.md NEVER).

```
Task (..., 기존 컬럼 ...,
      cost_usd REAL NOT NULL DEFAULT 0,                        -- result.total_cost_usd
      total_input_tokens INTEGER NOT NULL DEFAULT 0,           -- result.usage.input_tokens
      total_output_tokens INTEGER NOT NULL DEFAULT 0,          -- result.usage.output_tokens
      cache_creation_input_tokens INTEGER NOT NULL DEFAULT 0,  -- result.usage.cache_creation_input_tokens
      cache_read_input_tokens INTEGER NOT NULL DEFAULT 0,      -- result.usage.cache_read_input_tokens
      model_usage_json TEXT NOT NULL DEFAULT '{}'              -- result.modelUsage 원본 JSON
      )
```

기존 `estimated_input_tokens` / `estimated_output_tokens` 는 **유지** (호환성). 신규 컬럼은 result 이벤트 기반의 **권위있는** 값. 두 컬럼 셋 모두 채우되, 집계는 신규 컬럼 사용.

`AppState` 에 키 추가:

```
key="cost_warn_state",  value={daily_key:"2026-04-27", daily_warned_80:true, daily_warned_100:false,
                                weekly_key:"2026-W17", weekly_warned_80:false, weekly_warned_100:false}
```

인덱스: `tasks(created_at)` 는 이미 존재. 집계 쿼리는 `created_at` 또는 `finished_at` 범위 + `status='done'` 필터. 추가 인덱스 불필요 (10k 기준 OK), 필요 시 `idx_tasks_status_finished_at` 추가 검토 (오픈 이슈로).

## 8. API 계약 (요약)

기존 Gin 서버에 라우트 3개 추가. 모두 로컬 bind, 인증 없음 (v1).

```
GET /usage?from=&to=&group_by=day|week|month     # 시계열 집계
GET /usage/by-model?from=&to=                    # 모델 단위 합계
GET /usage/limits                                # 현재 누적 vs 한도
```

**`GET /usage` 응답 (예시)**:

```json
{
  "from": "2026-03-28",
  "to": "2026-04-27",
  "group_by": "day",
  "buckets": [
    {
      "bucket": "2026-04-27",
      "task_count": 4,
      "cost_usd": 1.23,
      "input_tokens": 12345,
      "output_tokens": 6789,
      "cache_read_tokens": 100,
      "cache_creation_tokens": 50,
      "failed_cost_usd": 0.0
    }
  ],
  "totals": {
    "task_count": 80,
    "cost_usd": 24.50,
    "input_tokens": 200000,
    "output_tokens": 80000,
    "cache_read_tokens": 5000,
    "cache_creation_tokens": 2000,
    "failed_cost_usd": 0.40
  }
}
```

**`GET /usage/by-model` 응답 (예시)**:

```json
{
  "from": "2026-03-28",
  "to": "2026-04-27",
  "models": [
    {
      "model_id": "claude-sonnet-4-5",
      "task_count": 60,
      "cost_usd": 18.20,
      "input_tokens": 150000,
      "output_tokens": 60000,
      "cache_read_tokens": 4000,
      "cache_creation_tokens": 1500
    },
    { "model_id": "claude-opus-4-5", "task_count": 20, "cost_usd": 6.30, "...": "..." }
  ]
}
```

**`GET /usage/limits` 응답 (예시)**:

```json
{
  "daily":  { "count_usd": 0.85, "max_usd": 1.00, "percent": 85.0, "date": "2026-04-27" },
  "weekly": { "count_usd": 4.20, "max_usd": 5.00, "percent": 84.0, "week": "2026-W17" }
}
```

## 9. 비기능 요구사항 (Non-functional)

| 항목 | 요구 |
|------|------|
| 성능 | `/usage` p95 < 200ms (task 수 ≤ 10k). sqlc 생성 쿼리 사용, raw SQL 금지 |
| 정확도 | 집계 결과는 동일 기간 task 행 합계와 ±0.001 USD 일치 |
| 일관성 | task 종료 → DB 반영까지 동일 트랜잭션 (Update + 카운터 += 1 은 분리되어도 OK, 양쪽 모두 멱등) |
| 보안 | 비용·토큰 수치는 PII 아님 — 평문 저장 OK. 로컬 bind 기본 |
| 테스트 커버리지 | 80% 이상 (CLAUDE.md 게이트). usage 집계 패키지 (sqlc 매핑·gap-fill·warn 게이트) **85% 이상** |
| lint | `golangci-lint run ./...` 통과 |
| 문서 | swag godoc + `/swagger/index.html` 갱신 |
| 호환성 | 기존 `/tasks`, `/modes/*` 응답 스키마 변경 없음 (필드 추가만 가능) |
| Migration | 단방향 추가 + down 에서 컬럼 drop. 기존 `000001_create_tasks_table.up.sql` 수정 금지 |

## 10. 의존성 / 리스크

**의존성**
- 기존 `internal/claude/stream` 의 `ResultEvent.TotalCostUSD`, `ModelUsage` 파싱 (이미 구현됨)
- `internal/usecase/budget_usecase.go` 의 `BudgetCounters` 키 산출 로직 (`DailyKey`, `WeeklyKey`) — cost 카운터 키 산출에 재사용
- `internal/scheduler/budget_gate.go` 의 `RolloverCounters` — cost 버킷에도 동일 의미로 적용
- `internal/slack/client.go` — warn 메시지 발송용 새 메서드 (`NotifyCostWarning`) 추가
- sqlc CLI (이미 설치됨, `sqlc.yaml` 존재)

**리스크**
- **R1 (Med)**: `result` 이벤트의 `total_cost_usd` 가 CLI 버전 업그레이드로 누락되거나 단위가 바뀌면 0 누적 → §10 R1 의 stream 파서 fallback 정책으로 흡수. 0 USD 인 task 는 모니터링 로그로 감지 (slog.Info "task done with zero cost").
- **R2 (Med)**: `model_usage_json` 을 TEXT 로 저장하면 by-model 집계 시 N+1 또는 application-side scan 이 됨 → 10k 기준은 문제 없으나, 향후 100k 단위 되면 별도 `task_model_usages` 테이블 정규화 필요. v1 는 application-side group-by 로 충분 (오픈 이슈).
- **R3 (Low)**: cost 한도 임계 80% 진입 후 task 가 한 번에 90% → 100% 로 점프하면 80% warn 은 발송하되 100% warn 도 별도 발송 — 두 단계가 독립이라 OK. 단, `[80,100)` 진입 검사가 "이전 percent < 80 && 현재 >= 80" 가 아니라 단순 `>= 80` 이면 매 task 마다 발송될 수 있음 → AppState `daily_warned_80` 플래그로 멱등화 필수.
- **R4 (Low)**: 빈 버킷 gap-fill 시 timezone 처리 — `reset_tz` 기준으로 일/주/월 키 산출. UTC 기준이면 한국 시간 자정 경계에서 어긋남 → 기존 `BudgetCounters` 의 키 산출 로직과 동일하게 `reset_tz` 적용.
- **R5 (Low)**: failed_cost_usd 가 done 합계와 별도라는 것을 운영자가 모르면 합계 불일치로 오해 → 응답 DTO 의 `failed_cost_usd` 필드와 swag description 으로 명시.

## 11. 범위 외 (Out of Scope)

- cost 한도 도달 시 dispatch 자동 차단 (v2 검토 — task count cap 처럼 budget gate 에 cost 차원 추가)
- per-repo / per-issue cost 분해 (필드는 이미 task.repo_full_name 으로 가능, API 만 미제공)
- 일일 cost 요약 Slack cron (v2)
- cost 예측 / 추세 그래프
- cost export (CSV / JSON dump) — `/usage` JSON 응답으로 갈음 가능
- 기존 task 의 retroactive cost backfill
- Anthropic API 키 호출 비용 (구독 세션만 대상)

## 12. 오픈 이슈 (Open Questions)

- [ ] **OI-1**: 집계 기준 시각으로 `created_at` 을 쓸지 `finished_at` 을 쓸지. 운영자 직관은 "끝난 시각" 이지만 finished_at 이 NULL 인 running/queued 도 있어 일관성 떨어짐. v1 초안: **`COALESCE(finished_at, created_at)`** 로 산출. 결정 필요.
- [ ] **OI-2**: `model_usage_json` 정규화 시점 — v1 은 TEXT 로 두고 application-side scan, 10k 임계 도달 시 `task_model_usages(task_id, model_id, ...)` 테이블 도입. 임계치 결정 필요 (50k? 100k?).
- [ ] **OI-3**: `/usage/by-model` 의 별도 엔드포인트 vs `/usage?breakdown=model` — 응답 스키마가 다르므로 v1 은 **별도 엔드포인트** 권장. 클라이언트 사용성 검증 필요.
- [ ] **OI-4**: cost warn 임계 (80%) 를 `config.yaml` 로 노출할지 고정할지. v1 초안: **고정 80% / 100%**. 운영자 피드백 후 외부화.
- [ ] **OI-5**: weekly 키 형식 — ISO week (`2026-W17`) vs `WEEK_OF_YEAR` 정수. 기존 BudgetCounters 가 ISO week 사용 → **ISO week 통일**.
- [ ] **OI-6**: 한도 도달 후 dispatch 차단 (BudgetGate 에 cost reason 추가) 을 v2 로 미루는 게 맞는지. CLI rate-limit block 이 사실상 wall hit 시 차단하므로 cost 차단은 중복 보험 — 우선순위 낮춤.

---

## 참고 — 영향 받는 파일 (Go 단일 스택)

PRD 단계에서는 개략만. 상세 task 분할은 `./usage-dashboard/go.md` 참조.

```
migrations/
  000004_add_cost_columns_to_tasks.{up,down}.sql      # 신규
db/query/
  usage.sql                                            # 신규 — sqlc 집계 쿼리
internal/
  domain/task.go                                       # Task 구조체에 6개 필드 추가
  domain/repository.go                                 # UsageRepository 인터페이스 신규
  repository/task_repository.go                        # gormTask 매핑 6개 필드 추가
  repository/usage_repository.go                       # 신규 — sqlc-backed 구현체
  usecase/usage_usecase.go                             # 신규 — gap-fill, by-model, limits
  usecase/budget_usecase.go                            # 변경 — cost warn state 영속
  scheduler/worker.go                                  # 변경 — applyRunUsage 확장 + cost warn 트리거
  claude/runner.go                                     # 변경 — RunResult 에 ModelUsage 포함
  claude/stream/parser.go                              # 변경 (소량) — Signal.Result 통과
  api/dto.go                                           # UsageResponse, UsageBucket, ModelUsage* 추가
  api/usage_handler.go                                 # 신규 — 3개 엔드포인트
  api/server.go                                        # 변경 — 라우트 등록
  config/config.go                                     # 변경 — limits.daily_max_cost_usd / weekly_max_cost_usd
  slack/client.go                                      # 변경 — NotifyCostWarning
docs/
  swagger/                                             # swag init 재생성 산출물
```

단일 스택이므로 역할별 분할 없이 **단일 구현 프롬프트** (`./usage-dashboard/go.md`) 로 전달합니다.
