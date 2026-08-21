# 小白阅读指南

这篇文档只回答两个问题：

1. 一次 JSON 序列化在这个项目里是怎样完成的？
2. 性能测试中的数字应该怎样比较？

读完本文后，再阅读[最终工程设计](engineering-design-order-batch.md)会容易很多。

## 1. 先用一句话理解项目

这个项目把固定的 Go 订单结构转换为 JSON，但没有完全使用 Go 完成，而是故意经过下面这条路径：

```text
Go 订单
  -> 整理成 C 能安全读取的连续数据
  -> 一次 cgo 调用
  -> C/JIT 写出 JSON
  -> AVX2 在合适的长字符串中查找转义字符
```

项目的主要目的不是替代所有 JSON 库，而是学习和验证：

```text
Go 与 C 如何传递批量数据
JIT 如何在运行时生成机器码
SIMD 适合加速 JSON 的哪一部分
怎样公平地进行端到端性能测试
```

## 2. 什么是 JSON 序列化

序列化就是把内存中的 Go 值转换为 JSON 字节。

为了说明概念，先只看订单中的两个字段。输入：

```go
OrderBatch{
    Orders: []Order{
        {
            OrderID: "O1",
            Status:  "paid",
        },
    },
}
```

输出中会出现：

```text
"orderId":"O1"
"status":"paid"
```

真实结构还包含 `payment`、`buyer`、`shipping` 和 `items`。完整定义见 [`model.go`](../model.go)。

## 3. 为什么不能直接把 Go 对象交给 C

Go 的 `string` 和 slice 并不只保存数据本身，它们还包含指向 Go 内存的指针。

例如：

```go
type Order struct {
    OrderID string
    Items   []Item
}
```

可以先粗略理解为：

```text
OrderID -> 指向字符串数据
Items   -> 指向 Item 数组
```

Go 垃圾回收器需要管理这些指针。为了遵守 cgo 指针规则，项目不会让 C 任意遍历原始 `OrderBatch`，而是先执行 `Pack`。

## 4. Pack 做了什么

`Pack` 把嵌套的 Go 对象整理成三块连续数据：

```text
[]OrderRow
[]ItemRow
[]byte stringPool
```

字符串不会直接放进 row，而是变成：

```go
type StringRef struct {
    Offset uint32
    Length uint32
}
```

例如字符串池为：

```text
O1paid北京
```

那么 `"paid"` 可以表示为：

```text
Offset = 2
Length = 4
```

C 根据 offset 和 length 就能找到字符串，不需要持有 Go string。

默认模式下，`Pack` 还负责：

- 校验 UTF-8。
- 保留 `null` 与空字符串的区别。
- 保留 nil slice 与空 slice 的区别。
- 计算最终 JSON 的精确长度。
- 判断整批字符串是否都不需要转义。

这里的代价是：每次普通 `Marshal` 都要先遍历一次订单并复制字符串。

## 5. 一次 Marshal 的完整过程

调用：

```go
output, err := encoder.Marshal(batch)
```

内部过程是：

```text
1. 从 sync.Pool 取得可复用工作区
2. Pack Go 订单
3. 分配最终输出
4. 进入一次 cgo 调用
5. C 遍历全部订单
6. static 或 JIT 函数写每个订单
7. 返回 JSON 字节
8. 归还 Pack 工作区
```

因此：

```text
Marshal = Pack + MarshalPacked
```

`MarshalPacked` 只包含后半段 native 编码。它适合研究 C/JIT 的编码能力，但不能单独代表普通用户看到的完整性能。

## 6. JIT 在哪里

创建 `BackendJIT` encoder 时，程序会在运行时生成一段 195 字节的 x86-64 机器码。

它按固定顺序处理：

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

机器码经过：

```text
mmap RW
  -> 写入机器码
  -> mprotect RX
  -> 执行
  -> Close 时 munmap
```

这是真正的 JIT，但当前 JIT 主要负责调用 8 个 C helper，并没有把字符串转义、整数格式化和 items 循环全部内联成机器码。

所以：

```text
使用了 JIT
不等于
一定比静态 C 更快
```

## 7. SIMD 在哪里

字符串写入 JSON 前，需要查找：

```text
控制字符 0x00..0x1f
双引号 "
反斜杠 \
```

标量扫描一次检查一个字节。AVX2 一次可以检查 32 字节。

但是使用 AVX2 也有准备成本，因此当前规则是：

```text
字符串长度 < 64 字节  -> 标量扫描
字符串长度 >= 64 字节 -> AVX2 扫描
```

真实测试数据中最长字符串只有 17 字节，所以真实订单 benchmark 没有使用 AVX2。该测试反映的是 Pack、JIT 和内存优化，不是 SIMD 收益。

单独的长字符串 benchmark 显示：

```text
16/32 字节：没有稳定收益
256 字节：  AVX2 约快 1.46 倍
4096 字节： AVX2 约快 3.33 倍
```

## 8. 怎样看性能表

常见指标：

| 指标 | 含义 | 判断方式 |
| --- | --- | --- |
| `ns/op` | 每次操作多少纳秒 | 越低越好 |
| `ms/op` | 每次操作多少毫秒 | 越低越好 |
| `MB/s` | 每秒处理多少 MB | 越高越好 |
| `B/op` | 每次操作分配多少字节 | 通常越低越好 |
| `allocs/op` | 每次操作发生多少次分配 | 通常越低越好 |

比较 JSON 库时，应先看端到端结果：

| 实现 | 端到端耗时 |
| --- | ---: |
| goccy/go-json | 20.50 ms |
| jitjson `TrustUTF8` | 22.37 ms |
| Sonic | 24.64 ms |
| jitjson 严格 UTF-8 | 25.60 ms |
| JSON Iterator | 33.24 ms |
| `encoding/json` | 35.48 ms |

这张表包含了普通调用者实际支付的完整成本。

下面这些是内部阶段，不能直接当作完整库性能：

| 内部阶段 | 耗时 |
| --- | ---: |
| Pack only | 17.48 ms |
| 预打包 static | 11.06 ms |
| 预打包 JIT | 11.62 ms |

不能用 `11.62 ms` 宣称 jitjson 比其他库快，因为普通 `Marshal` 之前还要执行 Pack。

## 9. 严格 UTF-8 与 TrustUTF8

默认模式会验证每个输入字符串：

```go
encoder, err := jitjson.NewEncoder(jitjson.Options{})
```

调用者能够保证全部字符串已经是合法 UTF-8 时，可以跳过验证：

```go
encoder, err := jitjson.NewEncoder(jitjson.Options{
    TrustUTF8: true,
})
```

!!! warning 注意 `TrustUTF8` 只适用于可信输入。非法 UTF-8 可能直接进入输出并形成非法 JSON。!!!

Sonic 的本次对比使用默认配置，没有额外开启字符串验证。因此 `TrustUTF8` 与 Sonic 的语义更接近；严格模式则多做了一项安全检查。

## 10. 为什么 JIT 没有带来数量级提升

性能不只取决于计算速度，还取决于数据搬运。

当前路径包含：

```text
读取 Go 对象
复制字符串到 string pool
构造 rows
跨越 cgo
再次写入最终 JSON
```

Sonic 的生成代码可以直接读取 Go 对象并写输出，没有 PackedBatch 这层转换。

经过优化后，本项目已经通过工作区复用、无转义 fast path 和无需预清零的输出缓冲区降低了开销，但字符串池复制仍然存在。

## 11. 正确性怎样保证

项目不会只看 benchmark，还会检查：

```text
native 输出 == 纯 Go reference
纯 Go reference == encoding/json
JIT 输出 == static C 输出
AVX2 扫描 == 标量扫描
```

还使用：

```text
go test
race detector
go vet
cgocheck2
fuzz
AddressSanitizer
UndefinedBehaviorSanitizer
```

正式测试命令见 [README](../README.md#验证)。

## 12. 推荐阅读顺序

第一次阅读：

1. [`model.go`](../model.go)：输入数据是什么。
2. [`packed.go`](../packed.go)：Go 对象怎样变成 C ABI。
3. [`encoder.go`](../encoder.go)：公开 API 和完整调用流程。
4. [`internal/native/bridge_linux_amd64.c`](../internal/native/bridge_linux_amd64.c)：一次 cgo 调用怎样编码整个批次。
5. [`internal/native/writer_linux_amd64.c`](../internal/native/writer_linux_amd64.c)：JSON 字段怎样写入。
6. [`internal/native/jit_linux_amd64.c`](../internal/native/jit_linux_amd64.c)：机器码怎样生成。
7. [`internal/native/simd_linux_amd64.c`](../internal/native/simd_linux_amd64.c)：AVX2 怎样扫描字符串。

需要查布局、ABI 和生命周期时，再阅读[最终工程设计](engineering-design-order-batch.md)。

需要复现实验时，阅读[Benchmark 结果](benchmark-results.md)。

## 13. 当前边界

这个项目目前：

- 只支持固定 `OrderBatch`。
- 只在 Linux/amd64/cgo 环境运行 native 后端。
- 不支持任意 Go 类型和完整 JSON tag。
- 不支持 map、interface、float、`omitempty` 和自定义 Marshaler。
- 不提供 JSON 解码。

M 芯片 Mac 可以阅读、修改和推送代码，但当前不能运行 Linux x86-64 JIT 后端。

它是完整的学习项目和性能实验，不是通用生产 JSON 库。
