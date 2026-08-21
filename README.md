# Go JIT + SIMD JSON 序列化 Demo

这是一个面向学习和性能分析的 Go JSON 序列化项目。它使用：

```text
Go API
  -> PackedBatch
  -> 单次 cgo 调用
  -> C 批量编码器
  -> 运行时生成的 x86-64 机器码
  -> 标量或 AVX2 字符串扫描
```

项目已经完成可运行版本，不是 `encoding/json` 的通用替代品。当前只编码固定的订单批次结构，以便把重点放在 cgo ABI、JIT、SIMD、内存安全和性能验证上。

## 支持范围

运行环境：

```text
操作系统：Linux
架构：    amd64
Go：      1.24 或更高
构建：    CGO_ENABLED=1，并安装 GCC
SIMD：    AVX2 可选，不支持时自动回退到标量路径
```

已支持：

- 顶层 `orders` 数组。
- `payment`、`buyer`、`shipping` 嵌套对象。
- `items` 变长对象数组。
- UTF-8 字符串、`int64`、布尔值和 `null`。
- JSON 基础字符串转义。
- nil slice 与空 slice 的区别。
- 静态 C 和 JIT 两种订单编码后端。
- 标量和 AVX2 两种字符串扫描模式。
- 并发复用同一个 `Encoder`。

暂不支持：

- 任意 Go 类型、反射编译和 JSON tag 解析。
- `omitempty`、匿名字段和自定义 `json.Marshaler`。
- `float`、map、interface 和解码。
- HTML 转义、无效 UTF-8 修复、U+2028/U+2029 特殊转义。
- Linux/amd64/cgo 之外的平台。

## 数据模型

最终测试结构见 [订单数据结构](docs/order-schema-proposal.md)。主要类型如下：

```go
type OrderBatch struct {
    Orders []Order `json:"orders"`
}

type Order struct {
    OrderID   string
    CreatedAt string
    Status    string
    Payment   Payment
    Buyer     Buyer
    Shipping  Shipping
    Items     []Item
    Remark    string
}
```

`trackingNo` 使用 `*string` 区分 `null` 和空字符串。商品价格保留为整数，不引入浮点数。

## 快速运行

克隆并进入项目目录：

```bash
$ git clone https://github.com/bing-bing-student/study_go_jit_simd_json.git
$ cd study_go_jit_simd_json
```

运行默认的 2.5 万条合成订单 demo：

```bash
$ go run ./cmd/demo
```

调整订单数量：

```bash
$ go run ./cmd/demo -orders 1000
```

将编码结果写入文件：

```bash
$ go run ./cmd/demo -orders 25000 -output orders.json
```

demo 会分别统计 `Pack` 和 JIT 编码耗时，并将最终字节与 `encoding/json` 逐字节比较。

## API 用法

默认选项是 `BackendJIT + ModeAuto`：

```go
encoder, err := jitjson.NewEncoder(jitjson.Options{})
if err != nil {
    return err
}
defer encoder.Close()

output, err := encoder.Marshal(batch)
```

默认会校验所有输入字符串的 UTF-8。调用者能够保证字符串合法时，可以使用与 Sonic 默认语义一致的可信模式：

```go
encoder, err := jitjson.NewEncoder(jitjson.Options{TrustUTF8: true})
```

!!! warning 注意 `TrustUTF8` 会跳过 UTF-8 校验；传入非法字符串可能生成非法 JSON，只能在输入已经受控时启用。!!!

`Marshal` 包含打包和 native 编码两个阶段：

```text
Marshal = Pack + MarshalPacked
```

同一批数据需要重复编码时，可以只打包一次：

```go
packed, err := jitjson.Pack(batch)
if err != nil {
    return err
}

output, err := encoder.MarshalPacked(packed)
```

可选模式：

| 配置 | 含义 |
| --- | --- |
| `ModeAuto` | CPU 支持 AVX2 时使用 AVX2，否则使用标量扫描 |
| `ModeScalar` | 强制使用标量字符串扫描 |
| `ModeAVX2` | 强制使用 AVX2，不支持时返回 `ErrUnsupportedCPU` |
| `BackendJIT` | 运行时生成订单字段编码调用序列 |
| `BackendStatic` | 使用编译期生成的静态 C 订单编码函数 |

`Encoder` 可以并发调用 `Marshal` 或 `MarshalPacked`，但使用完必须调用 `Close` 释放 native encoder 和 JIT 映射。

## JIT 做了什么

创建 JIT encoder 时，C 在运行时生成一段符合 System V AMD64 ABI 的机器码。当前机器码依次调用 8 个订单字段操作：

```text
orderId
createdAt
status
payment
buyer
shipping
items
remark
```

机器码通过以下过程创建：

```text
mmap(PROT_READ | PROT_WRITE)
  -> 写入机器码
  -> mprotect(PROT_READ | PROT_EXEC)
  -> 通过函数指针执行
  -> Close 时 munmap
```

这是真正的运行时机器码生成，但目前仍是“helper 调用序列”。订单批次循环和 items 循环保留在静态 C 中，因此 JIT 后端不一定比静态 C 更快。

## SIMD 做了什么

AVX2 用于查找需要 JSON 转义的字节：

```text
0x00..0x1f
双引号 "
反斜杠 \
```

扫描器每次读取 32 字节，通过比较和 `movemask` 找到第一个特殊字符。长度小于 64 字节的字符串直接走标量路径，避免 SIMD 准备成本超过收益。

数字格式化、固定字面量写入和数组循环仍使用标量代码。

## PackedBatch

Go 不会把含有 string、slice 或其他 Go 指针的业务结构体直接交给 C。`Pack` 会生成三块连续数据：

```text
[]OrderRow
[]ItemRow
[]byte stringPool
```

字符串在 row 中表示为：

```go
type StringRef struct {
    Offset uint32
    Length uint32
}
```

`OrderRow` 固定为 96 字节，`ItemRow` 固定为 32 字节。Go 使用 `unsafe.Sizeof/Offsetof` 校验布局，C 使用 `_Static_assert` 校验同一份 ABI。

整个批次只跨一次 cgo 边界。C 不保存 Go 指针，Go 在调用后使用 `runtime.KeepAlive` 保证参数生命周期。

## 验证

普通测试：

```bash
$ go test ./...
```

完整验证：

```bash
$ go test -race ./...
$ go vet ./...
$ GOEXPERIMENT=cgocheck2 go test ./...
$ go test -tags benchmark -run '^TestExternalRealDataMarshalers$' ./
$ go test -fuzz=FuzzMarshal -fuzztime=10s ./
$ bash scripts/test_sanitizers.sh
```

测试覆盖静态/JIT、标量/自动/AVX2、nil/empty、nullable 字符串、中文、转义、`int64` 边界、并发调用和 Go/C 布局。

## Benchmark

运行 `testdata/orders.json` 的多库对比：

```bash
$ GOMAXPROCS=8 go test -tags benchmark -run '^$' -bench '^BenchmarkRealDataMarshal$' -benchmem -benchtime=2s -count=5
```

运行主 benchmark：

```bash
$ go test -run '^$' -bench '^BenchmarkMarshal$' -benchmem
```

运行长字符串 SIMD 对照：

```bash
$ go test -run '^$' -bench '^BenchmarkLongStringSIMD$' -benchmem
```

benchmark 会分别报告：

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

当前结论是：

- 真实数据包含 25,385 条订单，紧凑输出为 7,766,179 字节。
- 真实数据中最长字符串只有 17 字节，不会触发当前 64 字节阈值的 AVX2 扫描。
- 严格 UTF-8 的 jitjson 为 25.60 ms，Sonic 为 24.64 ms；两者相差约 3.9%。
- 与 Sonic 默认语义一致的 `TrustUTF8` 模式为 22.37 ms，比 Sonic 耗时低约 9.2%。
- 同轮标准库为 35.48 ms，JSON Iterator 为 33.24 ms，goccy/go-json 为 20.50 ms。
- 预打包后的 native 编码明显快于 `encoding/json`。
- 完整 `Marshal` 的主要成本仍在 `Pack`，性能会随 schema 和数据分布变化。
- 静态 C 通常略快于当前 JIT helper 调用序列。
- AVX2 对 256 字节以上的普通字符串收益明显，对短字符串没有优势。

具体数字依赖 CPU、编译器和数据分布，应以本机 benchmark 为准。

## 项目结构

```text
.
├── cmd/demo/                    # 2.5 万条合成订单示例
├── docs/                        # schema、评估和最终工程设计
├── internal/native/             # cgo、C ABI、writer、SIMD 和 JIT
├── scripts/                     # 数据简化和 sanitizer 脚本
├── testdata/orders.json         # 25,385 条真实业务测试数据
├── tests/                       # 独立 C sanitizer harness
├── encoder.go                   # 公开 Encoder API
├── packed.go                    # Go -> PackedBatch
├── realdata_benchmark_test.go   # 多库真实数据 benchmark
├── realdata_test.go             # 真实数据正确性和统计
├── reference.go                 # 纯 Go 正确性基线
└── *_test.go                    # 单测、fuzz 和 benchmark
```

## 文档

- [最终工程设计](docs/engineering-design-order-batch.md)
- [Benchmark 结果](docs/benchmark-results.md)
- [工程实践评估](docs/assessment.md)
- [订单数据结构](docs/order-schema-proposal.md)
- [已归档的早期单 Record 设计](docs/engineering-design-cgo.md)

## 参考资料

- [Sonic](https://github.com/bytedance/sonic)：成熟的 Go JIT + SIMD JSON 实现，可用于对照真实工程复杂度。
- [Sonic 中文设计介绍](https://github.com/bytedance/sonic/blob/main/docs/INTRODUCTION_ZH_CN.md)：介绍 JIT 和 SIMD 在 JSON 编解码中的职责。
- [cgo 官方文档](https://go.dev/cmd/cgo/)：说明 Go 与 C 的调用方式及指针传递约束。
- [Intel Intrinsics Guide](https://www.intel.com/content/www/us/en/docs/intrinsics-guide/index.html)：查询 AVX2 intrinsic、参数和对应指令。
