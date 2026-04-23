# Keyword Config Convergence Design

## Goal

把查询层与关键词匹配相关的配置收敛到单一入口 `config/rules.json`，同时明确“哪些是可配置词表/阈值，哪些是必须保留在代码里的财务判断逻辑”，避免后续继续出现多套关键词配置并存、规则散落和难以维护的问题。

## Background

当前代码库里与关键词匹配相关的能力分散在两套机制里：

1. 查询主链路使用 `internal/query/rules_config.go` 提供的 `RuleConfig`。
2. 遗留的 `internal/config/keywords_manager.go` 仍然维护一套 `query_keywords.json` 风格的关键词结构。

实际生产查询主路径主要依赖 `RuleConfig`，但以下问题依然存在：

- 配置入口不唯一，未来继续扩展时容易把规则加到不同位置。
- 当前 `RuleConfig` 基本采用平铺字段，随着词表增多会继续膨胀。
- 一些纯 literal 关键词仍然硬编码在 router / engine / classifier 内，不利于业务侧调优。
- 另一部分逻辑其实不适合配置化，如果不划清边界，后续会把组合判断和财务语义错误地下放到配置里。

本设计的目标不是“把所有规则都配置化”，而是“把适合配置化的词典和阈值收敛，并保留代码里真正需要稳定实现的财务逻辑”。

## Non-Goals

以下内容不在本次配置收敛范围内：

- 不把开放项配对逻辑、证据置信度计算改成配置。
- 不把 AR/AP 的 FIFO、confirmed/probable/unmatched 机制改成配置。
- 不把财务语义判断（如回款不能直接当收入、税额差异解释）改成配置。
- 不重写整个查询引擎。
- 不在本次移除 `keywords_manager.go` 的所有遗留代码，只做冻结和兼容。

## Design Principles

### 1. Single Source of Truth

查询层关键词配置统一以 `config/rules.json` 为唯一配置源，代码加载入口统一收敛到 `internal/query/rules_config.go`。

### 2. Lexicon Config, Logic in Code

配置文件只承载：

- 意图关键词
- stopwords
- 优先级
- 冲突关系
- 最低置信度
- 角色词典
- 税务词典
- 内部主体后缀词典

代码保留：

- 组合判断
- 凭证级证据逻辑
- 财务判断与口径解释
- 开放项配对与置信度归因

### 3. Backward Compatibility First

现有平铺 schema 不立即废弃。先支持新 schema，同时兼容旧字段读取一个过渡周期，避免已有环境变量、测试和部署脚本立刻失效。

### 4. Progressive Migration

优先迁移“纯 literal 关键词”和“阈值参数”，最后再迁移角色/税务/内部主体词典。高风险财务逻辑保持不动。

## Current State Inventory

### Already Configurable via RuleConfig

- `generic_metric_stopwords`
- `intent_arap_keywords`
- `intent_hr_cost_keywords`
- `intent_tax_keywords`
- `intent_health_keywords`
- `intent_fallback_keywords`
- `intent_analysis_keywords`
- `intent_host_payload_keywords`
- `intent_monthly_summary_keywords`
- `fallback_monthly_expense_keywords`
- `high_priority_phrases`
- `intent_priority`
- `intent_conflicts`
- `intent_min_confidence`
- role classification thresholds

### Still Hardcoded but Good Candidates for Config

- large transaction keywords in `intent_router_v2.go`
- identity keywords in `intent_router_v2.go`
- precise balance keywords in `intent_router_v2.go`
- HR breakdown wording keywords in `engine.go`
- metric detection keywords in `engine.go`
- profit single-view block keywords in `engine.go`
- counterparty classification question keywords in `engine.go`
- customer/supplier/employee keyword dictionaries in `counterparty_classifier.go`
- tax input/output keyword dictionaries in `counterparty_classifier.go`
- internal organization suffixes and context keywords in `internal_party.go`

### Should Stay in Code

- `项目` + 指标组合判断
- HR/ARAP/Tax 等意图间的流程控制
- 开放项冲销证据模型
- tax normalizer 的财务差异解释逻辑
- internal transfer 的凭证级识别逻辑
- company alias scoring / entity extraction heuristics

## Proposed Rules Schema (v2)

目标 schema 使用嵌套结构表达“词典域”，避免继续无上限增加平铺字段。

```json
{
  "schema_version": 2,
  "router": {
    "stopwords": {
      "generic_metric": ["收入", "成本", "利润"]
    },
    "intents": {
      "arap": {
        "keywords": ["应收", "应付", "账款", "往来款"],
        "priority": 100,
        "min_confidence": 0.6,
        "conflicts": ["fallback", "general"],
        "high_priority_phrases": ["预收款", "预付款", "应收账款", "应付账款"]
      },
      "hr_cost": {
        "keywords": ["人力成本", "工资成本", "薪酬成本", "应付职工薪酬"]
      },
      "tax": {
        "keywords": ["税", "销项", "进项", "增值税"],
        "priority": 90,
        "min_confidence": 0.55,
        "conflicts": ["fallback", "general"]
      },
      "analysis": {
        "keywords": ["分析", "评分", "评价", "风险"]
      },
      "host_payload": {
        "keywords": ["宿主llm", "原始数据", "全量财报", "llm数据包"],
        "priority": 120
      },
      "monthly_summary": {
        "keywords": ["概括", "总结", "利润", "指标", "收入", "支出", "成本"],
        "priority": 70,
        "min_confidence": 0.5,
        "conflicts": ["general"]
      },
      "fallback": {
        "keywords": ["健康度", "供应商多少", "整体支出"],
        "priority": 40,
        "min_confidence": 0.45
      },
      "large_transaction": {
        "keywords": ["最大", "单笔", "流入对手方", "流出对手方"],
        "priority": 110
      },
      "identity": {
        "keywords": ["是谁", "身份", "干嘛的", "哪里的", "谁是"],
        "priority": 105
      },
      "precise": {
        "keywords": ["期末", "余额", "是多少", "查询余额", "还有多少"],
        "priority": 20
      }
    },
    "metric_keywords": {
      "revenue": ["收入", "营收", "销售额"],
      "cost": ["成本"],
      "profit": ["利润"]
    },
    "hr_breakdown_keywords": ["工资", "社保", "公积金", "分别", "拆分", "拆开", "明细", "构成"],
    "counterparty_classification_question_keywords": ["成本还是收入", "是成本还是收入", "供应商付款还是预收款", "客户还是供应商"],
    "profit_single_view_block_keywords": ["现金流", "回款", "到账", "银行卡", "差异", "为什么"]
  },
  "counterparty": {
    "roles": {
      "customer": ["应收", "回款", "收款", "结算款", "销售", "收入", "主营业务收入", "营业收入", "预收", "合同资产", "客户", "1122", "1121"],
      "supplier": ["应付", "付款", "采购", "成本", "材料", "供应商", "外包", "2202", "预付账款", "1123", "112301"],
      "employee": ["工资", "薪酬", "社保", "公积金", "报销", "差旅", "福利", "餐补", "伙食", "应付职工薪酬", "2211"]
    },
    "tax": {
      "output": ["销项税", "222101", "销项"],
      "input": ["进项税", "222102", "进项"]
    },
    "thresholds": {
      "mixed_min_ratio": 0.45,
      "mixed_min_positive_score": 1.0,
      "mixed_min_positive_roles": 2,
      "min_primary_score": 0.5,
      "min_confidence": 0.0
    }
  },
  "internal_party": {
    "org_suffixes": ["分公司", "子公司", "事业部", "办事处", "分部", "总部", "总公司"],
    "account_context_keywords": ["应付职工薪酬", "其他应收款", "其他应付款", "内部往来"]
  }
}
```

## Compatibility Strategy

### File Compatibility

`rules_config.go` 同时支持两种配置格式：

1. 旧版平铺格式
2. 新版嵌套式 `schema_version=2`

解析顺序：

1. 先加载默认值
2. 再读取文件覆盖
3. 文件里若存在 `schema_version=2`，优先按嵌套 schema 解析
4. 若没有，则按旧平铺字段解析
5. 最后应用环境变量覆盖

### Environment Variable Compatibility

环境变量短期保持旧名兼容，例如：

- `FINANCEQA_INTENT_ARAP_KEYWORDS`
- `FINANCEQA_ROLE_MIXED_MIN_RATIO`

新版本若需要补充新的词表覆盖，可以采用 JSON 或逗号分隔方式新增，但不强制一次性迁完。

### Deprecation Policy

- `internal/config/keywords_manager.go` 不再承接查询主链路新规则。
- 保留其现有测试和兼容能力，但在文档中标记为 legacy。
- 后续若没有实际依赖，可单独做删除计划，不放在本次设计里。

## Code Structure Changes

### 1. Rules Loader Refactor

修改 `internal/query/rules_config.go`：

- 在保留 `RuleConfig` 对外可用接口的同时，引入内部嵌套配置结构。
- 增加从嵌套 schema 到当前运行时配置结构的映射层。
- 提供统一 accessor，避免业务代码继续直接依赖各种散乱字段。

建议新增：

- `RouterLexicon`
- `CounterpartyLexicon`
- `InternalPartyLexicon`

### 2. Unified Lexicon Accessors

建议新增 `internal/query/rule_lexicon.go`，对业务代码暴露统一取词接口，例如：

- `cfg.IntentKeywords(IntentTaxQuery)`
- `cfg.MetricKeywords("profit")`
- `cfg.HRBreakdownKeywords()`
- `cfg.CounterpartyRoleKeywords(CounterpartyEmployee)`
- `cfg.InternalPartySuffixes()`

目标是让业务层代码不关心配置文件到底是平铺还是嵌套。

### 3. Call-Site Migration

以下文件需要逐步改用统一 accessor：

- `internal/query/intent_router_v2.go`
- `internal/query/helpers.go`
- `internal/query/engine.go`
- `internal/query/counterparty_classifier.go`
- `internal/query/internal_party.go`

## Migration Scope by Priority

### Priority 0

- 收敛配置入口到 `rules.json`
- 明确 `keywords_manager.go` 不再增加新查询规则
- 引入嵌套 schema 与兼容加载能力

### Priority 1

迁移所有纯 literal 问法词：

- large transaction keywords
- identity keywords
- precise keywords
- metric keywords
- HR breakdown keywords
- profit single-view block keywords
- counterparty classification question keywords

### Priority 2

迁移领域词典：

- customer/supplier/employee 角色词典
- tax input/output 词典
- internal party suffixes and account context keywords

### Priority 3

- 更新 README
- 增加示例 `rules.json`
- 补齐旧 schema / 新 schema / env override 测试

## Testing Strategy

### Unit Tests

- 旧平铺 schema 解析测试
- 新嵌套 schema 解析测试
- env override 解析测试
- lexicon accessor 测试
- router/engine/classifier 使用新词表后的回归测试

### Integration Tests

重点验证：

- `intent_trace` 行为保持稳定
- 利润单口径/双口径切换行为不回归
- HR breakdown、AR/AP、counterparty classification 不因词表迁移回归

### Real Data Regression

沿用当前真实数据回归：

- `tests/scripts/run_user15_realdata_check.sh`
- `tests/scripts/run_user19_realdata_check.sh`
- `tests/scripts/run_top20_realdata_check.sh`

## Risks and Mitigations

### Risk 1: Schema migration breaks existing deployments

Mitigation:

- 保留旧字段兼容加载
- 先补解析单测再切调用点

### Risk 2: Over-configurability leaks business logic into JSON

Mitigation:

- 明确只迁 literal lexicon and thresholds
- 所有组合判断仍留代码实现

### Risk 3: Query behavior changes silently

Mitigation:

- 所有调用点迁移都带对应单测
- 最后跑真实数据回归

### Risk 4: Two config systems continue diverging

Mitigation:

- 文档中明确 `rules.json` 是查询主链路唯一配置源
- `keywords_manager.go` 标记 legacy，并禁止新增查询规则接入

## Acceptance Criteria

设计完成后的实现应满足：

1. 查询主链路只依赖 `rules.json` 作为词表配置源。
2. 旧平铺 schema 仍能被解析。
3. 新嵌套 schema 可覆盖所有当前和计划迁移的关键词词表。
4. 业务判断逻辑未被错误地下放到配置文件中。
5. 单元、集成、真实数据回归行为与当前财务修复结果保持一致。

## Implementation Handoff

后续实现按以下顺序推进：

1. loader 和 schema 兼容层
2. router / engine 的 literal 关键词迁移
3. counterparty / internal_party 词典迁移
4. 测试与文档收尾
