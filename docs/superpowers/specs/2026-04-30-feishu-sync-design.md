# 飞书云盘与飞书表格同步设计

状态：Draft
日期：2026-04-30
范围：基于当前 `finance_qa` 代码库和 `tenant_uhub` 现有合同/OCR/财务表，设计飞书 PDF 合同/发票与飞书财务表格的自动同步机制。

V1 实现范围：仅实现主动扫描。webhook 仍保留为 V2 增强，不影响 V1 通过 cron/systemd timer 主动调用飞书 API 达到最终一致。

2026-05-01 更新：PDF OCR 主链路从原 `contract_ocr` 的 PaddleOCR + 正则方案，调整为 Gemini 直接识别 PDF 并输出结构化 JSON。`contract_ocr` 可继续作为历史工具和回退参考，但 V1 新增能力优先落在 `finance_qa` 内部的异步 Gemini OCR worker。

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

## 8. Gemini OCR 服务设计

### 8.1 定位

Gemini OCR 不嵌入飞书扫描流程，而是作为独立异步 worker 消费 `contract_main.ocr_status='pending'` 的 PDF。

```text
Feishu active scan
  -> 下载 PDF 快照
  -> 计算 file_hash
  -> 创建或更新 contract_main
  -> ocr_status = pending

Gemini OCR worker
  -> claim pending PDF
  -> 调 Gemini 直接识别 PDF
  -> 判断 document_type = contract / invoice / unknown
  -> 写 contract_main / contract_pages / contract_invoices
  -> ocr_status = done / failed
```

这样做的原因：

1. 飞书扫描保持快速、可重试，不被 OCR 网络耗时阻塞。
2. OCR 可独立限流、重试和统计成本。
3. 与当前 V1 主动扫描策略一致，不需要 webhook 或额外队列表。
4. 复用现有 `contract_main.ocr_status`，避免新增复杂状态模型。

### 8.2 模型选择和实测结论

当前实测模型：

```text
gemini-3.1-flash-lite-preview
```

Google 模型列表中没有裸的 `gemini-3.1-flash`。本设计按 3.1 Flash Lite Preview 落地，后续通过环境变量切换模型。

6 份真实 PDF 样本实测结果：

| 样本 | 类型 | 耗时 | 估算成本 |
|---|---:|---:|---:|
| 常年法律顾问合同 | 合同 | 5.72s | 约 ¥0.0111 |
| 百度边缘计算资源采购合同 | 合同 | 7.61s | 约 ¥0.0195 |
| 五块石数据服务合同 | 合同 | 6.59s | 约 ¥0.0127 |
| 五块石 30w 发票 | 发票 | 5.43s | 约 ¥0.0061 |
| 欧特欧 80w 发票 | 发票 | 5.19s | 约 ¥0.0062 |
| 众信 10 月发票 | 发票 | 4.31s | 约 ¥0.0065 |

平均约 5.81s/份，6 份总成本约 ¥0.062。合同金额语义明显好于正则，例如能正确区分“年度叁万元”和“两期各 15000 元”。

### 8.3 与现有表的映射

#### 8.3.1 `contract_main`

合同 PDF 识别后写入：

```text
contract_number
contract_title
party_a
party_a_credit_code
party_b
party_b_credit_code
sign_date
start_date
end_date
contract_amount
amount_currency
settlement_cycle
settlement_unit_price
price_unit
payment_terms
payment_method
tax_rate
service_scope
total_pages
processed_at
ocr_engine
ocr_status
extension_data
custom_metrics
```

文件和飞书字段仍由飞书扫描维护：

```text
file_name
relative_path
storage_key
file_size
file_hash
feishu_file_token
feishu_parent_token
feishu_slot_key
feishu_file_name
feishu_modified_time
sync_status
last_seen_at
```

`extension_data.gemini_ocr` 保存模型结构化输出、分期明细、证据片段和错误信息。`custom_metrics.gemini_ocr` 保存耗时、token、估算成本和质量评分。

#### 8.3.2 `contract_pages`

Gemini V1 不强制保存每页全文，因为当前目标是结构化识别。为保留查询证据，至少写一条 evidence page：

```text
contract_id
page_num = 0
page_number = 1
markdown_text = ocr_text_excerpt
plain_text = ocr_text_excerpt
raw_ocr_json = Gemini 原始响应或结构化响应
has_images = true
word_count / char_count
```

如果后续需要合同全文问答，再升级 prompt 让 Gemini 返回 `pages[]`，按页写入 `contract_pages`。

#### 8.3.3 `contract_invoices`

发票 PDF 识别后，在能可靠关联合同的前提下写入：

```text
contract_id
invoice_code
invoice_number
invoice_type
issue_date
check_code
machine_number
tax_bureau_code
tax_bureau_name
buyer_name
buyer_tax_id
buyer_address_phone
buyer_bank_account
seller_name
seller_tax_id
seller_address_phone
seller_bank_account
total_amount_without_tax
total_tax_amount
total_amount
total_amount_cn
items_json
remarks
payee
reviewer
drawer
status
file_name
file_path
storage_key
file_hash
match_method
match_confidence
internal_notes
extension_data
```

`contract_invoices.contract_id` 当前为非空字段。因此发票匹配不到合同或置信度不足时，V1 不直接写 `contract_invoices`，而是把发票结果保存在 PDF 对应 `contract_main.extension_data.gemini_ocr.invoice_candidate`，并设置：

```text
custom_metrics.gemini_ocr.quality_status = "needs_review"
```

等匹配成功或人工确认后，再落入 `contract_invoices`。

### 8.4 统一 Gemini 输出结构

Gemini prompt 要求只返回合法 JSON：

```json
{
  "document_type": "contract",
  "file_summary": "常年法律顾问合同",
  "contract": {
    "contract_title": "常年法律顾问合同",
    "contract_number": null,
    "party_a": "南京优集数据科技有限公司",
    "party_a_credit_code": null,
    "party_b": "北京市中闻（南京）律师事务所",
    "party_b_credit_code": null,
    "sign_date": null,
    "start_date": "2025-12-19",
    "end_date": "2026-12-18",
    "total_contract_amount": 30000,
    "currency": "CNY",
    "payment_schedule": [
      {"amount": 15000, "due_date": null, "condition": "合同签署后伍日内"},
      {"amount": 15000, "due_date": "2026-06-30", "condition": "尾款"}
    ],
    "payment_terms": "分两期支付",
    "service_scope_summary": "法律顾问服务",
    "settlement_cycle": "年度",
    "settlement_unit_price": null,
    "price_unit": null,
    "payment_method": null,
    "tax_rate": null
  },
  "invoice": {
    "invoice_type": null,
    "invoice_number": null,
    "invoice_code": null,
    "issue_date": null,
    "seller_name": null,
    "buyer_name": null,
    "pre_tax_amount": null,
    "tax_amount": null,
    "total_amount": null,
    "currency": "CNY",
    "items": []
  },
  "ocr_text_excerpt": "关键原文片段",
  "confidence_notes": "签约日期为空白",
  "quality_flags": []
}
```

Prompt 规则：

1. 不要凭文件名猜测字段。
2. 无法确定填 `null`。
3. 金额必须区分合同总额、分期金额、发票不含税金额、税额和价税合计。
4. 日期统一 `YYYY-MM-DD`。
5. 输出关键原文片段，便于人工核验。

### 8.5 质量控制

质量控制不再泛泛检查“字段有没有”，而是贴合现有表字段。

#### 8.5.1 合同质量规则

合同高置信条件：

1. `contract_title`、`party_a`、`party_b` 至少两个非空，最好三者齐全。
2. `contract_amount` 非空时必须大于 0。
3. `start_date` 和 `end_date` 同时存在时，必须满足 `start_date <= end_date`。
4. `tax_rate` 统一存小数，例如 6% 存 `0.06`；超出 `0..1` 标记异常。
5. `amount_currency` 默认为 `CNY`。
6. `payment_schedule` 不直接落主表，但必须放入 `extension_data.gemini_ocr.payment_schedule`。
7. `ocr_text_excerpt` 必须非空，写入 `contract_pages` 作为证据。

质量状态建议：

```text
pass          -- 可直接入库
needs_review  -- 已识别但核心字段缺失或存在可疑点
failed        -- Gemini 调用失败、JSON 解析失败、文件不可读
```

合同质量 flag：

```text
missing_parties
missing_title
missing_amount
date_range_invalid
tax_rate_invalid
low_evidence
unknown_document_type
```

#### 8.5.2 发票质量规则

发票高置信条件：

1. `invoice_number` 非空。
2. `issue_date` 可解析。
3. `buyer_name` 和 `seller_name` 非空。
4. `total_amount` 大于 0。
5. 如果 `pre_tax_amount` 和 `tax_amount` 都存在：

```text
abs(pre_tax_amount + tax_amount - total_amount) <= 0.02
```

6. `items_json` 至少保留项目名、金额、税率、税额。
7. 发票重复判断优先使用：

```text
invoice_number + seller_name + buyer_name + total_amount
```

8. 未匹配到合同或置信度不足时，不写 `contract_invoices`，只保存候选并标记 `needs_review`。

发票质量 flag：

```text
missing_invoice_number
missing_issue_date
missing_buyer_or_seller
amount_sum_mismatch
missing_total_amount
duplicate_invoice_candidate
contract_match_low_confidence
contract_match_missing
```

### 8.6 发票到合同的匹配策略

V1 自动匹配只做保守规则：

1. 同飞书文件夹下，如果已有 active 合同 PDF，且发票买卖方与合同双方存在交集，作为高置信候选。
2. 发票总额不超过合同金额，或与合同分期金额相近，增加置信度。
3. 发票开票日期在合同有效期内，增加置信度。
4. 同一发票号已存在时，按重复候选处理，不直接插入。

匹配结果：

```text
high   -> 写 contract_invoices
medium -> 保存 invoice_candidate，needs_review
low    -> 保存 invoice_candidate，needs_review
none   -> 保存 invoice_candidate，needs_review
```

`match_method` 建议取值：

```text
folder_party_amount
folder_party
amount_date
manual
unmatched
```

### 8.7 状态与重试

继续复用 `contract_main.ocr_status`：

```text
pending -> running -> done
pending -> running -> failed
failed  -> pending  # 手动或定时重试
```

Worker claim 规则：

```sql
SELECT id, storage_key, file_hash
FROM tenant_uhub.contract_main
WHERE ocr_status = 'pending'
  AND (sync_status IS NULL OR sync_status = 'active')
  AND storage_key IS NOT NULL
ORDER BY last_seen_at ASC, id ASC
FOR UPDATE SKIP LOCKED
LIMIT $1;
```

失败时：

1. `ocr_status='failed'`。
2. `extension_data.gemini_ocr.error` 保存错误。
3. `custom_metrics.gemini_ocr.retryable` 标记是否可重试。
4. 不清空已有成功 OCR 结果，避免临时失败破坏历史数据。

### 8.8 OSS ODS 快照

飞书主动扫描可以把原始采集文件先落到 OSS，作为轻量 ODS 层：

1. PDF：扫描器仍先下载到本地 snapshot 计算 SHA256；hash 已存在时复用已有 `contract_main.storage_key`，不重复上传；hash 新增或同业务位置内容变化时，上传到现有历史合同前缀，默认 `tenant/uhub/contract`，也可通过 `feishu_sync_sources.metadata_json.oss_prefix` 指到 `tenant/uhub/contract/优集客户合同` 等子目录。
2. 财务表格：hash 未变时跳过上传和导入；hash 变化时，下载或导出的 `.xlsx` 快照上传到现有历史财务前缀，默认 `tenant/uhub/finance/{year}`，也可通过 `feishu_sync_sources.metadata_json.oss_prefix` 指到 `tenant/uhub/finance/2026`，并记录在 `feishu_sync_sources.metadata_json.storage_key`。
3. 未配置 OSS 时，`storage_key` 继续使用本地 snapshot 路径，方便本地开发和测试。
4. OCR worker 只依赖 `storage_key`；如果是 `s3://...`，先下载临时文件再调用 Gemini。

目录关系是当前合同/发票归属判断的重要线索，因此 object key 不再使用自创的技术目录；正式文件必须落在已有 `tenant/uhub/contract`、`tenant/uhub/finance` 历史前缀下，目录归属的精确信息由 `feishu_sync_sources.metadata_json.oss_prefix` 与数据库字段共同维护。

### 8.9 配置

新增环境变量：

```text
GEMINI_API_KEY
GOOGLE_GEMINI_BASE_URL=https://api.ikuncode.cc
GEMINI_MODEL=gemini-3-flash-preview
GEMINI_PROXY=
GEMINI_OCR_TIMEOUT_SECONDS=240
GEMINI_OCR_MAX_FILE_MB=50
OCR_WORKER_LIMIT=10
OCR_WORKER_CONCURRENCY=2
OCR_COST_USD_PER_M_INPUT_TOKENS=0.25
OCR_COST_USD_PER_M_OUTPUT_TOKENS=1.50
OSS_ACCESS_KEY_ID
OSS_ACCESS_KEY_SECRET
OSS_BUCKET=boss-agent
OSS_ENDPOINT=https://oss-cn-shenzhen.aliyuncs.com
OSS_CONTRACT_PREFIX=tenant/uhub/contract
OSS_FINANCE_PREFIX=tenant/uhub/finance
OSS_SMOKE_PREFIX=tmp/financeqa-smoke
```

API key 只放环境变量或服务端 secret，不写入数据库、日志或文档。

### 8.10 CLI

建议新增：

```text
financeqa ocr process-pending --db <dsn> --limit 10
financeqa ocr process-file --db <dsn> --file <pdf> --contract-id <id>
financeqa ocr retry-failed --db <dsn> --limit 10
```

V1 优先实现 `process-pending` 和 `process-file`。常驻服务、HTTP 上传接口、人工审核 UI 放到后续版本。

## 9. 飞书财务表格同步流程

### 9.1 webhook 处理

```text
收到飞书文件或表格变更事件
  -> 判断 token 是否是 finance_workbook
  -> 标记 feishu_sync_sources.sync_status = pending
  -> next_scan_at = now + 1-3 minutes
  -> 返回成功
```

### 9.2 worker 处理财务表格

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

### 9.3 导入方式

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

### 9.4 财务表格删除

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

## 10. 主动扫描策略

### 10.1 高频扫描

worker 定期处理：

```text
sync_status = pending
OR next_scan_at <= now()
```

### 10.2 兜底扫描

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

## 11. 与现有代码的关系

### 11.1 新增模块

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

internal/ocr
  gemini_client.go       -- Gemini API 客户端
  prompt.go              -- 合同/发票统一 JSON prompt
  types.go               -- OCR 结果结构
  quality.go             -- 合同/发票质量控制
  repository.go          -- pending claim 和写回 contract_* 表
  worker.go              -- process-pending 编排
```

### 11.2 CLI

建议新增命令：

```text
financeqa feishu webhook-server
financeqa feishu sync-once --source-token <token>
financeqa feishu scan
financeqa feishu sources
financeqa ocr process-pending
financeqa ocr process-file
```

首版也可以先不做常驻 server，只实现 `scan` 和 `sync-once` 验证链路，再接入服务端 webhook。

### 11.3 查询层

合同明细查询需要过滤软删除 PDF：

```text
contract_main.sync_status IS NULL OR contract_main.sync_status = 'active'
```

财务问答层无需感知飞书，只继续查询 `fin_*` 表。

## 12. 配置

建议环境变量：

```text
FEISHU_APP_ID
FEISHU_APP_SECRET
FEISHU_WEBHOOK_VERIFY_TOKEN
FEISHU_WEBHOOK_ENCRYPT_KEY
FINANCEQA_FEISHU_SCAN_INTERVAL_SECONDS
FINANCEQA_FEISHU_STABILITY_DELAY_SECONDS
FINANCEQA_FEISHU_SNAPSHOT_DIR
OSS_ACCESS_KEY_ID
OSS_ACCESS_KEY_SECRET
OSS_BUCKET
OSS_ENDPOINT
OSS_CONTRACT_PREFIX
OSS_FINANCE_PREFIX
OSS_SMOKE_PREFIX
GEMINI_API_KEY
GOOGLE_GEMINI_BASE_URL
GEMINI_MODEL
GEMINI_PROXY
GEMINI_OCR_TIMEOUT_SECONDS
OCR_WORKER_LIMIT
OCR_WORKER_CONCURRENCY
```

来源配置写入 `feishu_sync_sources`，不要只写死在代码中。

## 13. 错误处理

### 13.1 webhook 错误

webhook 只要验签和入队成功，就返回成功。下载、导出、OCR、导入失败不影响 webhook 返回。

### 13.2 下载或导出失败

记录：

```text
feishu_sync_sources.sync_status = error
error_message = ...
next_scan_at = now + backoff
```

### 13.3 OCR 失败

更新 `contract_main.ocr_status = failed`，保留文件 hash 和 storage key，允许后续重试。错误详情写入 `extension_data.gemini_ocr.error`，不要覆盖已有成功字段。

### 13.4 财务表格导入失败

不更新 `last_content_hash`，避免错误快照被视为已成功同步。保留失败快照路径和错误信息，方便排查。

## 14. 测试用例

### 14.1 PDF 同步

1. 新 PDF 上传：创建 `contract_main`，创建 OCR。
2. 同 hash 重复上传：不重复 OCR，写 `contract_duplicate_logs`。
3. 同文件夹同文件名不同 hash：重新 OCR，写替换日志。
4. 删除 PDF：软删除 `contract_main`，查询不返回该合同。
5. 删除后同名重传且 hash 相同：恢复 active，复用 OCR。
6. webhook 早于上传完成：pending 重试，稳定后才 OCR。

### 14.2 财务表格同步

1. 首次同步 xlsx：导入 `fin_contracts` 和 `fin_*`。
2. hash 未变：跳过导入。
3. 修改收入 sheet：重导 `fin_fund_income`。
4. 修改成本 sheet：重导 `fin_cost_settlements` 和 cost group/member。
5. 修改合并单元格：group/member 正确替换，不残留旧 group。
6. 飞书表格不可访问：标 error/missing，不清空 `fin_*`。

### 14.3 Gemini OCR

1. pending 合同 PDF：claim 后标 running，成功后标 done。
2. Gemini 返回合同：写 `contract_main` 合同字段、`contract_pages` 证据、`extension_data` 原始结果。
3. Gemini 返回发票且高置信匹配合同：写 `contract_invoices`。
4. Gemini 返回发票但匹配不到合同：不写 `contract_invoices`，保存 `invoice_candidate` 并标 `needs_review`。
5. 发票金额勾稽失败：保存结果但标 `amount_sum_mismatch`。
6. 合同日期范围非法：保存结果但标 `date_range_invalid`。
7. Gemini API 失败：标 failed，保存错误，可重试。
8. JSON 解析失败：标 failed，保存原始响应片段。
9. 同一发票重复：不重复插入，标 `duplicate_invoice_candidate`。

## 15. 参考资料

1. 飞书事件订阅概览：`https://open.feishu.cn/document/server-docs/event-subscription-guide/overview`
2. 飞书云文档下载：`https://open.feishu.cn/document/server-docs/docs/drive-v1/download/download`
3. 飞书云文档导出任务：`https://open.feishu.cn/document/server-docs/docs/drive-v1/export/export-task`
4. 飞书电子表格 API 概览：`https://open.feishu.cn/document/server-docs/docs/sheets-v3/overview`
5. Gemini PDF 处理：`https://ai.google.dev/gemini-api/docs/document-processing`
6. Gemini API 价格：`https://ai.google.dev/gemini-api/docs/pricing`

## 16. 待确认事项

1. 真实 `tenant_uhub.contract_main`、`contract_invoices`、`contract_pages`、`contract_duplicate_logs` 已有哪些字段；本设计中的字段应优先映射到现有字段。当前线上 DB 凭据认证失败，设计基于代码模型和迁移推断。
2. 财务表格 token `Iel5bFZWSoGF7hxjyPpcn5Elnqd` 的实际类型：Excel 文件、飞书在线表格或其他云文档。
3. `contract_main.job_id` 和 `category_id` 是否仍有 NOT NULL 约束；如果有，飞书 pending 记录必须写入占位 `job_id` 和基于文件夹的 `category_id`。
4. 发票无法匹配合同时，是否接受 V1 保存为 `contract_main.extension_data.invoice_candidate`，暂不写 `contract_invoices`。
5. webhook 服务部署位置，以及是否需要飞书事件加密解密。
6. 财务表格同步时的公司名覆盖策略，是否固定为 `FINANCEQA_DEFAULT_COMPANY`。
