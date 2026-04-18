# GTM 히스토리

기능별 Go-To-Market 기록. 날짜는 **초안 작성 시점**, 버전은 **릴리스 태그**입니다.

- 기획: `/planner <기능> --gtm` (또는 `--marketing` / `--sales`)
- 릴리스: `/merge` 실행 시 해당 기능의 스냅샷에 `released_version` 이 자동 기록됩니다.

---

## 전체 히스토리 (시간순)

| 날짜 | 버전 | 기능 | 상태 | 마케팅 | 세일즈 | 문서 |
|------|------|------|------|--------|--------|------|
| — | — | — | — | — | — | — |

<!--
예시 행:
| 2026-04-17 | v1.8.0 | /marketing 커맨드 | released | ✓ | — | [→](./2026-04-17-marketing-command/) |
| 2026-04-20 | — | 로그인 기능 | draft | ✓ | ✓ | [→](./2026-04-20-login/) |
-->

---

## 버전별 조회

### 미릴리스 (draft)

- (없음)

<!--
예시:
- [로그인 기능](./2026-04-20-login/) — 2026-04-20 기획
-->

---

## 디렉토리 규칙

```
docs/gtm/
  history.md                         # 이 파일 — 인덱스
  {YYYY-MM-DD}-{feature-kebab}/      # 기능별 스냅샷
    marketing.md                     # 마케팅 전략 스냅샷
    sales.md                         # 세일즈 전략 스냅샷
    meta.yaml                        # 메타데이터 (feature, status, released_version 등)
```

**meta.yaml** 스키마:

```yaml
feature: login                       # kebab-case
created_at: 2026-04-20               # 초안 작성일
status: draft | released | archived  # 라이프사이클
prd_path: docs/specs/login.md        # PRD 링크
spec_dir: docs/specs/login/          # 역할 프롬프트 + 살아있는 marketing.md/sales.md
released_version: v1.9.0             # /merge 시 자동 기록. 미릴리스면 null
released_at: 2026-04-25              # 릴리스 날짜. 미릴리스면 null
plans: [marketing, sales]            # 생성된 문서 종류
```
