---
description: 현재 브랜치에서 PR 생성. base는 dev/main 자동 감지. 완료 후 /merge 자동 제안.
argument-hint: (인수 없음 — 자동으로 push + PR 생성)
---

현재 worktree의 브랜치에서 PR을 생성합니다.

---

## PR base branch 규칙 (필수)

> 🔒 **이 규칙은 모든 프로젝트·모든 호출에 예외 없이 적용됩니다.**
> 사용자·Claude 누구도 `gh pr create --base <다른브랜치>` 로 override 하면 안 됩니다.

```
로컬 `dev` 존재          → base = dev
로컬 없고 origin/dev 존재 → base = dev
둘 다 없음                → base = main
현재 브랜치가 dev         → base = main   (base == head 금지 예외)
```

## Step 1 — 사전 확인

```bash
BRANCH=$(git branch --show-current)

# 로컬 dev 우선, remote origin/dev 도 체크 (네트워크 실패 시 로컬 결과만 사용)
if git show-ref --verify --quiet refs/heads/dev \
   || git ls-remote --heads origin dev 2>/dev/null | grep -q refs/heads/dev; then
  BASE_BRANCH="dev"
else
  BASE_BRANCH="main"
fi

# 현재 브랜치가 dev 이면 main 으로 폴백 (base == head 방지)
[ "$BRANCH" = "dev" ] && BASE_BRANCH="main"

echo "브랜치: $BRANCH → PR base: $BASE_BRANCH"
```

이미 PR이 있으면 그 URL만 출력하고 Step 3으로 진행:

```bash
EXISTING=$(gh pr view --json url 2>/dev/null | jq -r '.url // empty')
[ -n "$EXISTING" ] && echo "ℹ️  이미 PR 존재: $EXISTING"
```

## Step 1.5 — 🔒 보안 리뷰 (자동, 필수)

> **모든 기능 PR 은 보안 리뷰를 통과해야 합니다.** 인자에 `--skip-security` 가 없으면 자동 실행.

### 리뷰 대상 수집

```bash
# base 대비 diff 생성
git fetch origin "$BASE_BRANCH" --quiet 2>/dev/null || true
DIFF=$(git diff "origin/$BASE_BRANCH"..."$BRANCH" 2>/dev/null | head -5000)
CHANGED_FILES=$(git diff --name-only "origin/$BASE_BRANCH"..."$BRANCH" 2>/dev/null)
```

### security-reviewer agent 호출

```
Agent(
  subagent_type="security-reviewer",
  prompt="""
  /pr 커맨드에서 자동 호출. 아래 diff 에 대해 OWASP Top 10 + 스택별 보안 pitfall
  검사 + 시크릿 유출 + 의존성 CVE 를 검토하고 리포트 반환.

  대상 브랜치: {BRANCH}
  Base: {BASE_BRANCH}
  변경 파일 수: {file_count}

  --- staged diff (최대 5000 줄) ---
  {DIFF}
  --- end diff ---

  리포트 끝에 반드시 판정값을 포함: PASS / REVIEW / BLOCK
  """
)
```

### 판정별 분기

| 판정 | 동작 |
|------|------|
| **PASS** (Critical/High 없음) | 바로 Step 2 로 진행 |
| **REVIEW** (High 발견) | 리포트 보여주고 "계속 진행? (y/N)" — 기본값 N |
| **BLOCK** (Critical 발견) | 리포트 보여주고 **exit** — "수정 후 다시 /pr 실행하세요" |

### 예외: `--skip-security` 옵션

긴급 hotfix 등에 사용. 여전히 리뷰는 실행되지만:
- Critical 도 경고로 강등 (차단 없음)
- 사용자에게 **이유 입력 요구** (필수)
- PR 본문에 자동 주입:
  ```markdown
  ⚠️ **SECURITY REVIEW SKIPPED** — reason: {사용자 입력}
  Issues found: Critical {N}, High {N}
  Reviewer must verify before merge.
  ```

> **이 옵션은 예외다**. 일반 기능 개발엔 쓰지 말 것. 사용 시 메모리에 자동 기록.

## Step 2 — push & PR 생성

```bash
git push -u origin $BRANCH

gh pr create \
  --base $BASE_BRANCH \
  --title "{type}: {기능 요약}" \
  --body "$(cat <<'EOF'
## Summary
-

## Changes
-

## Test plan
- [ ] 단위 테스트 추가/통과
- [ ] 로컬 동작 확인

EOF
)"
```

PR 제목의 `{type}` 은 브랜치 prefix에서 추론 (`feature/` → `feat`, `fix/` → `fix`, `refactor/` → `refactor` 등).

## Step 3 — `/merge` 자동 제안

PR 생성 직후 사용자에게 다음 단계를 물어봅니다:

```
✅ PR 생성 완료: <URL>

다음 단계로 /merge 를 실행할까요?
  y  — 즉시 머지 (리뷰/체크 조건 맞으면 바로 머지 + 정리)
  a  — auto-merge 큐잉 (체크 통과 시 자동 머지)
  N  — 지금은 머지하지 않음 (기본값, 나중에 /merge 수동 호출)
```

- `y` → `/merge` 를 곧바로 실행
- `a` → `/merge auto` 를 실행
- `N` 또는 응답 없음 → 종료. 나중에 `/merge` 를 수동 호출 가능

---

## Worktree 현황 확인

```bash
git worktree list
```

---

## 주의사항

- PR 생성에는 `gh` CLI 로그인 필요 (`gh auth status`)
- 팀 프로젝트에서 리뷰가 필요하면 `N` 선택 후 리뷰 완료 시 `/merge` 호출 권장
- 머지·태그·worktree 정리는 `/merge` 가 전담합니다 — 이 커맨드는 PR 생성만 담당
- **base branch override 금지** — 위 "PR base branch 규칙" 에 따라 자동 결정된 값만 사용. 사용자가 특정 base 를 요구해도 규칙을 우선 적용하고, 정말 다른 base 가 필요하면 사용자에게 "규칙 위반이지만 진행할까요?" 를 명시적으로 확인받은 뒤에만 수동 지정
