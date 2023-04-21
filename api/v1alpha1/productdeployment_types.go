// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"github.com/fluxcd/pkg/apis/meta"
	replicationv1 "github.com/open-component-model/replication-controller/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TODO: This might be something else with an optional identity and ref?
type Resource struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Rules struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Localization struct {
	Rules []Rules `json:"rules"`
}

type ValuesFile struct {
	Path string `json:"path"`
}

type Configuration struct {
	Rules      []Rules    `json:"rules"`
	ValuesFile ValuesFile `json:"valuesFile"`
}

type TargetRole struct {
	Type     string               `json:"type"`
	Selector metav1.LabelSelector `json:"selector"`
}

type Pipelines struct {
	Name          string        `json:"name"`
	Resource      Resource      `json:"resource"`
	Localization  Localization  `json:"localization"`
	Configuration Configuration `json:"configuration"`
	TargetRole    TargetRole    `json:"targetRole"`
}

// ProductDeploymentSpec defines the desired state of ProductDeployment.
type ProductDeploymentSpec struct {
	Component replicationv1.Component `json:"component"`
	Pipelines []Pipelines             `json:"pipelines"`

	//+optional
	TargetRef *meta.NamespacedObjectReference `json:"targetRef,omitempty"`
}

// ProductDeploymentStatus defines the observed state of ProductDeployment.
type ProductDeploymentStatus struct {
	// ObservedGeneration is the last reconciled generation.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status",description=""
	// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].message",description=""
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// GetConditions returns the conditions of the ComponentVersion.
func (in *ProductDeployment) GetConditions() []metav1.Condition {
	return in.Status.Conditions
}

// SetConditions sets the conditions of the ComponentVersion.
func (in *ProductDeployment) SetConditions(conditions []metav1.Condition) {
	in.Status.Conditions = conditions
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
