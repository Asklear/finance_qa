package dimensions

import (
	"context"
	"strings"
	"sync"
	"time"
)

// MemoryRepository is an in-memory implementation for tests and early integration.
type MemoryRepository struct {
	mu sync.RWMutex

	nextDimensionID   int64
	nextMemberID      int64
	nextMappingRuleID int64

	dimensions     map[int64]Dimension
	dimensionCodes map[string]int64

	members         map[int64]DimensionMember
	memberByDimCode map[int64]map[string]int64
	memberChildren  map[int64][]int64

	mappingRules         map[int64]MappingRule
	mappingRuleByCompany map[string]map[string]int64
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		nextDimensionID:      1,
		nextMemberID:         1,
		nextMappingRuleID:    1,
		dimensions:           make(map[int64]Dimension),
		dimensionCodes:       make(map[string]int64),
		members:              make(map[int64]DimensionMember),
		memberByDimCode:      make(map[int64]map[string]int64),
		memberChildren:       make(map[int64][]int64),
		mappingRules:         make(map[int64]MappingRule),
		mappingRuleByCompany: make(map[string]map[string]int64),
	}
}

func (r *MemoryRepository) CreateDimension(_ context.Context, dim Dimension) (Dimension, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	codeKey := strings.ToLower(dim.Code)
	if _, ok := r.dimensionCodes[codeKey]; ok {
		return Dimension{}, ErrAlreadyExists
	}

	now := time.Now().UTC()
	dim.ID = r.nextDimensionID
	r.nextDimensionID++
	dim.CreatedAt = now
	dim.UpdatedAt = now
	dim.Metadata = cloneMap(dim.Metadata)

	r.dimensions[dim.ID] = dim
	r.dimensionCodes[codeKey] = dim.ID

	return cloneDimension(dim), nil
}

func (r *MemoryRepository) GetDimensionByID(_ context.Context, id int64) (Dimension, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	dim, ok := r.dimensions[id]
	if !ok {
		return Dimension{}, ErrNotFound
	}
	return cloneDimension(dim), nil
}

func (r *MemoryRepository) GetDimensionByCode(_ context.Context, code string) (Dimension, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	id, ok := r.dimensionCodes[strings.ToLower(code)]
	if !ok {
		return Dimension{}, ErrNotFound
	}
	return cloneDimension(r.dimensions[id]), nil
}

func (r *MemoryRepository) UpdateDimension(_ context.Context, id int64, patch DimensionPatch) (Dimension, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	dim, ok := r.dimensions[id]
	if !ok {
		return Dimension{}, ErrNotFound
	}

	if patch.Name != nil {
		dim.Name = *patch.Name
	}
	if patch.Description != nil {
		dim.Description = patch.Description
	}
	if patch.Type != nil {
		dim.Type = *patch.Type
	}
	if patch.IsActive != nil {
		dim.IsActive = *patch.IsActive
	}
	if patch.IsHierarchical != nil {
		dim.IsHierarchical = *patch.IsHierarchical
	}
	if patch.MetadataSet {
		dim.Metadata = cloneMap(patch.Metadata)
	}
	now := time.Now().UTC()
	dim.UpdatedAt = now
	r.dimensions[id] = dim

	return cloneDimension(dim), nil
}

func (r *MemoryRepository) DeleteDimension(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	dim, ok := r.dimensions[id]
	if !ok {
		return ErrNotFound
	}
	delete(r.dimensions, id)
	delete(r.dimensionCodes, strings.ToLower(dim.Code))

	for memberID, member := range r.members {
		if member.DimensionID == id {
			delete(r.members, memberID)
			delete(r.memberChildren, memberID)
		}
	}
	delete(r.memberByDimCode, id)

	return nil
}

func (r *MemoryRepository) ListDimensions(_ context.Context, opts DimensionQueryOptions) ([]Dimension, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]Dimension, 0, len(r.dimensions))
	for _, d := range r.dimensions {
		if opts.Type != nil && d.Type != *opts.Type {
			continue
		}
		if opts.IsActive != nil && d.IsActive != *opts.IsActive {
			continue
		}
		if opts.Keyword != "" {
			k := strings.ToLower(opts.Keyword)
			desc := ""
			if d.Description != nil {
				desc = *d.Description
			}
			if !containsFold(d.Code, k) && !containsFold(d.Name, k) && !containsFold(desc, k) {
				continue
			}
		}
		items = append(items, cloneDimension(d))
	}

	total := len(items)
	start, end := normalizeWindow(total, opts.Limit, opts.Offset)
	if start >= end {
		return []Dimension{}, total, nil
	}
	return items[start:end], total, nil
}

func (r *MemoryRepository) CreateMember(_ context.Context, member DimensionMember) (DimensionMember, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.dimensions[member.DimensionID]; !ok {
		return DimensionMember{}, ErrNotFound
	}
	if _, ok := r.memberByDimCode[member.DimensionID]; !ok {
		r.memberByDimCode[member.DimensionID] = make(map[string]int64)
	}
	codeKey := strings.ToLower(member.Code)
	if _, ok := r.memberByDimCode[member.DimensionID][codeKey]; ok {
		return DimensionMember{}, ErrAlreadyExists
	}

	now := time.Now().UTC()
	member.ID = r.nextMemberID
	r.nextMemberID++
	member.CreatedAt = now
	member.UpdatedAt = now
	member.Metadata = cloneMap(member.Metadata)

	r.members[member.ID] = member
	r.memberByDimCode[member.DimensionID][codeKey] = member.ID
	if member.ParentID != nil {
		r.memberChildren[*member.ParentID] = append(r.memberChildren[*member.ParentID], member.ID)
	}

	return cloneMember(member), nil
}

func (r *MemoryRepository) GetMemberByID(_ context.Context, id int64) (DimensionMember, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	member, ok := r.members[id]
	if !ok {
		return DimensionMember{}, ErrNotFound
	}
	return cloneMember(member), nil
}

func (r *MemoryRepository) GetMemberByCode(_ context.Context, dimensionID int64, code string) (DimensionMember, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	byCode, ok := r.memberByDimCode[dimensionID]
	if !ok {
		return DimensionMember{}, ErrNotFound
	}
	id, ok := byCode[strings.ToLower(code)]
	if !ok {
		return DimensionMember{}, ErrNotFound
	}
	return cloneMember(r.members[id]), nil
}

func (r *MemoryRepository) UpdateMember(_ context.Context, id int64, patch MemberPatch) (DimensionMember, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	member, ok := r.members[id]
	if !ok {
		return DimensionMember{}, ErrNotFound
	}

	if patch.Name != nil {
		member.Name = *patch.Name
	}
	if patch.ParentIDSet {
		if member.ParentID != nil {
			r.memberChildren[*member.ParentID] = removeID(r.memberChildren[*member.ParentID], member.ID)
		}
		member.ParentID = patch.ParentID
		if member.ParentID != nil {
			r.memberChildren[*member.ParentID] = append(r.memberChildren[*member.ParentID], member.ID)
		}
	}
	if patch.IsActive != nil {
		member.IsActive = *patch.IsActive
	}
	if patch.SortOrder != nil {
		member.SortOrder = *patch.SortOrder
	}
	if patch.MetadataSet {
		member.Metadata = cloneMap(patch.Metadata)
	}
	member.UpdatedAt = time.Now().UTC()

	r.members[id] = member
	return cloneMember(member), nil
}

func (r *MemoryRepository) DeleteMember(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	member, ok := r.members[id]
	if !ok {
		return ErrNotFound
	}
	if member.ParentID != nil {
		r.memberChildren[*member.ParentID] = removeID(r.memberChildren[*member.ParentID], id)
	}
	delete(r.members, id)
	if codes, ok := r.memberByDimCode[member.DimensionID]; ok {
		delete(codes, strings.ToLower(member.Code))
	}
	delete(r.memberChildren, id)
	return nil
}

func (r *MemoryRepository) ListMembers(_ context.Context, opts MemberQueryOptions) ([]DimensionMember, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]DimensionMember, 0, len(r.members))
	for _, m := range r.members {
		if opts.DimensionID != nil && m.DimensionID != *opts.DimensionID {
			continue
		}
		if opts.ParentIsNull {
			if m.ParentID != nil {
				continue
			}
		} else if opts.ParentID != nil {
			if m.ParentID == nil || *m.ParentID != *opts.ParentID {
				continue
			}
		}
		if opts.IsActive != nil && m.IsActive != *opts.IsActive {
			continue
		}
		if opts.Level != nil && m.Level != *opts.Level {
			continue
		}
		if opts.Keyword != "" {
			k := strings.ToLower(opts.Keyword)
			if !containsFold(m.Code, k) && !containsFold(m.Name, k) {
				continue
			}
		}
		items = append(items, cloneMember(m))
	}

	total := len(items)
	start, end := normalizeWindow(total, opts.Limit, opts.Offset)
	if start >= end {
		return []DimensionMember{}, total, nil
	}
	return items[start:end], total, nil
}

func (r *MemoryRepository) CreateMappingRule(_ context.Context, rule MappingRule) (MappingRule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	companyKey := strings.ToLower(rule.Company)
	nameKey := strings.ToLower(rule.RuleName)
	if _, ok := r.mappingRuleByCompany[companyKey]; !ok {
		r.mappingRuleByCompany[companyKey] = make(map[string]int64)
	}
	if _, ok := r.mappingRuleByCompany[companyKey][nameKey]; ok {
		return MappingRule{}, ErrAlreadyExists
	}

	now := time.Now().UTC()
	rule.ID = r.nextMappingRuleID
	r.nextMappingRuleID++
	rule.CreatedAt = now

	r.mappingRules[rule.ID] = cloneMappingRule(rule)
	r.mappingRuleByCompany[companyKey][nameKey] = rule.ID

	return cloneMappingRule(rule), nil
}

func (r *MemoryRepository) GetMappingRuleByID(_ context.Context, id int64) (MappingRule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rule, ok := r.mappingRules[id]
	if !ok {
		return MappingRule{}, ErrNotFound
	}
	return cloneMappingRule(rule), nil
}

func (r *MemoryRepository) GetMappingRuleByName(_ context.Context, company, ruleName string) (MappingRule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	companyKey := strings.ToLower(company)
	nameKey := strings.ToLower(ruleName)
	byName, ok := r.mappingRuleByCompany[companyKey]
	if !ok {
		return MappingRule{}, ErrNotFound
	}
	id, ok := byName[nameKey]
	if !ok {
		return MappingRule{}, ErrNotFound
	}
	return cloneMappingRule(r.mappingRules[id]), nil
}

func (r *MemoryRepository) UpdateMappingRule(_ context.Context, id int64, patch MappingRulePatch) (MappingRule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rule, ok := r.mappingRules[id]
	if !ok {
		return MappingRule{}, ErrNotFound
	}

	oldCompanyKey := strings.ToLower(rule.Company)
	oldNameKey := strings.ToLower(rule.RuleName)

	if patch.RuleName != nil {
		rule.RuleName = *patch.RuleName
	}
	if patch.Priority != nil {
		rule.Priority = *patch.Priority
	}
	if patch.AccountCodePattern != nil {
		rule.AccountCodePattern = patch.AccountCodePattern
	}
	if patch.AccountNamePattern != nil {
		rule.AccountNamePattern = patch.AccountNamePattern
	}
	if patch.SummaryPattern != nil {
		rule.SummaryPattern = patch.SummaryPattern
	}
	if patch.CounterpartyPattern != nil {
		rule.CounterpartyPattern = patch.CounterpartyPattern
	}
	if patch.DimensionCode != nil {
		rule.DimensionCode = *patch.DimensionCode
	}
	if patch.MemberCode != nil {
		rule.MemberCode = *patch.MemberCode
	}
	if patch.AllocationRatio != nil {
		rule.AllocationRatio = *patch.AllocationRatio
	}
	if patch.ValidFrom != nil {
		rule.ValidFrom = patch.ValidFrom
	}
	if patch.ValidTo != nil {
		rule.ValidTo = patch.ValidTo
	}
	if patch.IsActive != nil {
		rule.IsActive = *patch.IsActive
	}

	now := time.Now().UTC()
	rule.UpdatedAt = &now

	newCompanyKey := strings.ToLower(rule.Company)
	newNameKey := strings.ToLower(rule.RuleName)
	if oldCompanyKey != newCompanyKey || oldNameKey != newNameKey {
		if _, ok := r.mappingRuleByCompany[newCompanyKey]; !ok {
			r.mappingRuleByCompany[newCompanyKey] = make(map[string]int64)
		}
		if otherID, exists := r.mappingRuleByCompany[newCompanyKey][newNameKey]; exists && otherID != id {
			return MappingRule{}, ErrAlreadyExists
		}
		delete(r.mappingRuleByCompany[oldCompanyKey], oldNameKey)
		r.mappingRuleByCompany[newCompanyKey][newNameKey] = id
	}

	r.mappingRules[id] = cloneMappingRule(rule)
	return cloneMappingRule(rule), nil
}

func (r *MemoryRepository) DeleteMappingRule(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rule, ok := r.mappingRules[id]
	if !ok {
		return ErrNotFound
	}
	delete(r.mappingRules, id)
	delete(r.mappingRuleByCompany[strings.ToLower(rule.Company)], strings.ToLower(rule.RuleName))
	return nil
}

func (r *MemoryRepository) ListMappingRules(_ context.Context, opts MappingRuleQueryOptions) ([]MappingRule, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]MappingRule, 0, len(r.mappingRules))
	for _, rule := range r.mappingRules {
		if opts.Company != "" && !strings.EqualFold(rule.Company, opts.Company) {
			continue
		}
		if opts.IsActive != nil && rule.IsActive != *opts.IsActive {
			continue
		}
		if opts.DimensionCode != "" && !strings.EqualFold(rule.DimensionCode, opts.DimensionCode) {
			continue
		}
		if opts.Keyword != "" {
			k := strings.ToLower(opts.Keyword)
			if !containsFold(rule.RuleName, k) {
				continue
			}
		}
		items = append(items, cloneMappingRule(rule))
	}

	total := len(items)
	start, end := normalizeWindow(total, opts.Limit, opts.Offset)
	if start >= end {
		return []MappingRule{}, total, nil
	}
	return items[start:end], total, nil
}

func removeID(ids []int64, id int64) []int64 {
	for i, v := range ids {
		if v == id {
			return append(ids[:i], ids[i+1:]...)
		}
	}
	return ids
}

func normalizeWindow(total, limit, offset int) (int, int) {
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	if limit <= 0 {
		return offset, total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return offset, end
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

func cloneDimension(d Dimension) Dimension {
	d.Metadata = cloneMap(d.Metadata)
	if d.Description != nil {
		desc := *d.Description
		d.Description = &desc
	}
	return d
}

func cloneMember(m DimensionMember) DimensionMember {
	m.Metadata = cloneMap(m.Metadata)
	if m.ParentID != nil {
		id := *m.ParentID
		m.ParentID = &id
	}
	return m
}

func cloneMappingRule(r MappingRule) MappingRule {
	if r.AccountCodePattern != nil {
		v := *r.AccountCodePattern
		r.AccountCodePattern = &v
	}
	if r.AccountNamePattern != nil {
		v := *r.AccountNamePattern
		r.AccountNamePattern = &v
	}
	if r.SummaryPattern != nil {
		v := *r.SummaryPattern
		r.SummaryPattern = &v
	}
	if r.CounterpartyPattern != nil {
		v := *r.CounterpartyPattern
		r.CounterpartyPattern = &v
	}
	if r.ValidFrom != nil {
		v := *r.ValidFrom
		r.ValidFrom = &v
	}
	if r.ValidTo != nil {
		v := *r.ValidTo
		r.ValidTo = &v
	}
	if r.UpdatedAt != nil {
		v := *r.UpdatedAt
		r.UpdatedAt = &v
	}
	return r
}

func cloneMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
