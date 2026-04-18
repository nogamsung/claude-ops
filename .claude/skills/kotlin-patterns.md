# Kotlin Spring Boot Code Patterns

## Generation Patterns

### Entity
```kotlin
@Entity
@Table(name = "orders")
class Order(
    @ManyToOne(fetch = FetchType.LAZY)
    @JoinColumn(name = "user_id", nullable = false)
    val user: User,

    @Column(nullable = false)
    var status: OrderStatus = OrderStatus.PENDING,

    @Id @GeneratedValue(strategy = GenerationType.IDENTITY)
    val id: Long = 0,

    @CreationTimestamp val createdAt: LocalDateTime = LocalDateTime.now(),
    @UpdateTimestamp var updatedAt: LocalDateTime = LocalDateTime.now(),
)
```

### Repository
```kotlin
// 기본 CRUD + 정적 쿼리
interface OrderRepository : JpaRepository<Order, Long>, OrderRepositoryCustom {
    fun findByIdAndUserId(id: Long, userId: Long): Order?
}

// 동적 쿼리 — QueryDSL (반드시 사용)
interface OrderRepositoryCustom {
    fun findByCondition(condition: OrderSearchCondition, pageable: Pageable): Page<Order>
}

class OrderRepositoryImpl(
    private val queryFactory: JPAQueryFactory,
) : OrderRepositoryCustom {
    private val order = QOrder.order

    override fun findByCondition(condition: OrderSearchCondition, pageable: Pageable): Page<Order> {
        val content = queryFactory.selectFrom(order)
            .where(
                condition.userId?.let { order.user.id.eq(it) },
                condition.status?.let { order.status.eq(it) },
            )
            .offset(pageable.offset)
            .limit(pageable.pageSize.toLong())
            .orderBy(order.createdAt.desc())
            .fetch()

        val total = queryFactory.select(order.count())
            .from(order)
            .where(
                condition.userId?.let { order.user.id.eq(it) },
                condition.status?.let { order.status.eq(it) },
            )
            .fetchOne() ?: 0L

        return PageImpl(content, pageable, total)
    }
}

// Search Condition DTO
data class OrderSearchCondition(
    val userId: Long? = null,
    val status: OrderStatus? = null,
)
```

> **QueryDSL 선택 기준**
> - 동적 조건 (nullable 파라미터) → `QueryDSL` 필수
> - 정적 단순 조건 → `JpaRepository` 메서드명 허용
> - 복잡한 집계·통계 → QueryDSL 우선, 필요 시 jOOQ 추가

### Service
```kotlin
@Service
@Transactional(readOnly = true)
class OrderService(
    private val orderRepository: OrderRepository,
    private val userRepository: UserRepository,
) {
    fun getOrder(id: Long): OrderResponse {
        val order = orderRepository.findById(id)
            .orElseThrow { EntityNotFoundException("Order not found: $id") }
        return OrderResponse.from(order)
    }

    @Transactional
    fun createOrder(userId: Long, request: CreateOrderRequest): OrderResponse {
        val user = userRepository.findById(userId)
            .orElseThrow { EntityNotFoundException("User not found: $userId") }
        val order = orderRepository.save(Order(user = user))
        return OrderResponse.from(order)
    }
}
```

### Controller (SpringDoc 어노테이션 필수)
```kotlin
@Tag(name = "Order", description = "주문 관리 API")
@RestController
@RequestMapping("/api/v1/orders")
@Validated
class OrderController(private val orderService: OrderService) {

    @Operation(summary = "주문 단건 조회")
    @ApiResponses(value = [
        ApiResponse(responseCode = "200", description = "조회 성공",
            content = [Content(schema = Schema(implementation = OrderResponse::class))]),
        ApiResponse(responseCode = "404", description = "주문 없음",
            content = [Content(schema = Schema(implementation = ErrorResponse::class))]),
    ])
    @GetMapping("/{id}")
    fun getOrder(
        @Parameter(description = "주문 ID", required = true) @PathVariable id: Long,
    ): ResponseEntity<OrderResponse> =
        ResponseEntity.ok(orderService.getOrder(id))

    @Operation(summary = "주문 생성")
    @ApiResponse(responseCode = "201", description = "생성 성공")
    @PostMapping
    fun createOrder(
        @AuthenticationPrincipal userId: Long,
        @RequestBody @Valid request: CreateOrderRequest,
    ): ResponseEntity<OrderResponse> =
        ResponseEntity.status(HttpStatus.CREATED).body(orderService.createOrder(userId, request))
}
```

### Response DTO (Schema 어노테이션 필수)
```kotlin
@Schema(description = "주문 응답")
data class OrderResponse(
    @Schema(description = "주문 ID", example = "1") val id: Long,
    @Schema(description = "주문 상태", example = "PENDING") val status: OrderStatus,
    @Schema(description = "생성일시") val createdAt: LocalDateTime,
) {
    companion object {
        fun from(order: Order) = OrderResponse(
            id = order.id,
            status = order.status,
            createdAt = order.createdAt,
        )
    }
}
```

### Request DTO (Schema 어노테이션 필수)
```kotlin
@Schema(description = "주문 생성 요청")
data class CreateOrderRequest(
    @field:NotNull
    @Schema(description = "상품 ID", example = "10", required = true) val productId: Long,
    @field:Min(1)
    @Schema(description = "수량", example = "2", required = true) val quantity: Int,
)
```

### Migration SQL
```sql
CREATE TABLE orders (
    id         BIGINT       NOT NULL AUTO_INCREMENT,
    user_id    BIGINT       NOT NULL,
    status     VARCHAR(20)  NOT NULL DEFAULT 'PENDING',
    created_at DATETIME(6)  NOT NULL,
    updated_at DATETIME(6)  NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT fk_orders_user FOREIGN KEY (user_id) REFERENCES users (id)
);
```

---

## Modification Patterns

### Field Addition Migration
```sql
-- V5__add_description_to_orders.sql
ALTER TABLE orders ADD COLUMN description VARCHAR(500) NULL;
```

### Pagination
```kotlin
// Repository
fun findAllByUserId(userId: Long, pageable: Pageable): Page<Order>

// Service
fun getOrders(userId: Long, pageable: Pageable): Page<OrderResponse> =
    orderRepository.findAllByUserId(userId, pageable).map { OrderResponse.from(it) }

// Controller
@GetMapping
fun getOrders(
    @AuthenticationPrincipal userId: Long,
    @PageableDefault(size = 20, sort = ["createdAt"], direction = Sort.Direction.DESC) pageable: Pageable,
): ResponseEntity<Page<OrderResponse>> =
    ResponseEntity.ok(orderService.getOrders(userId, pageable))
```

### Soft Delete
```kotlin
// Entity field
@Column(nullable = false)
var deletedAt: LocalDateTime? = null

val isDeleted: Boolean get() = deletedAt != null

// Repository
fun findByIdAndDeletedAtIsNull(id: Long): Order?

// Service
@Transactional
fun deleteOrder(id: Long) {
    val order = orderRepository.findByIdAndDeletedAtIsNull(id)
        ?: throw EntityNotFoundException("Order not found: $id")
    order.deletedAt = LocalDateTime.now()
}
```

---

## Test Patterns

### Service Unit Test
```kotlin
@ExtendWith(MockKExtension::class)
class OrderServiceTest {

    @MockK lateinit var orderRepository: OrderRepository
    @MockK lateinit var userRepository: UserRepository
    @InjectMockKs lateinit var orderService: OrderService

    @Nested
    inner class `getOrder` {
        @Test
        fun `주문이 존재하면 OrderResponse를 반환한다`() {
            val order = OrderFixture.create()
            every { orderRepository.findById(1L) } returns Optional.of(order)

            val result = orderService.getOrder(1L)

            assertThat(result.id).isEqualTo(order.id)
        }

        @Test
        fun `주문이 없으면 EntityNotFoundException을 던진다`() {
            every { orderRepository.findById(999L) } returns Optional.empty()
            assertThrows<EntityNotFoundException> { orderService.getOrder(999L) }
        }
    }
}
```

### Fixture Pattern
```kotlin
object OrderFixture {
    fun create(
        user: User = UserFixture.create(),
        status: OrderStatus = OrderStatus.PENDING,
        id: Long = 1L,
    ) = Order(user = user, status = status).apply {
        val idField = Order::class.java.getDeclaredField("id")
        idField.isAccessible = true
        idField.set(this, id)
    }
}
```

### Controller Test (@WebMvcTest)
```kotlin
@WebMvcTest(OrderController::class)
@Import(SecurityConfig::class)
class OrderControllerTest {

    @Autowired lateinit var mockMvc: MockMvc
    @MockkBean lateinit var orderService: OrderService

    @Test
    @WithMockUser
    fun `GET orders - id로 주문 조회 성공`() {
        val response = OrderResponse(id = 1L, status = OrderStatus.PENDING, createdAt = LocalDateTime.now())
        every { orderService.getOrder(1L) } returns response

        mockMvc.get("/api/v1/orders/1")
            .andExpect {
                status { isOk() }
                jsonPath("$.id") { value(1) }
            }
    }

    @Test
    @WithMockUser
    fun `POST orders - 유효성 실패 시 400 반환`() {
        mockMvc.post("/api/v1/orders") {
            contentType = MediaType.APPLICATION_JSON
            content = """{"productId": null, "quantity": 0}"""
        }.andExpect { status { isBadRequest() } }
    }
}
```

### Repository Test (@DataJpaTest + Testcontainers)
```kotlin
@DataJpaTest
@AutoConfigureTestDatabase(replace = AutoConfigureTestDatabase.Replace.NONE)
@Testcontainers
class OrderRepositoryTest {

    companion object {
        @Container
        val mysql = MySQLContainer("mysql:8.0")

        @JvmStatic
        @DynamicPropertySource
        fun properties(registry: DynamicPropertyRegistry) {
            registry.add("spring.datasource.url", mysql::getJdbcUrl)
            registry.add("spring.datasource.username", mysql::getUsername)
            registry.add("spring.datasource.password", mysql::getPassword)
        }
    }

    @Autowired lateinit var orderRepository: OrderRepository
    @Autowired lateinit var userRepository: UserRepository

    @Test
    fun `userId로 주문 목록을 조회한다`() {
        val user = userRepository.save(UserFixture.createUnsaved())
        repeat(2) { orderRepository.save(OrderFixture.createUnsaved(user = user)) }

        assertThat(orderRepository.findAllByUserId(user.id)).hasSize(2)
    }
}
```

### Test Anti-patterns
- mock 동작 자체를 테스트하는 것 (실제 로직 테스트해야 함)
- 테스트가 설정하지 않은 값에 대한 assertion
- 여러 동작을 하나의 거대한 테스트에 묶기
- 테스트 대상 클래스 자체를 mock
- 특정 값이 중요한데 `any()` 사용

---

## Multi-Module Patterns (Gradle)

### settings.gradle.kts
```kotlin
rootProject.name = "project-name"

include(":api", ":domain", ":infra")
// 필요 시 추가
// include(":core")   // 공통 유틸, 예외, 상수
// include(":batch")  // Spring Batch 모듈
```

### 루트 build.gradle.kts
```kotlin
plugins {
    kotlin("jvm") version "2.0.0" apply false
    kotlin("plugin.spring") version "2.0.0" apply false
    kotlin("plugin.jpa") version "2.0.0" apply false
    id("org.springframework.boot") version "3.3.0" apply false
    id("io.spring.dependency-management") version "1.1.5" apply false
}

subprojects {
    apply(plugin = "org.jetbrains.kotlin.jvm")
    apply(plugin = "io.spring.dependency-management")

    repositories { mavenCentral() }

    dependencies {
        implementation("org.jetbrains.kotlin:kotlin-reflect")
        testImplementation("org.springframework.boot:spring-boot-starter-test")
        testImplementation("io.mockk:mockk:1.13.10")
    }
}
```

### :domain 모듈 build.gradle.kts
```kotlin
// domain 모듈 — 외부 의존성 최소화
plugins {
    kotlin("plugin.jpa")
}

dependencies {
    implementation("org.springframework.boot:spring-boot-starter-data-jpa")
    implementation("com.querydsl:querydsl-jpa:5.1.0:jakarta")
    kapt("com.querydsl:querydsl-apt:5.1.0:jakarta")
}
```

### :infra 모듈 build.gradle.kts
```kotlin
dependencies {
    implementation(project(":domain"))
    implementation("org.springframework.boot:spring-boot-starter-data-jpa")
    // 외부 연동 (Redis, S3, Kafka 등) 의존성
}
```

### :api 모듈 build.gradle.kts
```kotlin
plugins {
    kotlin("plugin.spring")
    id("org.springframework.boot")
}

dependencies {
    implementation(project(":domain"))
    implementation(project(":infra"))
    implementation("org.springframework.boot:spring-boot-starter-web")
    implementation("org.springframework.boot:spring-boot-starter-security")
    implementation("org.springdoc:springdoc-openapi-starter-webmvc-ui:2.6.0")
}
```

### 모듈간 의존 규칙
| 모듈 | 의존 가능 | 의존 불가 |
|------|----------|----------|
| `:domain` | (없음 — 순수) | `:api`, `:infra` |
| `:infra` | `:domain` | `:api` |
| `:api` | `:domain`, `:infra` | (없음) |

### 멀티 모듈 테스트 실행
```bash
# 전체 테스트 + 커버리지
./gradlew test jacocoTestReport

# 특정 모듈만
./gradlew :domain:test
./gradlew :infra:test
./gradlew :api:test

# 빌드
./gradlew :api:bootJar
```
