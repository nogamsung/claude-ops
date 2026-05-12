#!/bin/bash
# .claude/hooks/usage-counter.sh
# PostToolUse hook — opt-in 사용량 집계 (로컬 .claude/.usage.json)
#
# 활성화 조건: .claude/settings.local.json 의 .telemetry == true
# (디폴트 disabled — 게이팅 통과 못 하면 즉시 exit 0)
#
# 외부 전송 절대 없음. 로컬 파일만 누적. tool_name 만 기록 (input/output 내용 X).
# 사용자 자기 작업 패턴 파악용. /memory 결정 근거.

# --- opt-in 게이팅 ---
if [ ! -f .claude/settings.local.json ] || ! command -v jq &>/dev/null; then
  exit 0
fi
ENABLED=$(jq -r '.telemetry // false' .claude/settings.local.json 2>/dev/null)
[ "$ENABLED" != "true" ] && exit 0

# --- input 파싱 ---
INPUT=$(cat 2>/dev/null || echo '{}')
TOOL=$(echo "$INPUT" | jq -r '.tool_name // empty' 2>/dev/null)
[ -z "$TOOL" ] && exit 0

# --- .usage.json 누적 ---
USAGE=.claude/.usage.json
TODAY=$(date +%Y-%m-%d)

if [ ! -f "$USAGE" ]; then
  echo "{\"since\":\"$TODAY\",\"tools\":{}}" > "$USAGE"
fi

# atomic 갱신: tmp → mv
TMP=$(mktemp)
jq --arg t "$TOOL" '.tools[$t] = ((.tools[$t] // 0) + 1)' "$USAGE" > "$TMP" 2>/dev/null && mv "$TMP" "$USAGE"

exit 0
