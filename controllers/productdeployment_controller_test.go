// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/patch"
	v1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/fluxcd/source-controller/api/v1beta2"
	gitv1alpha1 "github.com/open-component-model/git-controller/apis/mpas/v1alpha1"
	projectv1 "github.com/open-component-model/mpas-project-controller/api/v1alpha1"
	ocmv1alpha1 "github.com/open-component-model/ocm-controller/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	ocmmetav1 "github.com/open-component-model/ocm/pkg/contexts/ocm/compdesc/meta/v1"
	replicationv1 "github.com/open-component-model/replication-controller/api/v1alpha1"

	"github.com/open-component-model/mpas-product-controller/api/v1alpha1"
)

func TestProductDeploymentReconciler(t *testing.T) {
	manifests, err := os.ReadFile(filepath.Join("testdata", "values.tar.gz"))
	require.NoError(t, err)

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, string(manifests))
	}))
	defer testServer.Close()

	testNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-namespace",
			Labels: map[string]string{
				projectv1.ProjectKey: "project",
			},
		},
	}

	mpasNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mpas-system",
		},
	}

	repo := &v1beta2.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "repo",
			Namespace: testNamespace.Name,
		},
		Spec: v1beta2.GitRepositorySpec{
			URL: "oci://repo",
		},
		Status: v1beta2.GitRepositoryStatus{
			URL: "oci://repo",
			Artifact: &v1.Artifact{
				Path: "./",
				URL:  testServer.URL,
			},
		},
	}
	project := &projectv1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "project",
			Namespace: mpasNamespace.Name,
		},
		Spec: projectv1.ProjectSpec{
			Git: gitv1alpha1.RepositorySpec{
				CommitTemplate: &gitv1alpha1.CommitTemplate{
					Email:   "email@email.com",
					Message: "message",
					Name:    "name",
				},
			},
		},
		Status: projectv1.ProjectStatus{
			RepositoryRef: &meta.NamespacedObjectReference{
				Name:      "test-repository",
				Namespace: "mpas-system",
			},
			Inventory: &projectv1.ResourceInventory{
				Entries: []projectv1.ResourceRef{
					{
						// in the format '<namespace>_<name>_<group>_<kind>'.
						ID:      testNamespace.Name + "_repo_v1alpha1_GitRepository",
						Version: "v0.0.1",
					},
				},
			},
		},
	}
	conditions.MarkTrue(project, meta.ReadyCondition, meta.SucceededReason, "Reconciliation success")

	deployment := &v1alpha1.ProductDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-product-deployment-generator",
			Namespace: testNamespace.Name,
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
			Schema: []byte(`
#SchemaVersion: "v1.0.0"
// this field has a default value of 2
replicas: *2 | int
// this is a required field with of type string with a constraint
cacheAddr: *"tcp://redis:6379" | string & =~"^tcp://.+|^https://.+"`),
		},
	}

	fakeKube := env.FakeKubeClient(WithObjects(mpasNamespace, testNamespace, repo, project, deployment), WithStatusSubresource(repo, project, deployment))
	recorder := record.NewFakeRecorder(32)

	reconciler := ProductDeploymentReconciler{
		Client:              fakeKube,
		Scheme:              env.scheme,
		EventRecorder:       recorder,
		MpasSystemNamespace: "mpas-system",
	}

	_, err = reconciler.Reconcile(context.Background(), ctrl.Request{
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
	err = fakeKube.Get(context.Background(), types.NamespacedName{Name: "backend", Namespace: deployment.Namespace}, pipeline)
	require.NoError(t, err)

	assert.Equal(t, "manifests", pipeline.Spec.Resource.Name)
	assert.Equal(t, "localization", pipeline.Spec.Localization.Name)
}

func TestComponentVersionIsUpdated(t *testing.T) {
	manifests, err := os.ReadFile(filepath.Join("testdata", "values.tar.gz"))
	require.NoError(t, err)

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, string(manifests))
	}))
	defer testServer.Close()

	testNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-namespace",
			Labels: map[string]string{
				projectv1.ProjectKey: "project",
			},
		},
	}

	mpasNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mpas-system",
		},
	}

	repo := &v1beta2.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "repo",
			Namespace: testNamespace.Name,
		},
		Spec: v1beta2.GitRepositorySpec{
			URL: "oci://repo",
		},
		Status: v1beta2.GitRepositoryStatus{
			URL: "oci://repo",
			Artifact: &v1.Artifact{
				Path: "./",
				URL:  testServer.URL,
			},
		},
	}
	project := &projectv1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "project",
			Namespace: mpasNamespace.Name,
		},
		Spec: projectv1.ProjectSpec{
			Git: gitv1alpha1.RepositorySpec{
				CommitTemplate: &gitv1alpha1.CommitTemplate{
					Email:   "email@email.com",
					Message: "message",
					Name:    "name",
				},
			},
		},
		Status: projectv1.ProjectStatus{
			RepositoryRef: &meta.NamespacedObjectReference{
				Name:      "test-repository",
				Namespace: "mpas-system",
			},
			Inventory: &projectv1.ResourceInventory{
				Entries: []projectv1.ResourceRef{
					{
						// in the format '<namespace>_<name>_<group>_<kind>'.
						ID:      testNamespace.Name + "_repo_v1alpha1_GitRepository",
						Version: "v0.0.1",
					},
				},
			},
		},
	}
	conditions.MarkTrue(project, meta.ReadyCondition, meta.SucceededReason, "Reconciliation success")

	deployment := &v1alpha1.ProductDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-product-deployment-generator",
			Namespace: testNamespace.Name,
		},
		Spec: v1alpha1.ProductDeploymentSpec{
			Component: replicationv1.Component{
				Name:    "github.com/test/test-component",
				Version: "v2.0.0",
				Registry: replicationv1.Registry{
					URL: "ghcr.io/open-component-model/ocm",
				},
			},
			Schema: []byte(`
#SchemaVersion: "v1.0.0"
// this field has a default value of 2
replicas: *2 | int
// this is a required field with of type string with a constraint
cacheAddr: *"tcp://redis:6379" | string & =~"^tcp://.+|^https://.+"`),
		},
	}

	fakeKube := env.FakeKubeClient(WithObjects(mpasNamespace, testNamespace, repo, project, deployment), WithStatusSubresource(repo, project, deployment))

	reconciler := ProductDeploymentReconciler{
		Client:              fakeKube,
		Scheme:              env.scheme,
		EventRecorder:       record.NewFakeRecorder(32),
		MpasSystemNamespace: "mpas-system",
	}

	_, err = reconciler.Reconcile(context.Background(), ctrl.Request{
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
