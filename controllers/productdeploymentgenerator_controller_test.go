package controllers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	v1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gitv1alpha1delivery "github.com/open-component-model/git-controller/apis/delivery/v1alpha1"
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
	manifests, err := os.ReadFile(filepath.Join("testdata", "no-values.tar.gz"))
	require.NoError(t, err)

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, string(manifests))
	}))

	testNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-namespace",
			Annotations: map[string]string{
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
						ID:      testNamespace.Name + "_repo_v1alpha1_GitRepository",
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
			Namespace: testNamespace.Name,
		},
	}
	subscription := &v1alpha13.ComponentSubscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-subscription",
			Namespace: testNamespace.Name,
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
			Namespace: testNamespace.Name,
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
			Namespace: testNamespace.Name,
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
	fakeOcmClient.GetResourceDataReturns("validation", []byte(""), nil)
	fakeClient := env.FakeKubeClient(WithObjects(mpasNamespace, testNamespace, repo, project, obj, subscription, serviceAccount),
		WithStatusSubresource(repo, project, obj, subscription))
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

	// Get the Sync and Get the snapshot data?
	sync := &gitv1alpha1delivery.Sync{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: obj.Name + "-sync-" + reconciler.hashComponentVersion("v0.0.1"), Namespace: testNamespace.Name}, sync)

	require.NoError(t, err)

	assert.Equal(t, "test-repository", sync.Spec.RepositoryRef.Name)

	modifiedReadme, err := os.ReadFile(filepath.Join("testdata", "modified_readme.md"))
	require.NoError(t, err)

	assert.Equal(t, string(modifiedReadme), fakeWriter.readmeContent)
	assert.Equal(t, `backend:
    cacheAddr: tcp://redis:6379
    replicas: 2
`, fakeWriter.valuesContent)

	productDeployment, err := os.ReadFile(filepath.Join("testdata", "product_deployment.yaml"))
	require.NoError(t, err)

	assert.Equal(t, string(productDeployment), fakeWriter.productDeploymentContent)
}

func TestProductDeploymentGeneratorReconcilerWithValueFile(t *testing.T) {
	manifests, err := os.ReadFile(filepath.Join("testdata", "values.tar.gz"))
	require.NoError(t, err)

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, string(manifests))
	}))

	testNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-namespace",
			Annotations: map[string]string{
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
				Namespace: testNamespace.Name,
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

	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service-account",
			Namespace: testNamespace.Name,
		},
	}
	subscription := &v1alpha13.ComponentSubscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-subscription",
			Namespace: testNamespace.Name,
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
			Namespace: testNamespace.Name,
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
			Namespace: testNamespace.Name,
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
	fakeOcmClient.GetResourceDataReturns("validation", []byte(""), nil)
	fakeClient := env.FakeKubeClient(WithObjects(mpasNamespace, testNamespace, repo, project, obj, subscription, serviceAccount),
		WithStatusSubresource(repo, project, obj, subscription))
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

	// Get the Sync and Get the snapshot data?
	sync := &gitv1alpha1delivery.Sync{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: obj.Name + "-sync-" + reconciler.hashComponentVersion("v0.0.1"), Namespace: testNamespace.Name}, sync)

	require.NoError(t, err)

	assert.Equal(t, "test-repository", sync.Spec.RepositoryRef.Name)

	modifiedReadme, err := os.ReadFile(filepath.Join("testdata", "modified_readme.md"))
	require.NoError(t, err)

	assert.Equal(t, string(modifiedReadme), fakeWriter.readmeContent)
	assert.Equal(t, `backend:
    cacheAddr: tcp://redis:6379
    replicas: 2 #+mpas-ignore
`, fakeWriter.valuesContent)

	productDeployment, err := os.ReadFile(filepath.Join("testdata", "product_deployment.yaml"))
	require.NoError(t, err)

	assert.Equal(t, string(productDeployment), fakeWriter.productDeploymentContent)
}

type mockSnapshotWriter struct {
	client.Object

	digest string
	err    error

	readmeContent            string
	productDeploymentContent string
	valuesContent            string
}

var _ snapshot.Writer = &mockSnapshotWriter{}

func (m *mockSnapshotWriter) Write(ctx context.Context, owner ocmctrv1.SnapshotWriter, sourceDir string, identity ocmmetav1.Identity) (string, error) {
	productDeploymentContent, err := os.ReadFile(filepath.Join(sourceDir, "test-product-deployment-generator", "product-deployment.yaml"))
	if err != nil {
		return "", fmt.Errorf("failed to read product deployment content: %w", err)
	}

	valuesContent, err := os.ReadFile(filepath.Join(sourceDir, "test-product-deployment-generator", "values.yaml"))
	if err != nil {
		return "", fmt.Errorf("failed to read values file: %w", err)
	}

	readmeContent, err := os.ReadFile(filepath.Join(sourceDir, "test-product-deployment-generator", "README.md"))
	if err != nil {
		return "", fmt.Errorf("failed to read readme file: %w", err)
	}

	m.productDeploymentContent = string(productDeploymentContent)
	m.valuesContent = string(valuesContent)
	m.readmeContent = string(readmeContent)

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
	repository, _ := genericocireg.NewRepository(nil, nil, nil)
	return repository
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

func (r *mockResource) ComponentVersion() ocm.ComponentVersionAccess {
	return nil
}

func (r *mockResource) Meta() *ocm.ResourceMeta {
	return &ocm.ResourceMeta{ElementMeta: *r.resource.GetMeta()}
}
