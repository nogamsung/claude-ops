# MySQL DB 설계 패턴

## Section 1: MySQL 데이터 타입 선택 기준

| 타입 | 사용 시점 |
|------|----------|
| `BIGINT UNSIGNED AUTO_INCREMENT` | PK (항상) |
| `VARCHAR(n)` | 검색·인덱스가 필요한 문자열 (최대 255자 권장) |
| `TEXT` | 긴 본문·설명 등 검색 안 하는 텍스트 |
| `DECIMAL(p, s)` | 금액·가격·비율 — `FLOAT` / `DOUBLE` 절대 금지 |
| `TINYINT(1)` | boolean 대용 (0 = false, 1 = true) |
| `DATETIME(6)` | `created_at` / `updated_at` (마이크로초 포함) |
| `VARCHAR(20~50)` | 상태값 (ENUM 대신 권장 — ALTER 비용 낮음) |
| `ENUM` | 변경 빈도가 매우 낮고 값 목록이 고정된 경우만 허용 |
| `JSON` | 비정형 데이터 — 검색·인덱스가 불필요한 경우만 사용 |
| `INT` | 순서·수량 등 32비트로 충분한 정수 |

> **ENUM 주의**: 새 값 추가 시 `ALTER TABLE`이 필요하고 MySQL 버전에 따라 테이블 락이 발생합니다.
> 상태값처럼 변경 가능성이 있는 컬럼은 `VARCHAR` + 애플리케이션 레벨 검증을 권장합니다.

---

## Section 2: 공통 컬럼 패턴 (모든 테이블 필수)

모든 테이블에 아래 컬럼을 **반드시** 포함합니다.

```sql
id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
created_at DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
updated_at DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
PRIMARY KEY (id)
```

- `id`: PK, 항상 `BIGINT UNSIGNED AUTO_INCREMENT`
- `created_at`: 레코드 생성 시각, 기본값 자동 설정
- `updated_at`: 레코드 수정 시각, `ON UPDATE`로 자동 갱신
- `DEFAULT CURRENT_TIMESTAMP(6)` — 마이크로초 단위로 정밀도 보장

---

## Section 3: 인덱스 전략

### FK 컬럼 인덱스 (필수)
FK 컬럼에는 항상 `INDEX`를 추가합니다.

```sql
INDEX idx_{table}_{fk_col} ({fk_col})
```

### 복합 인덱스 — 자주 쓰는 조회 조건
```sql
-- user_id + status 조합으로 자주 조회하는 경우
INDEX idx_orders_user_status (user_id, status)
```

### UNIQUE 제약
```sql
UNIQUE KEY uk_{table}_{col} ({col})
-- 예: UNIQUE KEY uk_users_email (email)
```

### Covering Index 패턴
SELECT 컬럼이 인덱스에 포함되도록 설계하면 테이블 접근 없이 인덱스만으로 응답 가능합니다.

```sql
-- orders를 user_id로 조회하고 status만 SELECT하는 경우
INDEX idx_orders_user_status (user_id, status)
```

### 절대 금지 — 잘못된 인덱스
```sql
-- ❌ 카디널리티가 낮은 컬럼 단독 인덱스 (성별, boolean, 고정 상태값 등)
INDEX idx_users_gender (gender)   -- 2가지 값만 존재 → 풀스캔보다 오히려 느림
INDEX idx_orders_is_deleted (is_deleted)  -- 0/1 두 값 → 비효율

-- ✅ 대신 카디널리티 높은 컬럼과 복합 인덱스로 구성
INDEX idx_orders_user_deleted (user_id, deleted_at)
```

---

## Section 4: Soft Delete 패턴

논리 삭제가 필요한 테이블에는 `deleted_at` 컬럼을 추가합니다.

```sql
deleted_at DATETIME(6) NULL DEFAULT NULL,
INDEX idx_{table}_deleted_at (deleted_at)
```

- `NULL` → 삭제되지 않은 레코드
- `NOT NULL` 값 → 삭제된 레코드 (삭제 시각)

**조회 시 항상 아래 조건을 포함합니다:**
```sql
WHERE deleted_at IS NULL
```

**삭제 처리:**
```sql
UPDATE {table} SET deleted_at = CURRENT_TIMESTAMP(6) WHERE id = ?;
```

---

## Section 5: FK 제약 패턴

```sql
CONSTRAINT fk_{table}_{ref} FOREIGN KEY ({col}) REFERENCES {ref_table} ({col})
  ON DELETE RESTRICT   -- 기본값
  ON UPDATE CASCADE
```

### ON DELETE 옵션 선택 기준

| 옵션 | 사용 시점 |
|------|----------|
| `RESTRICT` | 부모 삭제 전 자식을 먼저 삭제해야 할 때 (기본값, 안전) |
| `CASCADE` | 부모 삭제 시 자식도 함께 삭제해야 할 때 (예: `order_items` → `orders`) |
| `SET NULL` | 부모 삭제 시 자식의 FK를 NULL로 남겨야 할 때 (약한 참조) |

> **`CASCADE` 사용 주의**: 의도치 않은 대량 삭제가 발생할 수 있으므로 관계가 명확한 경우에만 사용합니다.
> 예: `orders`가 삭제되면 `order_items`도 삭제 → CASCADE 적합
> 예: `users`가 삭제되어도 `orders`는 남겨야 함 → RESTRICT 또는 SET NULL

---

## Section 6: Flyway 네이밍 컨벤션 (Kotlin)

```
V{숫자}__{설명}.sql
예: V1__create_users_table.sql
    V2__create_orders_table.sql
    V3__add_phone_to_users.sql
    V4__create_idx_orders_user_id.sql
```

### 규칙

- 번호는 **순차 정수** (소수점 버전 금지: `V1.1__` 사용 불가)
- 구분자는 **언더스코어 2개** (`__`)
- 설명은 **snake_case**
- **기존 파일 절대 수정 금지** — 이미 적용된 migration은 내용 변경 시 Flyway 에러 발생
- 새 변경은 반드시 새 버전 파일로 추가

### 위치
```
src/main/resources/db/migration/
  V1__create_users_table.sql
  V2__create_orders_table.sql
  V3__add_phone_to_users.sql
```

### 기존 번호 확인 방법
```bash
ls src/main/resources/db/migration/ | sort
# 가장 큰 번호 + 1을 다음 버전으로 사용
```

---

## Section 7: golang-migrate 네이밍 컨벤션 (Go)

```
{6자리숫자}_{설명}.up.sql
{6자리숫자}_{설명}.down.sql
예: 000001_create_users_table.up.sql
    000001_create_users_table.down.sql
    000002_create_orders_table.up.sql
    000002_create_orders_table.down.sql
```

### 규칙

- 번호는 **6자리 0-padding 정수** (`000001`, `000002`, ...)
- `up` / `down` **쌍 반드시 생성** — 둘 중 하나만 있으면 migrate 실행 불가
- `down`은 `up`의 **완전한 역순** 작업 (주로 `DROP TABLE IF EXISTS`)
- 설명은 **snake_case**

### 위치
```
migrations/
  000001_create_users_table.up.sql
  000001_create_users_table.down.sql
  000002_create_orders_table.up.sql
  000002_create_orders_table.down.sql
```

### 기존 번호 확인 방법
```bash
ls migrations/*.up.sql | sort | tail -1
# 가장 큰 번호 + 1을 다음 번호로 사용
```

---

## Section 8: 완성된 예시 — users 테이블 (Kotlin/Go 공통)

### up

```sql
CREATE TABLE users (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    email      VARCHAR(255)    NOT NULL,
    name       VARCHAR(100)    NOT NULL,
    status     VARCHAR(20)     NOT NULL DEFAULT 'ACTIVE',
    deleted_at DATETIME(6)     NULL     DEFAULT NULL,
    created_at DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_users_email (email),
    INDEX idx_users_status (status),
    INDEX idx_users_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

### down

```sql
DROP TABLE IF EXISTS users;
```

---

## Section 9: 관계 테이블 예시 (N:M)

```sql
CREATE TABLE order_items (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    order_id   BIGINT UNSIGNED NOT NULL,
    product_id BIGINT UNSIGNED NOT NULL,
    quantity   INT             NOT NULL,
    price      DECIMAL(10, 2)  NOT NULL,
    created_at DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    INDEX idx_order_items_order_id (order_id),
    INDEX idx_order_items_product_id (product_id),
    CONSTRAINT fk_order_items_order   FOREIGN KEY (order_id)   REFERENCES orders (id) ON DELETE CASCADE  ON UPDATE CASCADE,
    CONSTRAINT fk_order_items_product FOREIGN KEY (product_id) REFERENCES products (id) ON DELETE RESTRICT ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

### down

```sql
DROP TABLE IF EXISTS order_items;
```

> **down 파일 작성 순서 주의**: FK 제약이 있는 자식 테이블을 먼저 DROP한 뒤 부모 테이블을 DROP합니다.
> 예: `order_items` → `orders` → `users` 순으로 DROP
