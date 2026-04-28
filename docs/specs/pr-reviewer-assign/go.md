# pr-reviewer-assign — Go 구현 가이드

상위 PRD: [`../pr-reviewer-assign.md`](../pr-reviewer-assign.md)

> 이 문서는 PRD 의 결정·근거를 반복하지 않습니다. 단계·체크리스트·수락 기준만 담습니다. 모순 시 PRD 가 우선.

---

## 0. 사전 확인

### CLAUDE.md 핵심 규칙
- DI 생성자 주입 / 전역 db 금지
- 모든 레이어 `ctx context.Context` 첫 인자
- `domain/` 외부 패키지 import 금지 — 본 기능의 `Codeowners`, `ReviewerResolver` 인터페이스는 `internal/github/` 패키지에 두고 domain 은 변경 최소
- 동적·페이징 쿼리는 sqlc — 본 기능은 SELECT 변경 없음 (issue_assignees 컬럼 추가만)
- 모든 신규/변경 핸들러 swag godoc 필수
- 기존 migration 수정 금지 — `000004_add_task_issue_assignees.{up,down}.sql` 신규만
- 테스트 없이 UseCase 메서드 추가 금지
- 커버리지 ≥80%, `internal/github/codeowners`, `internal/github/reviewer_resolver` 패키지는 ≥85%

### 영향받는 기존 파일 (수정)
- `internal/domain/task.go` — `IssueAssignees []string` 필드 추가, `EventKindReviewerWarning` (그리고 `EventKindReviewerDecided`) 상수
- `internal/repository/task_repository.go` — `gormTask` 의 `issue_assignees` 컬럼 매핑 (TEXT JSON 인코딩)
- `internal/github/pr_creator.go` — `--reviewer` 플래그 생성 위치 (line 93~95) 를 `ReviewerResolver.Resolve(...)` 결과로 대체. partial-reject stderr 파싱 + Task URL 추출 로직 추가
- `internal/github/poller.go` — `detectTaskType` 인근에서 `issue.Assignees` 추출하여 task 메타에 저장 (`Task.IssueAssignees`)
- `internal/github/client.go` — `GetCodeowners(ctx, repo string) ([]byte, error)` 추가 (`Repositories.DownloadContents` 또는 `GetContents` + base64 디코드). `GetAuthenticatedUser(ctx) (string, error)` 추가 (gh `api user` 또는 go-github `Users.Get(\"\")`) — startup 시 1회 호출 후 캐시
- `internal/config/config.go` `RepoConfig` — `ReviewerStrategy string`, `InheritIssueAssignee bool` 필드 추가
- `internal/config/validate.go` `validateRepos` — `reviewer_strategy ∈ {"", "config-only", "codeowners-only", "merge"}` enum 검증 (빈 값은 `config-only` 동의)
- `internal/scheduler/worker.go` `createPR` — `ReviewerResolver` 호출 추가, worktree path 전달 보장
- `internal/api/dto.go` `TaskResponse` — `IssueAssignees []string`, `ReviewerWarnings []string` 필드 추가
- `internal/slack/blocks.go` — `BuildReviewerWarning(task, rejected)` 신규 빌더
- `internal/slack/client.go` — `NotifyReviewerWarning(...)` 메서드 추가
- `cmd/scheduled-dev-agent/main.go` — DI 조립
- `config.example.yaml` — `reviewer_strategy`, `inherit_issue_assignee` 옵션 예시

### 신규 생성 파일
- `migrations/000004_add_task_issue_assignees.up.sql`
- `migrations/000004_add_task_issue_assignees.down.sql`
- `internal/github/codeowners/parser.go` — CODEOWNERS 텍스트 파서 (라인 단위 → `[]Rule{Pattern, Owners}`)
- `internal/github/codeowners/matcher.go` — 변경 파일 → owner 합집합 (마지막 매치 우선, GitHub 단순 규칙)
- `internal/github/codeowners/cache.go` — 메모리 LRU (entries=64, TTL=60s) — `hashicorp/golang-lru/v2/expirable` 또는 단순 `sync.Map + time.Time` 구현
- `internal/github/codeowners/client.go` — `Codeowners` 인터페이스 구현 (`OwnersFor`) — fetcher (3 위치 순차 시도) + cache + parser + matcher 결합
- `internal/github/reviewer_resolver.go` — `ReviewerResolver` 인터페이스 + 구현
- `internal/github/reviewer_sanitize.go` — self 제외 / 중복 제거 / >15 cut
- `internal/github/gh_reject_parser.go` — `gh pr create` stderr partial-reject 메시지 파싱
- 테스트:
  - `internal/github/codeowners/parser_test.go` — GitHub 공식 예제 5종 fixture
  - `internal/github/codeowners/matcher_test.go` — wildcard / last-match / 디렉토리 / `*` / `**`
  - `internal/github/codeowners/cache_test.go` — LRU TTL / 동시성
  - `internal/github/reviewer_resolver_test.go` — PRD §6.3 의사결정 매트릭스 테이블 테스트
  - `internal/github/reviewer_sanitize_test.go`
  - `internal/github/gh_reject_parser_test.go` — gh 2.x stderr 샘플 fixture
  - `internal/github/pr_creator_test.go` — 회귀 + reviewer 통합

> Migration 은 `issue_assignees` 컬럼 1개만 추가. config 만 확장.

---

## 1. 단계별 작업

### Step 1 — Migration

**파일**: `migrations/000004_add_task_issue_assignees.up.sql`
```sql
ALTER TABLE tasks ADD COLUMN issue_assignees TEXT NOT NULL DEFAULT '[]';
```

`000004_*.down.sql`:
```sql
ALTER TABLE tasks DROP COLUMN issue_assignees;
```

**수락 기준**
- migrate up/down 정상 적용
- 기존 row 들이 `issue_assignees='[]'` 로 초기화 (US-1 회귀 안전)

**Sub-agent**: `go-generator`

### Step 2 — Domain

**작업 내용**
1. `internal/domain/task.go`:
   ```go
   type Task struct {
       // ... 기존 ...
       IssueAssignees []string  // 픽업 시점의 issue.assignees 스냅샷 (login)
   }

   const (
       EventKindReviewerWarning EventKind = "reviewer_warning"
       EventKindReviewerDecided EventKind = "reviewer_decided"
   )
   ```
2. domain repository 인터페이스 변경 없음 (issue_assignees 는 task entity 일부)

**수락 기준**
- `go vet` 통과
- domain 외부 import 없음

**Sub-agent**: `go-modifier`

### Step 3 — Repository

**작업 내용**
- `internal/repository/task_repository.go` `gormTask`:
  ```go
  type gormTask struct {
      // ... 기존 ...
      IssueAssignees string `gorm:"column:issue_assignees;not null;default:'[]'"`
  }
  ```
- domain ↔ gormTask 매핑 시 JSON marshal/unmarshal (`encoding/json`)
- nil/빈 슬라이스 → `'[]'` 일관 처리

**수락 기준**
- 통합 테스트 (sqlite):
  - `Create(task with IssueAssignees=["u1","u2"])` → SELECT 후 동일 슬라이스
  - `Create(task with nil)` → SELECT 후 `[]string{}` (nil 아님)
- 회귀: 기존 task_repository_test 전부 통과
- 커버리지 ≥ 85%

**Sub-agent**: `go-modifier` + `go-tester`

### Step 4 — UseCase

**작업 내용**
- `internal/usecase/task_usecase.go` 의 `EnqueueFromIssue` (또는 동등 함수) 가 `issue.Assignees` 를 `Task.IssueAssignees` 에 저장하도록 시그니처 / 매핑 추가
- `TaskResponse` 매핑 함수 (DTO 변환) 가 `IssueAssignees`, `ReviewerWarnings` 채우도록 확장
- `ReviewerWarnings` 는 마지막 `reviewer_warning` 이벤트의 `payload.rejected` 를 추출 — repository 의 task event 조회 로직 재사용 (또는 task fetch 시 join)

**수락 기준**
- mock TaskRepo + TaskEventRepo 단위 테스트:
  - 이슈 픽업 시 assignees 스냅샷 저장
  - GetTask(id) → ReviewerWarnings 가 마지막 reviewer_warning event 의 rejected 와 일치
- 커버리지 ≥ 85%

**Sub-agent**: `go-modifier` + `go-tester`

### Step 5 — Codeowners 패키지 (`internal/github/codeowners`)

**작업 내용 (4개 파일)**

#### `parser.go`
- 입력: `[]byte` CODEOWNERS 본문
- 라인 단위 처리:
  - `#` 시작 / 빈 줄 → skip
  - `<pattern> <owner1> <owner2> ...` 형식 → `Rule{Pattern, Owners}` (owner 는 `@` prefix 제거 후 보존, `@org/team` 은 `org/team` 형식 유지)
  - 잘못된 라인 → skip + slog.Debug
- 출력: `[]Rule`

#### `matcher.go`
- 입력: `[]Rule`, `[]string changedFiles`
- 알고리즘 (PRD §10 R1 — GitHub "마지막 매치 우선" 단순 규칙):
  ```
  for file in changedFiles:
    matched := nil
    for rule in rules:    // 순서대로
      if rule.Pattern matches file: matched = rule  // 최후 매치가 winner
    if matched != nil: result.AddAll(matched.Owners)
  return result.Sorted()  // 결정성
  ```
- 패턴 매칭: `*`, `**`, 디렉토리 `/path/`, suffix `*.go`, prefix 등 — `path.Match` 또는 `doublestar/v4` 라이브러리
- 단위 테스트: GitHub 공식 docs 예제 5종 fixture (`docs/CODEOWNERS-examples.md` 참조 — 또는 인라인 fixture)

#### `cache.go`
- LRU (entries=64) + TTL (60s)
- 키: `repo full_name`, 값: `[]Rule` + fetched_at
- 미스 시 fetcher 호출
- ETag 미지원 (v1 — OI 외)

#### `client.go`
- `OwnersFor(ctx, repo, changedFiles []string) ([]string, error)`
  - 캐시 조회 → 미스면 GitHub fetch
  - GitHub fetch: 3 위치 순차 시도 (`.github/CODEOWNERS` → `CODEOWNERS` → `docs/CODEOWNERS`), 첫 매치만 캐시
  - 모두 404 → `ErrCodeownersNotFound` (sentinel) 반환 + 빈 slice
  - 5xx → 1회 재시도 (250ms backoff) → 여전히 실패 시 `ErrCodeownersFetch` + 빈 slice + slog.Warn
  - parser → matcher → 결과

**수락 기준**
- parser 5+ 케이스 테스트 (주석/와일드카드/팀/디렉토리/잘못된 라인)
- matcher 8+ 케이스: `*`, `**`, last-match wins, `*.go`, `/docs/`, prefix `src/api/`, 매치 없음, 다중 파일 union
- cache 테스트: hit/miss/TTL 만료/`-race` 동시성
- client 테스트 (mock GitHubClient):
  - 첫 위치 200 → 캐시 + 결과
  - 첫 위치 404 → 두 번째 시도 → 200
  - 모든 위치 404 → ErrCodeownersNotFound + 빈 slice
  - 5xx → 1 retry → 여전히 실패 시 빈 slice + Warn
- 커버리지 ≥ 90%

**Sub-agent**: `go-generator` + `go-tester`

### Step 6 — ReviewerResolver (`internal/github/reviewer_resolver.go`)

**작업 내용**
- 인터페이스:
  ```go
  type ReviewerResolver interface {
      Resolve(ctx context.Context, task *domain.Task, worktreePath string,
              repoCfg config.RepoConfig) (reviewers []string, warnings []string, err error)
  }
  ```
- 구현 (PRD §6.1 1:1):
  1. `changedFiles` 추출: `git -C <worktreePath> diff --name-only origin/<base>..HEAD` (실패 시 빈 slice + warning)
  2. strategy 분기:
     - `""` / `"config-only"` → `candidates = repoCfg.Reviewers`
     - `"codeowners-only"` → `candidates = codeowners.OwnersFor(...)` (404 면 빈 slice)
     - `"merge"` → `union(repoCfg.Reviewers, codeowners.OwnersFor(...))`
  3. `repoCfg.InheritIssueAssignee` true → `candidates ∪= task.IssueAssignees`
  4. `sanitize(candidates)`: self 제외 (`PRAuthor` startup 캐시값) / 대소문자 무시 중복 제거 / cut to 15 (config > codeowners > assignees 우선순위는 sanitize 단계에서 reorder)
  5. `TaskEvent{kind=reviewer_decided, payload={sources: {config:N, codeowners:N, assignees:N}, final:[...]}}` 1건 기록
  6. 반환

**수락 기준**
- 테이블 테스트 — PRD §6.3 의사결정 매트릭스 6 케이스 + `inherit_issue_assignee=true` 직교 6 케이스 = 12+ 케이스
- self exclude: candidates=["bot","u1"] + PRAuthor="bot" → 결과 ["u1"]
- > 15 인 경우 우선순위 cut 검증 + warning 발생
- changedFiles 추출 실패 시 codeowners-only 는 빈 slice + warning, merge 는 config 만
- `git diff` mock (또는 worktree fixture) 으로 실측 케이스 1+
- 커버리지 ≥ 90%

**Sub-agent**: `go-generator` + `go-tester`

### Step 7 — gh stderr partial-reject 파싱 (`internal/github/gh_reject_parser.go`)

**작업 내용**
- 입력: `gh pr create` stderr 텍스트 + non-zero exit code
- 정규식 (보수적): `(?m)^.*[Cc]ould not (?:request|add) reviewer (?:'|")?(\S+?)(?:'|")?\s*[:.]?$` 등 gh 2.x 메시지 패턴 — 실측 fixture 기반
- 출력: `(prURL string, rejected []string, isPartial bool)`
  - PR URL 이 stdout 또는 stderr 에 포함되어 있으면 `isPartial=true` (PR 자체는 생성됨)
  - 없으면 `isPartial=false` → 호출자가 "전체 실패" 로 fallback (안전 측 — PRD §10 R2)

**수락 기준**
- gh 2.x stderr fixture 5+ 케이스:
  - 단일 reviewer 거부 + PR URL 정상 출력 → isPartial=true, rejected=["user1"]
  - 다중 거부 + PR URL → rejected 다수
  - PR URL 없음 + 거부 → isPartial=false (전체 실패)
  - 미지의 메시지 → isPartial=false (안전 측)
- 정규식 보수성: 무관한 라인 무시
- 커버리지 ≥ 90%

**Sub-agent**: `go-generator` + `go-tester`

### Step 8 — pr_creator.go 수정

**작업 내용**
- 기존 `--reviewer` flatten 위치 (line 93~95) 를 `ReviewerResolver.Resolve(...)` 결과로 대체
- `gh pr create` 실행 후 stderr/stdout 캡처:
  - 성공 (exit 0) → 정상 PR URL 반환
  - exit non-zero → `gh_reject_parser` 통해 분석:
    - `isPartial=true` → PR URL + rejected slice 반환, `TaskEvent{kind=reviewer_warning, payload={rejected, reason}}` + Slack `NotifyReviewerWarning` 발송, 함수는 PR URL 반환 (성공 처리)
    - `isPartial=false` → 기존처럼 에러 반환
- candidate 가 빈 slice 면 `--reviewer` 플래그 자체 생략 (gh 가 빈 값 거부 안 하도록)

**수락 기준**
- 회귀 테스트 (기존 `pr_creator_test`):
  - reviewer_strategy 미설정 + reviewers=["u1"] → ghArgs 가 `--reviewer u1` 단일 (v0 동작과 바이트 동일 — US-1 수락 1)
  - reviewers=[] → `--reviewer` 미포함
- 신규 테스트:
  - merge 모드 + CODEOWNERS hit → 합집합 reviewer
  - codeowners-only + 404 → reviewer 미지정 + slog.Warn
  - partial-reject → PR URL 반환 + reviewer_warning 이벤트 1건 + Slack 호출 1회
  - 전체 실패 → 에러 반환 (기존 동작)
- 커버리지 ≥ 85%

**Sub-agent**: `go-modifier` + `go-tester`

### Step 9 — Poller / Worker / API 통합

**작업 내용**
1. `internal/github/poller.go`:
   - `detectTaskType` 인근에서 `issue.Assignees` 의 `Login` slice 추출 → `Task.IssueAssignees` 로 enqueue
2. `internal/scheduler/worker.go` `createPR`:
   - `ReviewerResolver` 호출, worktree path 전달 (이미 가능 — 기존 함수 시그니처 활용)
3. `internal/api/dto.go` `TaskResponse`:
   - `IssueAssignees []string \`json:"issue_assignees" example:"[\"user1\"]"\``
   - `ReviewerWarnings []string \`json:"reviewer_warnings,omitempty" example:"[\"user2\"]"\``
4. `GetTask` handler godoc `@Success` 모델 업데이트 (DTO 변경만 — 신규 핸들러는 없음)

**수락 기준**
- poller_test: assignees 가 task.IssueAssignees 에 저장됨
- API e2e (httptest): `GET /tasks/{id}` 응답에 신규 2개 필드 포함, swag 재생성 후 노출
- 커버리지 ≥ 80%

**Sub-agent**: `go-modifier` + `go-tester`

### Step 10 — DI 조립 (`cmd/scheduled-dev-agent/main.go`)

**작업 내용**
- startup 시 `prAuthor, err := ghClient.GetAuthenticatedUser(ctx)` 호출 (실패 시 `prAuthor=""` + Warn — sanitize self-exclude 동작 안 함, gh 가 거부 처리)
- `coCache := codeowners.NewCache(64, 60*time.Second)`
- `coClient := codeowners.NewClient(ghClient, coCache)`
- `resolver := github.NewReviewerResolver(coClient, prAuthor, slog)`
- `prCreator := github.NewPRCreator(ghClient, resolver, slackClient)` (기존 시그니처 확장)
- worker 에 `prCreator` 주입

**수락 기준**
- `go build ./...` 통과
- `gh api user` 실패 케이스도 부팅 정상 (Warn 후 진행)

**Sub-agent**: `go-modifier`

### Step 11 — 검증

```bash
sqlc generate                            # 본 기능은 sqlc 변경 없음, 기존 그대로 통과
mockery --name=Codeowners        --dir=internal/github/codeowners --output=mocks
mockery --name=ReviewerResolver  --dir=internal/github            --output=mocks
mockery --name=GitHubClient      --dir=internal/github            --output=mocks
go build ./...
go test -race -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | tail -1                                    # ≥80%
go tool cover -func=coverage.out | grep -E '(codeowners|reviewer_resolver)'   # ≥85%
go vet ./...
golangci-lint run ./...
swag init -g cmd/scheduled-dev-agent/main.go -o docs
```

**수락 기준 (전체)**
- 모든 명령 0 exit
- `.claude/hooks/pre-push.sh` 통과
- swag 재생성 후 `TaskResponse` 에 `issue_assignees`, `reviewer_warnings` 노출
- 회귀 0건: 기존 yaml (reviewer_strategy 미설정) → 기존 ghArgs 와 바이트 단위 동일 (US-1 핵심)

---

## 2. PRD 수락 기준 매핑

| PRD Goal | 검증 |
|----------|------|
| G1. 기존 동작 100% (회귀 0) | pr_creator_test US-1 케이스 — ghArgs byte-identical |
| G2. CODEOWNERS 자동 (merge / only) | resolver_test 의 §6.3 매트릭스 |
| G3. inherit_issue_assignee 직교 적용 | resolver_test 의 12 직교 케이스 |
| G4. reviewer 실패 → PR 자체는 성공 | gh_reject_parser_test + pr_creator_test partial 케이스 |
| G5. self / 빈 후보 자동 제외 | reviewer_sanitize_test |
| G6. team `@org/team` 동일 처리 | parser_test + resolver_test 의 team 케이스 |
| G7. CODEOWNERS API ≤ 1회/PR (TTL 60s) | cache_test hit/miss 검증 |

| PRD User Story | 검증 |
|----------------|------|
| US-1 (기존 동작 유지) | pr_creator_test 회귀 |
| US-2 (codeowners-only) | resolver_test |
| US-3 (merge) | resolver_test + 15 cap |
| US-4 (assignee 상속) | poller_test + resolver_test 직교 |
| US-5 (sanitize / partial-reject) | sanitize_test + gh_reject_parser_test |
| US-6 (team) | parser_test |
| US-7 (CODEOWNERS 부재) | client_test 404 케이스 |
| US-8 (캐시) | cache_test |

---

## 3. CLAUDE.md NEVER 체크리스트

- [ ] `domain/` 외부 패키지 import 금지 — `IssueAssignees []string` 만 추가
- [ ] raw SQL 금지 — issue_assignees 는 GORM 컬럼만 변경, 추가 SQL 없음
- [ ] 전역 var db 금지 — 모든 의존 cmd/main.go 주입
- [ ] `context.Background()` 핸들러 직접 사용 금지
- [ ] swag godoc 없는 endpoint 추가 금지 — 본 기능은 신규 endpoint 없음, `GetTask` `@Success` 모델만 갱신
- [ ] 기존 migration 파일 수정 금지 — 000004 신규만
- [ ] 테스트 없이 UseCase 메서드 추가 금지 — 모든 신규 메서드 단위 테스트 동시 추가
- [ ] secret/PII 로그 금지 — CODEOWNERS 응답에 user email 포함될 수 있음 → 로그 출력은 user login 만 (PRD §9 보안)
- [ ] `panic()` 금지 — 모든 에러 반환 + degrade
- [ ] `db/sqlc/` 수동 수정 금지

---

## 4. OI / 후속 결정 (PRD §12 와 동기화)

- **OI-1** (assignee 스냅샷 vs PR 직전 refetch): v1 은 **task 큐 시점 스냅샷** 사용 — poller 에서 캡처. PR 직전 refetch 는 v2 후보
- **OI-2** (CODEOWNERS 위치): `.github/CODEOWNERS` → `CODEOWNERS` → `docs/CODEOWNERS` 순. 첫 매치만 캐시
- **OI-3** (default `reviewer_strategy`): v1 은 `config-only` (회귀 안전 우선). config.example.yaml 에는 `merge` 예시를 주석으로 노출하여 옵트인 유도

---

## 5. 후속 수동 작업

```bash
# config.example.yaml 에 신규 옵션 예시 추가 (PRD §8 yaml 그대로)
github:
  repos:
    - name: "owner/repo"
      reviewers: ["user1"]
      reviewer_strategy: "merge"        # config-only | codeowners-only | merge
      inherit_issue_assignee: false

# README "Reviewer Assignment" 섹션 추가:
# - 모드 3종 차이
# - CODEOWNERS 캐시 TTL 60s 설명
# - GITHUB_TOKEN 의 contents 읽기 권한 필요 명시 (PRD §4 권한)
```
