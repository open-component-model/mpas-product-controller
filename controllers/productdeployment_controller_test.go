// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"testing"

	"github.com/fluxcd/pkg/runtime/patch"
	ocmv1alpha1 "github.com/open-component-model/ocm-controller/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	ocmmetav1 "github.com/open-component-model/ocm/pkg/contexts/ocm/compdesc/meta/v1"
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
						ElementMeta: ocmv1alpha1.ElementMeta{
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
						ElementMeta: ocmv1alpha1.ElementMeta{
							Name:    "localization",
							Version: "1.0.0",
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

	fakeKube := env.FakeKubeClient(WithObjects(deployment), WithStatusSubresource(deployment))
	recorder := record.NewFakeRecorder(32)

	reconciler := ProductDeploymentReconciler{
		Client:        fakeKube,
		Scheme:        env.scheme,
		EventRecorder: recorder,
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
}

func TestComponentVersionIsUpdated(t *testing.T) {
	deployment := &v1alpha1.ProductDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "default",
		},
		Spec: v1alpha1.ProductDeploymentSpec{
			Component: replicationv1.Component{
				Name:    "github.com/test/test-component",
				Version: "v2.0.0",
				Registry: replicationv1.Registry{
					URL: "ghcr.io/open-component-model/ocm",
				},
			},
		},
	}

	fakeKube := env.FakeKubeClient(WithObjects(deployment), WithStatusSubresource(deployment.DeepCopy()))

	reconciler := ProductDeploymentReconciler{
		Client:        fakeKube,
		Scheme:        env.scheme,
		EventRecorder: record.NewFakeRecorder(32),
	}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      deployment.Name,
			Namespace: deployment.Namespace,
		},
	})
	require.NoError(t, err)

	cv := &ocmv1alpha1.ComponentVersion{}
	err = fakeKube.Get(context.Background(), types.NamespacedName{Name: deployment.Name + "component-version", Namespace: deployment.Namespace}, cv)
	require.NoError(t, err)

	assert.Equal(t, deployment.Spec.Component.Version, cv.Spec.Version.Semver)

	patcher := patch.NewSerialPatcher(deployment, fakeKube)
	deployment.Spec.Component.Version = "v3.0.0"
	require.NoError(t, patcher.Patch(context.Background(), deployment))

	_, err = reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      deployment.Name,
			Namespace: deployment.Namespace,
		},
	})
	require.NoError(t, err)

	err = fakeKube.Get(context.Background(), types.NamespacedName{Name: deployment.Name + "component-version", Namespace: deployment.Namespace}, cv)
	require.NoError(t, err)

	assert.Equal(t, deployment.Spec.Component.Version, cv.Spec.Version.Semver)
}
