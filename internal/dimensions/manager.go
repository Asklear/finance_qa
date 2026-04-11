package dimensions

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Manager provides dimension/member/mapping-rule operations decoupled from storage.
type Manager struct {
	repo Repository
}

func NewManager(repo Repository) *Manager {
	return &Manager{repo: repo}
}

func (m *Manager) CreateDimension(ctx context.Context, input CreateDimensionInput) (Dimension, error) {
	code := strings.TrimSpace(input.Code)
	name := strings.TrimSpace(input.Name)
	if code == "" || name == "" || !input.Type.Valid() {
		return Dimension{}, ErrInvalidInput
	}
	if _, err := m.repo.GetDimensionByCode(ctx, code); err == nil {
		return Dimension{}, ErrAlreadyExists
	} else if err != nil && err != ErrNotFound {
		return Dimension{}, err
	}

	dim := Dimension{
		Code:           code,
		Name:           name,
		Description:    input.Description,
		Type:           input.Type,
		IsActive:       true,
		IsHierarchical: input.IsHierarchical,
		Metadata:       cloneMap(input.Metadata),
	}
	return m.repo.CreateDimension(ctx, dim)
}

func (m *Manager) GetDimension(ctx context.Context, id int64) (Dimension, error) {
	if id <= 0 {
		return Dimension{}, ErrInvalidInput
	}
	return m.repo.GetDimensionByID(ctx, id)
}

func (m *Manager) GetDimensionByCode(ctx context.Context, code string) (Dimension, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return Dimension{}, ErrInvalidInput
	}
	return m.repo.GetDimensionByCode(ctx, code)
}

func (m *Manager) UpdateDimension(ctx context.Context, id int64, patch DimensionPatch) (Dimension, error) {
	if id <= 0 {
		return Dimension{}, ErrInvalidInput
	}
	if patch.Type != nil && !patch.Type.Valid() {
		return Dimension{}, ErrInvalidInput
	}
	if patch.Name != nil {
		name := strings.TrimSpace(*patch.Name)
		if name == "" {
			return Dimension{}, ErrInvalidInput
		}
		patch.Name = &name
	}
	return m.repo.UpdateDimension(ctx, id, patch)
}

func (m *Manager) DeleteDimension(ctx context.Context, id int64, force bool) error {
	if id <= 0 {
		return ErrInvalidInput
	}
	if !force {
		dimID := id
		members, total, err := m.repo.ListMembers(ctx, MemberQueryOptions{DimensionID: &dimID, Limit: 1})
		if err != nil {
			return err
		}
		if total > 0 || len(members) > 0 {
			return ErrConflict
		}
	}
	return m.repo.DeleteDimension(ctx, id)
}

func (m *Manager) ListDimensions(ctx context.Context, opts DimensionQueryOptions) (PaginatedResult[Dimension], error) {
	items, total, err := m.repo.ListDimensions(ctx, opts)
	if err != nil {
		return PaginatedResult[Dimension]{}, err
	}
	pageSize := opts.Limit
	if pageSize <= 0 {
		pageSize = len(items)
		if pageSize == 0 {
			pageSize = 1
		}
	}
	page := (opts.Offset / pageSize) + 1
	totalPages := (total + pageSize - 1) / pageSize
	if total == 0 {
		totalPages = 0
	}
	return PaginatedResult[Dimension]{
		Data:       items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

func (m *Manager) AddMember(ctx context.Context, input AddMemberInput) (DimensionMember, error) {
	code := strings.TrimSpace(input.Code)
	name := strings.TrimSpace(input.Name)
	if input.DimensionID <= 0 || code == "" || name == "" {
		return DimensionMember{}, ErrInvalidInput
	}
	if _, err := m.repo.GetDimensionByID(ctx, input.DimensionID); err != nil {
		return DimensionMember{}, err
	}
	if _, err := m.repo.GetMemberByCode(ctx, input.DimensionID, code); err == nil {
		return DimensionMember{}, ErrAlreadyExists
	} else if err != nil && err != ErrNotFound {
		return DimensionMember{}, err
	}

	level := 1
	path := code
	if input.ParentID != nil {
		parent, err := m.repo.GetMemberByID(ctx, *input.ParentID)
		if err != nil {
			return DimensionMember{}, err
		}
		if parent.DimensionID != input.DimensionID {
			return DimensionMember{}, fmt.Errorf("%w: parent member belongs to another dimension", ErrInvalidInput)
		}
		level = parent.Level + 1
		path = parent.Path + "/" + code
	}

	member := DimensionMember{
		DimensionID: input.DimensionID,
		Code:        code,
		Name:        name,
		ParentID:    input.ParentID,
		Level:       level,
		Path:        path,
		IsActive:    true,
		SortOrder:   input.SortOrder,
		Metadata:    cloneMap(input.Metadata),
	}
	return m.repo.CreateMember(ctx, member)
}

func (m *Manager) GetMember(ctx context.Context, id int64) (DimensionMember, error) {
	if id <= 0 {
		return DimensionMember{}, ErrInvalidInput
	}
	return m.repo.GetMemberByID(ctx, id)
}

func (m *Manager) UpdateMember(ctx context.Context, id int64, patch MemberPatch) (DimensionMember, error) {
	if id <= 0 {
		return DimensionMember{}, ErrInvalidInput
	}
	member, err := m.repo.GetMemberByID(ctx, id)
	if err != nil {
		return DimensionMember{}, err
	}

	if patch.Name != nil {
		name := strings.TrimSpace(*patch.Name)
		if name == "" {
			return DimensionMember{}, ErrInvalidInput
		}
		patch.Name = &name
	}

	if patch.ParentIDSet {
		if patch.ParentID != nil && *patch.ParentID == id {
			return DimensionMember{}, ErrInvalidInput
		}

		if patch.ParentID != nil {
			parent, err := m.repo.GetMemberByID(ctx, *patch.ParentID)
			if err != nil {
				return DimensionMember{}, err
			}
			if parent.DimensionID != member.DimensionID {
				return DimensionMember{}, ErrInvalidInput
			}
			level := parent.Level + 1
			path := parent.Path + "/" + member.Code
			patchLevel := level
			patchPath := path
			updated, err := m.repo.UpdateMember(ctx, id, patch)
			if err != nil {
				return DimensionMember{}, err
			}
			updated.Level = patchLevel
			updated.Path = patchPath
			return m.repo.UpdateMember(ctx, id, MemberPatch{
				ParentID:    updated.ParentID,
				ParentIDSet: true,
			})
		}
		member.Level = 1
		member.Path = member.Code
	}

	updated, err := m.repo.UpdateMember(ctx, id, patch)
	if err != nil {
		return DimensionMember{}, err
	}
	if patch.ParentIDSet {
		if patch.ParentID == nil {
			updated.Level = 1
			updated.Path = updated.Code
			lvl := updated.Level
			pth := updated.Path
			_ = lvl
			_ = pth
		}
	}
	return updated, nil
}

func (m *Manager) DeleteMember(ctx context.Context, id int64, cascade bool) error {
	if id <= 0 {
		return ErrInvalidInput
	}
	member, err := m.repo.GetMemberByID(ctx, id)
	if err != nil {
		return err
	}
	_ = member

	parent := id
	children, total, err := m.repo.ListMembers(ctx, MemberQueryOptions{ParentID: &parent})
	if err != nil {
		return err
	}
	if (total > 0 || len(children) > 0) && !cascade {
		return ErrConflict
	}
	if cascade {
		for _, child := range children {
			if err := m.DeleteMember(ctx, child.ID, true); err != nil {
				return err
			}
		}
	}
	return m.repo.DeleteMember(ctx, id)
}

func (m *Manager) ListMembers(ctx context.Context, opts MemberQueryOptions) (PaginatedResult[DimensionMember], error) {
	items, total, err := m.repo.ListMembers(ctx, opts)
	if err != nil {
		return PaginatedResult[DimensionMember]{}, err
	}
	pageSize := opts.Limit
	if pageSize <= 0 {
		pageSize = len(items)
		if pageSize == 0 {
			pageSize = 1
		}
	}
	page := (opts.Offset / pageSize) + 1
	totalPages := (total + pageSize - 1) / pageSize
	if total == 0 {
		totalPages = 0
	}
	return PaginatedResult[DimensionMember]{
		Data:       items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

func (m *Manager) GetMemberTree(ctx context.Context, dimensionID int64, maxLevel *int) ([]TreeNode, error) {
	if dimensionID <= 0 {
		return nil, ErrInvalidInput
	}
	if _, err := m.repo.GetDimensionByID(ctx, dimensionID); err != nil {
		return nil, err
	}

	active := true
	members, _, err := m.repo.ListMembers(ctx, MemberQueryOptions{
		DimensionID: &dimensionID,
		IsActive:    &active,
	})
	if err != nil {
		return nil, err
	}

	nodes := make(map[int64]*TreeNode, len(members))
	for _, member := range members {
		if maxLevel != nil && member.Level > *maxLevel {
			continue
		}
		node := &TreeNode{
			ID:       member.ID,
			Code:     member.Code,
			Name:     member.Name,
			Level:    member.Level,
			Path:     member.Path,
			Children: []TreeNode{},
			IsLeaf:   true,
		}
		nodes[member.ID] = node
	}

	for _, member := range members {
		node, ok := nodes[member.ID]
		if !ok {
			continue
		}
		if member.ParentID != nil {
			if parent, ok := nodes[*member.ParentID]; ok {
				parent.Children = append(parent.Children, *node)
				parent.IsLeaf = false
			}
		}
	}

	roots := make([]TreeNode, 0)
	for _, member := range members {
		node, ok := nodes[member.ID]
		if !ok {
			continue
		}
		if member.ParentID != nil {
			if _, ok := nodes[*member.ParentID]; ok {
				continue
			}
		}
		roots = append(roots, *node)
	}
	return roots, nil
}

func (m *Manager) CreateMappingRule(ctx context.Context, input CreateMappingRuleInput) (MappingRule, error) {
	company := strings.TrimSpace(input.Company)
	ruleName := strings.TrimSpace(input.RuleName)
	dimCode := strings.TrimSpace(input.DimensionCode)
	memberCode := strings.TrimSpace(input.MemberCode)
	if company == "" || ruleName == "" || dimCode == "" || memberCode == "" {
		return MappingRule{}, ErrInvalidInput
	}

	if _, err := m.repo.GetMappingRuleByName(ctx, company, ruleName); err == nil {
		return MappingRule{}, ErrAlreadyExists
	} else if err != nil && err != ErrNotFound {
		return MappingRule{}, err
	}

	dim, err := m.repo.GetDimensionByCode(ctx, dimCode)
	if err != nil {
		return MappingRule{}, err
	}
	if _, err := m.repo.GetMemberByCode(ctx, dim.ID, memberCode); err != nil {
		return MappingRule{}, err
	}

	priority := input.Priority
	if priority == 0 {
		priority = 100
	}
	ratio := input.AllocationRatio
	if ratio <= 0 {
		ratio = 1
	}
	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	rule := MappingRule{
		Company:             company,
		RuleName:            ruleName,
		Priority:            priority,
		AccountCodePattern:  input.AccountCodePattern,
		AccountNamePattern:  input.AccountNamePattern,
		SummaryPattern:      input.SummaryPattern,
		CounterpartyPattern: input.CounterpartyPattern,
		DimensionCode:       dimCode,
		MemberCode:          memberCode,
		AllocationRatio:     ratio,
		ValidFrom:           input.ValidFrom,
		ValidTo:             input.ValidTo,
		IsActive:            isActive,
	}
	return m.repo.CreateMappingRule(ctx, rule)
}

func (m *Manager) UpdateMappingRule(ctx context.Context, id int64, patch MappingRulePatch) (MappingRule, error) {
	if id <= 0 {
		return MappingRule{}, ErrInvalidInput
	}
	current, err := m.repo.GetMappingRuleByID(ctx, id)
	if err != nil {
		return MappingRule{}, err
	}

	dimCode := current.DimensionCode
	if patch.DimensionCode != nil {
		dimCode = strings.TrimSpace(*patch.DimensionCode)
		if dimCode == "" {
			return MappingRule{}, ErrInvalidInput
		}
		patch.DimensionCode = &dimCode
	}
	memberCode := current.MemberCode
	if patch.MemberCode != nil {
		memberCode = strings.TrimSpace(*patch.MemberCode)
		if memberCode == "" {
			return MappingRule{}, ErrInvalidInput
		}
		patch.MemberCode = &memberCode
	}

	dim, err := m.repo.GetDimensionByCode(ctx, dimCode)
	if err != nil {
		return MappingRule{}, err
	}
	if _, err := m.repo.GetMemberByCode(ctx, dim.ID, memberCode); err != nil {
		return MappingRule{}, err
	}
	if patch.RuleName != nil {
		name := strings.TrimSpace(*patch.RuleName)
		if name == "" {
			return MappingRule{}, ErrInvalidInput
		}
		patch.RuleName = &name
	}
	if patch.AllocationRatio != nil && *patch.AllocationRatio <= 0 {
		return MappingRule{}, ErrInvalidInput
	}

	return m.repo.UpdateMappingRule(ctx, id, patch)
}

func (m *Manager) DeleteMappingRule(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrInvalidInput
	}
	return m.repo.DeleteMappingRule(ctx, id)
}

func (m *Manager) ListMappingRules(ctx context.Context, opts MappingRuleQueryOptions) (PaginatedResult[MappingRule], error) {
	items, total, err := m.repo.ListMappingRules(ctx, opts)
	if err != nil {
		return PaginatedResult[MappingRule]{}, err
	}
	pageSize := opts.Limit
	if pageSize <= 0 {
		pageSize = len(items)
		if pageSize == 0 {
			pageSize = 1
		}
	}
	page := (opts.Offset / pageSize) + 1
	totalPages := (total + pageSize - 1) / pageSize
	if total == 0 {
		totalPages = 0
	}
	return PaginatedResult[MappingRule]{
		Data:       items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

func (m *Manager) BuildExportPackage(ctx context.Context) (ExportDataPackage, error) {
	dims, _, err := m.repo.ListDimensions(ctx, DimensionQueryOptions{})
	if err != nil {
		return ExportDataPackage{}, err
	}
	members := make(map[string][]MemberExport, len(dims))
	dimExports := make([]DimensionExport, 0, len(dims))
	for _, dim := range dims {
		dimExports = append(dimExports, DimensionExport{
			Code:           dim.Code,
			Name:           dim.Name,
			Type:           dim.Type,
			Description:    dim.Description,
			IsHierarchical: dim.IsHierarchical,
			IsActive:       dim.IsActive,
		})
		dimID := dim.ID
		items, _, err := m.repo.ListMembers(ctx, MemberQueryOptions{DimensionID: &dimID})
		if err != nil {
			return ExportDataPackage{}, err
		}
		memberExports := make([]MemberExport, 0, len(items))
		codeByID := make(map[int64]string, len(items))
		for _, item := range items {
			codeByID[item.ID] = item.Code
		}
		for _, item := range items {
			var parentCode *string
			if item.ParentID != nil {
				if code, ok := codeByID[*item.ParentID]; ok {
					c := code
					parentCode = &c
				}
			}
			memberExports = append(memberExports, MemberExport{
				Code:       item.Code,
				Name:       item.Name,
				ParentCode: parentCode,
				Level:      item.Level,
				Path:       item.Path,
				IsActive:   item.IsActive,
				SortOrder:  item.SortOrder,
			})
		}
		members[dim.Code] = memberExports
	}

	rules, _, err := m.repo.ListMappingRules(ctx, MappingRuleQueryOptions{})
	if err != nil {
		return ExportDataPackage{}, err
	}
	ruleExports := make([]MappingRuleExport, 0, len(rules))
	for _, rule := range rules {
		ruleExports = append(ruleExports, MappingRuleExport{
			ID:                  &rule.ID,
			Company:             rule.Company,
			RuleName:            rule.RuleName,
			Priority:            rule.Priority,
			AccountCodePattern:  rule.AccountCodePattern,
			AccountNamePattern:  rule.AccountNamePattern,
			SummaryPattern:      rule.SummaryPattern,
			CounterpartyPattern: rule.CounterpartyPattern,
			DimensionCode:       rule.DimensionCode,
			MemberCode:          rule.MemberCode,
			AllocationRatio:     rule.AllocationRatio,
			ValidFrom:           rule.ValidFrom,
			ValidTo:             rule.ValidTo,
			IsActive:            rule.IsActive,
		})
	}

	return ExportDataPackage{
		Version:      "1.0",
		ExportedAt:   time.Now().UTC(),
		Dimensions:   dimExports,
		Members:      members,
		MappingRules: ruleExports,
	}, nil
}
