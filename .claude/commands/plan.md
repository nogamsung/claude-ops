---
description: 코드 전 설계 합의. 디폴트는 PRD + 역할 프롬프트 (planner agent). api/db 서브로 API·DB 설계, --light 로 가벼운 단일 변경 계획.
argument-hint: [api <Resource> | db <도메인>] <설명>  또는  <기능 설명> [--light | --teams | --output-only | --marketing | --sales | --gtm]
---

코드를 작성하기 전에 설계를 먼저 검토·합의합니다.
**worktree 생성 + 자동 실행까지 한 번에 원하면 `/start` 를 사용하세요.** `/plan` 은 설계만 합니다.

**요청:** $ARGUMENTS

---

## 진입 모드 결정

### Step 0 — 모노레포 service prefix 체크

`.claude/stacks.json` 이 존재하고 `mode: "monorepo"` 이면, 첫 토큰이 service 식별자면 **해당 service 의 경로·타입을 컨텍스트로 설정**한 뒤 prefix 를 소비하고 나머지 인자로 진행.

**Service 식별자**: `/new` 와 동일 문법 — `role` / `role:name` / `name` 지원.

```bash
# 예시:
# /plan backend api User           — backend 1개면 자동 / 여러 개면 interactive
# /plan backend:auth api User      — role+name 명시
# /plan auth api User              — name 유일할 때만 허용

TOKEN="$1"
# (1) role:name  (2) role 단독 + 유일성 검사  (3) name 단독 + 유일성 검사
STACK_PATH=$(echo "$MATCH" | jq -r '.path')
STACK_TYPE=$(echo "$MATCH" | jq -r '.type')
# → 이후 파일 경로 / 스택 감지는 $STACK_PATH 를 루트로 간주
```

prefix 생략 시 service 가 1개면 자동, 2개 이상이면 사용자에게 확인. 단일 스택 모드에서는 Step 0 을 건너뜁니다.

### Step 1 — 모드 분기

`$ARGUMENTS` 의 (prefix 소비 후) 첫 토큰으로 분기:

| 첫 토큰 | 모드 | 설명 |
|--------|------|------|
| `api` | [API 설계 모드](#api-설계-모드) | REST API 설계 → OpenAPI 3.0 YAML |
| `db` | [DB 설계 모드](#db-설계-모드) | MySQL 스키마 → Migration SQL |
| 그 외 / 없음 (+ `--light`) | [라이트 계획 모드](#라이트-계획-모드) | 가벼운 단일 변경 계획 |
| 그 외 / 없음 (디폴트) | [기획 모드](#기획-모드) | planner agent → PRD + 역할 프롬프트 |

**플래그**:
- `--light` — 라이트 계획 모드 (현 단순 작업용)
- `--teams` — 기획 모드에서 PRD 생성 후 generator agent 자동 실행 (이미 worktree 안일 때 유용)
- `--output-only` — 기획 모드의 디폴트와 동일 (명시 시 의미 없음, 호환성 유지)
- `--marketing` / `--sales` / `--gtm` — GTM 문서 추가

**자동 전환**: 기획 모드에서 요청을 분석했을 때
- "새 리소스 CRUD" 류 → API 설계가 선행되어야 하면 `/plan api <Resource>` 제안
- "새 테이블/스키마" 류 → DB 설계가 선행되어야 하면 `/plan db <도메인>` 제안

> 💡 **신규 기능을 처음부터 시작한다면 `/start <기능>`** — worktree 자동 생성 + PRD + 자동 실행. `/plan` 은 worktree 안에서 추가 설계 / API·DB 합의 / 라이트 계획용.

---

## 기획 모드 (디폴트)

신규 기능 또는 큰 변경을 PRD 와 역할별 프롬프트로 구조화합니다. **planner agent** 가 처리합니다.

### Step 1 — feature 이름 결정

`raw_request` 에서 kebab-case 이름 추출 (예: "로그인 기능" → `login`).

애매하면:
> "기능 이름을 kebab-case 로 알려주세요 (예: `login`, `payment-cancel`):"

`docs/specs/{feature}.md` 가 이미 있으면:
> "기존 PRD 가 있습니다. (a) 덮어쓰기 / (b) 갱신 (diff 제안) / (c) 중단"

### Step 2 — 인터뷰 (요청이 모호한 경우만)

요청이 명확하면 **0개 질문으로 통과**. 모호하면 한 번에 최대 3개:

- **시나리오**: 주요 사용자 플로우와 성공 기준
- **스코프**: 이번 이터레이션 vs 후속
- **(모노레포)** 스택 범위: backend only / full-stack / mobile 포함

### Step 3 — `planner` agent 호출

```
Agent(
  subagent_type="planner",
  prompt="raw_request=<원문>, feature_name=<kebab>, interview_answers=<요약>,
          mode=<single|monorepo>, stacks=<.claude/stacks.json 또는 단일 감지 결과>"
)
```

planner 가 PRD + 역할 프롬프트를 생성하고 리포트를 반환합니다.

### Step 3.5 — GTM 문서 (플래그 있을 때만)

`--marketing` / `--sales` / `--gtm` 중 하나 이상이면 `gtm-planner` agent 호출.

**플래그 매핑:**
- `--marketing` → `["marketing"]`
- `--sales` → `["sales"]`
- `--gtm` → `["marketing", "sales"]`

```
Agent(
  subagent_type="gtm-planner",
  prompt="feature_name=<kebab>, feature_display=<한글 또는 원문>,
          flags=[<위 매핑 결과>],
          prd_path=docs/specs/<feature>.md,
          spec_dir=docs/specs/<feature>/,
          raw_request=<원문>"
)
```

PRD 가 정상 생성되지 않았으면 건너뜁니다. `marketing-skills` 플러그인 없으면 fallback 모드 (경고만).

### Step 4 — 실행 (선택적)

**디폴트** (플래그 없음): 파일만 생성하고 종료. 다음 안내:

```
✅ 기획 완료

PRD:  docs/specs/{feature}.md
역할 프롬프트:
  docs/specs/{feature}/backend.md       → /new backend api <Resource>
  docs/specs/{feature}/frontend.md      → /new frontend component <Name>

다음:
  - 자동 실행을 원하면 동일 명령에 `--teams` 추가
  - 또는 worktree 부터 시작하려면 `/start <기능>` 사용
  - 수동 실행은 위 `/new ...` 명령 참고
```

**`--teams` 모드**: 즉시 generator agent 병렬 실행.

**모노레포** — 1회만 확인 후 진행:
> ```
> {N}개 service 에 대해 generator agent 를 병렬 실행합니다:
>   - backend:auth (kotlin-multi)
>   - frontend     (nextjs)
> 진행할까요? (yes / no, 기본 yes)
> ```

**단일 스택** — 묻지 않고 즉시 실행.

각 service 의 프롬프트를 단일 메시지에서 병렬 호출:

```
Agent(subagent_type="kotlin-generator", prompt=<docs/specs/{feature}/backend-auth.md 내용>)
Agent(subagent_type="nextjs-generator", prompt=<docs/specs/{feature}/frontend.md 내용>)
```

**스택 → agent 매핑:**

| stacks.json type | 호출할 agent |
|------------------|-------------|
| `kotlin` / `kotlin-multi` | `kotlin-generator` |
| `go` / `go-multi` | `go-generator` |
| `python` / `python-multi` | `python-generator` |
| `nextjs` / `nextjs-multi` | `nextjs-generator` |
| `flutter` | `flutter-generator` |

각 agent prompt 머리에:

> 이 작업은 `/plan --teams` 가 생성한 service 프롬프트에 따라 진행합니다. Step 별로 순서대로 수행하고, 다른 service 디렉토리는 건드리지 마세요. 작업 디렉토리: `{service.path}`. 완료 후 생성한 파일 목록을 짧게 리포트하세요.

---

## 라이트 계획 모드 (`--light`)

가벼운 단일 변경 (버그 수정, 작은 리팩토링, 한 두 파일 수정) 에 사용. PRD 없이 즉시 계획만.

### Step 1 — 요청 분석

요청을 읽고 판단:
- 무엇을 만들지 알 수 있는가?
- 어떤 파일/레이어가 영향받는지 알 수 있는가?
- 요구사항에 모순·빠진 부분이 없는가?

**명확** → Step 3 / **모호** → Step 2

### Step 2 — 소크라테스식 인터뷰

**한 번에 최대 3개**만:
- **도메인/범위**: 핵심 시나리오 / 어느 레이어·모듈 / 기존 코드와 연결
- **비즈니스 규칙**: 예외 케이스 / 유효성 / 호환성
- **기술 결정**: 신규 vs 수정 / 테스트 / 구현 방식

### Step 3 — 구현 계획 작성

```
## 구현 계획

### 변경할 파일
- [신규/수정] `경로/파일명.ext` — 변경 이유

### 영향받는 범위
- 연쇄적으로 영향받는 파일 목록

### 구현 순서
1. ...

### 주의사항
- CLAUDE.md 규칙 중 관련된 것
- 예상 엣지 케이스

### 테스트 계획
- 작성할 테스트 케이스
```

### Step 4 — 확인

> **이 계획대로 진행할까요? 수정이 필요한 부분이 있으면 알려주세요.**

승인되면 generator/modifier agent 로 구현 시작.

---

## API 설계 모드

**리소스명**: 두 번째 토큰 (없으면 사용자에게 물어보세요)

### Step 1 — 스택 감지

| 파일 | 스택 | 구현 커맨드 |
|------|------|------------|
| `build.gradle.kts` / `pom.xml` | Kotlin Spring Boot | `/new api <Resource>` |
| `go.mod` | Go Gin | `/new api <Resource>` |
| `pyproject.toml` (`fastapi` 의존성) | Python FastAPI | `/new api <Resource>` |

### Step 2 — 도메인 파악

**최대 3개** 질문:
- 리소스의 핵심 속성(필드)은?
- 어떤 사용자/역할이 호출? (인증 필요 여부)
- 특별한 비즈니스 액션은?
- 다른 리소스와 관계는?
- 목록 조회 필터·정렬 조건은?

### Step 3 — 엔드포인트 목록 초안

`api-designer` 에이전트로 다음 형식 제시:

```
[엔드포인트 목록]
  GET    /api/v1/{resources}          — 목록 조회 (페이지네이션)
  POST   /api/v1/{resources}          — 생성
  GET    /api/v1/{resources}/{id}     — 단건 조회
  PUT    /api/v1/{resources}/{id}     — 전체 수정
  PATCH  /api/v1/{resources}/{id}     — 부분 수정  ← 필요 시
  DELETE /api/v1/{resources}/{id}     — 삭제
  POST   /api/v1/{resources}/{id}/cancel  — 액션  ← 필요 시

[인증]   Bearer JWT  /  API Key  /  없음
[페이지네이션]   오프셋 (?page=0&size=20)  /  커서 기반
```

### Step 4 — Request / Response 스키마

```
[Request]
  Create{Resource}Request — 필드/타입/필수/제약
  Update{Resource}Request — 필드/타입/제약

[Response]
  {Resource}Response — id, 필드, createdAt

[에러 응답]
  400 입력값 유효성 / 401 인증 없음 / 404 리소스 없음
```

### Step 5 — OpenAPI YAML 출력

`api-designer` 가 OpenAPI 3.0 YAML 초안 생성. 파일명: `docs/api/{resource}.yaml`.

### Step 6 — 확인 및 구현 연결

> **이 설계대로 구현을 시작할까요?**

승인 시:
```
설계 완료. 구현 시작.
  /new api {리소스명}
```

> **핵심 원칙**: API 계약을 먼저 합의하면 프론트·백엔드가 병렬 작업할 수 있고, 스키마 변경 비용이 줄어듭니다.

---

## DB 설계 모드

**도메인 설명**: 두 번째 토큰 이후

### Step 1 — 스택 감지

| 파일 | 스택 | Migration 도구 |
|------|------|---------------|
| `build.gradle.kts` / `pom.xml` | Kotlin Spring Boot | Flyway |
| `go.mod` | Go Gin | golang-migrate |
| `pyproject.toml` (`fastapi` 의존성) | Python FastAPI | Alembic |

### Step 2 — 도메인 분석

필요한 **엔티티/테이블**, **관계**, **제약사항** 파악.

불명확하면 **최대 3개** 질문:
1. 어떤 엔티티/테이블?
2. 엔티티 간 관계?
3. Soft Delete 가 필요한 테이블?

### Step 3 — ERD 텍스트 출력

```
[users] 1 ──── N [orders]
[orders] 1 ──── N [order_items]
[products] 1 ──── N [order_items]
```

각 테이블 핵심 컬럼 (PK/FK/UNIQUE/Soft Delete) 표기.

> "위 ERD로 Migration SQL 을 생성할까요?"

### Step 4 — Migration SQL 생성

`db-patterns.md` 스킬을 **반드시** 참고.

**공통 규칙**: `id`/`created_at`/`updated_at` 자동 / `ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci` / FK 컬럼 INDEX 자동 / 금액은 `DECIMAL(10, 2)` / 상태값은 `VARCHAR(20~50)` / Soft Delete 는 `deleted_at DATETIME(6) NULL` + index.

**Kotlin (Flyway)**: `src/main/resources/db/migration/V{N}__create_{table}_table.sql` — 기존 파일 절대 수정 금지.

**Go (golang-migrate)**: `migrations/{N:06d}_create_{table}_table.{up,down}.sql` — up/down 쌍 필수, down 은 자식부터 역순.

**Python (Alembic)**:
- 먼저 SQLAlchemy Model (`app/models/{resource}.py`) 작성·업데이트
- `uv run alembic revision --autogenerate -m "create_{table}_table"` 안내
- 생성된 revision 파일 **반드시 검토** — autogenerate 가 놓치는 것: 서버 기본값, 체크 제약, 복합 인덱스 순서, ENUM 타입 변경
- `upgrade()` / `downgrade()` 쌍 모두 구현, 기존 revision 파일 수정 금지

### Step 5 — Entity 생성 연계

```
Migration SQL 생성 완료. 이제 Entity/Repository 를 생성하세요:
  /new api {Resource}
```

### 주의사항

- `db-patterns.md` 항상 참고
- 부모 테이블 먼저 생성, down 은 자식부터
- 생성한 파일 목록과 스키마 결정 이유 요약

---

## 사용 예시

```bash
# 신규 기능 — PRD + 역할 프롬프트만 (실행 없음)
/plan 로그인 기능

# 신규 기능 — PRD 후 즉시 generator 실행 (이미 worktree 안일 때)
/plan 결제 취소 기능 --teams

# GTM 포함
/plan 유료 플랜 런치 --gtm
/plan 블로그 발행 --marketing --teams

# 가벼운 단일 변경
/plan 사용자 이메일 검증 로직 보강 --light

# API 계약 합의
/plan api Order
/plan backend api User           # 모노레포

# DB 스키마
/plan db 주문 도메인
/plan backend db order            # 모노레포
```

---

## 주의사항

- **planner agent / gtm-planner agent 는 코드를 쓰지 않습니다.** PRD/문서만 산출.
- **`/start` 와의 차이**: `/start` 는 worktree 생성 + 자동 실행이 디폴트, `/plan` 은 설계만 (실행은 `--teams` 명시 시).
- **기존 PRD 덮어쓰기 확인 필수**.
- **Teams 모드 충돌 회피** — 모노레포는 역할별 경로 분리 시에만 안전.
- **`docs/specs/` + `docs/gtm/` 는 팀 공유 대상** — `.gitignore` 하지 않습니다.

> **핵심 원칙**: 코드를 먼저 짜고 나중에 방향을 수정하는 것보다, 방향을 먼저 합의하고 코드를 짜는 것이 항상 빠릅니다.
