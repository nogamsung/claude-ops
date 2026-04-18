---
description: Conventional Commits 형식으로 git 커밋 메시지 작성 및 커밋
argument-hint: [커밋 메시지 힌트] (없으면 변경사항을 분석하여 자동 생성)
---

Conventional Commits 규칙에 따라 커밋을 생성합니다.

**힌트**: $ARGUMENTS

## 커밋 메시지 형식

```
<type>(<scope>): <subject>

[body]

[footer]
```

### Type 목록
- `feat`: 새로운 기능 추가
- `fix`: 버그 수정
- `refactor`: 리팩토링 (기능 변경 없음)
- `test`: 테스트 추가/수정
- `docs`: 문서 수정
- `style`: 코드 포맷팅, 세미콜론 누락 등 (로직 변경 없음)
- `chore`: 빌드 설정, 패키지 업데이트 등
- `perf`: 성능 개선
- `ci`: CI/CD 설정 변경
- `build`: 빌드 시스템 변경

### Scope 예시 (프로젝트별)
- **Backend**: `auth`, `user`, `product`, `order`, `payment`
- **Frontend**: `layout`, `auth`, `dashboard`, `ui`
- **Mobile**: `auth`, `home`, `profile`, `nav`

## 진행 순서

1. `git status`로 변경된 파일 확인
2. `git diff --staged` 또는 `git diff`로 변경 내용 확인
3. 변경사항을 분석하여 적절한 커밋 메시지 초안 작성
4. `$ARGUMENTS`에 힌트가 있으면 참고하여 메시지 보완
5. 커밋 메시지를 사용자에게 보여주고 확인 요청
6. 확인 후 `git commit` 실행
7. **커밋 후 자동 실행** — 현재 브랜치 push:
   ```bash
   git push origin HEAD
   ```

8. **피처 브랜치 감지 시 `/pr` → `/merge` 체인 제안**
   ```bash
   BRANCH=$(git branch --show-current)
   case "$BRANCH" in
     feature/*|fix/*|hotfix/*|refactor/*|chore/*|docs/*|test/*|perf/*)
       echo "💡 피처 브랜치입니다. 다음 단계를 진행할까요?"
       echo "   y  — /pr 실행 → (PR 생성 후) /merge 까지 이어서 실행"
       echo "   p  — /pr 만 실행 (머지는 나중에 수동)"
       echo "   N  — 지금은 종료 (기본값)"
       ;;
   esac
   ```

   - `y` → `/pr` 실행 → 이어서 `/merge` 의 `y/a/N` 프롬프트로 체인 진행
   - `p` → `/pr` 만 실행 (그 안에서 `/merge` 자동 제안 받음)
   - `N` 또는 응답 없음 → 커맨드 종료
   - `main`/`dev` 브랜치이면 제안하지 않음

> ⚠️ `main`에 직접 커밋하지 않습니다. 항상 피처 브랜치에서 작업 후 PR을 통해 머지합니다.
> 버전 태그는 `/merge` 의 "머지 후 정리" 단계에서 생성됩니다.

**전체 체인 요약**:
```
/commit → (피처 브랜치) → /pr 제안 → [y] → /pr 실행 → /merge 제안 → [y/a] → /merge 실행
```
각 단계는 **사용자 확인**이 필요합니다 — 완전 자동이 아니라 연속 확인 체인.

---

## 문서 업데이트 (Step 6 전에 반드시 실행)

커밋 전 아래 문서를 **필수로** 업데이트합니다.
**사용자에게 별도로 확인하지 않고** 조용히 업데이트 후 "문서를 업데이트했습니다." 한 줄만 출력합니다.

> ⚠️ **필수 규칙**: 문서 업데이트 없이 커밋하지 않습니다. CHANGELOG는 항상 기록합니다.

### 1. CHANGELOG.md + VERSION 업데이트

`[Unreleased]` 섹션은 사용하지 않습니다. **모든 커밋은 즉시 버전으로 발행합니다.**

**버전 결정 규칙 (브랜치 무관하게 동일 적용):**
- `feat` → minor 버전 올림 (1.4.0 → 1.5.0), VERSION 파일도 함께 수정
- `fix` / `perf` → patch 버전 올림 (1.4.0 → 1.4.1), VERSION 파일도 함께 수정
- `refactor` / `chore` / `docs` / `test` / `style` / `ci` / `build` → patch 버전 올림 (1.4.0 → 1.4.1), VERSION 파일도 함께 수정

**CHANGELOG 항목 형식:**
```markdown
## [{새 버전}] - YYYY-MM-DD

### Added   ← feat
### Fixed   ← fix
### Changed ← refactor, perf, chore, docs, test, style, ci, build
### Removed ← 삭제된 기능이 있을 때
```

이미 오늘 날짜로 같은 버전 섹션이 있으면 해당 섹션에 항목을 추가합니다.

### 2. README.md 업데이트

변경된 파일에 따라 해당 README 섹션을 업데이트합니다:

| 변경 대상 | 업데이트할 README 섹션 |
|----------|----------------------|
| `.claude/commands/` 신규 파일 | **커맨드** 표에 행 추가 |
| `.claude/agents/` 신규 파일 | **Agents** 표에 행 추가 |
| `.claude/skills/` 신규 파일 | **디렉토리 구조** 목록에 추가 |
| `.claude/commands/` 신규 파일 | **디렉토리 구조** 목록에 추가 |
| VERSION 변경 | 뱃지 버전 번호 (`version-X.X.X-blue`) 및 구조 섹션 버전 업데이트 |

### 3. memory/MEMORY.md 업데이트

type이 `feat`이고 새로운 커맨드·에이전트·스킬을 추가한 경우에만 기록합니다:

```markdown
## YYYY-MM-DD: {커밋 subject}

**카테고리:** 결정

{변경 내용 요약 — 무엇을 왜 추가했는지}
```

## 커밋 메시지 작성 규칙
- `subject`는 50자 이내, 현재형 동사로 시작 (한국어 가능)
- `body`는 변경 이유와 영향을 설명 (선택사항)
- `BREAKING CHANGE:` footer로 하위 호환성 깨지는 변경 표시
- 여러 독립적인 변경사항이면 atomic commit 권장 (분리 제안)

## 예시

```
feat(auth): JWT 토큰 기반 로그인 API 구현

- POST /api/v1/auth/login 엔드포인트 추가
- AccessToken(1h) + RefreshToken(7d) 발급
- Spring Security 필터 체인 설정

Closes #42
```

```
fix(user): 이메일 중복 검사 누락 버그 수정
```

```
refactor(product): ProductService 레이어 분리

기존 Controller에 비즈니스 로직이 포함되어 있던 것을
Service 레이어로 분리하여 단일 책임 원칙 적용
```
