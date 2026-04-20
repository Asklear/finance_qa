# 分层架构图（Layered Architecture）

```mermaid
flowchart LR
    A["外部数据源<br/>Excel / 银行流水 / 科目余额表"] --> B["Ingest 层<br/>parser + ingest + 去重保护"]
    B --> C[("Configured DB<br/>PostgreSQL (default)<br/>SQLite (explicit compatibility)")]

    S["宿主 Skill 层<br/>SKILL.md + docs/SKILL_APPENDIX_FULL_2026-04-15.md"] --> H["宿主入口<br/>Claude Code / OpenClaw / CLI"]
    H --> BR["Bridge / CLI 层<br/>finance-query / finance-host-data / finance-upload"]
    BR --> Q["Query 层<br/>intent + routing + llm_payload fallback"]

    Q --> ACC["Accounting 层<br/>账面利润 / 双视角 / 开放项配对"]
    Q --> ANA["Analysis 层<br/>风险 / 健康度 / 周转"]
    Q --> C
    ACC --> C
    ANA --> C

    CFG["规则与配置<br/>config/rules.json + env"] -.读取.-> Q
    CFG -.读取.-> ACC

    Q --> O["输出<br/>structured JSON + trace + boss_reply"]
```

## 说明

1. 宿主默认先读取根目录 `SKILL.md`，需要细粒度规则时再按相对路径读取 `docs/SKILL_APPENDIX_FULL_2026-04-15.md`。
2. `Query` 是业务入口，负责意图识别、实体识别、路由、失败兜底与结构化返回。
3. `Accounting` 负责账面利润、双视角勾稽、应收应付开放项配对、人力成本与税额计算。
4. 底层数据库是“配置化 DB”：默认走 PostgreSQL，仅在显式传入 SQLite 路径时启用兼容模式；不再默认回退根目录 `finance.db`。
