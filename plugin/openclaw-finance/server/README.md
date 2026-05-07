# FinanceQA MCP Server

## 迁移说明

旧版 Python bridge 脚本 (`finance_bridge.py`) 已废弃，改为使用 Go 原生的 MCP 服务器。

## 使用方法

### 1. 直接运行 MCP 服务器

```bash
financeqa serve [--db <dsn>] [--company <name>]
```

### 2. 配置 OpenClaw

在 OpenClaw 配置中添加 MCP server:

```json
{
  "mcpServers": {
    "finance": {
      "command": "/path/to/financeqa",
      "args": ["serve", "--db", "your-db-dsn"]
    }
  }
}
```

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
