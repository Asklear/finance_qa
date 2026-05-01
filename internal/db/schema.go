package db

// TypeScriptCompatibleSchema mirrors src/database.ts SCHEMA for SQLite bootstrap.
const TypeScriptCompatibleSchema = `
CREATE TABLE IF NOT EXISTS balance_sheet (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    company TEXT,
    period TEXT,
    account_code TEXT,
    account_name TEXT,
    account_level INTEGER DEFAULT 1,
    opening_balance DECIMAL(18,2),
    closing_balance DECIMAL(18,2),
    file_version INTEGER DEFAULT 1,
    imported_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(company, period, account_name)
);
CREATE TABLE IF NOT EXISTS income_statement (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    company TEXT,
    period TEXT,
    item_name TEXT,
    current_amount DECIMAL(18,2),
    cumulative_amount DECIMAL(18,2),
    file_version INTEGER DEFAULT 1,
    imported_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(company, period, item_name)
);
CREATE TABLE IF NOT EXISTS balance_detail (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    company TEXT,
    year INTEGER,
    period TEXT,
    opening_period TEXT,
    account_code TEXT,
    account_name TEXT,
    account_level INTEGER DEFAULT 1,
    opening_debit DECIMAL(18,2),
    opening_credit DECIMAL(18,2),
    current_debit DECIMAL(18,2),
    current_credit DECIMAL(18,2),
    closing_debit DECIMAL(18,2),
    closing_credit DECIMAL(18,2),
    file_version INTEGER DEFAULT 1,
    imported_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS journal (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    company TEXT,
    period TEXT,
    voucher_date DATE,
    voucher_no TEXT,
    account_code TEXT,
    account_name TEXT,
    summary TEXT,
    direction TEXT,
    debit_amount DECIMAL(18,2),
    credit_amount DECIMAL(18,2),
    amount DECIMAL(18,2),
    counterparty TEXT,
    file_version INTEGER DEFAULT 1,
    imported_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS bank_statement (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    company TEXT,
    account_no TEXT,
    account_name TEXT,
    currency TEXT,
    transaction_date DATE,
    transaction_time TIME,
    transaction_type TEXT,
    debit_amount DECIMAL(18,2),
    credit_amount DECIMAL(18,2),
    balance DECIMAL(18,2),
    summary TEXT,
    counterparty_name TEXT,
    counterparty_account TEXT,
    file_version INTEGER DEFAULT 1,
    imported_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS fin_contracts (
    contract_id TEXT PRIMARY KEY,
    customer_name TEXT NOT NULL,
    contract_content TEXT,
    contract_start_date TEXT,
    contract_end_date TEXT,
    settlement_cycle TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS fin_revenue_settlements (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    contract_id TEXT NOT NULL,
    year_month TEXT NOT NULL,
    quantity DECIMAL(18,2),
    settlement_amount DECIMAL(18,2) NOT NULL,
    is_invoiced TEXT,
    invoice_amount DECIMAL(18,2),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS fin_cost_settlements (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    contract_id TEXT NOT NULL,
    year_month TEXT NOT NULL,
    source_report_type TEXT,
    source_sheet_name TEXT,
    quantity TEXT,
    settlement_amount DECIMAL(18,2) NOT NULL,
    is_invoiced TEXT,
    invoice_amount DECIMAL(18,2),
    paid_amount DECIMAL(18,2),
    account_code TEXT,
    contract_start_date TEXT,
    contract_end_date TEXT,
    settlement_cycle TEXT,
    settlement_unit_price TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS fin_cost_settlement_groups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    customer_name TEXT NOT NULL,
    year_month TEXT NOT NULL,
    source_report_type TEXT,
    source_sheet_name TEXT,
    source_start_row INTEGER,
    source_end_row INTEGER,
    merge_range TEXT,
    quantity TEXT,
    settlement_amount DECIMAL(18,2),
    is_invoiced TEXT,
    invoice_amount DECIMAL(18,2),
    paid_amount DECIMAL(18,2),
    account_code TEXT,
    contract_start_date TEXT,
    contract_end_date TEXT,
    settlement_cycle TEXT,
    settlement_unit_price TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS fin_cost_settlement_group_members (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id INTEGER NOT NULL,
    contract_id TEXT NOT NULL,
    source_row_number INTEGER,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS fin_fund_income (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    contract_id TEXT NOT NULL,
    year_month TEXT NOT NULL,
    source_report_type TEXT,
    source_sheet_name TEXT,
    quantity TEXT,
    settlement_amount DECIMAL(18,2),
    received_amount DECIMAL(18,2),
    is_invoiced TEXT,
    invoice_amount DECIMAL(18,2),
    contract_start_date TEXT,
    contract_end_date TEXT,
    settlement_cycle TEXT,
    settlement_unit_price TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS fin_fund_income_groups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    customer_name TEXT NOT NULL,
    year_month TEXT NOT NULL,
    source_report_type TEXT,
    source_sheet_name TEXT,
    source_start_row INTEGER,
    source_end_row INTEGER,
    merge_range TEXT,
    quantity TEXT,
    settlement_amount DECIMAL(18,2),
    received_amount DECIMAL(18,2),
    is_invoiced TEXT,
    invoice_amount DECIMAL(18,2),
    contract_start_date TEXT,
    contract_end_date TEXT,
    settlement_cycle TEXT,
    settlement_unit_price TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS fin_fund_income_group_members (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id INTEGER NOT NULL,
    contract_id TEXT NOT NULL,
    source_row_number INTEGER,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS table_idempotency_policies (
    table_name TEXT PRIMARY KEY,
    update_mode TEXT NOT NULL CHECK (update_mode IN ('full_replace', 'incremental_latest')),
    dedupe_key_columns TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS feishu_sync_sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_type TEXT NOT NULL,
    source_token TEXT NOT NULL,
    source_url TEXT,
    display_name TEXT,
    parent_token TEXT,
    sync_mode TEXT NOT NULL DEFAULT 'active_scan',
    sync_status TEXT NOT NULL DEFAULT 'active',
    last_revision TEXT,
    last_content_hash TEXT,
    last_event_at TIMESTAMP,
    next_scan_at TIMESTAMP,
    last_sync_at TIMESTAMP,
    last_success_at TIMESTAMP,
    error_message TEXT,
    metadata_json TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(source_type, source_token)
);
CREATE TABLE IF NOT EXISTS meta_table_comments (
    table_name TEXT PRIMARY KEY,
    comment TEXT,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS meta_column_comments (
    table_name TEXT NOT NULL,
    column_name TEXT NOT NULL,
    comment TEXT,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (table_name, column_name)
);
CREATE INDEX IF NOT EXISTS idx_balance_sheet_period ON balance_sheet(company, period);
CREATE INDEX IF NOT EXISTS idx_balance_sheet_account ON balance_sheet(company, period, account_name);
CREATE INDEX IF NOT EXISTS idx_income_statement_period ON income_statement(company, period);
CREATE INDEX IF NOT EXISTS idx_balance_detail_period ON balance_detail(company, period);
CREATE INDEX IF NOT EXISTS idx_journal_date ON journal(company, voucher_date);
CREATE INDEX IF NOT EXISTS idx_bank_statement_date ON bank_statement(company, transaction_date);
CREATE INDEX IF NOT EXISTS idx_bank_statement_company_date_credit ON bank_statement(company, transaction_date, credit_amount);
CREATE INDEX IF NOT EXISTS idx_fin_contracts_name ON fin_contracts(customer_name, contract_content);
CREATE INDEX IF NOT EXISTS idx_fin_revenue_settlements_contract_period ON fin_revenue_settlements(contract_id, year_month);
CREATE INDEX IF NOT EXISTS idx_fin_cost_settlements_contract_period ON fin_cost_settlements(contract_id, year_month);
CREATE INDEX IF NOT EXISTS idx_fin_cost_settlement_groups_period ON fin_cost_settlement_groups(customer_name, year_month);
CREATE INDEX IF NOT EXISTS idx_fin_cost_settlement_group_members_group ON fin_cost_settlement_group_members(group_id);
CREATE INDEX IF NOT EXISTS idx_fin_cost_settlement_group_members_contract ON fin_cost_settlement_group_members(contract_id);
CREATE INDEX IF NOT EXISTS idx_fin_fund_income_contract_period ON fin_fund_income(contract_id, year_month);
CREATE INDEX IF NOT EXISTS idx_fin_fund_income_groups_period ON fin_fund_income_groups(customer_name, year_month);
CREATE INDEX IF NOT EXISTS idx_fin_fund_income_group_members_group ON fin_fund_income_group_members(group_id);
CREATE INDEX IF NOT EXISTS idx_fin_fund_income_group_members_contract ON fin_fund_income_group_members(contract_id);
CREATE INDEX IF NOT EXISTS idx_table_idempotency_enabled ON table_idempotency_policies(enabled);
CREATE INDEX IF NOT EXISTS idx_feishu_sync_sources_status ON feishu_sync_sources(sync_status, next_scan_at);
CREATE TABLE IF NOT EXISTS dimensions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    code TEXT UNIQUE NOT NULL,       
    name TEXT NOT NULL,              
    type TEXT,                       
    description TEXT,
    is_hierarchical BOOLEAN DEFAULT 0,
    is_active BOOLEAN DEFAULT 1,
    metadata TEXT,                   
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS dimension_members (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    dimension_id INTEGER NOT NULL,   
    code TEXT NOT NULL,              
    name TEXT NOT NULL,              
    parent_id INTEGER,               
    level INTEGER DEFAULT 0,         
    path TEXT,                       
    is_active BOOLEAN DEFAULT 1,
    sort_order INTEGER DEFAULT 0,
    metadata TEXT,                   
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(dimension_id, code)
);
CREATE TABLE IF NOT EXISTS mapping_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    company TEXT NOT NULL,
    rule_name TEXT NOT NULL,
    priority INTEGER DEFAULT 100,    
    account_code_pattern TEXT,       
    account_name_pattern TEXT,       
    summary_pattern TEXT,            
    counterparty_pattern TEXT,       
    dimension_code TEXT NOT NULL,
    member_code TEXT NOT NULL,
    allocation_ratio DECIMAL(5,4) DEFAULT 1.0,
    valid_from TEXT,
    valid_to TEXT,
    is_active BOOLEAN DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_dimensions_code ON dimensions(code);
CREATE INDEX IF NOT EXISTS idx_dimension_members_lookup ON dimension_members(dimension_id, code);
CREATE INDEX IF NOT EXISTS idx_dimension_members_parent ON dimension_members(dimension_id, parent_id);
CREATE INDEX IF NOT EXISTS idx_mapping_rules_company ON mapping_rules(company, is_active);
CREATE INDEX IF NOT EXISTS idx_mapping_rules_priority ON mapping_rules(company, priority);
INSERT OR IGNORE INTO table_idempotency_policies(table_name, update_mode, dedupe_key_columns, enabled)
VALUES
('balance_sheet', 'full_replace', 'company,period,account_name', 1),
('income_statement', 'full_replace', 'company,period,item_name', 1),
('balance_detail', 'incremental_latest', 'company,period,account_code', 1),
('journal', 'incremental_latest', 'company,voucher_date,voucher_no,account_code,summary,debit_amount,credit_amount', 1),
('bank_statement', 'incremental_latest', 'company,account_no,account_name,currency,transaction_date,transaction_time,transaction_type,debit_amount,credit_amount,balance,summary,counterparty_name,counterparty_account', 1);
UPDATE table_idempotency_policies
SET update_mode = 'incremental_latest',
    dedupe_key_columns = 'company,period,account_code'
WHERE table_name = 'balance_detail';
`
