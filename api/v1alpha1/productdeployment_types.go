// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"time"

	ocmv1 "github.com/open-component-model/ocm-controller/api/v1alpha1"
	ocmmetav1 "github.com/open-component-model/ocm/pkg/contexts/ocm/compdesc/meta/v1"
	replicationv1 "github.com/open-component-model/replication-controller/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ProductDeploymentNameKey       = "product-deployment-name"
	ProductDeploymentKind          = "ProductDeployment"
	ProductDeploymentOwnerLabelKey = "mpas.ocm.system/product-deployment"
)

// ProductDeploymentSpec defines the desired state of ProductDeployment.
type ProductDeploymentSpec struct {
	// +required
	Component replicationv1.Component `json:"component"`
	// +required
	Pipelines []Pipeline `json:"pipelines"`
	// +required
	ServiceAccountName string `json:"serviceAccountName"`
	// Interval is the reconciliation interval, i.e. at what interval shall a reconciliation happen.
	// +required
	Interval metav1.Duration `json:"interval"`
	// +required
	Schema []byte `json:"schema"`
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

	// ActivePipelines has all the pipelines which are all still running.
	// +optional
	ActivePipelines []string `json:"activePipelines,omitempty"`
}

// IsComplete returns if the deployment has finished running all pipelines.
func (in *ProductDeployment) IsComplete() bool {
	return len(in.Status.ActivePipelines) == 0
}

type ResourceReference struct {
	// +required
	ocmv1.ElementMeta `json:",inline"`

	// +optional
	ReferencePath []ocmmetav1.Identity `json:"referencePath,omitempty"`
}

// Configuration defines a list of rules to follow and an optional values file.
type Configuration struct {
	Rules ResourceReference `json:"rules"`
}

// TargetRole the role defining what targets are available to deploy to.
type TargetRole struct {
	Type     TargetType           `json:"type"`
	Selector metav1.LabelSelector `json:"selector"`
}

// Pipeline defines a set of pipeline objects.
type Pipeline struct {
	Name          string            `json:"name"`
	Resource      ResourceReference `json:"resource"`
	Localization  ResourceReference `json:"localization"`
	Configuration Configuration     `json:"configuration"`
	TargetRole    TargetRole        `json:"targetRole"`
}

// GetConditions returns the conditions of the ComponentVersion.
func (in *ProductDeployment) GetConditions() []metav1.Condition {
	return in.Status.Conditions
}

// SetConditions sets the conditions of the ComponentVersion.
func (in *ProductDeployment) SetConditions(conditions []metav1.Condition) {
	in.Status.Conditions = conditions
}

func (in *ProductDeployment) GetVID() map[string]string {
	metadata := make(map[string]string)
	metadata[GroupVersion.Group+"/product_deployment"] = in.Name

	return metadata
}

// GetRequeueAfter returns the duration after which the ProductDeployment must be
// reconciled again.
func (in ProductDeployment) GetRequeueAfter() time.Duration {
	return in.Spec.Interval.Duration
}

func (in *ProductDeployment) SetObservedGeneration(v int64) {
	in.Status.ObservedGeneration = v
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
