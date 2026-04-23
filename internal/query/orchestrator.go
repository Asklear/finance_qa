package query

import (
	"context"
	"fmt"
)

type MissingCapabilityError struct {
	Capability SourceCapability
}

func (e *MissingCapabilityError) Error() string {
	return fmt.Sprintf("missing source adapter for capability %s", e.Capability)
}

type Orchestrator struct {
	registry *SourceRegistry
}

func NewOrchestrator(registry *SourceRegistry) *Orchestrator {
	if registry == nil {
		registry = NewSourceRegistry()
	}
	return &Orchestrator{registry: registry}
}

func (o *Orchestrator) Execute(ctx context.Context, spec QuerySpec) (AnswerFrame, error) {
	plan := PlanQuerySpec(spec)
	factSets := make([]FactSet, 0, len(plan.Capabilities))
	executed := map[string]struct{}{}
	for _, capability := range plan.Capabilities {
		adapter, ok := o.registry.Resolve(capability)
		if !ok {
			return AnswerFrame{}, &MissingCapabilityError{Capability: capability}
		}
		if _, seen := executed[adapter.Name()]; seen {
			continue
		}
		factSet, err := adapter.Fetch(ctx, spec)
		if err != nil {
			return AnswerFrame{}, err
		}
		executed[adapter.Name()] = struct{}{}
		factSets = append(factSets, factSet)
	}
	return AnswerFrame{
		Spec:     spec,
		Plan:     plan,
		FactSets: factSets,
	}, nil
}
