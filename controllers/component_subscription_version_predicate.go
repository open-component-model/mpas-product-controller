// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"github.com/open-component-model/replication-controller/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ComponentSubscriptionVersionChangedPredicate watches a subscription for reconciled version changes.
type ComponentSubscriptionVersionChangedPredicate struct {
	predicate.Funcs
}

// Update will check the new subscription has a new LastAppliedVersion. If yes, it will trigger a reconcile event.
func (ComponentSubscriptionVersionChangedPredicate) Update(e event.UpdateEvent) bool {
	if e.ObjectOld == nil || e.ObjectNew == nil {
		return false
	}

	oldComponentVersion, ok := e.ObjectOld.(*v1alpha1.ComponentSubscription)
	if !ok {
		return false
	}

	newComponentVersion, ok := e.ObjectNew.(*v1alpha1.ComponentSubscription)
	if !ok {
		return false
	}

	if oldComponentVersion.Status.LastAppliedVersion == "" && newComponentVersion.Status.LastAppliedVersion != "" {
		return true
	}

	if oldComponentVersion.Status.LastAppliedVersion != "" && newComponentVersion.Status.LastAppliedVersion != "" &&
		(oldComponentVersion.Status.LastAppliedVersion != newComponentVersion.Status.LastAppliedVersion) {
		return true
	}

	return false
}
