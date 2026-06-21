import importlib.util
import unittest
from pathlib import Path


SCRIPT_PATH = Path(__file__).resolve().parents[2] / "scripts" / "prepare_financeqa_shadow_migration.py"


def load_module():
    spec = importlib.util.spec_from_file_location("prepare_financeqa_shadow_migration", SCRIPT_PATH)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class PrepareFinanceQAShadowMigrationTest(unittest.TestCase):
    def test_fixed_table_list_excludes_financial_tables(self):
        module = load_module()

        self.assertEqual(len(module.MIGRATION_TABLES), 24)
        self.assertEqual(module.MIGRATION_TABLES, sorted(module.MIGRATION_TABLES))
        self.assertNotIn("financial_documents", module.MIGRATION_TABLES)
        self.assertNotIn("financial_links", module.MIGRATION_TABLES)
        self.assertNotIn("financial_rows", module.MIGRATION_TABLES)
        self.assertIn("fin_contracts", module.MIGRATION_TABLES)
        self.assertIn("contract_main", module.MIGRATION_TABLES)

    def test_shadow_env_merge_only_changes_db_keys(self):
        module = load_module()
        finance_env = {
            "PGHOST": "pgm-wz9evdimh3lx9b0kzo.pg.rds.aliyuncs.com",
            "PGPORT": "5432",
            "PGUSER": "bossagentdev",
            "PGPASSWORD": "old-secret",
            "PGDATABASE": "bossagent_app",
            "FINANCEQA_PG_SCHEMA": "tenant_uhub",
            "OSS_ENDPOINT": "https://oss-cn-shenzhen-internal.aliyuncs.com",
            "FINANCEQA_MCP_LISTEN": "127.0.0.1:3009",
        }
        shadow_env = {
            "DB_HOST": "pgm-wz9evdimh3lx9b0kzo.pg.rds.aliyuncs.com",
            "DB_PORT": "5432",
            "DB_USER": "bossagent_u_uhub_etl_shadow",
            "DB_PASSWORD": "new-secret",
            "DB_NAME": "bossagent_app",
            "DB_SEARCH_PATH": "tenant_uhub_etl_shadow,public",
        }

        merged, changed, forbidden = module.merge_financeqa_env(finance_env, shadow_env)

        self.assertEqual(forbidden, [])
        self.assertEqual(changed, ["FINANCEQA_PG_SCHEMA", "PGPASSWORD", "PGUSER"])
        self.assertEqual(merged["PGUSER"], "bossagent_u_uhub_etl_shadow")
        self.assertEqual(merged["FINANCEQA_PG_SCHEMA"], "tenant_uhub_etl_shadow")
        self.assertEqual(merged["OSS_ENDPOINT"], finance_env["OSS_ENDPOINT"])
        self.assertEqual(merged["FINANCEQA_MCP_LISTEN"], finance_env["FINANCEQA_MCP_LISTEN"])

    def test_rewrite_foreign_key_definition_targets_shadow_schema(self):
        module = load_module()

        rewritten = module.rewrite_fk_definition(
            "FOREIGN KEY (contract_id) REFERENCES tenant_uhub.contract_main(id) ON DELETE CASCADE",
            source_schema="tenant_uhub",
            target_schema="tenant_uhub_etl_shadow",
        )

        self.assertEqual(
            rewritten,
            "FOREIGN KEY (contract_id) REFERENCES tenant_uhub_etl_shadow.contract_main(id) ON DELETE CASCADE",
        )

        unqualified = module.rewrite_fk_definition(
            "FOREIGN KEY (contract_id) REFERENCES contract_main(id) ON DELETE CASCADE",
            source_schema="tenant_uhub",
            target_schema="tenant_uhub_etl_shadow",
        )

        self.assertEqual(
            unqualified,
            "FOREIGN KEY (contract_id) REFERENCES tenant_uhub_etl_shadow.contract_main(id) ON DELETE CASCADE",
        )

    def test_sequence_repair_sql_uses_target_sequence_and_sets_default(self):
        module = load_module()

        sql = module.sequence_repair_sql(
            target_schema="tenant_uhub_etl_shadow",
            table_name="contract_main",
            column_name="id",
        )

        joined = "\n".join(sql)
        self.assertIn('CREATE SEQUENCE IF NOT EXISTS "tenant_uhub_etl_shadow"."contract_main_id_seq"', joined)
        self.assertIn(
            'ALTER TABLE "tenant_uhub_etl_shadow"."contract_main" ALTER COLUMN "id" SET DEFAULT nextval',
            joined,
        )
        self.assertIn('setval(\'"tenant_uhub_etl_shadow"."contract_main_id_seq"\'::regclass', joined)
        self.assertNotIn("tenant_uhub.contract_main_id_seq", joined)

    def test_apply_requires_explicit_confirmation(self):
        module = load_module()

        with self.assertRaises(SystemExit):
            module.validate_write_intent(apply=True, yes=False)

        module.validate_write_intent(apply=True, yes=True)
        module.validate_write_intent(apply=False, yes=False)


if __name__ == "__main__":
    unittest.main()
