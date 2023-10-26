// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProductDescriptionSpec defines the desired state of ProductDescription
type ProductDescriptionSpec struct {
	// +required
	Description string `json:"description"`
	// +required
	Pipelines []ProductDescriptionPipeline `json:"pipelines"`

	//+optional
	TargetRoles []TargetRoles `json:"targetRoles,omitempty"`
}

// ProductDescriptionStatus defines the observed state of ProductDescription
type ProductDescriptionStatus struct{}

// TargetRoles defines a target role with a name.
type TargetRoles struct {
	// +required
	Name       string `json:"name"`
	TargetRole `json:",inline"`
}

// ProductDescriptionPipeline defines the details for a pipeline item.
type ProductDescriptionPipeline struct {
	// +required
	Name string `json:"name"`
	// +required
	Source ResourceReference `json:"source"`
	//+optional
	TargetRoleName string `json:"targetRoleName,omitempty"`
	//+optional
	Localization ResourceReference `json:"localization,omitempty"`
	//+optional
	Configuration DescriptionConfiguration `json:"configuration,omitempty"`
	//+optional
	Schema ResourceReference `json:"schema,omitempty"`
}

// DescriptionConfiguration contains details one parsing configuration items in a project description.
type DescriptionConfiguration struct {
	// +required
	Rules ResourceReference `json:"rules"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ProductDescription is the Schema for the productdescriptions API
type ProductDescription struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProductDescriptionSpec   `json:"spec,omitempty"`
	Status ProductDescriptionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ProductDescriptionList contains a list of ProductDescription
type ProductDescriptionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProductDescription `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ProductDescription{}, &ProductDescriptionList{})
}
