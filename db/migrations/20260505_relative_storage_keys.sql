-- Store OSS object keys as relative paths instead of s3://bucket/key.
--
-- Run with:
--   psql "$FINANCEQA_PG_DSN" -v ON_ERROR_STOP=1 -f db/migrations/20260505_relative_storage_keys.sql

BEGIN;

SET search_path TO tenant_uhub, public;

UPDATE contract_main
SET storage_key = regexp_replace(storage_key, '^s3://[^/]+/', '')
WHERE storage_key LIKE 's3://%';

UPDATE contract_invoices
SET storage_key = regexp_replace(storage_key, '^s3://[^/]+/', '')
WHERE storage_key LIKE 's3://%';

UPDATE feishu_sync_sources
SET metadata_json = jsonb_set(
    metadata_json,
    '{storage_key}',
    to_jsonb(regexp_replace(metadata_json->>'storage_key', '^s3://[^/]+/', '')),
    true
)::jsonb
WHERE metadata_json IS NOT NULL
  AND metadata_json ? 'storage_key'
  AND metadata_json->>'storage_key' LIKE 's3://%';

COMMENT ON COLUMN contract_main.storage_key IS '对象存储相对路径，正式环境为 OSS object key，例如 tenant/uhub/contract/xxx.pdf';
COMMENT ON COLUMN contract_invoices.storage_key IS '对象存储相对路径，正式环境为 OSS object key，例如 tenant/uhub/contract/xxx.pdf';

COMMIT;

-- Post-checks:
-- SELECT count(*) FROM contract_main WHERE storage_key LIKE 's3://%';
-- SELECT count(*) FROM contract_invoices WHERE storage_key LIKE 's3://%';
-- SELECT count(*) FROM feishu_sync_sources
-- WHERE metadata_json->>'storage_key' LIKE 's3://%';
