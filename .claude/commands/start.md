---
description: 신규 기능 시작 — worktree + PRD + 역할 프롬프트 + 자동 구현까지 한 번에. 새 기능 개발의 기본 진입점.
argument-hint: <기능 설명> [--no-worktree | --output-only | --marketing | --sales | --gtm]
---

새 기능 개발을 한 번에 시작합니다.
`/new worktree` + `/plan` + `planner agent` + 자동 실행을 묶은 단일 진입점입니다.

**요청:** $ARGUMENTS

---

## Step 1 — 플래그 파싱

`$ARGUMENTS` 에서 분리:

| 플래그 | 동작 |
|--------|------|
| `--no-worktree` | worktree 생성 없이 현재 위치에서 진행 (이미 worktree 안일 때) |
| `--output-only` | PRD/프롬프트만 생성, generator agent 실행 없음 |
| `--marketing` / `--sales` / `--gtm` | GTM 문서 추가 생성 (`/plan` 의 동일 플래그와 호환) |

플래그 외 나머지 = `raw_request`.

## Step 2 — feature 이름 결정

`raw_request` 에서 kebab-case 이름 추출 (예: "로그인 기능" → `login`).

애매하면 한 줄로 묻습니다:
> "기능 이름을 kebab-case 로 알려주세요 (예: `login`, `payment-cancel`):"

## Step 3 — Worktree 생성 (`--no-worktree` 가 아니면)

`/new worktree feature-{name}` 와 동일 로직 — 자세한 규칙은 `.claude/commands/new.md` 의 worktree 섹션 참고.

요약:
1. `.gitignore` 에 `.worktrees/` 등록 (없으면)
2. `git fetch origin --quiet || true`
3. base ref 결정: 원격 `dev` 있으면 `origin/dev`, 없으면 `origin/main`
4. `git worktree add .worktrees/feature-{name} -b feature/{name} "$BASE_REF"`
5. 스택별 의존성 자동 설치 (gradle / npm / go / flutter / uv)
6. **이후 모든 작업은 worktree 디렉토리 기준** — 사용자에게 1줄로 보고:
   > `→ worktree 생성: .worktrees/feature-{name} (base: $BASE_LABEL)`

이미 `.worktrees/` 안에서 호출됐으면 자동으로 `--no-worktree` 동작.

## Step 4 — 모드 판정

```bash
[ -f .claude/stacks.json ] && MODE="monorepo" || MODE="single"
```

## Step 5 — 인터뷰 (요청이 모호한 경우만)

요청이 명확하면 **0개 질문으로 통과**. 모호하면 한 번에 최대 3개.

**판정 기준** — `raw_request` 가 다음을 모두 포함하면 명확:
- 핵심 시나리오가 한 문장으로 설명됨
- 어느 레이어에 영향 (인증/결제/UI 등)이 식별됨
- 모노레포면 어느 스택에 걸쳐 있는지 명시 또는 자명

부족할 때만 묻습니다 (필요한 것만):
- **시나리오**: 주요 사용자 플로우와 성공 기준
- **스코프**: 이번 이터레이션 vs 후속
- **(모노레포)** 스택 범위: backend only / full-stack / mobile 포함

## Step 6 — `planner` agent 호출

```
Agent(
  subagent_type="planner",
  prompt="raw_request=<원문>, feature_name=<kebab>, interview_answers=<요약>,
          mode=<single|monorepo>, stacks=<.claude/stacks.json 또는 단일 감지 결과>"
)
```

planner 가 PRD (`docs/specs/{feature}.md`) + 역할 프롬프트 (`docs/specs/{feature}/{role}.md` 또는 `docs/specs/{feature}-prompt.md`) 생성. 리포트를 사용자에게 그대로 보여줍니다.

## Step 6.5 — GTM 문서 (플래그 있을 때만)

`--marketing` / `--sales` / `--gtm` 중 하나 이상이면 `gtm-planner` agent 호출. `/plan` 의 GTM 동작과 동일.

**플래그 매핑:**
- `--marketing` → `["marketing"]`
- `--sales` → `["sales"]`
- `--gtm` → `["marketing", "sales"]`

## Step 7 — 자동 실행 (디폴트)

> **핵심 디폴트 변화**: 단일 스택은 묻지 않고 바로 generator agent 실행. 모노레포만 1회 확인.

### 7-a. 단일 스택 모드 (`MODE=single`)

`--output-only` 가 없으면 **즉시** 루트 스택의 generator agent 호출:

| 감지된 스택 | 호출할 agent |
|-------------|-------------|
| `kotlin` / `kotlin-multi` | `kotlin-generator` |
| `go` / `go-multi` | `go-generator` |
| `python` / `python-multi` | `python-generator` |
| `nextjs` / `nextjs-multi` | `nextjs-generator` |
| `flutter` | `flutter-generator` |
| (코드 스택 없음 — marketing/sales/product) | 실행 생략, output-only 로 폴백 |

agent prompt: `docs/specs/{feature}-prompt.md` 본문 + 머리에 다음 지시:

> 이 작업은 `/start` 가 생성한 프롬프트에 따라 진행합니다. Step 별로 순서대로 수행하고, 완료 후 생성한 파일 목록을 짧게 리포트하세요.

### 7-b. 모노레포 모드 (`MODE=monorepo`)

`--output-only` 가 없으면 **1회만** 확인:

> ```
> {N}개 service 에 대해 generator agent 를 병렬 실행합니다:
>   - backend:auth (kotlin-multi)
>   - backend:ml   (python)
>   - frontend     (nextjs)
>
> 진행할까요? (yes / no, 기본 yes)
> ```

`yes` (또는 빈 입력) → 각 service 의 프롬프트 파일을 generator agent 로 **단일 메시지에서 병렬** 실행 (`/plan` Teams 모드와 동일).

**Service 별 프롬프트 파일명:**
- `name` 없음 → `docs/specs/{feature}/{role}.md`
- `name` 있음 → `docs/specs/{feature}/{role}-{name}.md`

### 7-c. Output-only 모드 (`--output-only`)

파일만 생성하고 다음 안내:

```
✅ 기획 완료 — 파일만 생성됐습니다

PRD:  docs/specs/{feature}.md
프롬프트:
  docs/specs/{feature}/backend.md       → /new backend api <Resource>
  docs/specs/{feature}/frontend.md      → /new frontend component <Name>
  docs/specs/{feature}/mobile.md        → /new mobile screen <Name>

다음:
  필요 시 위 명령으로 수동 실행하거나, 다시 /start 를 실행해 자동 모드로 진행하세요.
```

## Step 8 — 완료 보고

```
✅ {feature} 시작 완료

worktree:    .worktrees/feature-{name}/   (base: $BASE_LABEL)
PRD:         docs/specs/{feature}.md
역할 프롬프트:
  - docs/specs/{feature}/backend.md   (kotlin-multi)
  - docs/specs/{feature}/frontend.md  (nextjs)

[generator 실행 결과]
  backend:  생성 5개 / 수정 2개
  frontend: 생성 4개

다음:
  /commit                   → 변경사항 커밋
  /pr                       → PR 생성 (base: $BASE_LABEL 자동 감지)
  /merge                    → 머지 + 태그 + worktree 정리
```

---

## 사용 예시

```bash
# 가장 흔한 경우 — 신규 기능 시작
/start 로그인 기능
  → worktree(feature/login) + PRD + 단일 스택 자동 실행

# 모노레포 풀스택
/start 결제 취소 기능
  → worktree + PRD + 3개 service 프롬프트 + 1회 확인 후 병렬 실행

# 이미 worktree 안이거나 별도 워크트리 안 만들고 싶을 때
/start 비밀번호 재설정 --no-worktree

# 파일만 생성, 수동 실행 예정
/start 댓글 기능 --output-only

# GTM 포함 (마케팅 + 세일즈 전략까지)
/start 유료 플랜 런치 --gtm

# 마케팅 전략만 추가 (단일 스택 자동 실행 + 마케팅 문서)
/start 블로그 발행 --marketing
```

---

## `/plan` / `/new` 와의 관계

| 상황 | 사용 커맨드 |
|------|-------------|
| **신규 기능 시작** (대부분) | `/start <기능>` ← 권장 진입점 |
| **이미 worktree 안에서 추가 설계만** | `/plan <기능>` (PRD + 역할 프롬프트, 실행 없음) |
| **API 계약 합의만** | `/plan api <Resource>` |
| **DB 스키마 합의만** | `/plan db <도메인>` |
| **가벼운 단일 변경 계획** | `/plan <설명> --light` |
| **개별 리소스 스캐폴딩** | `/new api/component/screen <Name>` |
| **별도 worktree 만 만들기** | `/new worktree <type-name>` |

---

## 주의사항

- **planner agent 는 코드를 쓰지 않습니다.** PRD/프롬프트만 산출. 구현은 generator agent.
- **`--output-only` 가 없으면 디폴트는 자동 실행** — 단일 스택은 무확인, 모노레포만 1회 확인.
- **모노레포 인터랙션 1회 한정** — 이전 `/planner` 의 두 단계(실행 모드 선택 + 안전장치 yes/no) 가 1회로 통합.
- **기존 PRD 덮어쓰기 확인 필수** — `docs/specs/{feature}.md` 가 있으면 사용자에게 (덮어쓰기 / 갱신 / 중단) 묻습니다.
- **단일 스택 모드의 자동 실행** — 이전엔 사용자에게 (a/b) 매번 물었으나, 이제 디폴트가 자동. `--output-only` 로 옵트아웃.
- **`docs/specs/` + `docs/gtm/` 는 팀 공유 대상** — `.gitignore` 하지 않습니다.
