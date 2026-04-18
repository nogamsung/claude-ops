# GitHub Actions Patterns

## Trigger Patterns

### CI — PR + 브랜치 push
```yaml
on:
  push:
    branches: [main, dev]
  pull_request:
    branches: [main, dev]
```

### Release — 버전 태그 push
```yaml
on:
  push:
    tags: ['v*.*.*']
  workflow_dispatch:
    inputs:
      tag:
        description: 'Release tag (e.g. v1.2.3)'
        required: true
```

### 변경 경로 필터 (모노레포 / 부분 트리거)
```yaml
on:
  push:
    paths:
      - 'backend/**'
      - '.github/workflows/ci-backend.yml'
```

### 정기 실행
```yaml
on:
  schedule:
    - cron: '0 2 * * 1'  # 매주 월요일 02:00 UTC
  workflow_dispatch:
```

---

## 공통 Job 설정

### permissions 최소 권한 (필수)
```yaml
permissions:
  contents: read          # 기본 CI
# 릴리스·패키지 배포 시 추가:
  contents: write         # 릴리스 생성, 태그 push
  packages: write         # GitHub Packages 배포
  pull-requests: write    # PR 코멘트
  id-token: write         # OIDC (AWS/GCP keyless auth)
```

### concurrency — 동시 실행 방지
```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true  # PR은 최신 push만 실행
```

---

## Cache Patterns (스택별)

### Next.js / Node.js
```yaml
- uses: actions/setup-node@v4
  with:
    node-version: '20'
    cache: 'npm'          # package-lock.json 기반 자동 캐시

- run: npm ci             # npm install 대신 ci 사용 (재현성)
```

### Kotlin / Spring Boot (Gradle)
```yaml
- uses: actions/setup-java@v4
  with:
    java-version: '21'
    distribution: 'temurin'
    cache: 'gradle'       # ~/.gradle 캐시

- name: Grant execute permission
  run: chmod +x gradlew
```

### Go / Gin
```yaml
- uses: actions/setup-go@v5
  with:
    go-version-file: 'go.mod'   # go.mod에서 버전 자동 읽기
    cache: true                  # go.sum 기반 자동 캐시
```

### Flutter
```yaml
- uses: subosito/flutter-action@v2
  with:
    flutter-version-file: pubspec.yaml   # pubspec.yaml에서 버전 읽기
    cache: true
    cache-key: flutter-${{ hashFiles('pubspec.lock') }}
```

---

## CI Templates (스택별)

### Next.js CI
```yaml
# .github/workflows/ci.yml
name: CI

on:
  push:
    branches: [main, dev]
  pull_request:
    branches: [main, dev]

concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true

permissions:
  contents: read

jobs:
  lint-and-type-check:
    name: Lint & Type Check
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: 'npm'
      - run: npm ci
      - run: npm run lint
      - run: npm run type-check

  test:
    name: Test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: 'npm'
      - run: npm ci
      - run: npm run test -- --coverage
      - uses: actions/upload-artifact@v4
        if: always()
        with:
          name: coverage-report
          path: coverage/

  build:
    name: Build
    runs-on: ubuntu-latest
    needs: [lint-and-type-check, test]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: 'npm'
      - run: npm ci
      - run: npm run build
```

### Kotlin / Spring Boot CI
```yaml
# .github/workflows/ci.yml
name: CI

on:
  push:
    branches: [main, dev]
  pull_request:
    branches: [main, dev]

concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true

permissions:
  contents: read

jobs:
  lint:
    name: Lint (ktlint)
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-java@v4
        with:
          java-version: '21'
          distribution: 'temurin'
          cache: 'gradle'
      - run: chmod +x gradlew
      - run: ./gradlew ktlintCheck

  test:
    name: Test
    runs-on: ubuntu-latest
    services:
      mysql:
        image: mysql:8.0
        env:
          MYSQL_ROOT_PASSWORD: root
          MYSQL_DATABASE: testdb
        ports:
          - 3306:3306
        options: --health-cmd="mysqladmin ping" --health-interval=10s --health-timeout=5s --health-retries=3
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-java@v4
        with:
          java-version: '21'
          distribution: 'temurin'
          cache: 'gradle'
      - run: chmod +x gradlew
      - run: ./gradlew test jacocoTestReport
        env:
          SPRING_DATASOURCE_URL: jdbc:mysql://localhost:3306/testdb
          SPRING_DATASOURCE_USERNAME: root
          SPRING_DATASOURCE_PASSWORD: root
      - uses: actions/upload-artifact@v4
        if: always()
        with:
          name: jacoco-report
          path: build/reports/jacoco/

  build:
    name: Build JAR
    runs-on: ubuntu-latest
    needs: [lint, test]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-java@v4
        with:
          java-version: '21'
          distribution: 'temurin'
          cache: 'gradle'
      - run: chmod +x gradlew
      - run: ./gradlew bootJar -x test
      - uses: actions/upload-artifact@v4
        with:
          name: app-jar
          path: build/libs/*.jar
          retention-days: 7
```

### Go / Gin CI
```yaml
# .github/workflows/ci.yml
name: CI

on:
  push:
    branches: [main, dev]
  pull_request:
    branches: [main, dev]

concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true

permissions:
  contents: read

jobs:
  lint:
    name: Lint (golangci-lint)
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: 'go.mod'
          cache: true
      - uses: golangci/golangci-lint-action@v6
        with:
          version: latest

  test:
    name: Test
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16
        env:
          POSTGRES_USER: test
          POSTGRES_PASSWORD: test
          POSTGRES_DB: testdb
        ports:
          - 5432:5432
        options: --health-cmd pg_isready --health-interval 10s --health-timeout 5s --health-retries 5
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: 'go.mod'
          cache: true
      - run: go test ./... -coverprofile=coverage.out -covermode=atomic
        env:
          DATABASE_URL: postgres://test:test@localhost:5432/testdb?sslmode=disable
      - name: Check coverage threshold (80%)
        run: |
          COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | tr -d '%')
          echo "Coverage: ${COVERAGE}%"
          awk "BEGIN { if ($COVERAGE < 80) { print \"Coverage below 80%\"; exit 1 } }"

  build:
    name: Build
    runs-on: ubuntu-latest
    needs: [lint, test]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: 'go.mod'
          cache: true
      - run: go build -o bin/server ./cmd/main.go
```

### Flutter CI
```yaml
# .github/workflows/ci.yml
name: CI

on:
  push:
    branches: [main, dev]
  pull_request:
    branches: [main, dev]

concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true

permissions:
  contents: read

jobs:
  analyze-and-test:
    name: Analyze & Test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: subosito/flutter-action@v2
        with:
          flutter-version-file: pubspec.yaml
          cache: true
      - run: flutter pub get
      - run: flutter analyze
      - run: flutter test --coverage
      - uses: actions/upload-artifact@v4
        if: always()
        with:
          name: flutter-coverage
          path: coverage/lcov.info
```

---

## Release & Version Management

### VERSION 파일 기반 릴리스 (이 프로젝트 방식)
```yaml
# .github/workflows/release.yml
name: Release

on:
  push:
    tags: ['v*.*.*']

permissions:
  contents: write

jobs:
  release:
    name: Create GitHub Release
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0          # CHANGELOG 생성을 위해 전체 히스토리

      - name: Extract version from tag
        id: version
        run: echo "version=${GITHUB_REF_NAME#v}" >> $GITHUB_OUTPUT

      - name: Generate release notes
        id: notes
        run: |
          # 직전 태그와의 커밋 로그 추출
          PREV_TAG=$(git describe --tags --abbrev=0 HEAD^ 2>/dev/null || echo "")
          if [ -z "$PREV_TAG" ]; then
            COMMITS=$(git log --oneline --pretty=format:"- %s" HEAD)
          else
            COMMITS=$(git log --oneline --pretty=format:"- %s" ${PREV_TAG}..HEAD)
          fi
          echo "notes<<EOF" >> $GITHUB_OUTPUT
          echo "$COMMITS" >> $GITHUB_OUTPUT
          echo "EOF" >> $GITHUB_OUTPUT

      - uses: softprops/action-gh-release@v2
        with:
          name: Release ${{ github.ref_name }}
          body: |
            ## What's Changed
            ${{ steps.notes.outputs.notes }}
          draft: false
          prerelease: ${{ contains(github.ref_name, '-') }}  # v1.0.0-beta → prerelease
```

### 자동 버전 태깅 (Conventional Commits → semver)
```yaml
# .github/workflows/release.yml
name: Auto Release

on:
  push:
    branches: [main]

permissions:
  contents: write

jobs:
  release:
    name: Semantic Release
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
          token: ${{ secrets.GITHUB_TOKEN }}

      - uses: actions/setup-node@v4
        with:
          node-version: '20'

      - name: Install semantic-release
        run: npm install -g semantic-release @semantic-release/changelog @semantic-release/git

      - name: Run semantic-release
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: semantic-release
```

```json
// .releaserc.json (semantic-release 설정)
{
  "branches": ["main"],
  "plugins": [
    "@semantic-release/commit-analyzer",
    "@semantic-release/release-notes-generator",
    ["@semantic-release/changelog", { "changelogFile": "CHANGELOG.md" }],
    ["@semantic-release/git", {
      "assets": ["CHANGELOG.md", "VERSION"],
      "message": "chore(release): ${nextRelease.version} [skip ci]"
    }],
    "@semantic-release/github"
  ]
}
```

### package.json version 동기화
```yaml
- name: Bump version in package.json
  run: |
    VERSION=${GITHUB_REF_NAME#v}
    npm version $VERSION --no-git-tag-version
```

### VERSION 파일 → 태그 push (이 프로젝트 방식)
```yaml
# /commit 커맨드에서 VERSION 변경 감지 후 실행:
- name: Push version tag
  if: steps.changed.outputs.version == 'true'
  run: |
    VERSION=$(cat VERSION)
    git tag "v${VERSION}"
    git push origin "v${VERSION}"
```

---

## Package Publishing

### npm publish (GitHub Packages)
```yaml
# .github/workflows/publish.yml
name: Publish to GitHub Packages

on:
  release:
    types: [published]

permissions:
  contents: read
  packages: write

jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: '20'
          registry-url: 'https://npm.pkg.github.com'
          scope: '@${{ github.repository_owner }}'
      - run: npm ci
      - run: npm run build
      - run: npm publish
        env:
          NODE_AUTH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### npm publish (npmjs.com)
```yaml
jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: '20'
          registry-url: 'https://registry.npmjs.org'
      - run: npm ci && npm run build
      - run: npm publish --access public
        env:
          NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}
```

### Docker build & push (GitHub Container Registry)
```yaml
# .github/workflows/publish.yml
name: Docker Publish

on:
  push:
    tags: ['v*.*.*']

permissions:
  contents: read
  packages: write

jobs:
  docker:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Docker meta (태그 자동 생성)
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/${{ github.repository }}
          tags: |
            type=semver,pattern={{version}}       # v1.2.3 → 1.2.3
            type=semver,pattern={{major}}.{{minor}} # v1.2.3 → 1.2
            type=semver,pattern={{major}}           # v1.2.3 → 1
            type=sha,prefix=sha-                   # sha-abc1234
            type=raw,value=latest,enable=${{ github.ref == 'refs/tags/v*' && !contains(github.ref, '-') }}

      - uses: docker/setup-buildx-action@v3

      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
          platforms: linux/amd64,linux/arm64
```

### Gradle / Maven → GitHub Packages
```yaml
# .github/workflows/publish.yml
name: Publish Package

on:
  release:
    types: [published]

permissions:
  contents: read
  packages: write

jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-java@v4
        with:
          java-version: '21'
          distribution: 'temurin'
          cache: 'gradle'
      - run: chmod +x gradlew
      - run: ./gradlew publish
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

```kotlin
// build.gradle.kts — publishing 설정
publishing {
    repositories {
        maven {
            name = "GitHubPackages"
            url = uri("https://maven.pkg.github.com/${System.getenv("GITHUB_REPOSITORY")}")
            credentials {
                username = System.getenv("GITHUB_ACTOR")
                password = System.getenv("GITHUB_TOKEN")
            }
        }
    }
}
```

---

## Multi-Version Matrix

### Node.js 버전 매트릭스
```yaml
jobs:
  test:
    strategy:
      matrix:
        node-version: ['18', '20', '22']
      fail-fast: false    # 하나 실패해도 다른 버전 계속 실행
    runs-on: ubuntu-latest
    name: Test (Node ${{ matrix.node-version }})
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: ${{ matrix.node-version }}
          cache: 'npm'
      - run: npm ci && npm test
```

### OS 매트릭스
```yaml
strategy:
  matrix:
    os: [ubuntu-latest, windows-latest, macos-latest]
runs-on: ${{ matrix.os }}
```

### 제외·포함 조합
```yaml
strategy:
  matrix:
    os: [ubuntu-latest, windows-latest]
    node: ['18', '20']
    exclude:
      - os: windows-latest
        node: '18'
    include:
      - os: ubuntu-latest
        node: '22'
        experimental: true
```

---

## Reusable Workflow Pattern

### 재사용 가능 워크플로 정의
```yaml
# .github/workflows/reusable-test.yml
name: Reusable Test

on:
  workflow_call:
    inputs:
      node-version:
        type: string
        default: '20'
    secrets:
      DATABASE_URL:
        required: true
    outputs:
      coverage:
        value: ${{ jobs.test.outputs.coverage }}

jobs:
  test:
    runs-on: ubuntu-latest
    outputs:
      coverage: ${{ steps.coverage.outputs.value }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: ${{ inputs.node-version }}
          cache: 'npm'
      - run: npm ci && npm test
        env:
          DATABASE_URL: ${{ secrets.DATABASE_URL }}
      - id: coverage
        run: echo "value=$(cat coverage/coverage-summary.json | jq '.total.lines.pct')" >> $GITHUB_OUTPUT
```

### 재사용 워크플로 호출
```yaml
# .github/workflows/ci.yml
jobs:
  test:
    uses: ./.github/workflows/reusable-test.yml
    with:
      node-version: '20'
    secrets:
      DATABASE_URL: ${{ secrets.DATABASE_URL }}
```

---

## Environment & Secrets 관리

### 환경별 배포 분리
```yaml
jobs:
  deploy-staging:
    environment: staging          # GitHub Environment 설정 필요
    env:
      APP_URL: ${{ vars.STAGING_URL }}        # vars = non-secret 환경 변수
    steps:
      - run: echo "Deploy to staging"

  deploy-production:
    needs: deploy-staging
    environment: production       # production은 reviewers 승인 후 실행
    env:
      APP_URL: ${{ vars.PRODUCTION_URL }}
    steps:
      - run: echo "Deploy to production"
```

### OIDC keyless auth (AWS)
```yaml
permissions:
  id-token: write
  contents: read

- uses: aws-actions/configure-aws-credentials@v4
  with:
    role-to-assume: ${{ secrets.AWS_ROLE_ARN }}
    aws-region: ap-northeast-2
```

### 필요한 Secrets 체크리스트
| Secret | 용도 |
|--------|------|
| `GITHUB_TOKEN` | 자동 제공 — 릴리스·패키지 배포 |
| `NPM_TOKEN` | npmjs.com 배포 |
| `DOCKER_USERNAME` / `DOCKER_PASSWORD` | Docker Hub |
| `AWS_ROLE_ARN` | AWS OIDC |
| `GCP_WORKLOAD_IDENTITY_PROVIDER` | GCP OIDC |
| `SLACK_WEBHOOK_URL` | 배포 알림 |

---

## 알림 패턴

### Slack 배포 알림
```yaml
- name: Notify Slack
  if: always()
  uses: slackapi/slack-github-action@v2
  with:
    webhook: ${{ secrets.SLACK_WEBHOOK_URL }}
    webhook-type: incoming-webhook
    payload: |
      {
        "text": "${{ job.status == 'success' && '✅' || '❌' }} *${{ github.workflow }}* — ${{ github.ref_name }} by ${{ github.actor }}"
      }
```

### PR 커버리지 코멘트
```yaml
- uses: MishaKav/jest-coverage-comment@v1
  with:
    coverage-summary-path: coverage/coverage-summary.json
    title: 'Test Coverage'
```

---

## Stack별 Docker 배포 (GitHub Container Registry)

> Flutter 제외. Kotlin Spring Boot / Go Gin / Next.js 대상.
> 이미지는 `ghcr.io/{owner}/{repo}` 에 push.

### Kotlin Spring Boot — Dockerfile
```dockerfile
# Dockerfile
# ── Build ──────────────────────────────────────────────
FROM eclipse-temurin:21-jdk-alpine AS builder
WORKDIR /app

COPY gradlew .
COPY gradle/ gradle/
COPY build.gradle.kts settings.gradle.kts ./
RUN ./gradlew dependencies --no-daemon   # 의존성 레이어 캐시

COPY src/ src/
RUN ./gradlew bootJar -x test --no-daemon

# ── Runtime ────────────────────────────────────────────
FROM eclipse-temurin:21-jre-alpine
WORKDIR /app

RUN addgroup -S app && adduser -S app -G app
COPY --from=builder /app/build/libs/*.jar app.jar
USER app

EXPOSE 8080
ENTRYPOINT ["java", \
  "-XX:+UseContainerSupport", \
  "-XX:MaxRAMPercentage=75", \
  "-jar", "app.jar"]
```

### Go Gin — Dockerfile
```dockerfile
# Dockerfile
# ── Build ──────────────────────────────────────────────
FROM golang:1.22-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download   # 의존성 레이어 캐시

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s" \
    -o server ./cmd/main.go

# ── Runtime ────────────────────────────────────────────
FROM alpine:3.19
WORKDIR /app

RUN apk --no-cache add ca-certificates tzdata && \
    addgroup -S app && adduser -S app -G app

COPY --from=builder /app/server .
USER app

EXPOSE 8080
ENTRYPOINT ["./server"]
```

### Next.js — Dockerfile
```dockerfile
# Dockerfile
# next.config.js에 output: 'standalone' 필수
# ── Dependencies ───────────────────────────────────────
FROM node:20-alpine AS deps
WORKDIR /app
COPY package*.json ./
RUN npm ci

# ── Builder ────────────────────────────────────────────
FROM node:20-alpine AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
ENV NEXT_TELEMETRY_DISABLED=1
RUN npm run build

# ── Runtime ────────────────────────────────────────────
FROM node:20-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1

RUN addgroup -S app && adduser -S app -G app
COPY --from=builder /app/public ./public
COPY --from=builder /app/.next/standalone ./
COPY --from=builder /app/.next/static ./.next/static

USER app
EXPOSE 3000
ENTRYPOINT ["node", "server.js"]
```

```js
// next.config.js — standalone 빌드 활성화 필수
/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'standalone',
}
module.exports = nextConfig
```

---

### 공통 Docker 배포 워크플로 구조
```
트리거: push tags v*.*.* (릴리스) 또는 push main (스테이징)
  └─ build-and-push job
       ├─ docker/metadata-action → 태그 자동 생성
       │   semver: 1.2.3 / 1.2 / 1 / latest
       │   sha:    sha-abc1234
       ├─ docker/setup-buildx-action (멀티플랫폼)
       ├─ docker/login-action → ghcr.io
       └─ docker/build-push-action
           cache-from/to: type=gha  (GitHub Actions 캐시)
           platforms: linux/amd64,linux/arm64
```

### Kotlin Spring Boot — 배포 워크플로
```yaml
# .github/workflows/publish.yml
name: Docker Publish — Spring Boot

on:
  push:
    tags: ['v*.*.*']

permissions:
  contents: read
  packages: write

jobs:
  build-and-push:
    name: Build & Push Docker Image
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Docker metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/${{ github.repository }}
          tags: |
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}
            type=semver,pattern={{major}}
            type=sha,prefix=sha-
            type=raw,value=latest,enable={{is_default_branch}}

      - uses: docker/setup-buildx-action@v3

      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Build JAR first (레이어 캐시 활용)
        uses: actions/setup-java@v4
        with:
          java-version: '21'
          distribution: 'temurin'
          cache: 'gradle'

      - run: chmod +x gradlew && ./gradlew bootJar -x test --no-daemon

      - uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
          platforms: linux/amd64,linux/arm64
```

### Go Gin — 배포 워크플로
```yaml
# .github/workflows/publish.yml
name: Docker Publish — Go Gin

on:
  push:
    tags: ['v*.*.*']

permissions:
  contents: read
  packages: write

jobs:
  build-and-push:
    name: Build & Push Docker Image
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Docker metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/${{ github.repository }}
          tags: |
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}
            type=semver,pattern={{major}}
            type=sha,prefix=sha-
            type=raw,value=latest,enable={{is_default_branch}}

      - uses: docker/setup-buildx-action@v3

      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
          platforms: linux/amd64,linux/arm64
          # Go는 Dockerfile 내 go mod download가 캐시 레이어 역할
```

### Next.js — 배포 워크플로
```yaml
# .github/workflows/publish.yml
name: Docker Publish — Next.js

on:
  push:
    tags: ['v*.*.*']

permissions:
  contents: read
  packages: write

jobs:
  build-and-push:
    name: Build & Push Docker Image
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Docker metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/${{ github.repository }}
          tags: |
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}
            type=semver,pattern={{major}}
            type=sha,prefix=sha-
            type=raw,value=latest,enable={{is_default_branch}}

      - uses: docker/setup-buildx-action@v3

      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
          platforms: linux/amd64,linux/arm64
          build-args: |
            NEXT_PUBLIC_API_URL=${{ vars.NEXT_PUBLIC_API_URL }}
```

### dev 브랜치 push → 스테이징 이미지 자동 배포 (선택)
```yaml
# dev merge 시 :dev 태그로 스테이징 이미지 자동 업데이트
on:
  push:
    branches: [dev]   # dev merge 시 트리거

# metadata tags 교체:
tags: |
  type=raw,value=dev
  type=sha,prefix=sha-
```

### 생성된 이미지 확인
```bash
# GitHub 저장소 → Packages 탭에서 확인
# 또는 로컬에서:
docker pull ghcr.io/{owner}/{repo}:{tag}
docker run -p 8080:8080 ghcr.io/{owner}/{repo}:latest
```

### GHCR 이미지 공개 설정
> 기본적으로 private. 공개하려면:
> GitHub → 패키지 클릭 → Package settings → Change visibility → Public
