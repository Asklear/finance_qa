package db

// TypeScriptCompatibleSchema mirrors src/database.ts SCHEMA for SQLite bootstrap.
const TypeScriptCompatibleSchema = `
CREATE TABLE IF NOT EXISTS uploaded_files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    filename TEXT NOT NULL,
    file_path TEXT,
    file_type TEXT,
    file_size INTEGER,
    company TEXT,
    period TEXT,
    parse_status TEXT DEFAULT 'pending',
    parse_result TEXT,
    record_count INTEGER,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS file_registry (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    filename TEXT NOT NULL,
    file_path TEXT,
    file_md5 TEXT UNIQUE,
    file_size INTEGER,
    modified_time TIMESTAMP,
    company TEXT,
    report_type TEXT,
    period_start TEXT,
    period_end TEXT,
    version INTEGER DEFAULT 1,
    is_latest BOOLEAN DEFAULT 1,
    parse_status TEXT,
    parse_message TEXT,
    imported_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS entities (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    entity_type TEXT NOT NULL,
    name TEXT NOT NULL,
    aliases TEXT,
    category TEXT,
    metadata TEXT,
    source_file TEXT,
    first_seen TEXT,
    last_updated TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    usage_count INTEGER DEFAULT 0,
    UNIQUE(entity_type, name)
);
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
    imported_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS income_statement (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    company TEXT,
    period TEXT,
    item_name TEXT,
    current_amount DECIMAL(18,2),
    cumulative_amount DECIMAL(18,2),
    file_version INTEGER DEFAULT 1,
    imported_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
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
CREATE TABLE IF NOT EXISTS budget (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    company TEXT,
    year INTEGER,
    month INTEGER,
    period TEXT,
    account_code TEXT,
    account_name TEXT,
    account_level INTEGER DEFAULT 1,
    budget_amount DECIMAL(18,2),
    version INTEGER DEFAULT 1,
    imported_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(company, year, month, account_code, version)
);
CREATE INDEX IF NOT EXISTS idx_entities_type ON entities(entity_type);
CREATE INDEX IF NOT EXISTS idx_entities_name ON entities(name);
CREATE INDEX IF NOT EXISTS idx_balance_sheet_period ON balance_sheet(company, period);
CREATE INDEX IF NOT EXISTS idx_balance_sheet_account ON balance_sheet(company, period, account_name);
CREATE INDEX IF NOT EXISTS idx_income_statement_period ON income_statement(company, period);
CREATE INDEX IF NOT EXISTS idx_balance_detail_period ON balance_detail(company, period);
CREATE INDEX IF NOT EXISTS idx_journal_date ON journal(company, voucher_date);
CREATE INDEX IF NOT EXISTS idx_bank_statement_date ON bank_statement(company, transaction_date);
CREATE INDEX IF NOT EXISTS idx_budget_period ON budget(company, year, month);
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
CREATE TABLE IF NOT EXISTS dimension_mappings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    company TEXT NOT NULL,
    period TEXT NOT NULL,
    source_type TEXT NOT NULL,       
    source_id INTEGER,               
    dimension_code TEXT NOT NULL,
    member_code TEXT NOT NULL,
    allocation_ratio DECIMAL(5,4) DEFAULT 1.0,  
    allocated_amount DECIMAL(18,2),             
    journal_id INTEGER,              
    rule_id INTEGER,                 
    confidence DECIMAL(3,2),         
    is_manual BOOLEAN DEFAULT 0,     
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS fact_financials (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    company TEXT NOT NULL,
    period TEXT NOT NULL,
    customer_code TEXT,
    project_code TEXT,
    product_code TEXT,
    channel_code TEXT,
    department_code TEXT,
    region_code TEXT,
    metric_type TEXT NOT NULL,       
    amount_accounting DECIMAL(18,2), 
    amount_cash DECIMAL(18,2),       
    source_count INTEGER,            
    last_updated TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(company, period, customer_code, project_code, product_code,
           channel_code, department_code, region_code, metric_type)
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
CREATE TABLE IF NOT EXISTS allocation_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    company TEXT NOT NULL,
    rule_name TEXT NOT NULL,         
    expense_account_codes TEXT,      
    expense_pattern TEXT,            
    allocation_caliber TEXT NOT NULL, 
    target_dimension TEXT NOT NULL,  
    base_period_type TEXT DEFAULT 'current',  
    manual_ratios TEXT,
    exclude_members TEXT,            
    valid_from TEXT,
    valid_to TEXT,
    is_default BOOLEAN DEFAULT 0,    
    is_active BOOLEAN DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS allocation_executions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    execution_id TEXT UNIQUE NOT NULL,
    company TEXT NOT NULL,
    period TEXT NOT NULL,
    expense_type TEXT NOT NULL,
    caliber TEXT NOT NULL,
    original_amount DECIMAL(18,2),
    total_allocated DECIMAL(18,2),
    variance DECIMAL(18,2),
    execution_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    key_insight TEXT,
    details_json TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_dimensions_code ON dimensions(code);
CREATE INDEX IF NOT EXISTS idx_dimension_members_lookup ON dimension_members(dimension_id, code);
CREATE INDEX IF NOT EXISTS idx_dimension_members_parent ON dimension_members(dimension_id, parent_id);
CREATE INDEX IF NOT EXISTS idx_dim_mapping_lookup ON dimension_mappings(company, period, dimension_code, member_code);
CREATE INDEX IF NOT EXISTS idx_dim_mapping_source ON dimension_mappings(source_type, source_id);
CREATE INDEX IF NOT EXISTS idx_dim_mapping_journal ON dimension_mappings(journal_id);
CREATE INDEX IF NOT EXISTS idx_fact_financials_lookup ON fact_financials(company, period, metric_type);
CREATE INDEX IF NOT EXISTS idx_fact_financials_product ON fact_financials(company, period, product_code);
CREATE INDEX IF NOT EXISTS idx_fact_financials_project ON fact_financials(company, period, project_code);
CREATE INDEX IF NOT EXISTS idx_fact_financials_channel ON fact_financials(company, period, channel_code);
CREATE INDEX IF NOT EXISTS idx_fact_financials_department ON fact_financials(company, period, department_code);
CREATE INDEX IF NOT EXISTS idx_mapping_rules_company ON mapping_rules(company, is_active);
CREATE INDEX IF NOT EXISTS idx_mapping_rules_priority ON mapping_rules(company, priority);
CREATE INDEX IF NOT EXISTS idx_allocation_rules_company ON allocation_rules(company, is_active);
CREATE INDEX IF NOT EXISTS idx_allocation_executions_lookup ON allocation_executions(company, period);
CREATE TABLE IF NOT EXISTS smart_mapping_learnings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    journal_id INTEGER NOT NULL,
    original_summary TEXT,
    extracted_keywords TEXT,  
    suggested_member_code TEXT,
    adjusted_member_code TEXT NOT NULL,
    adjustment_type TEXT CHECK(adjustment_type IN ('accept', 'reject', 'modify')),
    confidence_delta DECIMAL(3,2),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_smart_learnings_journal ON smart_mapping_learnings(journal_id);
CREATE INDEX IF NOT EXISTS idx_smart_learnings_adjusted ON smart_mapping_learnings(adjusted_member_code);
CREATE INDEX IF NOT EXISTS idx_smart_learnings_keywords ON smart_mapping_learnings(extracted_keywords);
`
