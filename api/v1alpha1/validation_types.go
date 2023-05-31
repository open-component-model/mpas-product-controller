// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"github.com/fluxcd/pkg/apis/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ValidationSpec defines the desired state of Validation
// Fetches pull request ID and GitRepository from the Sync object.
type ValidationSpec struct {
	// ValidationRules points to the snapshot containing the rules
	// +required
	ValidationRules []ResourceReference `json:"validationRules"`
	// +required
	ServiceAccountName string `json:"serviceAccountName"`
	// +required
	Interval metav1.Duration `json:"interval"`
	// SyncRef references the Sync request that will create the git repository to track the values.yaml file.
	// +required
	SyncRef meta.NamespacedObjectReference `json:"syncRef"`
}

// ValidationStatus defines the observed state of Validation
type ValidationStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Validation is the Schema for the validations API
type Validation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ValidationSpec   `json:"spec,omitempty"`
	Status ValidationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ValidationList contains a list of Validation
type ValidationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Validation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Validation{}, &ValidationList{})
}
