// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
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

// TargetSpec defines the desired state of Target
type TargetSpec struct {
	// +required
	Type TargetType `json:"type"`
	// +optional
	Access *apiextensionsv1.JSON `json:"access,omitempty"`
}

// TargetStatus defines the observed state of Target
type TargetStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Target is the Schema for the targets API
type Target struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TargetSpec   `json:"spec,omitempty"`
	Status TargetStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TargetList contains a list of Target
type TargetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Target `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Target{}, &TargetList{})
}
