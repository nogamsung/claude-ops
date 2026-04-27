#!/bin/bash
# .claude/hooks/pre-push.sh
# Claude Code Pre-push 커버리지 게이트
#
# git push 감지 시 테스트 실행 → 라인 커버리지 90% 미만이면 푸시 차단.
#
# 동작 모드:
#   모노레포:  .claude/stacks.json 이 존재하면 각 stack.path 에서 순차 검증.
#              한 스택이라도 실패하면 전체 차단.
#   단일 스택: stacks.json 이 없으면 루트 디렉토리에서 자동 감지.
#
# 지원 스택 (각 스택의 실행 디렉토리 기준):
#   kotlin / kotlin-multi → Jacoco (build/reports/jacoco/test/jacocoTestReport.xml)
#   go / go-multi         → go test -coverprofile=coverage.out ./...
#   python / python-multi → uv run pytest --cov (coverage.xml)
#   nextjs / nextjs-multi → Jest --coverage (coverage/coverage-summary.json)
#   flutter               → flutter test --coverage (coverage/lcov.info)

INPUT=$(cat 2>/dev/null || echo '{}')

CMD=$(echo "$INPUT" | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    print(d.get('tool_input', {}).get('command', ''))
except:
    print('')
" 2>/dev/null || echo '')

# git push 가 아니면 즉시 통과
if [[ "$CMD" != *"git push"* ]]; then
  exit 0
fi

THRESHOLD=90

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo " [Pre-push] 커버리지 게이트 (기준: ${THRESHOLD}%)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# ─────────────────────────────────────────────────────────────
# 단일 스택 커버리지 검증 함수
#   $1: 스택 경로 (예: . , backend, web, app)
#   $2: 스택 타입 (kotlin | kotlin-multi | go | go-multi | nextjs | nextjs-multi | flutter | auto)
#   반환: 0 = 통과, 1 = 실패
# ─────────────────────────────────────────────────────────────
check_stack_coverage() {
  local SPATH="$1"
  local STYPE="$2"
  local COVERAGE="0"
  local STACK_LABEL=""
  local RET=0

  pushd "$SPATH" >/dev/null 2>&1 || { echo "[Pre-push] ⚠️  경로 없음: $SPATH"; return 1; }

  # auto 모드 — 파일 마커로 타입 추론
  if [ "$STYPE" = "auto" ]; then
    if [ -f "./gradlew" ]; then STYPE="kotlin"
    elif [ -f "go.mod" ]; then STYPE="go"
    elif [ -f "pyproject.toml" ] && command -v uv &>/dev/null && grep -q "fastapi" pyproject.toml 2>/dev/null; then STYPE="python"
    elif [ -f "package.json" ] && node -e "require('./package.json').dependencies?.next || require('./package.json').devDependencies?.next || process.exit(1)" 2>/dev/null; then STYPE="nextjs"
    elif [ -f "pubspec.yaml" ] && command -v flutter &>/dev/null; then STYPE="flutter"
    else
      echo "[Pre-push] ℹ️  [$SPATH] 지원 스택 감지 실패 — 건너뜀"
      popd >/dev/null; return 0
    fi
  fi

  case "$STYPE" in
    kotlin|kotlin-multi)
      STACK_LABEL="Kotlin Spring Boot"
      echo "[Pre-push] [$SPATH] 스택: $STACK_LABEL"
      echo "[Pre-push] ./gradlew test jacocoTestReport 실행 중..."
      if ! ./gradlew test jacocoTestReport --daemon -q 2>&1; then
        echo "[Pre-push] ❌ [$SPATH] 테스트 실패"
        popd >/dev/null; return 1
      fi
      local REPORT="build/reports/jacoco/test/jacocoTestReport.xml"
      # kotlin-multi: 루트에 없으면 하위 모듈에서 합산
      if [ ! -f "$REPORT" ] && [ "$STYPE" = "kotlin-multi" ]; then
        COVERAGE=$(python3 - <<'PYEOF'
import xml.etree.ElementTree as ET, glob
missed = covered = 0
for f in glob.glob("**/build/reports/jacoco/test/jacocoTestReport.xml", recursive=True):
    try:
        root = ET.parse(f).getroot()
        for c in root.findall('.//counter[@type="LINE"]'):
            missed  += int(c.get("missed", 0))
            covered += int(c.get("covered", 0))
    except: pass
total = missed + covered
print(f"{covered / total * 100:.1f}" if total > 0 else "0")
PYEOF
)
      elif [ -f "$REPORT" ]; then
        COVERAGE=$(python3 - <<PYEOF
import xml.etree.ElementTree as ET
try:
    root = ET.parse("$REPORT").getroot()
    counters = root.findall('.//counter[@type="LINE"]')
    missed  = sum(int(c.get("missed", 0)) for c in counters)
    covered = sum(int(c.get("covered", 0)) for c in counters)
    total = missed + covered
    print(f"{covered / total * 100:.1f}" if total > 0 else "0")
except Exception:
    print("0")
PYEOF
)
      else
        echo "[Pre-push] ⚠️  [$SPATH] Jacoco 리포트 없음: $REPORT"
        echo "           build.gradle.kts 에 jacoco 플러그인이 설정되어 있는지 확인하세요."
        popd >/dev/null; return 1
      fi
      ;;

    go|go-multi)
      STACK_LABEL="Go Gin"
      echo "[Pre-push] [$SPATH] 스택: $STACK_LABEL"
      echo "[Pre-push] go test -coverprofile=coverage.out ./... 실행 중..."
      if ! go test -coverprofile=coverage.out ./... 2>&1; then
        echo "[Pre-push] ❌ [$SPATH] 테스트 실패"
        popd >/dev/null; return 1
      fi
      if [ ! -f coverage.out ]; then
        echo "[Pre-push] ⚠️  [$SPATH] coverage.out 없음"
        popd >/dev/null; return 1
      fi
      COVERAGE=$(go tool cover -func=coverage.out 2>/dev/null | awk '/^total:/ {gsub("%","",$3); print $3}')
      [ -z "$COVERAGE" ] && COVERAGE="0"
      ;;

    python|python-multi)
      STACK_LABEL="Python FastAPI"
      echo "[Pre-push] [$SPATH] 스택: $STACK_LABEL"
      if ! command -v uv &>/dev/null; then
        echo "[Pre-push] ⚠️  [$SPATH] uv 명령이 없음 — 건너뜀"
        popd >/dev/null; return 0
      fi
      echo "[Pre-push] uv run pytest --cov --cov-report=xml 실행 중..."
      if ! uv run pytest --cov --cov-report=xml --cov-report=term 2>&1; then
        echo "[Pre-push] ❌ [$SPATH] 테스트 실패"
        popd >/dev/null; return 1
      fi
      if [ ! -f coverage.xml ]; then
        echo "[Pre-push] ⚠️  [$SPATH] coverage.xml 없음 — pytest-cov 설치 확인"
        popd >/dev/null; return 1
      fi
      COVERAGE=$(python3 - <<'PYEOF'
import xml.etree.ElementTree as ET
try:
    root = ET.parse("coverage.xml").getroot()
    rate = root.get("line-rate")
    print(f"{float(rate) * 100:.1f}" if rate else "0")
except Exception:
    print("0")
PYEOF
)
      ;;

    nextjs|nextjs-multi)
      STACK_LABEL="Next.js"
      echo "[Pre-push] [$SPATH] 스택: $STACK_LABEL"
      echo "[Pre-push] npx jest --coverage 실행 중..."
      if ! npx jest --coverage --coverageReporters=json-summary --passWithNoTests 2>&1; then
        echo "[Pre-push] ❌ [$SPATH] 테스트 실패"
        popd >/dev/null; return 1
      fi
      if [ ! -f coverage/coverage-summary.json ]; then
        echo "[Pre-push] ⚠️  [$SPATH] 커버리지 리포트 없음"
        popd >/dev/null; return 1
      fi
      COVERAGE=$(node -e "
try {
  const d = JSON.parse(require('fs').readFileSync('coverage/coverage-summary.json','utf8'));
  console.log(d.total.lines.pct);
} catch(e) { console.log('0'); }
" 2>/dev/null || echo '0')
      ;;

    flutter)
      STACK_LABEL="Flutter"
      echo "[Pre-push] [$SPATH] 스택: $STACK_LABEL"
      echo "[Pre-push] flutter test --coverage 실행 중..."
      if ! flutter test --coverage 2>&1; then
        echo "[Pre-push] ❌ [$SPATH] 테스트 실패"
        popd >/dev/null; return 1
      fi
      if [ ! -f coverage/lcov.info ]; then
        echo "[Pre-push] ⚠️  [$SPATH] lcov.info 없음"
        popd >/dev/null; return 1
      fi
      COVERAGE=$(python3 - <<'PYEOF'
lf = lh = 0
try:
    with open("coverage/lcov.info") as f:
        for line in f:
            if line.startswith("LF:"):
                lf += int(line.strip().split(":")[1])
            elif line.startswith("LH:"):
                lh += int(line.strip().split(":")[1])
    print(f"{lh / lf * 100:.1f}" if lf > 0 else "0")
except:
    print("0")
PYEOF
)
      ;;

    *)
      echo "[Pre-push] ℹ️  [$SPATH] 알 수 없는 스택 타입: $STYPE — 건너뜀"
      popd >/dev/null; return 0
      ;;
  esac

  popd >/dev/null

  echo "[Pre-push] [$SPATH] 라인 커버리지: ${COVERAGE}%"

  python3 -c "
import sys
try:
    sys.exit(0 if float('${COVERAGE}') >= ${THRESHOLD} else 1)
except:
    sys.exit(1)
" 2>/dev/null
  RET=$?

  if [ $RET -eq 0 ]; then
    echo "[Pre-push] ✅ [$SPATH] 통과"
  else
    echo "[Pre-push] ❌ [$SPATH] 커버리지 ${COVERAGE}% < ${THRESHOLD}%"
  fi
  return $RET
}

# ─────────────────────────────────────────────────────────────
# 모드 분기
# ─────────────────────────────────────────────────────────────
FAILED=0

if [ -f .claude/stacks.json ] && command -v jq >/dev/null 2>&1; then
  # 모노레포 모드 — 각 스택 순차 검증
  MODE=$(jq -r '.mode // "single"' .claude/stacks.json)
  if [ "$MODE" = "monorepo" ]; then
    echo "[Pre-push] 모노레포 모드 — $(jq '.stacks | length' .claude/stacks.json) 개 스택 검증"
    echo ""
    while IFS= read -r line; do
      SPATH=$(echo "$line" | jq -r '.path')
      STYPE=$(echo "$line" | jq -r '.type')
      echo "─── [$SPATH] ($STYPE) ───"
      if ! check_stack_coverage "$SPATH" "$STYPE"; then
        FAILED=1
      fi
      echo ""
    done < <(jq -c '.stacks[]' .claude/stacks.json)
  else
    # stacks.json 이 있지만 mode != monorepo → 단일 스택 정보로 실행
    SPATH=$(jq -r '.stacks[0].path // "."' .claude/stacks.json)
    STYPE=$(jq -r '.stacks[0].type // "auto"' .claude/stacks.json)
    check_stack_coverage "$SPATH" "$STYPE" || FAILED=1
  fi
else
  # 단일 스택 모드 — 루트에서 자동 감지
  check_stack_coverage "." "auto" || FAILED=1
fi

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
if [ $FAILED -eq 0 ]; then
  echo "[Pre-push] ✅ 모든 스택 통과 — 푸시 진행합니다"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  exit 0
else
  echo "[Pre-push] ❌ 커버리지 미달 — 푸시 차단"
  echo ""
  echo "  테스트를 추가하세요:"
  echo "    /test <파일경로>    # 테스트 자동 생성"
  echo ""
  echo "  추가 후 git push 재시도 시 재검사합니다."
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  exit 1
fi
