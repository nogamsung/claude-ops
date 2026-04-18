# API Design Patterns

백엔드 REST API 설계 시 참조하는 패턴 라이브러리입니다.
Kotlin Spring Boot (`/new api`)와 Go Gin (`/new api`) 양쪽에서 공통으로 사용합니다.

---

## 1. 리소스 모델링

### URL 구조 원칙
```
GET    /api/v1/orders          # 목록 조회 (복수형)
POST   /api/v1/orders          # 단건 생성
GET    /api/v1/orders/{id}     # 단건 조회
PUT    /api/v1/orders/{id}     # 전체 수정
PATCH  /api/v1/orders/{id}     # 부분 수정
DELETE /api/v1/orders/{id}     # 삭제

# 중첩 리소스 (소유 관계가 명확할 때만)
GET    /api/v1/orders/{id}/items
POST   /api/v1/orders/{id}/items

# 액션 엔드포인트 (동사가 필요할 때)
POST   /api/v1/orders/{id}/cancel
POST   /api/v1/orders/{id}/ship
```

### 네이밍 규칙
- URL: `kebab-case` (예: `/user-profiles`, `/order-items`)
- Query param: `snake_case` (예: `?sort_by=created_at&page_size=20`)
- JSON body/response: `camelCase`

---

## 2. 표준 응답 형식

### 단건 응답
```json
{
  "id": 1,
  "status": "PENDING",
  "createdAt": "2024-01-15T09:30:00Z"
}
```

### 목록 응답 (페이지네이션)
```json
{
  "content": [...],
  "page": {
    "number": 0,
    "size": 20,
    "totalElements": 153,
    "totalPages": 8
  }
}
```

### 커서 기반 페이지네이션 (대용량)
```json
{
  "content": [...],
  "cursor": {
    "next": "eyJpZCI6MTAwfQ==",
    "hasMore": true
  }
}
```

---

## 3. 표준 에러 응답 (RFC 7807 Problem Details)

```json
{
  "type": "https://api.example.com/errors/validation-failed",
  "title": "Validation Failed",
  "status": 400,
  "detail": "요청 데이터가 유효하지 않습니다.",
  "errors": [
    { "field": "email", "message": "올바른 이메일 형식이 아닙니다." },
    { "field": "price", "message": "0보다 커야 합니다." }
  ]
}
```

### HTTP 상태코드 사용 원칙
| 상태 | 상황 |
|------|------|
| 200 OK | 조회, 수정 성공 |
| 201 Created | 생성 성공 (`Location` 헤더 포함) |
| 204 No Content | 삭제 성공, 응답 body 없음 |
| 400 Bad Request | 클라이언트 입력값 오류 |
| 401 Unauthorized | 인증 없음 (토큰 없음/만료) |
| 403 Forbidden | 인증은 됐지만 권한 없음 |
| 404 Not Found | 리소스 없음 |
| 409 Conflict | 중복 생성, 상태 충돌 |
| 422 Unprocessable Entity | 형식은 맞지만 비즈니스 규칙 위반 |
| 500 Internal Server Error | 서버 내부 오류 |

---

## 4. 인증/인가 패턴

### Bearer Token (JWT)
```
Authorization: Bearer eyJhbGciOiJIUzI1NiJ9...
```

### API Key
```
X-API-Key: sk-proj-abc123
```

### OpenAPI 보안 스키마
```yaml
components:
  securitySchemes:
    BearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT
    ApiKeyAuth:
      type: apiKey
      in: header
      name: X-API-Key

security:
  - BearerAuth: []
```

---

## 5. 페이지네이션 / 필터링 Query Params

```
# 오프셋 페이지네이션
GET /api/v1/orders?page=0&size=20&sort=createdAt,desc

# 필터
GET /api/v1/orders?status=PENDING&user_id=42

# 검색
GET /api/v1/orders?q=keyword&from=2024-01-01&to=2024-12-31
```

---

## 6. 스택별 OpenAPI 어노테이션

### Kotlin Spring Boot (SpringDoc)
```kotlin
@Tag(name = "Orders", description = "주문 관리 API")
@RestController
@RequestMapping("/api/v1/orders")
class OrderController {

    @Operation(summary = "주문 목록 조회", description = "페이지네이션을 지원합니다.")
    @ApiResponse(responseCode = "200", description = "성공",
        content = [Content(schema = Schema(implementation = PagedOrderResponse::class))])
    @ApiResponse(responseCode = "401", description = "인증 필요")
    @GetMapping
    fun getOrders(
        @Parameter(description = "페이지 번호 (0부터 시작)", example = "0")
        @RequestParam(defaultValue = "0") page: Int
    ): ResponseEntity<PagedOrderResponse> { ... }
}
```

### Go Gin (swag)
```go
// GetOrders godoc
// @Summary      주문 목록 조회
// @Description  페이지네이션을 지원합니다.
// @Tags         orders
// @Produce      json
// @Param        page   query    int  false  "페이지 번호 (0부터 시작)"  default(0)
// @Success      200    {object} PagedOrderResponse
// @Failure      401    {object} ErrorResponse
// @Router       /api/v1/orders [get]
// @Security     BearerAuth
func (h *OrderHandler) GetOrders(c *gin.Context) { ... }
```

---

## 7. API 버전 관리 전략

| 전략 | 형태 | 추천 상황 |
|------|------|----------|
| URL 버전 | `/api/v1/`, `/api/v2/` | 기본값. 명확하고 캐시 친화적 |
| Header 버전 | `Accept: application/vnd.api+json;version=2` | URL 깔끔하게 유지할 때 |
| Query 버전 | `?version=2` | 빠른 프로토타입, 지양 권장 |

> 이 프로젝트 기본값: **URL 버전** (`/api/v1/`)

---

## 8. /plan api → /new api / /new api 연결 플로우

```
/plan api {리소스명}
  ↓ 엔드포인트 목록 설계
  ↓ Request/Response 스키마 설계
  ↓ OpenAPI YAML 초안 출력
  ↓ 사용자 확인
  ↓
  ├── Kotlin 프로젝트  →  /new api {리소스명}
  └── Go 프로젝트      →  /new api {리소스명}
```
