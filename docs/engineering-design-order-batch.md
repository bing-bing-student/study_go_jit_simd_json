# OrderBatch 最终工程设计

## 1. 状态

本文档描述当前已经实现的版本，不是待执行计划。

```text
schema：       已冻结
Go reference： 已实现
PackedBatch：  已实现
C static：     已实现
AVX2：         已实现
x86-64 JIT：   已实现
测试与基准：   已实现
```

目标是用一个范围受控的订单 JSON 编码器，完整验证：

```text
Go -> cgo -> C ABI -> JIT -> SIMD
```

项目不是通用 JSON 库，也不承诺生产级兼容性。

## 2. 运行范围

```text
OS：      Linux
架构：    amd64
Go：      1.24+
cgo：     必须启用
C 编译器：GCC
SIMD：    AVX2 可选
```

`ModeAuto` 会根据 CPU 能力选择 AVX2 或标量扫描。显式选择 `ModeAVX2` 而 CPU 不支持时，创建 encoder 会返回 `ErrUnsupportedCPU`。

## 3. 数据模型

顶层输入是：

```go
type OrderBatch struct {
    Orders []Order `json:"orders"`
}
```

每个订单包含：

```text
普通字符串：orderId、createdAt、status、method、name、city、address、remark
nullable：  trackingNo
整数：      buyer.id
布尔：      payment.paid
数组：      items
```

每个 item 包含：

```text
字符串：sku、title
整数：  qty、price
```

完整定义见 [`model.go`](../model.go)，字段取舍见 [`order-schema-proposal.md`](order-schema-proposal.md)。

## 4. 总体架构

```text
OrderBatch
  |
  | Pack
  v
PackedBatch
  |- []OrderRow
  |- []ItemRow
  |- []byte stringPool
  |- null flags
  |- stringsPlain
  `- exact output size
  |
  | MarshalPacked，一次 cgo 调用
  v
C batch encoder
  |- 写入 {"orders":[
  |- 遍历 OrderRow
  |    `- static order_fn 或 JIT order_fn
  |- 遍历 ItemRow
  |- scalar/AVX2 字符串扫描
  `- 返回 written + status
```

职责划分：

| 层 | 职责 |
| --- | --- |
| Go model | 提供固定业务结构 |
| Go reference | 正确性基线 |
| Pack | 校验和构造无 Go 指针的连续数据 |
| cgo wrapper | 一次同步调用、生命周期保护、错误映射 |
| C bridge | encoder 创建、CPU 分发、批次循环 |
| C writer | JSON 字面量、字符串、整数、bool、null、items |
| JIT emitter | 生成单订单字段操作调用序列 |
| AVX2 scanner | 批量定位字符串特殊字节 |

## 5. 公开 API

```go
type Options struct {
    Mode      Mode
    Backend   Backend
    TrustUTF8 bool
}

func NewEncoder(options Options) (*Encoder, error)
func (e *Encoder) Marshal(batch OrderBatch) ([]byte, error)
func (e *Encoder) MarshalPacked(batch *PackedBatch) ([]byte, error)
func (e *Encoder) CodeSize() int
func (e *Encoder) Close() error
func SupportsAVX2() bool

func Pack(batch OrderBatch) (*PackedBatch, error)
func MarshalReference(batch OrderBatch) ([]byte, error)
```

编码路径：

```text
Marshal(batch)
  = Pack(batch)
  + MarshalPacked(packed)
```

`MarshalPacked` 用于重复编码同一份数据，也用于单独测量 native 编码阶段。

默认零值选项：

```go
Options{
    Mode:      ModeAuto,
    Backend:   BackendJIT,
    TrustUTF8: false,
}
```

默认严格校验 UTF-8。`TrustUTF8` 只适用于调用者能够保证所有字符串合法的场景，与 Sonic 默认不校验输入字符串的语义一致。

## 6. PackedBatch

### 6.1 设计原因

Go 的 string、slice 和 pointer 都包含 Go 指针，不能把整个业务对象图交给 C 并让 C 任意访问。`Pack` 将数据转换为只含数值字段的 rows，以及独立的连续字符串池。

字符串引用：

```go
type StringRef struct {
    Offset uint32
    Length uint32
}
```

C 通过 `stringPool[offset:offset+length]` 读取字符串。

### 6.2 OrderRow 布局

`OrderRow` 固定为 96 字节：

```text
offset  size  field
0       8     orderId StringRef
8       8     createdAt StringRef
16      8     status StringRef
24      8     paymentMethod StringRef
32      8     buyerName StringRef
40      8     city StringRef
48      8     address StringRef
56      8     trackingNo StringRef
64      8     remark StringRef
72      4     itemStart
76      4     itemCount
80      8     buyerID
88      1     paid
89      1     hasTrackingNo
90      1     itemsNull
91      5     padding
```

### 6.3 ItemRow 布局

`ItemRow` 固定为 32 字节：

```text
offset  size  field
0       8     sku StringRef
8       8     title StringRef
16      8     qty
24      8     price
```

Go 侧使用 `unsafe.Sizeof` 和 `unsafe.Offsetof` 校验；C 侧使用 `_Static_assert(sizeof/offsetof)` 校验。

### 6.4 状态保留

仅保存 offset/length 无法区分某些 JSON 语义，因此额外保存：

```text
ordersNull：   nil orders 与空 orders
itemsNull：    nil items 与空 items
hasTrackingNo：null trackingNo 与空字符串
```

### 6.5 Pack 校验

`Pack` 执行：

1. 校验所有字符串是有效 UTF-8。
2. 检查字符串池和 item 索引不超过 `uint32`。
3. 构造 order rows。
4. 构造 item rows。
5. 构造连续 string pool。
6. 在打包过程中计算精确输出长度。

容量计算会统计实际字符串转义长度、bool 长度和 `int64` 十进制位数，使输出缓冲区只分配一次且大小与最终 JSON 一致。

公开 `Pack` 会精确预分配独立内存。`Encoder.Marshal` 使用 `sync.Pool` 复用 rows 和 string pool；热路径容量足够时跳过预分配统计。Pack 同时记录整批字符串是否需要转义，全部为普通字符串时 native writer 会直接复制字节。

## 7. cgo ABI

C 入口：

```c
jitjson_status_t jitjson_encoder_encode(
    const jitjson_encoder_t *encoder,
    const jitjson_order_row_t *orders,
    size_t order_count,
    uint8_t orders_null,
    const jitjson_item_row_t *items,
    size_t item_count,
    const uint8_t *strings,
    size_t string_size,
    uint8_t strings_plain,
    uint8_t *output,
    size_t output_cap,
    size_t *written
);
```

一次调用编码完整批次，避免按订单跨越 cgo 边界。

cgo wrapper 使用 `unsafe.SliceData` 取得连续缓冲区地址。调用完成后执行：

```go
runtime.KeepAlive(orders)
runtime.KeepAlive(items)
runtime.KeepAlive(strings)
runtime.KeepAlive(output)
```

约束：

1. C 只在当前同步调用内使用这些指针。
2. C 和 JIT 都不保存 Go 指针。
3. JIT 只调用 C helper，不调用 Go runtime。
4. row 内部不包含 Go 指针。

## 8. C writer

`jitjson_writer_t`：

```c
typedef struct {
    uint8_t *data;
    size_t cap;
    size_t len;
    jitjson_status_t status;
} jitjson_writer_t;
```

所有写入先检查容量。发生错误后设置 `status`，后续 writer 操作直接停止，最终由 bridge 返回状态码。

writer 支持：

```text
固定字面量
int64
bool
普通字符串
nullable 字符串
items 数组
```

字符串转义：

```text
"  -> \"
\  -> \\
\b -> \b
\f -> \f
\n -> \n
\r -> \r
\t -> \t
其他 0x00..0x1f -> \u00xx
```

## 9. JIT 设计

### 9.1 JIT 边界

C bridge 负责批次循环：

```c
for each order:
    encoder->order_fn(writer, order, batch)
```

`order_fn` 可以指向：

- 编译期生成的 `jitjson_encode_order_static`。
- 运行时生成的 JIT 函数。

items 的变长循环仍由静态 C helper 执行。

### 9.2 调用约定

JIT 函数签名：

```c
jitjson_status_t order_fn(
    jitjson_writer_t *writer,
    const jitjson_order_row_t *order,
    const jitjson_batch_view_t *batch
);
```

System V AMD64 ABI 入参：

```text
rdi = writer
rsi = order
rdx = batch
```

JIT prologue 将三个参数保存到 callee-saved 寄存器：

```text
r12 = writer
r13 = order
r14 = batch
```

每次调用 helper 前恢复：

```text
rdi = r12
rsi = r13
rdx = r14
```

当前按顺序调用 8 个操作：

```text
jitjson_op_order_id
jitjson_op_created_at
jitjson_op_status
jitjson_op_payment
jitjson_op_buyer
jitjson_op_shipping
jitjson_op_items
jitjson_op_remark
```

最后从 `writer.status` 读取返回值并恢复寄存器。当前生成机器码为 195 字节。

### 9.3 W^X

JIT 内存生命周期：

```text
mmap RW
  -> emitter 写入机器码
  -> __builtin___clear_cache
  -> mprotect RX
  -> 执行
  -> munmap
```

不会创建同时可写和可执行的长期 RWX 页面。

## 10. SIMD 设计

标量和 AVX2 scanner 的语义相同：返回第一个必须转义的字节位置。

AVX2 每轮处理 32 字节：

```c
bytes    = loadu(data + i)
controls = (bytes & 0xe0) == 0
quotes   = bytes == '"'
slashes  = bytes == '\\'
special  = controls | quotes | slashes
mask     = movemask(special)
```

`(byte & 0xe0) == 0` 只匹配 `0x00..0x1f`。它避免有符号 `int8` 比较把 UTF-8 的 `0x80..0xff` 误判为控制字符。

边界处理：

```text
完整 32 字节块 -> AVX2
不足 32 字节   -> 标量尾部
字符串 < 64    -> 整体使用标量
```

CPU 检测使用 GCC builtin：

```c
__builtin_cpu_init();
__builtin_cpu_supports("avx2");
```

AVX2 函数单独使用 `__attribute__((target("avx2")))`，整个包不要求统一以 `-mavx2` 编译。

## 11. Encoder 生命周期

`NewEncoder`：

1. 校验 mode/backend。
2. 创建 C encoder。
3. 选择标量或 AVX2 字符串 writer。
4. 选择 static 函数或生成 JIT 机器码。

`MarshalPacked`：

1. 取得 `RWMutex` 读锁。
2. 使用 `dirtmake.Bytes` 分配无需预清零、随后会被 native 完整覆盖的输出缓冲区。
3. 调用 native encoder。
4. 检查 `written` 必须等于 Pack 计算的精确长度。

`Close`：

1. 取得写锁，等待正在执行的 Marshal 完成。
2. 释放 JIT 映射和 C encoder。
3. 将实例标记为 closed。

同一个 encoder 可以并发编码，但不能在 `Close` 返回后继续使用。

## 12. 错误模型

Go 公开错误：

```text
ErrClosed
ErrNilPacked
ErrInvalidUTF8
ErrOutputTooLarge
ErrPackedTooLarge
ErrUnsupportedCPU
ErrInvalidMode
ErrNative
```

C 状态：

```text
OK
INVALID_ARGUMENT
NO_SPACE
UNSUPPORTED_CPU
JIT_ALLOC
JIT_PROTECT
INTERNAL
```

native 状态在 Go 层转换为带原因的公开错误。

## 13. 正确性验证

测试矩阵：

| 维度 | 覆盖 |
| --- | --- |
| 后端 | static、JIT |
| 扫描 | scalar、auto、AVX2 |
| 字符串 | ASCII、中文、引号、反斜杠、控制字符、长字符串 |
| 数值 | 正数、负数、`math.MinInt64`、`math.MaxInt64` |
| 容器 | nil orders、空 orders、nil items、空 items |
| nullable | nil、空字符串、普通 trackingNo |
| 生命周期 | Close、重复 Close、并发 Marshal |
| ABI | Go/C row 大小和偏移 |

命令：

```bash
$ go test ./...
$ go test -race ./...
$ go vet ./...
$ GOEXPERIMENT=cgocheck2 go test ./...
$ go test -fuzz=FuzzMarshal -fuzztime=10s ./
$ bash scripts/test_sanitizers.sh
```

sanitizer harness 会直接调用 static/JIT 和 scalar/AVX2 四种 native 组合。

## 14. 性能验证

主 benchmark 将阶段拆开：

```text
encoding/json
JSON Iterator
goccy/go-json
Sonic
Pack
预打包 static/JIT
JIT 严格 UTF-8 端到端
JIT TrustUTF8 端到端
```

必须同时观察：

```text
MarshalPacked：native 编码上限
Marshal：      真实端到端成本
```

当前性能特征：

1. 预打包 JIT 编码为 11.62 ms。
2. 严格 UTF-8 端到端为 25.60 ms，与 Sonic 的 24.64 ms 相差约 3.9%。
3. `TrustUTF8` 端到端为 22.37 ms，比 Sonic 耗时低约 9.2%。
4. 当前真实数据的最长字符串只有 17 字节，不会触发 64 字节阈值的 AVX2 扫描。
5. 本轮收益来自 scratch 复用、无转义 fast path、跳过热路径容量预扫描和无需预清零的输出分配。

真实数据多库对比、合成数据结果和测试方法见 [`benchmark-results.md`](benchmark-results.md)。

## 15. 文件映射

```text
model.go                              Go schema
format.go                             JSON 固定字面量
packed.go                             PackedBatch 和容量计算
reference.go                          纯 Go reference
encoder.go                            公开 API 和生命周期
internal/native/types.go              Go row ABI
internal/native/jitjson.h              C 公开 ABI
internal/native/internal.h             C 内部结构
internal/native/bridge_linux_amd64.c   encoder 和批次循环
internal/native/writer_linux_amd64.c   JSON writer
internal/native/simd_linux_amd64.c     scalar/AVX2 scanner
internal/native/jit_linux_amd64.c      x86-64 emitter 和 W^X
cmd/demo/main.go                       2.5 万条合成数据示例
realdata_test.go                       真实数据正确性和统计
realdata_benchmark_test.go             多库真实数据 benchmark
testdata/orders.json                   真实业务测试数据
```

## 16. 后续优化方向

按收益优先级排列：

1. 研究直接读取 Go 对象的生成代码，最终消除 string pool 复制。
2. 对混合数据增加 per-string 无转义标记，避免一个特殊字符串关闭整批 fast path。
3. 将更多订单操作直接内联到 JIT 机器码，减少 helper 调用。
4. 单独优化多商品订单的 items 循环。
5. 增加 caller-owned 输出缓冲区 API，减少结果分配。
6. 使用额外的长字符串数据集继续验证和调整 SIMD 阈值。

在这些工作之前，不应仅通过扩大测试字符串来制造更高的 SIMD 加速比。
