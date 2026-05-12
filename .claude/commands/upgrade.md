---
description: 내 프로젝트 .claude/ 와 최신 스타터 main 사이의 selective diff. 카테고리별로 골라 적용.
argument-hint: [diff (생략 시 동일) | apply <category[,category...]> | apply all] [--version <ref>]
---

내 프로젝트의 `.claude/` 와 최신 스타터(또는 핀 버전)를 **카테고리 단위로 비교·적용**합니다.

`bootstrap.sh` 는 all-or-nothing 교체이지만, `/upgrade` 는 `agents` 만 / `skills` 만 같이 부분 갱신할 수 있습니다.

**명령:** $ARGUMENTS

---

## 서브명령

| 인자 | 동작 |
|------|------|
| (없음) 또는 `diff` | 카테고리별 변경 통계 + 적용 가이드 |
| `apply <cat>[,<cat>...]` | 지정 카테고리만 갱신 (예: `apply skills,hooks`) |
| `apply all` | bootstrap update 와 동일 (custom/ 보존) |
| `--version <ref>` | 비교 기준을 main 대신 특정 태그/SHA 로 핀 (예: `--version v1.18.0`) |

**카테고리** (6개): `agents`, `commands`, `skills`, `templates`, `hooks`, `settings`

> `agents/custom/`, `commands/custom/`, `hooks/custom/`, `skills/custom/`, `settings.local.json` 은 **항상** 보존 — bootstrap.sh 와 동일 정책.

---

## Step 1 — 비교 대상 다운로드

```bash
TMP=$(mktemp -d)
git clone --quiet --depth=1 https://github.com/nogamsung/claude-code-starter.git "$TMP" 2>/dev/null
if [ -n "$VERSION_REF" ]; then
  (cd "$TMP" && git fetch --quiet --depth=1 origin "$VERSION_REF" && git checkout --quiet FETCH_HEAD) || {
    echo "❌ 버전 ref '$VERSION_REF' 를 origin 에서 찾지 못했습니다."
    exit 1
  }
fi
REMOTE_VERSION=$(cat "$TMP/VERSION")
LOCAL_VERSION=$(cat .claude/.starter-version 2>/dev/null || echo "(unknown)")
echo "내 프로젝트: $LOCAL_VERSION → 비교: $REMOTE_VERSION"
```

---

## Step 2 — 카테고리별 diff 통계

각 카테고리에 대해 `diff -rq <local> <remote>` 결과를 added(+), modified(~), removed(-) 로 분류.

```bash
categorize() {
  local cat="$1"; local local_dir="$2"; local remote_dir="$3"
  local added=0 modified=0 removed=0
  if [ -d "$remote_dir" ] && [ -d "$local_dir" ]; then
    while IFS= read -r line; do
      case "$line" in
        "Only in $remote_dir"*)  added=$((added+1)) ;;
        "Only in $local_dir"*)
          # custom/ 경로는 카운트 제외
          [[ "$line" == *"/custom"* ]] && continue
          removed=$((removed+1)) ;;
        "Files "*differ)         modified=$((modified+1)) ;;
      esac
    done < <(diff -rq "$local_dir" "$remote_dir" 2>/dev/null)
  elif [ -d "$remote_dir" ]; then
    added=$(find "$remote_dir" -type f | wc -l | tr -d ' ')
  fi
  printf "  %-10s : +%-3d ~%-3d -%-3d\n" "$cat" "$added" "$modified" "$removed"
}

echo "변경 요약 (스타터 $REMOTE_VERSION → 내 프로젝트 $LOCAL_VERSION)"
echo ""
categorize "agents"    .claude/agents    "$TMP/.claude/agents"
categorize "commands"  .claude/commands  "$TMP/.claude/commands"
categorize "skills"    .claude/skills    "$TMP/.claude/skills"
categorize "templates" .claude/templates "$TMP/.claude/templates"
categorize "hooks"     .claude/hooks     "$TMP/.claude/hooks"
# settings 는 파일 단일 (settings.json) — 별도
[ -f "$TMP/.claude/settings.json" ] && [ -f .claude/settings.json ] \
  && cmp -s .claude/settings.json "$TMP/.claude/settings.json" \
  && echo "  settings   : (변경 없음)" \
  || echo "  settings   : ~1   (settings.json 갱신 가능)"
```

> **+** 추가 (스타터에 새로 생김), **~** 변경 (양쪽 다른 내용), **-** 제거 (사용자만 보유 — 그대로 둡니다)

---

## Step 3 — 변경 파일 리스트 (각 카테고리 toggle)

사용자에게 자세히 보고 싶은 카테고리를 묻고, 해당 카테고리 안의 파일별 변경 내역을 나열.

```bash
list_category() {
  local cat="$1"; local local_dir="$2"; local remote_dir="$3"
  echo "=== $cat ==="
  diff -rq "$local_dir" "$remote_dir" 2>/dev/null | while IFS= read -r line; do
    case "$line" in
      "Only in $remote_dir"*)
        rel=${line#Only in $remote_dir}
        echo "  + ${rel#: }"
        ;;
      "Files "*differ)
        f=${line#Files }; f=${f% and *}
        echo "  ~ ${f#$local_dir/}"
        ;;
      "Only in $local_dir"*)
        [[ "$line" == *"/custom"* ]] && continue
        rel=${line#Only in $local_dir}
        echo "  - ${rel#: }   (보존됨 — custom/ 으로 이동 권장)"
        ;;
    esac
  done
}
```

---

## Step 4 — 적용

### `apply <category[,...]>`

선택된 카테고리만 `cp -r` 로 교체. **custom/ 디렉토리와 settings.local.json 은 자동 보존**.

```bash
apply_category() {
  local cat="$1"
  case "$cat" in
    agents|commands|skills|hooks)
      # custom/ 백업 → 카테고리 디렉토리 통째 교체 → custom/ 복원
      local CUSTOM_BAK=$(mktemp -d)
      [ -d ".claude/$cat/custom" ] && cp -r ".claude/$cat/custom" "$CUSTOM_BAK/"
      rm -rf ".claude/$cat"
      cp -r "$TMP/.claude/$cat" ".claude/$cat"
      [ -d "$CUSTOM_BAK/custom" ] && cp -r "$CUSTOM_BAK/custom" ".claude/$cat/"
      rm -rf "$CUSTOM_BAK"
      echo "✅ $cat 갱신 완료"
      ;;
    templates)
      rm -rf .claude/templates
      cp -r "$TMP/.claude/templates" .claude/templates
      echo "✅ templates 갱신 완료 (사용자 CLAUDE.md/settings.json 은 영향 없음)"
      ;;
    settings)
      cp "$TMP/.claude/settings.json" .claude/settings.json
      echo "✅ settings.json 갱신 (settings.local.json 은 보존)"
      ;;
    *)
      echo "❌ 알 수 없는 카테고리: $cat"
      return 1
      ;;
  esac
}
```

### `apply all`

`bootstrap.sh` 를 직접 호출 — 동일한 보존 정책.

```bash
[ -n "$VERSION_REF" ] && OPTS="--version $VERSION_REF" || OPTS=""
curl -fsSL https://raw.githubusercontent.com/nogamsung/claude-code-starter/main/bootstrap.sh | bash -s -- $OPTS
```

---

## Step 5 — 후속 안내

```bash
# .starter-version 갱신 (apply 한 카테고리 단위 부분 갱신이라도 비교 기준은 맞춰둠)
echo "$REMOTE_VERSION" > .claude/.starter-version

rm -rf "$TMP"

echo ""
echo "✅ 적용 완료. Claude Code 를 재시작하여 새 정의를 로드하세요."
```

---

## 사용 예시

```bash
# 변경 통계만 보기 (디폴트 dry-run 효과)
/upgrade

# skills 와 hooks 만 갱신
/upgrade apply skills,hooks

# v1.18.0 기준으로 비교 (롤백 시점 확인)
/upgrade --version v1.18.0

# 전부 갱신 (= bootstrap update)
/upgrade apply all
```

---

## 주의사항

- **카테고리 단위 부분 갱신 후** 의존성 불일치 가능성: 예) `commands/` 만 갱신했는데 새 command 가 미설치 agent 를 호출. 이런 경우 `/upgrade apply agents,commands` 같이 함께 적용.
- 사용자가 직접 수정한 stock 파일(`agents/code-reviewer.md` 등) 은 갱신 시 덮어써집니다 — 보존하려면 `agents/custom/code-reviewer-mine.md` 처럼 `custom/` 하위에 두세요.
- `templates/` 갱신은 **루트 CLAUDE.md / settings.json 에 영향 없음** (그건 `/init` 이 한 번만 복사). 다음 `/init` 에서 새 템플릿이 반영됩니다.
- `settings` 카테고리는 `.claude/settings.json` 만 — `settings.local.json` 은 항상 보존.
