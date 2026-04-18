---
description: Claude Code Starter 하네스를 설치·업데이트. 기존 .claude/ 가 있어도 백업 없이 최신 스타터로 전체 교체
argument-hint: [check | update] (생략 시 check)
---

Claude Code Starter 자체를 이 프로젝트에 적용/갱신합니다.

**명령:** $ARGUMENTS

---

## 서브명령

| 인자 | 동작 |
|------|------|
| (없음) 또는 `check` | 현재 설치된 스타터 버전과 원격 최신 버전 비교, 업데이트 필요 여부 안내 |
| `update` | bootstrap.sh를 실행해 `.claude/` 를 최신 버전으로 **백업 없이 전체 교체**, 이후 `/init` 안내 |

---

## `check` — 버전 비교

### Step 1 — 현재 버전 읽기

```bash
if [ -f .claude/.starter-version ]; then
  CURRENT=$(cat .claude/.starter-version)
else
  CURRENT="(없음 — 스타터가 설치되지 않았거나 구버전)"
fi
echo "현재 버전: $CURRENT"
```

### Step 2 — 원격 최신 버전 조회

```bash
LATEST=$(curl -fsSL https://raw.githubusercontent.com/nogamsung/claude-code-starter/main/VERSION)
echo "최신 버전: $LATEST"
```

### Step 3 — 결과 안내

- `CURRENT == LATEST` → "이미 최신입니다" 출력 후 종료
- 다르면 → "업데이트 가능. `/starter update` 를 실행하세요" 안내

---

## `update` — 재설치

### Step 1 — 경고 표시

사용자에게 다음을 명시적으로 알립니다:

```
⚠️  .claude/ 가 백업 없이 전체 교체됩니다.
    — 수정한 agent / 추가한 command / hooks / settings.local.json 이 모두 사라집니다.
    — memory/ 폴더는 건드리지 않습니다.

계속하려면 y를 입력하세요.
```

사용자가 `y` 이외 입력 시 중단.

### Step 2 — bootstrap.sh 실행

```bash
curl -fsSL https://raw.githubusercontent.com/nogamsung/claude-code-starter/main/bootstrap.sh | bash
```

bootstrap.sh는 기존 `.claude/` 감지 시 자동으로 update 모드로 동작합니다.

### Step 3 — 후속 안내

설치가 완료되면:

```
✅ 업데이트 완료
다음 단계:
  1. Claude Code를 재시작하여 새 커맨드를 로드하세요
  2. 스택 설정이 바뀌었다면 /init 재실행
```

---

## 주의사항

- `.starter-version` 파일은 **팀 공유 대상**입니다 (gitignore 하지 않음) — 팀원 간 스타터 버전 일치 추적
- `memory/` 폴더는 프로젝트별 Second Brain이므로 절대 건드리지 않습니다
- `curl | bash` 방식이므로 인터랙티브 입력이 제한됩니다. 업데이트는 **무조건 전체 교체** 방식으로 동작
