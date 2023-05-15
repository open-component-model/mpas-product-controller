// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

const (
	// ComponentSubscriptionGetFailedReason is used when the subscription object cannot be found.
	ComponentSubscriptionGetFailedReason = "ComponentSubscriptionGetFailed"

	// ProjectInNamespaceGetFailedReason is used when the Project in a given namespace has not been found.
	ProjectInNamespaceGetFailedReason = "ProjectInNamespaceGetFailed"

	// ComponentVersionGetFailedReason is used when the subscription object cannot be found.
	ComponentVersionGetFailedReason = "ComponentVersionGetFailed"

	// OCMAuthenticationFailedReason happens when we fail to load authentication for OCM repositories.
	OCMAuthenticationFailedReason = "OCMAuthenticationFailed"

	// ProductDescriptionGetFailedReason happens when we don't find the product description or fail to process it.
	ProductDescriptionGetFailedReason = "ProductDescriptionGetFailed"

	// CreateProductPipelineFailedReason is used when we fail to create a product pipeline base on a product description.
	CreateProductPipelineFailedReason = "CreateProductPipelineFailed"
	// CreateSyncFailedReason is used when we fail to create a git-controller.Sync object in the cluster.
	CreateSyncFailedReason = "CreateSyncFailed"

	// CreateSnapshotFailedReason is used when we fail to create an ocm-controller.Snapshot object in the cluster.
	CreateSnapshotFailedReason = "CreateSnapshotFailed"

	// NumberOfProductDescriptionsInComponentIncorrectReason is used when there are 0 or more than 1 product
	// descriptions in a component.
	NumberOfProductDescriptionsInComponentIncorrectReason = "NumberOfProductDescriptionsInComponentIncorrect"
)
