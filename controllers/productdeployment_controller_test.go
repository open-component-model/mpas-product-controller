// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"testing"

	ocmv1alpha1 "github.com/open-component-model/ocm-controller/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	ocmmetav1 "github.com/open-component-model/ocm/pkg/contexts/ocm/compdesc/meta/v1"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/compdesc/versions/ocm.software/v3alpha1"
	replicationv1 "github.com/open-component-model/replication-controller/api/v1alpha1"

	"github.com/open-component-model/mpas-product-controller/api/v1alpha1"
)

func TestProductDeploymentReconciler(t *testing.T) {
	deployment := &v1alpha1.ProductDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "default",
		},
		Spec: v1alpha1.ProductDeploymentSpec{
			Component: replicationv1.Component{
				Name:    "github.com/test/test-component",
				Version: "1.0.0",
			},
			Pipelines: []v1alpha1.Pipeline{
				{
					Name: "backend",
					Resource: v1alpha1.ResourceReference{
						ElementMeta: v3alpha1.ElementMeta{
							Name:    "manifests",
							Version: "1.0.0",
						},
						ReferencePath: []ocmmetav1.Identity{
							{
								"name": "backend",
							},
						},
					},
					Localization: v1alpha1.ResourceReference{
						ElementMeta: v3alpha1.ElementMeta{
							Name:    "localization",
							Version: "1.0.0",
						},
					},
					Configuration: v1alpha1.Configuration{
						Rules: v1alpha1.ResourceReference{
							ElementMeta: v3alpha1.ElementMeta{
								Name:    "configuration",
								Version: "1.0.0",
							},
						},
					},
					TargetRole: v1alpha1.TargetRole{
						Type:     "kubernetes",
						Selector: metav1.LabelSelector{},
					},
				},
			},
		},
	}

	fakeKube := env.FakeKubeClient(WithObjets(deployment), WithAddToScheme(v1alpha1.AddToScheme), WithAddToScheme(ocmv1alpha1.AddToScheme))

	reconciler := ProductDeploymentReconciler{
		Client: fakeKube,
		Scheme: env.scheme,
	}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      deployment.Name,
			Namespace: deployment.Namespace,
		},
	})
	require.NoError(t, err)

	err = fakeKube.Get(context.Background(), types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, deployment)
	require.NoError(t, err)
	assert.True(t, len(deployment.Status.ActivePipelines) == 1)

	pipeline := &v1alpha1.ProductDeploymentPipeline{}
	err = fakeKube.Get(context.Background(), types.NamespacedName{Name: "backend", Namespace: "default"}, pipeline)
	require.NoError(t, err)

	assert.Equal(t, "manifests", pipeline.Spec.Resource.Name)
	assert.Equal(t, "localization", pipeline.Spec.Localization.Name)
	assert.Equal(t, "configuration", pipeline.Spec.Configuration.Rules.Name)
}
