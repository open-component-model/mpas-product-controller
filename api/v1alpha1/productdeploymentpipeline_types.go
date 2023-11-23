// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"github.com/fluxcd/pkg/apis/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProductDeploymentPipelineSpec defines the desired state of ProductDeploymentPipeline
type ProductDeploymentPipelineSpec struct {
	// ComponentVersionRef is the name of the generated component version object.
	// +required
	ComponentVersionRef string `json:"componentVersionRef"`
	// +required
	Resource ResourceReference `json:"resource"`
	// +required
	Localization ResourceReference `json:"localization"`
	// +required
	Configuration Configuration `json:"configuration"`
	// +required
	TargetRole TargetRole `json:"targetRole"`
	// +required
	ConfigMapRef string `json:"configMapRef,omitempty"`

	//+optional
	TargetRef meta.NamespacedObjectReference `json:"targetRef"`
}

// ProductDeploymentPipelineStatus defines the observed state of ProductDeploymentPipeline
type ProductDeploymentPipelineStatus struct {
	// ObservedGeneration is the last reconciled generation.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status",description=""
	// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].message",description=""
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// SelectedTarget defines the target in which this pipeline was deployed into.
	// +optional
	SelectedTargetRef *meta.NamespacedObjectReference `json:"targetRef,omitempty"`

	// SnapshotRef references the last snapshot that this pipeline has eventually produced once completed.
	// +optional
	SnapshotRef *meta.NamespacedObjectReference `json:"snapshotRef,omitempty"`
}

// GetConditions returns the conditions of the ComponentVersion.
func (in *ProductDeploymentPipeline) GetConditions() []metav1.Condition {
	return in.Status.Conditions
}

// SetConditions sets the conditions of the ComponentVersion.
func (in *ProductDeploymentPipeline) SetConditions(conditions []metav1.Condition) {
	in.Status.Conditions = conditions
}

func (in *ProductDeploymentPipeline) GetVID() map[string]string {
	metadata := make(map[string]string)
	metadata[GroupVersion.Group+"/product_deployment_pipeline"] = in.Name

	return metadata
}

func (in *ProductDeploymentPipeline) SetObservedGeneration(v int64) {
	in.Status.ObservedGeneration = v
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ProductDeploymentPipeline is the Schema for the productdeploymentpipelines API
type ProductDeploymentPipeline struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProductDeploymentPipelineSpec   `json:"spec,omitempty"`
	Status ProductDeploymentPipelineStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ProductDeploymentPipelineList contains a list of ProductDeploymentPipeline
type ProductDeploymentPipelineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProductDeploymentPipeline `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ProductDeploymentPipeline{}, &ProductDeploymentPipelineList{})
}
