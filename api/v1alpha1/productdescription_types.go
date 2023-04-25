// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProductDescriptionSpec defines the desired state of ProductDescription
type ProductDescriptionSpec struct {
	Description string                       `json:"description"`
	Pipelines   []ProductDescriptionPipeline `json:"pipelines"`

	//+optional
	TargetRoles []TargetRoles `json:"targetRoles,omitempty"`
}

// ProductDescriptionStatus defines the observed state of ProductDescription
type ProductDescriptionStatus struct {
}

// TargetRoles defines a target role with a name.
type TargetRoles struct {
	Name       string `json:"name"`
	TargetRole `json:",inline"`
}

type ProductDescriptionPipeline struct {
	Name       string `json:"name"`
	Source     Ref    `json:"source"`
	Validation Ref    `json:"validation"`

	//+optional
	TargetRoleName string `json:"targetRoleName,omitempty"`
	//+optional
	Localization Ref `json:"localization,omitempty"`
	//+optional
	Configuration Ref `json:"configuration,omitempty"`
}

type ProductDescriptionPipelineConfiguration struct {
	Rules  Ref `json:"rules"`
	Readme Ref `json:"readme"`
}

// Ref describes a resource that could be referenced by a component version.
type Ref struct {
	Name    string `json:"name"`
	Version string `json:"version"`

	// ComponentReference is used to fetch the given resource if defined instead
	// of the parent component of this object.
	//+optional
	ComponentReference v1.LocalObjectReference `json:"componentReference,omitempty"`
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
