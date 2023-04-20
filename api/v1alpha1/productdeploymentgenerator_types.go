// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ProductDeploymentGeneratorSpec defines the desired state of ProductDeploymentGenerator
type ProductDeploymentGeneratorSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of ProductDeploymentGenerator. Edit productdeploymentgenerator_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// ProductDeploymentGeneratorStatus defines the observed state of ProductDeploymentGenerator
type ProductDeploymentGeneratorStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ProductDeploymentGenerator is the Schema for the productdeploymentgenerators API
type ProductDeploymentGenerator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProductDeploymentGeneratorSpec   `json:"spec,omitempty"`
	Status ProductDeploymentGeneratorStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ProductDeploymentGeneratorList contains a list of ProductDeploymentGenerator
type ProductDeploymentGeneratorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProductDeploymentGenerator `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ProductDeploymentGenerator{}, &ProductDeploymentGeneratorList{})
}
