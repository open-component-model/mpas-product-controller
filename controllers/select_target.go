// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

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
func (r *ProductDeploymentPipelineScheduler) FilterTarget(ctx context.Context, role v1alpha1.TargetRole, namespace string) (v1alpha1.Target, error) {
	targetList := &v1alpha1.TargetList{}

	m, err := v1.LabelSelectorAsSelector(&role.Selector)
	if err != nil {
		return v1alpha1.Target{}, fmt.Errorf("failed to parse label selectors to map: %w", err)
	}

	// We can't use client.MatchingFields for spec.type since we don't have a controller for Target
	// and thus, we can't index the type field.
	if err := r.List(ctx, targetList, client.InNamespace(namespace), client.MatchingLabelsSelector{
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
