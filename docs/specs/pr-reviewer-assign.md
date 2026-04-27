# PRD — pr-reviewer-assign

| 항목 | 값 |
|------|-----|
| 작성일 | 2026-04-27 |
| 상태 | draft |
| 스택 범위 | Go (단일 바이너리 — `internal/github/`, `internal/config/`) |
| 우선순위 | P1 |
| 작성자 | gs97ahn@gmail.com |
| 부모 PRD | [`./scheduled-dev-agent.md`](./scheduled-dev-agent.md) (§5 US-2, §12 OI-3 해소 항목) |

---

## 1. 배경

현재 `internal/github/pr_creator.go` 는 PR 생성 시 `config.RepoConfig.Reviewers` 배열을 그대로 `gh pr create --reviewer` 플래그에 펼친다. 운영자가 매 레포마다 reviewer 를 수동 나열해야 하므로 **CODEOWNERS 가 이미 정의된 레포에서는 정보가 이중관리** 되고, **이슈에 assignee 를 지정해도 PR 에 자동 전달되지 않는다**.

부모 PRD 의 OI-3 ("PR reviewer 자동 할당 규칙") 가 미해소 상태였고 이번에 다음 정책으로 확정한다:

1. **`config.repos[].reviewers`** — 명시적 선언이 가장 신뢰도 높음 (1순위)
2. **CODEOWNERS** — 변경 파일 경로 매칭으로 자동 도출 (2순위)
3. **이슈 assignee 상속** — 옵션 (3순위, 기본 비활성)

이슈 → PR 자동화에서 reviewer 누락은 머지 지연 / 잘못된 사람에게 알림 가는 문제로 이어진다. 이 기능은 `config` 와 GitHub 메타데이터를 결합하여 **운영자 추가 입력 없이** 적절한 reviewer 를 결정한다.

## 2. 목표 (Goals)

- G1. `repos[].reviewers` 만 설정된 기존 레포는 **기존 동작과 100% 동일** (회귀 0건)
- G2. CODEOWNERS 가 있는 레포는 **변경된 파일들의 owner 합집합** 을 자동 reviewer 로 추가 (mode=merge) 또는 단독 사용 (mode=codeowners-only)
- G3. `inherit_issue_assignee: true` 인 레포는 이슈 assignee 도 reviewer 후보에 포함
- G4. **PR 생성 자체는 reviewer 할당 실패와 독립** — reviewer 단계 에러는 warning 로그 + Slack `:warning:` 통지 후 PR URL 정상 반환
- G5. **자기 자신** (`gh` 가 거부) 또는 **빈 후보** 는 자동 제외, 남은 후보만 `--reviewer` 로 전달
- G6. **팀 reviewer** (`@org/team-name`) 는 user 와 동일하게 처리 (CODEOWNERS 의 `@org/team` 표기 보존)
- G7. CODEOWNERS 조회 1회당 GitHub API 호출 ≤ 1회 (PR 생성당). 캐시 TTL 60초 (메모리 LRU, 레포별)

## 3. 비목표 (Non-goals)

- CODEOWNERS 의 **고급 기능** (브랜치별 CODEOWNERS, 와일드카드 우선순위 정밀 재현) — v1 은 GitHub 의 단순 매칭 규칙 (마지막 매치 우선) 만 지원
- reviewer 가 **PR 머지 정책에 미치는 영향** (required reviewer protection 등) 변경 — 이 PRD 는 PR 생성 시 할당까지만 책임
- `gh pr edit --add-reviewer` 사후 보정 경로 — v1 은 `gh pr create --reviewer` 한 번에 처리. 실패 시 재시도 없이 warning
- 사용자가 **PR 머지 후** reviewer 통계 분석 / 자동 rotation
- GitLab / Bitbucket 의 동등 기능

## 4. 대상 사용자

- **Solo developer (P1)** — 부모 PRD 동일. 자기 레포는 단독 owner 라 사실상 reviewer 자동 할당이 불필요하지만 **다른 사람의 레포에 기여하는 경우** CODEOWNERS 자동 매칭이 가치 발생
- **Small team lead (P2)** — 부모 PRD 의 핵심 수혜자. CODEOWNERS 가 이미 정의된 팀 레포에서 config 중복 제거

권한: 별도 권한 변경 없음. 기존 `GITHUB_TOKEN` 으로 contents 읽기 (CODEOWNERS 파일 fetch) + PR 작성 가능해야 함.

## 5. 유저 스토리

| # | 스토리 | 수락 기준 |
|---|--------|-----------|
| US-1 | 운영자로서 **`repos[].reviewers` 만으로** 기존처럼 reviewer 할당이 동작하길 원한다 | 1) 신규 옵션을 모두 빈 값으로 두면 기존 v0 동작과 바이트 단위로 동일한 `gh pr create` args 생성 <br> 2) 기존 `pr_creator_test` 회귀 테스트 통과 (테스트 신규 추가 포함) |
| US-2 | 운영자로서 **CODEOWNERS 만 사용** 하는 모드를 켜고 싶다 | 1) `repos[].reviewer_strategy: "codeowners-only"` 로 선언 <br> 2) PR 생성 직전 변경 파일 목록(`git diff --name-only` 또는 worktree 의 staged files) 으로 CODEOWNERS 매칭 → user/team 합집합 산출 <br> 3) 매치된 owner 가 0명이면 reviewer 미지정으로 PR 생성 (warning 로그) <br> 4) `repos[].reviewers` 가 함께 설정되어 있어도 무시 (codeowners-only) |
| US-3 | 운영자로서 **config 우선 + CODEOWNERS 보완** (merge) 모드를 쓰고 싶다 | 1) `repos[].reviewer_strategy: "merge"` 로 선언 <br> 2) 후보 = `repos[].reviewers` ∪ CODEOWNERS_owners <br> 3) 중복 제거 (대소문자 무시 비교, 출력은 첫 등장 형태 유지) <br> 4) 최종 후보 수 > 15 면 앞에서부터 15명 cut + warning 로그 (gh CLI 한도) |
| US-4 | 운영자로서 **이슈 assignee 도 reviewer 로 상속** 하길 원한다 (옵션) | 1) `repos[].inherit_issue_assignee: true` 로 선언 (기본 false) <br> 2) 폴러가 task 생성 시 issue.assignees 를 task 메타에 저장 → PR 생성 단계에서 후보에 합집합 추가 <br> 3) `reviewer_strategy` 와 직교 — 어느 strategy 든 옵션 적용 가능 <br> 4) assignee 가 PR author (claude-ops 봇 자신) 와 동일하면 자동 제외 |
| US-5 | 운영자로서 **자기 자신·잘못된 user** 가 후보에 들어가도 PR 생성이 깨지지 않길 원한다 | 1) 후보 sanitize: PR author (config 의 `github.actor` 또는 `gh api user` 결과) 를 후보에서 제거 <br> 2) `gh pr create --reviewer` 가 일부 user 를 거부해도 (`Could not request reviewer`) 다른 user 는 할당된 채로 PR 자체는 생성 <br> 3) 거부된 user 목록을 `TaskEvent{kind=reviewer_warning, payload={rejected:[...]}}` 로 기록 <br> 4) Slack 알림에 `reviewers assigned (warnings: N)` 형태로 합산 표기 |
| US-6 | 운영자로서 **팀 reviewer** (`@org/team`) 도 동일하게 다뤄지길 원한다 | 1) CODEOWNERS 의 `@org/team-name` 항목은 형태 그대로 유지하여 `gh pr create --reviewer org/team-name` 로 전달 (gh CLI 는 prefix `@` 없이 받음) <br> 2) `repos[].reviewers` 에서도 `org/team` 형태 허용 <br> 3) `/` 포함 여부로 user/team 자동 판별 |
| US-7 | 운영자로서 **CODEOWNERS 가 없는 레포** 에서 silently 동작하길 원한다 | 1) GitHub API 가 404 (CODEOWNERS 미존재) 반환 시 ErrCodeownersNotFound 로 변환 → 빈 owner 목록 + slog.Debug 1줄 (에러 아님) <br> 2) `merge` 모드에선 그대로 `repos[].reviewers` 만 사용 <br> 3) `codeowners-only` 모드에선 reviewer 미지정으로 PR 생성 + warning 로그 |
| US-8 | 운영자로서 **CODEOWNERS 조회 캐시** 로 GitHub API 호출이 폭증하지 않길 원한다 | 1) 레포별 메모리 LRU (entries=64, TTL=60s) <br> 2) 60초 내 재조회는 캐시 히트 (slog.Debug 로그) <br> 3) ETag 기반 conditional GET 까지는 v1 범위 외 (단순 TTL) |

## 6. 핵심 플로우

### 6.1 행복 경로 — merge 모드 (기본 권장)

```
1. Worker.createPR(ctx, task) 호출 (기존)
2. PRCreator.CreatePR 가 git add + commit + push 까지 완료 (기존)
3. → ReviewerResolver.Resolve(ctx, task, worktreePath, repoCfg) 신규
   3a. changedFiles = git diff --name-only origin/{base}..HEAD (worktree 에서 실행)
   3b. switch repoCfg.ReviewerStrategy {
         case "config-only" or "":  candidates = repoCfg.Reviewers
         case "codeowners-only":    candidates = codeownersFor(changedFiles)
         case "merge":              candidates = union(repoCfg.Reviewers, codeownersFor(changedFiles))
       }
   3c. if repoCfg.InheritIssueAssignee: candidates ∪= task.IssueAssignees
   3d. candidates = sanitize(candidates) — self 제외, 중복 제거, > 15 면 앞 15
4. ghArgs = [...] + flatten("--reviewer", candidates)
5. prURL, _ := gh.RunGh(ctx, ghArgs...)
6. if err 가 partial reject (PR 은 생성되었으나 일부 reviewer 실패):
     parsed = parseGhRejectedReviewers(err.message)
     TaskEvent(kind=reviewer_warning, payload={rejected: parsed})
     Slack NotifyReviewerWarning(task, parsed)
     return prURL, prNum, nil  // PR 자체는 성공
7. return prURL, prNum, nil
```

### 6.2 예외 경로

- **CODEOWNERS 미존재**: `gh api repos/{repo}/contents/.github/CODEOWNERS` 가 404 → 빈 set 반환. `merge` 모드는 config 만, `codeowners-only` 는 reviewer 미지정 + warning.
- **CODEOWNERS 조회 실패** (네트워크/5xx): 에러 1회 재시도 (250ms backoff) → 여전히 실패 시 빈 set 으로 degrade + slog.Warn. PR 생성은 진행.
- **gh CLI 가 reviewer 단계에서만 실패** (PR 은 생성됨): `gh pr create` 가 `--reviewer` 일부 거부 시 stderr 에 `request_failed for reviewer X` 메시지 + non-zero exit 가능. 현재 구현은 전체 실패로 보지만, 본 PRD 에서는 stderr 패턴 매칭으로 **PR URL 추출 성공 시 부분 실패로 분류**.
- **changedFiles 추출 실패** (worktree 가 이미 정리되었거나 git 명령 오류): 빈 set 으로 degrade. `codeowners-only` 는 reviewer 미지정. `merge` 는 config 만.
- **CODEOWNERS 파일 파싱 오류** (잘못된 라인): 해당 라인만 skip + slog.Debug, 나머지 라인 정상 처리.
- **issue.assignees 부재** (`InheritIssueAssignee=true` 인데 빈 배열): 정상 — 후보에 추가하지 않음.
- **자기 자신만 남는 케이스** (sanitize 후보가 0명): reviewer 미지정으로 PR 생성 + slog.Debug.

### 6.3 모드 의사결정 매트릭스

| `reviewer_strategy` | `reviewers` | CODEOWNERS | 결과 |
|---------------------|-------------|------------|------|
| 미설정 / `config-only` | `[user1]` | 무관 | `[user1]` |
| `codeowners-only` | 무관 (무시) | 매치된 owner | CODEOWNERS 결과만 |
| `merge` | `[user1]` | `[user2, @org/team]` | `[user1, user2, org/team]` |
| `merge` | `[]` | `[user2]` | `[user2]` |
| `merge` | `[user1]` | (404) | `[user1]` |
| `codeowners-only` | `[user1]` | (404) | `[]` (warning) |

`InheritIssueAssignee=true` 면 위 결과에 task.IssueAssignees 합집합 추가.

## 7. 데이터 모델 (요약)

기존 `Task` 엔티티에 다음 1개 필드 추가:

```
Task.IssueAssignees []string  // task 픽업 시점의 issue.assignees 스냅샷 (login 만)
                              // SQLite 컬럼: TEXT (JSON encoded), default '[]'
```

신규 EventKind 1개:

```
const EventKindReviewerWarning EventKind = "reviewer_warning"
// payload_json: {"rejected": ["user1", "org/team"], "reason": "could not request reviewer"}
```

신규 AppState 키 없음. CODEOWNERS 캐시는 메모리 only (재시작 시 cold start 허용).

마이그레이션:

```
migrations/000004_add_task_issue_assignees.up.sql
  ALTER TABLE tasks ADD COLUMN issue_assignees TEXT NOT NULL DEFAULT '[]';
migrations/000004_add_task_issue_assignees.down.sql
  ALTER TABLE tasks DROP COLUMN issue_assignees;
```

## 8. API 계약 (요약)

신규 HTTP 엔드포인트는 없음. 기존 `GET /tasks/{id}` 응답 DTO 에 `issue_assignees: string[]` + `reviewer_warnings: string[]` (마지막 reviewer_warning 이벤트의 rejected) 추가.

`config.yaml` 스키마 확장:

```yaml
github:
  repos:
    - name: "owner/repo"
      default_branch: "main"
      labels: ["claude-ops"]

      # 기존 — config 명시적 선언
      reviewers: ["user1"]

      # 신규 — reviewer 결정 전략. 미설정 시 "config-only" (= 기존 동작).
      reviewer_strategy: "merge"   # config-only | codeowners-only | merge

      # 신규 — 이슈 assignee 도 reviewer 후보에 포함 (직교 옵션)
      inherit_issue_assignee: false

      checks:
        security: true
        perf: false
```

내부 인터페이스 (Go):

```go
// internal/github/codeowners 패키지
type Codeowners interface {
    // OwnersFor returns the union of owners (users + teams) for the given
    // changed file paths in the given repo. Returns empty slice if CODEOWNERS
    // does not exist or no rule matches.
    OwnersFor(ctx context.Context, repo string, changedFiles []string) ([]string, error)
}

// internal/github 패키지에 신규 추가
type ReviewerResolver interface {
    Resolve(ctx context.Context, task *domain.Task, worktreePath string,
            repoCfg config.RepoConfig) (reviewers []string, warnings []string, err error)
}
```

## 9. 비기능 요구사항

| 항목 | 요구 |
|------|------|
| 성능 | reviewer 결정 (CODEOWNERS 캐시 히트 기준) p95 < 50ms. 캐시 미스 (GitHub API 1콜) 포함 p95 < 800ms |
| 회귀 안전 | `reviewer_strategy` 미설정 레포는 v0 동작 유지 (기존 yaml 무수정) |
| 보안 | CODEOWNERS 응답이 base64 디코드 시 UTF-8 미매치면 raw 보존 + slog.Warn. PII (user email) 가 CODEOWNERS 에 포함될 수 있으니 로그 출력 시 user login 만 노출 |
| 관측성 | reviewer 결정 결과 (strategy, count, source breakdown) 를 Task 에 `task_event{kind=reviewer_decided}` 로 1건 기록 (payload 에 sources: {config: N, codeowners: N, assignees: N}) |
| 캐시 | 메모리 LRU 64 entries × TTL 60s. 멀티 인스턴스는 v1 미고려 |
| 테스트 커버리지 | 패키지별 ≥ 85% (`internal/github/codeowners`, `internal/github/reviewer_resolver`) — Go CLAUDE.md 게이트 + 부모 PRD §9 따름 |
| lint | `golangci-lint run ./...` 통과 |
| swag | 신규 HTTP 핸들러 없음 (응답 DTO 확장만) — `GetTask` 핸들러의 `@Success` 응답 모델 godoc 갱신 |

## 10. 의존성 / 리스크

**의존성**
- `github.com/google/go-github/v60` — `Repositories.GetContents` 또는 `Repositories.DownloadContents` 로 CODEOWNERS fetch (이미 `internal/github/client.go` 에서 사용 중)
- `git diff --name-only origin/{base}..HEAD` — worktree 에서 실행 가능 (Worker 가 push 직후, PR 생성 직전 단계 보장)
- 기존 `gh pr create --reviewer` 인터페이스 — 변경 없음

**리스크**
- **R1 (Med)**: CODEOWNERS 매칭 규칙은 GitHub 공식 동작과 완전히 동일하지 않을 수 있음 (특히 wildcard 우선순위) → v1 은 "마지막 매치 우선" 단순 규칙 + 단위 테스트로 GitHub 문서 예시 case 5종 검증. 미스매치 신고 시 issue 생성하여 추후 보정.
- **R2 (Low)**: `gh pr create` stderr 의 partial-reject 메시지 포맷이 gh 버전 업데이트로 변경되면 parsing 깨짐 → 정규식 보수적 매칭 + 알 수 없는 형태는 "전체 실패" 로 fallback (안전 측). 통합 테스트에서 gh 2.x major 변경 시 fixture 갱신.
- **R3 (Low)**: `inherit_issue_assignee=true` + 이슈 assignee 가 다수 (>10) 인 경우 reviewer 한도 (15) 초과 → 우선순위 정해서 cut: config > codeowners > assignees. cut 시 warning.
- **R4 (Low)**: 캐시 stale → 운영자가 CODEOWNERS 를 막 수정한 직후 60초간 옛 owner 가 reviewer 로 지정될 수 있음. 허용 trade-off (TTL 짧음).
- **R5 (Low)**: PR author 식별 — `gh api user` 호출 결과를 startup 시 1회 캐시. 토큰 권한 부족하면 빈 string → self-exclude 동작 안 함 (gh 가 거부하므로 partial-reject 경로로 흐름).

## 11. 범위 외 (Out of Scope)

- **CODEOWNERS 우선순위 / 와일드카드** 의 GitHub 동작 100% 재현 (v2 후보 — 정확도 이슈 누적되면 검토)
- **PR 생성 후 reviewer 추가** (`gh pr edit --add-reviewer`) 보정 경로
- **GitHub App 기반 인증** — 현재 PAT 만 지원
- **Reviewer rotation / round-robin** — 단순 합집합만
- **Slack 에서 reviewer 즉시 변경 버튼** (v2 후보)
- **GitLab Approver Rule, Bitbucket Default Reviewer** 호환

## 12. 오픈 이슈

- [ ] **OI-1**: `inherit_issue_assignee=true` 시 issue.assignees 가 task 큐 시점에 이미 비어있고 PR 직전에 다시 fetch 해야 하는 케이스 — 폴링 시점과 PR 생성 시점 간격이 큰 경우 (full mode + queued task) assignee 가 추가되었으면? v1 결정: **task 큐 시점 스냅샷 사용** (단순, 결정 가능). PR 생성 직전 refetch 는 v2 후보.
- [ ] **OI-2**: CODEOWNERS 위치는 `.github/CODEOWNERS` / `CODEOWNERS` / `docs/CODEOWNERS` 셋 중 하나. v1 은 셋 다 시도 (.github 우선) — 첫 매치 캐시. GitHub 가 Enterprise 서버에서 추가 위치를 지원하는 경우는 무시.
- [ ] **OI-3**: `reviewer_strategy` 의 default 를 `config-only` 로 둘지 `merge` 로 둘지 — v1 은 회귀 안전 우선 `config-only`. 운영자가 명시적으로 `merge` 옵트인. 추후 6개월 사용 데이터 보고 default 변경 검토.

---

## 참고 — 관련 기존 코드

- `internal/github/pr_creator.go` — `--reviewer` 플래그 생성 위치 (line 93~95)
- `internal/github/poller.go` — issue.assignees 캡처 추가 지점 (`detectTaskType` 인근)
- `internal/config/config.go` `RepoConfig` — 신규 필드 추가
- `internal/config/validate.go` `validateRepos` — `reviewer_strategy` enum 검증 추가
- `internal/scheduler/worker.go` `createPR` — 호출 시 worktree path 전달 보장 (이미 가능)
- `internal/slack/blocks.go` — `BuildReviewerWarning` 신규 빌더 추가

단일 스택 프로젝트이므로 역할별 분할 없이 **단일 구현 프롬프트** (`./pr-reviewer-assign/go.md`) 로 전달합니다.
