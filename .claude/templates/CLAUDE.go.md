# [프로젝트명] — Go Gin

## Stack
- **Language**: Go (latest stable)
- **Framework**: Gin
- **ORM**: GORM (단순 CRUD)
- **쿼리 생성**: **sqlc** (필수 — 동적·복잡 쿼리는 sqlc로 타입 안전하게 생성)
- **Migration**: golang-migrate
- **Lint**: **golangci-lint** (필수 — 모든 PR/push 전 통과 의무)
- **Docs**: **swaggo/swag** — Swagger UI `/swagger/index.html`
- **Validation**: Gin binding tags (`binding:"required"`)
- **Testing**: testify + mockery
- **Config**: godotenv / viper

## Agents
| 작업 | Agent |
|------|-------|
| 새 파일 생성 | `go-generator` |
| 기존 코드 수정 | `go-modifier` |
| 테스트 작성 | `go-tester` |
| 코드 리뷰 | `code-reviewer` |

## Commands
| 커맨드 | 용도 |
|--------|------|
| `/planner <기능>` | 기획서(PRD) + 구현 프롬프트 작성 (단일 스택은 단일 프롬프트 산출) |
| `/plan <기능>` | 코드 작성 전 설계 및 확인 |
| `/plan api <Resource>` | REST API 설계 → OpenAPI 3.0 YAML |
| `/plan db <도메인>` | MySQL 스키마 설계 → golang-migrate migration 자동 생성 |
| `/new <Resource>` | REST API 전체 스캐폴딩 (스택 자동 감지, 명시: `/new api`) |
| `/test [파일]` | 테스트 자동 생성 |
| `/review [staged\|diff\|파일]` | 코드 리뷰 |
| `/rule <실수 설명>` | 새 규칙을 이 파일에 추가 |
| `/commit [힌트]` | Conventional Commits 커밋 |
| `/pr` | PR 생성 + /merge 자동 제안 |
| `/merge [auto]` | GitHub 머지 실행 + 태그 + worktree 정리 |
| `/memory [add\|search]` | Second Brain 조회·추가·검색 |

---

## Git 브랜치 전략 & 병렬 작업 (Worktree)

| 브랜치 | 역할 | 보호 |
|--------|------|------|
| `main` | 프로덕션 릴리스 | PR + CI 통과 필수 |
| `dev` | 통합·스테이징 | PR + CI 통과 필수 |
| `feature/{name}` | 새 기능 | - |
| `fix/{name}` | 버그 수정 | - |
| `hotfix/{name}` | 긴급 수정 | - |
| `refactor/{name}` | 리팩토링 | - |
| `chore/{name}` | 설정·의존성 | - |

### Worktree 병렬 작업 흐름

```bash
# 작업 시작 — worktree로 격리된 작업공간 생성
/new feature-login    # feature/login + .worktrees/feature-login/
/new fix-signup       # fix/signup + .worktrees/fix-signup/
/new refactor-auth    # refactor/auth + .worktrees/refactor-auth/

# 여러 작업 동시 진행 가능
git worktree list
# /project                             [dev]
# /project/.worktrees/feature-login    [feature/login]
# /project/.worktrees/fix-signup       [fix/signup]

# 작업 후 PR 생성 (base: dev)
/pr

# PR merge 후 정리
git worktree remove .worktrees/feature-login
git branch -d feature/login

# dev → main 릴리스 PR
gh pr create --base main --title "release: v1.2.0"
```

### Worktree 디렉토리 규칙
- 위치: `.worktrees/{type}-{name}/` (프로젝트 내부, gitignore 필수)
- `.gitignore`에 `.worktrees/` 반드시 포함
- 각 worktree는 독립된 의존성·빌드 캐시 보유 (`go mod download` 자동 실행)
- `main` 직접 push 금지 — 반드시 `dev`를 거쳐 PR

---

## 아키텍처 규칙

### 디렉토리 구조
```
cmd/
└── main.go              # Entry point — DI 조립만 담당
internal/
├── domain/              # Entity, Repository interface, domain errors — 외부 의존성 없음
├── usecase/             # 비즈니스 로직 + UseCase DTO
├── repository/          # GORM + sqlc Repository 구현체
├── handler/             # Gin Handler + Response DTO
└── middleware/          # Auth, Logger, Recovery 등
migrations/              # golang-migrate SQL 파일 (up/down 쌍)
db/
├── query/               # sqlc SQL 쿼리 파일 (*.sql)
└── sqlc/                # sqlc 자동 생성 코드 (수동 수정 금지)
mocks/                   # mockery 자동 생성 mock
testutil/                # 테스트 Fixture
sqlc.yaml                # sqlc 설정
.golangci.yml            # golangci-lint 설정
```

### 레이어 의존 방향
`handler` → `usecase` → `domain` ← `repository`

**`domain/` 패키지는 어떤 외부 패키지도 import할 수 없습니다.**

---

## 반드시 지켜야 할 규칙 (MUST)

### 의존성 주입
```go
// ✅ 생성자 파라미터로만 주입
func NewOrderUseCase(repo domain.OrderRepository) *OrderUseCase {
    return &OrderUseCase{orderRepo: repo}
}

// ❌ 절대 금지 — 전역 변수
var db *gorm.DB
```

### 에러 처리
```go
// ✅ 에러 감싸서 전파
if err := r.db.First(&order, id).Error; err != nil {
    return nil, fmt.Errorf("FindByID: %w", err)
}

// ✅ 도메인 에러로 변환
if errors.Is(err, gorm.ErrRecordNotFound) {
    return nil, domain.ErrNotFound
}

// ❌ 절대 금지 — 에러 무시
result, _ := repo.FindByID(ctx, id)
```

### context 전파
```go
// ✅ 모든 레이어에서 context 전달
func (uc *OrderUseCase) GetOrder(ctx context.Context, id uint) (*domain.Order, error) {
    return uc.orderRepo.FindByID(ctx, id)
}

// ❌ context 누락
func (r *orderRepository) FindByID(id uint) (*domain.Order, error) {
```

### Handler 에러 응답
```go
// ✅ domain error → HTTP status 매핑
if errors.Is(err, domain.ErrNotFound) {
    c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
    return
}

// ❌ 절대 금지 — 내부 에러 메시지 그대로 노출
c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
```

---

## 절대 하면 안 되는 것 (NEVER)

- `domain/` 패키지에서 GORM, gin 등 외부 패키지 import
- Handler에서 Repository 직접 호출 (UseCase 우회)
- 전역 DB 연결 변수 사용
- `panic()`으로 에러 처리 (복구 가능한 에러에)
- 기존 Migration 파일 수정 (새 파일 추가만 가능)
- 패스워드, 토큰, PII를 로그에 출력
- 테스트 없이 새로운 UseCase 메서드 추가
- `context.Background()` 을 요청 핸들러에서 직접 사용 (`c.Request.Context()` 사용)
- `db/sqlc/` 아래 자동 생성 파일 수동 수정 (항상 `sqlc generate`로 재생성)
- sqlc 없이 raw SQL 문자열을 코드에 직접 작성
- golangci-lint 경고를 `//nolint` 주석으로 무분별하게 억제
- swag 주석 없이 새 Handler 엔드포인트 추가

---

## Swagger (swaggo) 규칙 (MUST)

### 설치
```bash
go install github.com/swaggo/swag/cmd/swag@latest
go get github.com/swaggo/gin-swagger
go get github.com/swaggo/files
```

### main.go — 전역 주석 + 라우트 등록
```go
// @title           [프로젝트명] API
// @version         1.0
// @description     API 명세서
// @host            localhost:8080
// @BasePath        /api/v1
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() { ... }
```

```go
// cmd/main.go — Swagger UI 라우트
import (
    swaggerFiles "github.com/swaggo/files"
    ginSwagger   "github.com/swaggo/gin-swagger"
    _ "github.com/yourorg/project/docs"  // swag generate 결과물
)

r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
```

### Handler 주석 (필수)
```go
// GetOrder godoc
// @Summary      주문 단건 조회
// @Description  ID로 주문을 조회합니다
// @Tags         orders
// @Produce      json
// @Param        id   path      int           true  "주문 ID"
// @Success      200  {object}  OrderResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Security     BearerAuth
// @Router       /orders/{id} [get]
func (h *OrderHandler) GetOrder(c *gin.Context) { ... }

// CreateOrder godoc
// @Summary      주문 생성
// @Tags         orders
// @Accept       json
// @Produce      json
// @Param        request  body      CreateOrderRequest  true  "주문 생성 요청"
// @Success      201      {object}  OrderResponse
// @Failure      400      {object}  ErrorResponse
// @Security     BearerAuth
// @Router       /orders [post]
func (h *OrderHandler) CreateOrder(c *gin.Context) { ... }
```

### Response/Request DTO 주석
```go
// OrderResponse godoc
type OrderResponse struct {
    ID        uint      `json:"id"         example:"1"`
    Status    string    `json:"status"     example:"PENDING"`
    CreatedAt time.Time `json:"created_at" example:"2024-01-01T00:00:00Z"`
}

// CreateOrderRequest godoc
type CreateOrderRequest struct {
    ProductID uint `json:"product_id" binding:"required" example:"10"`
    Quantity  int  `json:"quantity"   binding:"required,min=1" example:"2"`
}

// ErrorResponse godoc
type ErrorResponse struct {
    Error string `json:"error" example:"not found"`
}
```

### 문서 재생성 (Handler 주석 변경 시 필수)
```bash
swag init -g cmd/main.go -o docs
```

`.gitignore`에 `docs/` 추가 여부는 팀 정책에 따름 (CI에서 생성하는 경우 추가).

---

## sqlc 사용 규칙

### 쿼리 선택 기준
| 케이스 | 사용 기술 |
|--------|----------|
| 단순 CRUD (Insert, FindByID, Delete) | GORM |
| 조건 검색, 페이징, 조인 쿼리 | sqlc |
| 집계·통계·보고서 | sqlc |

### sqlc.yaml 기본 설정
```yaml
version: "2"
sql:
  - engine: "postgresql"   # 또는 mysql
    queries: "db/query/"
    schema: "migrations/"
    gen:
      go:
        package: "sqlcdb"
        out: "db/sqlc"
        emit_json_tags: true
        emit_interface: true
        emit_exact_table_names: false
```

### sqlc 쿼리 작성 예시
```sql
-- db/query/order.sql

-- name: ListOrdersByUserID :many
SELECT * FROM orders
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: GetOrderByID :one
SELECT * FROM orders WHERE id = $1;

-- name: SearchOrders :many
SELECT * FROM orders
WHERE (user_id = sqlc.narg('user_id') OR sqlc.narg('user_id') IS NULL)
  AND (status  = sqlc.narg('status')  OR sqlc.narg('status')  IS NULL)
ORDER BY created_at DESC;
```

### Repository에서 sqlc 사용
```go
// internal/repository/order_repository.go
type orderRepository struct {
    db      *gorm.DB
    queries *sqlcdb.Queries  // sqlc 자동 생성
}

// 단순 CRUD — GORM
func (r *orderRepository) Create(ctx context.Context, order *domain.Order) error {
    return r.db.WithContext(ctx).Create(order).Error
}

// 조건 검색 — sqlc
func (r *orderRepository) Search(ctx context.Context, params domain.OrderSearchParams) ([]*domain.Order, error) {
    rows, err := r.queries.SearchOrders(ctx, sqlcdb.SearchOrdersParams{
        UserID: pgtype.Int8{Int64: params.UserID, Valid: params.UserID != 0},
        Status: pgtype.Text{String: params.Status, Valid: params.Status != ""},
    })
    // ...
}
```

---

## golangci-lint 규칙

### .golangci.yml 기본 설정
```yaml
linters:
  enable:
    - errcheck       # 에러 무시 방지
    - govet          # go vet 검사
    - staticcheck    # 정적 분석
    - gosimple       # 코드 단순화 제안
    - unused         # 미사용 코드 탐지
    - gofmt          # 포맷 검사
    - goimports      # import 정렬
    - revive         # 스타일 검사
    - bodyclose      # HTTP response body 닫기 검사
    - noctx          # context 없는 HTTP 요청 탐지

linters-settings:
  revive:
    rules:
      - name: exported
      - name: var-naming

issues:
  exclude-use-default: false
  max-issues-per-linter: 0
  max-same-issues: 0
```

### lint 실행
```bash
# 로컬 실행
golangci-lint run ./...

# 특정 파일
golangci-lint run internal/usecase/...
```

**git push 전 `golangci-lint run ./...` 통과 필수** (`.claude/hooks/pre-push.sh` 자동 검사)

---

## 코드 품질 기준

- 모든 UseCase public 메서드에 단위 테스트 필수
- 새 Handler 엔드포인트마다 Handler 테스트 필수
- 새 DB 컬럼/테이블은 반드시 golang-migrate 파일과 함께 (up/down 쌍)
- mock은 직접 작성 금지 — mockery로 자동 생성

## 커버리지 게이트

**git push 전 라인 커버리지 80% 이상 필수** (`.claude/hooks/pre-push.sh` 자동 검사)

```bash
# 커버리지 확인
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out | grep total
```

---

## 학습된 규칙 (AI 실수 후 추가)

### 2026-04-14 — sqlc·golangci-lint 미사용으로 쿼리 안전성·코드 품질 저하
- **문제**: Go Gin 프로젝트에서 GORM만 사용하고 복잡한 쿼리를 raw SQL 문자열로 작성, lint 검사 없이 코드 생성
- **규칙**: 조건 검색·페이징·조인 쿼리는 **반드시 sqlc**로 타입 안전하게 생성. 모든 코드는 **golangci-lint** 통과 필수
- **이유**: raw SQL 문자열은 컴파일 타임 오류 검출 불가, lint 없이 작성된 코드는 errcheck 누락·미사용 변수 등 버그 유입 위험

<!-- /rule 커맨드로 새 규칙이 여기에 추가됩니다 -->

---

## 세션 시작 시 자동 참조

> 🧠 **새 작업을 시작하기 전에 `memory/MEMORY.md` 를 반드시 먼저 읽으세요.** 과거 결정·교훈을 맥락에 포함하여 같은 실수를 반복하지 않도록 합니다.

---

## Memory 관리 지침

> Claude는 아래 상황에서 `memory/MEMORY.md`를 **자동으로** 업데이트합니다.

**자동 기록 트리거:**
- `/plan` 승인 → 구현할 기능과 선택한 설계 방식 기록
- `/rule` 실행 → 어떤 실수였는지, 추가된 규칙 요약 기록
- 복잡한 버그 해결 → 원인, 해결 방법, 재발 방지 포인트 기록
- 외부 라이브러리/API 도입 결정 → 선택 이유, 대안 기록
- 아키텍처 또는 폴더 구조 변경 → 변경 전/후, 이유 기록

**`memory/MEMORY.md` vs `CLAUDE.md` 구분:**
- `memory/MEMORY.md` — 맥락과 히스토리 (왜 이 결정을 했는가)
- `CLAUDE.md` — 규칙 (앞으로 어떻게 해야 하는가)
