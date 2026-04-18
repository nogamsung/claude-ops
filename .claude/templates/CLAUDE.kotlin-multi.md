# [프로젝트명] — Kotlin Spring Boot (Gradle 멀티 모듈)

## Stack
- **Language**: Kotlin (latest stable)
- **Framework**: Spring Boot 3.x
- **Build**: Gradle (Kotlin DSL — `build.gradle.kts`) — **멀티 모듈 구조**
- **ORM**: Spring Data JPA + Hibernate + **QueryDSL** (필수 조합)
- **동적 쿼리**: QueryDSL (모든 동적 쿼리에 사용) — 복잡한 집계/보고서 쿼리는 jOOQ 추가 가능
- **Migration**: Flyway (절대 기존 migration 파일 수정 금지)
- **Security**: Spring Security + JWT
- **Docs**: SpringDoc OpenAPI (`springdoc-openapi-starter-webmvc-ui`) — Swagger UI `/swagger-ui.html`

## Agents
| 작업 | Agent |
|------|-------|
| 새 파일 생성 | `kotlin-generator` |
| 기존 코드 수정 | `kotlin-modifier` |
| 테스트 작성 | `kotlin-tester` |
| 코드 리뷰 | `code-reviewer` |

## Commands
| 커맨드 | 용도 |
|--------|------|
| `/planner <기능>` | 기획서(PRD) + 구현 프롬프트 작성 (단일 스택은 단일 프롬프트 산출) |
| `/plan <기능>` | 코드 작성 전 설계 및 확인 |
| `/plan api <Resource>` | REST API 설계 → OpenAPI 3.0 YAML |
| `/plan db <도메인>` | MySQL 스키마 설계 → Flyway migration 자동 생성 |
| `/new <Resource>` | REST API 전체 스캐폴딩 (멀티 모듈 경로 자동 적용, 명시: `/new api`) |
| `/new module <ModuleName>` | 새 Gradle 서브모듈 생성 |
| `/test [파일]` | 테스트 자동 생성 |
| `/review [staged\|diff\|파일]` | 코드 리뷰 |
| `/rule <실수 설명>` | 새 규칙을 이 파일에 추가 |
| `/commit [힌트]` | Conventional Commits 커밋 |
| `/pr` | PR 생성 + /merge 자동 제안 |
| `/merge [auto]` | GitHub 머지 실행 + 태그 + worktree 정리 |
| `/memory [add\|search]` | Second Brain 조회·추가·검색 |

## 플러그인 커맨드 (설치된 플러그인)
| 커맨드 | 플러그인 | 설명 |
|--------|---------|------|
| `/feature-dev <기능>` | feature-dev | 7단계 체계적 기능 개발 (탐색→설계→구현→리뷰) |
| `/code-review` | code-review | 현재 PR에 병렬 4-agent 자동 리뷰 + CLAUDE.md 준수 검사 |
| `/commit-push-pr` | commit-commands | 커밋→푸시→PR 생성 한 번에 |
| `/clean_gone` | commit-commands | 삭제된 원격 브랜치의 로컬 정리 |
| `/hookify [설명]` | hookify | 반복 실수를 자동 방지 훅으로 등록 |
| (자동) | kotlin-lsp | 타입 오류·심볼 참조·리팩토링 실시간 지원 |
| (자동) | context7 | Spring Boot·Kotlin 최신 공식 문서를 컨텍스트로 주입 |
| (자동) | security-guidance | 위험 명령어 실행 전 보안 경고 |
| (자동) | claude-md-management | CLAUDE.md 규칙 자동 관리 |

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
- 각 worktree는 독립된 의존성·빌드 캐시 보유
- `main` 직접 push 금지 — 반드시 `dev`를 거쳐 PR

---

## 아키텍처 규칙

### 멀티 모듈 디렉토리 구조
```
settings.gradle.kts              ← include(":api", ":domain", ":infra")
build.gradle.kts                 ← 루트 (공통 dependencies, plugins)
api/
  build.gradle.kts               ← depends on :domain, :infra
  src/main/kotlin/com/{company}/{project}/
    presentation/                ← Controller, DTO, Request/Response
    config/                      ← Spring 설정 클래스
  src/test/kotlin/
domain/
  build.gradle.kts               ← 외부 의존성 없음 (순수 도메인)
  src/main/kotlin/com/{company}/{project}/
    domain/                      ← Entity, Value Object
    application/                 ← Service (비즈니스 로직)
  src/test/kotlin/
infra/
  build.gradle.kts               ← depends on :domain
  src/main/kotlin/com/{company}/{project}/
    infrastructure/              ← Repository 구현체, 외부 연동
  src/test/kotlin/
```

### 모듈 의존 방향
`api` → `domain` ← `infra`

**절대 역방향 의존 금지.** `:domain`이 `:api` 또는 `:infra`를 import하면 안 됩니다.
**`:api`는 `:infra`를 직접 import하지 않습니다** — `:domain`의 인터페이스를 통해 간접 사용합니다.

---

## 반드시 지켜야 할 규칙 (MUST)

### 의존성 주입
```kotlin
// ✅ 생성자 주입만 허용
@Service
class UserService(private val userRepository: UserRepository)

// ❌ 절대 금지 — 필드 주입
@Autowired
lateinit var userRepository: UserRepository
```

### 트랜잭션
```kotlin
// ✅ 서비스 클래스에 readOnly = true
@Service
@Transactional(readOnly = true)
class UserService(...) {

    // ✅ 쓰기 메서드에만 @Transactional 추가
    @Transactional
    fun createUser(request: CreateUserRequest): UserResponse { ... }
}
```

### DTO 사용
```kotlin
// ✅ 반드시 DTO로 감싸서 반환
fun getUser(id: Long): UserResponse = UserResponse.from(findUser(id))

// ❌ 절대 금지 — Entity를 API 응답으로 직접 노출
fun getUser(id: Long): User = findUser(id)
```

### Kotlin Null Safety
```kotlin
// ✅ 안전한 처리
val name = user?.name ?: throw IllegalStateException("name is null")

// ❌ 절대 금지 — 확신 없이 !! 사용
val name = user!!.name
```

### 예외 처리
```kotlin
// ✅ 도메인 예외를 던지고 GlobalExceptionHandler에서 처리
throw EntityNotFoundException("User not found: $id")

// ❌ 절대 금지 — Controller에서 try-catch로 예외 삼키기
```

---

## 절대 하면 안 되는 것 (NEVER)

- `DROP TABLE`, `TRUNCATE` 등 raw DDL SQL 실행
- 기존 Flyway migration 파일 수정 (새 파일 추가만 가능)
- Entity 클래스에 비즈니스 로직 추가 (getter/setter 외)
- 패스워드, 토큰, PII를 로그에 출력
- `@SpringBootApplication` 클래스에 비즈니스 코드 추가
- 테스트 없이 새로운 public 메서드 추가
- N+1 쿼리를 유발하는 즉시 로딩(`FetchType.EAGER`) 추가
- SpringDoc 어노테이션 없이 새 Controller 엔드포인트 추가
- QueryDSL 없이 `@Query` JPQL 또는 Native Query로 동적 쿼리 작성
- 동적 조건이 있는 쿼리를 `JpaRepository` 메서드 이름 방식으로 억지 처리

### 모듈 간 의존 규칙 (멀티 모듈 전용)
- **`:api` 모듈이 `:infra`를 직접 import 금지** — `:domain` 인터페이스를 통해 간접 사용
- **`:domain` 모듈이 `:api` 또는 `:infra` import 금지** — 순수 도메인 레이어 유지

---

## SpringDoc OpenAPI 규칙 (MUST)

> SpringDoc은 `:api` 모듈에만 적용됩니다.

### 의존성 (api/build.gradle.kts)
```kotlin
dependencies {
    implementation("org.springdoc:springdoc-openapi-starter-webmvc-ui:2.6.0")
}
```

### application.yml
```yaml
springdoc:
  api-docs:
    path: /api-docs
  swagger-ui:
    path: /swagger-ui.html
    operations-sorter: method
  default-consumes-media-type: application/json
  default-produces-media-type: application/json
```

### Controller 어노테이션 (필수)
```kotlin
@Tag(name = "User", description = "사용자 관리 API")
@RestController
@RequestMapping("/api/v1/users")
class UserController(private val userService: UserService) {

    @Operation(summary = "사용자 단건 조회", description = "ID로 사용자를 조회합니다.")
    @ApiResponses(value = [
        ApiResponse(responseCode = "200", description = "조회 성공",
            content = [Content(schema = Schema(implementation = UserResponse::class))]),
        ApiResponse(responseCode = "404", description = "사용자 없음",
            content = [Content(schema = Schema(implementation = ErrorResponse::class))]),
    ])
    @GetMapping("/{id}")
    fun getUser(
        @Parameter(description = "사용자 ID", required = true) @PathVariable id: Long,
    ): ResponseEntity<UserResponse> = ResponseEntity.ok(userService.getUser(id))

    @Operation(summary = "사용자 생성")
    @ApiResponse(responseCode = "201", description = "생성 성공")
    @PostMapping
    fun createUser(
        @RequestBody @Valid request: CreateUserRequest,
    ): ResponseEntity<UserResponse> =
        ResponseEntity.status(HttpStatus.CREATED).body(userService.createUser(request))
}
```

### Request/Response DTO 어노테이션
```kotlin
@Schema(description = "사용자 생성 요청")
data class CreateUserRequest(
    @field:NotBlank
    @Schema(description = "이메일", example = "user@example.com", required = true)
    val email: String,

    @field:NotBlank
    @Schema(description = "이름", example = "홍길동", required = true)
    val name: String,
)

@Schema(description = "사용자 응답")
data class UserResponse(
    @Schema(description = "사용자 ID", example = "1")
    val id: Long,
    @Schema(description = "이메일", example = "user@example.com")
    val email: String,
    @Schema(description = "이름", example = "홍길동")
    val name: String,
)
```

### OpenAPI 전역 설정 (api/config/)
```kotlin
@Configuration
class SwaggerConfig {
    @Bean
    fun openAPI(): OpenAPI = OpenAPI()
        .info(Info()
            .title("[프로젝트명] API")
            .description("API 명세서")
            .version("v1.0.0")
            .contact(Contact().name("Team").email("team@example.com")))
        .components(Components()
            .addSecuritySchemes("bearerAuth",
                SecurityScheme()
                    .type(SecurityScheme.Type.HTTP)
                    .scheme("bearer")
                    .bearerFormat("JWT")))
        .addSecurityItem(SecurityRequirement().addList("bearerAuth"))
}
```

---

## QueryDSL 사용 규칙 (MUST)

> QueryDSL 구현체는 `:infra` 모듈에, 인터페이스는 `:domain` 모듈에 위치합니다.

### 의존성 (infra/build.gradle.kts)
```kotlin
val queryDslVersion = "5.1.0"

dependencies {
    implementation(project(":domain"))
    implementation("com.querydsl:querydsl-jpa:$queryDslVersion:jakarta")
    kapt("com.querydsl:querydsl-apt:$queryDslVersion:jakarta")
    // 복잡한 집계·보고서 쿼리가 필요한 경우 추가
    // implementation("org.jooq:jooq")
}
```

### Repository 구조
```kotlin
// ✅ domain 모듈 — 인터페이스 정의
interface UserRepository : JpaRepository<User, Long>, UserRepositoryCustom

interface UserRepositoryCustom {
    fun findByCondition(condition: UserSearchCondition): List<User>
}

// ✅ infra 모듈 — QueryDSL 구현체
class UserRepositoryImpl(private val queryFactory: JPAQueryFactory) : UserRepositoryCustom {
    override fun findByCondition(condition: UserSearchCondition): List<User> {
        val user = QUser.user
        return queryFactory.selectFrom(user)
            .where(
                condition.name?.let { user.name.contains(it) },
                condition.status?.let { user.status.eq(it) }
            )
            .fetch()
    }
}
```

### 쿼리 선택 기준
| 케이스 | 사용 기술 |
|--------|----------|
| 단순 CRUD (findById, save 등) | Spring Data JPA |
| 동적 조건 검색, 페이징 | QueryDSL |
| 복잡한 집계, 통계, 보고서 | QueryDSL 우선, 필요 시 jOOQ |
| 정적 JPQL (조건 없음) | `@Query` 허용 |

---

## 코드 품질 기준

- 모든 public 서비스 메서드에 단위 테스트 필수
- 새 엔드포인트마다 Controller 테스트 필수
- 새 DB 컬럼/테이블은 반드시 Flyway migration과 함께
- PR당 하나의 기능 단위로 atomic하게 커밋

## 커버리지 게이트

**git push 전 Jacoco 라인 커버리지 90% 이상 필수** (`.claude/hooks/pre-push.sh` 자동 검사)

커버리지 미달 시 동작:
1. pre-push 훅이 푸시를 차단하고 미달 수치를 출력합니다
2. 커버리지가 낮은 파일에 `/test <파일>` 커맨드로 테스트를 추가합니다
3. `./gradlew test jacocoTestReport`로 커버리지를 재확인합니다 (전체 모듈 포함)
4. 90% 이상이 되면 다시 `git push`를 시도합니다

**Jacoco 설정 (루트 build.gradle.kts에 반드시 포함):**
```kotlin
subprojects {
    apply(plugin = "jacoco")

    tasks.withType<JacocoReport> {
        dependsOn(tasks.withType<Test>())
        reports {
            xml.required = true
            html.required = true
        }
    }
}
```

### 테스트 실행
```bash
# 전체 테스트 + 커버리지 (모든 모듈)
./gradlew test jacocoTestReport

# 특정 모듈만
./gradlew :domain:test
./gradlew :infra:test
./gradlew :api:test

# 빌드
./gradlew :api:bootJar
```

---

## 학습된 규칙 (AI 실수 후 추가)

<!-- /rule 커맨드로 새 규칙이 여기에 추가됩니다 -->

---

## 세션 시작 시 자동 참조

> 🧠 **새 작업을 시작하기 전에 `memory/MEMORY.md` 를 반드시 먼저 읽으세요.** 과거 결정·교훈을 맥락에 포함하여 같은 실수를 반복하지 않도록 합니다.

---

## Memory 관리 지침

> Claude는 아래 상황에서 `memory/MEMORY.md`를 **자동으로** 업데이트합니다.
> 사용자가 요청하지 않아도 기록하고, 기록 후 "memory에 저장했습니다." 한 줄만 언급합니다.

**자동 기록 트리거:**
- `/plan` 승인 → 구현할 기능과 선택한 설계 방식 기록
- `/rule` 실행 → 어떤 실수였는지, 추가된 규칙 요약 기록
- 복잡한 버그 해결 → 원인, 해결 방법, 재발 방지 포인트 기록
- 외부 라이브러리/API 도입 결정 → 선택 이유, 대안 기록
- 아키텍처 또는 폴더 구조 변경 → 변경 전/후, 이유 기록
- 성능 문제 발견 및 해결 → 병목 지점, 해결 방법 기록

**`memory/MEMORY.md` vs `CLAUDE.md` 구분:**
- `memory/MEMORY.md` — 맥락과 히스토리 (왜 이 결정을 했는가)
- `CLAUDE.md` — 규칙 (앞으로 어떻게 해야 하는가)
