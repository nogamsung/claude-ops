#!/bin/bash
# .claude/hooks/safety-guard.sh
# PreToolUse(Bash) hook — 보호 브랜치에서 위험한 Git 명령 차단
#
# 차단 대상:
#   - main/master 브랜치에서 git push --force*, git reset --hard, git commit --amend
#   - rm -rf 루트/홈 디렉토리
#   - DROP TABLE, TRUNCATE (모든 상황)
#
# exit 2 로 차단 메시지 출력 (Claude Code hook spec).

INPUT=$(cat 2>/dev/null || echo '{}')

CMD=$(echo "$INPUT" | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    print(d.get('tool_input', {}).get('command', ''))
except Exception:
    print('')
" 2>/dev/null || echo '')

[ -z "$CMD" ] && exit 0

# 무조건 차단
case "$CMD" in
  *"rm -rf /"*|*"rm -rf ~"*)
    echo "[SafetyGuard] 차단: rm -rf 루트/홈 디렉토리" >&2
    exit 2
    ;;
  *"DROP TABLE"*|*"TRUNCATE "*|*"DROP DATABASE"*)
    echo "[SafetyGuard] 차단: 파괴적 SQL (DROP TABLE/TRUNCATE/DROP DATABASE)" >&2
    exit 2
    ;;
esac

# 보호 브랜치 감지
if git rev-parse --git-dir &>/dev/null; then
  BRANCH=$(git branch --show-current 2>/dev/null || echo "")
  case "$BRANCH" in
    main|master|production|release/*)
      case "$CMD" in
        *"git push --force"*|*"git push -f "*|*"git push -f"*)
          echo "[SafetyGuard] 차단: 보호 브랜치($BRANCH)에서 force push" >&2
          exit 2
          ;;
        *"git reset --hard"*)
          echo "[SafetyGuard] 차단: 보호 브랜치($BRANCH)에서 reset --hard — backup 브랜치 먼저 만들어주세요" >&2
          exit 2
          ;;
        *"git commit --amend"*)
          echo "[SafetyGuard] 차단: 보호 브랜치($BRANCH)에서 --amend — 새 커밋으로 대체하세요" >&2
          exit 2
          ;;
        *"git branch -D"*)
          echo "[SafetyGuard] 차단: 보호 브랜치($BRANCH)에서 branch -D" >&2
          exit 2
          ;;
      esac
      ;;
  esac
fi

exit 0
