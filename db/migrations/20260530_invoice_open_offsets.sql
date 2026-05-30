-- Preserve conservative note-derived offsets for invoice-open calculations.
-- Run with:
--   psql "$FINANCEQA_PG_DSN" -v ON_ERROR_STOP=1 -f db/migrations/20260530_invoice_open_offsets.sql

ALTER TABLE IF EXISTS fin_fund_income
    ADD COLUMN IF NOT EXISTS invoice_open_offset_amount NUMERIC(18,2),
    ADD COLUMN IF NOT EXISTS invoice_open_offset_reason TEXT;

ALTER TABLE IF EXISTS fin_fund_income_groups
    ADD COLUMN IF NOT EXISTS invoice_open_offset_amount NUMERIC(18,2),
    ADD COLUMN IF NOT EXISTS invoice_open_offset_reason TEXT;

ALTER TABLE IF EXISTS fin_cost_settlements
    ADD COLUMN IF NOT EXISTS invoice_open_offset_amount NUMERIC(18,2),
    ADD COLUMN IF NOT EXISTS invoice_open_offset_reason TEXT;

ALTER TABLE IF EXISTS fin_cost_settlement_groups
    ADD COLUMN IF NOT EXISTS invoice_open_offset_amount NUMERIC(18,2),
    ADD COLUMN IF NOT EXISTS invoice_open_offset_reason TEXT;

COMMENT ON COLUMN fin_fund_income.invoice_open_offset_amount IS '用于抵扣已开票未回款的备注调整金额，不改变实际回款金额';
COMMENT ON COLUMN fin_fund_income.invoice_open_offset_reason IS '备注调整金额的来源说明';
COMMENT ON COLUMN fin_fund_income_groups.invoice_open_offset_amount IS '用于抵扣已开票未回款的备注调整金额，不改变实际回款金额';
COMMENT ON COLUMN fin_fund_income_groups.invoice_open_offset_reason IS '备注调整金额的来源说明';
COMMENT ON COLUMN fin_cost_settlements.invoice_open_offset_amount IS '用于抵扣已收票未付款的备注调整金额，不改变实际付款金额';
COMMENT ON COLUMN fin_cost_settlements.invoice_open_offset_reason IS '备注调整金额的来源说明';
COMMENT ON COLUMN fin_cost_settlement_groups.invoice_open_offset_amount IS '用于抵扣已收票未付款的备注调整金额，不改变实际付款金额';
COMMENT ON COLUMN fin_cost_settlement_groups.invoice_open_offset_reason IS '备注调整金额的来源说明';
