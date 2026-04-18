---
description: 기획자 agent 호출 — 요청을 PRD로 구조화하고 활성 스택별 구현 프롬프트를 분배. 모노레포면 역할별로 자동 분리. Agent Teams 로 병렬 실행, GTM 문서 자동 생성도 옵션.
argument-hint: <기능 설명> [--teams | --output-only] [--marketing | --sales | --gtm]
---

사용자 요청을 받아 PRD + 역할별 구현 프롬프트를 생성하고, 선택적으로 구현 agent 들을 병렬 실행합니다.

**요청:** $ARGUMENTS

---

## Step 1 — 플래그 파싱

`$ARGUMENTS` 에서 다음 플래그를 분리:

**실행 모드** (상호 배타):
| 플래그 | 동작 |
|--------|------|
| `--teams` | Step 5 인터랙션 생략, 바로 Agent Teams 실행 |
| `--output-only` | Step 5 인터랙션 생략, 파일만 생성 후 종료 |
| (없음) | Step 5 에서 사용자에게 실행 모드 질문 |

**GTM 모드** (실행 모드와 조합 가능):
| 플래그 | 동작 |
|--------|------|
| `--marketing` | PRD + 마케팅 전략 (`docs/specs/{feature}/marketing.md` + `docs/gtm/` 스냅샷) |
| `--sales` | PRD + 세일즈 전략 (`docs/specs/{feature}/sales.md` + `docs/gtm/` 스냅샷) |
| `--gtm` | PRD + 마케팅 + 세일즈 (둘 다) |

`--marketing` / `--sales` / `--gtm` 중 하나라도 있으면 Step 4.5 (gtm-planner 호출) 활성.
`--gtm` 은 `--marketing --sales` 의 축약.

나머지 인자 = `raw_request`.

## Step 2 — feature 이름 결정

`raw_request` 에서 kebab-case 이름을 추출 (예: "로그인 기능" → `login`, "결제 취소 기능" → `payment-cancel`).

애매하면 사용자에게 물어봅니다:
> "이 기능의 이름을 kebab-case 로 알려주세요 (예: login, payment-cancel):"

`docs/specs/{feature}.md` 가 이미 있으면:
> "기존 PRD 가 있습니다. (a) 덮어쓰기 / (b) 갱신 (diff 제안) / (c) 중단"

## Step 3 — 소크라테스식 인터뷰 (요청이 모호한 경우)

**최대 3개만** 물어봅니다. 이미 `raw_request` 에 담겨 있으면 건너뜁니다.

- **핵심 시나리오**: 주요 사용자 플로우는? 성공 기준은?
- **스코프**: 이 기능의 범위 내 vs 후속 이터레이션?
- **비기능**: 성능·보안·접근성·i18n 요구가 있나요?
- (모노레포만) **스택 범위**: 이 기능은 어느 스택에 걸쳐 있나요? (backend only / full-stack / mobile 포함)

답변을 `interview_answers` 에 수집.

## Step 4 — `planner` agent 호출

`Agent(subagent_type="planner", prompt=...)` 호출. prompt 에 다음을 전달:

- `raw_request`
- `feature_name`
- `interview_answers` (있으면)
- `mode`: "monorepo" or "single" (`.claude/stacks.json` 유무로 판정)
- `stacks[]`: 활성 스택 목록 (`.claude/stacks.json` 내용 또는 단일 감지 결과)

agent 는 Step 7 리포트를 반환합니다. 반환 내용을 사용자에게 그대로 보여줍니다.

## Step 4.5 — GTM 문서 생성 (플래그 있을 때만)

`--marketing` / `--sales` / `--gtm` 중 하나 이상이면 여기서 `gtm-planner` agent 호출.

**플래그 → flags[] 매핑:**
- `--marketing` → `["marketing"]`
- `--sales` → `["sales"]`
- `--gtm` → `["marketing", "sales"]`
- 조합 (`--marketing --sales`) → `["marketing", "sales"]`

```
Agent(
  subagent_type="gtm-planner",
  prompt="feature_name=<kebab>, feature_display=<한글 또는 원문>,
          flags=[<위 매핑 결과>],
          prd_path=docs/specs/<feature>.md,
          spec_dir=docs/specs/<feature>/,
          raw_request=<원문>"
)
```

**의존성 체크**:
- PRD (`docs/specs/{feature}.md`) 가 Step 4 에서 정상 생성되지 않았으면 건너뜀
- `marketing-skills` 플러그인 없으면 gtm-planner 가 fallback 모드로 진행 (경고만)

gtm-planner 의 리포트를 Step 4 리포트 아래에 덧붙여 사용자에게 표시.

## Step 5 — 실행 모드 선택

**플래그로 확정됐으면 건너뜀.**

사용자에게 질문:
> ```
> 다음 중 선택하세요:
>
> (a) Agent Teams — 활성 역할(backend/frontend/mobile) 에 agent 를 병렬 할당해 즉시 구현 시작
>     ⚠️ 3개 agent 가 동시에 파일을 생성합니다. 현재 worktree 에서 실행되며,
>        역할별 경로 분리로 충돌은 최소화됩니다.
>
> (b) 프롬프트 출력만 — 파일만 생성하고 종료. 수동으로 /new ... 실행
>
> 선택: (a / b)
> ```

**안전장치**: (a) 선택 시 한 번 더 확인:
> "backend (kotlin-multi) + frontend (nextjs) + mobile (flutter) 3개 agent 를 병렬 실행합니다. 진행할까요? (yes / no)"

## Step 6 — 실행

### 6-a. Teams 모드

`.claude/stacks.json` 의 각 스택에 대해 **단일 메시지** 에서 Agent tool 병렬 호출:

```
Agent(subagent_type="kotlin-generator",  prompt=<docs/specs/{feature}/backend.md 내용>)
Agent(subagent_type="nextjs-generator",  prompt=<docs/specs/{feature}/frontend.md 내용>)
Agent(subagent_type="flutter-generator", prompt=<docs/specs/{feature}/mobile.md 내용>)
```

**스택 → agent 매핑 표:**

| stacks.json type | 호출할 agent |
|------------------|-------------|
| `kotlin` / `kotlin-multi` | `kotlin-generator` |
| `go` / `go-multi` | `go-generator` |
| `nextjs` / `nextjs-multi` | `nextjs-generator` |
| `flutter` | `flutter-generator` |

각 agent prompt 머리에 다음 지시 추가:

> 이 작업은 `/planner` 가 생성한 **역할 프롬프트**에 따라 진행합니다.
> 프롬프트 본문을 Step 별로 **순서대로** 수행하세요.
> 다른 역할 (frontend/mobile 등) 의 디렉토리는 건드리지 마세요.
> 완료 후 생성한 파일 목록을 짧게 리포트하세요.

병렬 실행 완료되면 각 agent 의 리포트를 합쳐 사용자에게 요약:

```
✅ Agent Teams 완료

backend (kotlin-generator):
  - 생성: 3개 / 수정: 2개
  - ...
frontend (nextjs-generator):
  - 생성: 4개
mobile (flutter-generator):
  - 생성: 5개

다음:
  /review                — 3개 역할 일괄 리뷰
  git status             — 변경 확인
```

### 6-b. Output-only 모드

파일 목록과 다음 단계 안내:

```
✅ 기획 완료 — 파일만 생성됐습니다

PRD:  docs/specs/{feature}.md
역할 프롬프트:
  docs/specs/{feature}/backend.md    → /new backend  api <Resource>  실행 시 참고
  docs/specs/{feature}/frontend.md   → /new frontend component <Name> 실행 시 참고
  docs/specs/{feature}/mobile.md     → /new mobile   screen <Name>    실행 시 참고

다음:
  /new backend  api <Resource>
  /new frontend component <Name>
  /new mobile   screen <Name>
  (또는 /planner {feature} --teams 로 병렬 실행)
```

---

## 단일 스택 모드 동작

`.claude/stacks.json` 이 없는 경우:

- Step 4 에서 mode="single" 로 호출
- 산출물: `docs/specs/{feature}.md` + `docs/specs/{feature}-prompt.md` (단일 프롬프트 파일)
- Step 5 에서 Teams 대신 단일 실행 모드 선택:
  - (a) Agent 실행 — 루트 스택의 generator 호출
  - (b) 프롬프트 출력만

---

## 사용 예시

```
# 모노레포 풀스택 기능
/planner 로그인 기능
  → 인터뷰 3개 질문
  → PRD + backend.md + frontend.md + mobile.md 생성
  → 실행 모드 질문

# 즉시 팀 실행 (모든 질문/확인 스킵 금지 — 안전장치는 유지)
/planner 결제 취소 기능 --teams

# 파일만 생성하고 수동 실행 예정
/planner 댓글 기능 --output-only

# 단일 스택 레포
/planner 비밀번호 재설정
  → PRD + 단일 프롬프트 생성

# GTM 포함 (마케팅 + 세일즈 전략까지)
/planner 유료 플랜 런치 --gtm
  → PRD + 역할 프롬프트
  → gtm-planner 호출 → docs/specs/{feature}/marketing.md + sales.md
  → docs/gtm/{YYYY-MM-DD}-{feature}/ 스냅샷 + history.md 갱신

# 마케팅만
/planner 블로그 발행 기능 --marketing

# 세일즈만 + 즉시 팀 실행
/planner 엔터프라이즈 SSO --sales --teams
```

---

## 주의사항

- **planner agent 는 코드를 쓰지 않습니다.** PRD 와 프롬프트만 산출. 구현은 별도 generator agent 가 담당.
- **gtm-planner agent 도 코드를 쓰지 않습니다.** 마케팅·세일즈 문서만 산출. 기술 구현과 완전히 분리된 GTM 문서 전담.
- **기존 PRD 덮어쓰기 확인 필수** — `docs/specs/{feature}.md` 가 있으면 사용자에게 물어봐야 합니다.
- **Teams 모드의 충돌 회피** — 역할별 경로가 분리(`backend/`, `web/`, `app/`) 되어야 안전합니다. 단일 스택에서 Teams 는 의미가 없으므로 자동으로 단일 실행으로 폴백합니다.
- **오픈 이슈는 차단 신호가 아닙니다** — PRD 에 `[ ]` 로 남아도 Teams 실행 가능. 단, 오픈 이슈가 핵심 계약(인증·권한 등) 관련이면 진행 전 해결 권장.
- **`docs/specs/` + `docs/gtm/` 는 팀 공유 대상** — `.gitignore` 하지 않습니다.
- **GTM 스냅샷은 날짜 기반** — 같은 기능을 재기획하면 새 날짜 디렉토리가 추가됩니다 (덮어쓰지 않음).
- **`/merge` 연동** — `docs/gtm/*-{feature}/` 스냅샷은 `/merge` 실행 시 `meta.yaml` 에 `released_version` 이 자동 기록됩니다. GTM 없는 기능은 스킵.
