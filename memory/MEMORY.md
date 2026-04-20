# Second Brain — scheduled-dev-agent

> 이 파일은 프로젝트의 기관 기억(institutional memory)입니다.
> 기술 결정, 교훈, 반복되는 패턴을 여기에 누적하세요.
> 규칙은 `CLAUDE.md`, 맥락과 히스토리는 이 파일에 기록합니다.

---

## 2026-04-20: 프로젝트 시작

**카테고리:** 결정

- **프로젝트명:** scheduled-dev-agent
- **모드:** 단일 스택 (Go)
- **스택:** Go · Gin · GORM(+sqlc) · SQLite · golang-migrate · golangci-lint · swaggo/swag · testify+mockery
- **목적:** Claude Code 플랜 사용량(token quota)을 낭비 없이 소진하기 위한 홈서버/VPS 상주 Go 단일 바이너리. 사용자가 비활성인 시간(수면·업무 외)에도 GitHub 이슈를 자동으로 Claude Code CLI 로 처리 → PR 생성.
- **핵심 기능:**
  1. **Active window 게이트** — `config.yaml` 의 시간대 안에서만 `claude` 프로세스 spawn (밖에서는 스폰 자체 거부)
  2. **이슈 → PR 파이프라인** — `markRunning → notifyStarted → provisionWorktree → renderPrompt → window 재확인 → Runner.Run → recordRunOutput → createPR → markDone`
  3. **Full usage mode** — 시간대 우회하고 rate-limit 신호까지 연속 실행 (HTTP API 토글, persistent)
  4. **Budget gate** — daily/weekly task 캡 + rate-limit 자동 throttle. Full mode 도 우회 불가. 기본 daily=5, weekly=daily*7
  5. **Slack 통지/제어** — Block Kit 메시지(시작/완료/실패/취소) + Stop 버튼(서명 검증 포함). p95 < 10s
  6. **라벨 기반 프롬프트 분기** — `feature` / `security` / `perf` → `prompts/{type}.tmpl`
- **제약사항:**
  - **Anthropic API key 사용 절대 금지** — 반드시 머신에 `claude login` 된 CLI 세션을 `os/exec.CommandContext` 로 호출 (PRD 명시 제약)
  - **단일 사용자·단일 머신 전용** — 멀티 테넌트/병렬 세션 X (v1 직렬 실행 1개)
  - GitHub 외 (GitLab/Bitbucket) 미지원, Claude 외 LLM (Cursor/Aider) 미지원
  - Stop SLA: SIGTERM 5초 → SIGKILL 30초
  - 외부 DB/큐 의존성 없음 (SQLite 단일 파일)
  - 우선순위 P0, draft 상태
- **외부 연동:**
  - **Claude Code CLI** — `claude -p <prompt> --output-format stream-json`, 독립 pgid (`Setpgid: true`) 로 spawn
  - **GitHub** — `gh` CLI (PR 생성) + REST API (이슈 polling, allowlist 기반)
  - **Slack** — Bot Token + Signing Secret, Block Kit interactions webhook
  - **SQLite** (GORM + sqlc 혼용 — 단순 CRUD 는 GORM, 동적·집계 쿼리는 sqlc)
  - 배포: systemd unit (`deployments/scheduled-dev-agent.service`) 또는 Docker Compose
  - **PRD 원본:** `docs/specs/scheduled-dev-agent.md` (개정일 2026-04-19)
- **레이어 의존**: `handler → usecase → domain ← repository`. `internal/domain/` 은 외부 패키지 import 금지 (Clean Architecture)
- **모듈 경로:** `github.com/gs97ahn/scheduled-dev-agent`, Go 1.25.0

---

## 2026-04-20: Claude Code 하네스 구성

**카테고리:** 참고

`/init go` 로 Go 단일 스택 하네스를 구성했습니다.

- **CLAUDE.md** — Go Gin 아키텍처 규칙 + Git 전략 (main only, dev 없음) + sqlc/golangci-lint 필수화 규칙
- **.claude/settings.json** — `permissions` (go/golangci-lint/migrate/mockery/swag/sqlc/git/docker 허용, force push 차단) + `hooks` (SessionStart, PreToolUse:safety+pre-push, PostToolUse:lint, Stop:go vet) 병합. 기존 `enabledPlugins` (zoom-plugin, frontend-design, typescript-lsp, marketing-skills) 보존
- **memory/MEMORY.md** — Second Brain 초기화
- **남은 agents** (7개): api-designer, code-reviewer, github-actions-designer, go-generator, go-modifier, go-tester, planner
- **남은 skills** (4개): api-design-patterns, db-patterns, github-actions-patterns, go-patterns
- **남은 templates** (4개): CLAUDE.go.md, settings.go.json, prd.md, role-prompt.md, memory.md
- **제거된 파일**: 14 agents + 5 skills + 21 templates (kotlin/nextjs/flutter/python/multi/monorepo 전부)

**Git 전략**: main only (dev 브랜치 없음 — 사용자 메모리 피드백 반영). PR base 는 항상 `main`.

앞으로 중요한 결정·교훈은 `/memory add` 로 기록하세요.
