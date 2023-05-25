// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import "github.com/fluxcd/pkg/apis/meta"

// KubernetesAccess defines a type that contains access information for Kubernetes clusters.
type KubernetesAccess struct {
	SecretRef       *meta.NamespacedObjectReference `yaml:"secretRef"`
	TargetNamespace string                          `yaml:"targetNamespace"`
}
