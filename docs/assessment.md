# 工程实践评估

## 结论

这个项目已经完成 demo 目标：

```text
固定 OrderBatch
+ PackedBatch
+ 单次 cgo 调用
+ 静态 C 对照后端
+ 真正的 x86-64 JIT
+ AVX2 字符串扫描
+ 标量回退
+ 完整正确性和性能验证
```

它适合研究 JIT、SIMD 和 Go/C 协作，但不应作为生产环境中 `encoding/json` 的替代品。

## 复杂度来源

订单结构本身不复杂，真正的工程难点来自多个边界同时存在：

1. Go 数据需要转换为不含 Go 指针的连续内存。
2. Go 和 C 必须对 row 大小、字段偏移和 padding 达成一致。
3. JSON 字符串转义必须覆盖所有控制字符。
4. AVX2 不能越界读取，也不能把 UTF-8 高位字节误判为控制字符。
5. JIT 机器码必须遵守 System V AMD64 ABI。
6. JIT 内存需要执行 W^X，不能长期保持 RWX。
7. `Marshal`、`Close` 和并发调用需要正确协调。
8. 性能分析必须把 `Pack`、cgo、JIT 和 SIMD 分开测量。

## 当前边界

### 固定 schema

项目只支持已经冻结的 `OrderBatch`。固定 schema 的价值在于：

- 字段顺序和字段类型已知。
- 不需要在热路径使用反射。
- JIT 可以生成确定的订单字段操作序列。
- Go/C ABI 可以用固定布局校验。

代价是任何字段变更都需要同步修改：

```text
Go model
PackedBatch
Go/C row ABI
C writer
JIT operation sequence
reference encoder
tests
```

### Linux/amd64/cgo

当前 JIT emitter 直接生成 x86-64 机器码，并使用 Linux 的 `mmap`、`mprotect` 和 `munmap`。其他平台只提供不支持的占位实现。

### JSON 语义

已支持：

```text
有效 UTF-8
基础 JSON 字符串转义
int64
bool
null
nil slice 与空 slice
```

未实现：

```text
HTML 转义
无效 UTF-8 替换
U+2028/U+2029 特殊转义
omitempty
自定义 Marshaler
```

因此项目输出可以是合法 JSON，并与固定 schema 的 Go 值保持语义一致，但对所有字符串不保证与 `encoding/json` 逐字节一致。

## 架构取舍

### 为什么使用 PackedBatch

不能把含有 string、slice 和 pointer 的 `OrderBatch` 直接交给 C。当前做法是：

```text
OrderBatch
  -> []OrderRow
  -> []ItemRow
  -> []byte stringPool
```

row 内只包含整数、布尔标志、索引和字符串 offset/length，不包含 Go 指针。三块数据只在一次同步 cgo 调用期间使用，C 不保存它们。

这比逐订单调用 cgo 更合理：

```text
错误方向：25000 条订单 -> 25000 次 cgo 调用
当前方向：25000 条订单 -> 1 次 cgo 调用
```

### 为什么同时保留 static 和 JIT

静态 C 后端是正确性和性能对照。JIT 后端生成 8 个 helper 的调用序列：

```text
orderId -> createdAt -> status -> payment -> buyer
-> shipping -> items -> remark
```

这样可以验证真正的机器码生成、ABI 和 W^X，而无需一开始就在机器码 emitter 中实现字符串转义、整数格式化和变长数组循环。

该设计也解释了当前结果：静态 C 编译器可以内联和优化完整函数，而 JIT 每个订单需要调用多个 helper，所以 JIT 不一定更快。

### 为什么 SIMD 只扫描字符串

JSON 字符串中的普通 UTF-8 字节可直接复制，需要定位的是：

```text
0x00..0x1f
"
\
```

AVX2 一次检查 32 字节，适合较长、转义字符较少的字符串。单个 `int64` 格式化、布尔值和短字段名不适合为了使用 SIMD 而强行向量化。

实测短字符串走 AVX2 可能更慢，因此当前阈值为 64 字节。

## 正确性策略

项目保留三个互相独立的实现层：

```text
encoding/json
纯 Go reference
C static/JIT encoder
```

验证规则：

1. 支持范围内，纯 Go reference 与 `encoding/json` 对照。
2. 所有 native 模式必须与纯 Go reference 逐字节一致。
3. SIMD 必须与标量扫描结果一致。
4. JIT 必须与静态 C 输出一致。
5. 输出必须通过 `json.Valid` 和反序列化回环检查。

测试工具：

```bash
$ go test ./...
$ go test -race ./...
$ go vet ./...
$ GOEXPERIMENT=cgocheck2 go test ./...
$ go test -fuzz=FuzzMarshal -fuzztime=10s ./
$ bash scripts/test_sanitizers.sh
```

这些测试分别覆盖 Go 逻辑、并发、cgo 指针规则、随机字符串输入以及 C 的地址和未定义行为检查。

## 性能结论

性能必须拆成两个视角。

### 预打包编码

`MarshalPacked` 只测：

```text
一次 cgo 调用
+ C writer
+ static/JIT 订单编码
+ scalar/AVX2 扫描
```

当前预打包 native 路径明显快于 `encoding/json`。

### 端到端编码

`Marshal` 测：

```text
Pack + MarshalPacked
```

当前主要额外成本仍来自 Go 对象到 C ABI 的转换：

- 遍历完整对象图并构造 order/item rows。
- 将所有字符串复制到连续 string pool。
- 默认校验每个字符串的 UTF-8。
- 计算精确输出长度。

Encoder 现在会复用 Pack 工作区，并把“整批字符串无需转义”的结论传给 native writer。真实数据中，严格 UTF-8 模式为 25.60 ms，与 Sonic 的 24.64 ms 相差约 3.9%；调用者保证输入合法时，`TrustUTF8` 模式为 22.37 ms。

两者的 JIT 数据路径并不相同：Sonic 生成的代码直接读取 Go 对象并写输出；jitjson 仍先转换成无 Go 指针的 C ABI。当前优化减少了这层转换的分配和重复扫描，但没有消除字符串池复制。

### SIMD 适用范围

当前观察：

```text
16/32 字节：标量通常更快
256 字节：  AVX2 开始有明确收益
4096 字节： AVX2 收益明显
```

真实数据的 275,608 个字符串全部短于 64 字节，因此没有触发 AVX2。较长的 remark、地址或大文本字段才是主要受益者。

## 风险状态

| 风险 | 当前处理 |
| --- | --- |
| cgo 保存 Go 指针 | 单次同步调用，C 不保存指针，调用后 `runtime.KeepAlive` |
| Go/C 布局不一致 | Go `unsafe.Sizeof/Offsetof` + C `_Static_assert` |
| AVX2 越界读取 | 只处理完整 32 字节块，尾部回到标量 |
| UTF-8 高位字节误判 | 使用 `(byte & 0xe0) == 0` 判断控制字符 |
| JIT 内存权限 | RW 写入后切换到 RX |
| JIT 生命周期 | `Encoder.Close` 与 `Marshal` 使用读写锁协调 |
| 输出缓冲区不足 | `Pack` 计算精确长度，C writer 每次写入仍检查边界 |
| nullable/empty 混淆 | 单独保存 `hasTrackingNo`、`ordersNull`、`itemsNull` |

## 生产化差距

如果要从 demo 演进为生产库，至少还需要：

1. 基于 Go 类型的 schema 编译和缓存。
2. 完整 `encoding/json` 语义兼容。
3. 更多类型、tag、Marshaler 和错误语义。
4. 多架构 emitter 或可靠的静态回退。
5. 更少复制的 Pack 方案或代码生成 adapter。
6. JIT 中更深的内联，减少 helper 调用。
7. 大规模真实数据、故障注入和长期并发压力测试。
8. 完整安全审计、模糊测试语料和发布兼容策略。

这些工作已经超出本学习项目的范围。
