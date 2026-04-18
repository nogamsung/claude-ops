---
description: 현재 파일 또는 지정한 파일에 대한 테스트 코드 자동 생성
argument-hint: [파일경로] (없으면 현재 선택된 파일 사용)
---

다음 지시사항에 따라 테스트 코드를 생성해주세요.

**대상**: $ARGUMENTS (없으면 현재 대화에서 언급된 파일 또는 사용자에게 물어보세요)

## 테스트 생성 규칙

대상 파일의 언어/프레임워크를 자동 감지하여 적절한 테스트를 생성하세요.

---

### Kotlin Spring Boot 테스트

**Service 테스트** (`@ExtendWith(MockKExtension::class)`)
- MockK로 의존성 목(mock) 처리
- 정상 케이스(Happy Path) 테스트
- 예외 케이스 테스트 (`shouldThrow`, `assertThrows`)
- `verify { ... }` 로 목 호출 검증

**Controller 테스트** (`@WebMvcTest`)
- `MockMvc` 사용
- 각 HTTP 메서드/상태코드 검증
- Request body validation 테스트
- 인증이 필요한 경우 `@WithMockUser`

**Repository 테스트** (`@DataJpaTest`)
- Testcontainers 또는 H2 in-memory
- 커스텀 쿼리 메서드 검증

```kotlin
// 예시 구조
@ExtendWith(MockKExtension::class)
class UserServiceTest {
    @MockK lateinit var userRepository: UserRepository
    @InjectMockKs lateinit var userService: UserService

    @Test
    fun `should return user when found`() { ... }

    @Test
    fun `should throw EntityNotFoundException when user not found`() { ... }
}
```

---

### Next.js / React 테스트

**컴포넌트 테스트** (React Testing Library)
- render, screen, userEvent 사용
- 렌더링 검증, 사용자 인터랙션 테스트
- 비동기 동작 (`waitFor`, `findBy`)
- 접근성 관련 쿼리 우선 (`getByRole`, `getByLabelText`)

**훅 테스트** (`renderHook`)
- `@tanstack/react-query` 관련은 `QueryClientProvider` 래핑
- Zustand store 모킹

```tsx
// 예시 구조
describe('UserCard', () => {
  it('사용자 이름을 표시한다', () => { ... })
  it('삭제 버튼 클릭시 onDelete를 호출한다', async () => { ... })
})
```

---

### Flutter 테스트

**Unit 테스트**
- Mockito 또는 mocktail로 의존성 목 처리
- Repository, UseCase, Provider 단위 테스트
- `Either` 결과 검증

**Widget 테스트**
- `WidgetTester` 사용
- 화면 렌더링 및 사용자 인터랙션 테스트
- `ProviderScope`로 Riverpod 프로바이더 오버라이드

```dart
// 예시 구조
void main() {
  group('LoginScreen', () {
    testWidgets('이메일 입력 필드가 표시된다', (tester) async { ... });
    testWidgets('폼 제출시 로그인이 호출된다', (tester) async { ... });
  });
}
```

---

## 테스트 커버리지 목표
- **정상 케이스**: 모든 public 메서드/함수의 happy path
- **경계값**: null, 빈 값, 최대/최소값
- **예외 케이스**: 예상 가능한 모든 에러 시나리오
- **비즈니스 규칙**: 도메인 로직의 모든 분기

테스트 파일 위치는 프로젝트의 기존 테스트 구조를 따르세요.
