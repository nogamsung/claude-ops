# [기능명] — [역할] 구현 프롬프트

> 이 파일은 `/planner` 가 생성한 **역할별 구현 지시서**입니다.
> 대응하는 PRD: [`../[feature].md`](../[feature].md)
> 대응하는 스택: [stack-type] (경로: `[stack-path]`)

---

## 맥락 (꼭 읽을 것)

- PRD 본문: `../[feature].md`
- 이 역할의 CLAUDE.md: `[stack-path]/CLAUDE.md` — 아키텍처 규칙, 패키지 구조, 금지사항
- 모노레포 매니페스트: `.claude/stacks.json`
- 관련 패턴 스킬: `.claude/skills/[stack-type]-patterns.md`

## 이 역할의 책임 범위

PRD 전체 중 **[role]** 이 담당하는 것만 여기에 실행합니다.

- 포함: (예) DB 스키마·Migration, API 엔드포인트·Controller·Service·Repository, 인증 미들웨어, 단위·통합 테스트
- 제외: UI (frontend 담당), Push 알림 (mobile 담당)

## 변경할/생성할 파일 (체크리스트)

> 실제 경로는 `[stack-path]/` 기준으로 기술. `/new` 커맨드의 표준 배치를 따릅니다.

### Migration / DB (해당 시)
- [ ] `[stack-path]/src/main/resources/db/migration/V{N}__create_[tables]_table.sql` (Kotlin/Flyway)
- [ ] `[stack-path]/migrations/{N:06d}_create_[tables]_table.{up,down}.sql` (Go/golang-migrate)

### Domain / Entity
- [ ] `[stack-path]/.../domain/[Entity].kt` 또는 `internal/domain/[entity].go`

### Repository
- [ ] 인터페이스 + 구현 (QueryDSL 3세트 / sqlc)

### Service / UseCase
- [ ] 비즈니스 로직 + 예외 처리

### Controller / Handler
- [ ] REST 엔드포인트 + DTO + OpenAPI 어노테이션

### Frontend 컴포넌트 / 화면 (해당 시)
- [ ] `[stack-path]/app/.../page.tsx` 또는 `components/features/.../[Name].tsx`
- [ ] API 클라이언트 훅 (`use[Name]`)
- [ ] 폼 상태 (React Hook Form + Zod)

### Mobile 화면 (해당 시)
- [ ] `[stack-path]/lib/features/[feature]/presentation/screens/[name]_screen.dart`
- [ ] Provider / AsyncNotifier
- [ ] RemoteDataSource + Repository

### 테스트
- [ ] 단위 테스트 (Service/UseCase 단위)
- [ ] 통합/위젯 테스트 (Controller/Handler/Widget)
- [ ] 커버리지 90% 이상 (pre-push 게이트)

## 구현 제약 (해당 스택 CLAUDE.md 와 충돌하지 않을 것)

이 역할의 `CLAUDE.md` 에 정의된 **반드시 지켜야 할 규칙**, **절대 하면 안 되는 것** 을 우선합니다. 이 프롬프트는 그 제약 안에서 동작해야 합니다.

## 다른 역할과의 계약 (Interface)

- **→ frontend 로 제공**: `POST /api/v1/...` 응답 스키마 `[ResponseDTO]`
- **← mobile 에서 호출됨**: 동일 엔드포인트, JWT Bearer 인증
- **← backend 가 제공**: (frontend/mobile 프롬프트에서는 반대로 기술)

계약 변경 시 반드시 PRD 의 "API 계약" 섹션 먼저 갱신 → 다른 역할 프롬프트 업데이트.

## 실행 지시

이 프롬프트를 받은 agent 는 아래 순서로 진행합니다:

1. `[stack-path]/CLAUDE.md` 를 먼저 읽어 스택별 규칙 숙지
2. 관련 기존 코드(엔티티·레이어·테스트 패턴) 탐색 — 있으면 패턴 재사용
3. 위 체크리스트의 **필요한 항목만** 생성 (예: Migration 불필요하면 스킵)
4. 파일 생성 순서: Migration → Domain → Repository → Service → Controller → Tests
5. 생성 완료 후 요약 리포트:
   - 생성된 파일 목록
   - 변경된 기존 파일 목록
   - 다른 역할(frontend/mobile)에게 알려야 하는 계약 변경사항
   - 후속 수동 작업 (예: `sqlc generate`, `./gradlew ktlintFormat`)

## 성공 기준

- 모든 체크리스트 항목 체크됨
- `[stack-path]` 에서 테스트 전체 통과
- 커버리지 90% 이상
- 이 역할의 CLAUDE.md 가 명시한 금지사항 위반 없음

---

> 이 프롬프트는 `/planner` 가 자동 생성했습니다. 수동 수정 후 agent 에게 전달해도 됩니다.
