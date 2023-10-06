package controllers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	apiv1 "github.com/fluxcd/source-controller/api/v1"
	sourcebeta2 "github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	gitv1alpha1 "github.com/open-component-model/git-controller/apis/delivery/v1alpha1"
	alpha1 "github.com/open-component-model/git-controller/apis/mpas/v1alpha1"
	gitmpasv1alpha1 "github.com/open-component-model/git-controller/apis/mpas/v1alpha1"
	mpasv1alpha1 "github.com/open-component-model/mpas-product-controller/api/v1alpha1"
	projectv1 "github.com/open-component-model/mpas-project-controller/api/v1alpha1"

	"github.com/open-component-model/mpas-product-controller/pkg/validators"
)

func TestBasicReconcile(t *testing.T) {
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
	values, err := os.ReadFile(filepath.Join("testdata", "values.tar.gz"))
	require.NoError(t, err)
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(values)
	}))

	repository := &gitmpasv1alpha1.Repository{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{},
		Spec:       gitmpasv1alpha1.RepositorySpec{},
		Status:     gitmpasv1alpha1.RepositoryStatus{},
	}
	sync := &gitv1alpha1.Sync{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sync",
			Namespace: testNamespace.Name,
		},
		Spec: gitv1alpha1.SyncSpec{},
		Status: gitv1alpha1.SyncStatus{
			PullRequestID: 1,
		},
	}
	conditions.MarkTrue(sync, meta.ReadyCondition, meta.SucceededReason, "reconciliation successful")

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
				Name:      repository.Name,
				Namespace: repository.Namespace,
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
	conditions.MarkTrue(project, meta.ReadyCondition, meta.SucceededReason, "")

	owner := &mpasv1alpha1.ProductDeploymentGenerator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-product-deployment-generator",
			Namespace: testNamespace.Name,
		},
		Spec:   mpasv1alpha1.ProductDeploymentGeneratorSpec{},
		Status: mpasv1alpha1.ProductDeploymentGeneratorStatus{},
	}
	obj := &mpasv1alpha1.Validation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-validation",
			Namespace: testNamespace.Name,
		},
		Spec: mpasv1alpha1.ValidationSpec{
			ValidationRules: []mpasv1alpha1.ValidationData{
				{
					Data: []byte(`package main

deny[msg] {
  not input.replicas

  msg := "replicas must be set"
}
`),
					Name: "backend",
				},
			},
			ServiceAccountName: "default",
			Interval:           metav1.Duration{Duration: 1 * time.Second},
			SyncRef: meta.NamespacedObjectReference{
				Name:      sync.Name,
				Namespace: sync.Namespace,
			},
		},
	}

	err = controllerutil.SetOwnerReference(owner, obj, env.scheme)
	require.NoError(t, err)

	fakeClient := env.FakeKubeClient(
		WithObjects(testNamespace, mpasNamespace, obj, project, repository, sync),
		WithStatusSubresource(obj, project, repository, sync),
	)

	mgr := ValidationReconciler{
		Client:              fakeClient,
		Scheme:              env.scheme,
		MpasSystemNamespace: "mpas-system",
		Validator:           &mockValidator{},
	}

	// First Reconcile which should create the Values tracking GitRepository.
	_, err = mgr.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{
		Name:      obj.Name,
		Namespace: obj.Namespace,
	}})
	require.NoError(t, err)

	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: obj.Name, Namespace: obj.Namespace}, obj)
	require.NoError(t, err)

	valueGitRepository := &sourcebeta2.GitRepository{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: obj.Status.GitRepositoryRef.Name, Namespace: obj.Status.GitRepositoryRef.Namespace}, valueGitRepository)
	require.NoError(t, err)

	_, err = controllerutil.CreateOrUpdate(context.Background(), fakeClient, valueGitRepository, func() error {
		valueGitRepository.Status.Artifact = &apiv1.Artifact{
			Path:   "",
			URL:    testServer.URL,
			Digest: "different",
		}

		conditions.MarkTrue(valueGitRepository, meta.ReadyCondition, meta.SucceededReason, "")

		return nil
	})
	require.NoError(t, err)

	// Requeue the object simulating the watch kicking off
	_, err = mgr.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{
		Name:      obj.Name,
		Namespace: obj.Namespace,
	}})
	require.NoError(t, err)

	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: obj.Name, Namespace: obj.Namespace}, obj)
	require.NoError(t, err)

	assert.True(t, conditions.IsTrue(obj, meta.ReadyCondition))
	assert.Equal(t, mpasv1alpha1.SuccessValidationOutcome, obj.Status.LastValidatedDigestOutcome)
	assert.NotNil(t, obj.Status.GitRepositoryRef)
}

func TestRemovingGitRepositoryWhenPullRequestIsMerged(t *testing.T) {
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
	repository := &gitmpasv1alpha1.Repository{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{},
		Spec:       gitmpasv1alpha1.RepositorySpec{},
		Status:     gitmpasv1alpha1.RepositoryStatus{},
	}
	sync := &gitv1alpha1.Sync{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sync",
			Namespace: testNamespace.Name,
		},
		Spec: gitv1alpha1.SyncSpec{},
		Status: gitv1alpha1.SyncStatus{
			PullRequestID: 1,
		},
	}
	conditions.MarkTrue(sync, meta.ReadyCondition, meta.SucceededReason, "reconciliation successful")

	gitRepository := &sourcebeta2.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-git-repository",
			Namespace: testNamespace.Name,
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
				Name:      repository.Name,
				Namespace: repository.Namespace,
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
	conditions.MarkTrue(project, meta.ReadyCondition, meta.SucceededReason, "")

	owner := &mpasv1alpha1.ProductDeploymentGenerator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-product-deployment-generator",
			Namespace: testNamespace.Name,
		},
	}
	obj := &mpasv1alpha1.Validation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-validation",
			Namespace: testNamespace.Name,
		},
		Spec: mpasv1alpha1.ValidationSpec{
			SyncRef: meta.NamespacedObjectReference{
				Name:      sync.Name,
				Namespace: sync.Namespace,
			},
		},
		Status: mpasv1alpha1.ValidationStatus{
			GitRepositoryRef: &meta.NamespacedObjectReference{
				Name:      gitRepository.Name,
				Namespace: gitRepository.Namespace,
			},
		},
	}
	conditions.MarkTrue(obj, meta.ReadyCondition, meta.SucceededReason, "")

	err := controllerutil.SetOwnerReference(owner, obj, env.scheme)
	require.NoError(t, err)

	fakeClient := env.FakeKubeClient(
		WithObjects(testNamespace, mpasNamespace, obj, project, repository, sync, gitRepository),
		WithStatusSubresource(obj, project, repository, sync, gitRepository),
	)

	mgr := ValidationReconciler{
		Client:              fakeClient,
		Scheme:              env.scheme,
		MpasSystemNamespace: "mpas-system",
		Validator:           &mockValidator{},
	}

	// First Reconcile which should create the Values tracking GitRepository.
	_, err = mgr.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{
		Name:      obj.Name,
		Namespace: obj.Namespace,
	}})
	require.NoError(t, err)

	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name:      gitRepository.Name,
		Namespace: gitRepository.Namespace,
	}, gitRepository)
	require.True(t, apierrors.IsNotFound(err))
}

type mockValidator struct{}

func (m *mockValidator) FailValidation(ctx context.Context, repository gitmpasv1alpha1.Repository, sync gitv1alpha1.Sync) error {
	return nil
}

func (m *mockValidator) PassValidation(ctx context.Context, repository gitmpasv1alpha1.Repository, sync gitv1alpha1.Sync) error {
	return nil
}

func (m *mockValidator) IsMergedOrClosed(ctx context.Context, repository gitmpasv1alpha1.Repository, sync gitv1alpha1.Sync) (bool, error) {
	return true, nil
}

var _ validators.Validator = &mockValidator{}
