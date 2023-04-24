// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package ocm

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/open-component-model/ocm/pkg/contexts/credentials/repositories/dockerconfig"
	"github.com/open-component-model/ocm/pkg/contexts/ocm"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/repositories/ocireg"
)

const dockerConfigKey = ".dockerconfigjson"

// Contract defines a subset of capabilities from the OCM library.
type Contract interface {
	CreateAuthenticatedOCMContext(ctx context.Context, serviceAccountName, namespace string) (ocm.Context, error)
	GetComponentVersion(ctx context.Context, octx ocm.Context, url, name, version string) (ocm.ComponentVersionAccess, error)
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
