# PRD — Claude Ops

| 항목 | 값 |
|------|-----|
| 작성일 | 2026-04-18 |
| 상태 | draft |
| 스택 범위 | Go (단일 바이너리, 홈서버/VPS 상주) |
| 우선순위 | P0 |
| 작성자 | gs97ahn@gmail.com |

---

## 1. 배경 (Background)

개발자는 Claude Code 의 플랜 사용량(plan usage / token quota) 을 최대한 낭비 없이 소진하고 싶다. 현재는 사람이 직접 터미널 앞에 앉아 `claude` CLI 를 켜야만 작업이 진행되므로, 퇴근 후·수면 시간 등 "사용자가 활성화되지 않은 시간" 의 usage 가 전혀 활용되지 못한다. 반대로 밤새 무제한으로 돌려버리면 다음 날 플랜이 소진되어 정작 필요한 시간에 사용할 수 없다.

이 기능은 **홈서버/VPS 에 상주하는 Go 단일 바이너리 (claude-ops)** 가 사용자가 미리 지정한 **활성 시간대(active window)** 에만 GitHub 이슈를 픽업 → Claude Code CLI (local login 세션) 로 개발/보안/성능 작업을 수행 → PR 을 자동 생성하고, 모든 이벤트를 **Slack** 으로 통지한다. 또한 "full usage 모드" 를 수동 토글하면 활성 시간대 게이트를 우회하여 플랜 한도 직전까지 연속 작업을 돌린다.

핵심 제약: **Anthropic API key 를 쓰지 않는다.** 반드시 머신에 이미 `claude login` 된 Claude Code CLI 세션을 `os/exec` 로 호출한다.

## 2. 목표 (Goals)

- G1. 활성 시간대에 한해 **자동으로** GitHub 이슈 → PR 전환 파이프라인 가동 (사용자 개입 0회)
- G2. 활성 시간대 외에는 `claude` 프로세스가 **절대 실행되지 않음** (scheduler gate enforced — 위반 시 서비스 크래시 또는 거부)
- G3. 작업 시작/종료/실패/중단 이벤트가 **10초 이내** Slack 채널로 전달
- G4. Slack Stop 버튼 또는 HTTP API 로 실행 중 task 를 **5초 이내** SIGTERM, 30초 내 SIGKILL 보장
- G5. Full usage 모드 토글 시 활성 시간대 무시하고 **Claude Code usage 한도 신호 감지까지** 연속 실행
- G6. 단일 Go 바이너리 + systemd unit / Dockerfile 로 배포 (외부 DB·큐 의존성 없음 — SQLite)

## 3. 비목표 (Non-goals)

- 멀티 테넌트 / 다중 사용자 지원 (v1 은 **단일 사용자·단일 머신 전용**)
- 웹 UI 대시보드 (Slack + HTTP API 만)
- Claude Code 외 다른 LLM (Cursor, Aider 등) 지원
- GitHub 외 GitLab / Bitbucket 지원
- 복수 Claude 세션 병렬 실행 (v1 은 **직렬 실행 1개** — 세션 충돌 리스크)
- Anthropic API key 기반 호출 경로 (명시적으로 제외)
- 자동 이슈 생성/트리아지 (이슈는 사람이 올린 것만 픽업)
- LLM 자체 fine-tuning / prompt caching 관리

## 4. 대상 사용자 (Users)

| 페르소나 | 역할 | 목표 |
|----------|------|------|
| Solo developer (P1) | 홈서버를 가진 개인 개발자, Claude Max/Pro 플랜 구독 | 자는 시간·업무 시간 외 usage 를 자동 소진하여 개인 프로젝트 진척 |
| Small team lead (P2) | 1~3인 팀 테크 리드 | 반복적인 보안/성능 점검 이슈를 밤사이 자동 처리 |

**권한 모델**: v1 은 서비스 운영자 = Claude Code 로그인 사용자 = GitHub token 소유자가 동일인. Slack 은 지정된 1개 채널만 interactive 권한 허용.

## 5. 유저 스토리 (User Stories)

| # | 스토리 | 수락 기준 |
|---|--------|-----------|
| US-1 | 운영자로서 **활성 시간대** 를 설정하여 그 시간에만 자동 작업이 돌아가도록 할 수 있어야 한다 | 1) `config.yaml` 의 `active_windows: [{days: [mon..fri], start: "09:00", end: "18:00", tz: "Asia/Seoul"}]` 로 선언 가능 <br> 2) 현재 시각이 윈도우 밖이면 `claude` 프로세스 spawn 시도 자체를 거부 (scheduler 거부 로그) <br> 3) 복수 윈도우 선언 가능 (예: 점심시간 제외 12:00-13:00 빈 구간) |
| US-2 | 운영자로서 **GitHub 레포 allowlist** 를 설정하여 지정한 레포만 스캔되도록 할 수 있어야 한다 | 1) `repos: [{name: "owner/repo", labels: ["claude-ops"], default_branch: "main", reviewers: ["user1"], checks: {security: true, perf: false}}]` <br> 2) allowlist 바깥 레포 이슈는 polling 하지 않음 <br> 3) 레포별 라벨 필터 AND 조건 (모든 라벨 보유한 이슈만 픽업) |
| US-3 | 운영자로서 GitHub 이슈가 올라오면 **활성 시간대에 자동으로 PR 까지** 받기를 원한다 | 1) 이슈 픽업 → worktree 생성 → Claude 실행 → commit → push → gh CLI 로 PR 생성 <br> 2) PR body 에 원본 이슈 링크 (`Closes #N`) 자동 삽입 <br> 3) PR 생성 실패 시 worktree 롤백 + Slack 실패 알림 |
| US-4 | 운영자로서 이슈 라벨로 **작업 유형** (개발/보안/성능) 을 구분시키고 싶다 | 1) `feature`, `security`, `perf` 라벨에 따라 Claude 프롬프트 템플릿 분기 <br> 2) 템플릿은 `prompts/{type}.tmpl` 에서 로드 (hot-reload 필요 없음) <br> 3) 라벨 없는 이슈는 기본 `feature` 템플릿 사용 |
| US-5 | 운영자로서 **활성 시간대 외에는 Claude 가 절대 호출되지 않기** 를 원한다 | 1) scheduler 루프가 윈도우 밖이면 큐에서 task 를 꺼내지 않음 <br> 2) 수동 API (`POST /tasks`) 도 윈도우 밖이면 409 Conflict 반환 (full mode 꺼진 상태 한정) <br> 3) e2e 테스트: fake clock 이 윈도우 밖일 때 `claude` exec 호출 0회 검증 |
| US-6 | 운영자로서 **Full usage 모드** 를 켜면 시간대 무시하고 한도까지 밀어붙이길 원한다 | 1) `POST /modes/full {enabled: true}` 로 토글 (persistent — 재시작 후에도 유지) <br> 2) 토글 on 상태에서는 active window 게이트 우회 <br> 3) Claude CLI stdout/stderr 에서 usage / rate limit 경고 감지 시 자동으로 full mode off + Slack 통지 <br> 4) off 복귀 후에는 다음 윈도우까지 대기 |
| US-7 | 운영자로서 **Slack 에서 작업 시작·종료** 를 확인하고 PR 을 한 번에 열고 싶다 | 1) 시작 시: `:rocket: Task started — {owner}/{repo}#{issue}` + 이슈 링크 버튼 + Stop 버튼 <br> 2) 완료 시: `:white_check_mark: Done — PR #{n}` + `View PR` 버튼 + 변경 파일 수 + diff 라인 요약 <br> 3) 메시지 delivery latency p95 < 10s |
| US-8 | 운영자로서 **Slack Stop 버튼** 또는 CLI/HTTP 로 실행 중 task 를 즉시 중단시키고 싶다 | 1) Slack button click → HTTP webhook → 해당 task 의 `claude` pgid 에 SIGTERM <br> 2) 5초 내 미종료 시 SIGKILL <br> 3) worktree `git worktree remove --force` 로 롤백, PR 생성 안 함 <br> 4) Slack 에 `:no_entry: Cancelled` 메시지 <br> 5) Slack signing secret 검증 통과 못하면 요청 거부 |
| US-9 | 운영자로서 모든 작업 이력을 **조회** 하여 사후 검증하고 싶다 | 1) `GET /tasks?status=...&limit=...` → queued/running/done/failed/cancelled 필터 <br> 2) `GET /tasks/{id}` → 이슈 정보, 시작/종료 시각, 추정 usage, PR URL, stdout 로그 경로 <br> 3) `GET /healthz` 200 OK + scheduler tick 시각 + full-mode 상태 |
| US-10 | 운영자로서 서비스를 **systemd 또는 Docker** 로 운영하고 싶다 | 1) `deployments/claude-ops.service` 파일 제공 (WorkingDirectory, Restart=on-failure) <br> 2) `deployments/Dockerfile` + `docker-compose.yml` 제공 (volumes: config, db, claude session, git worktrees) <br> 3) 두 배포 모두 `claude` CLI 세션 디렉토리 (`~/.claude`) 와 `gh` 인증 공유 문서화 |

## 6. 핵심 플로우 (Key Flows)

### 6.1 행복 경로 — 스케줄 기반 이슈 → PR

```
1. scheduler tick (default 30s)
2. 현재 시각이 active window 안인지 확인 (full mode on 이면 skip 검사)
3. 허용되면 GitHub poller 호출 → allowlist 레포의 라벨 매치 open 이슈 fetch
4. 새 이슈 발견 → Task 엔티티 INSERT (status=queued)
5. worker 가 queued 큐에서 pull → status=running
6. git worktree add .worktrees/task-{id} → cd
7. prompts/{type}.tmpl 렌더 (이슈 body + 메타 바인딩)
8. Slack "Task started" 메시지 (Stop 버튼 포함) 전송
9. os/exec.CommandContext("claude", "-p", prompt, "--output-format", "stream-json") spawn
   - 독립 pgid 로 (Setpgid: true) → 중단 시 프로세스 그룹 단위 kill
10. stdout 스트림 파싱 → usage / rate-limit 시그널 수집
11. 완료 시: git add/commit/push → gh pr create → PR URL 확보
12. status=done, pr_url 저장
13. Slack "Done" 메시지 (View PR + diff 요약) 전송
14. worktree remove
```

### 6.2 예외 경로

- **윈도우 밖 요청**: scheduler 가 큐에서 pull 하지 않음. 수동 `POST /tasks` 는 409.
- **Claude usage 한도 감지**: stream-json 에서 rate_limit 시그널 검출 → 진행 중 task 는 graceful stop (chunk 저장) → full mode 자동 해제 → Slack 경고.
- **PR 생성 실패** (gh 인증 만료·네트워크): Task status=failed, worktree 보존(재시도용), Slack `:x:` 알림 + 로그 snippet.
- **git push 충돌**: rebase 시도 1회 → 실패 시 fail.
- **Claude CLI crash**: exit code != 0 → task failed, stderr tail 150줄 Slack 첨부.
- **Slack webhook 서명 불일치**: 401 반환, 요청 무시.
- **서비스 재시작**: running 상태 task 는 복구하지 않음 (orphan 표시 + Slack 통지) — Claude CLI 자식 프로세스도 systemd 가 정리.

### 6.3 Full usage 모드 플로우

```
1. POST /modes/full {enabled: true}
2. mode 상태 SQLite 에 persist
3. scheduler 가 active window 게이트 bypass
4. 이슈 소진 후에도 idle poll 지속 (30s → 10s 간격 축소)
5. Claude stdout 에서 rate_limit 또는 usage_warning 감지
6. 현재 task graceful stop → full mode off → Slack 통지 → 다음 active window 까지 대기
```

## 7. 데이터 모델 (요약)

SQLite 단일 파일 (`data/agent.db`). golang-migrate 로 마이그레이션 관리. GORM 은 단순 CRUD, 검색·집계는 sqlc.

```
Task (id, repo_full_name, issue_number, issue_title, task_type, status,
      prompt_template, worktree_path, pr_url, pr_number, started_at, finished_at,
      estimated_input_tokens, estimated_output_tokens, exit_code, stderr_tail,
      created_at, updated_at)
    status ∈ {queued, running, done, failed, cancelled}
    task_type ∈ {feature, security, perf}

TaskEvent (id, task_id FK, kind, payload_json, created_at)
    kind ∈ {started, slack_sent, claude_stdout_chunk, cancelled, usage_warning, pr_created, failed}

AppState (key PK, value_json, updated_at)
    e.g. key="full_mode", value={enabled: true, enabled_at: ...}
         key="last_poll_at", value={...}
```

관계: `Task 1 ── N TaskEvent`. `AppState` 는 key-value 싱글톤.

## 8. API 계약 (요약)

HTTP server (chi 또는 Gin — **Go CLAUDE.md 가 Gin 명시이므로 Gin**), 기본 `:8787`, 로컬 bind 권장 (Slack webhook 만 역프록시로 공개).

```
GET    /healthz                    헬스체크
GET    /tasks?status=&limit=&cursor=  Task 목록
GET    /tasks/{id}                 Task 상세 + 최근 이벤트
POST   /tasks                      수동 트리거 (repo, issue_number) — window 밖이면 409 (full off 한정)
POST   /tasks/{id}/stop            강제 중단
GET    /modes/full                 현재 모드 조회
POST   /modes/full                 {enabled: bool} 토글
POST   /slack/interactions         Slack Block Kit interactive endpoint (signing secret 검증)
POST   /github/webhook             (선택) issue 이벤트 수신 — polling 보완
```

Slack Block Kit 메시지 스키마 예시:

```json
{
  "blocks": [
    {"type": "section", "text": {"type": "mrkdwn", "text": ":rocket: *Task started* — `owner/repo#42`"}},
    {"type": "actions", "elements": [
      {"type": "button", "text": {"type": "plain_text", "text": "Stop"}, "style": "danger",
       "value": "task:{id}", "action_id": "stop_task"},
      {"type": "button", "text": {"type": "plain_text", "text": "View Issue"},
       "url": "https://github.com/owner/repo/issues/42"}
    ]}
  ]
}
```

## 9. 비기능 요구사항 (Non-functional)

| 항목 | 요구 |
|------|------|
| 배포 | 단일 Go 바이너리 + systemd unit / Dockerfile. 외부 DB / 큐 의존성 없음 |
| 보안 | GitHub PAT · Slack bot token · signing secret 은 환경변수만 (config.yaml 금지). `~/.claude` 세션 디렉토리 파일 권한 0700 확인 |
| 활성 시간 게이트 | **scheduler 레이어에서 enforced**. Claude invoke 직전 `clock.Now()` 재검사 (double-gate) |
| 관측성 | slog JSON 로그 → stdout (systemd-journald). task 별 stdout 은 `data/logs/{task_id}.log` |
| 성능 | task 수 < 10k 기준 `/tasks` p95 < 100ms. scheduler tick 오차 ±2s |
| 신뢰성 | 서비스 재시작 후 5초 내 HTTP listen. running task 는 재개하지 않고 orphan 표시 |
| Slack latency | 시작/종료 메시지 p95 < 10s, Stop 반영 p95 < 5s |
| 테스트 커버리지 | 80% 이상 (Go CLAUDE.md 게이트) — scheduler·claude·slack 패키지는 **85% 이상** |
| lint | `golangci-lint run ./...` 통과 필수 |
| 문서 | Swagger UI `/swagger/index.html` 로 HTTP API 공개 |

## 10. 의존성 / 리스크

**외부 의존성**
- `claude` CLI (Anthropic, local login 된 세션 필수) — **머신에 미리 설치 & 로그인 되어 있어야 함**
- `gh` CLI (GitHub) — PR 생성 인증 공유 용이
- `git` (2.17+, `git worktree` 지원)
- Slack app (bot token, signing secret, interactivity webhook)
- GitHub PAT 또는 GitHub App (issue 읽기 권한)

**리스크**
- **R1 (High)**: Claude CLI 의 stdout 포맷이 변경되면 usage/rate-limit 파싱이 깨짐 → `stream-json` 사용 + 파서 유닛 테스트 다량 확보 + 알 수 없는 포맷은 "warning 로그 + 계속 실행" 으로 degrade
- **R2 (Med)**: active window 게이트 버그로 밤에 실행되면 플랜 소진 → double-gate (scheduler + claude runner 둘 다 검사) + e2e fake clock 테스트
- **R3 (Med)**: 동시 worktree 여러 개 존재 시 같은 브랜치 충돌 → v1 은 직렬 실행 1개로 제한, 세마포어 강제
- **R4 (Low)**: Slack Stop 서명 검증 누락 → 외부 공격자가 임의 kill → signing secret 검증 + timestamp replay 방어 (5분)
- **R5 (Low)**: SQLite write 동시성 → WAL 모드 + 단일 writer 보장

## 11. 범위 외 (Out of Scope)

- 여러 머신에 걸친 분산 실행 / task 스케일아웃
- 웹 대시보드 (v2 후보)
- LLM 결과의 자동 merge (사람 review 필수)
- 작업 결과 기반 후속 이슈 자동 생성 (v2)
- 코스트 대시보드 (usage 는 기록만 하고 조회 API 만 제공)

## 12. 오픈 이슈 (Open Questions)

- [x] **OI-1** (해소 — 2026-04-18 실측): Claude Code CLI `v2.1.113` 의 `-p --output-format stream-json --verbose` 실증 결과:

  **이벤트 타입 5종** (모두 `uuid`, `session_id` 필드 공통):
  - `system.hook_started` / `hook_response` — SessionStart hook 실행 추적
  - `system.init` — 세션 초기화. 주요 필드: `cwd`, `session_id`, `tools[]`, `mcp_servers[]`, `model`, `permissionMode`, `slash_commands[]`, `apiKeySource` (구독 세션은 `"none"` — **CLI 로그인 확인 지점**), `claude_code_version`, `output_style`, `agents[]`, `skills[]`, `plugins[]`, `memory_paths`, `fast_mode_state`
  - `assistant` — 스트리밍 assistant 턴. `message.content[].type` (text / tool_use / ...), `message.usage.{input_tokens, cache_creation_input_tokens, cache_read_input_tokens, output_tokens, service_tier}`
  - `rate_limit_event` — **full-mode 종료 탐지 핵심**. `rate_limit_info.{status: "allowed"|..., resetsAt: unix, rateLimitType: "five_hour", overageStatus: "rejected"|"allowed", overageDisabledReason, isUsingOverage}`
  - `result` — 최종 (정확히 1회). `subtype: "success"|"error"|"error_during_execution"`, `is_error`, `api_error_status`, `duration_ms`, `duration_api_ms`, `num_turns`, `result` (최종 텍스트), `stop_reason: "end_turn"|...`, `total_cost_usd`, `usage.{input_tokens, cache_creation_input_tokens, cache_read_input_tokens, output_tokens, server_tool_use, iterations[]}`, `modelUsage[modelId].{inputTokens, outputTokens, cacheReadInputTokens, cacheCreationInputTokens, webSearchRequests, costUSD, contextWindow, maxOutputTokens}`, `permission_denials[]`, `terminal_reason: "completed"|...`

  **파서 계약**:
  - `internal/claude/stream` 패키지에서 line-delimited JSON (NDJSON) 으로 파싱. 각 라인 → `envelope{Type string; Raw json.RawMessage}` 1차 디코드 → `Type` 에 따라 세부 타입으로 2차 디코드.
  - **알 수 없는 `type` 또는 필드는 무시하되 raw 를 로그에 `slog.Debug` 남김** (CLI 업그레이드 대비).
  - Terminal condition: `type == "result"` 수신 또는 프로세스 exit.
  - Rate-limit 감지: `type == "rate_limit_event" && rate_limit_info.status != "allowed"` OR `result.is_error && strings.Contains(result.result, "rate limit")`.
  - 세션 만료 감지: `system.init.apiKeySource` 가 `"none"` 이 아니거나, result 에 "please login" 류 문자열 포함.
  - 계약 테스트: `testdata/streams/*.ndjson` fixture (success / rate_limit / error / cancelled) 4 종 최소.

- [ ] **OI-2**: 동시 task 병렬 실행 지원 여부. v1 은 직렬 1개 고정이지만 Max 플랜 multi-session 이 허용되면 v1.1 에서 2~3 병렬 검토. 충돌 시나리오 (같은 레포 동시 worktree) 완화 방안 결정 필요.
- [ ] **OI-3**: PR reviewer 자동 할당 규칙 — 레포 config 의 `reviewers` 배열을 그대로 사용할지, GitHub CODEOWNERS 를 참조할지, 이슈 assignee 를 상속할지.
- [ ] **OI-4**: Full usage 모드 종료 후 복귀 정책 — rate limit 감지 후 얼마 뒤(예: `resetsAt` + jitter) 다시 full 시도할지, 아니면 다음 active window 까지 무조건 대기할지. (v1 초안: `resetsAt + 60s` 후 재시도, max 3회)
- [ ] **OI-5**: GitHub webhook 경로 지원 여부 — polling 만으로 충분한가, 아니면 webhook + polling fallback 이 필요한가. (홈서버 외부 노출 리스크 vs 지연)
- [ ] **OI-6**: Claude CLI 세션 만료 감지 — v1 초안: `system.init.apiKeySource == "none"` 이어야 정상. 만약 `--print` 실행 시 auth prompt 가 stderr 로 나오면 task fail + Slack 긴급 알림. 자동 재로그인 불가 (사람이 SSH 접속해서 `claude login` 필요).
- [x] **OI-7** (해소 — v1 템플릿 확정): 작업 프롬프트 템플릿 3종 — `prompts/feature.tmpl`, `prompts/security.tmpl`, `prompts/performance.tmpl`. 공통 변수: `{{.Repo}} {{.Issue.Number}} {{.Issue.Title}} {{.Issue.Body}} {{.Issue.Labels}} {{.Branch}} {{.BaseBranch}}`. 공통 지시: ① 작업 브랜치는 이미 체크아웃됨 ② 변경 후 `gh pr create` 는 호출 금지 (서비스가 담당) ③ 커밋만 수행, push 도 서비스가 담당 ④ 외부 네트워크 호출 금지 ⑤ 테스트가 있으면 실행 ⑥ 마지막 assistant 메시지에 `CHANGES:` 섹션으로 변경 요약 출력. 상세 초안은 `./claude-ops-prompt.md` §13 참조.

---

## 참고 — 아키텍처 레이아웃 (Go)

PRD 단계에서는 개략만. 상세 파일 리스트는 `./claude-ops-prompt.md` 참조.

```
cmd/claude-ops/main.go   # entrypoint, DI wiring, swag @title
internal/
  config/       # YAML + env loader, validation
  scheduler/    # active-window gate, tick loop, full-mode toggle
  github/       # issue poller, PR creator (gh shell-out)
  claude/       # Claude CLI runner (os/exec + stream-json parser + pgid kill)
  slack/        # Block Kit builder, interactions webhook, signature verify
  task/         # Task/TaskEvent domain + SQLite store (GORM + sqlc)
  api/          # Gin HTTP server, handlers, swag 주석
  apperr/       # domain errors
migrations/     # golang-migrate SQL
db/query/       # sqlc 쿼리
deployments/    # systemd unit, Dockerfile, docker-compose.yml
prompts/        # claude 프롬프트 템플릿 (feature / security / perf)
```

단일 스택 프로젝트이므로 역할별 분할 없이 **단일 구현 프롬프트** (`./claude-ops-prompt.md`) 로 전달합니다.
