# PRD — parallel-tasks

| 항목 | 값 |
|------|-----|
| 작성일 | 2026-04-27 |
| 상태 | draft |
| 스택 범위 | Go (단일 바이너리 — `scheduled-dev-agent` 의 scheduler/worker 확장) |
| 우선순위 | P1 |
| 작성자 | gs97ahn@gmail.com |
| 상위 PRD | [`./scheduled-dev-agent.md`](./scheduled-dev-agent.md) §3 비목표 / §10 R3 / §12 OI-2 해소 |

---

## 1. 배경

`scheduled-dev-agent` v1 은 §3 에서 "복수 Claude 세션 병렬 실행" 을 명시적으로 비목표로 두고, 세마포어 1개 (`scheduler.sem = make(chan struct{}, 1)`) 로 worker 를 직렬화해 동작한다. 이 제약은 ① Claude Code CLI 가 같은 머신·같은 로그인 세션 디렉토리(`~/.claude/`) 에 동시에 여러 프로세스가 쓸 때의 충돌 가능성, ② `git worktree` 가 같은 base branch 에서 동시 push 될 때의 충돌, ③ SQLite WAL 모드의 단일 writer 보장이라는 세 가지 위험 때문이었다.

§12 OI-2 는 v1.1 에서 2~3 병렬을 검토하기로 보류된 항목이다. 현재 운영 데이터로는 직렬 1개 처리 시 일일 5건 cap 까지 도달하는 task 의 총 처리 시간이 단순 합계라, 활성 시간대 (예: 09:00–18:00, 9 시간) 안에 5건이 평균 task 길이 (~30분) × 5 = 2.5h 로 시간 자원이 남는다. 그러나 task 가 길어지거나 (CI fix loop 동반 시) cap 이 7~10 으로 올라가면 활성 시간대를 모두 소진하고도 대기열이 남는 시나리오가 임박했다.

이 기능은 **동시 실행 슬롯을 1 → 2~3개로 확장** 하되, 위 세 가지 충돌 위험을 모두 게이트로 차단한다. 핵심 원칙은 "**병렬도는 config 로 점진적으로 올리고, 충돌 가능성이 0인 조합만 동시 dispatch**" 이다. 첫 release 는 `max_parallel_tasks=1` 기본값을 유지하여 v1 동작과 100% 동일하고, 운영자가 명시적으로 2 이상으로 올리는 시점부터만 병렬 코드 경로가 활성화된다 (feature-flag 방식).

핵심 제약:
- **Anthropic API key 금지** (상위 PRD 와 동일) — `claude` CLI 의 동시 실행 가능 여부는 **실측 spike 단계에서 검증**
- **활성 시간 / budget gate / rate-limit block** 은 모든 병렬 슬롯에 동일하게 enforced
- **같은 레포 동시 처리 금지** — v1.1 은 in-memory `sync.Map[repo]*sync.Mutex` 로 차단 (다른 레포끼리만 병렬)
- **SQLite 단일 writer** — `sql.DB.SetMaxOpenConns(1)` 는 그대로 유지. dispatch 의 task pickup 은 row-level locking 패턴으로 race 방지
- **Worker pool 패턴** — `errgroup.SetLimit(N)` 또는 N 워커 채널 패턴 (구현은 §6.4 참조)

## 2. 목표 (Goals)

- G1. `config.yaml` 의 `concurrency.max_parallel_tasks` (기본 `1`, 권장 최대 `3`) 로 동시 실행 슬롯 수를 선언적으로 조절. `0` 또는 `1` → v1 직렬 모드와 **바이트 동일** 동작
- G2. 다른 레포의 task 는 동시에 dispatch 가능하되 **같은 레포의 task 는 항상 직렬**. 동일 (repo, issue) 에 대해 race 가 발생해도 Task INSERT/dispatch 는 정확히 1개
- G3. 병렬 dispatch 시에도 active window / budget gate / rate-limit block 을 **각 슬롯마다 enforced** — 1개 슬롯이 budget 을 소진하면 다른 슬롯도 즉시 차단
- G4. 한 task 의 실패 / 취소 / claude crash 가 다른 슬롯의 task 에 **영향 없음** (격리)
- G5. **단계적 롤아웃** — `max_parallel_tasks=1` (v1.0 호환) → `2` (single-repo throughput 검증) → `3` (multi-repo cap) 로 운영자가 단계별 enable. 각 단계는 PRD §6.5 의 acceptance gate 를 통과해야 다음으로 이동
- G6. v1 의 `Stop` API · Slack Stop button · graceful shutdown 모두 **모든 병렬 슬롯에 적용** — 한 슬롯의 SIGTERM 이 다른 슬롯에 영향 주지 않음

## 3. 비목표 (Non-goals)

- 분산 실행 (다중 머신·다중 프로세스 간 분산 락) — v1.1 도 **단일 머신·단일 프로세스** 전제
- 같은 레포 내 동시 task 처리 (브랜치 충돌 회피 로직) — v2 후보, v1.1 은 single-flight per repo
- 동시 슬롯 수 > 3 의 안정성 보증 — `max_parallel_tasks` 의 hard cap 은 코드에서 `3` 으로 막음 (운영자가 5, 10 등을 넣어도 3 으로 clamp + warn 로그)
- task 우선순위 / 가중치 / 큐 재정렬 — FIFO 유지, 단 같은 레포 task 는 다른 레포 task 가 먼저 잡힐 수 있음 (head-of-line blocking 회피)
- Claude CLI 의 `--session-id` 자동 격리 — v1.1 은 `~/.claude/` 동시 쓰기를 spike 결과 기반으로 결정 (필요 시 `HOME=$tmp/claude-N` 환경변수 격리 검토 — §10 R2)
- 재시작 후 running task 자동 복구 — 상위 PRD §6.2 와 동일하게 orphan 표시만
- 모니터링 대시보드 / metric 노출 — slog 로그 + 기존 `GET /tasks` 만

## 4. 대상 사용자

| 페르소나 | 역할 | 목표 |
|----------|------|------|
| Solo developer (P1) | 다중 레포 운영자 (예: 개인 프로젝트 3~5개) | 활성 시간대 안에 cap 까지 task 를 빨리 소진 |
| Small team lead (P2) | 1~3인 팀, security/perf 자동화 늘리려는 운영자 | task throughput 을 하루 5건 → 10건 수준으로 확장 |

권한 모델 변경 없음 — 단일 사용자, 단일 머신.

## 5. 유저 스토리

| # | 스토리 | 수락 기준 |
|---|--------|----------|
| US-1 | 운영자로서 **동시 실행 슬롯 수** 를 config 로 조절하고 싶다 | 1) `concurrency.max_parallel_tasks` (기본 `1`, 최소 `0`, 최대 `3`) 가 추가됨 <br> 2) `0` 또는 `1` → 기존 직렬 동작과 동일 (worker pool 코드 경로 자체가 비활성) <br> 3) `>3` 입력 시 `3` 으로 clamp + `slog.Warn("parallel: clamped to max=3")` <br> 4) config validate 에서 음수 / 비숫자 reject |
| US-2 | 운영자로서 다른 레포의 task 가 **동시에** 진행되길 원한다 | 1) 다른 `repo_full_name` 을 가진 두 queued task 는 동시에 dispatch 됨 <br> 2) 동시 dispatch 의 시간차 < 200ms (scheduler tick 한 번 안에서) <br> 3) e2e 테스트: 두 개의 fake claude binary 가 stdout 을 동시에 흘리는 시점이 겹쳐야 통과 (`abs(start_a - start_b) < 200ms`) |
| US-3 | 운영자로서 **같은 레포의 task** 가 동시에 돌지 않기를 원한다 | 1) 같은 `repo_full_name` 의 두 queued task 는 항상 직렬로 dispatch (한 쪽이 done/failed/cancelled 가 되어야 다른 쪽이 시작) <br> 2) repo-level mutex 는 in-memory `sync.Map[repo_full_name]*sync.Mutex` 로 구현 <br> 3) head-of-line 회피: 큐 첫 task 의 repo 가 lock 잡혀 있으면 **그 다음 task 부터 다른 repo 를 검색** (FIFO 가 아닌 "FIFO with skip-on-conflict") |
| US-4 | 운영자로서 한 task 의 **실패가 다른 task 에 영향 없기를** 원한다 | 1) 한 슬롯의 worker goroutine 이 panic / crash 해도 scheduler 와 다른 슬롯은 계속 동작 (`recover()` + slog.Error) <br> 2) 한 슬롯의 `claude` 프로세스 SIGKILL 이 다른 슬롯의 pgid 에 영향 주지 않음 (각 task 별 독립 pgid 는 v1 에서도 이미 보장됨 — 검증만) <br> 3) e2e 테스트: 슬롯 A 가 fail 시점에 슬롯 B 는 정상 진행 검증 |
| US-5 | 운영자로서 **모든 슬롯이 budget gate 를 enforce** 하는지 확신하고 싶다 | 1) daily/weekly cap 카운터 increment 는 **각 worker 가 spawn 직전에 atomic 하게** (BudgetEnforcer.CheckAndIncrement) — 병렬 구조에서도 race 없음 <br> 2) 두 슬롯이 동시에 cap 에 도달하면 둘 중 하나만 진행, 다른 하나는 cancel + Slack 통지 <br> 3) rate_limit_block 이 한 슬롯에서 발생하면 **다음 tick 부터 모든 슬롯이 차단** (AppState 단일 진실 공급원 그대로) |
| US-6 | 운영자로서 **Stop API / Slack Stop 버튼** 으로 특정 task 만 중단시키고 싶다 | 1) `POST /tasks/{id}/stop` 은 해당 task 슬롯만 SIGTERM, 다른 슬롯에 영향 없음 <br> 2) Slack `stop_task` action 도 동일 — task_id 정확히 1개에만 적용 <br> 3) `cancelMap[task.ID]` 가 슬롯 수만큼 동시 보관됨 (sync.Map or sync.Mutex 보호) |
| US-7 | 운영자로서 graceful shutdown 시 **모든 슬롯이 안전하게 종료** 되길 원한다 | 1) `Scheduler.Stop()` → 모든 슬롯의 ctx cancel → 각 worker 가 SIGTERM 후 30s 내 SIGKILL (v1 의 `claude.Canceller` 동작 그대로) <br> 2) 모든 슬롯의 worktree 는 `git worktree remove --force` 로 정리 <br> 3) `Scheduler.Stop()` 의 wg.Wait() 는 max(slot_drain_time) 만큼 기다림 (worker 별 timeout 30s + safety margin) |
| US-8 | 운영자로서 단계적으로 병렬도를 올리고 싶다 (**spike → 2 → 3**) | 1) 첫 release 는 `max_parallel_tasks=1` 기본값으로 v1.0 와 동일 동작 검증 <br> 2) §6.5 의 spike 단계에서 fake claude 2개 동시 실행 충돌 여부 + `~/.claude/` 동시 쓰기 안전성을 e2e 측정 <br> 3) 통과 시 운영자가 `2` 로 올림 → 1주 운영 후 4가지 metric (성공률, claude crash 율, worktree 충돌, SQLite lock 에러) 모두 v1 대비 ±5% 안에 들어오면 `3` 으로 승격 <br> 4) §6.6 의 롤백 절차로 언제든지 `1` 로 즉시 복귀 가능 |

## 6. 핵심 플로우

### 6.1 행복 경로 — N 슬롯 동시 dispatch

```
1. scheduler tick (default 30s)
2. active window + full mode 게이트 통과
3. poller.Poll(ctx)  (free, 항상 실행)
4. budget gate snapshot — 차단 사유 있으면 dispatch 전체 스킵
5. for slot := 1..max_parallel_tasks:
     5.1. worker pool 의 빈 슬롯 확인 (sem 채널 or errgroup limit)
     5.2. queued task 1개 pick — repo lock 가능한 첫 task 우선
          (taskRepo.PickNextDispatchable 호출, §7 참조)
     5.3. 빈 슬롯 없으면 break
     5.4. taskID 별 ctx + cancelFn 등록 (cancelMap)
     5.5. go w.RunTask(taskCtx, task)
6. 각 worker 는 v1 의 RunTask 파이프라인 그대로 실행
   (provisionWorktree → renderPrompt → window double-gate → budget atomic check
    → claude.Run → createPR → markDone)
7. RunTask 끝나면 sem 슬롯 반납 + repo lock 해제 + cancelMap 제거
```

### 6.2 같은 레포 충돌 처리

```
큐 상태: [task-A(repo X), task-B(repo X), task-C(repo Y)]
slots: 2

tick 1:
  - slot 1 픽업: task-A → repo X 락 획득 → dispatch
  - slot 2 픽업 시도: 큐 다음 = task-B(repo X) → repo X 락 실패
                       → 큐 next = task-C(repo Y) → repo Y 락 획득 → dispatch
  - 결과: task-A + task-C 동시 진행, task-B 는 대기

tick 2 (task-A 종료 후):
  - slot 1 픽업: task-B(repo X) → repo X 락 획득 (A 가 해제) → dispatch
```

**구현 포인트**:
- repo lock 은 in-memory `sync.Map[string]*sync.Mutex`. `TryLock()` 사용 (Go 1.18+).
- 락 실패는 SQL 호출 없이 in-memory 결정 — tick latency 영향 없음.
- 락은 `RunTask` 의 **시작~끝** 전 구간 보유. defer 로 해제.

### 6.3 예외 경로

- **한 슬롯 worker panic**: top-level `defer recover()` → slog.Error + Slack 통지 + task=failed. 다른 슬롯 영향 없음.
- **claude CLI 동시 실행 시 세션 충돌 감지** (§10 R2 — spike 단계에서 측정): `~/.claude/sessions/` lock 파일 존재 시 `ErrSessionBusy` 반환. 임시 완화: 슬롯별 `HOME=$tmpdir/claude-{slot}` 환경변수 격리 검토. v1.1 의 첫 release 는 spike 결과에 따라 결정.
- **두 슬롯이 동시에 같은 task 를 pick**: §7 의 atomic `UPDATE tasks SET status='running' WHERE id=? AND status='queued'` 패턴으로 정확히 1슬롯만 RowsAffected=1. 나머지는 다음 task 로 진행.
- **budget gate 에서 한 슬롯이 cap 마지막 1슬롯을 가져감**: 다른 슬롯의 `CheckAndIncrement` 가 `BudgetReasonDailyCapReached` 반환 → cancel + Slack 통지.
- **rate_limit_block 발생 (한 슬롯의 Claude CLI 가 wall hit)**: AppState `rate_limit_block` 영속 → 다른 슬롯의 worker 가 다음 spawn 직전 budget gate 에서 차단 → cancel.
- **SQLite "database is locked" 에러**: `_busy_timeout=5000` (현재 설정) 으로 5s 까지 대기. 그래도 lock 되면 transient 에러로 분류 → 다음 tick 재시도. v1.1 에서 lock 발생률을 metric 으로 관찰.
- **slot 수 변경 (config reload)**: v1.1 은 hot-reload 미지원 — config 변경은 재시작 필요. README 에 명시.
- **graceful shutdown 중 새 dispatch**: `Stop()` 호출 후엔 `tick()` 이 즉시 return → 추가 dispatch 없음. 진행 중 task 만 drain.

### 6.4 Worker pool 패턴 결정

두 가지 후보:
- **(A) `errgroup.WithContext` + `SetLimit(N)`**: Go 표준 ergonomic, 한 worker err 이 group 전체 cancel — v1.1 에는 부적합 (격리 요구 G4 위배).
- **(B) N-buffered semaphore channel** + `sync.WaitGroup` (v1 의 `sem chan struct{}` 확장): 한 worker err 이 channel 통해 다른 worker 에 영향 없음. v1 의 `sem = make(chan struct{}, 1)` 을 `make(chan struct{}, N)` 으로 단순 확장.

**결정**: **(B) 채택**. 코드 변경 최소화 + 격리 보장 + v1 이 이미 검증된 패턴.

### 6.5 단계적 롤아웃 — spike → ramp-up

```
Phase 0  (release 직전): max_parallel_tasks=1 기본값 — v1.0 와 동일 동작 검증
                          ┗ 회귀 없음 검증 (전체 e2e suite 통과 + 1주 운영)

Phase 1  (spike): max_parallel_tasks=2 + 모든 task 가 같은 repo 의 dummy task
                  ┗ "같은 repo lock" 이 정확히 직렬로 동작하는지 e2e 측정
                  ┗ Claude CLI 2개를 fake binary 로 동시 spawn — pgid 격리, stdout 섞임 없음 검증
                  ┗ 1일 운영 측정. 4가지 metric (성공률, crash 율, worktree 충돌, SQLite lock 에러) 모두 v1 ±5%

Phase 2  (single-repo throughput 검증): max_parallel_tasks=2 + 다른 repo 의 task 혼재
                  ┗ 실제 ~/.claude/ 동시 쓰기를 운영 환경에서 측정
                  ┗ 1주 운영 후 metric 통과 → Phase 3 진행

Phase 3  (multi-repo cap): max_parallel_tasks=3
                  ┗ 코드의 hard cap. 운영자가 4 이상을 입력해도 3 으로 clamp
                  ┗ 1주 운영 측정 → 안정 시 v1.1 정식 release
```

각 phase 의 통과 기준은 §9 비기능 요구사항의 "안정성" 섹션 metric 으로 정의.

### 6.6 롤백 절차

이슈 발생 시 다음 중 가장 빠른 경로로 즉시 직렬 모드 복귀:

1. **(즉시, 재시작 X)**: 운영자가 SQLite 의 `app_states` 에 직접 `concurrency_override = {max: 1}` 를 INSERT — 다음 tick 부터 슬롯 1개만 사용. PRD §8 의 `PATCH /modes/concurrency` 도 동일 효과.
2. **(재시작 필요)**: `config.yaml` 의 `concurrency.max_parallel_tasks: 1` 로 되돌리고 `systemctl restart scheduled-dev-agent`.
3. **(코드 수준 kill switch)**: 환경변수 `PARALLEL_TASKS_DISABLED=1` 가 set 되어 있으면 config 와 무관하게 직렬 모드 강제 — 코드에서 가장 우선.

롤백 후 다음 작업:
- 진행 중이던 task 는 graceful shutdown 으로 drain.
- 발생한 충돌의 root cause 분석 → 본 PRD 에 학습 사항 기록 (§12 OI 추가).

## 7. 데이터 모델 (요약)

기존 SQLite (`data/agent.db`) 에 마이그레이션 1개 추가. 기존 migration 수정 금지.

```
Task (변경 없음)
  ※ status='running' 으로의 전이 race 를 막기 위한 SQL 패턴은 sqlc 쿼리로 추가
  ※ worker_id 컬럼은 v1.1 에서 추가하지 않음 (in-memory cancelMap 으로 충분)

AppState (신규 키)
  key="concurrency_override"  value={ max: int, updated_at }
                              ─ 0/1 → 직렬, 2~3 → 병렬, 그 외 → clamp
                              ─ 미존재 시 config.yaml 값 사용
```

**신규 sqlc 쿼리** (`db/query/task.sql` 에 추가):

```sql
-- name: PickNextDispatchable :one
-- 큐의 첫 task 부터 검색하되, 인메모리에서 차단된 repo 는 caller 가 다음 호출에서
-- excluded_repos 로 전달해 스킵한다. SQL 만으로 repo lock 을 표현할 수 없으므로
-- 락은 in-memory 가 권위 있는 출처. 본 쿼리는 단지 후보를 fetch.
SELECT id, repo_full_name, issue_number, ...
FROM tasks
WHERE status = 'queued'
  AND (sqlc.slice('excluded_repos') IS NULL
       OR repo_full_name NOT IN (sqlc.slice('excluded_repos')))
ORDER BY created_at ASC
LIMIT 1;

-- name: ClaimTaskAtomic :execrows
-- task 를 'queued' → 'running' 으로 atomic 전이. RowsAffected=1 이면 성공.
-- v1.1 의 race 방지 핵심 — 두 슬롯이 같은 ID 를 pick 해도 한 쪽만 성공.
UPDATE tasks
SET status = 'running', updated_at = datetime('now')
WHERE id = sqlc.arg('id') AND status = 'queued';
```

**Repo lock (in-memory)** — DB 가 아닌 코드 구조:

```go
// internal/scheduler/repolocks.go (NEW)
type RepoLocks struct {
    m sync.Map  // map[string]*sync.Mutex
}
func (r *RepoLocks) TryLock(repo string) (release func(), ok bool) { ... }
```

## 8. API 계약 (요약)

신규/변경 엔드포인트:

```
GET    /modes/concurrency               현재 동시 실행 캡 + 사용 중 슬롯 수 조회
PATCH  /modes/concurrency               { max?: int } 런타임 오버라이드 (0=config 폴백)
GET    /healthz                         (변경) busy_slots 필드 추가
GET    /tasks                           (변경 없음 — 직렬 시기와 동일)
```

응답 DTO:

```jsonc
// GET /modes/concurrency
{
  "max": 2,                    // 효과적 cap (override > config > default)
  "config_max": 1,             // config.yaml 값
  "override_max": 2,           // AppState 오버라이드 (없으면 0)
  "busy_slots": 1,             // 현재 dispatch 된 task 수
  "running_repos": ["a/b"]     // 락 잡고 있는 repo 목록 (관측용)
}

// PATCH /modes/concurrency body
{ "max": 2 }                   // 0=config 폴백, >3 은 3 으로 clamp + warn

// GET /healthz (변경)
{
  "status": "ok",
  "tick_at": "...",
  "full_mode": false,
  "busy_slots": 1,             // NEW
  "max_parallel_tasks": 2      // NEW
}
```

## 9. 비기능 요구사항

| 항목 | 요구 |
|------|------|
| 성능 | tick 한 번 안에서 N 슬롯 dispatch 가 < 200ms (SQL pickup × N + repo lock check 포함) |
| 안정성 — 성공률 | 병렬 모드의 task 성공률 ≥ v1 직렬 모드의 ±5% (1주 측정) |
| 안정성 — claude crash | exit_code != 0 비율 ≤ v1 직렬 모드의 ±5% |
| 안정성 — worktree 충돌 | git worktree add 실패율 < 1% (1주 측정) |
| 안정성 — SQLite lock 에러 | "database is locked" 발생률 < 0.1% (5s busy_timeout 후에도 실패한 경우만) |
| 격리 | 한 슬롯의 panic / SIGKILL / OOM 이 다른 슬롯의 task status 에 영향 없음 (e2e 검증) |
| Budget enforcement | atomic CheckAndIncrement 가 race-free (테스트: 2 goroutine 이 동시에 cap 마지막 1슬롯을 노릴 때 정확히 1개만 통과) |
| Repo isolation | 같은 repo task 동시 dispatch = 0건 (e2e 100회 반복 측정) |
| 보안 | 변경 없음 — 토큰·secret 노출 경로 추가 없음 |
| 관측성 | tick 마다 `slog.Debug("scheduler: dispatch", "busy_slots", N, "max", M)`. dispatch 이벤트마다 `slot_id` payload 추가 |
| 테스트 커버리지 | 80% (게이트). `internal/scheduler` 패키지는 **85% 이상** |
| lint | `golangci-lint run ./...` 통과 필수 |
| 호환성 | `max_parallel_tasks=1` (기본) 일 때 v1.0 의 동작과 **바이트 동일** — 기존 e2e suite 100% 통과 |

## 10. 의존성 / 리스크

**의존성**

- 기존 `internal/scheduler.Scheduler` (sem 채널, cancelMap, dispatch)
- 기존 `internal/scheduler.Worker` 의 RunTask pipeline (변경 최소화)
- 기존 `internal/usecase.BudgetUseCase.CheckAndIncrement` (이미 mutex 보호 — race-free)
- 기존 `internal/repository.NewDB` 의 `_journal_mode=WAL` + `MaxOpenConns(1)` 설정

**리스크**

- **R1 (High)**: Claude CLI 동시 실행 시 `~/.claude/sessions/` 또는 토큰 캐시 파일 충돌. 완화: ① **spike 단계에서 fake binary 로 충돌 시뮬레이션** ② 실제 운영 phase 1 에서 1일 측정 ③ 충돌 발견 시 슬롯별 `HOME=$tmpdir/claude-{slot}` 격리 (단, 이 경우 로그인 세션 분리 필요 — README 에 절차) ④ 끝까지 안전 담보 안 되면 `max=1` 영구 유지. **즉시 단정하지 말고 단계별 측정**.
- **R2 (High)**: 같은 repo 의 두 task 가 같은 base branch 에 동시 push 시 git push 충돌 → 한 쪽 task fail. 완화: §6.2 의 in-memory repo lock 으로 같은 repo 동시 dispatch 자체를 차단 (SQL 호출 없이 코드 수준에서 걸러냄). 다른 시스템 (사람의 직접 push) 와의 충돌은 별개 — 기존 v1 의 git push rebase 1회 재시도 로직 그대로.
- **R3 (Med)**: SQLite "database is locked" — 두 worker goroutine 이 동시에 같은 row 를 update 시도. 완화: ① `MaxOpenConns(1)` 단일 writer ② `_busy_timeout=5000` 으로 5s 대기 ③ atomic UPDATE 패턴으로 transition race 방지 ④ phase 1 에서 lock 발생률 metric 측정. 발생률 > 0.1% 이면 `max=1` 롤백.
- **R4 (Med)**: BudgetUseCase 의 mutex 가 N 슬롯 dispatch 의 hot path 가 되어 throughput 병목. 완화: 현재 mutex 는 single-flight 를 보장할 뿐 hold time 이 매우 짧음 (수 ms 의 SQLite write). N=3 까지는 무시 가능. 측정 후 문제면 atomic counter (sql.Exec UPDATE counts SET ...) 로 전환.
- **R5 (Low)**: `sync.Map` 의 mutex 누적 — 운영 중 repo 수 만큼 mutex pointer 가 누적. 완화: repo 수가 < 100 이라 무시 가능. v2 에서 LRU 정리 검토.
- **R6 (Low)**: graceful shutdown 시 N 슬롯 drain time 이 30s × N 이 아니라 max 30s (병렬 drain) 가 되도록 wg.Wait + ctx cancel 설계 검증 필요.
- **R7 (Med)**: hot-reload 미지원이므로 운영자가 config 변경 후 재시작 까먹으면 의도와 다른 동시성으로 운영. 완화: README + `/healthz` 에 `max_parallel_tasks` 노출, Slack 시작 메시지에 `slots=N` 포함.

## 11. 범위 외 (Out of Scope)

- 같은 repo 내 task 동시 처리 (브랜치 격리, sub-worktree 등)
- task 우선순위 / 예약 / SLA queue
- 분산 / 멀티 머신 실행
- 동시성 hot-reload (config 변경 즉시 반영)
- adaptive 동시성 (load 기반 자동 조절)
- claude CLI 의 `--session-id` 분리 자동 관리
- worker pool metric (Prometheus/OpenTelemetry) — 별도 PRD `usage-dashboard` 에서 다룸

## 12. 오픈 이슈

- [ ] **OI-1** (Phase 0 spike 결과 대기): Claude CLI 2개 동시 실행 시 `~/.claude/` 의 어느 파일이 충돌하는가? `lsof` / `strace` 로 spike 단계에서 측정. 결과에 따라 슬롯별 HOME 격리 필요 여부 결정.
- [ ] **OI-2**: repo lock 의 mutex 가 누적되는 문제 — `sync.Map` 의 mutex pointer 를 언제 비울지. 단순화 위해 v1.1 은 비우지 않음 (운영 중 repo 수 < 100 가정). 100 이상으로 늘면 LRU 도입 검토.
- [ ] **OI-3**: `PickNextDispatchable` 쿼리에서 excluded_repos 가 길어지면 (running_repos 가 max=3 까지 누적) IN 절 길이가 최대 3 — 무시 가능. 단, "FIFO with skip-on-conflict" 가 정말 starvation 을 일으키지 않는지 측정 필요. 같은 repo 의 task 만 여러 개 있고 다른 repo 가 없으면 슬롯이 1개만 사용됨 — 의도된 동작.
- [ ] **OI-4**: `rate_limit_block` 발생 시 진행 중인 다른 슬롯의 task 도 즉시 cancel 할지, 자연 완료 후 다음 dispatch 만 차단할지. 상위 PRD §6.2 와 일관되게 v1.1 은 **다음 dispatch 만 차단** (현재 진행 중인 task 는 그대로). 추후 정책 변경 가능.
- [ ] **OI-5**: graceful shutdown drain time 의 hard ceiling — max(30s × slots) 이 아닌 30s + safety margin (45s) 으로 묶을지. systemd `TimeoutStopSec` 와 일관되게 설정 필요. 기본 90s 권장.
- [ ] **OI-6**: phase 1 spike 의 실제 수행 주체 — 본 PRD 의 구현자 (Go agent) 가 e2e 테스트 작성 + 1일 dry-run 운영까지 책임지는가, 아니면 운영자에게 인계하는가. v1.1 은 코드 + e2e 까지 구현자 책임, 운영 측정은 운영자 책임으로 분담.

---

## 참고 — 영향받는 파일

```
internal/
  scheduler/
    scheduler.go          # MOD — sem 크기 N 으로 확장, dispatch 루프, repo lock 사용
    repolocks.go          # NEW — sync.Map 기반 per-repo TryLock
    repolocks_test.go     # NEW
    worker.go             # 변경 최소 (각 worker 가 동일 RunTask 그대로 사용)
  domain/
    repository.go         # MOD — TaskRepository 에 PickNextDispatchable, ClaimTaskAtomic 추가
  repository/
    task_repository.go    # MOD — 위 메서드 sqlc 호출 구현
  usecase/
    concurrency_usecase.go      # NEW — AppState 의 concurrency_override 관리
    concurrency_usecase_test.go # NEW
  api/
    handlers_concurrency.go     # NEW — GET/PATCH /modes/concurrency
    dto.go                      # MOD — ConcurrencyResponse, ConcurrencyPatchRequest, HealthResponse 확장
    server.go                   # MOD — 새 라우트 등록

migrations/
  000004_add_concurrency_appstate.up.sql / .down.sql
    (선택) AppState 키만 사용한다면 마이그레이션 불필요 — key-value 라 새 row 만 INSERT

db/query/
  task.sql                # MOD — PickNextDispatchable, ClaimTaskAtomic 쿼리 추가

config.example.yaml       # MOD
  concurrency:
    max_parallel_tasks: 1   # 기본 1, 권장 max 3

cmd/scheduled-dev-agent/main.go  # MOD — Scheduler.Config.MaxParallelTasks 주입

docs/specs/parallel-tasks.md     # 본 PRD
docs/specs/parallel-tasks/go.md  # 구현 프롬프트
```

단일 스택 프로젝트이므로 역할별 분할 없이 **단일 Go 구현 프롬프트** ([`./parallel-tasks/go.md`](./parallel-tasks/go.md)) 로 전달합니다.
