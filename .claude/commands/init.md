---
description: 프로젝트 스택 선언 → 불필요한 agent/template/skill 제거 → CLAUDE.md + settings.json 설치 + Second Brain 초기화
argument-hint: [kotlin | kotlin-multi | go | go-multi | python | python-multi | nextjs | nextjs-multi | flutter | monorepo | marketing | sales | product] (생략 시 자동 감지)
---

프로젝트의 스택을 설정하고 하네스를 구성합니다.
**모노레포** (backend + frontend + mobile 공존), **단일 스택**, **코드 없는 마케팅/세일즈 전담 모드** 를 지원합니다.

**선택한 스택:** $ARGUMENTS

---

## Step 1 — 스택 확인

### 1-1. `$ARGUMENTS` 해석

| 값 | 의미 |
|----|------|
| `kotlin` / `kotlin-multi` | Spring Boot 백엔드 (단일 / Gradle 멀티 모듈) |
| `go` / `go-multi` | Go Gin 백엔드 (단일 / Workspace 멀티 서비스) |
| `python` / `python-multi` | Python FastAPI 백엔드 (단일 / uv Workspace) |
| `nextjs` / `nextjs-multi` | Next.js 프론트엔드 (단일 / Turborepo) |
| `flutter` | Flutter 모바일 |
| `monorepo` | backend + frontend + mobile 모노레포 (자동 감지 강제) |
| `marketing` | 코드 없는 마케팅 전담 프로젝트 (랜딩 카피 · SEO · 콘텐츠 · 광고 · 이메일) |
| `sales` | 코드 없는 세일즈 전담 프로젝트 (덱 · 콜드메일 · 객관처리 · 가격 · 플레이북) |
| `product` | 코드 없는 Product Management 전담 (Discovery · Strategy · PRD · OKR · GTM · Analytics) |

### 1-2. 자동 감지 (인수 없을 때)

**먼저 모노레포 감지 시도**. 아래 역할 후보 디렉토리들을 **전부** 스캔 (동일 역할 다중 허용):

| 역할 | 허용 디렉토리 이름 패턴 (별칭 + suffix 허용) |
|------|---------------------------------------------|
| backend | `backend`, `api`, `server`, `backend-*`, `api-*`, `server-*` |
| frontend | `frontend`, `web`, `client`, `frontend-*`, `web-*`, `client-*` |
| mobile | `mobile`, `app`, `mobile-*`, `app-*` |

**이름 추출**:
- 기본 이름 (`backend`, `web`, `app` 등) → `name` 필드 생략 (role 이 유일한 경우)
- suffix 패턴 (`backend-auth`, `backend-ml`, `web-admin` 등) → suffix 부분이 `name` (`auth`, `ml`, `admin`)
- 동일 role 이 2개 이상이면 각 스택은 **반드시 `name`** 을 가져야 함 (없으면 사용자에게 수동 지정 요청)

각 디렉토리에서 아래 마커로 스택 타입 판정:

| 마커 (역할 디렉토리 기준) | 스택 타입 |
|---------------------------|-----------|
| `settings.gradle.kts` + `include(` | `kotlin-multi` |
| `build.gradle.kts` / `pom.xml` | `kotlin` |
| `go.work` | `go-multi` |
| `go.mod` | `go` |
| `pyproject.toml` + `[tool.uv.workspace]` | `python-multi` |
| `pyproject.toml` (fastapi 의존성) | `python` |
| `turbo.json` | `nextjs-multi` |
| `package.json` (`next` 의존성) | `nextjs` |
| `pubspec.yaml` | `flutter` |

**모노레포 판정 규칙:**

- **2개 이상** service 디렉토리 (동일 role 포함) 가 각자 유효한 스택 마커를 가지면 → `monorepo` 모드
- **1개** service 만 발견 → 사용자에게 "단일 스택으로 진행할까요, 아니면 단일-service 모노레포로 구성할까요?" 확인
- **0개** 발견 → 루트에서 기존 단일 스택 감지 (아래 표)

**동일 role 다중 감지 예시:**
```
my-project/
├── backend-auth/     (kotlin-multi)  → role=backend, name=auth
├── backend-ml/       (python)         → role=backend, name=ml
├── web/              (nextjs)         → role=frontend, name=web (또는 생략)
└── app/              (flutter)        → role=mobile,  name=app (또는 생략)
```
→ `stacks.json` 에 4개 service 등록, 같은 role 이어도 `name` 으로 구분됨

**이름 중복 금지**: 동일 role 내에서 `name` 이 같은 service 2개 불가. 감지 시 충돌이면 사용자에게 수정 요청.

> ⚠️ **marketing/sales/product 모드는 자동 감지하지 않습니다.** 코드 마커가 전혀 없을 때에도 이 모드들을 가정하지 마세요. 빈 디렉토리일 수 있으므로 사용자에게 **명시 선택**을 요청합니다:
> ```
> 코드 스택이 감지되지 않았습니다. 아래 중 선택하세요:
>   1. marketing  — 마케팅 전담 프로젝트 (코드 없음)
>   2. sales      — 세일즈 전담 프로젝트 (코드 없음)
>   3. product    — Product Management 전담 프로젝트 (코드 없음, pm-skills 플러그인 사용)
>   4. 취소 — 스택을 직접 명시 (예: /init kotlin)
> ```

**루트 단일 스택 감지 (폴백):**

| 감지 파일 | 스택 |
|-----------|------|
| `settings.gradle.kts` + `include(` 포함 | `kotlin-multi` |
| `build.gradle.kts` / `pom.xml` | `kotlin` |
| `go.work` | `go-multi` |
| `go.mod` | `go` |
| `pyproject.toml` + `[tool.uv.workspace]` | `python-multi` |
| `pyproject.toml` (`fastapi` 의존성) | `python` |
| `turbo.json` | `nextjs-multi` |
| `package.json` (`next` 의존성) | `nextjs` |
| `pubspec.yaml` | `flutter` |

감지 결과를 사용자에게 확인:
> "backend/ (kotlin-multi) + web/ (nextjs) + app/ (flutter) 모노레포를 감지했습니다. 이대로 진행할까요?"
>
> 또는
>
> "build.gradle.kts 를 감지했습니다. `kotlin` 단일 스택으로 진행할까요?"

감지 불가 시 직접 물어봅니다.

---

## Step 2 — 파일 정리

### 단일 스택 모드 — 유지 대상

| 스택 | 유지 agents | 유지 skills | 유지 templates |
|------|-------------|-------------|----------------|
| `kotlin` / `kotlin-multi` | kotlin-{gen,mod,test}, code-reviewer, **security-reviewer**, api-designer, ui-designer¹, github-actions-designer, **planner** | kotlin-patterns, db-patterns, api-design-patterns, github-actions-patterns, **security-patterns**, **docker-patterns**, **cache-patterns** | CLAUDE.kotlin[-multi], settings.kotlin[-multi], **prd**, **role-prompt** |
| `go` / `go-multi` | go-{gen,mod,test}, code-reviewer, **security-reviewer**, api-designer, github-actions-designer, **planner** | go-patterns, db-patterns, api-design-patterns, github-actions-patterns, **security-patterns**, **docker-patterns**, **cache-patterns** | CLAUDE.go[-multi], settings.go[-multi], **prd**, **role-prompt** |
| `python` / `python-multi` | python-{gen,mod,test}, **ai-{researcher,gen,mod,test}**, code-reviewer, **security-reviewer**, api-designer, github-actions-designer, **planner** | python-patterns, **ai-patterns**, db-patterns, api-design-patterns, github-actions-patterns, **security-patterns**, **docker-patterns**, **cache-patterns** | CLAUDE.python[-multi], settings.python[-multi], **prd**, **role-prompt** |
| `nextjs` / `nextjs-multi` | nextjs-{gen,mod,test}, code-reviewer, **security-reviewer**, ui-designer, github-actions-designer, **planner** | nextjs-patterns, ui-design-impl, github-actions-patterns, **security-patterns**, **docker-patterns**, **cache-patterns** | CLAUDE.nextjs[-multi], settings.nextjs[-multi], **prd**, **role-prompt** |
| `flutter` | flutter-{gen,mod,test}, code-reviewer, **security-reviewer**, ui-designer, github-actions-designer, **planner** | flutter-patterns, ui-design-impl, github-actions-patterns, **security-patterns** | CLAUDE.flutter, settings.flutter, **prd**, **role-prompt** |
| `marketing` | code-reviewer, **planner**, **gtm-planner** | (없음 — marketing-skills 플러그인 의존) | CLAUDE.marketing, settings.marketing, **prd**, **role-prompt**, **marketing-plan**, **gtm-history**, **memory** |
| `sales` | code-reviewer, **planner**, **gtm-planner** | (없음 — marketing-skills 플러그인 의존) | CLAUDE.sales, settings.sales, **prd**, **role-prompt**, **sales-plan**, **gtm-history**, **memory** |
| `product` | code-reviewer, **planner**, **gtm-planner** | (없음 — pm-skills 마켓플레이스 + marketing-skills 플러그인 의존) | CLAUDE.product, settings.product, **prd**, **role-prompt**, **marketing-plan**, **sales-plan**, **gtm-history**, **memory** |

¹ `ui-designer` 는 백엔드 단독일 땐 제거. 모노레포에서는 frontend/mobile 이 있으면 자동 유지.

> `planner` agent 와 `prd`/`role-prompt` 템플릿은 **모든 스택에서 유지**합니다. `/start` · `/plan` 커맨드가 이들을 사용합니다.
>
> **marketing/sales/product 모드**: 코드 스택 관련 agent (`kotlin-*`, `go-*`, `python-*`, `nextjs-*`, `flutter-*`, `ui-designer`, `api-designer`, `github-actions-designer`) 와 코드 관련 skills (`*-patterns`, `db-patterns`, `api-design-patterns`, `ui-design-impl`, `github-actions-patterns`) · 템플릿 (`CLAUDE.{코드스택}.md`, `settings.{코드스택}.json`) 을 **모두 제거**합니다. 작업은 marketing/sales 는 `marketing-skills` 플러그인, product 는 `pm-skills` 마켓플레이스 8개 플러그인(+ `marketing-skills`) 로 처리합니다.

### 모노레포 모드 — 유지 대상 (유니온)

감지된 스택들의 "유지 대상" **합집합**을 적용:

- **backend (kotlin/kotlin-multi)** 감지 → kotlin-{gen,mod,test}, api-designer, kotlin-patterns, db-patterns, api-design-patterns, **docker-patterns**, **cache-patterns**, CLAUDE.kotlin[-multi], settings.kotlin[-multi]
- **backend (go/go-multi)** 감지 → go-{gen,mod,test}, api-designer, go-patterns, db-patterns, api-design-patterns, **docker-patterns**, **cache-patterns**, CLAUDE.go[-multi], settings.go[-multi]
- **backend (python/python-multi)** 감지 → python-{gen,mod,test}, **ai-{researcher,gen,mod,test}**, api-designer, python-patterns, **ai-patterns**, db-patterns, api-design-patterns, **docker-patterns**, **cache-patterns**, CLAUDE.python[-multi], settings.python[-multi]
- **frontend (nextjs/nextjs-multi)** 감지 → nextjs-{gen,mod,test}, ui-designer, nextjs-patterns, ui-design-impl, **docker-patterns**, **cache-patterns**, CLAUDE.nextjs[-multi], settings.nextjs[-multi]
- **mobile (flutter)** 감지 → flutter-{gen,mod,test}, ui-designer, flutter-patterns, ui-design-impl, CLAUDE.flutter, settings.flutter
- **공통 유지**: code-reviewer, **security-reviewer**, github-actions-designer, **planner**, github-actions-patterns, **security-patterns**, CLAUDE.monorepo.md, settings.monorepo.json, memory.md, **prd.md**, **role-prompt.md**

### 제거 대상

유지 목록에 없는 `.claude/agents/`, `.claude/skills/`, `.claude/templates/` 하위 파일 제거. `.github/assets/` (스타터 대표 이미지) 제거.

> **커맨드는 전부 유지**합니다. `/new`, `/plan`, `/review` 는 역할 prefix 로 스택 분기를 내부 처리합니다.

사용자에게 유지/제거 목록을 보여주고 확인을 받습니다.

---

## Step 3 — 하네스 파일 설치

> 🔒 **CLAUDE.md ≤ 300줄 캡 (모든 모드 공통)** — 설치하거나 병합한 모든 CLAUDE.md (루트, 역할별 sub-CLAUDE.md 포함) 의 줄 수를 검사합니다. 300줄을 넘으면 초과분을 `.claude/skills/{topic}.md` 또는 `docs/{topic}.md` 로 이관하고 CLAUDE.md 에는 ``상세: `.claude/skills/{topic}.md`` 한 줄로 인덱스만 남깁니다. 사용자에게 이관 결과를 보고하고 확인을 받습니다.

### 3-A. 단일 스택 모드

#### 3-A-1. CLAUDE.md (루트)

**신규 프로젝트** (CLAUDE.md 없음):
```bash
cp .claude/templates/CLAUDE.{stack}.md ./CLAUDE.md
```

**기존 프로젝트**: 기존 파일에 템플릿의 "아키텍처 규칙", "반드시 지켜야 할 규칙", "절대 하면 안 되는 것" 섹션을 병합. 덮어쓰기 전 사용자 확인.

사용자에게 프로젝트명을 물어 `CLAUDE.md` 첫 줄의 `[프로젝트명]` 을 교체.

#### 3-A-2. settings.json

**신규**:
```bash
cp .claude/templates/settings.{stack}.json ./.claude/settings.json
```

**기존**: `hooks` + `permissions` 만 템플릿으로 업데이트. `enabledPlugins` 등 기존 설정 유지.

> `.claude/stacks.json` 은 단일 스택 모드에선 **생성하지 않습니다.** `pre-push.sh` 는 자동으로 루트 감지 폴백으로 동작합니다.

---

### 3-B. 모노레포 모드

#### 3-B-1. `.claude/stacks.json` 생성 (단일 진실의 원천)

감지 결과로 `.claude/stacks.json` 을 작성합니다. **동일 role 이 여러 개 있어도 됩니다** (`name` 으로 구분).

**단일 스택 예시** (기존 호환):
```json
{
  "mode": "monorepo",
  "stacks": [
    { "role": "backend",  "type": "kotlin-multi", "path": "backend" },
    { "role": "frontend", "type": "nextjs",       "path": "web" },
    { "role": "mobile",   "type": "flutter",      "path": "app" }
  ]
}
```

**다중 backend 예시** (신규):
```json
{
  "mode": "monorepo",
  "stacks": [
    { "role": "backend",  "name": "auth", "type": "kotlin-multi", "path": "backend-auth" },
    { "role": "backend",  "name": "ml",   "type": "python",       "path": "backend-ml" },
    { "role": "frontend", "type": "nextjs", "path": "web" }
  ]
}
```

**스키마 규칙:**
- `role` — 표준 이름 (`backend`/`frontend`/`mobile`) — 별칭 디렉토리를 써도 이 값은 표준
- `name` — **optional** (role 이 유일할 때 생략 가능). 동일 role 이 2개 이상이면 **필수**. 충돌 금지
- `type` — 실제 감지된 스택 타입
- `path` — 실제 발견된 디렉토리명

**name 자동 추출 규칙:**
- 디렉토리명이 `backend`, `api`, `server`, `frontend`, `web`, `client`, `mobile`, `app` 같은 기본 별칭이면 → `name` 생략
- 디렉토리명이 `backend-auth`, `web-admin` 처럼 suffix 가 있으면 → suffix 가 `name`
- 사용자가 감지 결과를 확인할 때 수정 가능

이 파일은 `/new`, `/start`, `/plan`, `.claude/hooks/pre-push.sh`, `settings.monorepo.json` hooks 가 모두 읽습니다.

**Service 식별자 (통칭 `service-id`)**:
- `name` 있으면 → `role:name` (예: `backend:auth`, `backend:ml`)
- `name` 없으면 → `role` 그대로 (예: `backend`, `frontend`)

#### 3-B-2. 루트 CLAUDE.md (인덱스)

```bash
cp .claude/templates/CLAUDE.monorepo.md ./CLAUDE.md
```

템플릿의 플레이스홀더를 실제 값으로 치환:
- `[프로젝트명]` → 사용자 입력
- `[backend-path]`, `[backend-stack]` → stacks.json 값
- `[frontend-path]`, `[frontend-stack]` → 동일
- `[mobile-path]`, `[mobile-stack]` → 동일
- 감지 안 된 역할의 행은 표에서 **제거**

#### 3-B-3. 역할별 CLAUDE.md (하위 디렉토리)

각 역할에 대해:

```bash
cp .claude/templates/CLAUDE.{type}.md ./{path}/CLAUDE.md
```

예:
```bash
cp .claude/templates/CLAUDE.kotlin-multi.md ./backend/CLAUDE.md
cp .claude/templates/CLAUDE.nextjs.md       ./web/CLAUDE.md
cp .claude/templates/CLAUDE.flutter.md      ./app/CLAUDE.md
```

각 하위 CLAUDE.md 첫 줄 `[프로젝트명]` 도 교체하고, 프로젝트명 뒤에 역할명 추가:
> `# MyApp / backend — Kotlin Spring Boot (Gradle Multi-Module)`

**기존 CLAUDE.md 가 하위 디렉토리에 이미 있는 경우**: 덮어쓰기 전 확인, 병합은 단일 스택 모드와 동일 규칙.

#### 3-B-4. 루트 settings.json (병합 템플릿)

```bash
cp .claude/templates/settings.monorepo.json ./.claude/settings.json
```

감지된 스택에 따라 `permissions.allow` 를 **실제 필요한 것만** 남기도록 후처리:
- backend(kotlin/kotlin-multi) 없음 → `Bash(./gradlew *)`, `Bash(./mvnw *)` 제거
- backend(go/go-multi) 없음 → `Bash(go *)` 제거
- backend(python/python-multi) 없음 → `Bash(uv *)`, `Bash(uvx *)`, `Bash(python *)`, `Bash(pytest *)`, `Bash(ruff *)`, `Bash(mypy *)`, `Bash(alembic *)`, `Bash(uvicorn *)` 제거
- frontend 없음 → `Bash(npm *)`, `Bash(npx *)`, `Bash(node *)` 제거
- mobile 없음 → `Bash(flutter *)`, `Bash(dart *)` 제거

`enabledPlugins` 도 필요 없는 플러그인 (예: kotlin-lsp when no kotlin) 제거.

`.claude/settings.json` 은 **루트 1개만** 유효. 하위 디렉토리에 별도 `.claude/` 를 만들지 않습니다.

---

### 3-C. Marketing / Sales / Product 모드

코드 스택이 없는 **문서 전담 프로젝트**입니다. `.claude/stacks.json` 은 **생성하지 않습니다** (코드 빌드/테스트 훅이 의미 없음).

#### 3-C-1. CLAUDE.md (루트)

**신규 프로젝트** (CLAUDE.md 없음):
```bash
cp .claude/templates/CLAUDE.marketing.md ./CLAUDE.md   # marketing 모드
cp .claude/templates/CLAUDE.sales.md     ./CLAUDE.md   # sales 모드
cp .claude/templates/CLAUDE.product.md   ./CLAUDE.md   # product 모드
```

**기존 프로젝트**: 기존 파일에 템플릿의 "필수 플러그인", "디렉토리 구조", "MUST", "NEVER" 섹션을 병합. 덮어쓰기 전 사용자 확인.

사용자에게 프로젝트명을 물어 `CLAUDE.md` 첫 줄의 `[프로젝트명]` 을 교체.

#### 3-C-2. settings.json

**신규**:
```bash
cp .claude/templates/settings.marketing.json ./.claude/settings.json   # marketing 모드
cp .claude/templates/settings.sales.json     ./.claude/settings.json   # sales 모드
cp .claude/templates/settings.product.json   ./.claude/settings.json   # product 모드
```

**기존**: `hooks` + `permissions` + `enabledPlugins` 를 템플릿으로 덮어쓰기. 다른 기존 `enabledPlugins` 는 유지 가능.

> **코드 빌드/테스트 훅 없음**: 이 3개 모드의 settings 에는 `pre-push.sh` · `post-edit-lint.sh` · `Stop` 훅이 없습니다. `safety-guard.sh` · `session-start.sh` 만 유지.

#### 3-C-3. 플러그인 가용성 확인

각 모드별 필수 플러그인이 설치돼 있지 않으면 관련 커맨드·스킬이 동작하지 않습니다.

**Step 6 완료 메시지에 모드별로 반드시 포함**:

- **marketing / sales 모드**:
  ```
  ⚠️ marketing-skills 플러그인 필요 — 아직 설치되지 않았다면:
      /plugin install marketing-skills@marketingskills
  ```

- **product 모드** (pm-skills 마켓플레이스 8개 플러그인 전부 필요):
  ```
  ⚠️ pm-skills 마켓플레이스 + 8개 플러그인 필요 — 아직 설치되지 않았다면:
      /plugin marketplace add phuryn/pm-skills
      /plugin install pm-toolkit@pm-skills
      /plugin install pm-product-discovery@pm-skills
      /plugin install pm-product-strategy@pm-skills
      /plugin install pm-execution@pm-skills
      /plugin install pm-go-to-market@pm-skills
      /plugin install pm-market-research@pm-skills
      /plugin install pm-data-analytics@pm-skills
      /plugin install pm-marketing-growth@pm-skills
  (선택) /plugin install marketing-skills@marketingskills
  ```

---

### 3-공통. 커버리지 게이트 훅

`.claude/hooks/pre-push.sh` 확인. `bootstrap.sh` 로 설치했으면 이미 존재. 없으면 복사. 훅은 `.claude/stacks.json` 유무로 모드를 자동 감지합니다.

> **marketing/sales 모드 예외**: `settings.{marketing,sales}.json` 에는 `pre-push.sh` 훅이 등록돼 있지 않으므로 이 단계는 **건너뜁니다** (파일만 남아 있어도 실행되지 않음).

### 3-공통. Second Brain 초기화

`/init` 은 새 프로젝트 시작이므로 **기존 memory 가 있어도 항상 초기화**:

```bash
mkdir -p memory
cp .claude/templates/memory.md ./memory/MEMORY.md
```

초기화 후 사용자에게 **한 번에** 질문:

> 1. 이 프로젝트의 목적 또는 배경을 한 줄로 설명해주세요.
> 2. 주요 도메인이나 핵심 기능은 무엇인가요?
> 3. 특별한 제약사항이 있나요? (마감일, 성능 요구사항, 팀 규모)
> 4. 연동할 외부 시스템이나 참고 레퍼런스가 있나요? (없으면 생략)

답변을 받으면 `memory/MEMORY.md` 에 아래 두 항목 기록:

```markdown
## YYYY-MM-DD: 프로젝트 시작

**카테고리:** 결정

- **프로젝트명:** [프로젝트명]
- **모드:** 단일 / 모노레포
- **스택:** [단일: kotlin] 또는 [모노레포: backend(kotlin-multi) + web(nextjs) + app(flutter)]
- **목적:** [질문 1 답변]
- **핵심 기능:** [질문 2 답변]
- **제약사항:** [질문 3 답변]
- **외부 연동:** [질문 4 답변 또는 없음]

---

## YYYY-MM-DD: Claude Code 하네스 구성

**카테고리:** 참고

/init [스택 또는 monorepo] 으로 하네스를 구성했습니다.
- CLAUDE.md: 아키텍처 규칙 및 코딩 컨벤션 (모노레포일 경우 루트 인덱스 + 역할별 CLAUDE.md)
- .claude/settings.json: 권한 및 훅 설정
- .claude/stacks.json: 스택 매니페스트 (모노레포만)
- memory/MEMORY.md: Second Brain 초기화

앞으로 중요한 결정·교훈은 /memory add 로 기록하세요.
```

---

## Step 4 — .gitignore 초기 설정

```bash
grep -q "\.worktrees/" .gitignore 2>/dev/null || echo ".worktrees/" >> .gitignore
git add .gitignore
git commit -m "chore: .worktrees/ gitignore 추가" 2>/dev/null || true
```

> `.worktrees/` 가 gitignore 에 없으면 worktree 디렉토리가 git 에 추적될 위험이 있습니다.

---

## Step 5 — Git 브랜치 전략 (신규 프로젝트만)

사용자에게 선택 요청 (기본값 A):

> **A. main + dev** (권장)
> ```
> main ← dev ← feature/* / fix/* / hotfix/* / ...
> ```
> - `dev`: 개발 통합 (스테이징), `main`: 프로덕션
> - PR: `feature/*` → `dev`, 릴리즈: `dev` → `main`
>
> **B. main only**
> ```
> main ← feature/* / fix/* / hotfix/* / ...
> ```
> - `main` 하나만, PR: `feature/*` → `main` 바로 머지

### A 선택

```bash
git checkout -b dev
git push -u origin dev
```

GitHub 브랜치 보호 안내:
- **main**: PR 필수 + status checks + restrict push
- **dev**: PR 필수 + status checks

### B 선택

`dev` 생성하지 않음. `main` 하나만 사용.

GitHub 브랜치 보호 안내:
- **main**: PR 필수 + status checks + restrict push

> ℹ️ `/new worktree` 는 `dev` 존재 여부로 전략을 자동 감지합니다. `dev` 없으면 자동으로 `main` 을 베이스로 사용.

---

## Step 6 — 완료 메시지

### 단일 스택 모드

```
✅ 프로젝트 하네스 구성 완료

프로젝트: [이름]
스택: [선택 스택]
제거된 파일: N개

[하네스 기둥 상태]
  기둥 1 (컨텍스트):     CLAUDE.md ✅
  기둥 2 (CI/CD 게이트): .claude/settings.json hooks ✅
  기둥 3 (도구 경계):    .claude/settings.json permissions ✅
  기둥 4 (피드백 루프):  /rule 커맨드 ✅
  기둥 5 (팀 지식):      memory/MEMORY.md ✅

[Git 브랜치 & Worktree]
  main + dev 또는 main only
  dev 브랜치: 생성됨 / 사용 안 함
  .worktrees/: gitignore 등록됨

남은 agents: [목록]
남은 commands: [목록]

이제 할 일:
  1. CLAUDE.md 를 열고 프로젝트에 맞게 커스터마이징
  2. GitHub 브랜치 보호 규칙 설정 (main·dev 또는 main만)
  3. /start <기능>               → 신규 기능 한 번에 시작 (worktree + PRD + 자동 구현)
  4. /plan <기능>                → 설계만 (worktree 없이 PRD + 역할 프롬프트)
  5. /plan <기능> --light        → 가벼운 단일 변경 계획
  6. AI 실수 시 /rule            → 규칙 추가
  7. 중요한 결정·교훈은 /memory add

[멀티 모듈 스택 추가]
  8. /new module <모듈명>        → 새 서브모듈/패키지/서비스
```

### Marketing / Sales 모드

```
✅ 프로젝트 하네스 구성 완료 ([marketing|sales] 모드)

프로젝트: [이름]
모드:    [marketing | sales] — 코드 없음
제거된 파일: N개

[하네스 기둥 상태]
  기둥 1 (컨텍스트):     CLAUDE.md ✅
  기둥 2 (CI/CD 게이트): .claude/settings.json hooks (safety-guard + session-start) ✅
  기둥 3 (도구 경계):    .claude/settings.json permissions (git · gh · 파일 작업만) ✅
  기둥 4 (피드백 루프):  /rule 커맨드 ✅
  기둥 5 (팀 지식):      memory/MEMORY.md ✅

[Git 브랜치 & Worktree]
  main + dev 또는 main only
  .worktrees/: gitignore 등록됨

남은 agents: code-reviewer, planner, gtm-planner
핵심 커맨드: /start, /plan, /marketing, /memory, /commit, /pr, /merge, /rule

⚠️ marketing-skills 플러그인 필요 — 아직 설치되지 않았다면:
    /plugin install marketing-skills@marketingskills

이제 할 일:
  1. CLAUDE.md 를 열고 프로젝트에 맞게 커스터마이징
  2. (권장) /marketing context         → product-marketing-context 생성
  3. /new worktree {type-name}         → 첫 작업 브랜치
  4. /plan <기능> [--marketing|--sales|--gtm]  → PRD + 전략 문서 생성
  5. /marketing <카테고리>             → 스킬 기반 작업 (copywriting / seo-audit / cold-email ...)
  6. 중요한 결정·캠페인 결과는 /memory add
```

### Product 모드

```
✅ 프로젝트 하네스 구성 완료 (product 모드)

프로젝트: [이름]
모드:    product — 코드 없음, Product Management 전담
제거된 파일: N개

[하네스 기둥 상태]
  기둥 1 (컨텍스트):     CLAUDE.md (pm-skills 기반) ✅
  기둥 2 (CI/CD 게이트): .claude/settings.json hooks (safety-guard + session-start) ✅
  기둥 3 (도구 경계):    .claude/settings.json permissions (git · gh · 파일 작업만) ✅
  기둥 4 (피드백 루프):  /rule 커맨드 ✅
  기둥 5 (팀 지식):      memory/MEMORY.md ✅

[Git 브랜치 & Worktree]
  main + dev 또는 main only
  .worktrees/: gitignore 등록됨

남은 agents: code-reviewer, planner, gtm-planner
핵심 커맨드: /discover, /strategy, /write-prd, /plan-launch, /north-star,
            /start, /plan, /marketing, /memory, /commit, /pr, /merge, /rule

⚠️ pm-skills 마켓플레이스 + 8개 플러그인 필요 — 아직 설치되지 않았다면:
    /plugin marketplace add phuryn/pm-skills
    /plugin install pm-toolkit@pm-skills
    /plugin install pm-product-discovery@pm-skills
    /plugin install pm-product-strategy@pm-skills
    /plugin install pm-execution@pm-skills
    /plugin install pm-go-to-market@pm-skills
    /plugin install pm-market-research@pm-skills
    /plugin install pm-data-analytics@pm-skills
    /plugin install pm-marketing-growth@pm-skills
(선택) /plugin install marketing-skills@marketingskills

이제 할 일:
  1. CLAUDE.md 를 열고 프로젝트·팀에 맞게 커스터마이징
  2. /new worktree {type-name}              → 첫 작업 브랜치
  3. /discover                              → 제품 발견 (아이디어 → 가정 → 실험)
  4. /strategy                              → 전략 수립 (포지셔닝·차별점)
  5. /write-prd <기능>                      → PRD 작성 (docs/prd/)
  6. /plan-okrs                             → OKR 계획 (docs/okrs/)
  7. /plan-launch <기능>                    → 런치 플랜 (docs/launch/)
  8. /plan <기능> --gtm                     → PRD + 마케팅 + 세일즈 통합
  9. 리서치 인사이트·실패한 가설은 /memory add
```

### 모노레포 모드

```
✅ 모노레포 하네스 구성 완료

프로젝트: [이름]
모드:    monorepo
스택:
  - backend  → [path] ([type])
  - frontend → [path] ([type])
  - mobile   → [path] ([type])

제거된 파일: N개

[하네스 기둥 상태]
  기둥 1 (컨텍스트):     루트 CLAUDE.md + 역할별 CLAUDE.md ✅
  기둥 2 (CI/CD 게이트): .claude/settings.json hooks (경로 가드) ✅
  기둥 3 (도구 경계):    .claude/settings.json permissions ✅
  기둥 4 (피드백 루프):  /rule 커맨드 ✅
  기둥 5 (팀 지식):      memory/MEMORY.md ✅
  매니페스트:            .claude/stacks.json ✅

[Git 브랜치 & Worktree]
  .worktrees/: gitignore 등록됨

남은 agents: [유니온 목록]
남은 templates: [유니온 목록]

이제 할 일:
  1. 각 역할 CLAUDE.md 를 프로젝트 맞게 커스터마이징
  2. GitHub 브랜치 보호 규칙 설정
  3. /start <기능>              → 신규 기능 한 번에 시작 (worktree + PRD + 자동 병렬 구현)
  4. /plan <기능>               → 설계만 (PRD + 역할별 프롬프트 3세트)
                                  (--teams 로 활성 스택 agent 병렬 실행)
  5. /new backend api <Resource>    → backend 스캐폴딩 (개별)
     /new frontend component <Name> → frontend 컴포넌트
     /new mobile screen <Name>      → mobile 화면
  6. /plan backend <기능>       → 역할 prefix 로 구현 설계
  7. git push — 활성 스택 모두 커버리지 검증
```
