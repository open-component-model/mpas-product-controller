package controllers

import (
	"context"
	"testing"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	mpasv1alpha1 "github.com/open-component-model/mpas-product-controller/api/v1alpha1"
	"github.com/open-component-model/mpas-product-controller/pkg/deployers"
	"github.com/open-component-model/ocm-controller/api/v1alpha1"
	ocmmetav1 "github.com/open-component-model/ocm/pkg/contexts/ocm/compdesc/meta/v1"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/compdesc/versions/ocm.software/v3alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestSnapshotIsAvailable(t *testing.T) {
	snapshot := &v1alpha1.Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "localization-snapshot",
			Namespace: "mpas-system",
		},
		Spec:   v1alpha1.SnapshotSpec{},
		Status: v1alpha1.SnapshotStatus{},
	}
	conditions.MarkTrue(snapshot, meta.ReadyCondition, meta.SucceededReason, "Reconciliation success")
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
		Status: mpasv1alpha1.ProductDeploymentPipelineStatus{
			SnapshotRef: &meta.NamespacedObjectReference{
				Name:      snapshot.Name,
				Namespace: snapshot.Namespace,
			},
		},
	}

	fakeClient := env.FakeKubeClient(
		WithObjects(target, obj, snapshot),
		WithStatusSubresource(target, obj, snapshot),
	)

	mgr := &ProductDeploymentPipelineScheduler{
		Client:              fakeClient,
		Scheme:              env.scheme,
		MpasSystemNamespace: "mpas-system",
		Deployer:            &mockDeployer{},
	}

	_, err := mgr.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: obj.Name, Namespace: obj.Namespace},
	})
	require.NoError(t, err)

	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name:      obj.Name,
		Namespace: obj.Namespace,
	}, obj)
	require.NoError(t, err)

	assert.True(t, conditions.IsTrue(obj, mpasv1alpha1.DeployedCondition))
}

func TestSnapshotIsUnavailable(t *testing.T) {
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
		Status: mpasv1alpha1.ProductDeploymentPipelineStatus{
			SnapshotRef: &meta.NamespacedObjectReference{
				Name:      "test-snapshot-name",
				Namespace: "mpas-system",
			},
		},
	}

	fakeClient := env.FakeKubeClient(
		WithObjects(obj),
	)

	mgr := &ProductDeploymentPipelineScheduler{
		Client:              fakeClient,
		Scheme:              env.scheme,
		MpasSystemNamespace: "mpas-system",
		Deployer:            &mockDeployer{},
	}

	_, err := mgr.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: obj.Name, Namespace: obj.Namespace},
	})
	require.NoError(t, err)

	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name:      obj.Name,
		Namespace: obj.Namespace,
	}, obj)
	require.NoError(t, err)

	assert.False(t, conditions.IsTrue(obj, mpasv1alpha1.DeployedCondition))
}

type mockDeployer struct {
}

func (m *mockDeployer) Deploy(ctx context.Context, obj *mpasv1alpha1.ProductDeploymentPipeline) error {
	return nil
}

var _ deployers.Deployer = &mockDeployer{}
