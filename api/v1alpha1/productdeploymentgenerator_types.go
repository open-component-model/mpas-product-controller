// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProductDeploymentGeneratorSpec defines the desired state of ProductDeploymentGenerator.
type ProductDeploymentGeneratorSpec struct {
	// Interval is the reconciliation interval, i.e. at what interval shall a reconciliation happen.
	// This is used to requeue objects for reconciliation in case the related subscription hasn't been finished yet.
	// +required
	Interval metav1.Duration `json:"interval"`

	// SubscriptionRef contains the reference to the component subscription from which the ProductDeployment is generated.
	// +required
	SubscriptionRef meta.NamespacedObjectReference `json:"subscriptionRef"`

	//+optional
	RepositoryRef *meta.LocalObjectReference `json:"repositoryRef,omitempty"`

	// ServiceAccountName is used to access ocm component repositories. No other auth option is defined.
	// https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/#add-imagepullsecrets-to-a-service-account
	// +required
	ServiceAccountName string `json:"serviceAccountName"`
}

// ProductDeploymentGeneratorStatus defines the observed state of ProductDeploymentGenerator.
type ProductDeploymentGeneratorStatus struct {
	// ObservedGeneration is the last reconciled generation.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status",description=""
	// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].message",description=""
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// +optional
	LatestSnapshotDigest string `json:"latestSnapshotDigest,omitempty"`

	// +optional
	SnapshotName string `json:"snapshotName,omitempty"`

	// LastReconciledVersion contains the ComponentSubscription's version that was used to generate the ProductDeployment.
	// +optional
	LastReconciledVersion string `json:"lastReconciledVersion,omitempty"`
}

// GetSnapshotDigest returns the latest snapshot digest for the localization.
func (in ProductDeploymentGenerator) GetSnapshotDigest() string {
	return in.Status.LatestSnapshotDigest
}

// GetSnapshotName returns the key for the snapshot produced by the Localization.
func (in ProductDeploymentGenerator) GetSnapshotName() string {
	return in.Status.SnapshotName
}

// GetConditions returns the conditions of the ComponentVersion.
func (in *ProductDeploymentGenerator) GetConditions() []metav1.Condition {
	return in.Status.Conditions
}

// SetConditions sets the conditions of the ComponentVersion.
func (in *ProductDeploymentGenerator) SetConditions(conditions []metav1.Condition) {
	in.Status.Conditions = conditions
}

// GetRequeueAfter returns the duration after which the ComponentVersion must be
// reconciled again.
func (in ProductDeploymentGenerator) GetRequeueAfter() time.Duration {
	return in.Spec.Interval.Duration
}

func (in *ProductDeploymentGenerator) GetVID() map[string]string {
	metadata := make(map[string]string)
	metadata[GroupVersion.Group+"/product_deployment_generator"] = in.Status.LatestSnapshotDigest

	return metadata
}

func (in *ProductDeploymentGenerator) SetObservedGeneration(v int64) {
	in.Status.ObservedGeneration = v
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ProductDeploymentGenerator is the Schema for the productdeploymentgenerators API.
type ProductDeploymentGenerator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProductDeploymentGeneratorSpec   `json:"spec,omitempty"`
	Status ProductDeploymentGeneratorStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ProductDeploymentGeneratorList contains a list of ProductDeploymentGenerator.
type ProductDeploymentGeneratorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProductDeploymentGenerator `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ProductDeploymentGenerator{}, &ProductDeploymentGeneratorList{})
}
