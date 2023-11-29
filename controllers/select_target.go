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
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/open-component-model/mpas-product-controller/api/v1alpha1"
)

// SelectTarget selects a target based on the provided filtering options.
func (r *ProductDeploymentPipelineScheduler) SelectTarget(ctx context.Context, role v1alpha1.TargetRole, namespace string) (v1alpha1.Target, error) {
	logger := log.FromContext(ctx)

	logger.Info("searching for targets in namespace", "namespace", namespace)
	targetList, err := r.searchForTargetsInNamespace(ctx, role.Selector, namespace)
	if err != nil {
		return v1alpha1.Target{}, fmt.Errorf("failed to search for targets in namespace %s: %w", namespace, err)
	}

	if len(targetList.Items) == 0 {
		logger.Info("no targets found in given namespace; searching for targets in mpas-system")
		targetList, err = r.searchForTargetsInNamespace(ctx, role.Selector, r.MpasSystemNamespace)
		if err != nil {
			return v1alpha1.Target{}, fmt.Errorf("failed to search for targets in mpas system namespace: %w", err)
		}
	}

	logger.Info("filtering targets based on criteria")
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
		r := rand.New(rand.NewSource(time.Now().Unix())) //nolint:gosec // good enough
		index := r.Intn(len(targets))

		return targets[index], nil
	}
}

func (r *ProductDeploymentPipelineScheduler) searchForTargetsInNamespace(
	ctx context.Context,
	selector v1.LabelSelector,
	namespace string,
) (*v1alpha1.TargetList, error) {
	targetList := &v1alpha1.TargetList{}
	m, err := v1.LabelSelectorAsSelector(&selector)
	if err != nil {
		return nil, fmt.Errorf("failed to parse label selectors to map: %w", err)
	}

	// We can't use client.MatchingFields for spec.type since we don't have a controller for Target
	// and thus, we can't index the type field.
	//nolint:godox // yep
	//TODO: post MVP add controller for targets
	if err := r.List(ctx, targetList, client.InNamespace(namespace), client.MatchingLabelsSelector{
		Selector: m,
	}); err != nil {
		return nil, fmt.Errorf("failed to list targets: %w", err)
	}

	return targetList, nil
}
