# Go Gin Code Patterns

## Generation Patterns

### Domain Entity
```go
// internal/domain/order.go
package domain

import "time"

type Order struct {
    ID        uint      `gorm:"primaryKey"`
    UserID    uint      `gorm:"not null;index"`
    Status    string    `gorm:"type:varchar(20);not null;default:'PENDING'"`
    CreatedAt time.Time
    UpdatedAt time.Time
}

type OrderStatus string

const (
    OrderStatusPending   OrderStatus = "PENDING"
    OrderStatusConfirmed OrderStatus = "CONFIRMED"
    OrderStatusCancelled OrderStatus = "CANCELLED"
)
```

### Domain Errors
```go
// internal/domain/errors.go
package domain

import "errors"

var (
    ErrNotFound     = errors.New("not found")
    ErrUnauthorized = errors.New("unauthorized")
    ErrBadRequest   = errors.New("bad request")
)
```

### Repository Interface (domain layer)
```go
// internal/domain/order_repository.go
package domain

import "context"

type OrderRepository interface {
    FindByID(ctx context.Context, id uint) (*Order, error)
    FindAllByUserID(ctx context.Context, userID uint) ([]*Order, error)
    Create(ctx context.Context, order *Order) error
    Update(ctx context.Context, order *Order) error
    Delete(ctx context.Context, id uint) error
}
```

### Repository Implementation (GORM + sqlc)
```go
// internal/repository/order_repository.go
package repository

import (
    "context"
    "errors"
    "fmt"

    "gorm.io/gorm"
    "github.com/yourorg/project/db/sqlc"
    "github.com/yourorg/project/internal/domain"
)

type orderRepository struct {
    db      *gorm.DB
    queries *sqlcdb.Queries // sqlc 자동 생성
}

func NewOrderRepository(db *gorm.DB, queries *sqlcdb.Queries) domain.OrderRepository {
    return &orderRepository{db: db, queries: queries}
}

// 단순 CRUD — GORM 사용
func (r *orderRepository) FindByID(ctx context.Context, id uint) (*domain.Order, error) {
    var order domain.Order
    if err := r.db.WithContext(ctx).First(&order, id).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            return nil, domain.ErrNotFound
        }
        return nil, fmt.Errorf("FindByID: %w", err)
    }
    return &order, nil
}

func (r *orderRepository) Create(ctx context.Context, order *domain.Order) error {
    return r.db.WithContext(ctx).Create(order).Error
}

func (r *orderRepository) Update(ctx context.Context, order *domain.Order) error {
    return r.db.WithContext(ctx).Save(order).Error
}

func (r *orderRepository) Delete(ctx context.Context, id uint) error {
    return r.db.WithContext(ctx).Delete(&domain.Order{}, id).Error
}

// 조건 검색·페이징 — sqlc 사용
func (r *orderRepository) Search(ctx context.Context, params domain.OrderSearchParams) ([]*domain.Order, error) {
    rows, err := r.queries.SearchOrders(ctx, sqlcdb.SearchOrdersParams{
        UserID: toNullInt64(params.UserID),
        Status: toNullString(params.Status),
        Limit:  int32(params.Limit),
        Offset: int32(params.Offset),
    })
    if err != nil {
        return nil, fmt.Errorf("Search: %w", err)
    }
    orders := make([]*domain.Order, 0, len(rows))
    for _, row := range rows {
        orders = append(orders, toDomainOrder(row))
    }
    return orders, nil
}
```

### sqlc 쿼리 파일
```sql
-- db/query/order.sql

-- name: GetOrderByID :one
SELECT * FROM orders WHERE id = $1;

-- name: ListOrdersByUserID :many
SELECT * FROM orders
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: SearchOrders :many
SELECT * FROM orders
WHERE (user_id = sqlc.narg('user_id') OR sqlc.narg('user_id') IS NULL)
  AND (status  = sqlc.narg('status')  OR sqlc.narg('status')  IS NULL)
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;

-- name: CreateOrder :one
INSERT INTO orders (user_id, status, created_at, updated_at)
VALUES ($1, $2, NOW(), NOW())
RETURNING *;
```

### sqlc.yaml
```yaml
version: "2"
sql:
  - engine: "postgresql"
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

> **쿼리 선택 기준**
> - 단순 CRUD (FindByID, Create, Update, Delete) → **GORM**
> - 조건 검색, 페이징, 조인, 집계 → **sqlc**
> - `db/sqlc/` 파일은 `sqlc generate`로만 갱신 — 수동 수정 금지

### UseCase
```go
// internal/usecase/order_usecase.go
package usecase

import (
    "context"
    "github.com/yourorg/project/internal/domain"
)

type OrderUseCase struct {
    orderRepo domain.OrderRepository
}

func NewOrderUseCase(orderRepo domain.OrderRepository) *OrderUseCase {
    return &OrderUseCase{orderRepo: orderRepo}
}

func (uc *OrderUseCase) GetOrder(ctx context.Context, id uint) (*domain.Order, error) {
    return uc.orderRepo.FindByID(ctx, id)
}

func (uc *OrderUseCase) CreateOrder(ctx context.Context, userID uint, req *CreateOrderRequest) (*domain.Order, error) {
    order := &domain.Order{
        UserID: userID,
        Status: string(domain.OrderStatusPending),
    }
    if err := uc.orderRepo.Create(ctx, order); err != nil {
        return nil, err
    }
    return order, nil
}

// UseCase DTOs
type CreateOrderRequest struct {
    ProductID uint `json:"product_id" binding:"required"`
    Quantity  int  `json:"quantity"   binding:"required,min=1"`
}
```

### Handler (swag 주석 필수)
```go
// internal/handler/order_handler.go
package handler

import (
    "errors"
    "net/http"
    "strconv"

    "github.com/gin-gonic/gin"
    "github.com/yourorg/project/internal/domain"
    "github.com/yourorg/project/internal/usecase"
)

type OrderHandler struct {
    orderUC *usecase.OrderUseCase
}

func NewOrderHandler(orderUC *usecase.OrderUseCase) *OrderHandler {
    return &OrderHandler{orderUC: orderUC}
}

func (h *OrderHandler) RegisterRoutes(rg *gin.RouterGroup) {
    g := rg.Group("/orders")
    g.GET("/:id", h.GetOrder)
    g.POST("", h.CreateOrder)
}

// GetOrder godoc
// @Summary      주문 단건 조회
// @Tags         orders
// @Produce      json
// @Param        id   path      int           true  "주문 ID"
// @Success      200  {object}  OrderResponse
// @Failure      404  {object}  ErrorResponse
// @Security     BearerAuth
// @Router       /orders/{id} [get]
func (h *OrderHandler) GetOrder(c *gin.Context) {
    id, err := strconv.ParseUint(c.Param("id"), 10, 32)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
        return
    }

    order, err := h.orderUC.GetOrder(c.Request.Context(), uint(id))
    if err != nil {
        if errors.Is(err, domain.ErrNotFound) {
            c.JSON(http.StatusNotFound, gin.H{"error": "order not found"})
            return
        }
        c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
        return
    }

    c.JSON(http.StatusOK, toOrderResponse(order))
}

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
func (h *OrderHandler) CreateOrder(c *gin.Context) {
    var req usecase.CreateOrderRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    userID := c.GetUint("userID") // auth middleware에서 주입
    order, err := h.orderUC.CreateOrder(c.Request.Context(), userID, &req)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
        return
    }

    c.JSON(http.StatusCreated, toOrderResponse(order))
}
```

### Response / Request DTO (swag example 태그 필수)
```go
// internal/handler/order_response.go
package handler

import (
    "time"
    "github.com/yourorg/project/internal/domain"
)

type OrderResponse struct {
    ID        uint      `json:"id"         example:"1"`
    Status    string    `json:"status"     example:"PENDING"`
    CreatedAt time.Time `json:"created_at" example:"2024-01-01T00:00:00Z"`
}

type ErrorResponse struct {
    Error string `json:"error" example:"not found"`
}

func toOrderResponse(order *domain.Order) *OrderResponse {
    return &OrderResponse{
        ID:        order.ID,
        Status:    order.Status,
        CreatedAt: order.CreatedAt,
    }
}
```

### Migration SQL (golang-migrate)
```sql
-- migrations/000001_create_orders_table.up.sql
CREATE TABLE orders (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    user_id    BIGINT UNSIGNED NOT NULL,
    status     VARCHAR(20)     NOT NULL DEFAULT 'PENDING',
    created_at DATETIME(3)     NOT NULL,
    updated_at DATETIME(3)     NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_orders_user_id (user_id)
);

-- migrations/000001_create_orders_table.down.sql
DROP TABLE IF EXISTS orders;
```

### Router Wiring (cmd/main.go)
```go
func setupRouter(db *gorm.DB) *gin.Engine {
    r := gin.New()
    r.Use(gin.Recovery(), middleware.Logger())

    orderRepo := repository.NewOrderRepository(db)
    orderUC   := usecase.NewOrderUseCase(orderRepo)
    orderH    := handler.NewOrderHandler(orderUC)

    api := r.Group("/api/v1")
    api.Use(middleware.Auth())
    {
        orderH.RegisterRoutes(api)
    }

    return r
}
```

---

## Modification Patterns

### 필드 추가
```go
// 1. domain entity에 추가
type Order struct {
    // ...
    Description string `gorm:"type:varchar(500)"` // ADDED
}

// 2. migration 생성
-- migrations/000002_add_description_to_orders.up.sql
ALTER TABLE orders ADD COLUMN description VARCHAR(500) NULL;

-- migrations/000002_add_description_to_orders.down.sql
ALTER TABLE orders DROP COLUMN description;

// 3. response DTO 업데이트
type OrderResponse struct {
    // ...
    Description string `json:"description"` // ADDED
}

func toOrderResponse(order *domain.Order) *OrderResponse {
    return &OrderResponse{
        // ...
        Description: order.Description, // ADDED
    }
}
```

### 새 엔드포인트 추가
```go
// Handler에 메서드 추가
func (h *OrderHandler) CancelOrder(c *gin.Context) {
    // ...
}

// RegisterRoutes에 라우트 추가
func (h *OrderHandler) RegisterRoutes(rg *gin.RouterGroup) {
    g := rg.Group("/orders")
    g.GET("/:id", h.GetOrder)
    g.POST("", h.CreateOrder)
    g.DELETE("/:id", h.CancelOrder) // ADDED
}

// UseCase에 메서드 추가 (handler → usecase 순으로)
func (uc *OrderUseCase) CancelOrder(ctx context.Context, id uint) error {
    order, err := uc.orderRepo.FindByID(ctx, id)
    if err != nil {
        return err
    }
    order.Status = string(domain.OrderStatusCancelled)
    return uc.orderRepo.Update(ctx, order)
}
```

---

## Test Patterns

### Testutil Fixture
```go
// testutil/order_fixture.go
package testutil

import (
    "time"
    "github.com/yourorg/project/internal/domain"
)

func OrderFixture(opts ...func(*domain.Order)) *domain.Order {
    o := &domain.Order{
        ID:        1,
        UserID:    1,
        Status:    "PENDING",
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
    }
    for _, opt := range opts {
        opt(o)
    }
    return o
}
```

### UseCase Unit Test (mockery)
```go
// internal/usecase/order_usecase_test.go
package usecase_test

import (
    "context"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "github.com/yourorg/project/internal/domain"
    "github.com/yourorg/project/internal/usecase"
    "github.com/yourorg/project/mocks"
    "github.com/yourorg/project/testutil"
)

func TestOrderUseCase_GetOrder(t *testing.T) {
    t.Run("주문이 존재하면 반환한다", func(t *testing.T) {
        mockRepo := mocks.NewOrderRepository(t)
        uc := usecase.NewOrderUseCase(mockRepo)

        expected := testutil.OrderFixture()
        mockRepo.On("FindByID", mock.Anything, uint(1)).Return(expected, nil)

        result, err := uc.GetOrder(context.Background(), 1)

        assert.NoError(t, err)
        assert.Equal(t, expected.ID, result.ID)
        mockRepo.AssertExpectations(t)
    })

    t.Run("주문이 없으면 ErrNotFound를 반환한다", func(t *testing.T) {
        mockRepo := mocks.NewOrderRepository(t)
        uc := usecase.NewOrderUseCase(mockRepo)

        mockRepo.On("FindByID", mock.Anything, uint(999)).Return(nil, domain.ErrNotFound)

        result, err := uc.GetOrder(context.Background(), 999)

        assert.Nil(t, result)
        assert.ErrorIs(t, err, domain.ErrNotFound)
        mockRepo.AssertExpectations(t)
    })
}
```

### Handler Test (httptest)
```go
// internal/handler/order_handler_test.go
package handler_test

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "github.com/yourorg/project/internal/domain"
    "github.com/yourorg/project/internal/handler"
    "github.com/yourorg/project/mocks"
    "github.com/yourorg/project/testutil"
)

func init() { gin.SetMode(gin.TestMode) }

func TestOrderHandler_GetOrder(t *testing.T) {
    t.Run("유효한 ID로 주문을 조회한다", func(t *testing.T) {
        mockUC := mocks.NewOrderUseCase(t)
        h := handler.NewOrderHandler(mockUC)

        order := testutil.OrderFixture()
        mockUC.On("GetOrder", mock.Anything, uint(1)).Return(order, nil)

        w := httptest.NewRecorder()
        r := gin.New()
        r.GET("/orders/:id", h.GetOrder)
        r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/orders/1", nil))

        assert.Equal(t, http.StatusOK, w.Code)

        var resp handler.OrderResponse
        assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
        assert.Equal(t, uint(1), resp.ID)
    })

    t.Run("존재하지 않는 주문은 404를 반환한다", func(t *testing.T) {
        mockUC := mocks.NewOrderUseCase(t)
        h := handler.NewOrderHandler(mockUC)

        mockUC.On("GetOrder", mock.Anything, uint(999)).Return(nil, domain.ErrNotFound)

        w := httptest.NewRecorder()
        r := gin.New()
        r.GET("/orders/:id", h.GetOrder)
        r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/orders/999", nil))

        assert.Equal(t, http.StatusNotFound, w.Code)
    })
}
```

### Test Anti-patterns
- `t.Fatal` / `panic` 대신 `assert.NoError` 사용
- context.Background() 대신 테스트용 timeout context 고려
- mock 없이 실제 DB에 연결하는 단위 테스트 작성 금지 (통합 테스트와 분리)
- 에러 무시 (`_`) 후 assertion 하는 패턴

---

## Multi-Service Patterns (Go Workspace)

### go.work
```
go 1.23

use (
    ./services/api
    ./services/worker
    ./pkg/shared
)
```

### pkg/shared/go.mod
```
module github.com/{org}/{project}/pkg/shared

go 1.23
```

### services/api/go.mod
```
module github.com/{org}/{project}/services/api

go 1.23

require (
    github.com/{org}/{project}/pkg/shared v0.0.0
    github.com/gin-gonic/gin v1.10.0
    // ...
)
```

### 공유 도메인 (pkg/shared/domain/)
```go
// pkg/shared/domain/user.go — 서비스 공통 Entity
package domain

type UserID uint

type User struct {
    ID    UserID
    Email string
    Name  string
}

// 서비스별 확장은 각 서비스 internal/domain/ 에서 embed
```

### 서비스에서 공유 모듈 사용
```go
// services/api/internal/domain/order.go
import "github.com/{org}/{project}/pkg/shared/domain"

type Order struct {
    ID     uint
    UserID domain.UserID  // ✅ 공유 타입 사용
    Status string
}
```

### 의존 규칙
| 모듈 | 의존 가능 | 의존 불가 |
|------|----------|----------|
| `pkg/shared` | (없음 — 순수) | services/* |
| `services/api` | `pkg/shared` | `services/worker` |
| `services/worker` | `pkg/shared` | `services/api` |

### Workspace 전체 빌드/테스트
```bash
# workspace 루트에서
go work sync
go test ./...              # 전체 테스트
go build ./...             # 전체 빌드

# 특정 서비스만
cd services/api && go test ./...
cd services/worker && go run cmd/main.go
```

### golangci-lint (workspace)
```bash
# 각 서비스 디렉토리에서 실행 (workspace root에서는 미지원)
cd services/api && golangci-lint run ./...
cd services/worker && golangci-lint run ./...
```
