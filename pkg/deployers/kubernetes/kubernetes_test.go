// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"
	"testing"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/open-component-model/mpas-product-controller/api/v1alpha1"
	ocmv1alpha1 "github.com/open-component-model/ocm-controller/api/v1alpha1"
	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestDeploy(t *testing.T) {
	kubeTarget := &v1alpha1.Target{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kube-target",
			Namespace: "mpas-system",
		},
		Spec: v1alpha1.TargetSpec{
			Type: v1alpha1.Kubernetes,
			Access: &apiextensionsv1.JSON{
				Raw: []byte(`secretRef:
  name: secret
targetNamespace: new-namespace
`),
			},
		},
	}
	configuration := &ocmv1alpha1.Configuration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "owner-configuration",
			Namespace: "mpas-system",
		},
	}

	snapshot := &ocmv1alpha1.Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot",
			Namespace: "mpas-system",
		},
		Spec: ocmv1alpha1.SnapshotSpec{
			Identity: nil,
			Digest:   "",
			Tag:      "",
			Suspend:  false,
		},
		Status: ocmv1alpha1.SnapshotStatus{
			Conditions:           nil,
			LastReconciledDigest: "",
			LastReconciledTag:    "",
			RepositoryURL:        "",
			ObservedGeneration:   0,
		},
	}

	err := controllerutil.SetOwnerReference(configuration, snapshot, env.scheme)
	require.NoError(t, err)

	obj := &v1alpha1.ProductDeploymentPipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline",
			Namespace: "mpas-system",
		},
		Status: v1alpha1.ProductDeploymentPipelineStatus{
			SelectedTarget: kubeTarget,
			SnapshotRef: &meta.NamespacedObjectReference{
				Name:      snapshot.Name,
				Namespace: "mpas-system",
			},
		},
	}
	fakeClient := env.FakeKubeClient(WithObjets(obj, configuration, snapshot, kubeTarget))
	deployer := NewDeployer(fakeClient, env.scheme, nil)

	err = deployer.Deploy(context.Background(), obj)
	require.NoError(t, err)
}
