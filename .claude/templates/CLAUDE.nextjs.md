# [프로젝트명] — Next.js

## Stack
- **Framework**: Next.js 14+ (App Router)
- **Language**: TypeScript (strict mode — `"strict": true`)
- **Styling**: Tailwind CSS + shadcn/ui
- **Server State**: TanStack Query v5
- **Client State**: Zustand
- **Forms**: React Hook Form + Zod
- **HTTP**: Axios (custom instance at `lib/api-client.ts`)

## Agents
| 작업 | Agent |
|------|-------|
| 새 파일 생성 | `nextjs-generator` |
| 기존 코드 수정 | `nextjs-modifier` |
| 테스트 작성 | `nextjs-tester` |
| 코드 리뷰 | `code-reviewer` |
| 디자인 시스템 / UI 일관성 | `ui-designer` |

## Commands
| 커맨드 | 용도 |
|--------|------|
| `/planner <기능>` | 기획서(PRD) + 구현 프롬프트 작성 (단일 스택은 단일 프롬프트 산출) |
| `/plan <기능>` | 코드 작성 전 설계 및 확인 |
| `ui-designer` 에이전트에게 요청 | DESIGN.md 설정 · 디자인 토큰 적용 · 디자인 리뷰 |
| `/new <Name> [--page\|--feature\|--ui]` | 컴포넌트 생성 (명시: `/new component`) |
| `/test [파일]` | 테스트 자동 생성 |
| `/review [staged\|diff\|파일]` | 코드 리뷰 |
| `/rule <실수 설명>` | 새 규칙을 이 파일에 추가 |
| `/commit [힌트]` | Conventional Commits 커밋 |
| `/pr` | PR 생성 + /merge 자동 제안 |
| `/merge [auto]` | GitHub 머지 실행 + 태그 + worktree 정리 |
| `/memory [add\|search]` | Second Brain 조회·추가·검색 |

## 플러그인 커맨드 (설치된 플러그인)
| 커맨드 | 플러그인 | 설명 |
|--------|---------|------|
| `/feature-dev <기능>` | feature-dev | 7단계 체계적 기능 개발 (탐색→설계→구현→리뷰) |
| `/code-review` | code-review | 현재 PR에 병렬 4-agent 자동 리뷰 + CLAUDE.md 준수 검사 |
| `/commit-push-pr` | commit-commands | 커밋→푸시→PR 생성 한 번에 |
| `/clean_gone` | commit-commands | 삭제된 원격 브랜치의 로컬 정리 |
| `/hookify [설명]` | hookify | 반복 실수를 자동 방지 훅으로 등록 |
| (자동) | typescript-lsp | 타입 오류·심볼 참조·리팩토링 실시간 지원 |
| (자동) | frontend-design | UI 컴포넌트 디자인 패턴 및 접근성 가이드 |
| (자동) | playwright | E2E 브라우저 테스트 자동화 |
| (자동) | context7 | Next.js·React 최신 공식 문서를 컨텍스트로 주입 |
| (자동) | security-guidance | 위험 명령어 실행 전 보안 경고 |
| (자동) | claude-md-management | CLAUDE.md 규칙 자동 관리 |

---

## Git 브랜치 전략 & 병렬 작업 (Worktree)

| 브랜치 | 역할 | 보호 |
|--------|------|------|
| `main` | 프로덕션 릴리스 | PR + CI 통과 필수 |
| `dev` | 통합·스테이징 | PR + CI 통과 필수 |
| `feature/{name}` | 새 기능 | - |
| `fix/{name}` | 버그 수정 | - |
| `hotfix/{name}` | 긴급 수정 | - |
| `refactor/{name}` | 리팩토링 | - |
| `chore/{name}` | 설정·의존성 | - |

### Worktree 병렬 작업 흐름

```bash
# 작업 시작 — worktree로 격리된 작업공간 생성
/new feature-login    # feature/login + .worktrees/feature-login/
/new fix-signup       # fix/signup + .worktrees/fix-signup/
/new refactor-auth    # refactor/auth + .worktrees/refactor-auth/

# 여러 작업 동시 진행 가능
git worktree list
# /project                             [dev]
# /project/.worktrees/feature-login    [feature/login]
# /project/.worktrees/fix-signup       [fix/signup]

# 작업 후 PR 생성 (base: dev)
/pr

# PR merge 후 정리
git worktree remove .worktrees/feature-login
git branch -d feature/login

# dev → main 릴리스 PR
gh pr create --base main --title "release: v1.2.0"
```

### Worktree 디렉토리 규칙
- 위치: `.worktrees/{type}-{name}/` (프로젝트 내부, gitignore 필수)
- `.gitignore`에 `.worktrees/` 반드시 포함
- 각 worktree는 독립된 `node_modules` 보유 (`npm ci` 자동 실행)
- `main` 직접 push 금지 — 반드시 `dev`를 거쳐 PR

---

## 아키텍처 규칙

### 디렉토리 구조
```
src/
├── app/                  # Next.js App Router — 라우팅만
│   ├── (auth)/
│   ├── (dashboard)/
│   └── api/
├── components/
│   ├── ui/               # shadcn/ui 기반 재사용 컴포넌트
│   └── features/         # 피처별 컴포넌트
├── hooks/                # 커스텀 훅
├── lib/                  # API 클라이언트, 유틸리티
├── stores/               # Zustand 스토어
└── types/                # TypeScript 타입 정의
```

### Server vs Client Component 결정 기준
```
Server Component (기본값):
  ✅ 데이터 fetch, async/await 필요
  ✅ 백엔드 직접 접근 필요
  ✅ 민감한 API 키 사용

Client Component ("use client" 추가):
  ✅ useState, useEffect 등 React hooks 사용
  ✅ onClick 등 이벤트 핸들러 사용
  ✅ 브라우저 API 사용 (localStorage 등)
```

---

## 반드시 지켜야 할 규칙 (MUST)

### Export 방식
```tsx
// ✅ Named export 사용
export function UserCard({ user }: UserCardProps) { ... }

// ❌ 절대 금지 — default export (라우팅 page.tsx 제외)
export default function UserCard() { ... }
```

### TypeScript
```tsx
// ✅ 명시적 타입
const getUser = (id: number): Promise<User> => ...

// ❌ 절대 금지 — any 사용
const data: any = await fetch(...)
```

### 데이터 페칭 (Client Component)
```tsx
// ✅ TanStack Query 사용
const { data, isLoading, isError } = useUsers()

// ❌ 절대 금지 — useEffect + fetch 조합
useEffect(() => { fetch('/api/users').then(...) }, [])
```

### 폼 처리
```tsx
// ✅ React Hook Form + Zod
const schema = z.object({ email: z.string().email() })
const form = useForm({ resolver: zodResolver(schema) })

// ❌ 절대 금지 — 수동 state + 자체 validation
const [email, setEmail] = useState('')
```

### 스타일링
```tsx
// ✅ Tailwind 클래스
<div className="flex items-center gap-4 p-4">

// ❌ 절대 금지 — 인라인 스타일
<div style={{ display: 'flex', padding: '16px' }}>
```

---

## 절대 하면 안 되는 것 (NEVER)

- `any` 타입 사용 (`unknown` + type narrowing으로 대체)
- Server Component에서 클라이언트 상태(useState) 사용
- `useEffect` 안에서 직접 데이터 페칭
- 환경 변수(`NEXT_PUBLIC_` 제외)를 클라이언트 컴포넌트에서 참조
- API Route 없이 클라이언트에서 DB 직접 접근
- `console.log`를 프로덕션 코드에 남기기
- 컴포넌트 파일 안에 비즈니스 로직 작성 (hooks로 분리)
- 테스트 없이 새로운 컴포넌트/훅 추가

---

## 성능 기준

- 이미지는 반드시 `next/image` 사용
- 라우트마다 `loading.tsx`, `error.tsx` 필수
- 목록 렌더링: `key`에 index 사용 금지 (고유 id 사용)

---

## 접근성 기준

- 인터랙티브 요소에 `aria-label` 또는 visible text 필수
- 이미지에 `alt` 필수
- `<button>` vs `<a>` 시맨틱 구분 (동작 vs 이동)

---

## 커버리지 게이트

**git push 전 Jest 라인 커버리지 90% 이상 필수** (`.claude/hooks/pre-push.sh` 자동 검사)

커버리지 미달 시 동작:
1. pre-push 훅이 푸시를 차단하고 미달 수치를 출력합니다
2. 커버리지가 낮은 파일에 `/test <파일>` 커맨드로 테스트를 추가합니다
3. `npx jest --coverage`로 커버리지를 재확인합니다
4. 90% 이상이 되면 다시 `git push`를 시도합니다

**jest.config.ts에 coverageThreshold 설정 권장:**
```ts
coverageThreshold: {
  global: {
    lines: 90,
    branches: 80,
    functions: 90,
    statements: 90,
  },
},
coverageReporters: ['json-summary', 'text', 'lcov'],
```

---

## 학습된 규칙 (AI 실수 후 추가)

<!-- /rule 커맨드로 새 규칙이 여기에 추가됩니다 -->

---

## 세션 시작 시 자동 참조

> 🧠 **새 작업을 시작하기 전에 `memory/MEMORY.md` 를 반드시 먼저 읽으세요.** 과거 결정·교훈을 맥락에 포함하여 같은 실수를 반복하지 않도록 합니다.

---

## Memory 관리 지침

> Claude는 아래 상황에서 `memory/MEMORY.md`를 **자동으로** 업데이트합니다.
> 사용자가 요청하지 않아도 기록하고, 기록 후 "memory에 저장했습니다." 한 줄만 언급합니다.

**자동 기록 트리거:**
- `/plan` 승인 → 구현할 기능과 선택한 설계 방식 기록
- `/rule` 실행 → 어떤 실수였는지, 추가된 규칙 요약 기록
- 복잡한 버그 해결 → 원인, 해결 방법, 재발 방지 포인트 기록
- 외부 라이브러리/API 도입 결정 → 선택 이유, 대안 기록
- 아키텍처 또는 폴더 구조 변경 → 변경 전/후, 이유 기록
- 성능 문제 발견 및 해결 → 병목 지점, 해결 방법 기록

**`memory/MEMORY.md` vs `CLAUDE.md` 구분:**
- `memory/MEMORY.md` — 맥락과 히스토리 (왜 이 결정을 했는가)
- `CLAUDE.md` — 규칙 (앞으로 어떻게 해야 하는가)
