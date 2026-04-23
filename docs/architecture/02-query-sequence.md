# 查询请求时序图（Query Sequence）

```mermaid
sequenceDiagram
    participant Boss as 老板/调用方
    participant Host as Claude/OpenClaw
    participant Bridge as finance_bridge / financeqa CLI
    participant CLI as financeqa query / host-data
    participant RT as engine_runtime
    participant Disp as query_dispatch
    participant Router as query_router + query_planner
    participant Orch as orchestrator + source_registry
    participant SA as source_adapter_*
    participant Direct as fallback/direct query
    participant Calc as accounting + analysis
    participant DB as ConfiguredDB(PostgreSQL/SQLite)

    Boss->>Host: 自然语言问题
    Host->>Host: 读取 SKILL.md
    opt 需要细粒度规则
        Host->>Host: 读取 docs/SKILL_APPENDIX_FULL.md
    end
    Host->>Bridge: finance-query(query)
    Bridge->>Bridge: 读取 SKILL.md 契约版本
    Bridge->>CLI: financeqa query
    CLI->>RT: NewEngine / Query(question)
    RT->>Disp: prepareQueryExecutionContext
    Disp->>Router: BuildQuerySpec + route planning
    alt 命中 orchestrator 路径
        Disp->>Orch: Execute(spec)
        Orch->>SA: Fetch per capability
        SA->>Calc: 账务/桥接/开放项计算
        SA->>DB: 查询官方表 / 序时账 / 银行流水
        DB-->>SA: 原始数据
        SA-->>Orch: FactSet
        Orch-->>Disp: AnswerFrame
        Disp->>DB: 读取表注释 source metadata
        DB-->>Disp: source_note/source_documents
        Disp->>Disp: orchestrated_answer.compose*
        Disp-->>CLI: success=true / answer_method=sql / query_pipeline=orchestrator
        CLI-->>Bridge: stdout JSON
        Bridge-->>Host: text(JSON + bridge_meta + boss_reply)
        Host-->>Boss: 老板可读回答
    else 命中 direct query / fallback
        Disp->>Direct: hr/tax/readiness/fallback/precise/counterparty
        Direct->>Calc: 双口径或专项计算
        Calc->>DB: 查询流水/序时账/利润表
        DB-->>Calc: 原始数据
        Calc-->>Direct: 结构化结果 + trace
        Direct-->>Disp: Result
        Disp->>DB: 读取表注释 source metadata
        DB-->>Disp: source_note/source_documents
        Disp-->>CLI: success=true / answer_method=sql
        CLI-->>Bridge: stdout JSON
        Bridge-->>Host: text(JSON + bridge_meta + boss_reply)
        Host-->>Boss: 老板可读回答
    else 证据不足或 query 非 0
        Disp-->>CLI: success=false 或 answer_method=llm_payload
        CLI-->>Bridge: 失败 JSON / 非 0 退出
        Bridge->>CLI: financeqa host-data
        CLI-->>Bridge: llm_payload JSON
        Bridge-->>Host: text(JSON + fallback_attempted + bridge_meta)
        Host-->>Boss: 基于 llm_payload 做保守归纳
    end
```

## 说明

1. OpenClaw 桥接返回的是 `text`，宿主需要把 `content[0].text` 再解析成 JSON。
2. 不能只看 CLI 退出码；即使 exit code 非 0，也要先解析 `stdout` 里的结构化 JSON。
3. 当前查询主链路已不是单个 `query.Engine` 大方法，而是 `engine_runtime -> query_dispatch -> router/planner -> orchestrator/direct query` 的分层执行。
4. 当响应里出现 `query_pipeline=orchestrator` 时，说明后端已经完成多源聚合与主口径选择，宿主不要再自行重排核心事实。
5. 当 `finance-query` 不能稳定回答时，bridge 会自动补调 `host-data`，返回 `llm_payload` 给宿主继续归纳。
6. 当响应里带有 `data.tax_inclusion` / `data.tax_inclusion_note` 时，宿主必须把这两项当成结构化口径约束消费，不要只从 `message` 文案里猜税口径。
7. 当 `bridge_meta.capabilities.tax_disclosure=true` 时，表示 bridge 已显式暴露税口径提示；宿主摘要时应优先引用结构化字段。
8. 当响应里带有 `data.source_note` 时，宿主应优先直接引用它；不要自己根据表名、SQL 或 `source_tables` 重新拼来源说明。
