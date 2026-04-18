# Flutter Code Patterns

## Generation Patterns

### Domain Entity (Freezed)
```dart
// lib/features/order/domain/entities/order.dart
import 'package:freezed_annotation/freezed_annotation.dart';

part 'order.freezed.dart';
part 'order.g.dart';

enum OrderStatus { pending, confirmed, shipped, delivered, cancelled }

@freezed
class Order with _$Order {
  const factory Order({
    required int id,
    required OrderStatus status,
    required DateTime createdAt,
  }) = _Order;

  factory Order.fromJson(Map<String, dynamic> json) => _$OrderFromJson(json);
}
```

### Repository Interface
```dart
// lib/features/order/domain/repositories/order_repository.dart
abstract interface class OrderRepository {
  Future<Either<Failure, List<Order>>> getOrders();
  Future<Either<Failure, Order>> getOrder(int id);
  Future<Either<Failure, Order>> createOrder(CreateOrderParams params);
}
```

### DataSource + Repository Impl
```dart
// lib/features/order/data/datasources/order_remote_data_source.dart
abstract interface class OrderRemoteDataSource {
  Future<List<OrderModel>> getOrders();
}

@LazySingleton(as: OrderRemoteDataSource)
class OrderRemoteDataSourceImpl implements OrderRemoteDataSource {
  const OrderRemoteDataSourceImpl(this._dio);
  final Dio _dio;

  @override
  Future<List<OrderModel>> getOrders() async {
    final response = await _dio.get('/orders');
    return (response.data as List)
        .map((e) => OrderModel.fromJson(e as Map<String, dynamic>))
        .toList();
  }
}

// lib/features/order/data/repositories/order_repository_impl.dart
@LazySingleton(as: OrderRepository)
class OrderRepositoryImpl implements OrderRepository {
  const OrderRepositoryImpl(this._remote);
  final OrderRemoteDataSource _remote;

  @override
  Future<Either<Failure, List<Order>>> getOrders() async {
    try {
      final models = await _remote.getOrders();
      return Right(models.map((m) => m.toEntity()).toList());
    } on DioException catch (e) {
      return Left(ServerFailure(e.message ?? 'Server error'));
    }
  }
}
```

### Riverpod Provider
```dart
// lib/features/order/presentation/providers/orders_provider.dart
part 'orders_provider.g.dart';

@riverpod
class OrdersNotifier extends _$OrdersNotifier {
  @override
  Future<List<Order>> build() async {
    final result = await ref.read(orderRepositoryProvider).getOrders();
    return result.fold(
      (failure) => throw Exception(failure.message),
      (orders) => orders,
    );
  }

  Future<void> refresh() => ref.refresh(ordersNotifierProvider.future);
}
```

### Screen
```dart
// lib/features/order/presentation/screens/orders_screen.dart
class OrdersScreen extends ConsumerWidget {
  const OrdersScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final ordersAsync = ref.watch(ordersNotifierProvider);

    return Scaffold(
      appBar: AppBar(title: const Text('주문 목록')),
      body: ordersAsync.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text('오류가 발생했습니다: $e'),
              ElevatedButton(
                onPressed: () => ref.invalidate(ordersNotifierProvider),
                child: const Text('다시 시도'),
              ),
            ],
          ),
        ),
        data: (orders) => orders.isEmpty
            ? const Center(child: Text('주문이 없습니다'))
            : ListView.builder(
                itemCount: orders.length,
                itemBuilder: (_, i) => OrderListTile(order: orders[i]),
              ),
      ),
    );
  }
}
```

### GoRouter Route Registration
```dart
GoRoute(
  path: '/orders',
  name: AppRoutes.orders,
  builder: (_, __) => const OrdersScreen(),
  routes: [
    GoRoute(
      path: ':id',
      name: AppRoutes.orderDetail,
      builder: (_, state) => OrderDetailScreen(
        id: int.parse(state.pathParameters['id']!),
      ),
    ),
  ],
),
```

---

## Modification Patterns

### Freezed 필드 추가
```dart
// Before
@freezed
class Order with _$Order {
  const factory Order({
    required int id,
    required OrderStatus status,
  }) = _Order;
}

// After — optional description 추가
@freezed
class Order with _$Order {
  const factory Order({
    required int id,
    required OrderStatus status,
    String? description,  // ADDED
  }) = _Order;
}
// → 모든 생성 호출부 업데이트 + build_runner 실행
```

### Provider 액션 추가
```dart
@riverpod
class OrdersNotifier extends _$OrdersNotifier {
  // 기존 build() 유지 ...

  // ADDED
  Future<void> cancelOrder(int id) async {
    final result = await ref.read(orderRepositoryProvider).cancelOrder(id);
    result.fold(
      (failure) => state = AsyncError(failure.message, StackTrace.current),
      (_) => ref.invalidateSelf(),
    );
  }
}
```

### Widget 파라미터 추가
```dart
// After — onTap 추가
class OrderListTile extends StatelessWidget {
  const OrderListTile({
    super.key,
    required this.order,
    this.onTap,  // ADDED
  });
  final Order order;
  final VoidCallback? onTap;  // ADDED

  @override
  Widget build(BuildContext context) => ListTile(
    title: Text(order.id.toString()),
    onTap: onTap,  // ADDED
  );
}
```

### StatelessWidget → ConsumerWidget
```dart
// After
class OrdersScreen extends ConsumerWidget {
  @override
  Widget build(BuildContext context, WidgetRef ref) {  // MODIFIED
    final orders = ref.watch(ordersNotifierProvider);
    // ...
  }
}
```

---

## Test Patterns

### Provider Unit Test
```dart
class MockOrderRepository extends Mock implements OrderRepository {}

void main() {
  late MockOrderRepository mockRepo;
  late ProviderContainer container;

  setUp(() {
    mockRepo = MockOrderRepository();
    container = ProviderContainer(
      overrides: [orderRepositoryProvider.overrideWithValue(mockRepo)],
    );
  });

  tearDown(() => container.dispose());

  group('OrdersNotifier', () {
    test('주문 목록을 성공적으로 불러온다', () async {
      when(() => mockRepo.getOrders())
          .thenAnswer((_) async => Right(OrderFixture.list()));

      final result = await container.read(ordersNotifierProvider.future);
      expect(result, hasLength(3));
    });

    test('저장소 에러 시 AsyncError 상태가 된다', () async {
      when(() => mockRepo.getOrders())
          .thenAnswer((_) async => Left(ServerFailure('서버 오류')));

      await expectLater(
        container.read(ordersNotifierProvider.future),
        throwsA(isA<Exception>()),
      );
    });
  });
}
```

### Widget Test
```dart
void main() {
  late MockOrderRepository mockRepo;

  setUp(() => mockRepo = MockOrderRepository());

  Widget buildSubject() => ProviderScope(
    overrides: [orderRepositoryProvider.overrideWithValue(mockRepo)],
    child: const MaterialApp(home: OrdersScreen()),
  );

  group('OrdersScreen', () {
    testWidgets('로딩 중 CircularProgressIndicator 표시', (tester) async {
      when(() => mockRepo.getOrders()).thenAnswer((_) async {
        await Future.delayed(const Duration(seconds: 1));
        return Right(OrderFixture.list());
      });

      await tester.pumpWidget(buildSubject());
      await tester.pump();
      expect(find.byType(CircularProgressIndicator), findsOneWidget);
    });

    testWidgets('주문 목록 정상 표시', (tester) async {
      when(() => mockRepo.getOrders())
          .thenAnswer((_) async => Right(OrderFixture.list(count: 3)));

      await tester.pumpWidget(buildSubject());
      await tester.pumpAndSettle();
      expect(find.byType(OrderListTile), findsNWidgets(3));
    });

    testWidgets('빈 목록 안내 메시지 표시', (tester) async {
      when(() => mockRepo.getOrders())
          .thenAnswer((_) async => const Right([]));

      await tester.pumpWidget(buildSubject());
      await tester.pumpAndSettle();
      expect(find.text('주문이 없습니다'), findsOneWidget);
    });
  });
}
```

### Fixture Pattern
```dart
class OrderFixture {
  static Order create({int id = 1, OrderStatus status = OrderStatus.pending}) =>
      Order(id: id, status: status, createdAt: DateTime(2024));

  static List<Order> list({int count = 3}) =>
      List.generate(count, (i) => create(id: i + 1));
}
```

### Test Anti-patterns
- `pumpAndSettle()` + 실제 타이머 조합 (무한 루프 위험) → `pump(Duration)` 사용
- `(widget as ConcreteWidget).privateField` 로 private 필드 접근
- Riverpod 내부 동작 테스트 (생성되는 state를 테스트해야 함)
- tearDown 없이 ProviderContainer 방치 (메모리 누수)
