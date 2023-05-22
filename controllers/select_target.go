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
func FilterTarget(ctx context.Context, c client.Client, role v1alpha1.TargetRole, namespace string) (*v1alpha1.Target, error) {
	targetList := &v1alpha1.TargetList{}

	m, err := v1.LabelSelectorAsMap(&role.Selector)
	if err != nil {
		return nil, fmt.Errorf("failed to parse label selectors to map: %w", err)
	}

	if err := c.List(ctx, targetList, client.InNamespace(namespace), client.MatchingLabels(m)); err != nil {
		return nil, fmt.Errorf("failed to list targets: %w", err)
	}

	switch len(targetList.Items) {
	case 0:
		return nil, fmt.Errorf("no targets found using the provided selector filters")
	case 1:
		return &targetList.Items[0], nil
	default:
		r := rand.New(rand.NewSource(time.Now().Unix()))
		index := r.Intn(len(targetList.Items))
		return &targetList.Items[index], nil
	}
}
