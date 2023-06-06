package rego

import (
	"context"
	"fmt"

	"github.com/open-component-model/mpas-product-controller/api/v1alpha1"
	"github.com/open-policy-agent/opa/rego"
)

// ValidateRules validates values data based on a given set of Rego rules.
func ValidateRules(ctx context.Context, rule v1alpha1.ValidationData, valuesData map[string]any) (bool, error) {
	valuesPart, ok := valuesData[rule.Name]
	if !ok {
		return false, fmt.Errorf("no values found for rule with name %s", rule.Name)
	}

	query, err := rego.New(
		rego.Query("x = data.main.deny"),
		rego.Module("validation.rego", string(rule.Data)),
	).PrepareForEval(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to parse validation content: %w", err)
	}

	results, err := query.Eval(ctx, rego.EvalInput(valuesPart))
	if err != nil {
		return false, fmt.Errorf("failed to evaluate values data: %w", err)
	}

	if len(results) == 0 {
		return false, fmt.Errorf("there were no results in the validation result set")
	}

	bindingsAny, ok := results[0].Bindings["x"]
	if !ok {
		return false, fmt.Errorf("x not found in bindings: %+v", results[0].Bindings)
	}

	bindings, ok := bindingsAny.([]any)
	if !ok {
		return false, fmt.Errorf("bindings wasn't a list of values but was: %+v", bindings)
	}

	// if there are no reg bindings, that means that the rules succeeded.
	return len(bindings) == 0, nil
}
