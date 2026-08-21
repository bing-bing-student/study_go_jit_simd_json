# 早期单 Record 设计（已归档）

这份文档原本描述固定 `Record{ID, Name, Active}` 的 cgo 方案。订单测试数据确认后，该方案已经被以下最终设计替代：

- 顶层输入改为 `OrderBatch`。
- Go 侧增加 `PackedBatch`。
- cgo ABI 改为 order rows、item rows 和 string pool。
- 整个订单批次只跨一次 cgo 边界。
- C 负责批次循环，JIT 负责编排单个订单的固定字段操作。
- AVX2 用于较长字符串的特殊字符扫描。

当前实现和后续修改应以 [OrderBatch 最终工程设计](engineering-design-order-batch.md) 为准。

保留这个文件名只是为了避免旧链接失效，不再作为实现依据。
