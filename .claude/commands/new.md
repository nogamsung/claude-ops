---
description: 리소스/파일/작업공간 생성 — 이름과 스택으로 자동 감지. 서브명령(api/component/screen/module/workflow/worktree)은 override용.
argument-hint: <Name> (자동 감지) 또는 <sub> <Name> [옵션] (명시 지정)
---

대상 종류를 **자동 감지**하여 생성 작업을 수행합니다. 감지가 애매하면 사용자에게 확인 후 진행.

**인수:** $ARGUMENTS

---

## 서브명령 결정 (역할 prefix → 자동 감지 → override 순서)

### Step 0 — 모노레포 역할 prefix 체크

`.claude/stacks.json` 이 존재하고 `mode: "monorepo"` 이면, 첫 토큰이 역할명이면 **그 역할 디렉토리로 cd 해서 실행**합니다.

| 첫 토큰 | 의미 |
|---------|------|
| `backend` | `.claude/stacks.json` 에서 `role: "backend"` 의 `path` 로 이동 |
| `frontend` | `role: "frontend"` 의 `path` 로 이동 |
| `mobile` | `role: "mobile"` 의 `path` 로 이동 |

실행 방식:

```bash
STACK_PATH=$(jq -r '.stacks[] | select(.role == "backend") | .path' .claude/stacks.json)
cd "$STACK_PATH"   # 이후 작업은 이 디렉토리를 루트로 간주
```

prefix 를 소비하고 나머지 인자로 Step 1 부터 진행합니다 (예: `/new backend api User` → `api User` 로 재평가).

prefix 없이 호출되고 모노레포이면:
- 활성 스택이 **1개면** 자동 선택 (예: backend 만 있음 → 자동으로 backend 경로)
- **2개 이상**이면 사용자에게 확인:
  > "어느 스택에서 실행할까요? (backend / frontend / mobile)"

단일 스택 모드(`.claude/stacks.json` 없음)에서는 Step 0 을 건너뜁니다.

### Step 1 — 명시 서브명령 우선 체크

첫 토큰이 아래 목록에 있으면 그 서브명령을 그대로 사용 (override):

| 명시 토큰 | 서브 |
|----------|------|
| `api` | REST API 스캐폴딩 |
| `component` | Next.js React 컴포넌트 |
| `screen` | Flutter 화면 |
| `module` | 멀티 모듈 서브모듈 |
| `workflow` | GitHub Actions |
| `worktree` | git worktree |

### Step 2 — 인수 패턴 기반 자동 감지

명시 토큰이 아니면 아래 순서로 패턴 매칭:

| 패턴 | 감지 서브 | 예시 |
|------|---------|------|
| 첫 토큰이 `feature-`·`fix-`·`hotfix-`·`refactor-`·`chore-`·`docs-`·`test-`·`perf-` 중 하나로 시작 | **worktree** | `/new feature-login` |
| 첫 토큰이 `ci`·`release`·`publish`·`scheduled` 중 하나 | **workflow** | `/new publish kotlin` |
| 위 둘 다 아니면 | → Step 3 (스택 기반) | |

### Step 3 — 스택 기반 기본값

프로젝트 루트 파일로 스택을 감지해 기본 서브를 결정:

| 감지 파일 | 기본 서브 | 이름 해석 |
|-----------|----------|----------|
| `go.mod` 또는 `build.gradle.kts` / `pom.xml` | **api** | PascalCase → 리소스명 |
| `package.json` (`next` 의존성) | **component** | PascalCase → 컴포넌트명 |
| `pubspec.yaml` | **screen** | PascalCase → 화면명 |

### Step 4 — 멀티 모듈 모호성 해소

프로젝트가 멀티 모듈(`settings.gradle.kts include(` / `go.work` / `turbo.json`)이고, 이름이 **소문자 단일 단어**(예: `notification`, `payment`)이면 사용자에게 확인:

> "`<name>` 을 멀티 모듈 서브모듈로 생성할까요? 아니면 API 리소스로 생성할까요? (module / api)"

### Step 5 — 감지 결과 확인 (첫 토큰이 명시 서브가 아닐 때만)

자동 감지된 결과를 사용자에게 한 줄로 보여주고 곧바로 진행:

```
→ Go 프로젝트 감지 · /new api User 로 해석합니다.
```

사용자가 다른 것을 원하면 명시 서브명령으로 다시 호출하도록 안내.

---

## 사용 예시

```
# 단일 스택 — 자동 감지 (권장)
/new User                     # Kotlin/Go → api / Next.js → component / Flutter → screen
/new UserCard --feature       # Next.js → component (--feature 플래그)
/new feature-login            # worktree (type prefix 인식)
/new fix-signup               # worktree
/new publish kotlin           # workflow (publish 키워드 인식)
/new ci nextjs                # workflow

# 단일 스택 — 명시 지정 (override)
/new api User                 # 자동 감지가 틀릴 때 강제 지정
/new module notification      # 멀티 모듈에서 소문자 이름을 모듈로 강제
/new workflow ci              # workflow 명시

# 모노레포 — 역할 prefix 로 대상 스택 명시
/new backend api User            # backend 경로에서 api 스캐폴딩
/new frontend component Button   # frontend 경로에서 컴포넌트
/new mobile screen Login         # mobile 경로에서 Flutter 화면
/new backend module notification # backend 멀티 모듈 서브모듈 추가
/new backend User                # 역할 명시 후 자동 감지 (backend → api)

# 모노레포 공용 (역할 불필요)
/new feature-login               # worktree — 루트에서 실행
/new workflow ci                 # workflow — 루트에서 실행
```

---

## `api` — REST API 스캐폴딩

> 💡 DB 스키마부터 설계하려면 `/plan db <도메인>` 을 먼저 실행하세요. Migration SQL을 먼저 만들고 이 커맨드로 Entity/Repository 코드를 생성하면 일관성이 보장됩니다.

**리소스명**: 두 번째 토큰 (없으면 사용자에게 물어보세요)

### 스택 자동 감지

| 감지 파일 | 스택 | 섹션 |
|-----------|------|------|
| `go.mod` | Go Gin | [Go Gin](#api--go-gin) |
| `settings.gradle.kts` / `build.gradle.kts` / `build.gradle` | Spring Boot | [Spring Boot](#api--spring-boot) |
| 위 파일 없음 | — | 사용자에게 스택 선택 요청 |

### api — Go Gin

**프로젝트 구조 감지**: `go.work` 존재 시 멀티 서비스 → 어느 서비스에 추가할지 물어봅니다. 선택 디렉토리를 루트로 삼아 단일 서비스 구조 그대로 적용. 공유 도메인은 `pkg/shared/` 배치.

**생성할 파일**:

1. **Domain Entity + Errors**
   - `internal/domain/{resource}.go` — Entity struct
   - `internal/domain/errors.go` — 없으면 생성

2. **Repository Interface**
   - `internal/domain/{resource}_repository.go` — CRUD 인터페이스

3. **Repository Implementation (GORM + sqlc)**
   - `internal/repository/{resource}_repository.go` — 단순 CRUD는 GORM, 조건 검색·페이징은 sqlc `*sqlcdb.Queries`
   - `db/query/{resource}.sql` — sqlc 쿼리

4. **UseCase**
   - `internal/usecase/{resource}_usecase.go` — 비즈니스 로직 + Request/SearchParams DTO

5. **Handler + Response DTO** (swag 주석 필수)
   - `internal/handler/{resource}_handler.go` — `@Summary`, `@Tags`, `@Router`, `@Success`, `@Failure` godoc, `RegisterRoutes` 포함
   - `internal/handler/{resource}_response.go` — `example:"..."` json 태그, 없으면 `ErrorResponse` 생성

6. **Migration**
   - `migrations/{nextNum:06d}_create_{resources}_table.up.sql`
   - `migrations/{nextNum:06d}_create_{resources}_table.down.sql`

7. **테스트**
   - `internal/usecase/{resource}_usecase_test.go` — mockery
   - `internal/handler/{resource}_handler_test.go` — httptest
   - `testutil/{resource}_fixture.go` — 없으면 생성

**주의사항 (Go Gin)**:
- `domain/` 패키지는 외부 import 금지 — 순수 Go 인터페이스만
- 기존 에러 응답 형식·middleware 방식 먼저 파악하고 따르기
- `cmd/main.go`의 DI 조립에 신규 의존성 연결 방법 안내
- 조건 검색·페이징은 sqlc — raw SQL 문자열 금지
- Handler 메서드 swag godoc 필수
- 생성 후 안내: `mockery --name={Resource}Repository --dir=internal/domain --output=mocks` / `sqlc generate` / `swag init -g cmd/main.go -o docs`

### api — Spring Boot

**프로젝트 구조 감지**: `settings.gradle.kts`에 `include(`가 있으면 멀티 모듈.

| 파일 | 단일 모듈 | 멀티 모듈 |
|------|----------|-----------|
| Entity, VO | `domain/` | `:domain` → `domain/src/main/kotlin/.../domain/` |
| Service | `application/` | `:domain` → `domain/src/main/kotlin/.../application/` |
| Repository (interface) | `domain/` | `:domain` → `domain/src/main/kotlin/.../domain/` |
| Repository (impl) | `infrastructure/` | `:infra` → `infra/src/main/kotlin/.../infrastructure/` |
| Controller, DTO | `presentation/` | `:api` → `api/src/main/kotlin/.../presentation/` |
| 테스트 | 각 모듈 `src/test/kotlin/` | 동일 |

**생성할 파일**:

1. **Domain Entity** — `domain/{Resource}.kt`, JPA `@Entity`, `@CreationTimestamp`/`@UpdateTimestamp`
2. **Repository (QueryDSL 3세트 필수)**
   - `infrastructure/{Resource}Repository.kt` — `JpaRepository` + Custom 상속
   - `infrastructure/{Resource}RepositoryCustom.kt` — 동적 쿼리 인터페이스
   - `infrastructure/{Resource}RepositoryImpl.kt` — `JPAQueryFactory` 구현체
   - `infrastructure/{Resource}SearchCondition.kt` — 검색 조건 DTO
3. **DTOs (Schema 필수)**
   - `presentation/dto/{Resource}Response.kt` — `@Schema` + companion `from()`
   - `presentation/dto/Create{Resource}Request.kt` — `@Schema` + Bean Validation
   - `presentation/dto/Update{Resource}Request.kt` — `@Schema` + Bean Validation
4. **Service** — `application/{Resource}Service.kt`, `@Service`, `@Transactional(readOnly = true)`, CRUD 메서드, `EntityNotFoundException`
5. **Controller (SpringDoc 필수)** — `presentation/{Resource}Controller.kt`, `@Tag`, `@RestController`, `@RequestMapping("/api/v1/{resources}")`, `@Operation`+`@ApiResponse`, `@Parameter`, `ResponseEntity`+`@Valid`
6. **테스트**
   - `test/.../application/{Resource}ServiceTest.kt` — MockK
   - `test/.../presentation/{Resource}ControllerTest.kt` — `@WebMvcTest`

**주의사항 (Spring Boot)**: 기존 패턴(패키지, 예외, 응답 형태) 먼저 파악. `GlobalExceptionHandler` 있으면 맞춰 예외 던지기. Kotlin 관용 표현 사용. QueryDSL 3세트, SpringDoc 어노테이션, `@Schema` 필수.

---

## `component` — Next.js 컴포넌트

**인수 파싱**: `<Name> [--page | --feature | --ui]` (기본 `--feature`)
- `--page`: `app/` 페이지 컴포넌트
- `--feature`: `components/features/` 피처 컴포넌트 (기본)
- `--ui`: `components/ui/` 재사용 UI 컴포넌트

**파일 구조**:

```
# --ui
components/ui/{component-name}/
├── {component-name}.tsx
├── {component-name}.test.tsx
└── index.ts

# --feature
components/features/{feature}/
├── {component-name}.tsx
├── use-{component-name}.ts     # 필요시
└── {component-name}.test.tsx

# --page
app/{route}/
├── page.tsx                    # Server Component
├── loading.tsx
└── error.tsx
```

**생성 규칙**: Props 타입 명시 / Named export (default 금지) / Server Component 기본, 인터랙티비티 시에만 `"use client"` / Tailwind / 시맨틱 HTML + aria / 로딩·에러 상태.

**선택 규칙**: API 연동 → TanStack Query / 폼 → React Hook Form + Zod / 전역 상태 → Zustand.

생성 후 파일 목록과 사용법 예시를 보여주세요.

---

## `screen` — Flutter 화면

**인수**: `<ScreenName>` (없으면 사용자에게 물어보세요)

현재 프로젝트 구조(Riverpod vs Bloc)를 파악한 후 생성.

**Feature 구조 (Clean Architecture)**:

```
lib/features/{feature_name}/
├── data/
│   ├── datasources/{feature_name}_remote_data_source.dart
│   └── repositories/{feature_name}_repository_impl.dart
├── domain/
│   ├── entities/{entity_name}.dart           # freezed
│   ├── repositories/{feature_name}_repository.dart
│   └── usecases/get_{feature_name}.dart
└── presentation/
    ├── screens/{screen_name}_screen.dart
    ├── widgets/
    └── providers/{screen_name}_provider.dart
```

**파일 규칙**:
- **Screen**: `ConsumerStatefulWidget`/`ConsumerWidget`, `Scaffold`, 로딩/에러/빈 상태 처리
- **Provider (Riverpod)**: `@riverpod`, `AsyncNotifier`/`Notifier`, `Either<Failure, T>`
- **Entity**: `@freezed`, `fromJson`/`toJson` (`json_serializable`)
- **GoRouter 등록**: 현재 `app_router.dart`에 새 라우트 등록, path constant 별도 정의

**포함 판단**: API 연동→Dio RemoteDataSource / 로컬 저장→Hive·SharedPreferences / 폼→`TextEditingController`+validation / 목록→`ListView.builder` pagination.

**주의사항**: `const` 생성자 최대 활용, `dispose()`에서 Controller 정리, `.g.dart`/`.freezed.dart`는 직접 생성 금지 (`build_runner` 안내), 생성 후 `flutter pub run build_runner build` 안내.

---

## `module` — 멀티 모듈 서브모듈 추가

**인수**: `<moduleName>` (없으면 사용자에게 물어보세요)

### 타입 감지

| 감지 조건 | 타입 |
|----------|------|
| `go.work` | Go Workspace |
| `turbo.json` | Next.js Turborepo |
| `settings.gradle.kts`에 `include(` | Kotlin 멀티 모듈 |

감지된 타입을 사용자에게 확인받습니다.

### module — Kotlin 멀티 모듈

1. **디렉토리 구조**: `{moduleName}/build.gradle.kts`, `src/main/kotlin/com/{company}/{project}/`, `src/main/resources/`, `src/test/kotlin/...` — 패키지는 기존 모듈 참고
2. **build.gradle.kts**: 용도별 의존성 확인 (순수 로직→`:domain`, 외부 연동→`:domain`+외부 lib, API→`:domain`+`:infra`)
3. **settings.gradle.kts 업데이트**: `include(":api", ":domain", ":infra", ":{moduleName}")`
4. **완료 안내**: 파일 목록, 사용 시 `implementation(project(":{moduleName}"))` 추가, `./gradlew :{moduleName}:build` 빌드 확인

### module — Next.js (Turborepo)

사용자에게 묻습니다: `apps/` (독립 배포 앱) vs `packages/` (공유 라이브러리)

**apps/** 선택: `apps/{moduleName}/{package.json, tsconfig.json, src/{app,components,hooks,lib,stores,types}/}`

**packages/** 선택: `packages/{moduleName}/{package.json, tsconfig.json, src/index.ts}`

**package.json**:
```json
{
  "name": "@project/{moduleName}",
  "version": "0.0.1",
  "exports": { ".": "./src/index.ts" },
  "scripts": { "lint": "eslint src/", "test": "jest", "build": "tsc" },
  "devDependencies": { "@project/config": "*" }
}
```

루트 `package.json`의 `workspaces`가 `"apps/*"`, `"packages/*"`로 등록되어 있으면 추가 작업 불필요. 사용 시 `"@project/{moduleName}": "*"` + `npm install`, 빌드 확인은 `turbo run build --filter=@project/{moduleName}`.

### module — Go Workspace

1. **디렉토리 구조**: `services/{moduleName}/{go.mod, cmd/main.go, internal/{domain,usecase,repository,handler,middleware}/, migrations/, db/{query,sqlc}/, mocks/, testutil/}`
2. **go.mod**: 기존 서비스 모듈 경로 패턴 확인 — `module github.com/{org}/{project}/services/{moduleName}`, `require github.com/{org}/{project}/pkg/shared v0.0.0`
3. **cmd/main.go**: 기본 골격 (log + DI 조립 자리)
4. **go.work 업데이트**: `use` 디렉티브에 `./services/{moduleName}` 추가
5. **`go work sync`** 실행
6. **완료 안내**: 파일 목록, 공유 도메인은 `pkg/shared/`, `cd services/{moduleName} && go build ./...` 빌드, `golangci-lint run ./...`는 각 서비스 디렉토리에서 실행 (workspace root 미지원)

---

## `workflow` — GitHub Actions

**인수**: `[ci | release | publish | scheduled] [스택명]` (예: `ci nextjs`, `publish kotlin`)

### Step 1 — 컨텍스트 파악
- `CLAUDE.md`, `package.json` / `build.gradle.kts` / `go.mod` / `pubspec.yaml` → 스택 특정
- `.github/workflows/` 기존 파일 → 중복·충돌 방지

### Step 2 — 패턴 참고
- `.claude/skills/github-actions-patterns.md` 읽기
- 스택 + 목적에 맞는 템플릿 선택

### Step 3 — 생성

| 목적 | 파일명 |
|------|--------|
| CI (빌드·테스트·린트) | `.github/workflows/ci.yml` |
| 릴리스·버전 태깅 | `.github/workflows/release.yml` |
| Docker 이미지 배포 (GHCR) | `.github/workflows/publish.yml` |
| 정기 실행 | `.github/workflows/scheduled.yml` |

**publish 추가 동작**: `Dockerfile` 없으면 스택별 멀티스테이지 Dockerfile 먼저 생성 (Kotlin: JDK builder→JRE runtime / Go: golang builder→alpine 정적 바이너리 / Next.js: deps→builder→runner, `next.config.js`에 `output: 'standalone'` 안내). Flutter는 Docker 배포 미지원 — 안내 후 종료.

### Step 4 — 완료 출력
1. 필요한 **GitHub Secrets** 목록
2. 필요한 **GitHub Environments** 목록 (있으면)
3. 외부 설정 안내 (npm token, OIDC role 등)

**주의사항**: `secrets.*` 하드코딩 금지 / `uses:` action 버전 고정 (`@v4` 이상) / 스택별 캐시 설정 필수 / 릴리스는 `push: tags: ['v*.*.*']` 트리거.

---

## `worktree` — git worktree 생성

**인수**: `<타입-이름>` (예: `feature-login`, `fix-signup`)

> PR 생성은 별도 커맨드 `/pr` 을 사용하세요.

> ⚠️ **브랜치 네이밍 제약**
> Git은 `dev`와 `dev/feature-*` 를 동시에 유지할 수 없습니다 (refs 충돌).
> 피처 브랜치는 `dev/` 대신 `feature/`, `fix/` 등 독립 prefix 사용.
> - 통합 브랜치: `dev`
> - 피처 브랜치: `feature/{name}`, `fix/{name}`, `hotfix/{name}` 등

### Step 1 — .worktrees/ 안전 확인

```bash
git check-ignore -q .worktrees 2>/dev/null
```

등록 안 되어 있으면:
```bash
echo ".worktrees/" >> .gitignore
git add .gitignore
git commit -m "chore: .worktrees/ gitignore 추가"
```

### Step 2 — 베이스 브랜치 결정 & 최신화

```bash
if git branch --list dev | grep -q dev; then
  BASE_BRANCH="dev"
else
  BASE_BRANCH="main"
fi
git checkout $BASE_BRANCH
git pull origin $BASE_BRANCH
```

### Step 3 — 브랜치명 결정

형식: `{타입}-{이름}` (kebab-case, 이름 1~2단어)

| 타입 | 의미 | 예시 |
|------|------|------|
| `feature` | 새 기능 | `feature-login` |
| `fix` | 버그 수정 | `fix-signup` |
| `hotfix` | 긴급 프로덕션 수정 | `hotfix-payment` |
| `refactor` | 리팩토링 | `refactor-auth` |
| `chore` | 설정·의존성·잡무 | `chore-deps` |
| `docs` | 문서 | `docs-api` |
| `test` | 테스트 추가/수정 | `test-user` |
| `perf` | 성능 개선 | `perf-query` |

타입 없이 이름만 (예: `login`) → `feature-login`으로 자동 처리.

### Step 4 — Worktree 생성

```bash
git worktree add .worktrees/{type}-{name} -b {type}/{name}
```

### Step 5 — 스택별 의존성 설치

```bash
cd .worktrees/{type}-{name}
[ -f go.mod ] && go mod download
[ -f package.json ] && npm ci
[ -f gradlew ] && ./gradlew dependencies --no-daemon -q
[ -f pubspec.yaml ] && flutter pub get
```

### Step 6 — 작업 안내

```
Worktree 준비 완료

브랜치:  {type}/{name}
경로:    .worktrees/{type}-{name}/
베이스:  {BASE_BRANCH}

병렬 작업:
  cd .worktrees/{type}-{name}   # 다른 터미널
  claude --dir .worktrees/{type}-{name}

작업 완료 후:
  /pr                  → PR 생성 (base: {BASE_BRANCH} 자동)
  /merge          → 머지 후 정리 (태그 + worktree 제거)
```

### Worktree 현황

```bash
git worktree list
```
