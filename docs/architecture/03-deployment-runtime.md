# 部署与运行图（Deployment & Runtime）

```mermaid
flowchart LR
    subgraph LocalRepo["本地仓库"]
        LR1["SKILL.md"]
        LR2["docs/SKILL_APPENDIX_FULL.md"]
        LR3["plugin/openclaw-finance/server/finance_bridge.py"]
        LR4["tests/scripts/build_openclaw_package.sh"]
        LR5["tests/scripts/sync_openclaw_bridge_and_skill.sh"]
        LR1 --> LR2
        LR4 --> LR1
        LR4 --> LR2
        LR5 --> LR1
        LR5 --> LR2
        LR5 --> LR3
    end

    subgraph Package["OpenClaw 安装包 dist/finance_qa_plugin"]
        PK1["SKILL.md"]
        PK2["docs/SKILL_APPENDIX_FULL.md"]
        PK3["bin/financeqa"]
        PK1 --> PK2
    end

    subgraph HostRepo["服务器仓库 /root/finance_qa"]
        HR1["SKILL.md"]
        HR2["docs/SKILL_APPENDIX_FULL.md"]
        HR3["plugin/openclaw-finance/server/finance_bridge.py"]
        HR4["financeqa"]
        HR1 --> HR2
    end

    subgraph OpenClaw["OpenClaw 默认目录"]
        OC1["/root/.openclaw/skills/finance/SKILL.md"]
        OC2["/root/.openclaw/skills/finance/docs/SKILL_APPENDIX_FULL.md"]
        OC3["/root/.openclaw/extensions/openclaw-finance/server/finance_bridge.py"]
        OC1 -. symlink .-> HR1
        OC2 -. symlink .-> HR2
        OC3 -. symlink .-> HR3
        OC1 --> OC2
    end

    subgraph Claude["Claude Code 工作区"]
        CC1["workspace/SKILL.md"]
        CC2["workspace/docs/SKILL_APPENDIX_FULL.md"]
        CC1 --> CC2
    end

    subgraph Runtime["运行时"]
        RT1["宿主 LLM"]
        RT2["finance_bridge.py / financeqa CLI"]
        DB[("Configured DB<br/>PostgreSQL default / explicit SQLite")]
        CFG["config/rules.json / env"]
        OUT["结构化结果(JSON text)<br/>success + answer_method + data + trace + bridge_meta"]
        RT1 --> RT2
        RT2 -. 读取 FINANCEQA_SKILL_PATH 或 repo-root SKILL.md .-> HR1
        RT2 -. 读取契约版本 .-> HR1
        RT2 --> DB
        RT2 -. 规则加载 .-> CFG
        RT2 --> OUT
    end

    LR1 --> PK1
    LR2 --> PK2
    PK1 --> HR1
    PK2 --> HR2
    PK3 --> HR4
    CC1 --> RT1
    OC1 --> RT1
    HR3 --> RT2
    HR4 --> RT2
```

## 说明

1. 规则支持两种覆盖：`rules.json` 文件、环境变量。
2. bridge 运行时只读取 `SKILL.md` 顶部契约版本，不在桥接层重复注入 skill 正文；细粒度规则仍由宿主按相对路径读取 appendix。
3. OpenClaw / Claude Code 发布时必须保留 `SKILL.md -> docs/SKILL_APPENDIX_FULL.md` 这条相对路径。
4. 线上调用建议优先走结构化 JSON；当 `answer_method=llm_payload` 时，由宿主基于 `llm_payload` 做最终语言归纳。

## 默认路径

1. 本地仓库根：`/Users/.../finance_qa`
2. 服务器仓库根默认：`/root/finance_qa`
3. OpenClaw skill 目录默认：`/root/.openclaw/skills/finance`
4. OpenClaw extension 目录默认：`/root/.openclaw/extensions/openclaw-finance/server`
5. bridge 默认读取 skill 路径：
   - `FINANCEQA_SKILL_PATH`
   - 否则仓库根 `SKILL.md`

## 分发约束

1. `build_openclaw_package.sh` 必须把以下文件一起打包：
   - `SKILL.md`
   - `docs/SKILL_APPENDIX_FULL.md`
   - `bin/financeqa`
2. `sync_openclaw_bridge_and_skill.sh` 必须把以下文件一起同步到服务器仓库：
   - `SKILL.md`
   - `docs/SKILL_APPENDIX_FULL.md`
   - `plugin/openclaw-finance/server/finance_bridge.py`
3. OpenClaw skill 目录内必须保留：
   - `SKILL.md`
   - `docs/SKILL_APPENDIX_FULL.md`
4. 如果线上目录不是默认值，应通过这些变量覆盖：
   - `REMOTE_REPO_DIR`
   - `REMOTE_REPO_BRIDGE_DIR`
   - `REMOTE_OPENCLAW_SKILL_DIR`
   - `REMOTE_OPENCLAW_EXT_DIR`

## 运行时要点

1. OpenClaw 读取的是 skill 目录中的 `SKILL.md`，再按相对路径继续读取 `docs/SKILL_APPENDIX_FULL.md`。
2. Claude Code 通常直接在完整工作区中读取 `SKILL.md` 与 `docs/...`，不依赖 OpenClaw 目录。
3. bridge 只把 `SKILL.md` 当作契约版本来源，不把 appendix 正文注入返回。
4. 当 `finance-query` 不能稳定回答时，bridge 会自动补调 `financeqa host-data`，并把 `llm_payload` 留给宿主继续归纳。
