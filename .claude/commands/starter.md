---
description: Claude Code Starter 하네스를 설치·업데이트·롤백. update 시 사용자 custom 자산은 기본 보존.
argument-hint: [check | update [--version v1.x.x] [--no-preserve] | rollback] (생략 시 check)
---

Claude Code Starter 자체를 이 프로젝트에 적용·갱신·되돌립니다.

**명령:** $ARGUMENTS

---

## 서브명령

| 인자 | 동작 |
|------|------|
| (없음) 또는 `check` | 현재 설치된 스타터 버전 + 원격 최신 + 이전 (rollback 후보) 표시 |
| `update` | 최신으로 업데이트 (custom 자산 기본 보존) |
| `update --version v1.x.x` | 특정 태그로 핀 업데이트 |
| `update --no-preserve` | custom 자산까지 모두 갈아엎고 전체 교체 |
| `rollback` | `.claude/.starter-version-prev` 에 기록된 직전 버전으로 되돌림 |

---

## `check` — 상태 표시

```bash
CURRENT="(없음)"
[ -f .claude/.starter-version ] && CURRENT=$(cat .claude/.starter-version)

PREV="(없음)"
[ -f .claude/.starter-version-prev ] && PREV=$(cat .claude/.starter-version-prev)

LATEST=$(curl -fsSL https://raw.githubusercontent.com/nogamsung/claude-code-starter/main/VERSION)

echo "현재: $CURRENT"
echo "최신: $LATEST"
echo "이전: $PREV   (rollback 가능 여부: $([ "$PREV" = "(없음)" ] && echo no || echo yes))"
```

- `CURRENT == LATEST` → "이미 최신입니다" 안내
- 다르면 → "`/starter update` 또는 `/starter update --version v$LATEST`" 안내

---

## `update` — 재설치

### Step 1 — 옵션 파싱

`$ARGUMENTS` 에서 `--version <tag>` 와 `--no-preserve` 를 추출합니다.

| 플래그 | 동작 |
|--------|------|
| `--version v1.x.x` | bootstrap.sh 에 `--version v1.x.x` 전달 |
| `--no-preserve` | custom 자산 보존 끄기 (전체 교체) |
| (없음) | 최신 + custom 자산 보존 (기본) |

### Step 2 — 사용자 확인

**기본 (보존 모드):**
```
ℹ️  업데이트가 진행됩니다.
    보존: agents/custom · commands/custom · hooks/custom · skills/custom · settings.local.json
    교체: 그 외 .claude/ 모든 파일
    영향 없음: memory/

진행하시겠습니까? (y/N)
```

**`--no-preserve` 모드:**
```
⚠️  --no-preserve: .claude/ 가 백업 없이 전체 교체됩니다.
    수정한 agent · 추가한 command · hooks · settings.local.json 모두 사라집니다.
    memory/ 는 건드리지 않습니다.

정말 진행하시겠습니까? (y/N)
```

`y` 외 입력 시 중단.

### Step 3 — bootstrap.sh 실행

```bash
# 옵션 조립
OPTS=""
[ -n "$VERSION" ] && OPTS="$OPTS --version $VERSION"
[ "$NO_PRESERVE" = "true" ] && OPTS="$OPTS --no-preserve"

curl -fsSL https://raw.githubusercontent.com/nogamsung/claude-code-starter/main/bootstrap.sh | bash -s -- $OPTS
```

### Step 4 — 후속 안내

```
✅ 업데이트 완료 (vX.Y.Z)
   이전 버전: vA.B.C  →  /starter rollback 으로 되돌리기 가능

다음 단계:
  1. Claude Code 재시작 (새 커맨드 로드)
  2. 스택 설정이 바뀌었다면 /init 재실행
```

---

## `rollback` — 직전 버전으로 되돌리기

### Step 1 — 이전 버전 읽기

```bash
if [ ! -f .claude/.starter-version-prev ]; then
  echo "❌ rollback 불가 — .starter-version-prev 가 없습니다 (update 한 적 없음)"
  exit 1
fi
PREV=$(cat .claude/.starter-version-prev)
CURRENT=$(cat .claude/.starter-version)
echo "현재: $CURRENT  →  롤백 대상: $PREV"
```

### Step 2 — 사용자 확인

```
⚠️  $CURRENT → $PREV 으로 되돌립니다.
    custom 자산은 그대로 보존됩니다 (--preserve 기본).

진행하시겠습니까? (y/N)
```

### Step 3 — bootstrap.sh 호출

```bash
curl -fsSL https://raw.githubusercontent.com/nogamsung/claude-code-starter/main/bootstrap.sh | bash -s -- --version "v$PREV"
```

bootstrap.sh 가 자동으로 현재 → 이전 으로 `.starter-version-prev` 를 갱신하므로, 다시 `rollback` 하면 원래 버전으로 돌아갑니다 (toggle 가능).

---

## 보존 정책 — 사용자 자산 디렉토리 규약

스타터를 update 해도 **다음 위치는 보존**됩니다 (기본):

```
.claude/
├── agents/custom/      ← 사용자 추가 agent
├── commands/custom/    ← 사용자 추가 command
├── hooks/custom/       ← 사용자 추가 hook
├── skills/custom/      ← 사용자 추가 skill
└── settings.local.json ← 개인 설정
```

> **권장:** 사용자가 만든 agent/command 는 **반드시** `custom/` 하위에 두세요. 루트(`agents/`, `commands/` 직속) 에 두면 update 시 사라집니다.

`memory/` 폴더는 `.claude/` 밖이므로 어떤 경우에도 영향 없습니다.

---

## 주의사항

- `.starter-version` 과 `.starter-version-prev` 는 **팀 공유** (gitignore 하지 않음)
- `curl | bash -s --` 로 인자 전달 가능 — 인터랙티브 입력은 여전히 제한
- `--version` 핀은 git 태그 외에 브랜치/커밋도 가능 (예: `--version main`, `--version <sha>`)
