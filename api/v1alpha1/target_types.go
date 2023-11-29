// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TargetType defines valid types for Targets.
type TargetType string

var (
	// Kubernetes defines a Kubernetes target type that uses a KubeConfig for target access.
	Kubernetes TargetType = "kubernetes"
	// SSH defines a remote machine target type that uses SSH access for target access.
	SSH TargetType = "ssh"
	// OCIRepository defines an oci repository target type that uses secrets for target access.
	OCIRepository TargetType = "ocirepository"
)

// TargetSpec defines the desired state of Target.
type TargetSpec struct {
	// +required
	Type TargetType `json:"type"`

	// +optional
	Access *apiextensionsv1.JSON `json:"access,omitempty"`

	// Interval is the reconciliation interval, i.e. at what interval shall a reconciliation happen.
	// This is used to requeue objects for reconciliation in case the related subscription hasn't been finished yet.
	// +required
	Interval metav1.Duration `json:"interval"`

	// ServiceAccountName is the name of the ServiceAccount to be created in the target namespace.
	// +optional
	ServiceAccountName string `json:"serviceAccountName"`

	// selector is a label query over secrets that should be used for target access.
	// If found, the secrets will added to the target ServiceAccount as image pull secrets.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors
	// +optional
	SecretsSelector *metav1.LabelSelector `json:"selector,omitempty"`
}

// TargetStatus defines the observed state of Target.
type TargetStatus struct {
	// ObservedGeneration is the last reconciled generation.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status",description=""
	// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].message",description=""
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// GetConditions returns the conditions of the ComponentVersion.
func (in *Target) GetConditions() []metav1.Condition {
	return in.Status.Conditions
}

// SetConditions sets the conditions of the ComponentVersion.
func (in *Target) SetConditions(conditions []metav1.Condition) {
	in.Status.Conditions = conditions
}

// GetRequeueAfter returns the duration after which the ComponentVersion must be
// reconciled again.
func (in Target) GetRequeueAfter() time.Duration {
	return in.Spec.Interval.Duration
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Target is the Schema for the targets API.
type Target struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TargetSpec   `json:"spec,omitempty"`
	Status TargetStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TargetList contains a list of Target.
type TargetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Target `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Target{}, &TargetList{})
}
