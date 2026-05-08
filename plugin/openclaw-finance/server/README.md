# FinanceQA Go MCP Runtime

## 迁移说明

旧版 Python bridge 脚本 (`finance_bridge.py`) 已废弃，改为使用 Go 原生的 MCP 服务器。

## 当前线上接入方式

当前线上 OpenClaw 不需要在 `openclaw.json` 里新增 `mcpServers`。OpenClaw 加载 `openclaw-finance` extension 后，由 extension 的 `dist/index.esm.js` 作为 MCP client，通过 stdio 启动 Go MCP：

```text
OpenClaw extension -> dist/index.esm.js -> ~/finance_qa/bin/financeqa serve
```

`FINANCEQA_BIN` 可覆盖二进制路径；线上固定路径是：

```bash
~/finance_qa/bin/financeqa
```

### 1. 直接运行 Go MCP 服务器

```bash
~/finance_qa/bin/financeqa serve [--db <dsn>] [--company <name>]
```

### 2. OpenClaw 配置要求

线上只需要确保 OpenClaw 已启用 finance extension 和全局 skill 路径：

```json
{
  "skills": {
    "load": {
      "extraDirs": ["/root/.openclaw/skills/finance"]
    }
  },
  "plugins": {
    "entries": {
      "openclaw-finance": {
        "enabled": true,
        "hooks": {
          "allowPromptInjection": true
        }
      }
    },
    "installs": {
      "openclaw-finance": {
        "source": "path",
        "sourcePath": "/root/.openclaw/extensions/openclaw-finance",
        "installPath": "/root/.openclaw/extensions/openclaw-finance",
        "version": "2.0.1"
      }
    }
  }
}
```

如果这几项已经存在，部署 Go MCP runtime、skill 或二进制时不需要修改 `openclaw.json` 的运行开关。当前线上 `openclaw.json` 的 `plugins.installs.openclaw-finance.version` 是 OpenClaw install metadata，需要与 Go MCP binary 和 OpenClaw plugin metadata 同步。

## 暴露的工具

- `finance-query` - 财务问答
- `finance-host-data` - 宿主LLM兜底数据
- `finance-upload` - 导入单个文件
- `finance-sync` - 批量同步目录
- `finance-dimensions` - 维度管理

## 兼容性

- 输出格式与原 Python bridge 完全一致
- `final_answer` 和 `host_summary_contract` 字段已原生支持
- 无需 Python 环境依赖
