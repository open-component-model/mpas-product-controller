// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"testing"

	gitv1alpha1delivery "github.com/open-component-model/git-controller/apis/delivery/v1alpha1"
	gitv1alpha1mpas "github.com/open-component-model/git-controller/apis/mpas/v1alpha1"
	projectv1 "github.com/open-component-model/mpas-project-controller/api/v1alpha1"
	v1alpha12 "github.com/open-component-model/ocm-controller/api/v1alpha1"
	replicationv1 "github.com/open-component-model/replication-controller/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/open-component-model/mpas-product-controller/api/v1alpha1"
)

type testEnv struct {
	scheme *runtime.Scheme
	obj    []client.Object
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
func WithObjets(obj ...client.Object) FakeKubeClientOption {
	return func(env *testEnv) {
		env.obj = obj
	}
}

// FakeKubeClient creates a fake kube client with some defaults and optional arguments.
func (t *testEnv) FakeKubeClient(opts ...FakeKubeClientOption) client.Client {
	for _, o := range opts {
		o(t)
	}

	return fake.NewClientBuilder().WithScheme(t.scheme).WithObjects(t.obj...).WithIndex(&v1alpha1.ProductDeploymentPipeline{}, pipelineOwnerKey, func(obj client.Object) []string {
		return []string{obj.GetName()}
	}).Build()
}

var env *testEnv

func TestMain(m *testing.M) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha12.AddToScheme(scheme)
	_ = projectv1.AddToScheme(scheme)
	_ = replicationv1.AddToScheme(scheme)
	_ = gitv1alpha1delivery.AddToScheme(scheme)
	_ = gitv1alpha1mpas.AddToScheme(scheme)

	env = &testEnv{
		scheme: scheme,
	}
	m.Run()
}
