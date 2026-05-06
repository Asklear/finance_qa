# docs 目录说明

本目录只保留当前代码仍然适用的正式文档。

## 正式文档

1. `SKILL_APPENDIX_FULL.md`：OpenClaw / Claude Code 按需读取的完整财务问答规则附录。
2. `architecture/01-layered-architecture.md`：当前分层架构图。
3. `architecture/02-query-sequence.md`：当前查询请求时序图。
4. `architecture/03-deployment-runtime.md`：当前部署与运行图，包含 OpenClaw/Claude bridge、飞书主动扫描、OSS ODS、Gemini OCR worker、定时任务和上线验收步骤。
5. `architecture/04-code-quality-audit.md`：当前代码重复度、模块正交性和测试覆盖整改清单。
6. `calc-plan.md`：`calc_plan` 结构化计算协议说明。

## 不再保留的文档类型

1. 历史修复计划、历史实施计划、历史设计草稿。
2. 已执行完毕或与当前目录结构不一致的 `docs/superpowers/*` 计划文档。
3. 旧口径测试报告或会误导当前老板口径的历史问答输出。
4. 与当前合同/专家表优先口径冲突的旧财务逻辑说明。

如果需要恢复历史文档，请从 Git 历史中按提交查看，不要重新放回当前正式文档入口。
