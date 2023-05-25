package controllers

import (
	"context"
	"testing"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	alpha1 "github.com/open-component-model/git-controller/apis/mpas/v1alpha1"
	projectv1 "github.com/open-component-model/mpas-project-controller/api/v1alpha1"
	v1alpha12 "github.com/open-component-model/ocm-controller/api/v1alpha1"
	"github.com/open-component-model/ocm-controller/pkg/snapshot"
	ocmmetav1 "github.com/open-component-model/ocm/pkg/contexts/ocm/compdesc/meta/v1"
	v1alpha13 "github.com/open-component-model/replication-controller/api/v1alpha1"
	"github.com/stretchr/testify/require"

	"github.com/open-component-model/mpas-product-controller/api/v1alpha1"
	"github.com/open-component-model/mpas-product-controller/pkg/ocm/fakes"
)

func TestProductDeploymentGeneratorReconciler(t *testing.T) {
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
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service-account",
			Namespace: "mpas-system",
		},
	}
	subscription := &v1alpha13.ComponentSubscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-subscription",
			Namespace: "mpas-system",
		},
		Spec: v1alpha13.ComponentSubscriptionSpec{
			Interval: metav1.Duration{Duration: time.Second},
			Source: v1alpha13.OCMRepository{
				URL: "https://github.com/open-component-model/source",
			},
			Component:          "github.com/open-component-model/mpas",
			ServiceAccountName: serviceAccount.Name,
			Semver:             "v0.0.1",
		},
		Status: v1alpha13.ComponentSubscriptionStatus{},
	}

	obj := &v1alpha1.ProductDeploymentGenerator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-product-deployment-generator",
			Namespace: "mpas-system",
		},
		Spec: v1alpha1.ProductDeploymentGeneratorSpec{
			Interval: metav1.Duration{Duration: time.Second},
			SubscriptionRef: meta.NamespacedObjectReference{
				Name:      subscription.Name,
				Namespace: subscription.Namespace,
			},
			ServiceAccountName: serviceAccount.Name,
		},
	}

	fakeClient := env.FakeKubeClient(WithObjets(project, obj, subscription, serviceAccount))
	fakeOcmClient := &fakes.MockOCM{}
	fakeWriter := &mockSnapshotWriter{}

	reconciler := &ProductDeploymentGeneratorReconciler{
		Client:              fakeClient,
		Scheme:              env.scheme,
		OCMClient:           fakeOcmClient,
		SnapshotWriter:      fakeWriter,
		MpasSystemNamespace: "mpas-system",
	}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      obj.Name,
			Namespace: obj.Namespace,
		},
	})

	require.NoError(t, err)
}

type mockSnapshotWriter struct {
	client.Object

	digest string
	err    error
}

var _ snapshot.Writer = &mockSnapshotWriter{}

func (m *mockSnapshotWriter) Write(ctx context.Context, owner v1alpha12.SnapshotWriter, sourceDir string, identity ocmmetav1.Identity) (string, error) {
	return m.digest, m.err
}
