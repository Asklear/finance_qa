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
CREATE INDEX IF NOT EXISTS idx_balance_sheet_period ON balance_sheet(company, period);
CREATE INDEX IF NOT EXISTS idx_balance_sheet_account ON balance_sheet(company, period, account_name);
CREATE INDEX IF NOT EXISTS idx_income_statement_period ON income_statement(company, period);
CREATE INDEX IF NOT EXISTS idx_balance_detail_period ON balance_detail(company, period);
CREATE INDEX IF NOT EXISTS idx_journal_date ON journal(company, voucher_date);
CREATE INDEX IF NOT EXISTS idx_bank_statement_date ON bank_statement(company, transaction_date);
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
`
