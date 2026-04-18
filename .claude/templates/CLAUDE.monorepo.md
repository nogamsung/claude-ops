# [프로젝트명] — Monorepo

## Stack (Monorepo)

이 저장소는 여러 스택이 공존하는 **모노레포**입니다.
각 스택별 세부 규칙은 해당 디렉토리의 `CLAUDE.md` 를 참고하세요.

Claude Code 는 현재 편집 중인 파일의 **상위 디렉토리들의 `CLAUDE.md` 를 누적 로드**합니다.
즉 `backend/src/…/Foo.kt` 를 작업할 때는 루트 + `backend/CLAUDE.md` 가 모두 컨텍스트에 들어옵니다.

| 역할 | 경로 | 스택 | 세부 규칙 |
|------|------|------|-----------|
| backend | `[backend-path]` | [backend-stack] | [`[backend-path]/CLAUDE.md`](./[backend-path]/CLAUDE.md) |
| frontend | `[frontend-path]` | [frontend-stack] | [`[frontend-path]/CLAUDE.md`](./[frontend-path]/CLAUDE.md) |
| mobile | `[mobile-path]` | [mobile-stack] | [`[mobile-path]/CLAUDE.md`](./[mobile-path]/CLAUDE.md) |

> `/init` 실행 시 실제 감지된 스택만 남고 나머지 행은 제거됩니다.

## 스택 매니페스트

`.claude/stacks.json` 이 이 저장소의 **단일 진실의 원천**입니다.
`/new`, `/plan`, `/test`, 그리고 `.claude/hooks/pre-push.sh` 는 이 파일을 읽어 스택별로 분기합니다.

```json
{
  "mode": "monorepo",
  "stacks": [
    { "role": "backend",  "type": "[backend-stack]",  "path": "[backend-path]" },
    { "role": "frontend", "type": "[frontend-stack]", "path": "[frontend-path]" },
    { "role": "mobile",   "type": "[mobile-stack]",   "path": "[mobile-path]" }
  ]
}
```

## Agents (역할별)

| 역할 | 새 파일 | 기존 수정 | 테스트 |
|------|---------|-----------|--------|
| backend | `{backend}-generator` | `{backend}-modifier` | `{backend}-tester` |
| frontend | `nextjs-generator` | `nextjs-modifier` | `nextjs-tester` |
| mobile | `flutter-generator` | `flutter-modifier` | `flutter-tester` |
| 공통 | `code-reviewer`, `api-designer`¹, `ui-designer`², `github-actions-designer` | | |

¹ backend 존재 시에만 활성
² frontend 또는 mobile 존재 시에만 활성

## Commands — 모노레포 사용법

모노레포에서는 **역할 prefix** 로 대상 스택을 명시합니다.

| 커맨드 | 단일 레포 | 모노레포 |
|--------|-----------|---------|
| `/planner <기능>` | PRD + 단일 프롬프트 | **PRD + backend/frontend/mobile 프롬프트 3세트** (역할 자동 분배, `--teams` 로 병렬 실행 가능) |
| `/new api User` | 루트에서 실행 | `/new backend api User` → `[backend-path]/` 에서 실행 |
| `/new component Button` | 루트에서 실행 | `/new frontend component Button` → `[frontend-path]/` 에서 실행 |
| `/new screen Login` | 루트에서 실행 | `/new mobile screen Login` → `[mobile-path]/` 에서 실행 |
| `/plan api User` | 루트 스택 감지 | `/plan backend api User` — 역할 prefix 로 스택 확정 |
| `/plan db order` | 루트 스택 감지 | `/plan backend db order` |
| `/test <파일>` | 루트에서 실행 | 파일 경로로 스택 자동 판단 |
| `/review` | 루트 전체 | 파일 경로로 스택 자동 판단 |

### 기획 → 구현 워크플로 (권장)

```
1. /planner 결제 취소 기능
     → 인터뷰 후 PRD 작성:
         docs/specs/payment-cancel.md
     → 역할별 프롬프트 자동 분배:
         docs/specs/payment-cancel/backend.md
         docs/specs/payment-cancel/frontend.md
         docs/specs/payment-cancel/mobile.md
2. 실행 모드 선택:
     (a) Agent Teams   → backend·frontend·mobile generator 를 병렬 호출 (한 메시지)
     (b) 프롬프트 출력 → 수동으로 각 역할에서 /new ... 실행
3. /review               → 3개 역할 일괄 리뷰
4. /pr                   → PR 생성
```

**역할 prefix** (stacks.json 의 `role` 값):
- `backend` — backend 스택의 경로에서 실행
- `frontend` — frontend 스택의 경로에서 실행
- `mobile` — mobile 스택의 경로에서 실행

prefix 생략 시 — 감지된 스택이 1개면 자동 선택, 2개 이상이면 사용자에게 확인 요청.

## 역할 디렉토리 이름 (별칭 허용)

`/init` 은 아래 후보 이름을 순서대로 탐색해 첫 번째로 발견된 것을 해당 역할로 사용합니다.

| 역할 | 허용 이름 |
|------|-----------|
| backend | `backend`, `api`, `server` |
| frontend | `frontend`, `web`, `client` |
| mobile | `mobile`, `app` |

별칭 디렉토리명을 써도 `/new`·`/plan` 등의 **prefix 는 표준 이름** (`backend`/`frontend`/`mobile`) 을 사용합니다. 실제 경로는 `.claude/stacks.json` 에서 lookup 됩니다.

## 중첩 멀티모듈 지원

각 역할 디렉토리 내부에서 다시 멀티모듈 구성이 가능합니다.

- `[backend-path]/settings.gradle.kts` 에 `include(` 가 있으면 → `kotlin-multi` 로 감지
- `[backend-path]/go.work` 가 있으면 → `go-multi` 로 감지
- `[frontend-path]/turbo.json` 이 있으면 → `nextjs-multi` 로 감지

`/new module <name>` 은 자동으로 해당 역할 디렉토리에서 실행됩니다.

## Git 브랜치 전략

단일 레포와 동일합니다. 자세한 규칙은 각 스택별 `CLAUDE.md` 참고.

- `main` ← 프로덕션
- `dev` (선택) ← 통합·스테이징
- `feature/*`, `fix/*`, `hotfix/*`, `refactor/*` ← 작업 브랜치

## 커버리지 게이트

`git push` 시 `.claude/hooks/pre-push.sh` 가 **활성 스택 전부** 순차 검증:

- backend (kotlin/kotlin-multi) → `./gradlew test jacocoTestReport` (해당 경로에서)
- backend (go/go-multi) → `go test -coverprofile=coverage.out ./...` (해당 경로에서)
- frontend (nextjs/nextjs-multi) → `npx jest --coverage` (해당 경로에서)
- mobile (flutter) → `flutter test --coverage` (해당 경로에서)

**임계값 90%**. 한 스택이라도 실패하면 push 차단.

---

## 주의사항 (공통)

- **스택 경계를 넘지 마세요** — backend 코드가 frontend 디렉토리를 직접 참조하지 않습니다. API 계약(`docs/api/*.yaml`) 으로만 연결합니다.
- **의존성 버전 관리** — 각 스택은 독립된 의존성 파일(`build.gradle.kts`, `package.json`, `pubspec.yaml`) 을 가집니다. 루트에 공유 manifest 없습니다.
- **CI/CD 분리** — GitHub Actions 워크플로도 `paths-filter` 로 변경된 스택만 실행하도록 설정 권장 (`/new workflow ci` 실행 시 자동 반영).
- **PR 스코프** — 가능하면 한 PR 에서는 한 스택만 변경. 불가피하게 걸치면 커밋 분리.

각 스택별 아키텍처 규칙·코딩 컨벤션·금지사항은 해당 디렉토리 `CLAUDE.md` 를 **반드시** 읽으세요.
