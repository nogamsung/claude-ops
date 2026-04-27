# PRD — ci-fix-loop

| 항목 | 값 |
|------|-----|
| 작성일 | 2026-04-27 |
| 상태 | draft |
| 스택 범위 | Go (단일 바이너리, scheduled-dev-agent 의 후속 모듈) |
| 우선순위 | P1 |
| 작성자 | gs97ahn@gmail.com |
| 모 PRD | [`./scheduled-dev-agent.md`](./scheduled-dev-agent.md) |

---

## 1. 배경

`scheduled-dev-agent` 의 워커는 이슈 → PR 까지 자동화하지만 **PR 생성 직후 GitHub Actions CI 가 실패** 하면 사람이 직접 로그를 확인하고 후속 수정 커밋을 만들어야 한다. 활성 시간대 외(밤·새벽)에 PR 이 만들어졌다면 다음 윈도우까지 CI 실패는 방치되어, 본 시스템의 "사람 개입 0회" 목표(G1)가 깨진다.

이 기능은 **PR 생성 직후 CI 결과를 polling 하다가 실패 conclusion 을 발견하면 같은 worktree·branch 에서 자동 후속 fix task 를 spawn 해 추가 커밋을 같은 PR 에 push** 하는 보정 루프를 추가한다. 무한 루프 방지를 위해 **retry cap (기본 2회)** 를 적용하며, 활성 시간대·budget gate·rate-limit block 등 모든 기존 게이트를 그대로 통과해야 한다.

핵심 제약: GitHub webhook 은 별도 PRD(github-webhook) 에서 통합 예정이므로 **v1 은 polling 만**. CI 상태 조회는 `gh pr checks <pr_number> --json conclusion,name,detailsUrl` 로 통일.

## 2. 목표 (Goals)

- G1. CI failure 발견부터 fix task enqueue 까지 **2분 이내** (poll interval ≤ 60s 기준)
- G2. 같은 PR 에 대해 fix attempt 가 **기본 max 2회** 까지만 시도되고 그 이상은 자동 escalate (Slack 통지 + 멈춤)
- G3. 동일 PR + 동일 head commit SHA 조합으로 **fix task 는 정확히 0 또는 1개** 만 enqueue (중복 방지)
- G4. fix task 도 active window / budget gate / rate-limit block 을 **그대로 enforced** — 야간에 자동 spawn 되지 않음
- G5. fix 프롬프트에 **실패한 step 이름과 log tail (gh run view --log-failed)** 이 항상 포함되어 Claude 가 원인 식별 가능
- G6. 기존 worker pipeline (markRunning → ... → createPR → markDone) 변경 최소화 — fix loop 는 별도 stage 로 부착

## 3. 비목표 (Non-goals)

- GitHub webhook (`workflow_run`, `check_run`) 수신 — 별도 PRD `github-webhook` 에서 다룸
- CI 의 특정 step 단위 재실행 (`gh run rerun --failed`) 만으로 해결 시도 — Claude 호출 없이 인프라 재시도하는 경로는 v2
- CI 실패 원인 자체 분류 / triage (테스트 vs 린트 vs 빌드) — Claude 가 로그를 보고 알아서 판단
- max attempt 초과 시 자동 PR close — v1 은 stuck 상태로 남기고 Slack 알림만
- 다른 PR (다른 task 가 만든) 의 CI 모니터링 — 본 task 가 만든 PR 만 추적
- merge 자동화 — 사람 review 필수 원칙 유지

## 4. 대상 사용자

| 페르소나 | 역할 | 목표 |
|----------|------|------|
| Solo developer (P1) | 홈서버 운영자, scheduled-dev-agent 운영 중 | 수면·업무 시간 외에 만들어진 PR 의 CI 실패가 다음 윈도우까지 방치되지 않게 |
| Small team lead (P2) | 보안/성능 자동 처리 운영자 | 자동 PR 의 CI 실패가 본인 업무 시간을 잠식하지 않게 |

권한 모델: 모 PRD 와 동일 — 운영자 = `gh` CLI 인증 사용자.

## 5. 유저 스토리

| # | 스토리 | 수락 기준 |
|---|--------|----------|
| US-1 | 운영자로서 자동 PR 의 CI 가 실패하면 **사람 개입 없이 후속 fix 커밋이 같은 PR 에 추가** 되기를 원한다 | 1) PR 생성 후 워커가 `gh pr checks` 를 polling (default 60s) <br> 2) 모든 check 가 종료된 시점에 하나라도 `conclusion ∈ {failure, timed_out, cancelled, action_required}` 면 fix task 를 enqueue <br> 3) fix task 는 같은 worktree·branch 를 재사용 → 추가 커밋 → 같은 PR 에 push (PR 번호·URL 변경 없음) <br> 4) 모든 check 가 `success` 면 polling 종료 + Slack `:white_check_mark: CI passed` |
| US-2 | 운영자로서 **무한 루프** 방지를 위해 fix attempt 횟수에 상한이 있기를 원한다 | 1) `Task.fix_attempt_count` 컬럼이 추가되어 fix task 마다 +1 <br> 2) `config.yaml` 의 `ci_fix.max_attempts` (default 2) 를 초과하면 새 fix task 를 enqueue 하지 않음 <br> 3) 한도 도달 시 Slack `:warning: CI fix exhausted (2/2 attempts)` + PR 코멘트 1줄 (`@user CI still failing after 2 auto-fix attempts`) <br> 4) 부모-자식 관계는 `parent_task_id` FK 로 추적 (체인 길이 검증 가능) |
| US-3 | 운영자로서 fix Claude 가 **실패한 step 이름과 log tail** 을 보고 원인을 파악하기를 원한다 | 1) `gh run view <run_id> --log-failed --json` 으로 실패 step + log tail (최근 200줄) 추출 <br> 2) `prompts/ci-fix.tmpl` 에 `{{.FailedStep}}`, `{{.LogTail}}`, `{{.PRNumber}}`, `{{.PreviousAttempts}}` 바인딩 <br> 3) log tail 이 16KB 초과 시 끝부분 200줄로 절단 + `[truncated]` 마커 <br> 4) 시크릿 마스킹: `${{ secrets.* }}` 토큰 패턴은 `***` 로 치환 후 프롬프트 삽입 |
| US-4 | 운영자로서 fix task 도 **활성 시간대·budget gate·rate-limit block** 을 그대로 통과해야 한다고 본다 | 1) fix task 는 status=queued 로 enqueue → 기존 scheduler tick 이 dispatch <br> 2) window 밖이면 다음 윈도우까지 대기 (즉시 실행 금지) <br> 3) daily/weekly cap 카운터에 fix task 도 1건으로 카운트 <br> 4) rate_limit_block 활성 중이면 dispatch 차단 — 모 PRD §6.2 동일 |
| US-5 | 운영자로서 같은 PR + 같은 commit SHA 에 대해 **중복 fix task** 가 만들어지지 않기를 원한다 | 1) fix task 생성 전 `(parent_task_id, head_sha)` 유니크 검사 <br> 2) 이미 동일 키로 fix task 가 존재하면 스킵 (info log 만) <br> 3) head_sha 는 `gh pr view --json headRefOid` 로 조회 <br> 4) head_sha 가 새로 push 되어 바뀌면 attempt 카운터를 새로 사용 (다른 commit 대상이므로 별개) |
| US-6 | 운영자로서 CI 결과 polling 상태를 **`/tasks/{id}` 로 조회** 하여 진행도를 알고 싶다 | 1) Task 상세에 `ci_status ∈ {pending, watching, passed, failed, exhausted}` 가 노출됨 <br> 2) TaskEvent 에 `ci_check_polled`, `ci_failure_detected`, `ci_fix_enqueued`, `ci_fix_exhausted` kind 추가 <br> 3) `GET /tasks?ci_status=watching` 필터 지원 <br> 4) Swagger 에 신규 enum 반영 |
| US-7 | 운영자로서 fix 루프를 **글로벌 토글** 로 끌 수 있어야 한다 | 1) `config.yaml` 의 `ci_fix.enabled` (default true) 로 기능 자체 on/off <br> 2) 또는 `PATCH /modes/ci-fix {enabled: bool}` 로 런타임 토글 (AppState `ci_fix_mode` 영속) <br> 3) off 상태에서는 PR 생성 후 polling 도 시작하지 않음 (기존 워커 동작과 동일) |

## 6. 핵심 플로우

### 6.1 행복 경로 — CI 실패 → 자동 fix → 통과

```
1. 기존 worker 가 createPR 성공 → markDone
2. (NEW) ci-watcher 가 task.pr_number, task.repo_full_name 으로 polling job 등록
   - poll interval: 60s (config: ci_fix.poll_interval)
   - poll timeout:  30m (config: ci_fix.poll_timeout) — 그 이상이면 timeout 처리
3. tick 마다 `gh pr checks <pr> --json conclusion,name,detailsUrl,status` 호출
4. 모든 check status == "completed" 가 될 때까지 대기 (in_progress / queued 는 계속 polling)
5. 종료 후 conclusion 조사:
   a) 모두 success → ci_status=passed, polling 종료, Slack 통지
   b) failure 등 발견 → 6.1.1 분기
   c) action_required (manual approval) → ci_status=stuck, polling 종료, Slack 통지
```

#### 6.1.1 fix task enqueue

```
6. PR head_sha 조회 → (parent_task_id, head_sha) 로 중복 검사
7. fix_attempt_count 가 max_attempts 미만인지 확인
8. gh run view <failed_run_id> --log-failed 로 로그 추출 (시크릿 마스킹)
9. 새 Task INSERT:
     - task_type = 'ci-fix'
     - parent_task_id = 원본 task.id
     - fix_attempt_count = 부모.fix_attempt_count + 1 (부모가 ci-fix 면 그 카운트 + 1)
     - repo_full_name, issue_number = 부모와 동일
     - worktree_path = 부모 worktree 재사용 (제거되지 않음 — 6.3 참조)
     - prompt_template = "ci-fix"
     - status = 'queued'
10. TaskEvent kind='ci_fix_enqueued' 기록
11. Slack 통지 (:gear: CI failed — fix attempt {n}/{max})
12. scheduler tick 이 다음 윈도우 안에서 budget gate 통과 시 dispatch
13. fix worker 가 워크트리 진입 → ci-fix.tmpl 렌더 (FailedStep, LogTail 포함)
14. claude 실행 → commit → push (같은 branch) → PR 자동 업데이트 (gh pr create 호출 안 함)
15. fix task 의 polling job 다시 등록 (재귀)
```

### 6.2 예외 경로

- **PR 이 닫히거나 merge 됨**: polling 즉시 중단, ci_status=closed 로 마킹.
- **gh CLI 인증 만료**: polling 실패 → 3회 재시도 후 task event `ci_check_polled` payload 에 err 기록 + Slack 1회 알림 (rate-limited per task).
- **head_sha 변경 (사람 또는 다른 시스템이 직접 푸시)**: attempt 카운터 새로 시작 (별개 commit 으로 간주). 중복 검사 키도 새 sha 기준.
- **max_attempts 도달**: ci_status=exhausted, polling 종료. Slack 통지 + (옵션) PR 코멘트. AppState 에 `ci_fix_exhausted_at` 시각 기록.
- **poll_timeout 초과 (30m)**: ci_status=timeout, polling 종료. CI 가 실제로 끝났는지 한 번 더 확인 후 마킹. Slack 통지.
- **active window 밖에서 fix task dispatch 시도**: scheduler 가 큐에 보관만 함. polling 은 백그라운드에서 계속 진행 (CI 끝났는지는 관찰).
- **rate_limit_block 활성**: fix task dispatch 차단 (모 PRD §6.2 와 동일). reset 후 다음 tick 부터 dispatch.
- **PR 생성 자체가 실패한 task**: ci-watcher 등록 안 됨 (pr_number == 0 가드).
- **재시작 후 watching 중이던 task 복구**: AppState 의 `ci_watch_jobs` 또는 `tasks where ci_status='watching'` SELECT 후 polling 재개. 재시작 동안 누락된 CI 결과는 첫 tick 에 즉시 평가됨.

### 6.3 worktree 수명 변경

기존 워커는 `markDone` 직후 worktree 를 제거(`git worktree remove --force`)했다. fix loop 는 같은 worktree 에서 추가 커밋이 필요하므로:

- ci-fix 가 enabled 인 task 는 markDone 시점에 worktree 를 **보존**
- 다음 중 하나가 발생하면 cleanup:
  - ci_status=passed
  - ci_status=exhausted / timeout / closed / stuck
  - polling 자체가 비활성화됨
- 정기 청소: scheduler tick 시 ci_status ∈ {passed, exhausted, ...} 이고 cleanup 안 된 worktree 가 있으면 일괄 제거 (orphan 방어)

## 7. 데이터 모델 (요약)

기존 SQLite (`data/agent.db`) 에 컬럼·테이블 확장. 새 migration 두 개 추가 (기존 migration 수정 금지).

```
Task (변경)
  + parent_task_id        TEXT    NULL (FK → tasks.id, ON DELETE SET NULL)
  + fix_attempt_count     INTEGER NOT NULL DEFAULT 0
  + ci_status             TEXT    NOT NULL DEFAULT '' CHECK(ci_status IN
                           ('', 'pending', 'watching', 'passed', 'failed',
                            'exhausted', 'timeout', 'stuck', 'closed'))
  + head_sha              TEXT    NOT NULL DEFAULT ''
  + ci_last_polled_at     DATETIME NULL

  task_type CHECK 확장: ('feature', 'security', 'perf', 'ci-fix')

Index (신규)
  idx_tasks_parent          ON tasks(parent_task_id)
  idx_tasks_ci_status       ON tasks(ci_status)
  uniq_tasks_parent_headsha ON tasks(parent_task_id, head_sha) WHERE task_type = 'ci-fix'
                              (SQLite partial index — 중복 방지 원자성)

TaskEvent.kind 확장
  + ci_check_polled         payload: {run_ids, summary, status_per_check}
  + ci_failure_detected     payload: {failed_step, run_id, conclusion, head_sha}
  + ci_fix_enqueued         payload: {child_task_id, attempt, max_attempts}
  + ci_fix_exhausted        payload: {attempts}

AppState (신규 키)
  ci_fix_mode      value: {enabled: bool, updated_at}
  ci_watch_jobs    value: {tasks: [{task_id, pr_number, repo, next_poll_at_unix}]}
                    (재시작 후 polling 복구용 — DB SELECT 로 대체 가능. 우선 후자.)
```

`Task 1 ── N Task` (parent ↔ child fix-chain). 체인 길이는 `fix_attempt_count` 와 일치.

## 8. API 계약 (요약)

신규/변경 엔드포인트만:

```
GET    /tasks?status=&ci_status=&limit=&cursor=     ci_status 필터 추가
GET    /tasks/{id}                                  응답에 ci_status, head_sha,
                                                    fix_attempt_count, parent_task_id 추가
GET    /modes/ci-fix                                {enabled: bool}
PATCH  /modes/ci-fix                                {enabled: bool}
```

**Response DTO 변경 (`internal/api/dto.go` TaskResponse 확장):**

```jsonc
{
  "id": "...",
  "repo_full_name": "owner/repo",
  "issue_number": 42,
  "task_type": "ci-fix",            // 신규 enum 값
  "status": "queued",
  "ci_status": "watching",          // NEW
  "parent_task_id": "abc-123",      // NEW (nullable)
  "fix_attempt_count": 1,           // NEW
  "head_sha": "f00ba2...",          // NEW
  "ci_last_polled_at": "2026-04-27T10:00:00Z"  // NEW
}
```

## 9. 비기능 요구사항

| 항목 | 요구 |
|------|------|
| polling 성능 | tick 당 `gh` 호출은 `watching` 상태 task 수에 비례. v1 은 직렬 1개 제한이라 사실상 1회/tick |
| 보안 | log tail 의 secret 패턴 마스킹: `(?i)(token|secret|password|api[_-]?key)\s*[:=]\s*\S+` 및 `${{ secrets.* }}` |
| 신뢰성 | 재시작 후 `watching` task 의 polling 5초 내 재개 |
| 활성시간 게이트 | fix task dispatch 는 모 PRD 의 budget gate / window gate 통과 필수. polling 자체는 게이트 무관 (gh CLI 호출만) |
| 관측성 | ci_status 변화마다 TaskEvent 1건. polling tick 은 `slog.Debug` |
| 테스트 커버리지 | 80% (게이트). `internal/ci` 패키지는 **85% 이상** |
| 호환성 | 기존 task (ci_status='') 는 polling 미적용 — 마이그레이션은 컬럼 추가만, 기존 row 영향 없음 |

## 10. 의존성 / 리스크

**의존성**

- `gh` CLI 의 `pr checks --json` / `run view --log-failed` 안정성 (모 PRD 와 동일 의존)
- 기존 `internal/scheduler.Worker` 의 markDone 단계 hook 추가 가능성
- 기존 `internal/github.PRCreator` 의 worktree 보존 옵션 (현재는 무조건 정리 안 됨 — worker 가 정리)

**리스크**

- **R1 (Med)**: GitHub CI 가 일시적으로 `failure` 보고 후 retry 로 `success` 복구 — fix task 가 불필요하게 spawn. 완화: 모든 check 가 `status=completed` 가 된 뒤에만 평가. 운영자가 `gh run rerun` 한 경우는 head_sha 동일하므로 중복 검사가 막아줌.
- **R2 (Med)**: log tail 에 시크릿 누출 위험 → Claude 프롬프트로 흘러감. 완화: 패턴 기반 마스킹 + 단위 테스트 다량 + log tail 200줄 cap.
- **R3 (Low)**: 부모 task 가 cancelled / failed 인데 PR 만 만들어졌다면? — 현재 워커는 createPR 실패 시 fail 이므로 PR 이 없어 polling 등록 안 됨. createPR 성공 후 markDone 직전에 죽으면 orphan 가능 — 재시작 시 `pr_url != '' AND ci_status = ''` 인 task 를 일괄 watching 으로 승격.
- **R4 (Low)**: 같은 PR 에 사람이 직접 push (force-push 포함) → head_sha 가 바뀌면서 attempt 카운터 리셋. 의도된 동작이지만 무한 루프 가능성 미세하게 존재. 완화: 사람-author push (`gh pr view --json commits` 의 author 가 bot 이 아닌 경우) 는 watching 중단.
- **R5 (Med)**: ci-fix Claude 가 실패 원인을 못 찾아 동일 변경을 반복 → 2회 모두 같은 패턴으로 fail. 완화: ci-fix 프롬프트에 `PreviousAttempts` 변수로 직전 시도 요약을 포함시켜 차이를 강제. 완전 해결은 아님 → max_attempts=2 가 안전판.

## 11. 범위 외 (Out of Scope)

- GitHub webhook 기반 즉시 트리거 (별도 PRD `github-webhook`)
- CI 자체 재실행만으로 해결 (`gh run rerun --failed`) — Claude 호출 없는 경로는 v2
- merge queue / auto-merge 통합
- fix attempt 별 diff 비교 / 자동 squash
- 다중 PR 동시 polling (v1 직렬 1 task 제한 그대로)

## 12. 오픈 이슈

- [ ] **OI-1**: log tail 의 시크릿 마스킹 패턴이 충분한가? `gh run view --log-failed` 출력에 secret 이 포함되는 케이스 (예: 환경변수 echo) 의 실측 샘플 확보 필요.
- [ ] **OI-2**: `gh pr view --json commits[].author` 로 사람 push 감지가 안정적인가? GitHub bot account (Dependabot, Renovate) 와 본 시스템 (GitHub Actions context 의 `github-actions[bot]` 또는 운영자 PAT) 구분 룰 확정 필요.
- [ ] **OI-3**: max_attempts 도달 시 PR 코멘트 자동 작성 여부 (US-2 수락기준 3 의 옵션). 코멘트 spam 우려 vs 사람 알림 필요성. 기본은 **작성** 으로 두되 config 토글 (`ci_fix.comment_on_exhaustion`) 제공 검토.
- [ ] **OI-4**: polling 을 별도 goroutine pool 로 둘 것인가, 아니면 scheduler tick 안에 inline 으로 둘 것인가? 동시 직렬 1 task 제약이라 inline 이 단순. 단, watching task 가 누적되면 (사람이 PR 닫지 않은 경우) 큰 부하 가능성 — 일정 cap 필요할 수 있음.
- [ ] **OI-5**: ci-fix.tmpl 의 정확한 텍스트 — Claude 에게 "직전 변경을 무효화하지 말고 추가 fix 만 적용" 임을 어떻게 강제할지. 모 PRD 의 OI-7 prompt 정책 (외부 네트워크 금지 / push 금지 / `CHANGES:` 섹션 출력) 와 일관되게 작성 필요.

---

## 참고 — 영향받는 패키지 (Go)

```
internal/
  ci/              # NEW — CIWatcher, gh-checks 파서, log-tail 추출, secret 마스킹
  ci/stream/       # NEW (옵션) — gh pr checks JSON 파서 분리
  scheduler/       # MOD — Worker.markDone 후 ci-watcher hook, scheduler tick 에 polling
  domain/          # MOD — Task 컬럼 추가, EventKind 추가, TaskType=ci-fix
  repository/      # MOD — sqlc 쿼리 추가 (ListWatching, FindByParentAndHeadSHA)
  usecase/         # MOD — TaskUseCase 에 EnqueueFixTask, ListByCIStatus
  api/             # MOD — TaskResponse 확장, /modes/ci-fix 핸들러
  config/          # MOD — CIFixConfig 추가 (enabled, max_attempts, poll_interval, poll_timeout, comment_on_exhaustion)
  github/          # MOD — Client 에 GhPRChecks, GhRunLog, GhPRView, GhPRComment 추가

migrations/
  000004_add_ci_fix_columns_to_tasks.up.sql / .down.sql
  (필요시) 000005_extend_task_type_check.up.sql / .down.sql

db/query/
  task.sql 에 sqlc 쿼리 추가:
    - GetWatchingTasks
    - FindFixChildByParentAndHeadSHA
    - UpdateCIStatus

prompts/
  ci-fix.tmpl    # NEW

config.example.yaml
  ci_fix:
    enabled: true
    max_attempts: 2
    poll_interval: "60s"
    poll_timeout: "30m"
    comment_on_exhaustion: true
```

단일 스택 프로젝트이므로 역할별 분할 없이 **단일 Go 구현 프롬프트** ([`./ci-fix-loop/go.md`](./ci-fix-loop/go.md)) 로 전달합니다.
