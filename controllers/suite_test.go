// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/object"
	envtest "github.com/fluxcd/pkg/runtime/testenv"
	sourcebeta2 "github.com/fluxcd/source-controller/api/v1beta2"
	gitv1alpha1delivery "github.com/open-component-model/git-controller/apis/delivery/v1alpha1"
	gitv1alpha1mpas "github.com/open-component-model/git-controller/apis/mpas/v1alpha1"
	projectv1 "github.com/open-component-model/mpas-project-controller/api/v1alpha1"
	v1alpha12 "github.com/open-component-model/ocm-controller/api/v1alpha1"
	replicationv1 "github.com/open-component-model/replication-controller/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/open-component-model/mpas-product-controller/api/v1alpha1"
)

type testEnv struct {
	scheme       *runtime.Scheme
	obj          []client.Object
	timeout      time.Duration
	pollInterval time.Duration
	subs         []client.Object
	envtest      *envtest.Environment
}

// FakeKubeClientOption defines options to construct a fake kube client. There are some defaults involved.
// Scheme gets corev1 and ocmv1alpha1 schemes by default. Anything that is passed in will override current
// defaults.
type FakeKubeClientOption func(testEnv *testEnv)

// WithAddToScheme adds the scheme
func WithAddToScheme(addToScheme func(s *runtime.Scheme) error) FakeKubeClientOption {
	return func(env *testEnv) {
		if err := addToScheme(env.scheme); err != nil {
			panic(err)
		}
	}
}

// WithObjects provides an option to set objects for the fake client.
func WithObjects(obj ...client.Object) FakeKubeClientOption {
	return func(env *testEnv) {
		env.obj = obj
	}
}

// WithStatusSubresource provides an option to set objects for the fake client.
func WithStatusSubresource(obj ...client.Object) FakeKubeClientOption {
	return func(env *testEnv) {
		env.subs = obj
	}
}

// FakeKubeClient creates a fake kube client with some defaults and optional arguments.
func (t *testEnv) FakeKubeClient(opts ...FakeKubeClientOption) client.Client {
	for _, o := range opts {
		o(t)
	}

	return fake.NewClientBuilder().WithScheme(t.scheme).WithObjects(t.obj...).WithStatusSubresource(t.subs...).WithIndex(&v1alpha1.ProductDeploymentPipeline{}, controllerMetadataKey, func(obj client.Object) []string {
		return []string{obj.GetName()}
	}).
		WithIndex(&corev1.ConfigMap{}, controllerMetadataKey, func(obj client.Object) []string {
			owner := obj.GetLabels()[v1alpha1.ProductDeploymentOwnerLabelKey]
			return []string{owner}
		}).
		Build()
}

var (
	env *testEnv
	ctx = ctrl.SetupSignalHandler()
)

func TestMain(m *testing.M) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha12.AddToScheme(scheme)
	_ = projectv1.AddToScheme(scheme)
	_ = replicationv1.AddToScheme(scheme)
	_ = gitv1alpha1delivery.AddToScheme(scheme)
	_ = gitv1alpha1mpas.AddToScheme(scheme)
	_ = sourcebeta2.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	env = &testEnv{
		scheme:       scheme,
		timeout:      time.Second * 20,
		pollInterval: time.Millisecond * 250,
	}

	// call flag.Parse() here because we depend on testing.Short() in our tests
	flag.Parse()

	if !testing.Short() {
		envtest := envtest.New(
			envtest.WithCRDPath(filepath.Join("..", "config", "crd", "bases")),
			envtest.WithMaxConcurrentReconciles(4),
			envtest.WithScheme(scheme),
		)

		env.envtest = envtest

		if err := (&TargetReconciler{
			Client:        envtest,
			EventRecorder: record.NewFakeRecorder(32),
		}).SetupWithManager(ctx, envtest); err != nil {
			panic(fmt.Sprintf("Failed to start the target reconciler: %v", err))
		}

		go func() {
			fmt.Println("Starting the test environment")
			if err := env.envtest.Start(ctx); err != nil {
				panic(fmt.Sprintf("Failed to start the test environment manager: %v", err))
			}
		}()
		<-env.envtest.Manager.Elected()
	}

	code := m.Run()

	if !testing.Short() {
		fmt.Println("Stopping the test environment")
		if err := env.envtest.Stop(); err != nil {
			panic(fmt.Sprintf("Failed to stop the test environment: %v", err))
		}
	}

	os.Exit(code)
}

// waitForReadyConditionis a generic test helper to wait for an object to be
// ready.
func waitForReadyCondition(ctx context.Context, t *testing.T, c client.Client, obj conditions.Setter) {
	key := client.ObjectKeyFromObject(obj)
	assert.Eventually(t, func() bool {
		if err := c.Get(ctx, key, obj); err != nil {
			return false
		}
		if !conditions.IsReady(obj) {
			return false
		}
		readyCondition := conditions.Get(obj, meta.ReadyCondition)
		statusObservedGen, err := object.GetStatusObservedGeneration(obj)
		if err != nil {
			return false
		}
		return obj.GetGeneration() == readyCondition.ObservedGeneration &&
			obj.GetGeneration() == statusObservedGen
	}, env.timeout, env.pollInterval, "wait for object ready")
}

// equalConditions is a generic test helper to compare two lists of conditions.
func equalConditions(actual, expected []metav1.Condition) bool {
	if len(actual) != len(expected) {
		return false
	}
	for _, cond := range expected {
		if !containsCondition(actual, cond) {
			return false
		}
	}
	return true
}

// containsCondition is a generic test helper to check if a condition is
// present in a list of conditions.
func containsCondition(conditions []metav1.Condition, condition metav1.Condition) bool {
	for _, cond := range conditions {
		if cond.Type == condition.Type &&
			cond.Status == condition.Status &&
			cond.Reason == condition.Reason &&
			cond.Message == condition.Message {
			return true
		}
	}
	return false
}
