# 查询请求时序图（Query Sequence）

```mermaid
sequenceDiagram
    participant Boss as 老板/调用方
    participant Host as Claude/OpenClaw
    participant Bridge as finance_bridge / financeqa CLI
    participant CLI as financeqa query / host-data
    participant Eng as query.Engine
    participant Calc as accounting + analysis
    participant DB as ConfiguredDB(PostgreSQL/SQLite)

    Boss->>Host: 自然语言问题
    Host->>Host: 读取 SKILL.md
    opt 需要细粒度规则
        Host->>Host: 读取 docs/SKILL_APPENDIX_FULL_2026-04-15.md
    end
    Host->>Bridge: finance-query(query)
    Bridge->>Bridge: 读取 SKILL.md 契约版本
    Bridge->>CLI: financeqa query
    Eng->>Eng: 归一化 + 期间提取 + 意图识别
    Eng->>Eng: 实体识别 + 路由
    alt 可直接规则计算
        Eng->>Calc: 双口径计算
        Calc->>DB: 查询流水/序时账/利润表
        DB-->>Calc: 原始数据
        Calc-->>Eng: 结构化结果 + trace
        Eng-->>CLI: success=true / answer_method=sql
        CLI-->>Bridge: stdout JSON
        Bridge-->>Host: text(JSON + bridge_meta + boss_reply)
        Host-->>Boss: 老板可读回答
    else 证据不足或 query 非 0
        Eng-->>CLI: success=false 或 answer_method=llm_payload
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
3. 当 `finance-query` 不能稳定回答时，bridge 会自动补调 `host-data`，返回 `llm_payload` 给宿主继续归纳。
