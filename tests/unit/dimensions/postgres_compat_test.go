package dimensions_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"financeqa/internal/dimensions"
)

const fakePGRepoDriverName = "financeqa-fake-pg-dimensions"

var registerFakePGRepoDriver sync.Once

func TestSQLiteRepositoryCreateDimensionWorksWhenLastInsertIDUnsupported(t *testing.T) {
	repo := newFakePGRepo(t)

	got, err := repo.CreateDimension(context.Background(), dimensions.Dimension{
		Code:           "product",
		Name:           "Product",
		Type:           dimensions.DimensionTypeProduct,
		IsActive:       true,
		IsHierarchical: false,
	})
	if err != nil {
		t.Fatalf("create dimension: %v", err)
	}
	if got.ID != 1 {
		t.Fatalf("dimension id = %d, want 1", got.ID)
	}
	if got.Code != "product" || got.Name != "Product" {
		t.Fatalf("unexpected dimension: %+v", got)
	}
}

func TestSQLiteRepositoryCreateMemberWorksWhenLastInsertIDUnsupported(t *testing.T) {
	repo := newFakePGRepo(t)

	got, err := repo.CreateMember(context.Background(), dimensions.DimensionMember{
		DimensionID: 42,
		Code:        "P001",
		Name:        "SaaS",
		Level:       1,
		Path:        "P001",
		IsActive:    true,
	})
	if err != nil {
		t.Fatalf("create member: %v", err)
	}
	if got.ID != 1 {
		t.Fatalf("member id = %d, want 1", got.ID)
	}
	if got.DimensionID != 42 || got.Code != "P001" || got.Name != "SaaS" {
		t.Fatalf("unexpected member: %+v", got)
	}
}

func TestSQLiteRepositoryCreateMappingRuleWorksWhenLastInsertIDUnsupported(t *testing.T) {
	repo := newFakePGRepo(t)

	active := true
	accountCode := "6001%"
	got, err := repo.CreateMappingRule(context.Background(), dimensions.MappingRule{
		Company:            "ACME",
		RuleName:           "Revenue",
		Priority:           10,
		AccountCodePattern: &accountCode,
		DimensionCode:      "product",
		MemberCode:         "P001",
		AllocationRatio:    1,
		IsActive:           active,
	})
	if err != nil {
		t.Fatalf("create mapping rule: %v", err)
	}
	if got.ID != 1 {
		t.Fatalf("mapping rule id = %d, want 1", got.ID)
	}
	if got.Company != "ACME" || got.RuleName != "Revenue" {
		t.Fatalf("unexpected mapping rule: %+v", got)
	}
}

func newFakePGRepo(t *testing.T) *dimensions.SQLiteRepository {
	t.Helper()
	registerFakePGRepoDriver.Do(func() {
		sql.Register(fakePGRepoDriverName, &fakePGRepoDriver{})
	})
	db, err := sql.Open(fakePGRepoDriverName, "")
	if err != nil {
		t.Fatalf("open fake repo db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return dimensions.NewSQLiteRepository(db)
}

type fakePGRepoDriver struct{}

func (d *fakePGRepoDriver) Open(name string) (driver.Conn, error) {
	return &fakePGRepoConn{state: newFakePGRepoState()}, nil
}

type fakePGRepoConn struct {
	state *fakePGRepoState
}

func (c *fakePGRepoConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare not supported")
}
func (c *fakePGRepoConn) Close() error { return nil }
func (c *fakePGRepoConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions not supported")
}

func (c *fakePGRepoConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	switch {
	case strings.Contains(normalizeSQL(query), "insert into dimensions"):
		c.state.insertDimension(args)
		return fakeNoLastInsertResult{}, nil
	case strings.Contains(normalizeSQL(query), "insert into dimension_members"):
		c.state.insertMember(args)
		return fakeNoLastInsertResult{}, nil
	case strings.Contains(normalizeSQL(query), "insert into mapping_rules"):
		c.state.insertMappingRule(args)
		return fakeNoLastInsertResult{}, nil
	default:
		return nil, fmt.Errorf("unsupported exec query: %s", query)
	}
}

func (c *fakePGRepoConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	norm := normalizeSQL(query)
	switch {
	case strings.Contains(norm, "insert into dimensions") && strings.Contains(norm, "returning"):
		row := c.state.insertDimension(args)
		return newFakeRows(
			[]string{"id", "code", "name", "type", "description", "is_hierarchical", "is_active", "metadata", "created_at", "updated_at"},
			[][]driver.Value{{
				row.ID, row.Code, row.Name, row.Type, row.Description, row.IsHierarchical, row.IsActive, row.Metadata, row.CreatedAt, row.UpdatedAt,
			}},
		), nil
	case strings.Contains(norm, "insert into dimension_members") && strings.Contains(norm, "returning"):
		row := c.state.insertMember(args)
		return newFakeRows(
			[]string{"id", "dimension_id", "code", "name", "parent_id", "level", "path", "is_active", "sort_order", "metadata", "created_at", "updated_at"},
			[][]driver.Value{{
				row.ID, row.DimensionID, row.Code, row.Name, row.ParentID, row.Level, row.Path, row.IsActive, row.SortOrder, row.Metadata, row.CreatedAt, row.UpdatedAt,
			}},
		), nil
	case strings.Contains(norm, "insert into mapping_rules") && strings.Contains(norm, "returning"):
		row := c.state.insertMappingRule(args)
		return newFakeRows(
			[]string{"id", "company", "rule_name", "priority", "account_code_pattern", "account_name_pattern", "summary_pattern", "counterparty_pattern", "dimension_code", "member_code", "allocation_ratio", "valid_from", "valid_to", "is_active", "created_at"},
			[][]driver.Value{{
				row.ID, row.Company, row.RuleName, row.Priority, row.AccountCodePattern, row.AccountNamePattern, row.SummaryPattern, row.CounterpartyPattern, row.DimensionCode, row.MemberCode, row.AllocationRatio, row.ValidFrom, row.ValidTo, row.IsActive, row.CreatedAt,
			}},
		), nil
	case strings.Contains(norm, "from dimensions where id ="):
		if row, ok := c.state.dimensionByID(asInt64(args[0].Value)); ok {
			return newFakeRows(
				[]string{"id", "code", "name", "type", "description", "is_hierarchical", "is_active", "metadata", "created_at", "updated_at"},
				[][]driver.Value{{row.ID, row.Code, row.Name, row.Type, row.Description, row.IsHierarchical, row.IsActive, row.Metadata, row.CreatedAt, row.UpdatedAt}},
			), nil
		}
		return newFakeRows(nil, nil), nil
	case strings.Contains(norm, "from dimension_members where id ="):
		if row, ok := c.state.memberByID(asInt64(args[0].Value)); ok {
			return newFakeRows(
				[]string{"id", "dimension_id", "code", "name", "parent_id", "level", "path", "is_active", "sort_order", "metadata", "created_at", "updated_at"},
				[][]driver.Value{{row.ID, row.DimensionID, row.Code, row.Name, row.ParentID, row.Level, row.Path, row.IsActive, row.SortOrder, row.Metadata, row.CreatedAt, row.UpdatedAt}},
			), nil
		}
		return newFakeRows(nil, nil), nil
	case strings.Contains(norm, "from mapping_rules where id ="):
		if row, ok := c.state.mappingRuleByID(asInt64(args[0].Value)); ok {
			return newFakeRows(
				[]string{"id", "company", "rule_name", "priority", "account_code_pattern", "account_name_pattern", "summary_pattern", "counterparty_pattern", "dimension_code", "member_code", "allocation_ratio", "valid_from", "valid_to", "is_active", "created_at"},
				[][]driver.Value{{row.ID, row.Company, row.RuleName, row.Priority, row.AccountCodePattern, row.AccountNamePattern, row.SummaryPattern, row.CounterpartyPattern, row.DimensionCode, row.MemberCode, row.AllocationRatio, row.ValidFrom, row.ValidTo, row.IsActive, row.CreatedAt}},
			), nil
		}
		return newFakeRows(nil, nil), nil
	default:
		return nil, fmt.Errorf("unsupported query: %s", query)
	}
}

type fakePGRepoState struct {
	nextID       int64
	dimensions   map[int64]fakeDimensionRow
	members      map[int64]fakeMemberRow
	mappingRules map[int64]fakeMappingRuleRow
}

func newFakePGRepoState() *fakePGRepoState {
	return &fakePGRepoState{
		nextID:       1,
		dimensions:   make(map[int64]fakeDimensionRow),
		members:      make(map[int64]fakeMemberRow),
		mappingRules: make(map[int64]fakeMappingRuleRow),
	}
}

func (s *fakePGRepoState) next() int64 {
	id := s.nextID
	s.nextID++
	return id
}

func (s *fakePGRepoState) insertDimension(args []driver.NamedValue) fakeDimensionRow {
	now := time.Unix(1700000000, 0).UTC()
	row := fakeDimensionRow{
		ID:             s.next(),
		Code:           asString(args[0].Value),
		Name:           asString(args[1].Value),
		Type:           asString(args[2].Value),
		Description:    normalizeNullable(args[3].Value),
		IsHierarchical: asBool(args[4].Value),
		IsActive:       asBool(args[5].Value),
		Metadata:       normalizeNullable(args[6].Value),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.dimensions[row.ID] = row
	return row
}

func (s *fakePGRepoState) insertMember(args []driver.NamedValue) fakeMemberRow {
	now := time.Unix(1700000000, 0).UTC()
	row := fakeMemberRow{
		ID:          s.next(),
		DimensionID: asInt64(args[0].Value),
		Code:        asString(args[1].Value),
		Name:        asString(args[2].Value),
		ParentID:    normalizeNullableInt64(args[3].Value),
		Level:       int(asInt64(args[4].Value)),
		Path:        asString(args[5].Value),
		IsActive:    asBool(args[6].Value),
		SortOrder:   int(asInt64(args[7].Value)),
		Metadata:    normalizeNullable(args[8].Value),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.members[row.ID] = row
	return row
}

func (s *fakePGRepoState) insertMappingRule(args []driver.NamedValue) fakeMappingRuleRow {
	now := time.Unix(1700000000, 0).UTC()
	row := fakeMappingRuleRow{
		ID:                  s.next(),
		Company:             asString(args[0].Value),
		RuleName:            asString(args[1].Value),
		Priority:            int(asInt64(args[2].Value)),
		AccountCodePattern:  normalizeNullable(args[3].Value),
		AccountNamePattern:  normalizeNullable(args[4].Value),
		SummaryPattern:      normalizeNullable(args[5].Value),
		CounterpartyPattern: normalizeNullable(args[6].Value),
		DimensionCode:       asString(args[7].Value),
		MemberCode:          asString(args[8].Value),
		AllocationRatio:     asFloat64(args[9].Value),
		ValidFrom:           normalizeNullable(args[10].Value),
		ValidTo:             normalizeNullable(args[11].Value),
		IsActive:            asBool(args[12].Value),
		CreatedAt:           now,
	}
	s.mappingRules[row.ID] = row
	return row
}

func (s *fakePGRepoState) dimensionByID(id int64) (fakeDimensionRow, bool) {
	row, ok := s.dimensions[id]
	return row, ok
}

func (s *fakePGRepoState) memberByID(id int64) (fakeMemberRow, bool) {
	row, ok := s.members[id]
	return row, ok
}

func (s *fakePGRepoState) mappingRuleByID(id int64) (fakeMappingRuleRow, bool) {
	row, ok := s.mappingRules[id]
	return row, ok
}

type fakeDimensionRow struct {
	ID             int64
	Code           string
	Name           string
	Type           string
	Description    any
	IsHierarchical bool
	IsActive       bool
	Metadata       any
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type fakeMemberRow struct {
	ID          int64
	DimensionID int64
	Code        string
	Name        string
	ParentID    any
	Level       int
	Path        string
	IsActive    bool
	SortOrder   int
	Metadata    any
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type fakeMappingRuleRow struct {
	ID                  int64
	Company             string
	RuleName            string
	Priority            int
	AccountCodePattern  any
	AccountNamePattern  any
	SummaryPattern      any
	CounterpartyPattern any
	DimensionCode       string
	MemberCode          string
	AllocationRatio     float64
	ValidFrom           any
	ValidTo             any
	IsActive            bool
	CreatedAt           time.Time
}

type fakeNoLastInsertResult struct{}

func (fakeNoLastInsertResult) LastInsertId() (int64, error) {
	return 0, errors.New("LastInsertId is not supported by this driver")
}
func (fakeNoLastInsertResult) RowsAffected() (int64, error) { return 1, nil }

type fakeRows struct {
	columns []string
	data    [][]driver.Value
	idx     int
}

func newFakeRows(columns []string, data [][]driver.Value) *fakeRows {
	return &fakeRows{columns: columns, data: data}
}

func (r *fakeRows) Columns() []string { return r.columns }
func (r *fakeRows) Close() error      { return nil }

func (r *fakeRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.data) {
		return io.EOF
	}
	copy(dest, r.data[r.idx])
	r.idx++
	return nil
}

func normalizeSQL(query string) string {
	return strings.Join(strings.Fields(strings.ToLower(query)), " ")
}

func asString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	default:
		return fmt.Sprint(v)
	}
}

func asInt64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int32:
		return int64(x)
	case int:
		return int64(x)
	case float64:
		return int64(x)
	default:
		return 0
	}
}

func asFloat64(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int64:
		return float64(x)
	case int:
		return float64(x)
	default:
		return 0
	}
}

func asBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case int64:
		return x != 0
	case int:
		return x != 0
	default:
		return false
	}
}

func normalizeNullable(v any) any {
	if v == nil {
		return nil
	}
	s := strings.TrimSpace(asString(v))
	if s == "" || s == "<nil>" {
		return nil
	}
	return s
}

func normalizeNullableInt64(v any) any {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	default:
		return nil
	}
}
