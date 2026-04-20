# scheduled-dev-agent — Go Gin

## Stack
Go · Gin · GORM(단순 CRUD) + **sqlc**(동적·복잡 쿼리) · golang-migrate · **golangci-lint** · **swaggo/swag** · testify + mockery

## Agents & Commands
| 목적 | Agent / Command |
|------|----------------|
| 새 파일 생성 | `go-generator` |
| 기존 코드 수정 | `go-modifier` |
| 테스트 작성 | `go-tester` |
| 코드 리뷰 | `code-reviewer` · `/review` |
| API 설계 | `/plan api <Resource>` |
| DB 설계 | `/plan db <도메인>` |
| REST API 스캐폴딩 | `/new <Resource>` |
| 커밋/PR/머지 | `/commit` · `/pr` · `/merge` |
| 기획 → 프롬프트 | `/planner <기능>` |
| Second Brain | `/memory [add\|search]` |

## Git 전략
| 브랜치 | 역할 |
|--------|------|
| `main` | 통합·프로덕션 — PR+CI 필수 (단일 브랜치 전략, dev 없음) |
| `{feature\|fix\|hotfix\|refactor\|chore}/{name}` | 작업 브랜치 — PR base 는 항상 `main` |

Worktree `.worktrees/{type}-{name}/`. `main` 직접 push 금지.

## 디렉토리 구조
```
cmd/main.go              # DI 조립만
internal/
├── domain/              # Entity, Repository interface — 외부 import 금지
├── usecase/             # 비즈니스 로직 + DTO
├── repository/          # GORM + sqlc 구현체
├── handler/             # Gin Handler + Response DTO (swag godoc 필수)
└── middleware/
migrations/              # up/down 쌍
db/{query,sqlc}/         # sqlc SQL · 자동 생성 (수동 수정 금지)
mocks/ · testutil/
sqlc.yaml · .golangci.yml
```

**레이어 의존**: `handler` → `usecase` → `domain` ← `repository`. `domain/` 은 어떤 외부 패키지도 import 금지.

## MUST
- **DI**: 생성자 파라미터 주입만 — 전역 `var db *gorm.DB` 금지
- **에러**: `fmt.Errorf("...: %w", err)` 감싸기, `gorm.ErrRecordNotFound` → `domain.ErrNotFound` 변환
- **context**: 모든 레이어 `ctx context.Context` 첫 인자, 요청 핸들러는 `c.Request.Context()`
- **Handler 응답**: domain error → HTTP status 매핑. 내부 에러 메시지 그대로 노출 금지
- **sqlc**: 조건 검색·페이징·조인·집계 쿼리는 sqlc 필수 (raw SQL 문자열 금지)
- **swag**: 모든 Handler 에 `@Summary` · `@Tags` · `@Router` · `@Success` · `@Failure` godoc 필수
- **Response DTO**: `example:"..."` json 태그 필수

## NEVER
- `domain/` 에서 GORM, gin 등 외부 패키지 import
- Handler → Repository 직접 호출 (UseCase 우회)
- 전역 DB 연결 변수
- 복구 가능한 에러에 `panic()`
- 기존 migration 수정 (새 파일 추가만)
- 패스워드·토큰·PII 로그
- 테스트 없이 UseCase 메서드 추가
- `context.Background()` 를 요청 핸들러에서 직접 사용
- `db/sqlc/` 수동 수정 (`sqlc generate` 로 재생성)
- sqlc 없이 raw SQL 하드코딩
- `//nolint` 이유 없이 남발
- swag 주석 없이 엔드포인트 추가

## 명령어
```bash
go build ./... / go test ./... / go vet ./...
golangci-lint run ./...
sqlc generate
swag init -g cmd/main.go -o docs
mockery --name=<Interface> --dir=internal/domain --output=mocks
```

**상세 패턴**: `.claude/skills/go-patterns.md` · API 설계: `.claude/skills/api-design-patterns.md`.

**커버리지 게이트**: git push 전 ≥80% + `golangci-lint` 통과 (`.claude/hooks/pre-push.sh`).

## 학습된 규칙

### 2026-04-14 — sqlc·golangci-lint 필수화
동적 쿼리는 sqlc 로 타입 안전 생성. raw SQL 문자열 금지. 모든 코드 golangci-lint 통과 필수.

<!-- /rule 로 여기에 추가됩니다 -->

## Memory
세션 시작 시 `memory/MEMORY.md` 자동 로드. `/plan`, `/rule`, 버그 해결, 라이브러리 도입, 아키텍처 변경 → 자동 기록.
