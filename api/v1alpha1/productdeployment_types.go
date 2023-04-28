// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"github.com/open-component-model/ocm/pkg/contexts/ocm/compdesc/versions/ocm.software/v3alpha1"
	replicationv1 "github.com/open-component-model/replication-controller/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProductDeploymentSpec defines the desired state of ProductDeployment.
type ProductDeploymentSpec struct {
	Component replicationv1.Component `json:"component"`
	Pipelines []Pipeline              `json:"pipelines"`
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

// Localization defines a list of rules which are named versions.
type Localization struct {
	Rules v3alpha1.ElementMeta `json:"rules"`
}

// ValuesFile defines a path to a values file containing User configuration.
type ValuesFile struct {
	Path string `json:"path"`
}

// Configuration defines a list of rules to follow and an optional values file.
type Configuration struct {
	Rules v3alpha1.ElementMeta `json:"rules"`

	//+optional
	ValuesFile ValuesFile `json:"valuesFile,omitempty"`
}

// TargetRole the role defining what targets are available to deploy to.
type TargetRole struct {
	Type     string               `json:"type"`
	Selector metav1.LabelSelector `json:"selector"`
}

// Pipeline defines a set of Loca
type Pipeline struct {
	Name          string               `json:"name"`
	Resource      v3alpha1.ElementMeta `json:"resource"`
	Localization  Localization         `json:"localization"`
	Configuration Configuration        `json:"configuration"`
	TargetRole    TargetRole           `json:"targetRole"`
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
