#!/bin/bash
# .claude/hooks/session-start.sh
# SessionStart hook — 간결한 프로젝트 컨텍스트 주입
#
# CLAUDE.md 와 별개로 현재 git 상태·변경 파일·활성 스택만 요약.
# 매 세션 로드되므로 출력은 최대 ~30줄로 제한.

set -e

echo "[Context] $(date +%Y-%m-%d)"

# 1) 스택 정보 (stacks.json 있으면 요약, 없으면 스택 힌트만)
if [ -f .claude/stacks.json ] && command -v jq &>/dev/null; then
  MODE=$(jq -r '.mode // "single"' .claude/stacks.json 2>/dev/null)
  STACKS=$(jq -r '[.stacks[]? | "\(.role)=\(.type)@\(.path)"] | join(", ")' .claude/stacks.json 2>/dev/null)
  [ -n "$STACKS" ] && echo "[Stacks] mode=$MODE · $STACKS"
fi

# 2) Git 상태 (간결)
if git rev-parse --git-dir &>/dev/null; then
  BRANCH=$(git branch --show-current 2>/dev/null || echo "detached")
  CHANGED=$(git status --porcelain 2>/dev/null | wc -l | tr -d ' ')
  AHEAD=$(git rev-list --count @{u}..HEAD 2>/dev/null || echo "?")
  echo "[Git] branch=$BRANCH · changed=$CHANGED · ahead=$AHEAD"

  # 3) 최근 커밋 3개 제목만
  LAST_COMMITS=$(git log --oneline -3 2>/dev/null | head -3)
  if [ -n "$LAST_COMMITS" ]; then
    echo "[Recent]"
    echo "$LAST_COMMITS" | sed 's/^/  /'
  fi
fi

# 4) MEMORY.md 존재하면 알림 (내용은 CLAUDE.md 가 자동 로드하니 여기선 힌트만)
[ -f memory/MEMORY.md ] && echo "[Memory] memory/MEMORY.md 로드 가능"

exit 0
