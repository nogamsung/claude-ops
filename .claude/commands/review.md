---
description: 코드 리뷰. 파일/staged/diff 범용 리뷰 또는 REST API 전용 리뷰(api 모드)를 통합 제공
argument-hint: [api | <파일경로> | staged | diff] (없으면 현재 대화 컨텍스트)
---

`code-reviewer` 에이전트로 리뷰를 진행합니다.

**대상:** $ARGUMENTS

---

## 모드 결정

첫 토큰으로 분기:

| 첫 토큰 | 모드 | 대상 |
|--------|------|------|
| `api` | [API 전용 리뷰](#api-모드) | Controller/Handler 스캔 (Kotlin·Go 전용) |
| `staged` | 범용 리뷰 | `git diff --staged` |
| `diff` | 범용 리뷰 | `git diff HEAD` |
| 파일 경로 | 범용 리뷰 | 해당 파일 |
| (없음) | 범용 리뷰 | 현재 대화 컨텍스트 또는 사용자에게 물어봄 |

---

## 범용 모드

### Step 1 — 리뷰 대상 파악

- 파일 경로 → 해당 파일 Read
- `staged` → `git diff --staged`
- `diff` → `git diff HEAD`
- 인수 없음 → 현재 대화에서 언급된 코드 사용 또는 사용자에게 물어봄

### Step 2 — 컨텍스트 파악

- 스택 확인 (Kotlin/Spring Boot, Next.js, Flutter, Go Gin)
- 관련 파일(인터페이스, 부모 클래스, 사용처) 필요시 추가 Read

### Step 3 — code-reviewer 에이전트 호출

수집한 코드와 컨텍스트를 전달하여 리뷰 요청.

### Step 4 — 결과 출력

```
## 코드 리뷰 결과

**전체 평가**: Approved / Approved with minor changes / Changes requested

### 🚨 Critical (반드시 수정)
### ⚠️ Major (수정 권장)
### 💡 Minor (개선 제안)
### ✅ 잘 된 점

### 수정 코드 예시 (필요시)
```

리뷰는 **건설적이고 구체적**이어야 합니다. 각 지적에 대해 왜 문제인지 + 어떻게 수정할지 명시.

---

## api 모드

Kotlin Spring Boot · Go Gin 백엔드 전용. RESTful 컨벤션·보안·OpenAPI 문서 완성도를 체크합니다.

두 번째 토큰이 파일 경로/리소스명이면 해당 대상만, 없으면 전체 Controller/Handler 스캔.

### Step 1 — 대상 파일 수집

| 스택 | 대상 |
|------|------|
| Kotlin Spring Boot | `**/presentation/**Controller.kt` |
| Go Gin | `**/handler/**_handler.go` |

### Step 2 — REST 컨벤션 체크

**URL 구조**
- [ ] 리소스명이 복수형 명사 (`/orders` not `/order`, `/getOrders`)
- [ ] 계층 관계가 3단계를 초과하지 않음
- [ ] 액션은 `POST /resource/{id}/action` 패턴

**HTTP 메서드**
- [ ] GET — 조회만, 사이드 이펙트 없음
- [ ] POST — 생성 / 201 Created
- [ ] PUT — 전체 수정 / PATCH — 부분 수정 구분
- [ ] DELETE — 삭제 / 204 No Content

**상태코드**
- [ ] 생성: 201 (not 200)
- [ ] 삭제: 204 (not 200)
- [ ] 인증 없음: 401 (not 403)
- [ ] 권한 없음: 403 (not 401)
- [ ] 비즈니스 규칙 위반: 422 (not 400)

### Step 3 — 보안 체크

**인증/인가**
- [ ] 인증 필요 엔드포인트에 `@PreAuthorize` / JWT 미들웨어 적용
- [ ] 공개 엔드포인트는 명시적 허용 목록
- [ ] 타인 리소스 접근 방지 (userId 검증)

**과도한 데이터 노출**
- [ ] 민감 필드(비밀번호·해시) Response 미포함
- [ ] 내부 DB PK 직접 노출 여부 (필요 시 UUID/난수 ID)
- [ ] 관계 엔티티 무한 중첩 직렬화 방지

**입력값 검증**
- [ ] Bean Validation (`@Valid`) / binding 체크
- [ ] Path variable `{id}` 타입 검증

### Step 4 — OpenAPI 문서 완성도

**Kotlin Spring Boot (SpringDoc)**
- [ ] 클래스 레벨 `@Tag`
- [ ] 메서드별 `@Operation(summary)`
- [ ] 성공/실패 `@ApiResponse`
- [ ] Path/Query `@Parameter(description)`
- [ ] DTO 필드 `@Schema`

**Go Gin (swag)**
- [ ] Handler godoc swag 주석
- [ ] `@Summary`, `@Tags`, `@Router`
- [ ] `@Success`, `@Failure` 응답 타입
- [ ] 인증 필요 엔드포인트에 `@Security BearerAuth`
- [ ] Response DTO 필드 `example:"..."` json 태그

### Step 5 — 결과 출력

```
## API 리뷰 결과

### [파일명 또는 리소스명]

#### 잘된 점
- ...

#### 개선 필요
| 심각도 | 항목 | 위치 | 권장 수정 |
|--------|------|------|----------|
| 높음   | 인증 미적용 | GET /api/v1/orders/{id} | JWT 미들웨어 추가 |
| 중간   | 상태코드 오류 | POST /api/v1/orders → 200 | 201 Created로 변경 |
| 낮음   | @Operation 누락 | OrderController:45 | SpringDoc 어노테이션 추가 |

#### 수정 제안 코드 (심각도 높음)
```

**심각도 기준**

| 심각도 | 기준 |
|--------|------|
| 높음 | 보안 취약점 (인증 누락, 민감 데이터 노출) |
| 중간 | 잘못된 HTTP 상태코드, 비즈니스 규칙 오류 가능성 |
| 낮음 | 컨벤션 불일치, 문서 누락 |
