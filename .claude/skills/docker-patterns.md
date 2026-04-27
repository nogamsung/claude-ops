# Docker Patterns

스택별 프로덕션급 Dockerfile + 로컬 개발용 docker-compose 템플릿. `/new dockerfile` 이 이 skill 을 참조.

## 공통 원칙

- **멀티스테이지 빌드** — builder → runtime 분리. runtime 이미지 크기 최소화
- **non-root 실행** — `USER app` 또는 `USER 1000` 필수
- **`.dockerignore`** — `node_modules/`, `.git/`, `dist/`, `.env`, `target/`, `build/`, `.venv/` 제외
- **HEALTHCHECK** — 컨테이너 상태 확인
- **고정 버전** — `python:3.12-slim` 처럼 major+minor. `latest` 금지
- **Layer caching** — deps 설치를 source copy 와 분리
- **환경 변수** — 기본값 제공, runtime override 가능
- **로그는 stdout/stderr** — 파일 로깅 금지 (컨테이너 철학)

---

## Python FastAPI (uv)

**`Dockerfile`:**
```dockerfile
# ---------- Builder ----------
FROM python:3.12-slim AS builder

WORKDIR /app

# uv 설치 (공식 이미지)
COPY --from=ghcr.io/astral-sh/uv:latest /uv /usr/local/bin/uv

# deps 먼저 (cache)
COPY pyproject.toml uv.lock ./
RUN uv sync --frozen --no-install-project --no-dev

# 소스 복사 + 프로젝트 install
COPY . .
RUN uv sync --frozen --no-dev

# ---------- Runtime ----------
FROM python:3.12-slim AS runtime

# non-root 유저
RUN useradd -m -u 1000 app

WORKDIR /app

# builder 에서 venv 복사
COPY --from=builder --chown=app:app /app/.venv /app/.venv
COPY --from=builder --chown=app:app /app /app

ENV PATH="/app/.venv/bin:$PATH" \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PORT=8000

USER app
EXPOSE 8000

HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
  CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:${PORT}/health').read()" || exit 1

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000", "--workers", "4"]
```

**주의**: AI/ML 스택 포함 시 `FROM pytorch/pytorch:2.3.0-cuda12.1-cudnn8-runtime` 같은 베이스로 교체 (CUDA 필요시).

---

## Kotlin Spring Boot (Gradle)

**`Dockerfile`:**
```dockerfile
# ---------- Builder ----------
FROM eclipse-temurin:21-jdk-alpine AS builder

WORKDIR /app

# Gradle wrapper + deps
COPY gradlew gradlew.bat ./
COPY gradle/ gradle/
COPY build.gradle.kts settings.gradle.kts ./
COPY domain/build.gradle.kts domain/
COPY infra/build.gradle.kts infra/
COPY api/build.gradle.kts api/

RUN ./gradlew dependencies --no-daemon || true

# 소스 + 빌드
COPY . .
RUN ./gradlew :api:bootJar --no-daemon -x test

# ---------- Runtime ----------
FROM eclipse-temurin:21-jre-alpine AS runtime

RUN addgroup -S app && adduser -S app -G app

WORKDIR /app

COPY --from=builder --chown=app:app /app/api/build/libs/*.jar app.jar

ENV JAVA_OPTS="-XX:MaxRAMPercentage=75 -XX:+UseZGC -XX:+ZGenerational" \
    SERVER_PORT=8080

USER app
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
  CMD wget -qO- http://localhost:${SERVER_PORT}/actuator/health || exit 1

ENTRYPOINT ["sh", "-c", "java $JAVA_OPTS -jar app.jar"]
```

**단일 모듈일 경우**: `:api:bootJar` → `bootJar`, `api/build/libs/*.jar` → `build/libs/*.jar`.

---

## Go Gin

**`Dockerfile`:**
```dockerfile
# ---------- Builder ----------
FROM golang:1.23-alpine AS builder

WORKDIR /app

# deps 먼저
COPY go.mod go.sum ./
RUN go mod download

# 소스 + 정적 빌드
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/server ./cmd/server

# ---------- Runtime ----------
FROM gcr.io/distroless/static:nonroot AS runtime

COPY --from=builder --chown=nonroot:nonroot /app/server /app/server

USER nonroot:nonroot
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD ["/app/server", "healthcheck"] || exit 1

ENTRYPOINT ["/app/server"]
```

> distroless 는 가장 얇지만 shell 없음 → HEALTHCHECK 이 바이너리로만 가능. 디버깅 시 `gcr.io/distroless/static:debug-nonroot`.

---

## Next.js (standalone)

**먼저 `next.config.js` 에:**
```javascript
module.exports = { output: 'standalone' };
```

**`Dockerfile`:**
```dockerfile
# ---------- Deps ----------
FROM node:20-alpine AS deps

RUN apk add --no-cache libc6-compat
WORKDIR /app

COPY package.json package-lock.json* ./
RUN npm ci

# ---------- Builder ----------
FROM node:20-alpine AS builder

WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .

ENV NEXT_TELEMETRY_DISABLED=1
RUN npm run build

# ---------- Runtime ----------
FROM node:20-alpine AS runtime

WORKDIR /app

ENV NODE_ENV=production \
    NEXT_TELEMETRY_DISABLED=1 \
    PORT=3000

RUN addgroup -g 1001 -S nodejs && adduser -S nextjs -u 1001

COPY --from=builder /app/public ./public
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static

USER nextjs
EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:3000/api/health || exit 1

CMD ["node", "server.js"]
```

**Turborepo** 인 경우: builder 스테이지에서 `npx turbo prune --docker <app>` 로 필요한 packages 만 추출 후 빌드.

---

## Flutter (Docker 지원 안 함)

Flutter 는 모바일 앱이라 Docker 이미지로 배포 안 함. 대신 `Dockerfile` 이 필요한 경우는 **CI/CD 빌드 환경용**:

```dockerfile
FROM ghcr.io/cirruslabs/flutter:stable AS builder

WORKDIR /app
COPY . .

RUN flutter pub get
RUN flutter build apk --release
# 또는: flutter build ios --release --no-codesign  (macOS only)
```

결과물 (`build/app/outputs/flutter-apk/app-release.apk`) 를 CI artifact 로 업로드.

---

## `.dockerignore` (공통 + 스택별)

**모든 스택 공통:**
```
.git/
.gitignore
README.md
CHANGELOG.md
.env
.env.*
!.env.example
.vscode/
.idea/
.DS_Store
.worktrees/
docs/
tests/
**/*.test.*
**/*.spec.*
*.md
```

**Python 추가:**
```
__pycache__/
*.pyc
*.pyo
.venv/
.pytest_cache/
.ruff_cache/
.mypy_cache/
mlruns/
*.pt
*.safetensors
```

**Kotlin/Gradle 추가:**
```
build/
.gradle/
!gradle/wrapper/gradle-wrapper.jar
out/
```

**Go 추가:**
```
*.exe
*.exe~
*.dll
*.so
*.dylib
*.test
*.out
vendor/
```

**Node.js/Next.js 추가:**
```
node_modules/
.next/
out/
dist/
coverage/
.cache/
```

---

## `docker-compose.yml` — 로컬 개발용

**Python FastAPI + Postgres (pgvector) + Redis:**
```yaml
services:
  app:
    build: .
    ports:
      - "8000:8000"
    environment:
      DATABASE_URL: postgresql+asyncpg://app:app@db:5432/appdb
      REDIS_URL: redis://redis:6379/0
    depends_on:
      db:
        condition: service_healthy
      redis:
        condition: service_healthy
    volumes:
      - ./app:/app/app  # 개발 중 hot reload
    command: ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--reload"]

  db:
    image: pgvector/pgvector:pg16
    environment:
      POSTGRES_USER: app
      POSTGRES_PASSWORD: app
      POSTGRES_DB: appdb
    ports:
      - "5432:5432"
    volumes:
      - db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U app -d appdb"]
      interval: 5s
      timeout: 5s
      retries: 10

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 10

volumes:
  db_data:
  redis_data:
```

**Kotlin Spring Boot + MySQL:**
```yaml
services:
  app:
    build: .
    ports:
      - "8080:8080"
    environment:
      SPRING_PROFILES_ACTIVE: dev
      SPRING_DATASOURCE_URL: jdbc:mysql://db:3306/appdb
      SPRING_DATASOURCE_USERNAME: app
      SPRING_DATASOURCE_PASSWORD: app
    depends_on:
      db:
        condition: service_healthy

  db:
    image: mysql:8
    environment:
      MYSQL_ROOT_PASSWORD: root
      MYSQL_DATABASE: appdb
      MYSQL_USER: app
      MYSQL_PASSWORD: app
    ports:
      - "3306:3306"
    volumes:
      - db_data:/var/lib/mysql
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      interval: 5s
      retries: 20

volumes:
  db_data:
```

**모노레포 (backend-api + backend-ai + web):**
```yaml
services:
  backend-api:
    build: ./backend-api
    ports:
      - "8080:8080"
    depends_on: [db]

  backend-ai:
    build: ./backend-ai
    ports:
      - "8000:8000"
    environment:
      DATABASE_URL: postgresql+asyncpg://app:app@db:5432/appdb
      REDIS_URL: redis://redis:6379/0
      ANTHROPIC_API_KEY: ${ANTHROPIC_API_KEY}
    depends_on: [db, redis]

  web:
    build: ./web
    ports:
      - "3000:3000"
    environment:
      NEXT_PUBLIC_API_URL: http://localhost:8080
    depends_on: [backend-api, backend-ai]

  db:
    image: pgvector/pgvector:pg16
    environment:
      POSTGRES_USER: app
      POSTGRES_PASSWORD: app
      POSTGRES_DB: appdb
    ports: ["5432:5432"]
    volumes:
      - db_data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    ports: ["6379:6379"]
    volumes:
      - redis_data:/data

volumes:
  db_data:
  redis_data:
```

---

## GHCR 배포 (`.github/workflows/publish.yml` 과 연동)

이 skill 로 Dockerfile 을 만들고 나면 `/new workflow publish` 로 GitHub Actions 까지 연결.

**예시 workflow** (`github-actions-patterns.md` 참조):
```yaml
- uses: docker/build-push-action@v5
  with:
    push: true
    tags: |
      ghcr.io/${{ github.repository }}:latest
      ghcr.io/${{ github.repository }}:${{ github.sha }}
    cache-from: type=gha
    cache-to: type=gha,mode=max
    platforms: linux/amd64,linux/arm64
```

---

## 이미지 크기 최적화 팁

| 스택 | 예상 크기 | 비법 |
|------|----------|------|
| Go (distroless) | ~20 MB | CGO_ENABLED=0 + distroless/static |
| Next.js (standalone) | ~150 MB | output: 'standalone', alpine base |
| Python (slim) | ~200 MB | --no-dev, .venv 만 복사 |
| Python (AI/PyTorch) | ~3 GB | CUDA 포함. CPU-only 면 ~1 GB. `torch-cpu` extra 사용 |
| Kotlin (Alpine JRE) | ~250 MB | JRE 21 + bootJar. GraalVM native → ~80 MB (설정 복잡) |

---

## 보안 체크 (security-reviewer 와 연계)

Dockerfile 작성 시 security-reviewer 가 검사:

- ❌ `USER root` 또는 USER 지정 안 함
- ❌ `COPY . .` 에 `.env` 파일 포함 (dockerignore 누락)
- ❌ `latest` 태그
- ❌ `ADD http://...` (signature 검증 없는 원격 다운로드)
- ❌ curl | bash 패턴
- ✅ 멀티스테이지 + non-root + pinned version + HEALTHCHECK + .dockerignore
