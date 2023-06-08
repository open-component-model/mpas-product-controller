// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ValidationSpec defines the desired state of Validation
// Fetches pull request ID and GitRepository from the Sync object.
type ValidationSpec struct {
	// ValidationRules points to the snapshot containing the rules
	// +required
	ValidationRules []ValidationData `json:"validationRules"`
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
	// ObservedGeneration is the last reconciled generation.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status",description=""
	// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].message",description=""
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// GitRepositoryRef points to the GitRepository that tracks changes to the values.yaml file.
	// +optional
	GitRepositoryRef *meta.NamespacedObjectReference `json:"gitRepositoryRef,omitempty"`

	// LastValidatedDigest contains the last digest that has been validated.
	// +optional
	LastValidatedDigest string `json:"lastValidatedDigest,omitempty"`

	// LastValidatedDigestOutcome contains the outcome of the last digest's validation. Can be failed or success.
	// +optional
	LastValidatedDigestOutcome ValidationOutcome `json:"lastValidatedDigestOutcome,omitempty"`
}

// ValidationOutcome defines the allowed outcome of a validation.
type ValidationOutcome string

var (
	FailedValidationOutcome  ValidationOutcome = "failed"
	SuccessValidationOutcome ValidationOutcome = "success"
)

// GetConditions returns the conditions of the ComponentVersion.
func (in *Validation) GetConditions() []metav1.Condition {
	return in.Status.Conditions
}

// SetConditions sets the conditions of the ComponentVersion.
func (in *Validation) SetConditions(conditions []metav1.Condition) {
	in.Status.Conditions = conditions
}

// GetRequeueAfter returns the duration after which the ComponentVersion must be
// reconciled again.
func (in Validation) GetRequeueAfter() time.Duration {
	return in.Spec.Interval.Duration
}

// ValidationData contains information about the rules and to which resource they belong to.
type ValidationData struct {
	Data []byte `json:"data"`
	Name string `json:"name"`
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
