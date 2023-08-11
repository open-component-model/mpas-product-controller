// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"fmt"
	"testing"

	"github.com/open-component-model/mpas-product-controller/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestTargetSelection(t *testing.T) {
	testCases := []struct {
		name    string
		targets []client.Object
		role    v1alpha1.TargetRole
		result  v1alpha1.Target
		random  bool
		err     error
	}{
		{
			name: "single target",
			targets: []client.Object{
				&v1alpha1.Target{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-target",
						Namespace: "default",
						Labels: map[string]string{
							"label1": "value",
						},
					},
					Spec: v1alpha1.TargetSpec{
						Type:   v1alpha1.Kubernetes,
						Access: nil,
					},
				},
			},
			role: v1alpha1.TargetRole{
				Type: v1alpha1.Kubernetes,
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"label1": "value",
					},
				},
			},
			result: v1alpha1.Target{
				ObjectMeta: metav1.ObjectMeta{Name: "kube-target", Namespace: "default"},
			},
		},
		{
			name: "fallback to mpas-system",
			targets: []client.Object{
				&v1alpha1.Target{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-target-1",
						Namespace: "ocm-system", // to verify that there are targets just not in the default namespace
						Labels: map[string]string{
							"label1": "value",
						},
					},
					Spec: v1alpha1.TargetSpec{
						Type:   v1alpha1.Kubernetes,
						Access: nil,
					},
				},
				&v1alpha1.Target{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-target-2",
						Namespace: "mpas-system",
						Labels: map[string]string{
							"label1": "value",
						},
					},
					Spec: v1alpha1.TargetSpec{
						Type:   v1alpha1.Kubernetes,
						Access: nil,
					},
				},
			},
			role: v1alpha1.TargetRole{
				Type: v1alpha1.Kubernetes,
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"label1": "value",
					},
				},
			},
			result: v1alpha1.Target{
				ObjectMeta: metav1.ObjectMeta{Name: "kube-target-2", Namespace: "mpas-system"},
			},
		},
		{
			name: "multiple targets of same type",
			targets: []client.Object{
				&v1alpha1.Target{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-target",
						Namespace: "default",
						Labels: map[string]string{
							"label1": "value",
						},
					},
					Spec: v1alpha1.TargetSpec{
						Type:   v1alpha1.Kubernetes,
						Access: nil,
					},
				},
				&v1alpha1.Target{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-target-2",
						Namespace: "default",
						Labels: map[string]string{
							"label1": "value",
						},
					},
					Spec: v1alpha1.TargetSpec{
						Type:   v1alpha1.Kubernetes,
						Access: nil,
					},
				},
			},
			role: v1alpha1.TargetRole{
				Type: v1alpha1.Kubernetes,
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"label1": "value",
					},
				},
			},
			result: v1alpha1.Target{
				ObjectMeta: metav1.ObjectMeta{Name: "kube-target", Namespace: "default"},
				Spec: v1alpha1.TargetSpec{
					Type: v1alpha1.Kubernetes,
				},
			},
			random: true,
		},
		{
			name: "multiple target of different types",
			targets: []client.Object{
				&v1alpha1.Target{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-target",
						Namespace: "default",
						Labels: map[string]string{
							"label1": "value",
						},
					},
					Spec: v1alpha1.TargetSpec{
						Type:   v1alpha1.OCIRepository,
						Access: nil,
					},
				},
				&v1alpha1.Target{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-target-2",
						Namespace: "default",
						Labels: map[string]string{
							"label1": "value",
						},
					},
					Spec: v1alpha1.TargetSpec{
						Type:   v1alpha1.Kubernetes,
						Access: nil,
					},
				},
			},
			role: v1alpha1.TargetRole{
				Type: v1alpha1.Kubernetes,
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"label1": "value",
					},
				},
			},
			result: v1alpha1.Target{
				ObjectMeta: metav1.ObjectMeta{Name: "kube-target-2", Namespace: "default"},
			},
		},
		{
			name: "multiple target of different types and same types but different labels with single result",
			targets: []client.Object{
				&v1alpha1.Target{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-target",
						Namespace: "default",
						Labels: map[string]string{
							"label1": "value",
						},
					},
					Spec: v1alpha1.TargetSpec{
						Type:   v1alpha1.OCIRepository,
						Access: nil,
					},
				},
				&v1alpha1.Target{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-target-2",
						Namespace: "default",
						Labels: map[string]string{
							"label2": "value",
						},
					},
					Spec: v1alpha1.TargetSpec{
						Type:   v1alpha1.OCIRepository,
						Access: nil,
					},
				},
				&v1alpha1.Target{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-target-3",
						Namespace: "default",
						Labels: map[string]string{
							"label3": "value",
						},
					},
					Spec: v1alpha1.TargetSpec{
						Type:   v1alpha1.Kubernetes,
						Access: nil,
					},
				},
			},
			role: v1alpha1.TargetRole{
				Type: v1alpha1.OCIRepository,
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"label1": "value",
					},
				},
			},
			result: v1alpha1.Target{
				ObjectMeta: metav1.ObjectMeta{Name: "kube-target", Namespace: "default"},
			},
		},
		{
			name: "no target type",
			targets: []client.Object{
				&v1alpha1.Target{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-target",
						Namespace: "default",
						Labels: map[string]string{
							"label1": "value",
						},
					},
					Spec: v1alpha1.TargetSpec{
						Type:   v1alpha1.Kubernetes,
						Access: nil,
					},
				},
			},
			role: v1alpha1.TargetRole{
				Type: v1alpha1.OCIRepository,
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"label1": "value",
					},
				},
			},
			err: fmt.Errorf("no targets found using the provided selector filters"),
		},
		{
			name: "no target label matches",
			err:  fmt.Errorf("no targets found using the provided selector filters"),
			targets: []client.Object{
				&v1alpha1.Target{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-target",
						Namespace: "default",
						Labels: map[string]string{
							"label2": "value",
						},
					},
					Spec: v1alpha1.TargetSpec{
						Type:   v1alpha1.OCIRepository,
						Access: nil,
					},
				},
			},
			role: v1alpha1.TargetRole{
				Type: v1alpha1.OCIRepository,
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"label1": "value",
					},
				},
			},
		},
		{
			name: "using match expression",
			targets: []client.Object{
				&v1alpha1.Target{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-target",
						Namespace: "default",
						Labels: map[string]string{
							"label1": "in-list",
						},
					},
					Spec: v1alpha1.TargetSpec{
						Type:   v1alpha1.OCIRepository,
						Access: nil,
					},
				},
			},
			role: v1alpha1.TargetRole{
				Type: v1alpha1.OCIRepository,
				Selector: metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "label1",
							Operator: "In",
							Values:   []string{"optional", "in-list"},
						},
					},
				},
			},
			result: v1alpha1.Target{
				ObjectMeta: metav1.ObjectMeta{Name: "kube-target", Namespace: "default"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Helper()

			fakeClient := env.FakeKubeClient(WithObjets(tc.targets...))

			r := &ProductDeploymentPipelineScheduler{
				Client:              fakeClient,
				Scheme:              env.scheme,
				MpasSystemNamespace: "mpas-system",
			}
			target, err := r.SelectTarget(context.Background(), tc.role, "default")
			if tc.err != nil {
				require.ErrorContains(t, err, tc.err.Error())
			} else {
				require.NoError(t, err)
			}

			if tc.random {
				assert.Equal(t, target.Spec.Type, tc.result.Spec.Type)
			} else {
				assert.Equal(t, target.Name, tc.result.Name)
				assert.Equal(t, target.Namespace, tc.result.Namespace)
			}
		})
	}
}
