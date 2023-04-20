// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ProductDeploymentSpec defines the desired state of ProductDeployment
type ProductDeploymentSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of ProductDeployment. Edit productdeployment_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// ProductDeploymentStatus defines the observed state of ProductDeployment
type ProductDeploymentStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ProductDeployment is the Schema for the productdeployments API
type ProductDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProductDeploymentSpec   `json:"spec,omitempty"`
	Status ProductDeploymentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ProductDeploymentList contains a list of ProductDeployment
type ProductDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProductDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ProductDeployment{}, &ProductDeploymentList{})
}
