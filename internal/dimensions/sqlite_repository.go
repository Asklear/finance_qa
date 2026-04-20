package dimensions

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(db *sql.DB) *SQLiteRepository {
	return &SQLiteRepository{db: db}
}

func (r *SQLiteRepository) CreateDimension(ctx context.Context, dim Dimension) (Dimension, error) {
	now := time.Now().UTC()
	metadata, err := marshalMap(dim.Metadata)
	if err != nil {
		return Dimension{}, err
	}
	row := r.db.QueryRowContext(ctx, `
INSERT INTO dimensions (code, name, type, description, is_hierarchical, is_active, metadata, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id, code, name, type, description, is_hierarchical, is_active, metadata, created_at, updated_at
`, dim.Code, dim.Name, string(dim.Type), dim.Description, dim.IsHierarchical, dim.IsActive, metadata, now, now)
	item, err := scanDimension(row)
	if err != nil {
		return Dimension{}, mapSQLErr(err)
	}
	return item, nil
}

func (r *SQLiteRepository) GetDimensionByID(ctx context.Context, id int64) (Dimension, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT id, code, name, type, description, is_hierarchical, is_active, metadata, created_at, updated_at
FROM dimensions WHERE id = ?
`, id)
	return scanDimension(row)
}

func (r *SQLiteRepository) GetDimensionByCode(ctx context.Context, code string) (Dimension, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT id, code, name, type, description, is_hierarchical, is_active, metadata, created_at, updated_at
FROM dimensions WHERE lower(code) = lower(?)
`, code)
	return scanDimension(row)
}

func (r *SQLiteRepository) UpdateDimension(ctx context.Context, id int64, patch DimensionPatch) (Dimension, error) {
	current, err := r.GetDimensionByID(ctx, id)
	if err != nil {
		return Dimension{}, err
	}
	if patch.Name != nil {
		current.Name = *patch.Name
	}
	if patch.Description != nil {
		current.Description = patch.Description
	}
	if patch.Type != nil {
		current.Type = *patch.Type
	}
	if patch.IsActive != nil {
		current.IsActive = *patch.IsActive
	}
	if patch.IsHierarchical != nil {
		current.IsHierarchical = *patch.IsHierarchical
	}
	if patch.MetadataSet {
		current.Metadata = cloneMap(patch.Metadata)
	}
	metadata, err := marshalMap(current.Metadata)
	if err != nil {
		return Dimension{}, err
	}
	now := time.Now().UTC()
	_, err = r.db.ExecContext(ctx, `
UPDATE dimensions
SET name = ?, type = ?, description = ?, is_hierarchical = ?, is_active = ?, metadata = ?, updated_at = ?
WHERE id = ?
`, current.Name, string(current.Type), current.Description, current.IsHierarchical, current.IsActive, metadata, now, id)
	if err != nil {
		return Dimension{}, mapSQLErr(err)
	}
	return r.GetDimensionByID(ctx, id)
}

func (r *SQLiteRepository) DeleteDimension(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM dimensions WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *SQLiteRepository) ListDimensions(ctx context.Context, opts DimensionQueryOptions) ([]Dimension, int, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, code, name, type, description, is_hierarchical, is_active, metadata, created_at, updated_at
FROM dimensions
ORDER BY id
`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []Dimension
	for rows.Next() {
		item, err := scanDimension(rows)
		if err != nil {
			return nil, 0, err
		}
		if opts.Type != nil && item.Type != *opts.Type {
			continue
		}
		if opts.IsActive != nil && item.IsActive != *opts.IsActive {
			continue
		}
		if opts.Keyword != "" && !containsFold(item.Code, opts.Keyword) && !containsFold(item.Name, opts.Keyword) {
			continue
		}
		items = append(items, item)
	}
	total := len(items)
	start, end := normalizeWindow(total, opts.Limit, opts.Offset)
	if start >= end {
		return []Dimension{}, total, nil
	}
	return items[start:end], total, nil
}

func (r *SQLiteRepository) CreateMember(ctx context.Context, member DimensionMember) (DimensionMember, error) {
	now := time.Now().UTC()
	metadata, err := marshalMap(member.Metadata)
	if err != nil {
		return DimensionMember{}, err
	}
	row := r.db.QueryRowContext(ctx, `
INSERT INTO dimension_members (dimension_id, code, name, parent_id, level, path, is_active, sort_order, metadata, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id, dimension_id, code, name, parent_id, level, path, is_active, sort_order, metadata, created_at, updated_at
`, member.DimensionID, member.Code, member.Name, member.ParentID, member.Level, member.Path, member.IsActive, member.SortOrder, metadata, now, now)
	item, err := scanMember(row)
	if err != nil {
		return DimensionMember{}, mapSQLErr(err)
	}
	return item, nil
}

func (r *SQLiteRepository) GetMemberByID(ctx context.Context, id int64) (DimensionMember, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT id, dimension_id, code, name, parent_id, level, path, is_active, sort_order, metadata, created_at, updated_at
FROM dimension_members WHERE id = ?
`, id)
	return scanMember(row)
}

func (r *SQLiteRepository) GetMemberByCode(ctx context.Context, dimensionID int64, code string) (DimensionMember, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT id, dimension_id, code, name, parent_id, level, path, is_active, sort_order, metadata, created_at, updated_at
FROM dimension_members WHERE dimension_id = ? AND lower(code) = lower(?)
`, dimensionID, code)
	return scanMember(row)
}

func (r *SQLiteRepository) UpdateMember(ctx context.Context, id int64, patch MemberPatch) (DimensionMember, error) {
	current, err := r.GetMemberByID(ctx, id)
	if err != nil {
		return DimensionMember{}, err
	}
	if patch.Name != nil {
		current.Name = *patch.Name
	}
	if patch.ParentIDSet {
		current.ParentID = patch.ParentID
		if current.ParentID == nil {
			current.Level = 1
			current.Path = current.Code
		} else {
			parent, err := r.GetMemberByID(ctx, *current.ParentID)
			if err != nil {
				return DimensionMember{}, err
			}
			current.Level = parent.Level + 1
			current.Path = parent.Path + "/" + current.Code
		}
	}
	if patch.IsActive != nil {
		current.IsActive = *patch.IsActive
	}
	if patch.SortOrder != nil {
		current.SortOrder = *patch.SortOrder
	}
	if patch.MetadataSet {
		current.Metadata = cloneMap(patch.Metadata)
	}
	metadata, err := marshalMap(current.Metadata)
	if err != nil {
		return DimensionMember{}, err
	}
	now := time.Now().UTC()
	_, err = r.db.ExecContext(ctx, `
UPDATE dimension_members
SET name = ?, parent_id = ?, level = ?, path = ?, is_active = ?, sort_order = ?, metadata = ?, updated_at = ?
WHERE id = ?
`, current.Name, current.ParentID, current.Level, current.Path, current.IsActive, current.SortOrder, metadata, now, id)
	if err != nil {
		return DimensionMember{}, mapSQLErr(err)
	}
	return r.GetMemberByID(ctx, id)
}

func (r *SQLiteRepository) DeleteMember(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM dimension_members WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *SQLiteRepository) ListMembers(ctx context.Context, opts MemberQueryOptions) ([]DimensionMember, int, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, dimension_id, code, name, parent_id, level, path, is_active, sort_order, metadata, created_at, updated_at
FROM dimension_members
ORDER BY id
`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []DimensionMember
	for rows.Next() {
		item, err := scanMember(rows)
		if err != nil {
			return nil, 0, err
		}
		if opts.DimensionID != nil && item.DimensionID != *opts.DimensionID {
			continue
		}
		if opts.ParentIsNull {
			if item.ParentID != nil {
				continue
			}
		} else if opts.ParentID != nil {
			if item.ParentID == nil || *item.ParentID != *opts.ParentID {
				continue
			}
		}
		if opts.IsActive != nil && item.IsActive != *opts.IsActive {
			continue
		}
		if opts.Level != nil && item.Level != *opts.Level {
			continue
		}
		if opts.Keyword != "" && !containsFold(item.Code, opts.Keyword) && !containsFold(item.Name, opts.Keyword) {
			continue
		}
		items = append(items, item)
	}
	total := len(items)
	start, end := normalizeWindow(total, opts.Limit, opts.Offset)
	if start >= end {
		return []DimensionMember{}, total, nil
	}
	return items[start:end], total, nil
}

func (r *SQLiteRepository) CreateMappingRule(ctx context.Context, rule MappingRule) (MappingRule, error) {
	now := time.Now().UTC()
	row := r.db.QueryRowContext(ctx, `
INSERT INTO mapping_rules (company, rule_name, priority, account_code_pattern, account_name_pattern, summary_pattern, counterparty_pattern, dimension_code, member_code, allocation_ratio, valid_from, valid_to, is_active, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id, company, rule_name, priority, account_code_pattern, account_name_pattern, summary_pattern, counterparty_pattern, dimension_code, member_code, allocation_ratio, valid_from, valid_to, is_active, created_at
`, rule.Company, rule.RuleName, rule.Priority, rule.AccountCodePattern, rule.AccountNamePattern, rule.SummaryPattern, rule.CounterpartyPattern, rule.DimensionCode, rule.MemberCode, rule.AllocationRatio, rule.ValidFrom, rule.ValidTo, rule.IsActive, now)
	item, err := scanMappingRule(row)
	if err != nil {
		return MappingRule{}, mapSQLErr(err)
	}
	return item, nil
}

func (r *SQLiteRepository) GetMappingRuleByID(ctx context.Context, id int64) (MappingRule, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT id, company, rule_name, priority, account_code_pattern, account_name_pattern, summary_pattern, counterparty_pattern, dimension_code, member_code, allocation_ratio, valid_from, valid_to, is_active, created_at
FROM mapping_rules WHERE id = ?
`, id)
	return scanMappingRule(row)
}

func (r *SQLiteRepository) GetMappingRuleByName(ctx context.Context, company, ruleName string) (MappingRule, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT id, company, rule_name, priority, account_code_pattern, account_name_pattern, summary_pattern, counterparty_pattern, dimension_code, member_code, allocation_ratio, valid_from, valid_to, is_active, created_at
FROM mapping_rules WHERE lower(company) = lower(?) AND lower(rule_name) = lower(?)
`, company, ruleName)
	return scanMappingRule(row)
}

func (r *SQLiteRepository) UpdateMappingRule(ctx context.Context, id int64, patch MappingRulePatch) (MappingRule, error) {
	current, err := r.GetMappingRuleByID(ctx, id)
	if err != nil {
		return MappingRule{}, err
	}
	if patch.RuleName != nil {
		current.RuleName = *patch.RuleName
	}
	if patch.Priority != nil {
		current.Priority = *patch.Priority
	}
	if patch.AccountCodePattern != nil {
		current.AccountCodePattern = patch.AccountCodePattern
	}
	if patch.AccountNamePattern != nil {
		current.AccountNamePattern = patch.AccountNamePattern
	}
	if patch.SummaryPattern != nil {
		current.SummaryPattern = patch.SummaryPattern
	}
	if patch.CounterpartyPattern != nil {
		current.CounterpartyPattern = patch.CounterpartyPattern
	}
	if patch.DimensionCode != nil {
		current.DimensionCode = *patch.DimensionCode
	}
	if patch.MemberCode != nil {
		current.MemberCode = *patch.MemberCode
	}
	if patch.AllocationRatio != nil {
		current.AllocationRatio = *patch.AllocationRatio
	}
	if patch.ValidFrom != nil {
		current.ValidFrom = patch.ValidFrom
	}
	if patch.ValidTo != nil {
		current.ValidTo = patch.ValidTo
	}
	if patch.IsActive != nil {
		current.IsActive = *patch.IsActive
	}
	_, err = r.db.ExecContext(ctx, `
UPDATE mapping_rules
SET rule_name = ?, priority = ?, account_code_pattern = ?, account_name_pattern = ?, summary_pattern = ?, counterparty_pattern = ?, dimension_code = ?, member_code = ?, allocation_ratio = ?, valid_from = ?, valid_to = ?, is_active = ?
WHERE id = ?
`, current.RuleName, current.Priority, current.AccountCodePattern, current.AccountNamePattern, current.SummaryPattern, current.CounterpartyPattern, current.DimensionCode, current.MemberCode, current.AllocationRatio, current.ValidFrom, current.ValidTo, current.IsActive, id)
	if err != nil {
		return MappingRule{}, mapSQLErr(err)
	}
	return r.GetMappingRuleByID(ctx, id)
}

func (r *SQLiteRepository) DeleteMappingRule(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM mapping_rules WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *SQLiteRepository) ListMappingRules(ctx context.Context, opts MappingRuleQueryOptions) ([]MappingRule, int, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, company, rule_name, priority, account_code_pattern, account_name_pattern, summary_pattern, counterparty_pattern, dimension_code, member_code, allocation_ratio, valid_from, valid_to, is_active, created_at
FROM mapping_rules
ORDER BY priority DESC, id ASC
`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []MappingRule
	for rows.Next() {
		item, err := scanMappingRule(rows)
		if err != nil {
			return nil, 0, err
		}
		if opts.Company != "" && !strings.EqualFold(item.Company, opts.Company) {
			continue
		}
		if opts.IsActive != nil && item.IsActive != *opts.IsActive {
			continue
		}
		if opts.DimensionCode != "" && !strings.EqualFold(item.DimensionCode, opts.DimensionCode) {
			continue
		}
		if opts.Keyword != "" && !containsFold(item.RuleName, opts.Keyword) {
			continue
		}
		items = append(items, item)
	}
	total := len(items)
	start, end := normalizeWindow(total, opts.Limit, opts.Offset)
	if start >= end {
		return []MappingRule{}, total, nil
	}
	return items[start:end], total, nil
}

type scanner interface{ Scan(dest ...any) error }

func scanDimension(s scanner) (Dimension, error) {
	var d Dimension
	var typ string
	var description sql.NullString
	var metadata sql.NullString
	if err := s.Scan(&d.ID, &d.Code, &d.Name, &typ, &description, &d.IsHierarchical, &d.IsActive, &metadata, &d.CreatedAt, &d.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return Dimension{}, ErrNotFound
		}
		return Dimension{}, err
	}
	d.Type = DimensionType(typ)
	if description.Valid {
		desc := description.String
		d.Description = &desc
	}
	if metadata.Valid && metadata.String != "" {
		_ = json.Unmarshal([]byte(metadata.String), &d.Metadata)
	}
	return d, nil
}

func scanMember(s scanner) (DimensionMember, error) {
	var m DimensionMember
	var parentID sql.NullInt64
	var metadata sql.NullString
	if err := s.Scan(&m.ID, &m.DimensionID, &m.Code, &m.Name, &parentID, &m.Level, &m.Path, &m.IsActive, &m.SortOrder, &metadata, &m.CreatedAt, &m.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return DimensionMember{}, ErrNotFound
		}
		return DimensionMember{}, err
	}
	if parentID.Valid {
		v := parentID.Int64
		m.ParentID = &v
	}
	if metadata.Valid && metadata.String != "" {
		_ = json.Unmarshal([]byte(metadata.String), &m.Metadata)
	}
	return m, nil
}

func scanMappingRule(s scanner) (MappingRule, error) {
	var r MappingRule
	var accountCodePattern, accountNamePattern, summaryPattern, counterpartyPattern sql.NullString
	var validFrom, validTo sql.NullString
	if err := s.Scan(&r.ID, &r.Company, &r.RuleName, &r.Priority, &accountCodePattern, &accountNamePattern, &summaryPattern, &counterpartyPattern, &r.DimensionCode, &r.MemberCode, &r.AllocationRatio, &validFrom, &validTo, &r.IsActive, &r.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return MappingRule{}, ErrNotFound
		}
		return MappingRule{}, err
	}
	if accountCodePattern.Valid {
		v := accountCodePattern.String
		r.AccountCodePattern = &v
	}
	if accountNamePattern.Valid {
		v := accountNamePattern.String
		r.AccountNamePattern = &v
	}
	if summaryPattern.Valid {
		v := summaryPattern.String
		r.SummaryPattern = &v
	}
	if counterpartyPattern.Valid {
		v := counterpartyPattern.String
		r.CounterpartyPattern = &v
	}
	if validFrom.Valid {
		v := validFrom.String
		r.ValidFrom = &v
	}
	if validTo.Valid {
		v := validTo.String
		r.ValidTo = &v
	}
	return r, nil
}

func marshalMap(m map[string]any) (string, error) {
	if len(m) == 0 {
		return "", nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("marshal metadata: %w", err)
	}
	return string(b), nil
}

func mapSQLErr(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "unique") {
		return ErrAlreadyExists
	}
	return err
}
