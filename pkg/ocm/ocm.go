// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package ocm

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/containers/image/v5/pkg/compression"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/open-component-model/ocm/pkg/contexts/credentials/repositories/dockerconfig"
	"github.com/open-component-model/ocm/pkg/contexts/ocm"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/compdesc"
	ocmmetav1 "github.com/open-component-model/ocm/pkg/contexts/ocm/compdesc/meta/v1"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/repositories/ocireg"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/utils"

	"github.com/open-component-model/mpas-product-controller/api/v1alpha1"
)

const (
	dockerConfigKey = ".dockerconfigjson"

	// ProductDescriptionType defines the type of the ProductDescription resource in the component version.
	ProductDescriptionType = "productdescription.mpas.ocm.software"
)

// Contract defines a subset of capabilities from the OCM library.
type Contract interface {
	CreateAuthenticatedOCMContext(ctx context.Context, serviceAccountName, namespace string) (ocm.Context, error)
	GetComponentVersion(ctx context.Context, octx ocm.Context, url, name, version string) (ocm.ComponentVersionAccess, error)
	GetProductDescription(ctx context.Context, octx ocm.Context, cv ocm.ComponentVersionAccess) ([]byte, error)
	GetResourceData(ctx context.Context, cv ocm.ComponentVersionAccess, ref v1alpha1.ResourceReference) ([]byte, error)
}

// Client implements the OCM fetcher interface.
type Client struct {
	client client.Client
}

var _ Contract = &Client{}

// NewClient creates a new fetcher Client using the provided k8s client.
func NewClient(client client.Client) *Client {
	return &Client{
		client: client,
	}
}

// CreateAuthenticatedOCMContext authenticates the client using a service account.
func (c *Client) CreateAuthenticatedOCMContext(ctx context.Context, serviceAccountName, namespace string) (ocm.Context, error) {
	octx := ocm.New()

	if err := c.configureServiceAccountAccess(ctx, octx, serviceAccountName, namespace); err != nil {
		return nil, fmt.Errorf("failed to configure service account access: %w", err)
	}

	return octx, nil
}

func (c *Client) configureServiceAccountAccess(ctx context.Context, octx ocm.Context, serviceAccountName, namespace string) error {
	logger := log.FromContext(ctx)

	logger.V(4).Info("configuring service account credentials")
	account := &corev1.ServiceAccount{}
	if err := c.client.Get(ctx, types.NamespacedName{
		Name:      serviceAccountName,
		Namespace: namespace,
	}, account); err != nil {
		return fmt.Errorf("failed to fetch service account: %w", err)
	}

	logger.V(4).Info("got service account", "name", account.GetName())

	for _, imagePullSecret := range account.ImagePullSecrets {
		secret := &corev1.Secret{}

		if err := c.client.Get(ctx, types.NamespacedName{
			Name:      imagePullSecret.Name,
			Namespace: namespace,
		}, secret); err != nil {
			return fmt.Errorf("failed to get image pull secret: %w", err)
		}

		data, ok := secret.Data[dockerConfigKey]
		if !ok {
			return fmt.Errorf("failed to find .dockerconfigjson in secret %s", secret.Name)
		}

		repository := dockerconfig.NewRepositorySpecForConfig(data, true)

		if _, err := octx.CredentialsContext().RepositoryForSpec(repository); err != nil {
			return fmt.Errorf("failed to configure credentials for repository: %w", err)
		}
	}

	return nil
}

// GetComponentVersion returns a component Version. It's the caller's responsibility to clean it up and close the component Version once done with it.
func (c *Client) GetComponentVersion(ctx context.Context, octx ocm.Context, url, name, version string) (ocm.ComponentVersionAccess, error) {
	repoSpec := ocireg.NewRepositorySpec(url, nil)
	repo, err := octx.RepositoryForSpec(repoSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to get repository for spec: %w", err)
	}
	defer repo.Close()

	cv, err := repo.LookupComponentVersion(name, version)
	if err != nil {
		return nil, fmt.Errorf("failed to look up component Version: %w", err)
	}

	return cv, nil
}

var ErrNoProductDescription = errors.New("no product description")

func (c *Client) GetProductDescription(ctx context.Context, octx ocm.Context, cv ocm.ComponentVersionAccess) ([]byte, error) {
	resources, err := cv.GetResourcesByResourceSelectors(compdesc.ResourceSelectorFunc(func(obj compdesc.ResourceSelectionContext) (bool, error) {
		return obj.GetType() == ProductDescriptionType, nil
	}))
	if err != nil {
		return nil, fmt.Errorf("failed to get resource by selector: %w", err)
	}

	if len(resources) == 0 {
		return nil, ErrNoProductDescription
	}

	resource := resources[0]

	am, err := resource.AccessMethod()
	if err != nil {
		return nil, fmt.Errorf("failed to get access method for product description: %w", err)
	}

	reader, err := am.Reader()
	if err != nil {
		return nil, fmt.Errorf("failed to get product description content: %w", err)
	}

	decompressed, _, err := compression.AutoDecompress(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to get decompressed reader: %w", err)
	}

	content, err := io.ReadAll(decompressed)
	if err != nil {
		return nil, fmt.Errorf("failed to read content for product description: %w", err)
	}

	return content, nil
}

func (c *Client) GetResourceData(ctx context.Context, cv ocm.ComponentVersionAccess, ref v1alpha1.ResourceReference) ([]byte, error) {
	logger := log.FromContext(ctx)
	var identities []ocmmetav1.Identity
	identities = append(identities, ref.ReferencePath...)

	identity := ocmmetav1.NewIdentity(ref.Name)
	logger.V(4).Info("looking for resource data", "resource", ref.Name, "version", ref.Version, "identities", identities, "identity", identity)

	res, _, err := utils.ResolveResourceReference(cv, ocmmetav1.NewNestedResourceRef(identity, identities), cv.Repository())
	if err != nil {
		return nil, fmt.Errorf("failed to resolve reference path to resource: %w", err)
	}

	am, err := res.AccessMethod()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch access method for resource: %w", err)
	}

	reader, err := am.Reader()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch reader for resource: %w", err)
	}

	uncompressed, _, err := compression.AutoDecompress(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to auto decompress: %w", err)
	}
	defer uncompressed.Close()

	content, err := io.ReadAll(uncompressed)
	if err != nil {
		return nil, fmt.Errorf("failed to read resource data: %w", err)
	}

	return content, nil
}
