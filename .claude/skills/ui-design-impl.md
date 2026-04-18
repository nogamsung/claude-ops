# UI Design System Implementation

## Stack Detection

프로젝트 루트에서 스택을 감지한다:
- `package.json` + `next` 의존성 → **Next.js 모드**
- `pubspec.yaml` → **Flutter 모드**

---

## DESIGN.md 없는 경우 — 사용자에게 제시할 옵션

```
DESIGN.md가 없습니다. 어떻게 진행할까요?

1. 유명 디자인에서 영감 선택 (Vercel, Stripe, Linear, Notion, Cursor 등)
2. 프로젝트 맞춤 DESIGN.md 새로 작성
3. 나중에 설정 (현재 작업은 기본 스타일로 진행)
```

**옵션 1** — 카테고리별 추천:
```
Developer Tools: cursor, vercel, warp, raycast, linear.app
AI Platforms:    claude, openai, mistral.ai
SaaS/Productivity: notion, airtable, zapier, intercom
Design Tools:    figma, framer, webflow
Fintech:         stripe, revolut, wise, coinbase
```
선택 후 핵심 특징 요약 → 프로젝트에 맞게 커스터마이징하여 DESIGN.md 작성.

**옵션 2** — 아래 질문으로 커스텀 DESIGN.md 작성:
1. 브랜드 분위기 한 문장 (예: "신뢰감 있는 미니멀 B2B 툴")
2. 주색상 (브랜드 컬러 또는 선호 색 계열)
3. 다크/라이트/시스템 모드?
4. 참고하고 싶은 디자인이 있나요?

---

## Next.js 구현

### Tailwind 설정 (`tailwind.config.ts`)
DESIGN.md의 컬러 팔레트·폰트·스페이싱 스케일을 `theme.extend`에 반영:
```ts
theme: {
  extend: {
    colors: {
      brand: { DEFAULT: '#...', hover: '#...', muted: '#...' },
      surface: { DEFAULT: '#...', elevated: '#...' },
      text: { primary: '#...', secondary: '#...', muted: '#...' },
      border: { DEFAULT: '#...', strong: '#...' },
    },
    fontFamily: {
      sans: ['...', 'system-ui', 'sans-serif'],
      mono: ['...', 'monospace'],
    },
    fontSize: { /* DESIGN.md 타이포그래피 계층 반영 */ },
    borderRadius: { /* DESIGN.md 라디우스 스케일 */ },
    boxShadow: { /* DESIGN.md elevation 시스템 */ },
  }
}
```

### CSS 변수 (`app/globals.css`)
```css
@layer base {
  :root {
    --color-brand: ...;
    --color-surface: ...;
    /* DESIGN.md 모든 시맨틱 컬러 토큰 */
  }
  .dark {
    /* 다크 모드 토큰 */
  }
}
```

### 컴포넌트 생성 규칙
- 컬러: 하드코딩 금지, CSS 변수 또는 Tailwind 시맨틱 클래스만 사용
- 타이포그래피: DESIGN.md 계층 준수 (heading, body, caption)
- 스페이싱: DESIGN.md 스케일 사용 (임의 px 값 금지)
- 컴포넌트 상태: hover, focus, active, disabled 모두 구현
- shadcn/ui 사용 시: `components.json`의 cssVars와 DESIGN.md 토큰 연결

### 디자인 리뷰 체크리스트
- 하드코딩 색상값 탐지 (`#fff`, `rgb(...)`)
- 임의 스페이싱 탐지 (`p-[13px]`)
- DESIGN.md에 없는 폰트 패밀리
- Do's and Don'ts 위반 패턴

---

## Flutter 구현

### 추출 대상 토큰 매핑
| DESIGN.md | Flutter 대상 |
|-----------|------------|
| Color Palette | `ColorScheme` (Material 3) |
| Typography Rules | `TextTheme` (displayLarge ~ bodySmall) |
| Spacing Scale | `lib/core/theme/app_spacing.dart` |
| Border Radius | `lib/core/theme/app_radius.dart` |
| Elevation/Shadows | `lib/core/theme/app_shadows.dart` |

### 생성 파일 구조
```
lib/core/theme/
├── app_theme.dart       # ThemeData (light + dark)
├── app_colors.dart      # 컬러 팔레트 상수
├── app_text_styles.dart # TextTheme 정의
├── app_spacing.dart     # 스페이싱 상수
└── app_radius.dart      # BorderRadius 상수
```

### 코드 패턴
```dart
// app_colors.dart
abstract class AppColors {
  static const Color primary   = Color(0xFF...);
  static const Color surface   = Color(0xFF...);
  static const Color onSurface = Color(0xFF...);
}

// app_theme.dart
class AppTheme {
  static ThemeData get light => ThemeData(
    useMaterial3: true,
    colorScheme: ColorScheme.fromSeed(
      seedColor: AppColors.primary,
      brightness: Brightness.light,
    ),
    textTheme: AppTextStyles.textTheme,
  );

  static ThemeData get dark => ThemeData(
    useMaterial3: true,
    colorScheme: ColorScheme.fromSeed(
      seedColor: AppColors.primary,
      brightness: Brightness.dark,
    ),
  );
}
```

### Flutter 모드 제외 항목
CSS/Tailwind 클래스 스펙, HTML 컴포넌트 예시, 웹 전용 반응형 breakpoint, z-index, CSS 변수 정의

### Flutter 추가 작업
- DESIGN.md "Do's and Don'ts" → Flutter 위젯 선택 기준으로 번역
- 애니메이션 가이드라인 → `AnimationDuration` 상수
- 컴포넌트 상태 가이드라인 → `MaterialState` 처리

---

## 완료 보고 형식

```
✅ 디자인 시스템 적용 완료

DESIGN.md: [선택한 디자인 또는 커스텀]
스택: [Next.js | Flutter]

[Next.js]
- tailwind.config.ts: 컬러 X개, 폰트 Y개, 스페이싱 Z개 토큰 반영
- globals.css: CSS 변수 N개 정의
- 생성/수정 컴포넌트: [목록]

[Flutter]
- lib/core/theme/ 파일 N개 생성
- ColorScheme: [primary, secondary 컬러]
- TextTheme: [폰트 패밀리]
- 스페이싱 상수: N개

주의사항:
- [DESIGN.md 핵심 Do's and Don'ts 요약]
```
