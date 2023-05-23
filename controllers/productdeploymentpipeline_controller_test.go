package controllers

import (
	"context"
	"testing"
	"time"

	kustomizev1beta2 "github.com/fluxcd/kustomize-controller/api/v1beta2"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	alpha1 "github.com/open-component-model/git-controller/apis/mpas/v1alpha1"
	projectv1 "github.com/open-component-model/mpas-project-controller/api/v1alpha1"
	"github.com/open-component-model/ocm-controller/api/v1alpha1"
	ocmmetav1 "github.com/open-component-model/ocm/pkg/contexts/ocm/compdesc/meta/v1"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/compdesc/versions/ocm.software/v3alpha1"
	replicationv1 "github.com/open-component-model/replication-controller/api/v1alpha1"

	mpasv1alpha1 "github.com/open-component-model/mpas-product-controller/api/v1alpha1"
)

func TestProductDeploymentPipelineReconciler(t *testing.T) {
	owner := &mpasv1alpha1.ProductDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pipeline-owner",
			Namespace: "mpas-system",
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
			Namespace: "mpas-system",
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
			Namespace: "mpas-system",
		},
		Spec: mpasv1alpha1.ProductDeploymentPipelineSpec{
			ComponentVersionRef: "test-pipeline-component-version",
			Resource: mpasv1alpha1.ResourceReference{
				ElementMeta: v3alpha1.ElementMeta{
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
				ElementMeta: v3alpha1.ElementMeta{
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
			Access: &mpasv1alpha1.Access{
				SecretRef: &meta.NamespacedObjectReference{
					Name:      "kube-config",
					Namespace: "mpas-system",
				},
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
			Namespace: "mpas-system",
		},
		Spec:   v1alpha1.SnapshotSpec{},
		Status: v1alpha1.SnapshotStatus{},
	}
	localization := &v1alpha1.Localization{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Name + "-localization",
			Namespace: obj.Namespace,
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
		WithAddToScheme(v1alpha1.AddToScheme),
		WithAddToScheme(projectv1.AddToScheme),
		WithAddToScheme(v1beta2.AddToScheme),
		WithAddToScheme(kustomizev1beta2.AddToScheme),
		WithObjets(project, owner, obj, cv, target, snapshot, localization),
	)

	mgr := &ProductDeploymentPipelineReconciler{
		Client:              fakeClient,
		Scheme:              env.scheme,
		MpasSystemNamespace: "mpas-system",
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
	assert.Equal(t, "kube-target", obj.Status.SelectedTarget.Name)
}
