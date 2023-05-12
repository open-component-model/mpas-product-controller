package controllers

import (
	"context"
	"testing"

	"github.com/open-component-model/mpas-product-controller/api/v1alpha1"
	ocmmetav1 "github.com/open-component-model/ocm/pkg/contexts/ocm/compdesc/meta/v1"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/compdesc/versions/ocm.software/v3alpha1"
	replicationv1 "github.com/open-component-model/replication-controller/api/v1alpha1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestProductDeploymentReconciler(t *testing.T) {
	deployment := &v1alpha1.ProductDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "default",
		},
		Spec: v1alpha1.ProductDeploymentSpec{
			Component: replicationv1.Component{
				Name:    "github.com/test/test-component",
				Version: "1.0.0",
			},
			Pipelines: []v1alpha1.Pipeline{
				{
					Name: "backend",
					Resource: v1alpha1.ResourceReference{
						ElementMeta: v3alpha1.ElementMeta{
							Name:    "manifests",
							Version: "1.0.0",
						},
						ReferencePath: []ocmmetav1.Identity{
							{
								"name": "backend",
							},
						},
					},
					Localization: v1alpha1.ResourceReference{
						ElementMeta: v3alpha1.ElementMeta{
							Name:    "config",
							Version: "1.0.0",
						},
					},
					Configuration: v1alpha1.Configuration{},
					TargetRole: v1alpha1.TargetRole{
						Type:     "kubernetes",
						Selector: metav1.LabelSelector{},
					},
				},
			},
		},
	}

	fakeKube := env.FakeKubeClient(WithObjets(deployment), WithAddToScheme(v1alpha1.AddToScheme))

	reconciler := ProductDeploymentReconciler{
		Client: fakeKube,
		Scheme: env.scheme,
	}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      deployment.Name,
			Namespace: deployment.Namespace,
		},
	})

	require.NoError(t, err)
}
