# Next.js Code Patterns

## Generation Patterns

### Page (Server Component)
```tsx
// app/(dashboard)/orders/page.tsx
import { OrderList } from "@/components/features/orders/order-list"
import { getOrders } from "@/lib/api/orders"

export const metadata = { title: "주문 목록" }

export default async function OrdersPage() {
  const orders = await getOrders()
  return (
    <main className="container mx-auto py-8">
      <h1 className="text-2xl font-bold mb-6">주문 목록</h1>
      <OrderList initialData={orders} />
    </main>
  )
}
```

### Feature Component (Client)
```tsx
// components/features/orders/order-list.tsx
"use client"

import { useOrders } from "@/hooks/use-orders"
import { OrderCard } from "./order-card"
import type { Order } from "@/types/order"

interface OrderListProps {
  initialData: Order[]
}

export function OrderList({ initialData }: OrderListProps) {
  const { data: orders, isLoading } = useOrders({ initialData })

  if (isLoading) return <OrderListSkeleton />

  return (
    <ul className="space-y-4">
      {orders.map((order) => (
        <li key={order.id}>
          <OrderCard order={order} />
        </li>
      ))}
    </ul>
  )
}
```

### Custom Hook
```tsx
// hooks/use-orders.ts
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { getOrders, createOrder } from "@/lib/api/orders"
import type { Order, CreateOrderRequest } from "@/types/order"

export const orderKeys = {
  all: ["orders"] as const,
  detail: (id: number) => ["orders", id] as const,
}

export function useOrders(options?: { initialData?: Order[] }) {
  return useQuery({
    queryKey: orderKeys.all,
    queryFn: getOrders,
    initialData: options?.initialData,
  })
}

export function useCreateOrder() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: CreateOrderRequest) => createOrder(data),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: orderKeys.all }),
  })
}
```

### API Client Functions
```tsx
// lib/api/orders.ts
import { apiClient } from "@/lib/api-client"
import type { Order, CreateOrderRequest } from "@/types/order"

export const getOrders = (): Promise<Order[]> =>
  apiClient.get("/orders").then((r) => r.data)

export const getOrder = (id: number): Promise<Order> =>
  apiClient.get(`/orders/${id}`).then((r) => r.data)

export const createOrder = (data: CreateOrderRequest): Promise<Order> =>
  apiClient.post("/orders", data).then((r) => r.data)
```

### Types
```tsx
// types/order.ts
export type OrderStatus = "PENDING" | "CONFIRMED" | "SHIPPED" | "DELIVERED" | "CANCELLED"

export interface Order {
  id: number
  status: OrderStatus
  createdAt: string
}

export interface CreateOrderRequest {
  productId: number
  quantity: number
}
```

### Zustand Store
```tsx
// stores/cart-store.ts
import { create } from "zustand"
import { persist } from "zustand/middleware"

interface CartItem { productId: number; quantity: number }

interface CartState {
  items: CartItem[]
  add: (item: CartItem) => void
  remove: (productId: number) => void
  clear: () => void
}

export const useCartStore = create<CartState>()(
  persist(
    (set) => ({
      items: [],
      add: (item) =>
        set((s) => ({
          items: s.items.find((i) => i.productId === item.productId)
            ? s.items.map((i) =>
                i.productId === item.productId
                  ? { ...i, quantity: i.quantity + item.quantity }
                  : i
              )
            : [...s.items, item],
        })),
      remove: (productId) =>
        set((s) => ({ items: s.items.filter((i) => i.productId !== productId) })),
      clear: () => set({ items: [] }),
    }),
    { name: "cart" }
  )
)
```

### Form Component
```tsx
// components/features/orders/create-order-form.tsx
"use client"

import { useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { useCreateOrder } from "@/hooks/use-orders"

const schema = z.object({
  productId: z.number().positive(),
  quantity: z.number().int().min(1, "최소 1개 이상"),
})
type FormValues = z.infer<typeof schema>

export function CreateOrderForm() {
  const { mutate, isPending } = useCreateOrder()
  const form = useForm<FormValues>({ resolver: zodResolver(schema) })

  return (
    <form onSubmit={form.handleSubmit((v) => mutate(v))} className="space-y-4">
      {/* fields */}
      <button type="submit" disabled={isPending}>
        {isPending ? "처리 중..." : "주문하기"}
      </button>
    </form>
  )
}
```

---

## Modification Patterns

### Prop 추가
```tsx
// Before
interface OrderCardProps { order: Order }

// After — optional onDelete 추가
interface OrderCardProps {
  order: Order
  onDelete?: (id: number) => void  // ADDED
}
export function OrderCard({ order, onDelete }: OrderCardProps) {
  return (
    <div>
      {/* existing JSX */}
      {onDelete && (  // ADDED
        <button onClick={() => onDelete(order.id)}>삭제</button>
      )}
    </div>
  )
}
```

### Server → Client 전환
```tsx
// Parent (Server Component) — 계속 fetch
export default async function OrdersPage() {
  const initialData = await getOrders()
  return <OrderList initialData={initialData} />  // MODIFIED
}

// Child (Client Component로 전환)
"use client"
export function OrderList({ initialData }: { initialData: Order[] }) {
  const { data } = useOrders({ initialData })
  // ...
}
```

### 성능 최적화
```tsx
// 고비용 자식 메모이제이션
const MemoizedChart = memo(Chart, (prev, next) => prev.data === next.data)

// 안정적인 콜백 참조
const handleDelete = useCallback((id: number) => {
  deleteOrder(id)
}, [deleteOrder])

// 스토어에서 필요한 것만 선택
const itemCount = useCartStore((s) => s.items.length)
```

---

## Test Patterns

### Component Test
```tsx
// components/features/orders/order-card.test.tsx
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { OrderCard } from "./order-card"
import { OrderFixture } from "@/test/fixtures/order-fixture"

describe("OrderCard", () => {
  it("주문 상태와 ID를 표시한다", () => {
    render(<OrderCard order={OrderFixture.create({ id: 42, status: "PENDING" })} />)
    expect(screen.getByText("42")).toBeInTheDocument()
    expect(screen.getByText("PENDING")).toBeInTheDocument()
  })

  it("onDelete prop이 있으면 삭제 버튼을 표시한다", async () => {
    const user = userEvent.setup()
    const onDelete = jest.fn()
    render(<OrderCard order={OrderFixture.create()} onDelete={onDelete} />)

    await user.click(screen.getByRole("button", { name: "삭제" }))
    expect(onDelete).toHaveBeenCalledWith(expect.any(Number))
  })

  it("onDelete prop이 없으면 삭제 버튼을 숨긴다", () => {
    render(<OrderCard order={OrderFixture.create()} />)
    expect(screen.queryByRole("button", { name: "삭제" })).not.toBeInTheDocument()
  })
})
```

### Hook Test (MSW)
```tsx
// hooks/use-orders.test.ts
function createWrapper() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
}

describe("useOrders", () => {
  it("주문 목록을 가져온다", async () => {
    server.use(
      http.get("/api/orders", () => HttpResponse.json([{ id: 1, status: "PENDING" }]))
    )
    const { result } = renderHook(() => useOrders(), { wrapper: createWrapper() })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data).toHaveLength(1)
  })

  it("API 에러 시 isError가 true가 된다", async () => {
    server.use(
      http.get("/api/orders", () => HttpResponse.json(null, { status: 500 }))
    )
    const { result } = renderHook(() => useOrders(), { wrapper: createWrapper() })
    await waitFor(() => expect(result.current.isError).toBe(true))
  })
})
```

### Form Test
```tsx
it("이메일이 비어있으면 유효성 에러를 표시한다", async () => {
  const user = userEvent.setup()
  render(<LoginForm onSubmit={jest.fn()} />)

  await user.click(screen.getByRole("button", { name: "로그인" }))
  expect(await screen.findByText("이메일을 입력하세요")).toBeInTheDocument()
})
```

### Fixture + MSW 핸들러
```ts
// test/fixtures/order-fixture.ts
export const OrderFixture = {
  create: (overrides: Partial<Order> = {}): Order => ({
    id: 1,
    status: "PENDING",
    createdAt: "2024-01-01T00:00:00Z",
    ...overrides,
  }),
  list: (count = 3): Order[] =>
    Array.from({ length: count }, (_, i) => OrderFixture.create({ id: i + 1 })),
}

// test/msw/handlers.ts
export const handlers = [
  http.get("/api/orders", () => HttpResponse.json(OrderFixture.list())),
  http.post("/api/orders", async () =>
    HttpResponse.json(OrderFixture.create(), { status: 201 })
  ),
]
```

### Test Anti-patterns
- semantic query 있는데 `getByTestId` 사용
- 구현 세부사항(내부 상태·메서드 호출) 테스트
- trivial 정적 마크업 외 스냅샷 테스트
- 테스트 대상 컴포넌트 자체를 mock
- 서드파티 라이브러리 동작 테스트 (예: React Hook Form 자체 validation)

---

## Multi-Package Patterns (Turborepo)

### turbo.json
```json
{
  "$schema": "https://turbo.build/schema.json",
  "tasks": {
    "build": {
      "dependsOn": ["^build"],
      "outputs": [".next/**", "dist/**"]
    },
    "lint": { "dependsOn": ["^lint"] },
    "test": { "dependsOn": ["^build"] },
    "dev": { "cache": false, "persistent": true }
  }
}
```

### 루트 package.json
```json
{
  "name": "project-root",
  "private": true,
  "workspaces": ["apps/*", "packages/*"],
  "scripts": {
    "dev": "turbo run dev",
    "build": "turbo run build",
    "lint": "turbo run lint",
    "test": "turbo run test"
  },
  "devDependencies": {
    "turbo": "^2.0.0"
  }
}
```

### packages/ui/package.json
```json
{
  "name": "@project/ui",
  "version": "0.0.1",
  "exports": {
    ".": "./src/index.ts"
  },
  "scripts": {
    "lint": "eslint src/",
    "test": "jest"
  },
  "peerDependencies": {
    "react": "^18",
    "react-dom": "^18"
  },
  "devDependencies": {
    "@project/config": "*"
  }
}
```

### apps/web에서 공유 패키지 사용
```tsx
// apps/web/package.json dependencies에 "@project/ui": "*" 추가 후
import { Button, Card } from "@project/ui"
import { type User } from "@project/lib/types"
import { apiClient } from "@project/lib/api"
```

### 패키지 의존 규칙
| 패키지 | 의존 가능 | 의존 불가 |
|--------|----------|----------|
| `packages/config` | (없음) | apps/*, 다른 packages |
| `packages/lib` | `packages/config` | apps/*, `packages/ui` |
| `packages/ui` | `packages/config`, `packages/lib` | apps/* |
| `apps/web` | 모든 packages | 다른 apps |

### 특정 앱/패키지만 실행
```bash
turbo run dev --filter=web           # apps/web만
turbo run test --filter=@project/ui  # ui 패키지만
turbo run build --filter=web...      # web + 의존 패키지 모두
```
