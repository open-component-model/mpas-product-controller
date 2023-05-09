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

	OCMAuthenticationFailedReason = "OCMAuthenticationFailed"

	ProductDescriptionGetFailedReason = "ProductDescriptionGetFailed"

	CreateProductPipelineFailedReason = "CreateProductPipelineFailed"
	CreateSyncFailedReason            = "CreateSyncFailedReason"
	CreateSnapshotFailedReason        = "CreateSnapshotFailedReason"

	NumberOfProductDescriptionsInComponentIncorrectReason = "NumberOfProductDescriptionsInComponentIncorrect"
)
