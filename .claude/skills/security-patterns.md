# Security Patterns

보안 리뷰·작성 시 참고할 체크리스트 + 스택별 pitfall. security-reviewer 전용이지만, 각 generator/modifier agent 도 코드 작성 시 참조.

## OWASP Top 10 — 빠른 참조

| ID | 이름 | 요약 | 빈번한 실수 |
|----|------|------|-----------|
| A01 | Broken Access Control | 인가 검사 누락 | IDOR (`/users/:id` 다른 사람 ID 접근), 서버측 role 체크 없음, path traversal |
| A02 | Cryptographic Failures | 약한 암호화·평문 | 평문 비밀번호 저장, MD5/SHA1 해시, 약한 JWT secret, HTTP 강제 |
| A03 | Injection | 외부 입력 직접 실행 | f-string/string concat SQL, `subprocess(shell=True)`, LDAP, XPath |
| A04 | Insecure Design | 설계 수준 결함 | rate limit 없음, 비밀번호 재설정 토큰 예측 가능, race condition |
| A05 | Security Misconfig | 기본 설정 노출 | DEBUG 켜짐, 과도한 CORS, 디렉토리 리스팅, 불필요한 HTTP 메서드 |
| A06 | Vulnerable Components | 오래된 라이브러리 | CVE 있는 의존성 방치, 미검증 transitive dep |
| A07 | Auth Failures | 인증 흐름 결함 | 약한 세션, 토큰 탈취 가능, MFA 우회, 비밀번호 정책 미흡 |
| A08 | Data Integrity Failures | 서명 검증 없음 | `pickle.loads()`, Java `ObjectInputStream`, unsigned JWT 허용 |
| A09 | Logging Failures | 민감정보 로그 | 패스워드·토큰·PII 로깅, 로그 조작 가능, audit trail 부재 |
| A10 | SSRF | 서버측 URL fetch | `requests.get(user_url)`, AWS metadata 서비스 접근 가능 |

## 공통 안티패턴 (언어 무관)

### 시크릿 관리
❌ `API_KEY = "sk-abc123..."` 하드코딩
❌ `.env` 파일을 git 에 커밋
❌ 로그에 토큰·패스워드 출력

✅ 환경변수 + Pydantic Settings / Spring `@Value` / Next.js `process.env`
✅ `.gitignore` 에 `.env*` (단, `.env.example` 은 예외)
✅ 프로덕션: Secret Manager (AWS Secrets Manager, Vault, Doppler)

### 인증 토큰 저장 (클라이언트)
❌ `localStorage` 에 JWT (XSS 시 탈취)
✅ HttpOnly Secure Cookie + SameSite=Strict/Lax
✅ 모바일: OS secure storage (iOS Keychain, Android Keystore)

### 비밀번호 저장
❌ 평문, MD5, SHA1, SHA256 (raw)
✅ **bcrypt**, **argon2id**, scrypt (work factor 10+)
✅ `hmac.compare_digest()` 로 비교 (timing attack 방지)

### CORS
❌ `Access-Control-Allow-Origin: *` + `Access-Control-Allow-Credentials: true` (브라우저가 reject)
✅ 명시적 origin 화이트리스트
✅ preflight 캐시 적절히 (`Access-Control-Max-Age: 600`)

### CSP (Content Security Policy)
- `default-src 'self'`
- `'unsafe-inline'`, `'unsafe-eval'` 피하기
- nonce 또는 hash 기반 inline script 허용

---

## 스택별 체크리스트

### Python FastAPI

**SQL Injection 방지:**
```python
# ❌ 위험
async def get_user(db, user_id: str):
    return await db.execute(text(f"SELECT * FROM users WHERE id = {user_id}"))

# ✅ 안전 (parameterized)
async def get_user(db, user_id: int):
    return await db.execute(text("SELECT * FROM users WHERE id = :id"), {"id": user_id})

# ✅ ORM 사용
async def get_user(db, user_id: int):
    return await db.execute(select(User).where(User.id == user_id))
```

**인증 토큰 검증:**
```python
# ❌ 약한 JWT secret
jwt.encode(payload, "secret", algorithm="HS256")

# ✅ 강한 secret + 만료 + signature verify
SECRET = settings.JWT_SECRET  # 최소 256bit random
jwt.encode({"sub": user.id, "exp": now + timedelta(minutes=15)}, SECRET, algorithm="HS256")
jwt.decode(token, SECRET, algorithms=["HS256"])  # algorithm 명시 필수
```

**민감 정보 응답 금지:**
```python
# ❌ stack trace 노출
@app.exception_handler(Exception)
async def handler(request, exc):
    return JSONResponse({"error": str(exc)})  # 내부 구조 유출

# ✅ 안전
@app.exception_handler(Exception)
async def handler(request, exc):
    logger.exception("Unhandled", extra={"path": request.url.path})
    return JSONResponse({"error": "Internal server error"}, status_code=500)
```

**Pydantic 검증:**
```python
# ✅ 입력 검증 + sanitization
class CreateUserRequest(BaseModel):
    email: EmailStr
    password: str = Field(min_length=12, max_length=128)
    age: int = Field(ge=0, le=150)
```

**패스워드 해싱:**
```python
from passlib.context import CryptContext
pwd_ctx = CryptContext(schemes=["argon2"], deprecated="auto")
hashed = pwd_ctx.hash(plain_password)
pwd_ctx.verify(plain, hashed)
```

**위험한 API:**
- ❌ `pickle.loads(external_data)` — RCE
- ❌ `eval()`, `exec()`, `subprocess(shell=True)` — injection
- ❌ `yaml.load()` — use `yaml.safe_load()`
- ❌ `requests.get(url, verify=False)` — MITM 취약

---

### Kotlin Spring Boot

**Spring Security 필수 설정:**
```kotlin
@EnableWebSecurity
class SecurityConfig {
    @Bean
    fun filterChain(http: HttpSecurity): SecurityFilterChain = http
        .csrf { it.disable() }  // API only. 웹이면 활성화 필수
        .sessionManagement { it.sessionCreationPolicy(STATELESS) }
        .authorizeHttpRequests { auth ->
            auth.requestMatchers("/api/public/**").permitAll()
                .anyRequest().authenticated()
        }
        .oauth2ResourceServer { it.jwt { } }
        .build()
}
```

**Controller 권한 검증:**
```kotlin
// ❌ 인가 없음
@GetMapping("/users/{id}")
fun getUser(@PathVariable id: Long) = service.find(id)

// ✅ @PreAuthorize 명시
@GetMapping("/users/{id}")
@PreAuthorize("hasRole('ADMIN') or #id == authentication.principal.id")
fun getUser(@PathVariable id: Long) = service.find(id)
```

**JPA 쿼리 안전:**
```kotlin
// ❌ string concat
@Query(value = "SELECT * FROM users WHERE email = '${email}'", nativeQuery = true)

// ✅ parameter binding
@Query(value = "SELECT * FROM users WHERE email = :email", nativeQuery = true)
fun findByEmail(@Param("email") email: String): User?
```

**민감 필드 응답 제외:**
```kotlin
@Entity
data class User(
    val id: Long,
    val email: String,

    @JsonIgnore
    @Column(name = "password_hash")
    val passwordHash: String,  // 응답에서 제외
)
```

**패스워드 해싱:**
```kotlin
@Bean
fun passwordEncoder() = BCryptPasswordEncoder(12)  // 또는 Argon2PasswordEncoder
```

---

### Go Gin

**SQL 안전 (sqlc 권장):**
```go
// ❌ string concat
db.Raw(fmt.Sprintf("SELECT * FROM users WHERE id=%d", id))

// ✅ placeholders (GORM)
db.Where("id = ?", id).First(&user)

// ✅ sqlc 사용 — 컴파일 타임 쿼리 생성
queries.GetUser(ctx, userID)
```

**Context 검증:**
```go
// ✅ binding 태그로 검증
type CreateUserRequest struct {
    Email    string `json:"email" binding:"required,email"`
    Password string `json:"password" binding:"required,min=12"`
}

if err := c.ShouldBindJSON(&req); err != nil {
    c.JSON(400, gin.H{"error": "invalid input"})
    return
}
```

**난수는 crypto/rand:**
```go
// ❌ 예측 가능
import "math/rand"
token := rand.Int63()

// ✅ 암호학적 난수
import "crypto/rand"
b := make([]byte, 32)
rand.Read(b)
token := base64.URLEncoding.EncodeToString(b)
```

**Middleware 인증:**
```go
func AuthMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        token := c.GetHeader("Authorization")
        claims, err := jwt.Parse(token, keyFn)
        if err != nil {
            c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"})
            return
        }
        c.Set("userID", claims["sub"])
        c.Next()
    }
}
```

---

### Next.js

**Server Action 권한 검사:**
```typescript
// ✅ 모든 Server Action 에 인증·인가 검증 (first lines)
"use server";

export async function deletePost(postId: string) {
  const session = await getSession();
  if (!session) throw new Error("Unauthorized");

  const post = await db.post.findUnique({ where: { id: postId } });
  if (post?.authorId !== session.userId) throw new Error("Forbidden");

  // 실제 삭제 로직
}
```

**XSS 방지:**
```tsx
// ❌ user input → dangerouslySetInnerHTML
<div dangerouslySetInnerHTML={{ __html: userContent }} />

// ✅ React 는 기본 escape. 정말 HTML 필요하면 DOMPurify
import DOMPurify from "dompurify";
<div dangerouslySetInnerHTML={{ __html: DOMPurify.sanitize(userContent) }} />
```

**Environment 변수:**
```typescript
// ❌ 클라이언트에서 민감 정보 접근 (`NEXT_PUBLIC_` prefix 는 번들에 노출됨)
const apiKey = process.env.NEXT_PUBLIC_SECRET_KEY;  // 금지

// ✅ server-only
// lib/env.server.ts
import "server-only";
export const SECRET_KEY = process.env.SECRET_KEY!;
```

**Middleware 로 경로 보호:**
```typescript
// middleware.ts
export function middleware(request: NextRequest) {
  const token = request.cookies.get("session")?.value;
  if (!token && request.nextUrl.pathname.startsWith("/dashboard")) {
    return NextResponse.redirect(new URL("/login", request.url));
  }
}
export const config = { matcher: ["/dashboard/:path*"] };
```

**CSP headers:**
```javascript
// next.config.js
module.exports = {
  async headers() {
    return [{
      source: "/:path*",
      headers: [
        { key: "Content-Security-Policy", value: "default-src 'self'; script-src 'self' 'unsafe-inline'" },
        { key: "X-Frame-Options", value: "DENY" },
        { key: "X-Content-Type-Options", value: "nosniff" },
        { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
      ],
    }];
  },
};
```

---

### Flutter

**Secure Storage:**
```dart
// ❌ SharedPreferences 에 토큰 저장 (암호화 안 됨)
final prefs = await SharedPreferences.getInstance();
await prefs.setString("token", token);

// ✅ flutter_secure_storage (iOS Keychain / Android Keystore)
const storage = FlutterSecureStorage();
await storage.write(key: "token", value: token);
```

**Certificate Pinning:**
```dart
import 'package:dio/dio.dart';
import 'package:dio_http2_adapter/dio_http2_adapter.dart';

final dio = Dio();
dio.httpClientAdapter = Http2Adapter(ConnectionManager(
  onClientCreate: (_, config) {
    config.onBadCertificate = (cert) =>
      cert.sha1.toLowerCase() == 'expected_pin_here';
  },
));
```

**Deep link 검증:**
```dart
// ✅ 파라미터 화이트리스트
if (!allowedSchemes.contains(uri.scheme)) return;
if (uri.path.contains('..')) return;  // path traversal
```

---

## 의존성 감사 명령 (스택별)

```bash
# Python
uv pip compile pyproject.toml --format requirements.txt | pip-audit --format json
# 또는
uv run pip-audit

# Node.js
npm audit --audit-level=high
# 또는 (덜 시끄러움)
npm audit --production

# Kotlin (Gradle) — dependency-check 플러그인 필요
./gradlew dependencyCheckAnalyze
# 결과: build/reports/dependency-check-report.html

# Go
go list -json -m all | nancy sleuth
# 또는
govulncheck ./...

# Flutter
flutter pub outdated
dart pub deps --json | # 별도 도구 필요
```

---

## 시크릿 스캐닝 (git 에 이미 푸시됐다면)

**즉시 조치:**
1. **해당 키를 rotate** — provider dashboard 에서 revoke + 재발급
2. **git history 에서 제거** (이미 공개된 경우에도, 봇 스캔 방지):
   ```bash
   # BFG Repo-Cleaner
   bfg --replace-text passwords.txt

   # 또는 git-filter-repo
   git filter-repo --path secret.env --invert-paths
   ```
3. **팀에 공지** — 새 clone 필요

**사전 방지:**
- `pre-commit` hook 에 `detect-secrets` 또는 `gitleaks`
- GitHub secret scanning 활성화 (push protection)

---

## Rate Limiting

| 스택 | 라이브러리 |
|------|-----------|
| FastAPI | `slowapi` (Redis backend 권장) |
| Spring Boot | Bucket4j + Redis |
| Gin | `gin-contrib/limit` 또는 Redis 기반 커스텀 |
| Next.js | Vercel `@upstash/ratelimit` 또는 middleware |

**권장 기본값:**
- 로그인: 10 req/min per IP
- Public API: 100 req/min per IP
- 인증된 API: 1000 req/min per user

---

## 감사 로그 (Audit Trail)

민감 작업 (로그인, 권한 변경, 결제, 삭제) 은 구조화 로그로 기록:

```json
{
  "timestamp": "2026-04-24T12:34:56Z",
  "event": "user.login",
  "user_id": "u_abc",
  "ip": "1.2.3.4",
  "user_agent": "...",
  "success": true,
  "mfa": "totp"
}
```

**로그에 넣지 말 것**: 패스워드, 토큰 원문, 신용카드 번호, 주민번호.
