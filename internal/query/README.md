# internal/query

`internal/query` 是财务问答的对外门面，CLI 和 MCP 通过这里创建 `Engine` 并执行 `Engine.Query`。

## 根目录

根目录保留跨业务域的主流程代码：

- `engine_*.go`: `Engine` 生命周期、DB 初始化、公司上下文、缓存和最新账期锚点。
- `query_*.go`: 问题解析、`QuerySpec` 构造、执行计划、业务路由、结果收尾。
- `*_facade.go`: 子包的兼容门面。外部仍可通过 `query.*` 使用稳定 API，具体实现放在对应子目录。
- 业务域文件仍保留在根目录，例如 `core_metrics_*`、`contract_*`、`arap_*`、`reconciliation_*`。这些文件大量依赖 `*Engine`、DB 和内部 helper，当前不为了降低文件数量而强行拆包。

### Go 包组织原则

- `internal/query` 是一个高内聚查询引擎包。Go 里同一目录即同一 package，业务文件较多本身不是问题，关键是保持文件前缀清晰、依赖方向稳定、未导出 helper 不被迫外泄。
- 只有输入输出清楚、低耦合、不会反向依赖 `Engine` 的代码才拆入子包，例如账期解析、计算器、规则解析、基础模型和轻量消息格式化。
- 直接访问 DB、依赖 `*Engine` 缓存/公司上下文、或需要大量未导出 helper 的业务实现留在根包。强拆这类代码通常会带来 import cycle、过度导出和更差的可测试性。
- 新增业务能力优先使用清晰文件前缀归类；只有当边界稳定、测试可以通过窄接口覆盖时，再考虑拆子包。
- 当前目录整理已完成低风险部分，后续不继续按文件数量驱动拆分；有具体业务改动时再顺手抽离低耦合逻辑。

### 根目录文件簇

- `query_context*`, `query_execution*`, `query_domain_stage*`, `query_finalize*`: 查询入口、上下文准备、执行阶段调度和结果收尾。
- `query_spec*`, `query_planner*`, `query_policy*`, `query_family*`, `query_entity*`, `intent_*`, `route_*`: 意图识别、问题规格化、路由规划和路由决策。
- `source_adapter_*`, `source_registry*`, `source_probe*`: 面向 `FactSet` 的来源适配器、来源探测和注册逻辑。
- `source_attribution_*`: 结果来源标注、原始表和上传文件映射说明。
- `core_metrics_*`: 收入、成本、利润、净利润等核心指标查询、口径校验、区间汇总和消息生成。
- `contract_aggregate_*`: 公司或项目范围的合同收入、成本、应收应付和利润聚合。
- `contract_dimension_*`: 单个合同主体、合同内容、客户/供应商合同维度查询。
- `contract_detail_*`: 合同明细探测、合同页和发票明细收集。
- `contract_finance_*`, `cost_settlement_*`, `fund_income_*`: 合同资金口径的底层分组汇总。
- `arap_*`: 应收应付查询、官方余额表口径和 open item 口径。
- `reconciliation_*`: 账面、现金和往来证据之间的调节分析。
- `counterparty_*`, `internal_party_*`, `entity_*`: 交易对手、内部主体和实体识别。
- `fallback_*`, `readiness_*`, `precise_*`, `bank_cash_*`, `tax_*`, `hr_*`, `expense_*`, `supplier_*`: 独立业务问答入口或兜底能力。
- `orchestrated_*`, `orchestrator*`: 多来源编排查询的入口和状态处理。
- `rules_config_*`, `rule_lexicon*`: 规则配置 provider、加载、默认规则和词库。
- `helpers*`, `result_helpers*`, `metric_*`, `semantic_catalog*`: 根包共享工具，后续只有在依赖清晰时再拆。

### 命名约定

- 核心指标统一使用 `core_metrics_*`，避免 `core_metric_*` 和 `core_metrics_*` 混用。
- 合同聚合使用 `contract_aggregate_*`；合同主体/维度查询使用 `contract_dimension_*`；合同明细使用 `contract_detail_*`。
- `contracts_facade.go` 只表示 `contracts/` 子包的根包兼容门面，不代表业务查询文件前缀。
- 新增业务域文件优先沿用现有前缀；如果需要新前缀，先在本 README 记录边界。

### 配置边界

- 查询代码通过 `RuleConfigProvider` 获取当前规则配置；默认 provider 从 `FINANCEQA_RULES_PATH`、`FINANCEQA_*` 环境变量和规则文件组成缓存 key。
- `NewEngine(..., WithRuleConfigProvider(provider))` 可注入规则配置，便于测试或后续线上配置隔离。
- `CurrentRuleConfig()` 保持兼容，仍返回默认 provider 的当前配置。
- 完整 `RuleConfig` 仍留在根包，因为它引用 intent、metric、contract、counterparty 等查询层模型；`rules/` 子包只承载低耦合解析和 map 规范化工具。

### 来源适配器边界

- `source_adapter_*` 只负责把领域查询结果转换成 `FactSet`，不直接拥有 `*Engine`。
- 每个 adapter 依赖 `source_runtime.go` 中的窄接口，例如合同、应收应付、核心指标、供应商付款和数据完备性 runtime。
- `NewDefaultSourceRegistry(runtime DefaultSourceRuntime)` 是默认组合根，负责把同一个 runtime 注册到内置 adapter；当前生产 runtime 是 `*Engine`。
- 新增 adapter 时先定义最小 runtime 接口，并为 adapter 增加 fake runtime 单测，避免因为测试方便把 DB 或 `*Engine` 重新耦合进 adapter。

## 子目录

- `period/`: 中文账期解析、账期范围格式化、月末日期、子账期识别、锚点日期解析。
- `calc/`: 通用算术表达式执行器、计算计划类型、算术一致性校验。
- `bridge/`: 面向 MCP/宿主模型的兼容摘要和 `final_answer` 生成。
- `cashflow/`: 银行/现金账户方向判断、现金流入流出汇总结构。
- `company/`: 公司名、简称和问题文本中的公司提及匹配。
- `arap/`: 应收应付 open item 消息格式化等低耦合辅助能力。
- `contracts/`: 合同维度的基础类型。
- `coremetrics/`: 核心指标利润现金桥等低耦合辅助能力。
- `entity/`: 低耦合实体文本辅助判断，例如业务维度标签过滤。
- `fact/`: 来源适配器返回的事实载荷模型、权威等级和覆盖状态。
- `orchestration/`: 编排结果组装时的 `FactSet` 查找、trace 提取和轻量转换工具。
- `reconciliation/`: 调节分析的基础类型。
- `result/`: 查询结果返回结构和 trace 字段补齐。
- `rules/`: 规则配置解析、map 规范化等低耦合工具。
- `stringset/`: 字符串集合去重、查找和默认值选择工具。

## 迁移规则

新增或迁移文件时按职责放置：

- 纯基础能力优先放入子目录，并在根目录通过 facade 暴露必要兼容 API。
- 依赖 `*Engine`、直接访问 DB、或需要大量未导出 helper 的业务查询文件，先留在根目录，避免为了移动文件引入循环依赖或过度导出。
- 拆分业务域时，应一次只迁移一个边界清楚的低耦合域，并先保留原有入口和测试语义不变。
- 不以“根目录文件数”为单独拆分理由；优先保证查询行为稳定、私有封装完整、测试覆盖可维护。
