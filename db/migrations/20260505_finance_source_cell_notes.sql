-- Preserve source workbook cell comments for imported contract finance rows.
--
-- Run with:
--   psql "$FINANCEQA_PG_DSN" -v ON_ERROR_STOP=1 -f db/migrations/20260505_finance_source_cell_notes.sql

BEGIN;

SET search_path TO tenant_uhub, public;

ALTER TABLE fin_fund_income
    ADD COLUMN IF NOT EXISTS source_cell_notes JSONB;

ALTER TABLE fin_cost_settlements
    ADD COLUMN IF NOT EXISTS source_cell_notes JSONB;

ALTER TABLE fin_fund_income_groups
    ADD COLUMN IF NOT EXISTS source_cell_notes JSONB;

ALTER TABLE fin_cost_settlement_groups
    ADD COLUMN IF NOT EXISTS source_cell_notes JSONB;

COMMENT ON COLUMN fin_fund_income.source_cell_notes IS '导入来源单元格备注 JSON，按 Excel 单元格坐标保存作者和备注文本';
COMMENT ON COLUMN fin_cost_settlements.source_cell_notes IS '导入来源单元格备注 JSON，按 Excel 单元格坐标保存作者和备注文本';
COMMENT ON COLUMN fin_fund_income_groups.source_cell_notes IS '导入来源单元格备注 JSON，按 Excel 单元格坐标保存作者和备注文本';
COMMENT ON COLUMN fin_cost_settlement_groups.source_cell_notes IS '导入来源单元格备注 JSON，按 Excel 单元格坐标保存作者和备注文本';

COMMIT;
