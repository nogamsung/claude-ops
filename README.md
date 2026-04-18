# scheduled-dev-agent

홈서버/VPS 상주 Go 단일 바이너리. GitHub 이슈를 Claude Code CLI로 자동 처리 → PR 생성.

PRD: [`docs/specs/scheduled-dev-agent.md`](docs/specs/scheduled-dev-agent.md)

---

## 빠른 시작

### 필수 전제 조건

```bash
# 1. Claude Code CLI 설치 및 로그인 (대상 머신에서)
claude login

# 2. GitHub CLI 인증
gh auth login

# 3. git 2.17+ 확인
git --version
```

### 설치 및 실행

```bash
# 소스 빌드
make build

# 환경 변수 설정
cp .env.example .env
vim .env  # GITHUB_TOKEN, SLACK_BOT_TOKEN, SLACK_SIGNING_SECRET 입력

# 설정 파일 편집
cp config.example.yaml config.yaml
vim config.yaml  # active_windows, repos 설정

# 실행
make run
# 또는
./bin/scheduled-dev-agent -config config.yaml
```

### API 사용 예시

```bash
# 헬스 체크
curl http://127.0.0.1:8787/healthz

# 작업 목록 조회
curl http://127.0.0.1:8787/tasks?status=queued

# Full usage 모드 활성화 (시간대 게이트 우회)
curl -X POST http://127.0.0.1:8787/modes/full \
  -H 'Content-Type: application/json' \
  -d '{"enabled": true}'

# 수동 작업 트리거
curl -X POST http://127.0.0.1:8787/tasks \
  -H 'Content-Type: application/json' \
  -d '{"repo": "owner/repo", "issue_number": 42}'
```

---

## 개발

```bash
make test       # 전체 테스트
make cover      # 커버리지 측정
make lint       # golangci-lint
make swag       # Swagger 문서 생성
make sqlc       # sqlc 코드 생성 (sqlc 설치 필요)
```

## 배포

[`deployments/README.md`](deployments/README.md) 참고.

---

## 아키텍처

```
internal/
  config/       YAML + env 설정 로더
  scheduler/    active-window 게이트, tick 루프, full-mode 토글
  github/       이슈 poller, PR creator (gh CLI)
  claude/       Claude CLI 실행 + stream-json 파서 + pgid kill
  slack/        Block Kit 빌더, interactions webhook, 서명 검증
  usecase/      비즈니스 로직 (task, mode)
  api/          Gin HTTP 핸들러 + Swagger 주석
  repository/   GORM + SQLite 구현체
  domain/       순수 도메인 엔티티 (외부 의존성 없음)
```
