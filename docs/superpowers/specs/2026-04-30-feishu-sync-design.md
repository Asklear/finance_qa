# 飞书云盘与飞书表格同步设计

状态：Draft  
日期：2026-04-30  
范围：基于当前 `finance_qa` 代码库和 `tenant_uhub` 现有合同/OCR/财务表，设计飞书 PDF 合同/发票与飞书财务表格的自动同步机制。

V1 实现范围：仅实现主动扫描。webhook 仍保留为 V2 增强，不影响 V1 通过 cron/systemd timer 主动调用飞书 API 达到最终一致。

## 1. 背景

当前系统已经存在两类数据链路：

1. 合同/发票 PDF OCR 链路：`tenant_uhub.contract_main`、`tenant_uhub.contract_categories`、`tenant_uhub.contract_duplicate_logs`，以及查询侧已使用的 `contract_pages`、`contract_invoices`、`contract_invoice_summaries` 等 `contract_*` 表。
2. 合同/项目财务表格链路：`tenant_uhub.fin_contracts`、`tenant_uhub.fin_fund_income`、`tenant_uhub.fin_cost_settlements`，以及合并金额相关的 group/member 表。

代码库中 `internal/ingest/contracts.go` 已经能解析合同收入、成本和资金收入工作簿，并能维护：

1. `fin_contracts`
2. `fin_fund_income`
3. `fin_cost_settlements`
4. `fin_cost_settlement_groups`
5. `fin_cost_settlement_group_members`
6. `fin_fund_income_groups`
7. `fin_fund_income_group_members`

因此飞书同步不应重新设计一套数据表或绕过导入层，而应作为远程采集层接入现有 OCR 和 ingest 逻辑。

## 2. 目标

1. 从飞书云盘文件夹自动同步 PDF 合同/发票。
2. 从飞书表格或飞书云盘中的财务工作簿自动更新 `fin_*` 合同/项目财务表。
3. 使用 webhook 提升实时性，使用主动扫描保证最终一致。
4. 复用现有 `contract_*` 表和 `fin_*` 表。
5. 增加一张轻量飞书状态表，保存来源配置、同步游标、hash 和错误信息。
6. 避免上传未完成时启动 OCR 或导入。
7. 保留可审计性：重复上传、同名替换、删除后重传都可追踪。

## 3. 非目标

首版不做以下事情：

1. 不新增完整文档版本树。
2. 不把 PDF OCR 数据自动关联到 `fin_contracts` 的经营结算口径。
3. 不根据 OCR 文本自动判断合同/发票是否应覆盖某个业务对象。
4. 不做飞书表格行级增量更新。
5. 不在 webhook 请求内下载、OCR、导入或清库。
6. 不因为飞书财务表格被删除就自动清空 `fin_*` 数据。

## 4. 已知飞书来源

### 4.1 财务表格

```text
https://ucngfmhi7qmy.feishu.cn/file/Iel5bFZWSoGF7hxjyPpcn5Elnqd
```

当前 token：

```text
Iel5bFZWSoGF7hxjyPpcn5Elnqd
```

实现时需要先通过飞书 Drive 元数据确认它是上传的 Excel 文件、飞书在线表格，还是其他云文档类型：

1. 如果是 Excel 文件，直接下载为 `.xlsx`。
2. 如果是飞书在线表格，优先通过导出任务导出为 `.xlsx`。
3. 不建议首版直接用 Sheets values API 读取单元格，因为合并单元格信息会影响 group/member 表的正确性。

### 4.2 PDF 云盘文件夹

```text
https://ucngfmhi7qmy.feishu.cn/drive/folder/JeTEfS3qQly8RJd0CJNcASumnCg
https://ucngfmhi7qmy.feishu.cn/drive/folder/S4Q0fl7AwlUbjedXUzDcP0panid
https://ucngfmhi7qmy.feishu.cn/drive/folder/FB8dfZLpQlHmuFdwsWKc5tJ5nJc
```

当前 folder tokens：

```text
JeTEfS3qQly8RJd0CJNcASumnCg
S4Q0fl7AwlUbjedXUzDcP0panid
FB8dfZLpQlHmuFdwsWKc5tJ5nJc
```

## 5. 核心原则

### 5.1 webhook 是提醒，主动扫描是事实确认

飞书 webhook 只负责告诉系统“某个来源可能变化了”。系统不能直接相信 webhook 事件并立刻写业务数据。

```text
Feishu webhook
  -> 标记 feishu_sync_sources 为 pending
  -> 快速返回成功
  -> worker 延迟处理
```

实际状态以 worker 主动调用飞书 API 后得到的文件夹列表、文件元数据、下载结果和内容 hash 为准。

### 5.2 PDF 用 hash 去重，用同文件夹同文件名表示当前业务位置

在只有 PDF 和文件名、没有合同编号/发票号码的前提下，系统无法可靠判断“这是业务上的同一份合同修正版，还是另一份新合同”。因此首版采用明确产品规则：

```text
同文件夹 + 同文件名 = 同一个业务位置的当前版本
PDF 内容 hash = 是否需要重新 OCR 的依据
```

### 5.3 财务表格整份快照替换，不做行级增量

财务表格会影响合并金额 group/member 表。用户删除行、移动行或改变合并单元格时，行级增量很容易造成残留 group/member 关系。因此首版按整份工作簿快照处理：

```text
导出或下载 xlsx
  -> 计算 snapshot hash
  -> hash 未变则跳过
  -> hash 变化则调用现有 Importer，incremental=false
```

现有导入逻辑会按 `source_report_type + source_sheet_name` 清理旧数据并写入新数据。

## 6. 数据模型

### 6.1 新增 `feishu_sync_sources`

这张表只保存飞书来源级状态和同步游标，不保存每个 PDF 的完整版本历史。

建议 schema：

```sql
CREATE TABLE tenant_uhub.feishu_sync_sources (
    id BIGSERIAL PRIMARY KEY,
    source_type TEXT NOT NULL,
    source_token TEXT NOT NULL,
    source_url TEXT,
    display_name TEXT,
    parent_token TEXT,
    sync_mode TEXT NOT NULL DEFAULT 'webhook_and_scan',
    sync_status TEXT NOT NULL DEFAULT 'active',
    last_revision TEXT,
    last_content_hash TEXT,
    last_event_at TIMESTAMP,
    next_scan_at TIMESTAMP,
    last_sync_at TIMESTAMP,
    last_success_at TIMESTAMP,
    error_message TEXT,
    metadata_json JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(source_type, source_token)
);
```

`source_type` 建议取值：

```text
pdf_folder
finance_workbook
```

`sync_status` 建议取值：

```text
active
pending
missing
error
disabled
```

首批记录：

```text
finance_workbook / Iel5bFZWSoGF7hxjyPpcn5Elnqd
pdf_folder / JeTEfS3qQly8RJd0CJNcASumnCg
pdf_folder / S4Q0fl7AwlUbjedXUzDcP0panid
pdf_folder / FB8dfZLpQlHmuFdwsWKc5tJ5nJc
```

### 6.2 `contract_main` 飞书字段

PDF 单文件状态挂在 `contract_main` 上，避免另建 OCR 状态表。

如果真实表已有等价字段，复用已有字段；没有时建议补：

```sql
ALTER TABLE tenant_uhub.contract_main
    ADD COLUMN IF NOT EXISTS feishu_file_token TEXT,
    ADD COLUMN IF NOT EXISTS feishu_parent_token TEXT,
    ADD COLUMN IF NOT EXISTS feishu_slot_key TEXT,
    ADD COLUMN IF NOT EXISTS feishu_file_name TEXT,
    ADD COLUMN IF NOT EXISTS feishu_modified_time TIMESTAMP,
    ADD COLUMN IF NOT EXISTS feishu_deleted_at TIMESTAMP,
    ADD COLUMN IF NOT EXISTS file_size BIGINT,
    ADD COLUMN IF NOT EXISTS sync_status TEXT,
    ADD COLUMN IF NOT EXISTS ocr_status TEXT,
    ADD COLUMN IF NOT EXISTS last_seen_at TIMESTAMP;
```

建议索引：

```sql
CREATE INDEX IF NOT EXISTS idx_contract_main_feishu_token
    ON tenant_uhub.contract_main(feishu_file_token);

CREATE INDEX IF NOT EXISTS idx_contract_main_feishu_slot
    ON tenant_uhub.contract_main(feishu_slot_key);

CREATE INDEX IF NOT EXISTS idx_contract_main_file_hash
    ON tenant_uhub.contract_main(file_hash);
```

`feishu_slot_key`：

```text
feishu_parent_token + ":" + normalized(file_name)
```

### 6.3 `contract_duplicate_logs`

继续使用现有重复上传日志表。若字段不足，优先增加 JSON 扩展字段，而不是再建新表。

建议至少能记录：

```text
event_type             -- same_hash_duplicate / same_slot_replaced / deleted_then_reuploaded
source_file_token
target_contract_id
existing_contract_id
file_hash
old_file_hash
slot_key
message
metadata_json
created_at
```

如果现有字段不同，保留现有字段名，只要能表达上述信息即可。

## 7. PDF 云盘同步流程

### 7.1 webhook 处理

```text
收到飞书事件
  -> 验签
  -> 解析 file_token / folder_token
  -> 判断是否属于配置的 pdf_folder
  -> 将对应 feishu_sync_sources 标为 pending
  -> 设置 next_scan_at = now + 1-3 minutes
  -> 返回飞书成功
```

webhook 不下载文件，不启动 OCR，不修改财务事实。

### 7.2 worker 处理 PDF 文件夹

```text
扫描 pending 或到期的 pdf_folder
  -> 拉取文件夹当前 PDF 文件列表
  -> 对每个 PDF 拉元数据
  -> 对新增/变更文件做稳定检查
  -> 下载 PDF
  -> 计算 file_hash
  -> 基于 token / slot / hash 决策
  -> 创建或复用 OCR
  -> 更新 contract_main
  -> 写 contract_duplicate_logs
```

### 7.3 稳定检查

上传未完成时不进入 OCR：

```text
文件不可下载 -> 重试
size 为空或为 0 -> 重试
modified_time 刚变化 -> 延迟重试
下载后 hash 变化 -> 延迟重试
连续两次元数据稳定 -> 处理
```

### 7.4 PDF 决策表

| 场景 | 处理 |
|---|---|
| 新 token + 新 slot + 新 hash | 新增 `contract_main`，创建 OCR |
| 新 token + 已存在 hash | 不重新 OCR，复用已有 OCR，写 `same_hash_duplicate` |
| 同 token + hash 变化 | 同一飞书文件更新，重新 OCR，更新当前记录 |
| 同 slot，旧 token 删除后新 token 出现，hash 相同 | 恢复或复用 OCR，写 `deleted_then_reuploaded` |
| 同 slot，hash 不同 | 视为替换当前版本，重新 OCR，写 `same_slot_replaced` |
| 飞书文件删除 | `sync_status='deleted'`，写 `feishu_deleted_at`，不硬删 OCR |

### 7.5 删除处理

删除只软删除：

```sql
UPDATE tenant_uhub.contract_main
SET sync_status = 'deleted',
    feishu_deleted_at = CURRENT_TIMESTAMP
WHERE feishu_file_token = $1;
```

合同明细查询默认排除删除记录：

```text
sync_status IS NULL OR sync_status = 'active'
```

历史 OCR、分页正文、发票抽取结果和重复日志保留，用于审计和恢复。

## 8. 飞书财务表格同步流程

### 8.1 webhook 处理

```text
收到飞书文件或表格变更事件
  -> 判断 token 是否是 finance_workbook
  -> 标记 feishu_sync_sources.sync_status = pending
  -> next_scan_at = now + 1-3 minutes
  -> 返回成功
```

### 8.2 worker 处理财务表格

```text
扫描 pending 或到期的 finance_workbook
  -> 拉飞书元数据
  -> 判断文档类型
  -> 如果是 Excel 文件，下载 xlsx
  -> 如果是飞书在线表格，导出 xlsx
  -> 等待导出/下载稳定
  -> 计算 snapshot_hash
  -> 与 feishu_sync_sources.last_content_hash 比较
  -> hash 未变：跳过
  -> hash 变化：保存快照，调用现有 Importer
```

### 8.3 导入方式

首版应复用现有逻辑：

```go
importer.ImportFileWithOptions(ctx, dbPath, snapshotPath, ingest.ImportOptions{
    Incremental: false,
    CompanyOverride: "...",
})
```

`incremental=false` 的原因：

1. 财务表格可能删除行。
2. group/member 表依赖合并单元格和行号。
3. 同步源是一个“当前快照”，不是追加流水。
4. 现有 `replaceRevenueCostSettlements` / `replaceFundIncomeRows` 已按 source scope 清理旧数据。

### 8.4 财务表格删除

如果飞书财务表格本身被删除或不可访问：

```text
feishu_sync_sources.sync_status = missing 或 error
记录 error_message
告警
不自动清空 fin_* 表
```

清空 `fin_*` 属于高风险动作，应设计独立的人工命令，例如：

```text
financeqa feishu purge-finance-source --source-token <token>
```

首版不自动执行。

## 9. 主动扫描策略

### 9.1 高频扫描

worker 定期处理：

```text
sync_status = pending
OR next_scan_at <= now()
```

### 9.2 兜底扫描

每 30-60 分钟扫描所有 `active` 来源：

```text
pdf_folder:
  -> 拉取文件夹当前 PDF 列表
  -> 发现新增 token：创建 pending
  -> 发现 active token 不存在：软删除

finance_workbook:
  -> 拉元数据或 revision
  -> 发现 revision/hash 变化：创建 pending
  -> 不可访问：标 missing/error，不清库
```

兜底扫描用于修正 webhook 漏事件、乱序事件、移动/删除事件。

## 10. 与现有代码的关系

### 10.1 新增模块

建议新增：

```text
internal/feishu
  auth.go             -- tenant_access_token / app_access_token
  drive.go            -- 文件夹列表、文件元数据、文件下载
  export.go           -- 云文档导出 xlsx
  webhook.go          -- 验签、challenge、事件解析

internal/feishusync
  sources.go          -- feishu_sync_sources 仓储
  pdf_worker.go       -- PDF 文件夹同步
  workbook_worker.go  -- 财务表格同步
  scheduler.go        -- pending + 兜底扫描
```

### 10.2 CLI

建议新增命令：

```text
financeqa feishu webhook-server
financeqa feishu sync-once --source-token <token>
financeqa feishu scan
financeqa feishu sources
```

首版也可以先不做常驻 server，只实现 `scan` 和 `sync-once` 验证链路，再接入服务端 webhook。

### 10.3 查询层

合同明细查询需要过滤软删除 PDF：

```text
contract_main.sync_status IS NULL OR contract_main.sync_status = 'active'
```

财务问答层无需感知飞书，只继续查询 `fin_*` 表。

## 11. 配置

建议环境变量：

```text
FEISHU_APP_ID
FEISHU_APP_SECRET
FEISHU_WEBHOOK_VERIFY_TOKEN
FEISHU_WEBHOOK_ENCRYPT_KEY
FINANCEQA_FEISHU_SCAN_INTERVAL_SECONDS
FINANCEQA_FEISHU_STABILITY_DELAY_SECONDS
FINANCEQA_FEISHU_SNAPSHOT_DIR
```

来源配置写入 `feishu_sync_sources`，不要只写死在代码中。

## 12. 错误处理

### 12.1 webhook 错误

webhook 只要验签和入队成功，就返回成功。下载、导出、OCR、导入失败不影响 webhook 返回。

### 12.2 下载或导出失败

记录：

```text
feishu_sync_sources.sync_status = error
error_message = ...
next_scan_at = now + backoff
```

### 12.3 OCR 失败

更新 `contract_main.ocr_status = failed`，保留文件 hash 和 storage key，允许后续重试。

### 12.4 财务表格导入失败

不更新 `last_content_hash`，避免错误快照被视为已成功同步。保留失败快照路径和错误信息，方便排查。

## 13. 测试用例

### 13.1 PDF 同步

1. 新 PDF 上传：创建 `contract_main`，创建 OCR。
2. 同 hash 重复上传：不重复 OCR，写 `contract_duplicate_logs`。
3. 同文件夹同文件名不同 hash：重新 OCR，写替换日志。
4. 删除 PDF：软删除 `contract_main`，查询不返回该合同。
5. 删除后同名重传且 hash 相同：恢复 active，复用 OCR。
6. webhook 早于上传完成：pending 重试，稳定后才 OCR。

### 13.2 财务表格同步

1. 首次同步 xlsx：导入 `fin_contracts` 和 `fin_*`。
2. hash 未变：跳过导入。
3. 修改收入 sheet：重导 `fin_fund_income`。
4. 修改成本 sheet：重导 `fin_cost_settlements` 和 cost group/member。
5. 修改合并单元格：group/member 正确替换，不残留旧 group。
6. 飞书表格不可访问：标 error/missing，不清空 `fin_*`。

## 14. 参考资料

1. 飞书事件订阅概览：`https://open.feishu.cn/document/server-docs/event-subscription-guide/overview`
2. 飞书云文档下载：`https://open.feishu.cn/document/server-docs/docs/drive-v1/download/download`
3. 飞书云文档导出任务：`https://open.feishu.cn/document/server-docs/docs/drive-v1/export/export-task`
4. 飞书电子表格 API 概览：`https://open.feishu.cn/document/server-docs/docs/sheets-v3/overview`

## 15. 待确认事项

1. 真实 `tenant_uhub.contract_main`、`contract_duplicate_logs` 已有哪些字段；本设计中的字段应优先映射到现有字段。
2. 财务表格 token `Iel5bFZWSoGF7hxjyPpcn5Elnqd` 的实际类型：Excel 文件、飞书在线表格或其他云文档。
3. OCR 任务当前由哪个服务创建，`contract_main` 与 OCR job 的状态字段是否已有标准枚举。
4. webhook 服务部署位置，以及是否需要飞书事件加密解密。
5. 财务表格同步时的公司名覆盖策略，是否固定为 `FINANCEQA_DEFAULT_COMPANY`。
