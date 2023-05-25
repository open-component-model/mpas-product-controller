package controllers

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	alpha1 "github.com/open-component-model/git-controller/apis/mpas/v1alpha1"
	gitv1alpha1 "github.com/open-component-model/git-controller/apis/mpas/v1alpha1"
	projectv1 "github.com/open-component-model/mpas-project-controller/api/v1alpha1"
	ocmctrv1 "github.com/open-component-model/ocm-controller/api/v1alpha1"
	"github.com/open-component-model/ocm-controller/pkg/snapshot"
	"github.com/open-component-model/ocm/pkg/common/accessobj"
	"github.com/open-component-model/ocm/pkg/contexts/ocm"
	ocmdesc "github.com/open-component-model/ocm/pkg/contexts/ocm/compdesc"
	ocmmetav1 "github.com/open-component-model/ocm/pkg/contexts/ocm/compdesc/meta/v1"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/repositories/comparch"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/repositories/genericocireg"
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
			Git: alpha1.RepositorySpec{
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
						ID:      "mpas-system_repo_v1alpha1_GitRepository",
						Version: "v0.0.1",
					},
				},
			},
		},
	}
	conditions.MarkTrue(project, meta.ReadyCondition, meta.SucceededReason, "Reconciliation success")

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
		Status: v1alpha13.ComponentSubscriptionStatus{
			LastAttemptedVersion:    "v0.0.1",
			ObservedGeneration:      0,
			LastAppliedVersion:      "v0.0.1",
			ReplicatedRepositoryURL: "https://github.com/open-component-model/source",
		},
	}
	conditions.MarkTrue(subscription, meta.ReadyCondition, meta.SucceededReason, "Reconciliation success")

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
	cv := &ocmctrv1.ComponentVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-component",
			Namespace: "default",
		},
		Spec: ocmctrv1.ComponentVersionSpec{
			Interval:  metav1.Duration{Duration: 10 * time.Minute},
			Component: "github.com/open-component-model/mpas",
			Version: ocmctrv1.Version{
				Semver: "v0.0.1",
			},
			Repository: ocmctrv1.Repository{
				URL: "github.com/open-component-model/test",
			},
			Verify: []ocmctrv1.Signature{},
			References: ocmctrv1.ReferencesConfig{
				Expand: true,
			},
		},
	}
	root := &mockComponent{
		t: t,
		descriptor: &ocmdesc.ComponentDescriptor{
			ComponentSpec: ocmdesc.ComponentSpec{
				ObjectMeta: ocmmetav1.ObjectMeta{
					Name:    cv.Spec.Component,
					Version: "v0.0.1",
				},
				References: ocmdesc.References{
					{
						ElementMeta: ocmdesc.ElementMeta{
							Name:    "test-ref-1",
							Version: "v0.0.1",
						},
						ComponentName: "github.com/open-component-model/test-component",
					},
				},
			},
		},
	}

	productDescription, err := os.ReadFile(filepath.Join("testdata", "product_description.yaml"))
	require.NoError(t, err)
	config, err := os.ReadFile(filepath.Join("testdata", "config.yaml"))
	require.NoError(t, err)
	manifest, err := os.ReadFile(filepath.Join("testdata", "manifests.tar"))
	require.NoError(t, err)
	readme, err := os.ReadFile(filepath.Join("testdata", "README.md"))
	require.NoError(t, err)

	fakeOcmClient := &fakes.MockOCM{}
	fakeOcmClient.GetComponentVersionReturnsForName(root.descriptor.ComponentSpec.Name, root, nil)
	fakeOcmClient.GetProductDescriptionReturns(productDescription, nil)
	fakeOcmClient.GetResourceDataReturns("config", config, nil)
	fakeOcmClient.GetResourceDataReturns("manifest", manifest, nil)
	fakeOcmClient.GetResourceDataReturns("instructions", readme, nil)
	fakeClient := env.FakeKubeClient(WithObjets(project, obj, subscription, serviceAccount))
	fakeWriter := &mockSnapshotWriter{}

	reconciler := &ProductDeploymentGeneratorReconciler{
		Client:              fakeClient,
		Scheme:              env.scheme,
		OCMClient:           fakeOcmClient,
		SnapshotWriter:      fakeWriter,
		MpasSystemNamespace: "mpas-system",
	}

	_, err = reconciler.Reconcile(context.Background(), ctrl.Request{
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

func (m *mockSnapshotWriter) Write(ctx context.Context, owner ocmctrv1.SnapshotWriter, sourceDir string, identity ocmmetav1.Identity) (string, error) {
	return m.digest, m.err
}

type mockComponent struct {
	ocm.ComponentVersionAccess
	descriptor *ocmdesc.ComponentDescriptor
	t          *testing.T
}

func (m *mockComponent) GetName() string {
	return m.descriptor.ComponentSpec.Name
}

func (m *mockComponent) GetDescriptor() *ocmdesc.ComponentDescriptor {
	return m.descriptor
}

// how to get resource access for resource?
func (m *mockComponent) GetResource(id ocmmetav1.Identity) (ocm.ResourceAccess, error) {
	r, err := m.descriptor.GetResourceByIdentity(id)
	if err != nil {
		return nil, err
	}
	return &mockResource{resource: r, ctx: ocm.DefaultContext()}, nil
}

func (m *mockComponent) Repository() ocm.Repository {
	return &genericocireg.Repository{}
}

func (m *mockComponent) Dup() (ocm.ComponentVersionAccess, error) {
	return m, nil
}

func (m *mockComponent) Close() error {
	return nil
}

type mockResource struct {
	ctx      ocm.Context
	resource ocmdesc.Resource
}

func (r *mockResource) Access() (ocm.AccessSpec, error) {
	return r.ctx.AccessSpecForSpec(r.resource.Access)
}

func (r *mockResource) AccessMethod() (ocm.AccessMethod, error) {
	ca, err := comparch.New(r.ctx, accessobj.ACC_CREATE, nil, nil, nil, 0600)
	if err != nil {
		return nil, err
	}
	spec, err := r.ctx.AccessSpecForSpec(r.resource.Access)
	if err != nil {
		return nil, err
	}
	return spec.AccessMethod(ca)
}

func (r *mockResource) Meta() *ocm.ResourceMeta {
	return &ocm.ResourceMeta{ElementMeta: *r.resource.GetMeta()}
}
