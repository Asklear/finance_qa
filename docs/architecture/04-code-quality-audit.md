# 代码质量与测试覆盖整改清单

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 梳理当前代码重复度、模块边界和测试覆盖的真实状态，并给出可执行的整改优先级。

**Architecture:** 以当前 `internal/query` 作为核心业务编排层为中心，按“重复代码收敛、模块正交性整理、测试补强”三条线并行推进。优先处理生产代码里可直接抽象的重复实现，再逐步拆解 query 层的交叉职责，最后补齐覆盖薄弱的支撑包。

**Tech Stack:** Go, `go test`, `go tool cover`, `dupl`.

---

## 现状结论

1. 业务逻辑回归已经有基础：真实题库回归 `TestRealdataQuestionSuites` 已通过 54 题。
2. 整体测试覆盖率中等偏上，但不均衡：全仓库 statement coverage 为 `46.5%`。
3. 代码重复度存在明确热点，主要集中在少数函数对和测试样板段，不是全仓库普遍重复。
4. 模块边界整体可读，但 `internal/query` 仍然承担了过多路由、汇总、探测、兜底职责，正交性不够彻底。

## 当前整改进度

已完成的 P0 收敛：

1. 抽出 `internal/db/table_introspection.go`，统一 `TableExists` / `ColumnsExist` 的表与列探测逻辑。
2. 抽出 `internal/query/contract_finance_totals.go`，统一合同收入/成本汇总的直接表与分组表计算逻辑。
3. 收敛 `internal/feishu/client.go` 的 tenant/app token 缓存重复逻辑。

已完成的 P2 测试补强：

1. `internal/config` 补充关键词配置、用户配置持久化和防御性拷贝测试。
2. `internal/support` 补充 `.env` 解析、路径默认值和来源摘要测试。
3. `internal/dimensions` 补充内存仓库、SQLite 仓库、标准规则初始化和 mapper 优先级测试。
4. `internal/accounting` 补充月度利润、利润表、现金流、双口径和余额试算测试。
5. `internal/analysis` 补充账龄、预警、周转天数、利润现金桥和日期/分桶 helper 测试。

当前 `dupl -t 150` 的生产代码重复热点已缩减到：

- `dupl -t 150` 当前没有再报生产代码重复片段

验收结果：

1. `go test ./... -count=1` 已通过。
2. `find cmd internal -name '*.go' ! -name '*_test.go' -print | /Users/gaorongvc/go/bin/dupl -files -plumbing -t 150 | wc -l` 结果为 `0`。

## 覆盖率摘要

最近一次低覆盖包定向补测结果：

- `financeqa/internal/config`：`75.6%`
- `financeqa/internal/support`：`75.7%`
- `financeqa/internal/dimensions`：`53.2%`
- `financeqa/internal/accounting`：`75.4%`
- `financeqa/internal/analysis`：`65.1%`

整改前 `go test ./... -cover -count=1` 的包级覆盖结果：

- `financeqa/internal/feishusync`：`72.3%`
- `financeqa/internal/storage`：`66.9%`
- `financeqa/internal/feishu`：`61.8%`
- `financeqa/internal/ocr`：`61.8%`
- `financeqa/internal/query`：`59.0%`
- `financeqa/internal/db`：`57.4%`
- `financeqa/internal/parser`：`54.4%`
- `financeqa/internal/ingest`：`31.3%`
- `financeqa/cmd/financeqa`：`22.9%`
- `financeqa/internal/openitems`：`19.3%`
- `financeqa/internal/accounting`：`1.4%`
- `financeqa/internal/analysis`：`0.2%`
- `financeqa/internal/config` / `internal/dimensions` / `internal/support`：`0.0%`

## 重复代码热点

`dupl -t 150` 发现的生产代码重复主要有这些：

1. `internal/feishu/client.go`
   - 两段几乎对称的分页/请求处理逻辑重复。
2. `internal/feishusync/repository.go` 和 `internal/ocr/repository.go`
   - OCR 状态、领任务、标记完成/失败的数据库模式高度相似。
3. `internal/query/cost_settlement_groups.go` 和 `internal/query/fund_income_groups.go`
   - 两套分组汇总逻辑几乎同构，只有字段名和表名不同。
4. `internal/query/contract_aggregate_collect.go`
   - 同一类聚合逻辑在多个分支里重复。
5. `internal/ingest/contracts.go`
   - 同类字段清洗/落库片段重复。
6. `internal/query/source_probe_contracts.go`
   - 轻量探测逻辑重复出现。

测试里也有大量重复样板，最明显的是 `tests/unit/ingest/contract_import_test.go` 和 `tests/unit/query/entity_routing_test.go`，这类重复不优先改业务，但可以后续考虑测试辅助函数抽取。

## 模块正交性判断

### 相对清晰的边界

- `internal/feishu`：飞书 API 客户端。
- `internal/feishusync`：飞书同步与扫描。
- `internal/ocr`：OCR 任务与仓储。
- `internal/storage`：OSS/S3 存储适配。
- `internal/db`：数据库封装。
- `internal/parser`：表格/文件解析。

### 交叉职责较重的边界

- `internal/query`：
  - 查询改写
  - 实体识别
  - source probe
  - 路由决策
  - 合同/专家表汇总
  - 现金口径回退
  - 结果归因

这说明架构不是“乱”，但 query 层仍是业务中心仓，后续最好把稳定能力再拆成更小的子模块。

## 整改优先级

### P0：明显重复，适合先抽

1. 把 `internal/feishusync/repository.go` 和 `internal/ocr/repository.go` 中的共用数据库状态机抽成内部共享 helper。
2. 把 `internal/query/cost_settlement_groups.go` 与 `internal/query/fund_income_groups.go` 提炼成通用 group 汇总模板。
3. 把 `internal/feishu/client.go` 重复请求/翻页逻辑合并成统一分页器或请求执行器。

### P1：降低 query 层耦合

1. 拆分 `source_probe_contracts.go`、`contract_aggregate_collect.go`、`query_execution` 的交叉逻辑。
2. 把“探测是否覆盖”“决定走哪条路径”“真正执行 SQL”分开。
3. 把合同/发票/财务表口径统一成更少的共享接口。

### P2：补测试短板

1. 已完成：给 `internal/config`、`internal/support`、`internal/dimensions` 补最小单测。
2. 已完成：给 `internal/accounting`、`internal/analysis` 补核心口径测试。
3. 后续可继续：给 `internal/ingest` 增补字段清洗、空值、重复行、边界值测试。

## 验收口径

整改完成后，至少要满足下面几条：

1. 生产代码重复组数量下降，且不再新增明显同构函数对。
2. `internal/query` 的职责边界更清楚，新增能力不再直接堆到同一个文件。
3. 核心业务回归题库继续保持全绿。
4. 包级覆盖率不要求“一刀切”提升，但支撑包不能长期保持 0%。

## 建议的测试口径

以后不要只看行覆盖率，至少分三层看：

1. 真实业务题库回归：验证老板问答口径和金额结果。
2. 功能路径测试：验证扫描、OCR、入库、同步、落库、路由。
3. 代码覆盖率：作为边缘补充，不单独代表业务正确。
