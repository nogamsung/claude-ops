---
description: 마케팅 작업 라우터 — 카피·SEO·CRO·광고·이메일·리텐션 등 marketing-skills 플러그인을 자연어/서브명령으로 호출
argument-hint: [category] [task] | <자연어 요청>
---

마케팅 관련 작업을 적절한 `marketing-skills:*` 스킬로 라우팅합니다.

**요청:** $ARGUMENTS

---

## Step 0 — 플러그인 가용성 확인

`marketing-skills:*` 스킬이 하나도 없으면 종료:

```
marketing-skills 플러그인이 설치되어 있지 않습니다.
플러그인 설치 후 다시 시도해주세요.
```

## Step 1 — Context 자동 로드

`.agents/product-marketing-context.md` 존재 여부 확인:

- **있음** → 파일 내용을 읽어 후속 스킬 호출 시 컨텍스트로 주입
- **없음** → 사용자에게 한 번만 안내 (강제하지 않음):
  > 💡 `product-marketing-context` 가 설정돼 있지 않습니다. 반복 작업이라면 먼저 설정을 권장합니다.
  > 지금 설정하시겠어요? (yes → `marketing-skills:product-marketing-context` 실행 / no → 계속 진행)

`memory/MEMORY.md` 에 마케팅 관련 결정(카테고리: 결정/규칙 중 "마케팅·카피·SEO·CRO·광고" 키워드 포함)이 있으면 함께 참고.

## Step 2 — 인수 파싱 & 모드 분기

`$ARGUMENTS` 의 첫 토큰으로 분기:

| 조건 | 모드 |
|------|------|
| 인수 없음 | [메뉴 모드](#메뉴-모드) |
| 첫 토큰이 카테고리명 (strategy/seo/cro/channel/retention/context) | [서브명령 모드](#서브명령-모드) |
| 그 외 (자유 텍스트) | [자연어 라우팅 모드](#자연어-라우팅-모드) |

---

## 메뉴 모드

6개 카테고리와 사용 예시를 출력하고 종료:

```
/marketing — 마케팅 작업 라우터

1. strategy   전략/리서치      ideas, positioning, audience, launch, pricing
2. seo        콘텐츠/SEO       copy, seo, schema, content, competitor
3. cro        전환(CRO)        landing, signup, onboarding, paywall, popup, form
4. channel    채널/획득         ads, email (cold/sequence), social, community, lead-magnet
5. retention  유지/수익         churn, referral, revops, sales, analytics, ab-test
6. context    기본 컨텍스트    product-marketing-context 설정

사용 예시:
  /marketing seo audit                  — SEO 감사
  /marketing cro landing                — 랜딩페이지 CRO
  /marketing channel email welcome      — 웰컴 이메일 시퀀스
  /marketing 회원가입 전환율이 낮아    — 자연어 (자동 라우팅)
```

---

## 서브명령 모드

`/marketing <category> <task> [나머지]` — 카테고리 × 태스크 → 스킬 매핑.

### 매핑 표

#### strategy — 전략/리서치
| task | 스킬 |
|------|------|
| ideas | `marketing-skills:marketing-ideas` |
| psychology | `marketing-skills:marketing-psychology` |
| research / audience / icp | `marketing-skills:customer-research` |
| content-strategy / plan | `marketing-skills:content-strategy` |
| launch / release / ph | `marketing-skills:launch-strategy` |
| pricing | `marketing-skills:pricing-strategy` |
| positioning / context | `marketing-skills:product-marketing-context` |

#### seo — 콘텐츠/SEO
| task | 스킬 |
|------|------|
| copy / headline / landing-copy | `marketing-skills:copywriting` |
| edit / polish / refresh | `marketing-skills:copy-editing` |
| audit / seo | `marketing-skills:seo-audit` |
| ai / aeo / geo | `marketing-skills:ai-seo` |
| schema / structured-data | `marketing-skills:schema-markup` |
| programmatic / pseo / templates | `marketing-skills:programmatic-seo` |
| architecture / sitemap / ia | `marketing-skills:site-architecture` |
| competitor / vs / alternative | `marketing-skills:competitor-alternatives` |

#### cro — 전환 최적화
| task | 스킬 |
|------|------|
| landing / page | `marketing-skills:page-cro` |
| signup / registration | `marketing-skills:signup-flow-cro` |
| onboarding / activation | `marketing-skills:onboarding-cro` |
| paywall / upgrade | `marketing-skills:paywall-upgrade-cro` |
| popup / modal / banner | `marketing-skills:popup-cro` |
| form | `marketing-skills:form-cro` |
| aso / app-store | `marketing-skills:aso-audit` |

#### channel — 채널/획득
| task | 스킬 |
|------|------|
| ads / ppc / paid | `marketing-skills:paid-ads` |
| ad-creative / headlines | `marketing-skills:ad-creative` |
| email / sequence / drip | `marketing-skills:email-sequence` |
| cold-email / outreach | `marketing-skills:cold-email` |
| social / linkedin / twitter | `marketing-skills:social-content` |
| community / discord | `marketing-skills:community-marketing` |
| lead-magnet / ebook / gated | `marketing-skills:lead-magnets` |
| free-tool / calculator | `marketing-skills:free-tool-strategy` |

#### retention — 유지/수익
| task | 스킬 |
|------|------|
| churn / cancel / dunning | `marketing-skills:churn-prevention` |
| referral / affiliate | `marketing-skills:referral-program` |
| revops / lead-scoring | `marketing-skills:revops` |
| sales / deck / one-pager | `marketing-skills:sales-enablement` |
| analytics / ga4 / tracking | `marketing-skills:analytics-tracking` |
| ab-test / experiment | `marketing-skills:ab-test-setup` |

#### context
→ `marketing-skills:product-marketing-context` (task 무시)

### 처리 순서

1. 카테고리 + task 로 스킬 확정
2. task 매칭 실패 시: 해당 카테고리의 task 목록을 보여주고 종료
3. 매칭된 스킬 호출. `$ARGUMENTS` 의 나머지(카테고리/task 제외)를 스킬 args 로 전달
4. 호출 전 한 줄로 알림: `→ marketing-skills:<skill-name> 실행`
5. 호출 직후 Step 5 "후속 제안" 출력

---

## 자연어 라우팅 모드

자유 텍스트 요청을 키워드·의도로 분석해 **가장 높은 점수 스킬 1개**를 자동 실행.

### Step A — 의도 키워드 매칭

아래 트리거 사전으로 점수 계산 (한·영 모두 포함). 매칭된 스킬들에 +1점씩, 복수 키워드 매칭은 누적.

| 트리거 (ko/en) | 스킬 |
|----------------|------|
| 카피, 헤드라인, 랜딩 카피, 카피라이팅, copywriting, copy, headline, landing copy, hero, CTA, value prop | `copywriting` |
| 카피 다듬, 카피 고쳐, 리라이트, 폴리시, copy edit, polish, rewrite, refresh, proofread | `copy-editing` |
| SEO, 랭킹, 트래픽, 검색 순위, seo audit, technical seo, not ranking, on-page | `seo-audit` |
| AI 검색, ChatGPT 인용, AEO, GEO, AI overview, AI citation, perplexity | `ai-seo` |
| 스키마, 구조화 데이터, rich snippet, JSON-LD, structured data | `schema-markup` |
| 프로그래매틱, 템플릿 페이지, pSEO, scale pages, programmatic, directory pages | `programmatic-seo` |
| 사이트맵, 정보 구조, IA, 네비게이션, site map, navigation, url structure | `site-architecture` |
| 경쟁사, 비교 페이지, vs, alternative, competitor, battle card | `competitor-alternatives` |
| 콘텐츠 전략, 블로그 주제, 에디토리얼, content strategy, topic cluster, editorial | `content-strategy` |
| 마케팅 아이디어, 성장 아이디어, 뭘 해야할지, marketing idea, growth idea, stuck | `marketing-ideas` |
| 심리, 인지편향, 설득, psychology, persuasion, cognitive bias, anchoring, social proof | `marketing-psychology` |
| 고객 리서치, ICP, 페르소나, 인터뷰 분석, customer research, persona, JTBD, VOC, review mining | `customer-research` |
| 포지셔닝, 제품 컨텍스트, product context, positioning | `product-marketing-context` |
| 런치, 출시, 프로덕트 헌트, launch, product hunt, go-to-market, GTM | `launch-strategy` |
| 가격, 프라이싱, pricing, freemium, free trial, willingness to pay | `pricing-strategy` |
| 랜딩페이지 전환, 페이지 전환율, landing, page CRO, conversion, bounce | `page-cro` |
| 회원가입, 가입 이탈, signup, registration, trial signup | `signup-flow-cro` |
| 온보딩, 활성화, aha moment, onboarding, activation, first-run | `onboarding-cro` |
| 페이월, 업그레이드, upsell, paywall, upgrade screen, feature gate | `paywall-upgrade-cro` |
| 팝업, 모달, 배너, popup, modal, overlay, exit intent | `popup-cro` |
| 폼, 리드 폼, form optimization, lead form, demo request | `form-cro` |
| ASO, 앱스토어, app store optimization, play store listing | `aso-audit` |
| 광고, PPC, 페이드, 구글 광고, 메타 광고, paid ads, google ads, facebook ads, roas | `paid-ads` |
| 광고 카피, 헤드라인 생성, ad creative, ad copy, RSA headlines | `ad-creative` |
| 이메일 시퀀스, 드립, 웰컴 메일, 온보딩 메일, email sequence, drip, nurture, lifecycle | `email-sequence` |
| 콜드 메일, 아웃바운드, cold email, outbound, prospecting | `cold-email` |
| 소셜, 링크드인, 트위터, social, linkedin post, twitter thread, carousel | `social-content` |
| 커뮤니티, 디스코드, 슬랙, community, discord, advocate | `community-marketing` |
| 리드마그넷, 게이티드, ebook, cheat sheet, lead magnet, gated content | `lead-magnets` |
| 무료 툴, 계산기, free tool, calculator, generator | `free-tool-strategy` |
| 이탈, 해지, 처닝, churn, cancel, dunning, retention, save offer | `churn-prevention` |
| 리퍼럴, 추천 프로그램, 어필리에이트, referral, affiliate, word of mouth | `referral-program` |
| 리드 스코어링, MQL, SQL, revops, lead scoring, pipeline | `revops` |
| 세일즈 덱, 원페이저, 제안서, sales deck, one-pager, objection handling | `sales-enablement` |
| 트래킹, 애널리틱스, GA4, GTM, UTM, analytics, conversion tracking | `analytics-tracking` |
| AB 테스트, 실험, split test, ab test, experiment, variant | `ab-test-setup` |

### Step B — 점수 집계 & 단일 선택

1. 모든 트리거 중 매칭된 것을 합산 → 스킬별 점수
2. **최고 점수 스킬 1개**를 선정
   - 동점이면 표 등장 순서상 위쪽(= 더 일반적인 스킬)을 우선
3. 매칭 0점이면:
   ```
   어떤 마케팅 작업인지 판단하기 어렵습니다.
   /marketing 으로 카테고리 메뉴를 확인하거나, 더 구체적으로 작성해주세요.
   ```

### Step C — 호출

```
→ 의도 분석: "<요약>" → marketing-skills:<skill-name>
```

Skill 도구로 호출:

```
Skill(skill="marketing-skills:<skill-name>", args="<원본 $ARGUMENTS>")
```

`.agents/product-marketing-context.md` 내용이 있으면 args 앞에 컨텍스트 섹션으로 덧붙여 전달.

---

## Step 5 — 후속 제안

스킬 실행이 끝나면 카테고리별로 자연스러운 다음 단계 1-2개를 제안:

| 완료된 작업 | 다음 단계 제안 |
|-------------|----------------|
| `copywriting` | `/marketing cro landing` (작성한 카피로 CRO), `/marketing seo audit` |
| `seo-audit` | `/marketing seo schema`, `/marketing seo programmatic` |
| `page-cro` / `signup-flow-cro` | `/marketing retention ab-test`, `/marketing retention analytics` |
| `email-sequence` | `/marketing retention analytics`, `/marketing retention churn` |
| `paid-ads` | `/marketing channel ad-creative`, `/marketing cro landing` |
| `launch-strategy` | `/marketing channel social`, `/marketing channel email` |
| `pricing-strategy` | `/marketing cro paywall`, `/marketing retention churn` |
| `product-marketing-context` | `/marketing strategy ideas`, `/marketing seo copy` |

---

## 주의사항

- **이 커맨드는 코드를 작성하지 않습니다** — marketing-skills 로의 라우터일 뿐
- **스택 무관** — monorepo 역할 prefix(backend/frontend/mobile) 체크 없음
- **언어** — 한국어 입력 → 한글 키워드로 의도 매칭 후 영어 트리거 스킬 호출. 스킬 호출 시 원문을 그대로 전달하여 스킬 내부에서 한국어 응답이 가능하도록 함
- **자연어 라우팅의 한계** — 점수 기반이라 항상 완벽하진 않습니다. 결과가 기대와 다르면 서브명령 모드(`/marketing <category> <task>`)로 명시적으로 지정
