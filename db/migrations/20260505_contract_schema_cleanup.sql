-- Contract OCR schema cleanup.
-- Run with psql after fixing database credentials:
--   psql "$FINANCEQA_PG_DSN" -v ON_ERROR_STOP=1 -f db/migrations/20260505_contract_schema_cleanup.sql
--
-- The application no longer reads contract_main.document_kind or
-- contract_invoice_summaries. Invoices are stored in contract_invoices.

BEGIN;

SET search_path TO tenant_uhub, public;

CREATE TABLE IF NOT EXISTS contract_schema_cleanup_archive_20260505 (
    id BIGSERIAL PRIMARY KEY,
    archived_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    source_table TEXT NOT NULL,
    source_pk TEXT,
    payload JSONB NOT NULL
);

INSERT INTO contract_schema_cleanup_archive_20260505(source_table, source_pk, payload)
SELECT 'contract_main', id::TEXT, to_jsonb(contract_main)
FROM contract_main;

INSERT INTO contract_schema_cleanup_archive_20260505(source_table, source_pk, payload)
SELECT 'contract_invoices', id::TEXT, to_jsonb(contract_invoices)
FROM contract_invoices;

DO $$
DECLARE
    cleanup_schema TEXT := current_schema();
    table_name TEXT;
BEGIN
    FOREACH table_name IN ARRAY ARRAY[
        'contract_invoice_summaries',
        'contract_images',
        'contract_customer_extensions',
        'contract_service_extensions',
        'contract_supplier_extensions',
        'contract_invoices_backup_20260505_115641',
        'contract_main_invoice_backup_20260505_115641'
    ]
    LOOP
        IF to_regclass(format('%I.%I', cleanup_schema, table_name)) IS NOT NULL THEN
            EXECUTE format(
                'INSERT INTO %I.contract_schema_cleanup_archive_20260505(source_table, source_pk, payload)
                 SELECT %L, NULL, to_jsonb(t) FROM %I.%I AS t',
                cleanup_schema,
                table_name,
                cleanup_schema,
                table_name
            );
        END IF;
    END LOOP;
END $$;

DROP INDEX IF EXISTS idx_contract_main_feishu_relation;
CREATE INDEX IF NOT EXISTS idx_contract_main_feishu_relation
    ON contract_main(feishu_root_token, feishu_relation_key);

ALTER SEQUENCE IF EXISTS contract_invoice_summaries_id_seq OWNED BY NONE;
ALTER SEQUENCE IF EXISTS contract_images_id_seq OWNED BY NONE;
ALTER SEQUENCE IF EXISTS contract_customer_extensions_id_seq OWNED BY NONE;
ALTER SEQUENCE IF EXISTS contract_service_extensions_id_seq OWNED BY NONE;
ALTER SEQUENCE IF EXISTS contract_supplier_extensions_id_seq OWNED BY NONE;

ALTER TABLE IF EXISTS contract_invoice_summaries ALTER COLUMN id DROP DEFAULT;
ALTER TABLE IF EXISTS contract_images ALTER COLUMN id DROP DEFAULT;
ALTER TABLE IF EXISTS contract_customer_extensions ALTER COLUMN id DROP DEFAULT;
ALTER TABLE IF EXISTS contract_service_extensions ALTER COLUMN id DROP DEFAULT;
ALTER TABLE IF EXISTS contract_supplier_extensions ALTER COLUMN id DROP DEFAULT;

ALTER TABLE contract_main
    DROP COLUMN IF EXISTS linked_contract_main_id,
    DROP COLUMN IF EXISTS document_kind,
    DROP COLUMN IF EXISTS relative_path,
    DROP COLUMN IF EXISTS jsonl_path,
    DROP COLUMN IF EXISTS file_modified_at,
    DROP COLUMN IF EXISTS file_version,
    DROP COLUMN IF EXISTS tags,
    DROP COLUMN IF EXISTS remarks,
    DROP COLUMN IF EXISTS feishu_modified_time;

ALTER TABLE contract_invoices
    DROP COLUMN IF EXISTS file_path,
    DROP COLUMN IF EXISTS internal_notes,
    DROP COLUMN IF EXISTS payment_batch;

DROP TABLE IF EXISTS contract_invoice_summaries;
DROP TABLE IF EXISTS contract_images;
DROP TABLE IF EXISTS contract_customer_extensions;
DROP TABLE IF EXISTS contract_service_extensions;
DROP TABLE IF EXISTS contract_supplier_extensions;
DROP TABLE IF EXISTS contract_invoices_backup_20260505_115641;
DROP TABLE IF EXISTS contract_main_invoice_backup_20260505_115641;

COMMENT ON COLUMN contract_main.sub_category IS '合同子分类；由合同内容识别得出，用于区分不同合同类型';

COMMIT;

-- Post-checks:
-- SELECT table_name FROM information_schema.tables
-- WHERE table_schema = 'tenant_uhub'
--   AND table_name IN (
--     'contract_invoice_summaries',
--     'contract_images',
--     'contract_customer_extensions',
--     'contract_service_extensions',
--     'contract_supplier_extensions',
--     'contract_invoices_backup_20260505_115641',
--     'contract_main_invoice_backup_20260505_115641'
--   );
--
-- SELECT column_name FROM information_schema.columns
-- WHERE table_schema = 'tenant_uhub'
--   AND table_name = 'contract_main'
--   AND column_name IN (
--     'linked_contract_main_id',
--     'document_kind',
--     'relative_path',
--     'jsonl_path',
--     'file_modified_at',
--     'file_version',
--     'tags',
--     'remarks',
--     'feishu_modified_time'
--   );
--
-- SELECT column_name FROM information_schema.columns
-- WHERE table_schema = 'tenant_uhub'
--   AND table_name = 'contract_invoices'
--   AND column_name IN ('file_path', 'internal_notes', 'payment_batch');
