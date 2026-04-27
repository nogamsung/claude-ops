---
description: Claude Code 하네스(settings.json, hooks, agents)를 검증·dry-run·수정. 토큰 사용량 추정과 권한 중복 탐지도 포함.
argument-hint: [check | doctor | dry-run <hook> | size | lint-settings]
---

Claude Code 하네스 엔지니어링 전용 커맨드. 인수에 따라 서브 명령 실행.

**인수:** $ARGUMENTS

---

## 서브 명령

### `check` (기본값) — 하네스 전체 건강 체크
인수 없이 `/harness` 호출 시 실행. 아래 항목을 순서대로 검사:

1. **settings.json / settings.local.json 유효성**
   - `jq . .claude/settings.json` 로 JSON 파싱 확인
   - `allow` 리스트에 내장 도구와 중복되는 Bash 항목 경고 (`ls *`, `find *`, `grep *`, `cat *`)
   - `deny` 에 `rm -rf /`, `git push --force*` 포함 여부 확인
   - `enabledPlugins` 에 명백한 중복/비활성 플러그인 경고

2. **훅 스크립트 상태**
   - `.claude/hooks/*.sh` 실행 권한(`-x`) 확인
   - 각 훅이 `.claude/stacks.json` 없을 때 안전하게 `exit 0` 하는지 `grep` 확인
   - `settings.json` 에 참조된 훅 경로가 실제 존재하는지 확인

3. **Agent / Skill / Command description 길이**
   - 각 `agents/*.md`, `commands/*.md` 의 frontmatter `description` 길이 평균·최대 출력
   - 300자 초과 description 경고 (세션마다 로드됨)

4. **CLAUDE.md 크기**
   - 현재 CLAUDE.md 줄 수 → 150줄 초과 시 경고
   - `memory/MEMORY.md` 라인 수 → 200줄 초과 시 경고

5. **stacks.json 일관성** (모노레포)
   - 각 stack.path 가 실제 디렉토리인지
   - type 이 감지 가능한지 (`go.mod`, `pyproject.toml`, `pubspec.yaml`, `package.json`, `build.gradle.kts`)

보고 형식:
```
✓ settings.json valid
⚠ allow 에 중복: ls *, find *, grep *, cat * — 내장 Glob/Grep/Read 로 대체 가능
✓ 훅 모두 실행 가능
...
```

### `doctor` — 자동 수정 제안
`check` 와 동일 검사 + 발견된 문제를 Edit 으로 자동 수정 제안. 각 수정은 사용자 확인 후 적용.

예시 제안:
- `allow` 에서 `ls *` 제거
- `chmod +x .claude/hooks/*.sh`
- 300자 초과 description 을 body 로 이동

### `dry-run <hook>` — 훅 실제 실행 (세션 영향 없음)
지정한 훅 스크립트를 테스트 입력으로 실행해 출력 확인.

사용 가능한 훅:
- `session-start` — SessionStart 훅
- `safety-guard` — PreToolUse Bash 안전 훅 (테스트 명령 필요: `dry-run safety-guard "git push --force"`)
- `post-edit-lint` — PostToolUse 훅 (테스트 파일 필요: `dry-run post-edit-lint app/main.py`)
- `pre-push` — pre-push 커버리지 게이트

실행 방식:
```bash
# safety-guard 예시
echo '{"tool_input":{"command":"git push --force"}}' | bash .claude/hooks/safety-guard.sh
echo "exit=$?"
```

### `size` — 토큰 영향 요약
세션마다 로드되는 항목별 줄 수/문자 수 집계.

| 위치 | 줄 수 | 문자 수 | 세션 로드 |
|------|-------|--------|---------|
| CLAUDE.md | ... | ... | 매번 |
| settings.json 내 allow/deny | ... | ... | 매번 |
| agents/*.md frontmatter | ... | ... | 매번 (Agent schema) |
| commands/*.md frontmatter | ... | ... | 매번 (skill list) |
| skills/*.md | (on-invoke) | - | 호출 시만 |

`wc` 로 집계해 표 형태로 출력.

### `lint-settings` — settings.json 정책 검증
엄격한 정책 체크:
- `allow` 에 와일드카드 단독 (`Bash(*)`, `WebFetch(*)`) 금지
- `deny` 가 `allow` 에 의해 무효화되는 패턴 감지
- 동일 도구가 `allow` 와 `deny` 양쪽에 있는지
- `enabledPlugins` 에 지원 중단된/오타 plugin 이름

---

## 실행 흐름

1. 인수 파싱 — 첫 토큰이 위 서브 명령 중 하나면 해당 실행
2. 인수 없으면 `check` 실행
3. 결과를 **간결한 표 + 경고 bullet** 형태로 출력
4. 치명적 문제(훅 실행 불가, settings 파싱 실패)는 exit code 1 로 표기

---

## 주의사항

- `/harness` 는 **읽기·검증 전용** 을 기본으로. `doctor` 만 수정 제안 포함
- 자동 수정은 반드시 사용자 확인 후 (`Bash` / `Edit` 호출 전 diff 보여주기)
- `stacks.json` 이 없는 빈 프로젝트에서도 동작해야 함 (훅 테스트 환경)
- 플러그인 제거 제안은 신중히 — 사용 이력 모르므로 "검토하세요" 수준으로

## 출력 예시

```
/harness check

▸ settings.json          ✓ valid JSON
▸ enabledPlugins         ✓ 8 plugins (github, feature-dev, ...)
▸ allow 중복 검사        ⚠ find *, grep * 중복 → 내장 Glob/Grep 사용 권장
▸ 훅 실행 권한           ✓ session-start.sh, safety-guard.sh, post-edit-lint.sh, pre-push.sh
▸ 훅 안전 가드           ✓ 모두 stacks.json 없을 때 exit 0
▸ CLAUDE.md              58 lines (목표 ≤150 ✓)
▸ agent descriptions     avg 180c, max 420c ⚠ code-reviewer
▸ stacks.json            없음 (단일 스택 모드)

요약: 1 warning · 0 error
```
