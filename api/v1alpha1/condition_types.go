// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

const (
	// ComponentSubscriptionGetFailedReason is used when the subscription object cannot be found.
	ComponentSubscriptionGetFailedReason = "ComponentSubscriptionGetFailed"

	// ComponentSubscriptionNotReadyReason is used when the subscription object cannot be found.
	ComponentSubscriptionNotReadyReason = "ComponentSubscriptionNotReady"

	// SemverParseFailedReason is used when we failed to parse a semver.
	SemverParseFailedReason = "SemverParseFailed"

	// ProjectInNamespaceGetFailedReason is used when the Project in a given namespace has not been found.
	ProjectInNamespaceGetFailedReason = "ProjectInNamespaceGetFailed"

	// ProjectNotReadyReason is used when the Project in a given namespace has not been found.
	ProjectNotReadyReason = "ProjectNotReady"

	// RepositoryInformationMissingReason is used when the Project in a given namespace has not been found.
	RepositoryInformationMissingReason = "RepositoryInformationMissing"

	// ComponentVersionGetFailedReason is used when the subscription object cannot be found.
	ComponentVersionGetFailedReason = "ComponentVersionGetFailed"

	// OCMAuthenticationFailedReason happens when we fail to load authentication for OCM repositories.
	OCMAuthenticationFailedReason = "OCMAuthenticationFailed"

	// ProductDescriptionGetFailedReason happens when we don't find the product description or fail to process it.
	ProductDescriptionGetFailedReason = "ProductDescriptionGetFailed"

	// TemporaryFolderGenerationFailedReason happens when we don't find the product description or fail to process it.
	TemporaryFolderGenerationFailedReason = "TemporaryFolderGenerationFailed"

	// CreateProductPipelineFailedReason is used when we fail to create a product pipeline base on a product description.
	CreateProductPipelineFailedReason = "CreateProductPipelineFailed"

	// ProductPipelineSchedulingFailedReason is used when we one or more pipelines cannot be scheduled because they don't have a target.
	ProductPipelineSchedulingFailedReason = "ProductPipelineSchedulingFailed"

	// CreateSyncFailedReason is used when we fail to create a git-controller.Sync object in the cluster.
	CreateSyncFailedReason = "CreateSyncFailed"

	// CreateValidationFailedReason is used when we fail to create a Validation object in the cluster.
	CreateValidationFailedReason = "CreateValidationFailed"

	// CreateSnapshotFailedReason is used when we fail to create an ocm-controller.Snapshot object in the cluster.
	CreateSnapshotFailedReason = "CreateSnapshotFailed"

	// CommitTemplateEmptyReason is used when a the commit template is not set.
	CommitTemplateEmptyReason = "CommitTemplateEmpty"

	// CreateComponentVersionFailedReason is used when we fail to create an ocm-controller.ComponentVersion object in the cluster.
	CreateComponentVersionFailedReason = "CreateComponentVersionFailed"

	// CreateLocalizationFailedReason is used when we fail to create an ocm-controller.Localization object in the cluster.
	CreateLocalizationFailedReason = "CreateLocalizationFailed"

	// CreateConfigurationFailedReason is used when we fail to create an ocm-controller.Configuration object in the cluster.
	CreateConfigurationFailedReason = "CreateConfigurationFailed"

	// CreateOCIRepositoryFailedReason is used when we fail to create an OCIRepository object in the cluster.
	CreateOCIRepositoryFailedReason = "CreateOCIRepositoryFailed"

	// PipelineDeploymentFailedReason is used when we fail to deploy a pipeline.
	PipelineDeploymentFailedReason = "PipelineDeploymentFailed"

	// PipelineTargetSelectionFailedReason is used when we fail to select a target environment for the pipeline.
	PipelineTargetSelectionFailedReason = "PipelineTargetSelectionFailed"

	// ValidationFailedReason is used when the validation of a resource failed.
	ValidationFailedReason = "ValidationFailed"

	// SchemaGenerationFailedReason is used when we couldn't generate the schema.
	SchemaGenerationFailedReason = "SchemaGenerationFailed"

	// GitRepositoryCleanUpFailedReason is used when we couldn't delete the GitRepository.
	GitRepositoryCleanUpFailedReason = "GitRepositoryCleanUpFailed"

	// NamespaceCreateOrUpdateFailedReason indicates that the project namespace could not be reconciled.
	NamespaceCreateOrUpdateFailedReason string = "NamespaceCreateOrUpdateFailed"

	// SecretRetrievalFailedReason indicates that the project namespace could not be retrieved from the namespace.
	SecretRetrievalFailedReason = "SecretRetrievalFailed"

	// ServiceAccountCreateOrUpdateFailedReason indicates that the project service account could not be reconciled.
	ServiceAccountCreateOrUpdateFailedReason string = "ServiceAccountCreateOrUpdateFailed"

	// AccessInvalidReason indicates that the access credentials or target are invalid.
	AccessInvalidReason = "AccessInvalid"

	// CreateConfigMapFailedReason indicates that the config map could not be created.
	CreateConfigMapFailedReason = "CreateConfigMapFailed"

	// ConfigRetrievalFailedReason indicates that the config file could not be retrieved.
	ConfigRetrievalFailedReason = "ConfigRetrievalFailed"

	// SchemaRetrievalFailedReason indicates that the schema file could not be retrieved.
	SchemaRetrievalFailedReason = "SchemaRetrievalFailed"

	// ConfigValidationFailedReason indicates that the config file could not be validated.
	ConfigValidationFailedReason = "ConfigValidationFailed"
)

const (
	// DeployedCondition defines the condition when a Pipeline object has successfully been deployed.
	DeployedCondition = "Deployed"
)
