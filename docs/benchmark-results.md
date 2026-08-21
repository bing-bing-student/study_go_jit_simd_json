# Benchmark 结果

## 测试环境

```text
日期：2026-08-20
系统：Linux 5.4.143.bsk.8-amd64
CPU： Intel Xeon Platinum 8260 @ 2.40 GHz
Go：  1.24.4
GCC： 8.3.0
并发：GOMAXPROCS=8
```

命令：

```bash
$ GOMAXPROCS=8 go test -tags benchmark -run '^$' -bench '^BenchmarkRealDataMarshal$' -benchmem -benchtime=2s -count=5
$ go test -tags benchmark -run '^TestExternalRealDataMarshalers$' ./
$ go test -run '^$' -bench '^BenchmarkMarshal$' -benchmem -benchtime=1s -count=3
$ go test -run '^$' -bench '^BenchmarkLongStringSIMD$' -benchmem -benchtime=1s -count=3
$ go run ./cmd/demo -orders 25000
```

表格使用多轮结果的中位数。耗时越低越好。

## 真实业务数据

文件：`testdata/orders.json`

```text
源文件大小：          13,807,817 bytes
订单数量：            25,385
商品数量：            25,385
紧凑 JSON 大小：      7,766,179 bytes
字符串池大小：        2,101,414 bytes
字符串数量：          275,608
trackingNo 为 null：  3,627
最长字符串：          17 bytes
长度 >= 64 的字符串： 0
```

对比版本：

| 实现 | 版本 |
| --- | --- |
| Go `encoding/json` | Go 1.24.4 |
| JSON Iterator | v1.1.12 |
| goccy/go-json | v0.10.6 |
| Sonic | v1.15.2 |
| jitjson | 当前工作区 |

所有实现的输出均已与 `encoding/json` 逐字节比较。文件读取、JSON 解析、首次缓存和首次 JIT 编译不在计时区间。

以下结果是固定 `GOMAXPROCS=8`、每组 2 秒、运行 5 轮后的中位数：

| 路径 | ms/op | MB/s | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| goccy/go-json | 20.50 | 378.76 | 8,191,486 | 2 |
| jitjson JIT auto + Pack，`TrustUTF8` | 22.37 | 347.18 | 7,774,419 | 2 |
| Sonic | 24.64 | 315.19 | 41,698,155 | 38 |
| jitjson JIT auto + Pack，严格 UTF-8 | 25.60 | 303.37 | 7,774,421 | 2 |
| JSON Iterator | 33.24 | 233.64 | 7,774,428 | 2 |
| `encoding/json` | 35.48 | 218.89 | 7,774,425 | 2 |
| jitjson Pack only | 17.48 | 444.38 | 5,365,858 | 4 |
| jitjson static auto，预打包 | 11.06 | 702.34 | 7,774,219 | 2 |
| jitjson JIT scalar，预打包 | 11.29 | 687.78 | 7,774,218 | 2 |
| jitjson JIT auto，预打包 | 11.62 | 668.54 | 7,774,219 | 2 |

结论：

1. goccy/go-json 仍是本组最快的通用库。
2. jitjson 严格 UTF-8 模式为 25.60 ms，与 Sonic 的 24.64 ms 相差约 3.9%，属于同一性能档。
3. Sonic 默认不校验输入字符串 UTF-8。与其默认语义一致的 jitjson `TrustUTF8` 为 22.37 ms，耗时比 Sonic 低约 9.2%。
4. jitjson 预打包 JIT 路径为 11.62 ms，但该数字不包含 Pack，不能替代端到端结果。
5. 真实数据没有任何字符串达到 64 字节，因此当前收益来自减少复制、分配和重复扫描，而不是 AVX2。
6. 当前数据中每个订单恰好一个商品，不能代表多商品订单下的数组循环性能。

### 本轮优化效果

根据 Sonic 源码和端到端 profile，新增以下优化：

- Encoder 使用 `sync.Pool` 复用 OrderRow、ItemRow 和 string pool 工作区。
- 热路径复用容量时跳过仅用于首次预分配的对象图统计。
- Pack 记录整批字符串是否需要 JSON 转义；全为普通字符串时，native writer 不再重复扫描。
- 复用 OrderRow 时只重置条件字段，不再清零整块 row buffer。
- 使用与 Sonic 相同的 `dirtmake.Bytes` 分配无需预清零、随后会被 native 完整覆盖的输出缓冲区。
- 增加显式 `TrustUTF8`，在调用者保证输入合法时跳过 UTF-8 校验。

| 指标 | 最初实现 | 当前严格模式 | 当前 TrustUTF8 |
| --- | ---: | ---: | ---: |
| 端到端 | 38.03 ms | 25.60 ms | 22.37 ms |
| 端到端分配字节 | 22.99 MB | 7.77 MB | 7.77 MB |
| 端到端分配次数 | 41 | 2 | 2 |
| 预打包 JIT | 17.94 ms | 11.62 ms | 11.62 ms |

`TrustUTF8` 不改变字符串转义逻辑，只跳过合法性校验。非法 UTF-8 可能原样进入输出，因此默认保持关闭。

Sonic loader 与 `GOEXPERIMENT=cgocheck2` 不兼容，因此外部库对比放在 `benchmark` build tag 中。核心 jitjson 测试仍独立通过 `cgocheck2`。

## 合成 100 条订单

| 路径 | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `encoding/json` | 166,502 | 41,072 | 2 |
| 纯 Go reference | 123,642 | 40,960 | 1 |
| `Pack` | 83,361 | 29,920 | 4 |
| static scalar | 84,160 | 40,968 | 2 |
| JIT scalar | 82,757 | 40,968 | 2 |
| static AVX2 | 80,003 | 40,968 | 2 |
| JIT AVX2 | 79,812 | 40,968 | 2 |
| JIT AVX2 + Pack | 148,103 | 40,993 | 2 |

结论：

1. 预打包的 JIT AVX2 编码约为 `encoding/json` 的 2.09 倍吞吐。
2. 公开 `Pack` 为 83.4 µs；Encoder 热路径还会复用其工作区。
3. 完整 `Pack + JIT AVX2` 比 `encoding/json` 耗时低约 11.1%。
4. static 与 JIT 位于同一性能区间，差异受运行波动影响。
5. native 路径的主要分配来自最终输出；机器码在 `NewEncoder` 时只生成一次。

## 长字符串 SIMD

| remark 长度 | scalar ns/op | AVX2 ns/op | scalar / AVX2 |
| --- | ---: | ---: | ---: |
| 16 bytes | 753.0 | 754.2 | 1.00x |
| 32 bytes | 785.9 | 798.0 | 0.98x |
| 256 bytes | 1,235 | 847.9 | 1.46x |
| 4096 bytes | 9,142 | 2,743 | 3.33x |

每个测试字符串末尾包含一个换行符，确保不会走整批无转义 fast path。16 和 32 字节的差异接近噪声范围，不能据此宣称 SIMD 有收益。256 字节开始出现稳定优势，4096 字节时收益明显。

当前 C writer 对小于 64 字节的单个字符串使用标量扫描。表中的 16/32 字节 AVX2 模式实际也会走该标量阈值，因此两组只反映模式分发和测量波动。

## 2.5 万条合成订单

```text
订单数量：      25000
商品数量：      49999
字符串池：      3675949 bytes
JSON 大小：     10440225 bytes
Pack 耗时：     26.442111ms
JIT 编码耗时：  23.319506ms
JIT 代码大小：  195 bytes
CPU 支持 AVX2： true
结果验证：      通过
```

这组数据由 `cmd/demo` 合成，用于补充多商品、长字符串和转义场景。真实业务性能结论以 `testdata/orders.json` 的多库 benchmark 为准。

## 解释边界

- `MarshalPacked` 代表预处理完成后的 native 编码能力。
- `Marshal` 才代表普通调用者看到的端到端成本。
- JIT 创建成本不包含在热调用 benchmark 中。
- 结果只适用于当前 CPU、编译器、schema 和数据分布。
- SIMD 对长普通字符串有优势，不代表所有订单字段都能加速。
