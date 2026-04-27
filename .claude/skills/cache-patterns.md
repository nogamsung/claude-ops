# Cache Patterns (Redis)

Redis 를 활용한 캐싱·세션·레이트리밋·분산 락 패턴. 모든 백엔드 스택에서 공유.

## 언제 Redis 를 쓰는가

| 용도 | 설명 |
|------|------|
| **Cache** | DB 쿼리 결과·LLM 응답·외부 API 응답 저장. TTL 만료 |
| **Session** | JWT 대신 서버측 세션. logout 즉시 무효화 가능 |
| **Rate Limit** | IP/유저별 요청 카운터 (fixed window, sliding window, token bucket) |
| **Distributed Lock** | 여러 워커 간 동시 실행 방지 (cron, 메시지 중복 처리 등) |
| **Pub/Sub** | 실시간 이벤트 전파 (WebSocket, SSE 백본) |
| **Queue** | 간단한 job queue (Celery/Sidekiq/BullMQ 백엔드로도) |
| **Leaderboard** | Sorted Set 으로 랭킹 |

## 공통 원칙

- **키 네임스페이싱**: `app:entity:{id}` 또는 `{env}:{entity}:{id}` 형식 (env prefix 로 dev/prod 격리)
- **TTL 필수**: 모든 cache 키에 expire 설정. Redis 메모리 터지면 치명적
- **Serialization**: JSON 권장. 언어 간 호환성. Python `pickle` 금지 (언어 종속 + security)
- **Connection Pool**: 앱당 커넥션 풀 1개 재사용
- **Fallback**: Redis 다운 시 DB 로 폴백 (cache 가 필수 의존성 되면 단점)
- **Cache invalidation**: Write 시점에 invalidate — "Write-through" 또는 "Cache-aside + explicit delete"
- **Stampede 방지**: 인기 캐시 만료 시 DB 폭주 — probabilistic early expiration 또는 mutex

---

## Python FastAPI

**패키지**: `redis>=5.0` (async 지원, 구 `aioredis` 통합됨)

### 클라이언트 초기화 (`app/core/redis.py`)

```python
from redis.asyncio import ConnectionPool, Redis
from app.core.config import settings

_pool: ConnectionPool | None = None

def get_redis_pool() -> ConnectionPool:
    global _pool
    if _pool is None:
        _pool = ConnectionPool.from_url(
            settings.REDIS_URL,
            max_connections=50,
            decode_responses=True,
        )
    return _pool

async def get_redis() -> Redis:
    return Redis(connection_pool=get_redis_pool())

# FastAPI dependency
from typing import Annotated
from fastapi import Depends

RedisDep = Annotated[Redis, Depends(get_redis)]
```

### Cache-Aside 패턴

```python
# app/services/user_service.py
import json
from app.core.redis import RedisDep

CACHE_TTL = 300  # 5 min

class UserService:
    async def get_user(self, redis: Redis, user_id: int) -> dict:
        key = f"user:{user_id}"

        # 1. cache hit?
        cached = await redis.get(key)
        if cached:
            return json.loads(cached)

        # 2. cache miss → DB
        user = await self.repo.get_by_id(user_id)
        if user:
            await redis.setex(key, CACHE_TTL, json.dumps(user.to_dict()))

        return user.to_dict() if user else None

    async def update_user(self, redis: Redis, user_id: int, data: dict):
        await self.repo.update(user_id, data)
        # 쓰기 시 invalidate (중요)
        await redis.delete(f"user:{user_id}")
```

### Rate Limiting (sliding window with Sorted Set)

```python
# app/middleware/rate_limit.py
import time
from fastapi import Request, HTTPException

async def check_rate_limit(redis: Redis, key: str, limit: int, window: int):
    """sliding window counter. window: seconds, limit: max requests"""
    now = time.time()
    pipe = redis.pipeline()
    pipe.zremrangebyscore(key, 0, now - window)  # 오래된 것 제거
    pipe.zcard(key)                               # 현재 카운트
    pipe.zadd(key, {str(now): now})
    pipe.expire(key, window)
    _, count, _, _ = await pipe.execute()

    if count >= limit:
        raise HTTPException(429, "Rate limit exceeded")

# 사용
@app.middleware("http")
async def rate_limit_mw(request: Request, call_next):
    redis = await get_redis()
    ip = request.client.host
    await check_rate_limit(redis, f"ratelimit:{ip}", limit=100, window=60)
    return await call_next(request)
```

**더 간단한 옵션**: `slowapi` 라이브러리 (`@limiter.limit("100/minute")`).

### Distributed Lock

```python
# app/core/lock.py
import uuid
from contextlib import asynccontextmanager

@asynccontextmanager
async def redis_lock(redis: Redis, key: str, ttl: int = 30):
    """NX + EX 를 이용한 간단 락. 고급은 redlock-py 사용"""
    token = str(uuid.uuid4())
    acquired = await redis.set(f"lock:{key}", token, nx=True, ex=ttl)
    if not acquired:
        raise RuntimeError(f"Failed to acquire lock: {key}")
    try:
        yield
    finally:
        # Lua 스크립트로 atomic check-and-delete
        script = """
        if redis.call("get", KEYS[1]) == ARGV[1] then
            return redis.call("del", KEYS[1])
        else
            return 0
        end
        """
        await redis.eval(script, 1, f"lock:{key}", token)

# 사용
async def send_daily_report():
    async with redis_lock(redis, "daily-report", ttl=600):
        # 여러 워커가 동시에 실행해도 하나만 통과
        ...
```

### 세션 (JWT 대안)

```python
# app/services/session_service.py
import secrets, json
from datetime import timedelta

SESSION_TTL = 60 * 60 * 24 * 7  # 7 days

async def create_session(redis: Redis, user_id: int) -> str:
    session_id = secrets.token_urlsafe(32)
    await redis.setex(
        f"session:{session_id}",
        SESSION_TTL,
        json.dumps({"user_id": user_id}),
    )
    return session_id

async def get_session(redis: Redis, session_id: str) -> dict | None:
    data = await redis.get(f"session:{session_id}")
    return json.loads(data) if data else None

async def destroy_session(redis: Redis, session_id: str):
    await redis.delete(f"session:{session_id}")
```

---

## Kotlin Spring Boot

**의존성**: `spring-boot-starter-data-redis` + `lettuce-core` (기본)

### 설정 (`application.yml`)

```yaml
spring:
  data:
    redis:
      host: ${REDIS_HOST:localhost}
      port: 6379
      password: ${REDIS_PASSWORD:}
      lettuce:
        pool:
          max-active: 50
          max-idle: 10
          min-idle: 2
```

### 캐시 추상화 (`@Cacheable`)

```kotlin
@Configuration
@EnableCaching
class CacheConfig {
    @Bean
    fun cacheManager(factory: RedisConnectionFactory): CacheManager {
        val config = RedisCacheConfiguration.defaultCacheConfig()
            .entryTtl(Duration.ofMinutes(5))
            .serializeValuesWith(SerializationPair.fromSerializer(GenericJackson2JsonRedisSerializer()))

        return RedisCacheManager.builder(factory)
            .cacheDefaults(config)
            .withCacheConfiguration("users", config.entryTtl(Duration.ofMinutes(30)))
            .build()
    }
}

// 사용
@Service
class UserService(private val repo: UserRepository) {
    @Cacheable(value = ["users"], key = "#id")
    fun getUser(id: Long): User = repo.findById(id).orElseThrow()

    @CacheEvict(value = ["users"], key = "#id")
    fun updateUser(id: Long, request: UpdateUserRequest) { ... }
}
```

### RedisTemplate 직접 사용

```kotlin
@Service
class SessionService(private val redis: StringRedisTemplate) {
    fun create(userId: Long): String {
        val sessionId = UUID.randomUUID().toString()
        redis.opsForValue().set(
            "session:$sessionId",
            userId.toString(),
            Duration.ofDays(7)
        )
        return sessionId
    }

    fun get(sessionId: String): Long? =
        redis.opsForValue().get("session:$sessionId")?.toLong()

    fun destroy(sessionId: String) {
        redis.delete("session:$sessionId")
    }
}
```

### Rate Limit (Bucket4j + Redis)

```kotlin
// build.gradle.kts: implementation("com.bucket4j:bucket4j_jdk17-core:8.10.1")
// implementation("com.bucket4j:bucket4j_jdk17-redis:8.10.1")

@Component
class RateLimitFilter(private val redis: StringRedisTemplate) : OncePerRequestFilter() {
    override fun doFilterInternal(request: HttpServletRequest, response: HttpServletResponse, chain: FilterChain) {
        val key = "ratelimit:${request.remoteAddr}"
        val bucket = resolveBucket(key)
        if (bucket.tryConsume(1)) {
            chain.doFilter(request, response)
        } else {
            response.status = 429
            response.writer.write("Rate limit exceeded")
        }
    }
}
```

### Distributed Lock (Redisson)

```kotlin
// implementation("org.redisson:redisson-spring-boot-starter:3.33.0")

@Service
class ReportService(private val redisson: RedissonClient) {
    fun sendDailyReport() {
        val lock: RLock = redisson.getLock("daily-report")
        if (lock.tryLock(0, 600, TimeUnit.SECONDS)) {
            try {
                // 실제 작업
            } finally {
                lock.unlock()
            }
        }
    }
}
```

---

## Go Gin

**패키지**: `github.com/redis/go-redis/v9`

### 클라이언트 초기화

```go
// internal/cache/redis.go
package cache

import (
    "context"
    "github.com/redis/go-redis/v9"
    "os"
)

var Client *redis.Client

func Init() {
    Client = redis.NewClient(&redis.Options{
        Addr:     os.Getenv("REDIS_URL"),
        PoolSize: 50,
    })
}
```

### Cache-Aside

```go
// internal/service/user_service.go
const cacheTTL = 5 * time.Minute

func (s *UserService) GetUser(ctx context.Context, id int64) (*User, error) {
    key := fmt.Sprintf("user:%d", id)

    // 1. cache hit
    if data, err := cache.Client.Get(ctx, key).Bytes(); err == nil {
        var u User
        if err := json.Unmarshal(data, &u); err == nil {
            return &u, nil
        }
    }

    // 2. cache miss → DB
    user, err := s.repo.GetByID(ctx, id)
    if err != nil {
        return nil, err
    }

    // 3. write cache
    if data, err := json.Marshal(user); err == nil {
        cache.Client.Set(ctx, key, data, cacheTTL)
    }

    return user, nil
}

func (s *UserService) UpdateUser(ctx context.Context, id int64, req UpdateRequest) error {
    if err := s.repo.Update(ctx, id, req); err != nil {
        return err
    }
    cache.Client.Del(ctx, fmt.Sprintf("user:%d", id))
    return nil
}
```

### Rate Limit Middleware

```go
func RateLimitMiddleware(limit int, window time.Duration) gin.HandlerFunc {
    return func(c *gin.Context) {
        ctx := c.Request.Context()
        key := fmt.Sprintf("ratelimit:%s", c.ClientIP())
        now := time.Now().UnixNano()

        pipe := cache.Client.Pipeline()
        pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprint(now - window.Nanoseconds()))
        count := pipe.ZCard(ctx, key)
        pipe.ZAdd(ctx, key, redis.Z{Score: float64(now), Member: now})
        pipe.Expire(ctx, key, window)
        pipe.Exec(ctx)

        if count.Val() >= int64(limit) {
            c.JSON(429, gin.H{"error": "rate limit exceeded"})
            c.Abort()
            return
        }
        c.Next()
    }
}
```

---

## Next.js (서버사이드)

**패키지**: `ioredis` 또는 `@upstash/redis` (Vercel 서버리스)

### 일반 Redis (Node.js)

```typescript
// lib/redis.ts
import Redis from "ioredis";

declare global {
  var redis: Redis | undefined;
}

export const redis =
  global.redis ??
  new Redis(process.env.REDIS_URL!, {
    maxRetriesPerRequest: 3,
    lazyConnect: true,
  });

if (process.env.NODE_ENV !== "production") global.redis = redis;
```

### Route Handler 에서 캐싱

```typescript
// app/api/users/[id]/route.ts
import { redis } from "@/lib/redis";
import { db } from "@/lib/db";

export async function GET(req: Request, { params }: { params: { id: string } }) {
  const key = `user:${params.id}`;

  const cached = await redis.get(key);
  if (cached) return Response.json(JSON.parse(cached));

  const user = await db.user.findUnique({ where: { id: params.id } });
  if (user) await redis.setex(key, 300, JSON.stringify(user));

  return Response.json(user);
}
```

### Vercel / Edge Runtime: Upstash

```typescript
// lib/redis-edge.ts
import { Redis } from "@upstash/redis";

export const redis = Redis.fromEnv();  // UPSTASH_REDIS_REST_URL + UPSTASH_REDIS_REST_TOKEN

// Edge middleware 에서 사용 가능
```

### Rate Limit (Upstash 공식)

```typescript
// middleware.ts
import { Ratelimit } from "@upstash/ratelimit";
import { Redis } from "@upstash/redis";

const ratelimit = new Ratelimit({
  redis: Redis.fromEnv(),
  limiter: Ratelimit.slidingWindow(100, "1 m"),
});

export async function middleware(req: Request) {
  const ip = req.headers.get("x-forwarded-for") ?? "anonymous";
  const { success } = await ratelimit.limit(ip);
  if (!success) return new Response("Too Many Requests", { status: 429 });
}
```

---

## 키 네이밍 컨벤션 (권장)

```
<service>:<entity>:<id>                    # app:user:123
<service>:<entity>:<id>:<field>            # app:user:123:profile
<service>:cache:<query_hash>               # 복잡 쿼리 결과
session:<session_id>                       # session:abc123
ratelimit:<ip>:<endpoint>                  # ratelimit:1.2.3.4:/api/login
lock:<resource>                            # lock:daily-report
pubsub:<channel>                           # pubsub:user-events
leaderboard:<type>:<period>                # leaderboard:xp:weekly
```

**환경 prefix** (dev/prod 같은 Redis 인스턴스 공유 시):
```
dev:app:user:123
prod:app:user:123
```

---

## TTL 가이드

| 용도 | 권장 TTL |
|------|----------|
| 유저 프로필 (자주 조회) | 5~30분 |
| DB 쿼리 결과 (무거움) | 1~5분 |
| LLM 응답 (결정적 프롬프트) | 1시간 ~ 1일 |
| Session | 7~30일 (refresh 로 연장) |
| Password reset token | 15분 |
| OTP | 5분 |
| Rate limit counter | window 와 동일 |
| Idempotency key | 24시간 |

---

## Cache invalidation 패턴

### 1. TTL-only (단순, 약간 stale 허용)
그냥 TTL 만 설정. 읽기만 많고 쓰기 드물면 충분.

### 2. Write-through (쓰기 시 즉시 갱신)
```python
async def update_user(id, data):
    user = await repo.update(id, data)
    await redis.setex(f"user:{id}", TTL, json.dumps(user.dict()))
```

### 3. Cache-aside + explicit delete (가장 흔함)
```python
async def update_user(id, data):
    await repo.update(id, data)
    await redis.delete(f"user:{id}")  # 다음 read 가 DB + 재캐싱
```

### 4. Tag-based invalidation (복잡한 관계)
```python
# User 수정 시 "user:123" + 연관된 "post:list:by_user:123" 모두 무효화
await redis.delete("user:123", "post:list:by_user:123")

# 또는 Redis SETS 로 tag 관리
await redis.sadd("tag:user:123", "user:123", "post:list:by_user:123")
# invalidate:
keys = await redis.smembers("tag:user:123")
await redis.delete(*keys, "tag:user:123")
```

---

## 모니터링 / 장애 대응

- **`INFO memory`**: Redis 메모리 사용량 추적
- **`CONFIG GET maxmemory-policy`**: `allkeys-lru` 권장 (LRU eviction)
- **Slow log**: `CONFIG SET slowlog-log-slower-than 10000` (10ms 이상 쿼리 로깅)
- **Redis 다운 대응**: 모든 cache 호출에 try/except 로 fallback → DB 직접 쿼리
- **Connection leak**: 커넥션 풀 max size 모니터링
- **Cluster 모드**: 샤딩 필요하면 Redis Cluster / Sentinel (HA)

---

## 안티패턴 ❌

- `KEYS *` 프로덕션 사용 — O(N) 블로킹. 대신 `SCAN` 사용
- 큰 value (>1MB) — 네트워크·메모리 낭비. 파일은 S3, 메타만 Redis
- 무한 TTL (`SET key value` without EX) — 메모리 누수 원인 1위
- Pickle serialization (Python) — security + 언어 종속
- Cache 를 단일 진실 원천으로 — cache 는 항상 선택적, DB 가 source of truth
