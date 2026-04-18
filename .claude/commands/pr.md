---
description: 현재 브랜치에서 PR 생성. base는 dev/main 자동 감지. 완료 후 /merge 자동 제안.
argument-hint: (인수 없음 — 자동으로 push + PR 생성)
---

현재 worktree의 브랜치에서 PR을 생성합니다.

---

## Step 1 — 사전 확인

```bash
BRANCH=$(git branch --show-current)
BASE_BRANCH=$(git branch --list dev | grep -q dev && echo "dev" || echo "main")
echo "브랜치: $BRANCH → PR base: $BASE_BRANCH"
```

- `dev` 브랜치 있음 → base = `dev`
- `dev` 없음 → base = `main`

이미 PR이 있으면 그 URL만 출력하고 Step 3으로 진행:

```bash
EXISTING=$(gh pr view --json url 2>/dev/null | jq -r '.url // empty')
[ -n "$EXISTING" ] && echo "ℹ️  이미 PR 존재: $EXISTING"
```

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
