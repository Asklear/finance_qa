# 查询请求时序图（Query Sequence）

```mermaid
sequenceDiagram
    participant Boss as 老板/调用方
    participant Host as OpenClaw/Claude
    participant Bridge as finance_bridge / financeqa CLI
    participant RT as engine_runtime
    participant Rewrite as query_rewrite
    participant Entity as DB candidate entity resolver
    participant Catalog as semantic_catalog + source mapping
    participant Probe as source_probe + route_decision
    participant Exec as query_execution / stage policy
    participant Bank as bank_cash_queries
    participant Contract as contract_aggregate / contracts
    participant Orch as orchestrator + source_adapter_*
    participant Direct as fallback / hr / tax / arap / counterparty
    participant Final as query_finalize + source_attribution
    participant DB as ConfiguredDB(PostgreSQL / explicit SQLite)

    Boss->>Host: 自然语言问题
    Host->>Host: 读取 SKILL.md
    opt 需要细粒度规则
        Host->>Host: 读取 docs/SKILL_APPENDIX_FULL.md
    end
    Host->>Bridge: finance-query({query})
    Bridge->>Bridge: 校验 skill_contract_version / appendix 存在性
    Bridge->>RT: financeqa query / Engine.Query
    RT->>Rewrite: 改写老板问题为意图槽位
    Rewrite-->>RT: 指标 / 实体片段 / 主期间 / 子期间 / 现金语义 / 合同优先标记
    RT->>Entity: 用数据库候选实体打分确认
    Entity->>DB: 读取银行流水、序时账、摘要、合同客户、合同内容候选
    DB-->>Entity: candidate entities
    Entity-->>RT: 高置信度实体或拒绝实体
    RT->>Catalog: 构建数据源能力目录
    Catalog->>DB: 读取字段注释 / 功能目录 / fin_file_mappings
    DB-->>Catalog: 表能力 / 字段语义 / 来源 Excel 映射
    RT->>Probe: 对候选来源做轻量覆盖探测
    Probe->>DB: 探测合同/专家表、银行流水、财务账覆盖情况
    DB-->>Probe: probe_results
    Probe-->>RT: route_decision(selected_source / fallback_reason)
    RT->>Exec: 构建执行计划并按阶段执行

    alt selected_source = bank_statement 或明确现金问题
        Exec->>Bank: 查询实际到账/支出/净增加/回款/付款
        Bank->>DB: 读取 fin_bank_statement
        DB-->>Bank: 银行流水事实
        Bank-->>Exec: 现金口径结果
    else selected_source = contract_aggregate
        Exec->>Contract: 查询合同/专家表汇总
        Contract->>DB: 读取 fin_contracts + fin_fund_income + fin_cost_settlements
        DB-->>Contract: 合同收入/成本/开票/回款/付款事实
        Contract-->>Exec: 老板口径合同汇总
    else 需要多源编排
        Exec->>Orch: Execute(QuerySpec)
        Orch->>DB: 读取 source_adapter 所需表
        DB-->>Orch: FactSet
        Orch-->>Exec: AnswerFrame
    else 专项或兜底问题
        Exec->>Direct: hr / tax / readiness / arap / counterparty / precise
        Direct->>DB: 查询专项表
        DB-->>Direct: 专项事实
        Direct-->>Exec: 结构化结果
    end

    Exec->>Final: 合并结果、trace、route_decision、source_plan
    Final->>DB: 读取来源元数据
    DB-->>Final: source_note / source_update_note / source_documents
    Final-->>Bridge: stdout JSON(success / data / boss_reply / bridge_meta)
    Bridge-->>Host: content[0].text(JSON)
    Host->>Host: 解析 JSON，保留审计字段，净化老板可见字段
    Host-->>Boss: 金额 + 期间 + 口径 + 来源 Excel + 必要建议

    opt finance-query 不能稳定直答
        Bridge->>RT: financeqa host-data
        RT->>DB: 抽取 llm_payload 所需表
        DB-->>RT: raw payload / extraction_errors
        RT-->>Bridge: llm_payload JSON
        Bridge-->>Host: fallback_attempted + bridge_meta
        Host-->>Boss: 基于完整 payload 保守归纳；若 extraction_errors 存在则提示先修复数据包
    end
```

## 说明

1. OpenClaw 桥接返回的是 `content[0].text`，宿主必须先把 text 解析成 JSON，再生成自然语言回答。
2. 不能只看 CLI 退出码；业务失败时 `stdout` 仍可能包含结构化 JSON。
3. 老板核心指标先走合同/专家表候选，但必须经过轻量探测确认覆盖；不能因为识别到实体就绕开合同口径。
4. 明确现金问题不强行走合同汇总，直接优先银行流水。
5. `route_decision/probe_results` 是宿主和审计链路使用的口径解释字段，不能原样贴给老板。
6. `source_note` 是老板可见来源说明的主入口；`source_update_note` 是老板可见来源更新时间。宿主应优先直接引用，不要用 SQL、表名、字段名、表注释或历史记忆重拼来源。
7. 财务来源文件名和更新时间只从 `fin_file_mappings` 获取；没有映射就不展示该财务来源。合同和发票来源分别来自 `contract_main`、`contract_invoices`。
8. `source_cell_notes` 和 `remarks` 是宿主 LLM 可用的补充解释字段，用于备注、批注、谈判状态、异常说明和单元格依据；普通金额答案不默认展开，且不能替代 `source_note/source_update_note`。
9. 如果结果来自序时账经营口径，必须消费 `data.tax_inclusion/data.tax_inclusion_note`，说明默认未主动剔税。
10. 老板可见回复必须过滤数据库 id、合同 id、科目代码、SQL、trace、bridge_meta 等辅助字段。
