---
description: 코드 작성 전 설계 합의. 범용 계획 + api/db 설계를 통합한 단일 진입점. 자연어 요청 또는 api/db 서브명령으로 분기.
argument-hint: [api <Resource> | db <도메인>] <설명>  또는  <기능 설명>
---

코드를 작성하기 전에 설계를 먼저 검토·합의합니다.

**요청:** $ARGUMENTS

---

## 진입 모드 결정

### Step 0 — 모노레포 역할 prefix 체크

`.claude/stacks.json` 이 존재하고 `mode: "monorepo"` 이면, 첫 토큰이 역할명(`backend` / `frontend` / `mobile`)이면 **해당 스택의 경로·타입을 컨텍스트로 설정**한 뒤 prefix 를 소비하고 나머지 인자로 진행.

```bash
# 예: /plan backend api User
STACK_PATH=$(jq -r '.stacks[] | select(.role == "backend") | .path' .claude/stacks.json)
STACK_TYPE=$(jq -r '.stacks[] | select(.role == "backend") | .type' .claude/stacks.json)
# → 이후 파일 경로 / 스택 감지는 $STACK_PATH 를 루트로 간주
```

prefix 생략 시:
- 활성 스택이 **1개면** 자동 선택
- **2개 이상**이면 사용자에게 확인:
  > "어느 스택에 대한 계획인가요? (backend / frontend / mobile)"

단일 스택 모드에서는 Step 0 을 건너뜁니다.

### Step 1 — 모드 분기

`$ARGUMENTS` 의 (prefix 소비 후) 첫 토큰으로 분기:

| 첫 토큰 | 모드 | 설명 |
|--------|------|------|
| `api` | [API 설계 모드](#api-설계-모드) | REST API 설계 → OpenAPI 3.0 YAML |
| `db` | [DB 설계 모드](#db-설계-모드) | MySQL 스키마 → Migration SQL |
| 그 외 / 없음 | [범용 계획 모드](#범용-계획-모드) | 소크라테스식 인터뷰 + 구현 계획 |

**자동 전환**: 범용 계획 모드에서 요청을 분석했을 때
- "새 리소스 CRUD" 류 → API 설계가 선행되어야 하면 사용자에게 `/plan api <Resource>` 제안 (모노레포면 `/plan backend api <Resource>`)
- "새 테이블/스키마" 류 → DB 설계가 선행되어야 하면 `/plan db <도메인>` 제안 (모노레포면 `/plan backend db <도메인>`)

---

## 범용 계획 모드

### Step 1 — 요청 분석

요청을 읽고 판단:
- 무엇을 만들지 알 수 있는가?
- 어떤 파일/레이어가 영향받는지 알 수 있는가?
- 요구사항에 모순·빠진 부분이 없는가?

**명확** → Step 3 / **모호** → Step 2

### Step 2 — 소크라테스식 인터뷰

**한 번에 최대 3개**만 물어봅니다.

**도메인/범위**: 핵심 사용자 시나리오 / 어느 레이어·모듈 / 기존 코드와 연결
**비즈니스 규칙**: 예외 케이스 / 유효성 검사 / 기존 동작 호환
**기술 결정**: 신규 vs 수정 / 테스트 포함 여부 / 선호 구현 방식

### Step 3 — 구현 계획 작성

CLAUDE.md 아키텍처 규칙 참고하여 다음 형식으로 제시:

```
## 구현 계획

### 변경할 파일
- [신규/수정] `경로/파일명.ext` — 변경 이유

### 영향받는 범위
- 연쇄적으로 영향받는 파일 목록

### 구현 순서
1. 첫 번째 단계
2. 두 번째 단계

### 주의사항
- CLAUDE.md 규칙 중 관련된 것
- 예상 엣지 케이스

### 테스트 계획
- 작성할 테스트 케이스 목록
```

### Step 4 — 확인

> **이 계획대로 진행할까요? 수정이 필요한 부분이 있으면 알려주세요.**

승인되면 해당 스택의 generator/modifier agent로 구현 시작.

---

## API 설계 모드

**리소스명**: 두 번째 토큰 (없으면 사용자에게 물어보세요)

### Step 1 — 스택 감지

| 파일 | 스택 | 구현 커맨드 |
|------|------|------------|
| `build.gradle.kts` / `pom.xml` | Kotlin Spring Boot | `/new api <Resource>` |
| `go.mod` | Go Gin | `/new api <Resource>` |

### Step 2 — 도메인 파악

**최대 3개** 질문:
- 리소스의 핵심 속성(필드)은?
- 어떤 사용자/역할이 호출? (인증 필요 여부)
- 특별한 비즈니스 액션은? (예: 주문 취소, 상태 변경)
- 다른 리소스와 관계는? (예: Order → OrderItem)
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

[인증]
  Bearer JWT  /  API Key  /  없음

[페이지네이션]
  오프셋 (?page=0&size=20)  /  커서 기반
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

`api-designer`가 OpenAPI 3.0 YAML 초안 생성. 파일명: `docs/api/{resource}.yaml` (없으면 콘솔 출력).

### Step 6 — 확인 및 구현 연결

> **이 설계대로 구현을 시작할까요? 수정이 필요한 부분이 있으면 알려주세요.**

승인 시:

```
설계 완료. 구현 시작.

  /new api {리소스명}
```

> **핵심 원칙**: API 계약을 먼저 합의하면 프론트·백엔드가 병렬 작업할 수 있고, 스키마 변경 비용이 줄어듭니다.

---

## DB 설계 모드

**도메인 설명**: 두 번째 토큰 이후 (없으면 사용자에게 물어보세요)

### Step 1 — 스택 감지

| 파일 | 스택 | Migration 도구 |
|------|------|---------------|
| `build.gradle.kts` / `pom.xml` | Kotlin Spring Boot | Flyway |
| `go.mod` | Go Gin | golang-migrate |

둘 다 없으면 사용자에게 선택 요청.

### Step 2 — 도메인 분석

필요한 **엔티티/테이블**, **관계**(1:1, 1:N, N:M), **제약사항**(UNIQUE, NOT NULL, Soft Delete) 파악.

불명확하면 **최대 3개** 질문:
1. 어떤 엔티티/테이블이 필요한가요?
2. 엔티티 간 관계는?
3. Soft Delete가 필요한 테이블은?

### Step 3 — ERD 텍스트 출력

```
[users] 1 ──── N [orders]
[orders] 1 ──── N [order_items]
[products] 1 ──── N [order_items]
```

각 테이블 핵심 컬럼 (PK/FK/UNIQUE/Soft Delete 표기)도 함께.

> "위 ERD로 Migration SQL을 생성할까요? 수정이 필요하면 말씀해주세요."

### Step 4 — Migration SQL 생성

`db-patterns.md` 스킬을 **반드시** 참고.

**공통 규칙**
- 공통 컬럼 자동 포함: `id`, `created_at`, `updated_at`
- `ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`
- FK 컬럼에 INDEX 자동 생성
- 금액/가격 → `DECIMAL(10, 2)` (`FLOAT` 금지)
- 상태값 → `VARCHAR(20~50)` (ENUM 지양)
- Soft Delete → `deleted_at DATETIME(6) NULL DEFAULT NULL` + `INDEX idx_{table}_deleted_at (deleted_at)`

**Kotlin (Flyway)**: `src/main/resources/db/migration/V{N}__create_{table}_table.sql`
- `ls src/main/resources/db/migration/ | sort` 로 다음 번호 확인
- 패턴: `V{N}__create_{table}_table.sql` / `V{N}__add_{col}_to_{table}.sql` / `V{N}__create_idx_{table}_{col}.sql`
- **기존 파일 절대 수정 금지**

**Go (golang-migrate)**: `migrations/{N:06d}_create_{table}_table.{up,down}.sql`
- `ls migrations/*.up.sql | sort | tail -1` 로 다음 번호 확인
- `up`/`down` 쌍 반드시 생성
- `down`은 FK 자식 테이블부터 역순 `DROP TABLE IF EXISTS`

### Step 5 — Entity 생성 연계 안내

```
Migration SQL 생성 완료. 이제 Entity/Repository를 생성하세요:

  /new api {Resource}
```

### 주의사항

- `db-patterns.md` 항상 참고 (타입·인덱스·FK 패턴)
- 테이블 참조 순서: 부모 먼저 생성
- down 파일은 up의 역순 (자식 먼저 DROP)
- 생성한 파일 목록과 스키마 결정 이유를 요약

---

> **핵심 원칙**: 코드를 먼저 짜고 나중에 방향을 수정하는 것보다, 방향을 먼저 합의하고 코드를 짜는 것이 항상 빠릅니다.
