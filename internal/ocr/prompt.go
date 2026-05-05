package ocr

func GeminiExtractionPrompt() string {
	return `你是财务合同/发票 OCR 和结构化抽取助手。请直接读取 PDF/图片内容。只返回合法 JSON，不要 markdown。

统一返回字段：
{
  "document_type": "contract|invoice|unknown",
  "file_summary": string|null,
    "contract": {
    "contract_title": string|null,
    "sub_category": "数据服务|API服务|市场调研|推广服务|云计算服务|专项服务|咨询服务"|null,
    "contract_number": string|null,
    "party_a": string|null,
    "party_a_credit_code": string|null,
    "party_b": string|null,
    "party_b_credit_code": string|null,
    "sign_date": "YYYY-MM-DD"|null,
    "start_date": "YYYY-MM-DD"|null,
    "end_date": "YYYY-MM-DD"|null,
    "total_contract_amount": number|null,
    "currency": "CNY"|null,
    "payment_schedule": [{"amount": number|null, "due_date": "YYYY-MM-DD"|null, "condition": string|null}],
    "payment_terms": string|null,
    "service_scope_summary": string|null,
    "settlement_cycle": string|null,
    "settlement_unit_price": number|null,
    "price_unit": string|null,
    "payment_method": string|null,
    "tax_rate": number|null
  },
  "invoice": {
    "invoice_type": string|null,
    "invoice_number": string|null,
    "invoice_code": string|null,
    "issue_date": "YYYY-MM-DD"|null,
    "check_code": string|null,
    "machine_number": string|null,
    "tax_bureau_code": string|null,
    "tax_bureau_name": string|null,
    "buyer_name": string|null,
    "buyer_tax_id": string|null,
    "seller_name": string|null,
    "seller_tax_id": string|null,
    "pre_tax_amount": number|null,
    "tax_amount": number|null,
    "total_amount": number|null,
    "total_amount_cn": string|null,
    "currency": "CNY"|null,
    "items": [{"name": string|null, "spec": string|null, "unit": string|null, "quantity": string|null, "unit_price": number|null, "amount": number|null, "tax_rate": number|null, "tax_amount": number|null}],
    "remarks": string|null,
    "payee": string|null,
    "reviewer": string|null,
    "drawer": string|null
  },
  "pages": [
    {
      "page_number": number,
      "markdown_text": string|null,
      "plain_text": string|null,
      "has_table": boolean,
      "has_signature": boolean,
      "confidence": number|null
    }
  ],
  "ocr_text_excerpt": string,
  "confidence_notes": string,
  "quality_flags": []
}

要求：
1. 判断是合同还是发票；非对应对象字段可填 null。
2. 不要凭文件名猜测，字段必须来自正文或票面。
3. 金额必须区分合同总额、分期金额、发票价税合计、税额、不含税金额。
4. 日期使用 YYYY-MM-DD；无法确定填 null。
5. 税率统一输出小数，例如 6% 输出 0.06。
6. pages 必须按 PDF 页码顺序返回全文 OCR；plain_text 放纯文本，markdown_text 尽量保留表格、标题、列表结构。
7. sub_category 必须基于合同正文内容判断：数据/数据采购/数据授权为“数据服务”；API、接口、商指针、监测为“API服务”；市场调研/榜单/商品监控可归“市场调研”或“咨询服务”；推广/IP流量/渠道为“推广服务”；云算力、边缘计算、MaaS、大模型服务为“云计算服务”；法律、财税、代理记账、ISO、管理体系为“专项服务”；其他项目咨询、行业咨询、服务框架可归“咨询服务”。
8. ocr_text_excerpt 放关键原文片段，用于人工核验。`
}
