package controllers

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/open-component-model/mpas-product-controller/api/v1alpha1"
)

// FilterTarget selects a target based on the provided filtering options.
func FilterTarget(ctx context.Context, c client.Client, role v1alpha1.TargetRole, namespace string) (v1alpha1.Target, error) {
	targetList := &v1alpha1.TargetList{}

	m, err := v1.LabelSelectorAsSelector(&role.Selector)
	if err != nil {
		return v1alpha1.Target{}, fmt.Errorf("failed to parse label selectors to map: %w", err)
	}

	// we can't use client.MatchingFields for spec.type, because we don't have spec.type registered.
	// maybe we should and the above range could be avoided?
	if err := c.List(ctx, targetList, client.InNamespace(namespace), client.MatchingLabelsSelector{
		Selector: m,
	}); err != nil {
		return v1alpha1.Target{}, fmt.Errorf("failed to list targets: %w", err)
	}

	var targets []v1alpha1.Target
	for _, target := range targetList.Items {
		if target.Spec.Type == role.Type {
			targets = append(targets, target)
		}
	}

	switch len(targets) {
	case 0:
		return v1alpha1.Target{}, fmt.Errorf("no targets found using the provided selector filters")
	case 1:
		return targets[0], nil
	default:
		r := rand.New(rand.NewSource(time.Now().Unix()))
		index := r.Intn(len(targets))
		return targets[index], nil
	}
}
