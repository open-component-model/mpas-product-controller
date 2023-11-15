// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"testing"
	"time"

	kustomizev1beta2 "github.com/fluxcd/kustomize-controller/api/v1beta2"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	alpha1 "github.com/open-component-model/git-controller/apis/mpas/v1alpha1"
	projectv1 "github.com/open-component-model/mpas-project-controller/api/v1alpha1"
	"github.com/open-component-model/ocm-controller/api/v1alpha1"
	ocmmetav1 "github.com/open-component-model/ocm/pkg/contexts/ocm/compdesc/meta/v1"
	replicationv1 "github.com/open-component-model/replication-controller/api/v1alpha1"

	mpasv1alpha1 "github.com/open-component-model/mpas-product-controller/api/v1alpha1"
)

func TestProductDeploymentPipelineReconciler(t *testing.T) {
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
	owner := &mpasv1alpha1.ProductDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pipeline-owner",
			Namespace: testNamespace.Name,
		},
		Spec: mpasv1alpha1.ProductDeploymentSpec{
			Component: replicationv1.Component{
				Name:    "github.com/skarlso/component",
				Version: "0.0.1",
				Registry: replicationv1.Registry{
					URL: "https://github.com/Skarlso/test",
				},
			},
			Pipelines:          nil,
			ServiceAccountName: "mpas-admin",
		},
	}

	cv := &v1alpha1.ComponentVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline-component-version",
			Namespace: testNamespace.Name,
		},
		Spec: v1alpha1.ComponentVersionSpec{
			Interval:  metav1.Duration{Duration: time.Second},
			Component: "github.com/skarlso/component",
			Version: v1alpha1.Version{
				Semver: "v0.0.1",
			},
			Repository: v1alpha1.Repository{
				URL: "https://github.com/Skarlso/test",
			},
			References: v1alpha1.ReferencesConfig{
				Expand: true,
			},
			ServiceAccountName: "mpas-admin",
		},
	}

	obj := &mpasv1alpha1.ProductDeploymentPipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline",
			Namespace: testNamespace.Name,
		},
		Spec: mpasv1alpha1.ProductDeploymentPipelineSpec{
			ComponentVersionRef: "test-pipeline-component-version",
			Resource: mpasv1alpha1.ResourceReference{
				ElementMeta: v1alpha1.ElementMeta{
					Name:    "backend",
					Version: "0.0.1",
				},
				ReferencePath: []ocmmetav1.Identity{
					{
						"name": "deployment",
					},
				},
			},
			Localization: mpasv1alpha1.ResourceReference{
				ElementMeta: v1alpha1.ElementMeta{
					Name:    "manifests",
					Version: "0.0.1",
				},
				ReferencePath: []ocmmetav1.Identity{
					{
						"name": "manifests",
					},
				},
			},
			TargetRole: mpasv1alpha1.TargetRole{
				Type: mpasv1alpha1.Kubernetes,
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"label1": "value1",
					},
				},
			},
		},
	}

	err := controllerutil.SetControllerReference(owner, obj, env.scheme)
	require.NoError(t, err)

	target := &mpasv1alpha1.Target{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-target",
			Namespace: "mpas-system",
			Labels: map[string]string{
				"label1": "value1",
			},
		},
		Spec: mpasv1alpha1.TargetSpec{
			Type: mpasv1alpha1.Kubernetes,
			Access: &apiextensionsv1.JSON{
				Raw: []byte(`secretRef:
  name: kube-config
  namespace: mpas-system
targetNamespace: mpas-system
`),
			},
		},
	}

	project := &projectv1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "project",
			Namespace: "mpas-system",
		},
		Spec: projectv1.ProjectSpec{
			Git: alpha1.RepositorySpec{},
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
						ID:      "mpas-system_repo_v1alpha1_GitRepository",
						Version: "v0.0.1",
					},
				},
			},
		},
	}
	snapshot := &v1alpha1.Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Name + "-localization-snapshot",
			Namespace: testNamespace.Name,
		},
		Spec:   v1alpha1.SnapshotSpec{},
		Status: v1alpha1.SnapshotStatus{},
	}
	localization := &v1alpha1.Localization{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Name + "-localization",
			Namespace: testNamespace.Name,
		},
		Spec: v1alpha1.MutationSpec{
			Interval: metav1.Duration{Duration: 10 * time.Minute},
			SourceRef: v1alpha1.ObjectReference{
				NamespacedObjectKindReference: meta.NamespacedObjectKindReference{
					Kind:      "ComponentVersion",
					Name:      obj.Spec.ComponentVersionRef,
					Namespace: obj.Namespace,
				},
			},
			ConfigRef: &v1alpha1.ObjectReference{
				NamespacedObjectKindReference: meta.NamespacedObjectKindReference{
					Kind:      "ComponentVersion",
					Name:      obj.Spec.ComponentVersionRef,
					Namespace: obj.Namespace,
				},
			},
		},
		Status: v1alpha1.MutationStatus{
			LatestSnapshotDigest: "digest",
			SnapshotName:         snapshot.Name,
		},
	}

	fakeClient := env.FakeKubeClient(
		WithAddToScheme(kustomizev1beta2.AddToScheme),
		WithObjects(testNamespace, mpasNamespace, project, owner, obj, cv, target, snapshot, localization),
		WithStatusSubresource(project, owner, cv, target, snapshot, obj, localization))

	mgr := &ProductDeploymentPipelineReconciler{
		Client:              fakeClient,
		Scheme:              env.scheme,
		MpasSystemNamespace: "mpas-system",
		EventRecorder:       record.NewFakeRecorder(32),
	}

	_, err = mgr.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: obj.Name, Namespace: obj.Namespace},
	})
	require.NoError(t, err)

	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name:      obj.Name,
		Namespace: obj.Namespace,
	}, obj)
	require.NoError(t, err)

	assert.True(t, conditions.IsTrue(obj, meta.ReadyCondition))
}
