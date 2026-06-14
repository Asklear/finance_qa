# Finance QA - Go 版本核心架构

`finance_qa` 是一个旨在替代传统手工处理，打通底层序时账（Journal）与宏观财务报表映射链条的智能化查询引擎系统。
本项目从原先的 Node.js/TypeScript 重构为了 **Go** 版本，显著提升了并发解析稳定性及数据处理速度。

## 一、核心特色能力

### 1. 老板口径优先的数据血缘
系统既保留底层“序时帐 / 银行流水 / 科目余额 / 利润表 / 资产负债表”的可追溯证据，也优先使用财务专家按老板意图整理后的合同维度台账：
- **老板口径主源**：收入、销售额、营收、成本、利润、客户、供应商、项目、合同类问题，优先探测 `fin_contracts + fin_fund_income + fin_cost_settlements`。
- **PDF 内容主源**：当用户问合同条款、合同全文、服务范围、付款条款、发票内容、发票号码、购买方/销售方或票面项目时，查询 `contract_main + contract_pages + contract_invoices`；这类问题不从 `fin_*` 经营台账推断。
- **底层财务证据**：当公司级合同/专家表无法覆盖问题时，默认先说明合同口径缺口；带有数据库确认真实主体的金额、付款、回款、应收、应付类问题可由后端显式标记后受控回退到账面或流水，用户显式要求账上或银行流水口径时也直接查对应底层证据。
- **全格式兼容**：内建 `Python/xlrd` 回落机制，支持老式用友/金蝶产生的 OLE2 腐败文件，并实现了**全自动多页签 (Multi-sheet) 遍历解析**。
- **智能期间识别**：支持复合期间报表（如 `2026.01-2026.02`）的无损提取与分录重组。
- **财务演算引擎**：`calculator.go` 自动利用财务准则聚沙成塔，可复核科目余额表与利润逻辑。

### 2. 老板口径优先 + 双视角兜底
系统不再把所有核心指标默认拆成“现金/财务账”两套口径。当前顺序是：
* **合同/专家表口径(老板默认口径)**：优先回答收入、销售额、营收、成本、利润、客户、供应商、项目和合同问题，并返回来源 Excel 说明。
* **业务现金流口径(明确问钱/到账/付款/银行卡时优先)**：直查银行流水，回答实际到账、实际支出、回款、付款、净增加等问题。
* **财务做账口径(经营/财务补充或兜底)**：严守权责发生制，用序时帐、利润表、科目余额等解释经营确认、应收应付、税额和利润差异。

## 核心能力

- **三位一体身份核验 (Trinity Identity Detector)**：系统不再盲目识别动词，而是通过**银行现金流向（In/Out）**、**会计科目归属（AR/AP）**及**税务特征（进项/销项）**三位一体交叉核验，自动锁定实体身为“客户”、“供应商”或“项目”。
- **Intent Router V2（可解释路由）**：查询入口统一走 V2 路由，返回 `intent_trace`（命中规则、得分、最终意图、置信度），支持冲突裁决和最小置信度回退。
- **老板问题改写与轻量探测**：先把自然语言问题改写为期间、指标、实体、粒度、口径等意图槽位，再利用表/字段注释和少量真实数据探测，判断合同/专家表是否能直接回答。
- **数据库候选实体识别 (DB Candidate Scoring)**：实体不再靠自由文本片段或修饰词黑名单直接判定，而是从银行流水、序时账摘要、合同客户和合同内容召回候选，再按完整命中、简称命中、来源可信度和候选差距打分；低置信度或多候选接近时拒绝把修饰词、指标词、时间词当实体。
- **审计穿透挖掘 (Summary Penetration)**：自动扫描序时账摘要字段，提取银行流水中缺失的往来单位信息。
- **自动日期锚定 (Dynamic Anchoring)**：智能识别数据库最新业务月份，确保模糊时间查询（如“今年”、“本月”）准确命中。
- **资产负债审计**：实时计算任意日期的科目余额与资产负债表勾稽关系。
- **零配置执行**：支持**项目根目录自动探测**（基于 `go.mod` 自动寻址），确保系统始终准确命中真相源。

## 二、运行与测试指南

### 1. 环境依赖
* **开发环境**：`Go >= 1.20`
* **底层库依赖（可选）**：`Python3 + xlrd` 仅在解析极老旧 XLS 文件时作为回退路径；常规场景可不安装。
* **数据库**：默认使用 PostgreSQL（通过 `.env` 或环境变量配置）；仅在显式传入 SQLite 路径时才走本地 SQLite 兼容模式。

### 1.1 账密管理（推荐 `.env`）

项目已支持自动加载 `.env`（不会覆盖已有环境变量）：

1. 复制 `.env.example` 为 `.env`
2. 填写 `PGHOST/PGPORT/PGUSER/PGPASSWORD/PGDATABASE`
3. 可选设置 `FINANCEQA_PG_SCHEMA`（默认 `tenant_uhub`）

注意：
1. `.env` 已加入 `.gitignore`，不会提交到仓库。
2. 线上建议把 `.env` 放在 `~/finance_qa/.env` 或 `/etc/financeqa/financeqa.env`，并通过 `FINANCEQA_ENV_FILE` 指向它，权限建议 `600`。
3. 飞书、OSS、Gemini 等密钥只放 `.env` 或线上 secret，不写入 README、日志或数据库。

线上 `.env` 至少需要包含：

```env
# PostgreSQL
PGHOST=...
PGPORT=5432
PGUSER=...
PGPASSWORD=...
PGDATABASE=...
FINANCEQA_PG_SCHEMA=tenant_uhub

# Feishu active scan
FEISHU_APP_ID=cli_xxx
FEISHU_APP_SECRET=replace_with_secret
FEISHU_AUTH_MODE=tenant
# FEISHU_AUTH_MODE=user
# FEISHU_USER_TOKEN_FILE=~/finance_qa/secrets/feishu_user_token.json
# FEISHU_OAUTH_REDIRECT_URI=http://127.0.0.1:8787/feishu/oauth/callback
FINANCEQA_FEISHU_SNAPSHOT_DIR=tmp/feishu_snapshots

# OSS ODS
OSS_ACCESS_KEY_ID=replace_with_access_key_id
OSS_ACCESS_KEY_SECRET=replace_with_access_key_secret
OSS_BUCKET=boss-agent
OSS_ENDPOINT=https://oss-cn-shenzhen.aliyuncs.com
OSS_CONTRACT_PREFIX=tenant/uhub/contract
OSS_FINANCE_PREFIX=tenant/uhub/finance
OSS_SMOKE_PREFIX=tmp/financeqa-smoke

# Gemini OCR
GEMINI_API_KEY=replace_with_key
GOOGLE_GEMINI_BASE_URL=https://api.ikuncode.cc
GEMINI_MODEL=gemini-3-flash-preview
GEMINI_PROXY=
OCR_WORKER_CONCURRENCY=2
```

### 2. 构建与运行
```bash
# 1. 编译系统
go build ./cmd/financeqa/...

# 如果从 macOS 本机部署到 Linux 服务器，使用交叉编译
GOOS=linux GOARCH=amd64 go build -o bin/financeqa ./cmd/financeqa

# 2. 从本地文件夹全量导入初始化数据库
./bin/financeqa sync "/path/to/exported/excel/files"

# 3. 命令行自然语言查询大体验
./bin/financeqa query --company "你的公司名称" "这个月花了多少钱"
./bin/financeqa query "今年客户收入总和汇总"
```

### 2.1 飞书主动扫描（V1）

V1 不需要 webhook 回调地址，也不需要轮询自己的回调接口；由服务器定时任务主动调用飞书 API，扫描已配置的飞书财务表格和 PDF 云盘文件夹。线上推荐用 `systemd timer` 管理扫描和 OCR，而不是 cron：可以用 `systemctl` 查看下次运行、失败状态和日志，并且 `oneshot` service 未结束时不会叠加启动同一个任务。

运行前需要设置 `FEISHU_APP_ID` 和 `FEISHU_APP_SECRET`，并确保飞书应用有云盘文件列表、下载、导出权限，且已被授权访问目标文件/文件夹。

飞书应用权限可导入：

```json
{
  "scopes": {
    "tenant": [
      "drive:drive:readonly",
      "drive:drive.metadata:readonly",
      "drive:file:readonly",
      "drive:export:readonly",
      "sheets:spreadsheet:readonly"
    ],
    "user": []
  }
}
```

还需要把应用授权到具体文档：PDF 来源文件夹至少可阅读/可下载，财务表格至少可阅读/可导出。只开应用权限但没有文档协作者授权时，真实扫描仍会被飞书拒绝。

如果目标文件夹/表格无法把应用加入协作者，可以改用用户身份扫描。先在飞书开放平台把 OAuth 回调地址配置为：

```text
http://127.0.0.1:8787/feishu/oauth/callback
```

然后通过 SSH 端口转发在服务器上执行一次用户授权：

```bash
export SERVER="lzh"
ssh -L 8787:127.0.0.1:8787 "$SERVER" 'cd ~/finance_qa && ./bin/financeqa feishu oauth-login --token-file ~/finance_qa/secrets/feishu_user_token.json'
```

命令会打印授权链接，在本机浏览器打开并同意授权；回调会通过 SSH 隧道写入服务器 token file。授权后服务器 `.env` 配置：

```env
FEISHU_AUTH_MODE=user
FEISHU_USER_TOKEN_FILE=~/finance_qa/secrets/feishu_user_token.json
FEISHU_OAUTH_REDIRECT_URI=http://127.0.0.1:8787/feishu/oauth/callback
```

后续 `feishu scan` 会自动刷新 `user_access_token`，不需要每次打开浏览器。

`seed-sources` 不再内置任何租户 token，必须先通过环境变量或 JSON 文件配置来源。服务器推荐使用文件，避免把很长的 JSON 写进 systemd unit：

```json
[
  {
    "source_type": "finance_workbook_folder",
    "source_token": "replace_with_finance_folder_token",
    "source_url": "https://example.feishu.cn/drive/folder/replace_with_finance_folder_token",
    "display_name": "飞书财务表文件夹",
    "metadata_json": {"oss_prefix": "tenant/uhub/finance"}
  },
  {
    "source_type": "pdf_folder",
    "source_token": "replace_with_folder_token",
    "source_url": "https://example.feishu.cn/drive/folder/replace_with_folder_token",
    "display_name": "飞书 PDF 文件夹",
    "metadata_json": {"oss_prefix": "tenant/uhub/contract"}
  }
]
```

`finance_workbook_folder` 是推荐配置：扫描器每次会列出共享文件夹内的工作簿，选择最新修改的 `.xlsx`/可导出表格下载导入，并把实际文件 token、文件名、OSS key 写回 `feishu_sync_sources.metadata_json`。这样原财务表被删除后重新上传为新文件，来源仍然是稳定的文件夹 token，不会因为旧文件 token 失效而中断。只有在确定财务表永远不会删除重传时，才使用固定文件 token 的 `finance_workbook`。

如果用户删除原财务表后重新上传一份新表，扫描器会把它当作文件夹中的新工作簿处理，但 OSS 不会盲目覆盖旧对象：先计算新表 SHA256，同 hash 会直接复用已有 `storage_key`；不同 hash 且目标历史路径已存在时，会写入带 hash 后缀的新对象，保留旧快照用于追溯。数据库中的来源状态只更新为最新文件 token、文件名、hash 和 `storage_key`。

```env
FEISHU_SYNC_SOURCES_FILE=~/finance_qa/secrets/feishu_sources.json
# 或 FEISHU_SYNC_SOURCES_JSON='[...]'
```

```bash
# 1. 写入已配置的飞书来源
./bin/financeqa feishu seed-sources

# 2. 查看来源状态
./bin/financeqa feishu sources

# 3. 扫描全部来源
./bin/financeqa feishu scan --company "南京优集数据科技有限公司"

# 4. 只扫描一个来源
./bin/financeqa feishu sync-once --source-token replace_with_source_token
```

PDF 扫描规则：扫描器会递归遍历飞书来源文件夹，按“来源根目录 + 相对路径 + 文件名”识别同一业务位置，内容 hash 变化才重新进入待 OCR 状态；合同 PDF 写入 `contract_main`，发票 PDF 写入 `contract_invoices`，云盘中消失的文件会在对应表标记为 `sync_status='deleted'`，不会硬删除 OCR 记录。路径中包含 `发票`、`开票` 或 `invoice` 的 PDF 会标记为发票，关系 key 取去掉发票目录后的业务目录；同一关系 key 下的发票通过 `contract_invoices.contract_id` 关联到合同，支持发票晚于合同上传。未匹配到合同的发票不会猜测落库，会等待下一轮扫描。财务表格按整份 `.xlsx` 快照导入，hash 未变则跳过，hash 变化则复用现有导入链路做整表刷新；导出的 Excel 批注/单元格备注会按单元格坐标保存到 `source_cell_notes`，覆盖 `fin_fund_income`、`fin_cost_settlements` 及对应合并组表；收入明细里可见的“备注”列会单独保存到 `fin_fund_income.remarks` 和 `fin_fund_income_groups.remarks`。如果某一行只有“备注”列有内容、没有任何金额列，导入器会保留一条当季末月的 0 金额 `fin_fund_income` 记录，便于查询谈判状态或备注金额，但不会影响收入、收款、开票合计。

如果配置了 `OSS_ACCESS_KEY_ID`、`OSS_ACCESS_KEY_SECRET`、`OSS_BUCKET`、`OSS_ENDPOINT`，扫描会把飞书原始 PDF/XLSX 快照上传到 OSS ODS 层，但只使用现有历史业务前缀：合同/发票默认在 `tenant/uhub/contract` 并保留飞书相对目录，财务表默认在 `tenant/uhub/finance`；只有文件名能识别年份，或 `feishu_sync_sources.metadata_json.oss_prefix` 明确指定 `tenant/uhub/finance/2025`、`tenant/uhub/finance/2026` 时，才进入年份子目录。合同/发票的 `contract_main.storage_key`、`contract_invoices.storage_key` 和财务来源的 `feishu_sync_sources.metadata_json.storage_key` 都保存 OSS object key 相对路径，例如 `tenant/uhub/contract/...pdf`，不保存 `s3://bucket/...`。上传前会先按 SHA256 查数据库；同 hash 直接复用已有 `storage_key`，不会重复上传。若目标 OSS key 已存在，会尝试读取远端对象 SHA256，一致时复用远端对象，不覆盖上传；不一致时才写入带 hash 后缀的新对象。未配置 OSS 时仍保留本地 snapshot 路径，便于本地开发。

历史库如果已有 `s3://boss-agent/...` 形式的 `storage_key`，执行一次迁移即可改成相对 key：

```bash
psql "$FINANCEQA_PG_DSN" -v ON_ERROR_STOP=1 -f db/migrations/20260505_relative_storage_keys.sql
```

如果飞书应用仍在审核中，无法真实调用飞书 API，可以先验证下游链路：

```bash
# OSS 真实上传/下载 smoke
RUN_LIVE_OSS_SMOKE=1 go test ./internal/storage -run TestLiveOSSUploadDownloadSmoke -count=1 -v

# 假设飞书已经返回文件后的 PDF/财务表处理逻辑
go test ./internal/feishusync -run 'TestPDFScanner|TestWorkbookScanner' -count=1
```

systemd 定时运行：

```bash
# 安装或更新 unit
sudo cp deploy/systemd/financeqa-feishu-scan.* /etc/systemd/system/
sudo cp deploy/systemd/financeqa-ocr-worker.* /etc/systemd/system/
sudo systemctl daemon-reload

# 启用定时器：飞书扫描在上次结束 10 分钟后再跑；OCR 在上次结束 5 分钟后再跑
sudo systemctl enable --now financeqa-feishu-scan.timer
sudo systemctl enable --now financeqa-ocr-worker.timer

# 查看运行计划、最近一次状态和日志
systemctl list-timers 'financeqa-*'
systemctl status financeqa-feishu-scan.service
journalctl -u financeqa-feishu-scan.service -n 100 --no-pager
```

### 2.2 Gemini OCR Worker

飞书扫描只负责把发生变化的 PDF 写入待 OCR 状态：合同进入 `contract_main.ocr_status='pending'`，发票进入 `contract_invoices.ocr_status='pending'`。OCR 独立运行，按批次消费两张表的 pending 记录；合同结果写回 `contract_main` 和 `contract_pages` 全文，发票结果更新同一条 `contract_invoices` 记录，不再把发票 PDF 放进 `contract_main`。

```bash
# 处理待 OCR 的 PDF
./bin/financeqa ocr process-pending --limit 10 --concurrency 2

# 使用 ikuncode 时不需要 GEMINI_PROXY；只有直连官方 Gemini 且网络需要时才设置代理

# 调试单个 PDF，不写数据库
./bin/financeqa ocr process-file --file "/path/to/sample.pdf"

# 也可以直接处理 contract_main.storage_key 中的 OSS 相对路径 PDF
./bin/financeqa ocr process-file --file "tenant/uhub/contract/优集客户合同/合同A.pdf"

# 调试单个 PDF，并写回指定 contract_main.id
./bin/financeqa ocr process-file --file "/path/to/sample.pdf" --contract-id 123
```

OCR worker 的 timer 使用 `deploy/systemd/financeqa-ocr-worker.*`。默认命令会读取 `.env` 中的 `OCR_WORKER_LIMIT` 和 `OCR_WORKER_CONCURRENCY`；也可以直接手动运行：

```bash
./bin/financeqa ocr process-pending --limit 10 --concurrency 2
systemctl status financeqa-ocr-worker.service
journalctl -u financeqa-ocr-worker.service -n 100 --no-pager
```

上线验收建议按顺序执行：

```bash
go test ./... -count=1
go build -o bin/financeqa ./cmd/financeqa/...
RUN_LIVE_OSS_SMOKE=1 go test ./internal/storage -run TestLiveOSSUploadDownloadSmoke -count=1 -v
./bin/financeqa feishu seed-sources
./bin/financeqa feishu scan --company "南京优集数据科技有限公司"
./bin/financeqa ocr process-pending --limit 10 --concurrency 2
```

飞书审核通过前，最后两步真实扫描可能失败；这时只代表飞书访问尚未完成，不代表 OSS、OCR 或数据库下游不可用。

### 3. 运行测试套件
新重构版本的测试已深度囊括各项业务边界：
```bash
# 执行审计回归报告，一键核验 17 道核心生产审计题 (南京优集实测集)
/opt/homebrew/bin/go run tests/scripts/prod_audit_regression.go

# 执行后端核心模块单测
go test ./internal/accounting/ -v
```

### 4. 规则配置化（统一入口：`config/rules.json`）

系统已支持把关键规则从硬编码抽离出来，便于线上快速调参。

1. 默认规则文件：`config/rules.json`
2. 启用文件覆盖：设置 `FINANCEQA_RULES_PATH`
3. 也可用环境变量直接覆盖

```bash
# 方式 1：加载规则文件
FINANCEQA_RULES_PATH=./config/rules.json ./bin/financeqa query --company "南京优集数据科技有限公司" "2026年2月收入/成本/利润分别是多少"

# 方式 2：直接覆盖 stopwords
FINANCEQA_METRIC_STOPWORDS="收入,成本,利润,经营状况" ./bin/financeqa query --company "南京优集数据科技有限公司" "飞未2月收入多少"
```

当前默认 schema 为 `schema_version = 2`，按词典域组织：

1. `router.stopwords.generic_metric`：泛指标词，避免被误识别成实体。
2. `router.intents.<intent>.keywords`：意图关键词。
3. `router.intents.<intent>.priority` / `min_confidence` / `conflicts` / `high_priority_phrases`：意图优先级与冲突裁决。
4. `router.metric_keywords`：收入 / 成本 / 利润等核心指标关键词。
5. `router.hr_breakdown_keywords`：工资 / 社保 / 公积金等拆分问法关键词。
6. `router.counterparty_classification_question_keywords`：如“成本还是收入”“客户还是供应商”等分类问法。
7. `router.profit_single_view_block_keywords`：出现这些词时，利润问题不走单一账面口径。
8. `router.fallback_monthly_expense_keywords`：整体支出 / 支出汇总类问法关键词。
9. `counterparty.roles`：客户 / 供应商 / 员工角色词典。
10. `counterparty.tax`：销项 / 进项税词典。
11. `counterparty.thresholds`：角色识别阈值。
12. `internal_party.org_suffixes`：内部主体组织后缀词典。
13. `internal_party.account_context_keywords`：内部往来/代发等上下文词典。

兼容说明：

1. 旧的平铺字段仍可继续读取一个过渡周期。
2. 环境变量覆盖顺序不变，仍然会在文件之后生效。
3. 查询层新的关键词规则请统一写入 `config/rules.json`。

支持的环境变量覆盖项：

1. `FINANCEQA_RULES_PATH`
2. `FINANCEQA_METRIC_STOPWORDS`（逗号分隔）
3. `FINANCEQA_ROLE_MIXED_MIN_RATIO`
4. `FINANCEQA_ROLE_MIXED_MIN_POSITIVE_SCORE`
5. `FINANCEQA_ROLE_MIXED_MIN_POSITIVE_ROLES`
6. `FINANCEQA_ROLE_MIN_PRIMARY_SCORE`
7. `FINANCEQA_ROLE_MIN_CONFIDENCE`
8. `FINANCEQA_INTENT_ARAP_KEYWORDS`
9. `FINANCEQA_INTENT_HR_COST_KEYWORDS`
10. `FINANCEQA_INTENT_TAX_KEYWORDS`
11. `FINANCEQA_INTENT_HEALTH_KEYWORDS`
12. `FINANCEQA_INTENT_FALLBACK_KEYWORDS`
13. `FINANCEQA_INTENT_ANALYSIS_KEYWORDS`
14. `FINANCEQA_INTENT_HOST_PAYLOAD_KEYWORDS`
15. `FINANCEQA_INTENT_MONTHLY_SUMMARY_KEYWORDS`
16. `FINANCEQA_FALLBACK_MONTHLY_EXPENSE_KEYWORDS`
17. `FINANCEQA_HIGH_PRIORITY_PHRASES`（JSON 对象）
18. `FINANCEQA_INTENT_PRIORITY`（JSON 对象）
19. `FINANCEQA_INTENT_CONFLICTS`（JSON 对象）
20. `FINANCEQA_INTENT_MIN_CONFIDENCE`（JSON 对象）

仍保留在代码中的内容：

1. 项目 + 指标的组合判断。
2. 开放项配对、置信度和财务证据逻辑。
3. 税额、收入、成本、现金桥等财务语义判断。
4. 内部转账、往来冲销等凭证级推断逻辑。

保留原则：

1. 高频、经常需要线上微调的词放到 `rules.json`。
2. 配置文件只放词表和阈值，组合逻辑与财务语义继续放代码里。
3. 遗留的 `internal/config/keywords_manager.go` 仅保留给旧 `query_keywords.json` 场景，查询主链路不再新增规则到那里。

## 三、代码集成与调用指南 (API / SDK)

除了使用 CLI 之外，本模块被设计为极具解耦性的 Go SDK。你可以非常简单地将其接入到任何现有的 HTTP 服务（如 Gin/Fiber/HTTPMUX）或者更大的 LLM RAG Agent 层中：

```go
package main

import (
	"fmt"
	"encoding/json"
	"financeqa/internal/query"
)

func main() {
	// 1. 实例化查询引擎 (需传入 PostgreSQL DSN 或显式 SQLite 路径，以及默认公司名)
	// 如果公司名传空，引擎会尝试自动从自然语言中提取
	engine, err := query.NewEngine("host=127.0.0.1 port=5432 user=finance password=secret dbname=finance search_path=tenant_uhub,public", "模拟财务")
	if err != nil {
		panic(err)
	}

	// 2. 将自然语言问题直接喂入 query 分析树
	// 解析器会自动执行：实体/期间/指标识别 -> 老板口径改写 -> 数据源轻量探测 -> PostgreSQL 查询或兜底数据包
	res := engine.Query("今年合作伙伴A客户销售额是多少")

	// 3. 处理结果 (res 包含 Success / Message / Data / SQL)
	if res.Success {
		// Data 为灵活的 map[string]any，包含了历史履历(history)和多口径对比
		output, _ := json.MarshalIndent(res.Data, "", "  ")
		fmt.Printf("查询成功: %s\n%s\n", res.Message, string(output))
	} else {
		// 捕捉 Fallback 未接住的意图断层
		fmt.Printf("查询失败: %s\n", res.Message)
	}
}
```

> **系统接入优势**：`engine.Query()` 是线程（Goroutine）安全的，因为底层使用的 `sql.DB` 自带连接池机制，你可以安全地在 Web 服务器的 handler 里面并发调用它。

## 四、 系统架构与模块设计

本项目采用分层解耦的 Go 后端架构，确保了从原始凭证解析到自然语言查询的全链路稳定性。

### 1. 架构图（分三张图）

为了提升可读性，原先“一张大图”已拆为三张独立图：

1. [分层架构图（Layered Architecture）](docs/architecture/01-layered-architecture.md)
2. [查询请求时序图（Query Sequence）](docs/architecture/02-query-sequence.md)
3. [部署与运行图（Deployment & Runtime）](docs/architecture/03-deployment-runtime.md)

阅读建议：
1. 先看分层图，理解系统边界；
2. 再看时序图，理解一次查询如何流转；
3. 最后看部署图，理解线上/本地如何运行与接入。

### 2. 逻辑分层
*   **接入层 (Parser & Ingest)**：处理各版本用友、金蝶及银行导出的 Excel 原始数据。具备自动脱敏、元数据提取（日期/公司识别）及数据清洗能力。
*   **持久层 (DB & Dimensions)**：PostgreSQL 优先，默认使用 `tenant_uhub` 等业务 schema；仅在显式传入 SQLite 路径时走本地兼容模式。采用多维模型（Dimensions）管理财务周期，支持快速切换公司与会计月份。
*   **计算层 (Accounting)**：核心业务大脑。实现了从“序时账”自动平衡“科目余额表”及“利润表”的算法，并支持合同/专家表老板口径、现金流口径、权责发生制口径之间的可解释切换；默认不把非合同口径自动冒充合同口径。
*   **查询层 (Query)**：混合式自然语言引擎。集成业务规则库、正则表达式匹配、Intent Router V2、老板问题改写、数据源能力目录和轻量探测；当规则计算无法稳定回答时，返回 `llm_payload` 给上层 Agent 做最终判别（本仓库不直接调用宿主 LLM）。

### 2. 目录结构
```text
finance_qa/
├── cmd/
│   └── financeqa/          # CLI 工具主入口 (main.go)
├── internal/               # 核心业务逻辑 (不向外部包公开)
│   ├── accounting/         # 财务结算引擎核心 (科目平衡、利润计算、双口径对比)
│   ├── analysis/           # 财务指标分析 (账龄分析、健康度评估)
│   ├── config/             # 配置管理与关键字管理
│   ├── db/                 # 数据库 Schema 管理与初始化
│   ├── dimensions/         # 财务维度建模与仓储模式
│   ├── ingest/             # 数据流水线与同步处理器
│   ├── parser/             # Excel 解析器与元数据自动提取
│   ├── query/              # 自然语言查询引擎 (含词法归一化、Intent Router V2、llm_payload 输出)
│   ├── support/            # 全局路径与工具支持
│   └── types/              # 通用数据结构定义
├── tests/                  # 跨包测试、集成回归、测试数据与测试脚本
│   ├── unit/               # 黑盒/契约型单元测试（按领域归类，不要求与源码同目录）
│   ├── integration/        # 集成/契约/回归测试（跨模块、桥接层、真实题库）
│   ├── testdata/           # 样本库 (已脱敏的典型财务报表样本)
│   ├── scripts/            # 测试与部署辅助脚本
│   └── README.md           # 测试目录放置规范
├── docs/                   # 项目说明文档
└── README.md
```

## 五、 环境与测试

### 1. 环境依赖
*   **Go**: `>= 1.20`
*   **Python3（可选）**: 仅在极老旧 XLS 容错回退场景下需要 `xlrd`。
*   **环境变量**: 默认无需 `OPENAI_API_KEY`；本仓库在兜底时输出 `llm_payload` 供上层 Agent 使用，不直接调用宿主 LLM。

### 2. 运行测试
本项目采用全自动化的集成测试套件，可一键验证重构后的业务逻辑对齐情况：
```bash
# 运行全部 Go 测试（包含 internal 包内测试 + tests 目录测试）
go test ./... -count=1

# 运行包内单元测试（适合改动 internal 逻辑后快速回归）
go test ./internal/... -count=1

# 运行 tests 目录下的黑盒/契约单元测试
go test ./tests/unit/... -count=1

# 运行集成测试 (全量覆盖业务场景)
go test ./tests/integration/... -count=1

# 运行回归检查工具 (自动输出生产提问审计对照表)
/opt/homebrew/bin/go run tests/scripts/prod_audit_regression.go

# 运行 20 道老板高频问题真实数据检查（JSON 题库驱动）
./tests/scripts/run_top20_realdata_check.sh

# 运行用户确认的 19 条问题真实数据检查
./tests/scripts/run_user19_realdata_check.sh
```

真实数据检查脚本的 Markdown 报告默认输出到 `scratch/reports/`，该目录属于本地运行产物，不纳入正式文档入口。

### 3. 测试文件应该放哪里
仓库当前采用混合布局，不要求所有 `*_test.go` 都放进顶层 `tests/`：

- 放在源码旁边：适合测试单个 package 的内部逻辑、未导出辅助函数、SQL 兼容边界。
- 放在 `tests/unit/`：适合黑盒单元测试、规则契约测试、跨多个 package 的轻量级行为验证。
- 放在 `tests/integration/`：适合集成测试、bridge/host 契约测试、真实数据题库回归、线上 smoke。
- 放在 `tests/testdata/`：仅存放测试输入、基准输出和报告样本，不放执行逻辑。
- 放在 `tests/scripts/`：仅存放测试驱动脚本、部署校验脚本和辅助工具。

快速判断标准：

- 如果测试必须和某个 package 紧耦合，优先就地放。
- 如果测试代表“外部调用者视角”，优先放 `tests/unit` 或 `tests/integration`。
- 如果测试会连 PostgreSQL、桥接层、CLI、线上环境，统一放 `tests/integration`。
- 新增测试前，先看 [tests/README.md](/Users/gaorongvc/work/other/finance_qa/tests/README.md) 的放置规范。

## 六、查询结果契约（对接层必读）

`query` 返回统一 JSON，至少包含以下字段：

1. `success`
2. `message`
3. `answer_method`（`sql` 或 `llm_payload`）
4. `data`
5. `executed_sql`
6. `calculation_logs`

`data` 中建议重点消费：

1. `intent_trace.router_version`
2. `intent_trace.matched`
3. `intent_trace.scores`
4. `intent_trace.final_intent`
5. `intent_trace.confidence`
6. `trace.executed_sql`
7. `trace.calculation_logs`

老板核心指标（收入/营收/销售额/成本/利润/客户/供应商/项目/合同）默认按“合同/专家表优先”输出：

1. 先探测 `fin_contracts + fin_fund_income + fin_cost_settlements` 是否能回答老板问题；能回答时，以这些表作为主要口径。
2. 用户问合同 PDF 或发票 PDF 的内容、条款、正文、全文、页码、发票号码、票面项目、购买方/销售方、税额、备注时，走 `contract_*` 明细库：合同主信息在 `contract_main`，合同 OCR 全文在 `contract_pages`，发票结构化 OCR 在 `contract_invoices`。
3. 合同/专家表不能覆盖时分两类处理：整公司核心汇总、合同/发票 PDF 原文问题保持严格合同口径并说明缺口；如果问题带有数据库候选确认过的真实主体，并且是金额、付款、回款、应收、应付这类可由财务账或流水回答的问题，后端可以显式返回 `contract_fallback_target` 并受控回退，但必须说明回退来源，不把非合同结果伪装成合同口径。
4. 问题明确包含 `银行`、`银行卡`、`到账`、`实际收款`、`实际支出`、`回款`、`付款` 等现金语义时，优先走银行流水。
5. 当问题继续追问 `差异原因`、`为什么不一样`、`回款和利润差异` 时，再展开 `cash_flow`、`difference_bridge` 与 `profit_cash_bridge`。
6. 老板可见回答必须翻译成财务概念与 Excel 来源，不展示数据库 ID、科目代码、SQL、`route_decision`、`probe_results` 等辅助字段。
7. 老板可见回答必须保留 `data.source_note` 和 `data.source_update_note`；不要从表名、SQL 或记忆中重写来源文件名。
8. 财务来源文件名和更新时间只从 `tenant_uhub.fin_file_mappings` 获取；没有映射就不展示该财务来源，不用表注释或硬编码文件名兜底。合同来源来自 `tenant_uhub.contract_main`，发票来源来自 `tenant_uhub.contract_invoices`。
9. 财务表中的 Excel 批注/单元格备注保存到 `source_cell_notes`，收入明细的独立“备注”列保存到 `remarks`；这些字段主要给宿主 LLM 解释谈判状态、备注金额、异常说明和数据来源，不作为默认老板可见字段。只有用户问备注、批注、谈判状态或来源依据时，才把它们翻译成业务语言展示。

Go MCP 层（`financeqa serve`）会额外补充：

1. `data.exposed_fields.dual_perspective`
2. `data.exposed_fields.hr_breakdown`
3. `data.exposed_fields.arithmetic_checks`
4. `data.exposed_fields.intent_trace`
5. `data.query_spec`
6. `data.route_decision`
7. `data.route_decision.probe_results`
8. `data.source_note`
9. `data.source_update_note`
10. `data.source_documents`
11. `data.primary_source_tables`
12. `data.supporting_source_documents`
13. `data.fact_sets` 或 `data.llm_payload` 中的 `source_cell_notes`
14. `data.fact_sets` 或 `data.llm_payload` 中的 `remarks`
15. `bridge_meta.skill_contract_version`
16. `bridge_meta.protocol_version`（当前 `v2`）
17. `bridge_meta.capabilities`
18. `bridge_meta.capabilities.exposed_tools`
19. `bridge_meta.skill_appendix_relative_path`
20. `bridge_meta.skill_appendix_path`
21. `bridge_meta.skill_appendix_exists`
22. `bridge_meta.final_answer_available`
23. `bridge_meta.final_answer_source`

来源规则：财务类来源文件名和更新时间统一来自 `fin_file_mappings`。如果某个财务来源没有映射，查询层不会再用表注释、旧文件名或硬编码文案兜底；这代表当前库里没有可对老板展示的来源文件。合同和发票文件分别从 `contract_main`、`contract_invoices` 取文件名、更新时间和 OSS 路径。财务事实行里的 `source_cell_notes` 和 `remarks` 是补充解释材料，宿主应保留给 LLM 兜底和审计，不要用它们替代 `source_note/source_update_note`。

说明：Go MCP 层不再读取/注入 `SKILL.md` 或 appendix 的正文规则，skill 仍由宿主（OpenClaw/Claude Code）skills 机制加载；但 Go MCP 层会读取 `SKILL.md` 顶部的契约版本标记，校验 appendix 相对路径是否存在，并把这些元数据写回响应。
当前推荐使用“核心版 SKILL + 附录”：

1. 核心注入：仓库根目录 `SKILL.md`（短上下文高准确）
2. 详细规则：`docs/SKILL_APPENDIX_FULL.md`（按需查阅）
3. 发布到 Claude Code / OpenClaw 时，需保留 `SKILL.md -> docs/SKILL_APPENDIX_FULL.md` 这条相对路径
4. 线上 OpenClaw 全局 skill 目录：`~/.openclaw/skills/finance -> ~/finance_qa`，必须用目录级软链接；文件级软链接会被 OpenClaw skill loader 判定为越过配置根目录并跳过
5. 线上 Claude Code 当前 skill 目录：`~/.claude/skills/finance -> ~/finance_qa`，同样用目录级软链接，避免重复更新 `SKILL.md` 与 appendix
6. OpenClaw extension 只保留 runtime 实文件，`~/.openclaw/extensions/openclaw-finance/skills/finance` 不再发布；extension 目录不改成指向仓库的 symlink，避免把源码/README 混入安装态 runtime
7. 旧路径 `~/.openclaw/workspace/skills/finance-orchestrator` 已废弃，不再作为发布或验证目标

## 七、MCP 模式部署（推荐）

系统已支持 **MCP (Model Context Protocol)** 模式。当前线上 OpenClaw 不通过 `openclaw.json.mcpServers` 注册独立 server，而是加载 `openclaw-finance` extension；extension 内部作为 MCP client，可选择本机 stdio 或远程 HTTPS MCP。

- 同机部署：OpenClaw extension 通过 stdio 启动 `~/finance_qa/bin/financeqa serve`，兼容既有路径。
- 分机部署：OpenClaw extension 只作为 thin connector，通过 `mcp_url` + `mcp_token_file` 调 FinanceQA 主机上的 `financeqa serve-http`。OpenClaw Agent 主机不需要数据库、飞书、OSS、Gemini 环境，也不需要 `financeqa` Go binary。

### 1. MCP 架构

本机 stdio 兼容路径：

```
┌─────────────────────────────────────────────────────────────┐
│                      OpenClaw Agent                        │
│                  ┌──────────────────┐                      │
│                  │ MCP Client (JS)  │                      │
│                  │ (index.esm.js)   │                      │
│                  └────────┬─────────┘                      │
└───────────────────────────┼─────────────────────────────────┘
                            │ stdio (JSON-RPC)
┌───────────────────────────┼─────────────────────────────────┐
│                      ┌────▼─────┐                            │
│                      │ MCP      │                            │
│                      │ Server   │                            │
│                      │ (Go)     │                            │
│                      └────┬─────┘                            │
│                           │                                  │
│  ┌────────────────────────┼──────────────────────────────┐  │
│  │        financeqa       │                              │  │
│  │  ┌─────────┬───────────┼───────────┬───────────────┐  │  │
│  │  │ ingest  │ dimensions│  query    │  accounting   │  │  │
│  │  └─────────┴───────────┴───────────┴───────────────┘  │  │
│  └────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

分机远程 MCP 路径：

```text
OpenClaw Agent Host
  ~/.openclaw/extensions/openclaw-finance/
  ~/.openclaw/openclaw.json
    plugins.entries.openclaw-finance.config:
      transport=remote
      mcp_url=https://financeqa.example.com/mcp
      mcp_token_file=/root/finance_qa/secrets/mcp_read_token
        |
        | HTTPS + Authorization: Bearer <token>
        v
FinanceQA Host
  Caddy/Nginx /mcp -> 127.0.0.1:3009/mcp
  systemd financeqa-mcp.service
  /root/finance_qa/bin/financeqa serve-http
  /root/finance_qa/secrets/mcp_read_token
  /root/finance_qa/secrets/mcp_admin_token
```

远程默认使用 read token，只开放 `finance-query`、`finance-host-data` 和 `finance-dimensions action=list`。`finance-upload`、`finance-sync` 和维度写操作需要 admin token，不建议给普通 OpenClaw Agent 配置。

### 2. OpenClaw 部署步骤

#### 同机 stdio 部署

```bash
# 1. 构建带符号剥离的二进制（减小体积 30-50%）
GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o bin/financeqa ./cmd/financeqa

# 2. 上传到服务器。SERVER 使用本地 SSH config alias；KEY_PATH 只有不走 SSH config 时才需要。
export SERVER="lzh"
scp bin/financeqa "$SERVER:/root/finance_qa/bin/"

# 3. 使用仓库源文件部署插件 runtime，脚本会本地交叉编译 Linux 二进制并上传
# OpenClaw extension 只放 runtime 实文件；OpenClaw/Claude skill 目录级软链接到 ~/finance_qa
# 脚本默认会在部署后重启 OpenClaw Gateway，确保新的 extension runtime 被加载
SERVER="$SERVER" tests/scripts/sync_openclaw_bridge_and_skill.sh

# 4. 核验 openclaw.json 运行配置，并同步 OpenClaw install metadata 版本
#    脚本只读校验 skill 路径和 openclaw-finance 插件开关；只写 plugins.installs.openclaw-finance.version/installedAt。
```

#### 分机 remote MCP 部署

FinanceQA 主机先生成 token 文件。token 只放远端 secrets 文件，不提交、不写入 `.env`、不贴到日志：

```bash
install -d -m 700 /root/finance_qa/secrets
openssl rand -base64 48 > /root/finance_qa/secrets/mcp_read_token
openssl rand -base64 48 > /root/finance_qa/secrets/mcp_admin_token
chmod 600 /root/finance_qa/secrets/mcp_read_token /root/finance_qa/secrets/mcp_admin_token
```

部署 FinanceQA MCP server：

```bash
SERVER="<financeqa-host>" MODE=server tests/scripts/sync_openclaw_bridge_and_skill.sh

ssh "<financeqa-host>" 'systemctl status financeqa-mcp.service --no-pager'
ssh "<financeqa-host>" 'curl -i http://127.0.0.1:3009/health'
```

Caddy 只暴露 HTTPS `/mcp`，不要直接暴露 PostgreSQL 或 `127.0.0.1:3009`：

```caddy
financeqa.example.com {
    handle /mcp {
        reverse_proxy 127.0.0.1:3009 {
            header_up Authorization {http.request.header.Authorization}
            header_up Host {http.request.host}
        }
    }
}
```

OpenClaw Agent 主机只部署 connector runtime 和 skill：

```bash
SERVER="<openclaw-agent-host>" MODE=connector tests/scripts/sync_openclaw_bridge_and_skill.sh
```

OpenClaw Agent 主机 `~/.openclaw/openclaw.json` 的插件配置增加 remote config：

```json
{
  "plugins": {
    "entries": {
      "openclaw-finance": {
        "enabled": true,
        "hooks": {
          "allowPromptInjection": true
        },
        "config": {
          "transport": "remote",
          "mcp_url": "https://financeqa.example.com/mcp",
          "mcp_token_file": "/root/finance_qa/secrets/mcp_read_token",
          "timeout_ms": 60000
        }
      }
    }
  }
}
```

`MODE=all` 适合同一台机器同时作为 FinanceQA server 和 OpenClaw connector；`MODE=server` 只部署 Go binary + `financeqa-mcp.service`；`MODE=connector` 只同步 OpenClaw extension runtime、skill 和 wrapper。

线上 `~/.openclaw/openclaw.json` 关键配置：

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
        "version": "2.1.1"
      }
    }
  }
}
```

说明：`plugin/openclaw-finance/server/README.md` 里的旧 `mcpServers` 示例已废弃；当前 extension 会自行连接 Go MCP，本机模式走 stdio，远程模式走 HTTPS `/mcp`。

### 3. 构建优化

**开发构建**（本地调试，含符号表）：
```bash
go build -o bin/financeqa ./cmd/financeqa
# 约 28MB（含 DWARF 调试信息）
```

**生产构建**（推荐，体积更小）：
```bash
go build -ldflags "-s -w" -o bin/financeqa ./cmd/financeqa
# 约 19MB（-30% 体积）
```

- `-s`：删除符号表
- `-w`：删除 DWARF 调试信息

### 4. MCP 暴露的工具

| 工具名 | 对应 CLI | 说明 |
|--------|----------|------|
| `finance-query` | `financeqa query` | 自然语言查询财务数据 |
| `finance-host-data` | `financeqa host-data` | 输出全量数据供 LLM 兜底 |
| `finance-upload` | `financeqa import` | 单文件导入 |
| `finance-sync` | `financeqa sync` | 目录批量导入 |
| `finance-dimensions` | `financeqa dimensions` | 维度管理 |

### 5. 版本兼容性

- finance_qa / Go MCP: `~/finance_qa/bin/financeqa version`（当前 `2.1.1`）
- OpenClaw Plugin: `~/.openclaw/extensions/openclaw-finance/package.json`（当前 `2.1.1`，且 `openclaw.extensions` 必须包含 `./dist/index.esm.js` 才能被 OpenClaw 发现）
- OpenClaw Config: `~/.openclaw/openclaw.json` 的 `plugins.installs.openclaw-finance.version` 需与插件版本同步；运行配置只校验 `skills.load.extraDirs`、`plugins.entries.openclaw-finance.enabled` 和 `hooks.allowPromptInjection`

Go MCP、OpenClaw Plugin metadata 与 `openclaw.json` 中的 OpenClaw install metadata semver 需要保持同步；`plugins.entries` 运行开关只做启用配置校验。
替换 `~/.openclaw/extensions/openclaw-finance` 的 runtime 文件后必须重启 OpenClaw Gateway，否则 live agent 可能继续使用内存里的旧插件实例。

## 八、Agent 对接能力矩阵

为保证 OpenClaw / Claude Code 全面调用代码库功能，建议按下表接入：

1. Go MCP 工具（开箱即用）：
   - `finance-query` → `financeqa query`
   - `finance-host-data` → `financeqa host-data`
   - `finance-upload` → `financeqa import`（单文件）
   - `finance-sync` → `financeqa sync`（目录批量导入）
   - `finance-dimensions` → `financeqa dimensions ...`（维度导入导出与规则管理）
2. 直接 CLI/SDK（显式维护时使用）：
   - `financeqa config show`
   - `financeqa keywords intents`
   - Go SDK：`query.NewEngine / Engine.Query / Engine.HostLLMPayload`
3. OpenClaw/Claude 调 Go MCP 时，`finance-query` 的推荐调用格式是：

```json
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"finance-query","arguments":{"query":"2026年Q1利润多少？"}}}
```

## 八、回答红线（不能犯）

1. 不能把“银行到账”直接当“当月收入确认”。
2. 不能把供应商付款/工资/税费误归因成收入差异。
3. 不能在证据不足时编造结算月份、合同归属或开票归属。
4. 面向老板不能只回一个数字，必须保留口径、期间、主要来源和必要解释；但不要展示老板看不懂的数据库辅助字段。
5. 不能只看 CLI 退出码；业务失败时仍要先解析 stdout 的结构化 JSON（其中可能含 `llm_payload`）。
6. 接口层可以保留 `executed_sql`、`calculation_logs`、`trace`、`route_decision` 等调试字段，但宿主给老板总结时必须转成业务语言。
