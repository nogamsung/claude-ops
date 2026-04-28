# PRD — github-webhook

| 항목 | 값 |
|------|-----|
| 작성일 | 2026-04-27 |
| 상태 | draft |
| 스택 범위 | Go (단일 바이너리 — `scheduled-dev-agent` 에 추가) |
| 우선순위 | P1 |
| 작성자 | gs97ahn@gmail.com |
| 상위 PRD | [`./scheduled-dev-agent.md`](./scheduled-dev-agent.md) §8 OI-5 해소 |

---

## 1. 배경 (Background)

현재 `internal/github/poller.go` 는 `runtime.tick_interval` (기본 30s) 주기로 GitHub `Issues.ListByRepo` 를 호출해 새 이슈를 enqueue 한다. 이 방식은 **활성 시간대 이내에서도 이슈가 올라온 직후 최대 30초 + GitHub eventual consistency 지연** 이 task 시작까지 누적된다는 문제가 있다. 운영자가 라벨을 붙였는데도 즉각 반응하지 않는 사용자 경험은 "예약 자동화" 라기보다 "지연 자동화" 로 보인다.

이 기능은 **GitHub webhook 엔드포인트 (`POST /github/webhook`)** 를 노출해 이슈 이벤트를 즉시 수신·enqueue 하여 polling latency 를 30s → **< 2s (p95)** 로 단축한다. polling 은 **fallback 으로 그대로 유지** 한다 — 네트워크 단절·webhook delivery 실패·서비스 재시작 시 누락된 이벤트를 다음 tick 에서 회수하기 위함.

핵심 제약:
- **인증**: GitHub webhook secret 으로 HMAC-SHA256 서명 검증 (이미 `internal/slack/verify.go` 에 동일 패턴 존재 — 차용)
- **idempotency**: `X-GitHub-Delivery` UUID 를 키로 중복 처리 방지 (재전송·polling 과의 race)
- **활성 시간대 게이트**: webhook 으로 enqueue 되더라도 dispatch 는 기존 scheduler 의 active window + budget gate 를 그대로 통과해야 한다
- **레포 allowlist · 라벨 필터**: polling 과 동일 규칙 적용

## 2. 목표 (Goals)

- G1. 이슈에 `claude-ops` (config 의 `repo.labels` AND 매치) 라벨이 추가되면 **p95 < 2s 이내** task 가 `queued` 상태로 INSERT 됨
- G2. 동일 delivery (재전송) 또는 polling 과 webhook 이 같은 이슈를 동시에 발견해도 **Task row 는 정확히 1개만** 생성됨 (dedup)
- G3. webhook secret 검증 실패 / replay window (5분) 초과 / allowlist 바깥 레포 이벤트는 **401 또는 무시** 되며 task INSERT 0건
- G4. polling 은 그대로 동작 — webhook 이 죽거나 network 가 단절된 동안 누락된 이슈는 다음 polling tick 이 회수
- G5. webhook 처리 자체는 **active window 와 무관** 하게 즉시 enqueue (status=queued). 실제 dispatch 는 기존 scheduler 게이트가 결정

## 3. 비목표 (Non-goals)

- v1 은 **`issues` 이벤트만** 처리 (action: `opened`, `labeled`). `pull_request`, `push`, `issue_comment` 는 후순위
- GitHub App 방식 (JWT, installation token) — v1 은 **레포 webhook + shared secret** 만
- Webhook delivery 누락 자동 재요청 (GitHub Redelivery API) — polling 으로 자연 회수
- 외부 노출용 reverse proxy 설정 자동화 (운영자가 직접 nginx/Caddy/cloudflared 등으로 구성)
- 이벤트별 metric/대시보드 (slog 로그만)
- webhook URL 자체의 회전·교체 자동화

## 4. 대상 사용자 (Users)

| 페르소나 | 역할 | 목표 |
|----------|------|------|
| Solo developer | 홈서버에 서비스 운영, GitHub repo 관리자 | 라벨 단 이슈가 즉시 PR 까지 진행되길 원함 |
| Small team lead | 1~3 인 팀 테크 리드 | webhook 누락 대비 polling fallback 신뢰성 확인 |

상위 PRD 와 동일하게 **운영자 = GitHub repo 관리자 = Slack 채널 운영자** 단일 페르소나.

## 5. 유저 스토리 (User Stories)

| # | 스토리 | 수락 기준 |
|---|--------|-----------|
| US-1 | 운영자로서 이슈에 `claude-ops` 라벨을 추가하면 **즉시** task 가 큐잉되길 원한다 | 1) GitHub `issues` 이벤트 (action: `labeled`) 수신 → 라벨 추가 시점 → Task INSERT 까지 p95 < 2s <br> 2) 라벨 매치는 polling 과 동일 (config 의 `repo.labels` AND 조건) <br> 3) Task `task_type` 은 polling 과 동일한 `detectTaskType` 로직 적용 |
| US-2 | 운영자로서 새 이슈 (action: `opened`) 가 라벨과 함께 올라오면 즉시 큐잉되길 원한다 | 1) `issue.labels[]` 가 라벨 필터를 만족하면 INSERT <br> 2) 라벨 미매치면 무시 (응답 200 + 로그) <br> 3) `issue.pull_request != nil` 이면 무시 (PR 이벤트는 v1 범위 외) |
| US-3 | 운영자로서 webhook 가짜 호출로 task 가 생성되지 않길 원한다 | 1) `X-Hub-Signature-256` 헤더 누락 → 401 <br> 2) HMAC-SHA256 검증 실패 → 401 <br> 3) `X-GitHub-Delivery` 헤더 누락 → 400 <br> 4) GitHub 의 ping 이벤트 (`X-GitHub-Event: ping`) → 200 + 무시 |
| US-4 | 운영자로서 동일 이벤트 재전송 / polling 중복으로 task 가 두 번 생기지 않길 원한다 | 1) 동일 `X-GitHub-Delivery` UUID 가 5분 이내 재수신되면 무시 (응답 200) <br> 2) polling 과 webhook 이 같은 (repo, issue_number) 를 동시에 봐도 `tasks` row 는 1개 (`ExistsByRepoAndIssue` 재확인 + UNIQUE 제약) <br> 3) dedup 캐시는 in-memory + AppState 영속 (재시작 시에도 1시간 보존) |
| US-5 | 운영자로서 webhook 이 잠시 끊겨도 polling 으로 누락 이슈를 회수하길 원한다 | 1) webhook 503/network 단절 시 GitHub 가 재전송하지 못해도 다음 polling tick (≤ poll_interval) 이 동일 이슈를 발견해 INSERT <br> 2) polling 과 webhook 둘 다 INSERT 시도해도 US-4 의 dedup 으로 1개만 생성 <br> 3) 운영자가 webhook 비활성화하고 polling 만으로 운영 가능 (config 토글) |
| US-6 | 운영자로서 webhook 수신 로그를 사후에 확인하고 싶다 | 1) 모든 수신 webhook 은 slog JSON 로그로 기록: `{event, action, delivery_id, repo, issue, accepted/ignored, latency_ms}` <br> 2) PII (issue body) 는 로그에 남기지 않음 (title, number 만) <br> 3) `TaskEvent` 의 `kind=started` payload 에 `source: "webhook"` 또는 `"polling"` 추가하여 사후 추적 |
| US-7 | 운영자로서 webhook 자체에 의해 active window 가 무시되지 않길 원한다 | 1) webhook 으로 enqueue 된 task 도 scheduler 의 window gate 를 통과해야 dispatch 됨 (window 밖이면 status=queued 인 채로 대기) <br> 2) full mode 토글·budget gate 도 그대로 enforced <br> 3) e2e 테스트: window 밖 webhook 수신 → INSERT 됨 → claude exec 호출 0회 검증 |

## 6. 핵심 플로우 (Key Flows)

### 6.1 행복 경로 — webhook 즉시 수신

```
1. GitHub 이슈에 'claude-ops' 라벨 추가
2. GitHub → POST /github/webhook
   헤더: X-Hub-Signature-256, X-GitHub-Delivery, X-GitHub-Event: issues
   바디: { action: "labeled", issue: {...}, repository: {...}, label: {...} }
3. 핸들러: raw body 읽기 → HMAC-SHA256 검증
4. event=ping → 200 즉시 반환
5. event=issues, action ∈ {opened, labeled}: 처리
   - allowlist 체크: repository.full_name 이 config.github.repos[].name 에 있는지
   - PR 이벤트 필터: issue.pull_request 가 있으면 무시
   - 라벨 매치: hasAllLabels(issue, repo.labels)
   - delivery_id dedup: cache.has(delivery_id) → 무시
   - ExistsByRepoAndIssue → 이미 있으면 무시 (polling race)
6. Task INSERT (status=queued, source=webhook 이벤트 payload 에 기록)
7. 200 응답 (latency_ms 로그)
8. 이후 scheduler tick → window/budget gate 통과 시 dispatch (기존 흐름)
```

### 6.2 예외 경로

- **서명 헤더 누락 / 검증 실패**: 401 + slog Warn (`signature_verification_failed`). body 무시.
- **timestamp replay (5분 초과)**: GitHub webhook 은 timestamp 헤더가 없으므로 **delivery_id 캐시** 가 replay 방지 책임. 동일 delivery 5분 이내 재수신 시 200 + ignore.
  > **OI 결정 (2026-04-27)**: Slack 의 `X-Slack-Request-Timestamp` 와 달리 GitHub 는 공식 timestamp 헤더를 제공하지 않는다. 따라서 "5분 replay window" 는 `webhook_dedup` 의 delivery-ID TTL(기본 5분)로 구현된다. `webhook_verifier.go` 상단 주석에도 동일 내용 기록됨. <!-- OI-resolved: replay window via dedup TTL -->
- **allowlist 바깥 레포**: 200 + `ignored: not_in_allowlist` 로그. 401 이 아닌 200 으로 응답해야 GitHub 가 재시도하지 않음.
- **PR 이벤트** (`issue.pull_request != nil`): 200 + ignore.
- **라벨 미매치** (`labeled` 이벤트지만 필터 라벨이 빠진 경우): 200 + ignore.
- **JSON 파싱 실패**: 400 + slog Error.
- **DB INSERT 실패** (`ExistsByRepoAndIssue` 또는 `Create`): 500 — GitHub 가 재시도 → 다음 시도에서 dedup 으로 자연 해소.
- **action 이 `unlabeled`/`closed`/`assigned` 등 v1 외**: 200 + ignore.
- **content-type 불일치** (`application/json` 외): 400.

### 6.3 polling fallback 플로우 (변경 없음)

기존 `internal/github/poller.go` 는 그대로 동작. webhook 활성 여부와 무관하게 `runtime.tick_interval` 마다 호출. webhook 으로 이미 INSERT 된 이슈는 `ExistsByRepoAndIssue == true` 로 자연 스킵.

운영자가 webhook 만 사용하고 싶다면 config 의 `github.poll_interval` 을 길게 (예: 10m) 설정하여 fallback 빈도만 낮춤. 단, 완전 비활성화는 권장하지 않음 (R2 참조).

## 7. 데이터 모델 (요약)

신규 테이블 없음. 기존 `Task`, `TaskEvent`, `AppState` 재사용.

```
Task (변경 없음)
TaskEvent.payload_json: { source: "webhook"|"polling", delivery_id?: "uuid" }   # ADDED: 사후 추적용
AppState
    key="webhook_dedup"   value={ deliveries: { "uuid": expires_at_unix, ... } }   # NEW
        — 메모리 캐시 + 1시간 단위 flush. 재시작 직후 5분 grace period 동안만 영속본 사용
```

dedup 캐시 구현:
- in-memory: `sync.Map[deliveryID]expiresAt` — 5분 TTL
- AppState 영속: 1분 단위 flush 로 재시작 후에도 5분 grace period 보장 (write 비용 < dedup 정확성)
- v1 은 단일 머신·단일 프로세스 전제이므로 분산 dedup 불필요

`tasks` 테이블에는 **UNIQUE INDEX (repo_full_name, issue_number) WHERE status IN ('queued', 'running')** 제약을 추가 검토 — 이미 `ExistsByRepoAndIssue` 가 있으므로 v1 은 우선 application-layer dedup 만, race 발생 시 v1.1 에서 DB 제약 추가.

## 8. API 계약 (요약)

상위 PRD §8 의 placeholder (`POST /github/webhook`) 를 실제로 구현.

```
POST /github/webhook
  Headers:
    Content-Type:        application/json
    X-Hub-Signature-256: sha256=<hex>      # GitHub HMAC-SHA256 서명 (필수)
    X-GitHub-Delivery:   <uuid>            # delivery 식별자 (필수, dedup 키)
    X-GitHub-Event:      issues|ping|...   # 이벤트 종류 (필수)
  Body:                  GitHub IssueEvent payload (https://docs.github.com/webhooks/webhook-events-and-payloads#issues)

  Responses:
    200 OK              { accepted: bool, reason?: "queued"|"ignored:..."|"duplicate" }
    400 Bad Request     ErrorResponse — 헤더 누락 또는 JSON 파싱 실패
    401 Unauthorized    ErrorResponse — 서명 검증 실패
    500 Internal Error  ErrorResponse — DB 오류 (GitHub 가 재시도)
```

응답 DTO (swag godoc 필수):

```go
// WebhookResponse is the response for POST /github/webhook.
type WebhookResponse struct {
    Accepted bool   `json:"accepted" example:"true"`
    Reason   string `json:"reason,omitempty" example:"queued"`
    TaskID   string `json:"task_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000"`
}
```

서명 검증 알고리즘 (GitHub 공식):

```
expected = "sha256=" + hex(HMAC_SHA256(secret, raw_body))
hmac.Equal(expected, request.Header["X-Hub-Signature-256"])
```

**기존 `internal/slack/verify.go` 와의 차이점**:
- Slack 은 `v0=<hex>` + timestamp 별도 헤더, GitHub 는 `sha256=<hex>` 단일 헤더
- timestamp replay 방어가 GitHub 헤더에 없음 → **delivery_id 캐시** 로 대체
- HMAC 알고리즘은 동일 (`crypto/hmac` + `crypto/sha256`)

## 9. 비기능 요구사항 (Non-functional)

| 항목 | 요구 |
|------|------|
| 성능 | webhook → Task INSERT p95 < 2s (network 지연 제외 < 200ms 처리) |
| 보안 | secret 은 환경변수 (`GITHUB_WEBHOOK_SECRET`), config.yaml 금지. 검증 실패는 모두 401 |
| 보안 | constant-time 비교 (`hmac.Equal`) — timing attack 방지 |
| Idempotency | 동일 delivery_id 재수신 → no-op + 200. 메모리 + AppState dual layer |
| 관측성 | slog JSON: `{event, action, delivery_id, repo, issue, accepted, reason, latency_ms}` |
| 신뢰성 | webhook 다운 / 검증 실패 시에도 polling 이 fallback. e2e 테스트로 검증 |
| 활성 시간 게이트 | webhook 은 enqueue 만, dispatch 는 기존 window/budget gate 통과 필수 |
| 테스트 커버리지 | 80% 이상 (Go CLAUDE.md 게이트) — webhook handler · 검증·dedup 은 **85% 이상** |
| lint | `golangci-lint run ./...` 통과 필수 |
| 문서 | swag godoc 필수 — `/swagger/index.html` 에 `POST /github/webhook` 노출 |

## 10. 의존성 / 리스크

**의존성**
- 기존 `internal/slack/verify.go` 의 HMAC 패턴 (참고용 — 코드 재사용보다는 패턴 차용)
- 기존 `internal/github/poller.go` 의 `hasAllLabels` / `detectTaskType` 함수 — webhook handler 도 같은 함수 호출하도록 export 또는 분리 필요
- 외부 노출 — 운영자가 reverse proxy (nginx, Caddy, cloudflared tunnel 등) 로 `/github/webhook` 만 공개. v1 은 문서화만, 자동 구성 안 함
- GitHub webhook 등록 — 운영자가 GitHub repo Settings → Webhooks 에서 수동 등록 (Payload URL, Secret, Events: Issues)

**리스크**

- **R1 (High)**: 외부 공개 webhook 엔드포인트 → DDoS / 봇 스캔. 완화: rate limit middleware (IP 별 60req/min), 검증 실패 시 빠른 401, body 크기 제한 (1MB)
- **R2 (Med)**: webhook 만 신뢰하고 polling 비활성화 → GitHub delivery 누락 시 영구 미처리. 완화: README 와 config.example.yaml 에 "polling 은 항상 활성화 권장" 명시. v1 은 polling 비활성화 토글을 의도적으로 제공하지 않음 (poll_interval 만 조절 가능)
- **R3 (Med)**: dedup 캐시 누락 (재시작 직후 5분 grace period 끝나기 전 재시작 반복) → 동일 이슈 race. 완화: `ExistsByRepoAndIssue` 가 2차 방어선. v1.1 에서 DB UNIQUE 제약 추가 검토
- **R4 (Low)**: 서명 검증에서 raw body 가 아닌 파싱된 body 를 검증해 mismatch → Gin 의 `c.Request.Body` 를 `io.ReadAll` 한 후 검증해야 함. 기존 Slack handler 와 동일 패턴
- **R5 (Low)**: GitHub 이 webhook 을 SHA-1 (`X-Hub-Signature`) 으로도 보낼 수 있음 → v1 은 SHA-256 만 지원, SHA-1 은 무시 (GitHub 가 둘 다 보내므로 안전)

## 11. 설계 결정 기록 (Design Decisions)

| 결정 | 내용 | 일자 |
|------|------|------|
| Replay window 구현 방식 | GitHub 공식 timestamp 헤더 부재로 5분 replay window 는 `webhook_dedup` delivery-ID TTL 로 대체. `webhook_verifier.go` 주석 + §6.2 에 명시. | 2026-04-27 |
| Secret 미설정 시 라우트 미등록 | `GITHUB_WEBHOOK_SECRET` 미설정 시 `/github/webhook` 라우트 자체를 등록하지 않아 엔드포인트 존재 노출 방지 (503 → 404). `api.NewRouter` + `cmd/main.go` 에 반영. | 2026-04-27 |
| dedup TOCTOU race 제거 | `sync.Map LoadOrStore → TTL check → Store` 패턴을 `sync.Mutex + map[string]time.Time` 으로 교체. `CheckAndAdd` + `evictExpired` 전체를 단일 lock 내에서 수행. | 2026-04-27 |

## 11. 범위 외 (Out of Scope)

- `pull_request`, `push`, `issue_comment` 이벤트 (v2)
- GitHub App 인증 (v2)
- Redelivery API 자동 호출 (polling 으로 충분)
- 멀티 webhook (organization-level webhook) 지원 — v1 은 repo-level 단일 secret
- webhook 수신 메트릭 대시보드 (slog 로그만)
- 이슈 closing/unlabeled 이벤트로 queued task 자동 cancel — v1 은 enqueue only

## 12. 오픈 이슈 (Open Questions)

- [ ] **OI-1**: 외부 노출 방식 권장 가이드 — Cloudflare Tunnel vs nginx + Let's Encrypt vs ngrok. 홈서버 시나리오에서 어느 것을 README 기본 가이드로 채택할지. 보안 vs 설치 난이도 trade-off 결정 필요
- [ ] **OI-2**: webhook secret 은 단일 secret 인가 multiple secret rotation 지원할 것인가. v1 은 단일이지만, GitHub Webhooks 는 multiple secret 을 지원하지 않으므로 사실상 결정됨 — rotation 은 운영자가 GitHub 측 secret + 환경변수 동시 교체 + 무중단 재시작 책임. README 에 명시할 것
- [ ] **OI-3**: dedup 캐시의 in-memory + AppState dual layer 가 v1 에 과한가? 단일 머신·단일 프로세스이므로 in-memory 만으로 충분할 수도 있음. 재시작 직후 GitHub 재전송 빈도 측정 후 결정 — v1 초안은 in-memory 만, AppState 영속은 OI 로 보류
- [ ] **OI-4**: webhook 수신 시 `TaskEvent` 에 `kind=webhook_received` 같은 신규 EventKind 를 추가할지, 기존 `started` 의 payload 만 확장할지. 후자가 단순하지만 사후 분석 시 별도 kind 가 더 직관적 — 결정 필요

---

## 참고 — 영향받는 기존 파일

- `internal/api/server.go` — 새 라우트 등록
- `internal/api/dto.go` — `WebhookResponse` 추가
- `internal/api/errors.go` — 변경 없음 (신규 도메인 에러 없음)
- `internal/config/config.go` — `Env.GitHubWebhookSecret` 필드 + `LoadEnv` 에 환경변수 매핑
- `internal/config/validate.go` — `ValidateEnv` 에서 webhook secret 비어 있을 때 경고만 (필수 아님 — webhook 비활성화 모드 허용)
- `internal/github/poller.go` — `hasAllLabels`, `detectTaskType` 을 패키지 외부에서도 호출 가능하도록 export (또는 webhook 코드를 같은 패키지에 두기)
- `cmd/scheduled-dev-agent/main.go` — webhook handler DI 조립

신규 파일은 `./github-webhook/go.md` 의 체크리스트 참조.

---

> 단일 스택 프로젝트이므로 역할별 분할 없이 **단일 구현 프롬프트** ([`./github-webhook/go.md`](./github-webhook/go.md)) 로 전달합니다.
