# 订单测试数据结构

> 状态：已确认并实现。本文保留字段取舍和真实数据准备方法；最终 ABI 见 [OrderBatch 最终工程设计](engineering-design-order-batch.md)。

## 1. 目标

现有订单数据来自真实业务，约 2.5 万条，具有较好的代表性。但原始结构字段较多，其中很多字段对 JSON 序列化器来说只是重复测试同一种字符串或整数类型。

简化目标不是把数据变成玩具，而是保留以下典型 JSON 能力：

```text
顶层对象
对象数组
嵌套对象
嵌套对象数组
普通字符串
中文 UTF-8
整数
负数
布尔值
null
字符串转义
```

同时控制第一版 JIT、cgo 和 SIMD 的工程复杂度。

## 2. 推荐结构

```json
{
  "orders": [
    {
      "orderId": "O0",
      "createdAt": "2024-01-01",
      "status": "pending",
      "payment": {
        "method": "ali",
        "paid": false
      },
      "buyer": {
        "id": 0,
        "name": "张伟"
      },
      "shipping": {
        "city": "北京",
        "address": "北京朝阳0号",
        "trackingNo": null
      },
      "items": [
        {
          "sku": "S0",
          "title": "JM键盘",
          "qty": 1,
          "price": 1
        }
      ],
      "remark": "工作日送"
    }
  ]
}
```

对应 Go 类型：

```go
type OrderBatch struct {
    Orders []Order `json:"orders"`
}

type Order struct {
    OrderID   string   `json:"orderId"`
    CreatedAt string   `json:"createdAt"`
    Status    string   `json:"status"`
    Payment   Payment  `json:"payment"`
    Buyer     Buyer    `json:"buyer"`
    Shipping  Shipping `json:"shipping"`
    Items     []Item   `json:"items"`
    Remark    string   `json:"remark"`
}

type Payment struct {
    Method string `json:"method"`
    Paid   bool   `json:"paid"`
}

type Buyer struct {
    ID   int64  `json:"id"`
    Name string `json:"name"`
}

type Shipping struct {
    City       string  `json:"city"`
    Address    string  `json:"address"`
    TrackingNo *string `json:"trackingNo"`
}

type Item struct {
    SKU   string `json:"sku"`
    Title string `json:"title"`
    Qty   int64  `json:"qty"`
    Price int64  `json:"price"`
}
```

商品价格使用整数，不引入浮点数。实际业务如果使用“分”为单位，可以在文档中明确。

## 3. 字段取舍

### 3.1 保留字段

| 字段 | 保留原因 |
| --- | --- |
| `orders` | 顶层对象中的批量数组，是本项目最重要的真实场景 |
| `orderId` | 普通短字符串和唯一标识 |
| `createdAt` | 典型时间字符串，暂不引入 `time.Time` 自定义编码 |
| `status` | 典型低基数字符串 |
| `payment` | 保留嵌套对象 |
| `payment.method` | 普通枚举字符串 |
| `payment.paid` | 布尔值 |
| `buyer` | 保留第二类嵌套对象 |
| `buyer.id` | 整数 |
| `buyer.name` | 中文 UTF-8 字符串 |
| `shipping` | 保留地址和可空字段 |
| `shipping.city` | 短中文字符串 |
| `shipping.address` | 较长中文字符串 |
| `shipping.trackingNo` | 测试字符串与 `null` |
| `items` | 测试嵌套对象数组和变长循环 |
| `items.sku` | 短字符串 |
| `items.title` | 中文商品字符串 |
| `items.qty` | 整数 |
| `items.price` | 金额整数 |
| `remark` | 适合放置长字符串与转义字符 |

### 3.2 删除或合并字段

| 原字段 | 处理 | 原因 |
| --- | --- | --- |
| `payment.tradeNo` | 删除 | 和 `orderId` 都是普通标识字符串，序列化行为重复 |
| `buyer.phone` | 删除 | 涉及业务隐私，且只是另一个普通字符串 |
| `buyer.vipLevel` | 删除 | 已有 `buyer.id`、`qty`、`price` 等整数 |
| `shipping.district` | 删除 | `city` 和 `address` 已覆盖地址字符串 |
| `shipping.courier` | 删除 | 与 `status`、`method` 同属短枚举字符串 |
| `amount` | 整体删除 | `goods`、`freight`、`discount`、`payable` 都是重复整数场景，`items.price` 已足够覆盖 |
| `items.category` | 删除 | 与 `status`、`method` 都是枚举字符串 |
| `tags` | 删除 | 已有更复杂的 `items` 数组；第一版不再增加 `[]string` |

这个方案删除了金额对象和多个重复字段，但仍保留：

```text
3 类嵌套对象
1 类嵌套对象数组
1 个 nullable 字符串
多个中文和 ASCII 字符串
多个整数
1 个布尔值
```

它已经明显超过简单 demo 数据，又没有膨胀到完整生产 JSON 库。

## 4. 为什么不继续简化

如果继续删除 `payment`、`buyer` 或 `shipping`，最终只剩扁平结构体，无法验证 JIT 对嵌套字段执行序列的编排。

如果删除 `items`，无法验证：

```text
变长数组
数组分隔符
嵌套对象重复编码
订单和商品之间的索引关系
```

如果删除 `trackingNo` 的 `null`，无法验证 nullable 字段。

如果删除 `remark`，真实数据中的长字符串和特殊字符比例可能太低，SIMD 字符串扫描难以展示收益。

因此推荐结构已经接近合理下限。

## 5. 2.5 万条数据的分布建议

生产数据本身用于真实性测试，同时再准备少量人工边界数据。不要为了覆盖边界而大幅扭曲全部生产数据。

### 5.1 生产数据集

建议保留原始分布：

```text
订单数约 25000
状态按线上真实比例
支付方式按线上真实比例
商品数量按线上真实比例
中文姓名和地址保持真实长度分布
trackingNo 的 null 比例保持真实分布
```

数据要脱敏：

```text
订单号重新编号
姓名使用模拟姓名
地址保留长度和字符类型，但替换真实内容
物流单号重新生成
remark 删除个人隐私
```

### 5.2 边界数据集

额外添加几十条人工订单，覆盖：

```text
items 长度：0、1、2、5、20
trackingNo：null、空字符串、普通字符串
remark 长度：0、15、31、32、33、63、64、65、256、1024
remark 包含：双引号、反斜杠、换行、制表符
中文、英文和中英文混合
buyer.id、qty、price：0、1、-1、较大整数
bool：true、false
```

`31/32/33` 和 `63/64/65` 用于验证 AVX2 块边界。

### 5.3 SIMD 预期

大多数订单字段都小于 32 字节，因此生产数据中 SIMD 不一定带来很大提升。

这是正常结果，不应人为把所有字符串扩展到很长。建议分别报告：

```text
真实 25000 条订单
短字符串订单
长 remark 订单
大量转义字符订单
```

这样可以看出 SIMD 在什么情况下有效，而不是只展示最有利的数据。

## 6. 对 cgo 架构的影响

原来的单条 `Record` 方案不能直接扩展为每条订单一次 cgo 调用。

错误方案：

```text
25000 条订单
  -> 25000 次 cgo 调用
```

正确方向：

```text
Go OrderBatch
  -> Pack 成无 Go 指针的连续数据
  -> 整个 batch 一次 cgo 调用
  -> C 循环订单
  -> JIT 编码每个订单
  -> AVX2 helper 扫描字符串
```

### 6.1 PackedBatch

Go 侧增加显式打包阶段：

```text
OrderBatch
  -> order rows
  -> item rows
  -> string pool
```

其中：

```text
order rows 只保存整数、布尔值、字符串 offset/length、items start/count
item rows 只保存整数、字符串 offset/length
string pool 保存所有连续 UTF-8 字节
```

这些缓冲区内部不包含 Go 指针，可以在一次同步 cgo 调用期间传给 C。

概念结构：

```go
type stringRef struct {
    Offset uint32
    Length uint32
}

type packedOrder struct {
    OrderID    stringRef
    CreatedAt  stringRef
    Status     stringRef
    Method     stringRef
    Paid       uint8
    BuyerID    int64
    BuyerName  stringRef
    City       stringRef
    Address    stringRef
    TrackingNo stringRef
    HasTrackingNo uint8
    ItemStart  uint32
    ItemCount  uint32
    Remark     stringRef
}

type packedItem struct {
    SKU   stringRef
    Title stringRef
    Qty   int64
    Price int64
}
```

最终布局和 padding 需要在 Go/C 两边做 `sizeof`、`offsetof` 一致性测试，不能只依赖肉眼判断。

### 6.2 API 分层

建议同时提供：

```go
func Pack(batch OrderBatch) (*PackedBatch, error)
func (e *Encoder) MarshalPacked(batch *PackedBatch) ([]byte, error)
func (e *Encoder) Marshal(batch OrderBatch) ([]byte, error)
```

其中：

```text
Marshal       = Pack + MarshalPacked
MarshalPacked = 只测 cgo、JIT、SIMD 编码阶段
```

benchmark 必须同时报告两者，否则只测预打包数据会掩盖 Go 到 C 数据转换成本。

### 6.3 JIT 边界

第一版 JIT 不需要生成完整的订单和商品循环：

```text
C 静态 batch 循环
  -> 调用 JIT order encoder
       -> 写固定订单字段
       -> 调用静态 C items helper
       -> 调用 SIMD string helper
```

这样仍然是真 JIT，同时把变长数组循环留在更容易验证的静态 C 中。

后续再考虑把 items 编码流程也编译成机器码。

## 7. 数据转换命令

仓库提供了 [`scripts/simplify_orders.jq`](../scripts/simplify_orders.jq)。

使用：

```bash
jq -f scripts/simplify_orders.jq orders_original.json > orders_simplified.json
```

检查订单数量：

```bash
jq '.orders | length' orders_simplified.json
```

检查商品数量分布：

```bash
jq '[.orders[].items | length] | group_by(.) | map({count: length, itemCount: .[0]})' orders_simplified.json
```

检查 `trackingNo` 的 null 数量：

```bash
jq '[.orders[] | select(.shipping.trackingNo == null)] | length' orders_simplified.json
```

## 8. 复杂度变化

和单条 `Record` 相比，订单批次新增：

```text
Go 数据打包
两种 row 布局
连续字符串池
嵌套对象输出
items 变长循环
null 状态
批量容量估算
Pack 与 MarshalPacked 两套 benchmark
```

预计开发时间从约 `1～2 周` 调整为约 `2～4 周`。其中最容易出错的是：

1. Go/C row 布局不一致。
2. 字符串 offset/length 越界。
3. items start/count 越界。
4. `trackingNo` 的 null 和空字符串混淆。
5. 只测试 `MarshalPacked`，遗漏 `Pack` 的真实成本。

这个复杂度仍然适合学习项目，但必须按阶段推进，不能一次性实现完整 JIT。

## 9. 已落地的设计

这份结构已经落实为：

1. `OrderBatch`、`Order`、`Item` 等固定 Go 类型。
2. order rows、item rows 和 string pool 组成的 packed batch ABI。
3. `Pack`、`MarshalPacked` 和完整 `Marshal` 三层 API。
4. 一次 cgo 调用处理完整订单批次。
5. C 嵌套对象和 items helper。
6. static/JIT 与 scalar/AVX2 对照路径。
7. 单独统计 Pack 成本的 benchmark。

当前 `testdata/orders.json` 已提供 25,385 条真实业务测试订单，并已接入正确性测试和多库 benchmark。合成数据继续用于补充长字符串、转义字符和多商品等真实数据未覆盖的边界。
