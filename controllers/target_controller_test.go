// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"fmt"
	"testing"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/open-component-model/mpas-product-controller/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTargetReconciler_Reconcile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}
	tests := []struct {
		name         string
		secretLabels map[string]string
		secretName   string
		sa           bool
		wantErr      assert.ErrorAssertionFunc
		wantResult   assert.ValueAssertionFunc
	}{
		{
			name: "Target Ready with no service account",
			wantResult: func(t assert.TestingT, a any, b ...any) bool {
				target, ok := a.(*v1alpha1.Target)
				if !ok {
					assert.Fail(t, "a is not a *v1alpha1.Target")
				}

				return equalConditions(target.Status.Conditions,
					[]metav1.Condition{*conditions.TrueCondition(meta.ReadyCondition, meta.SucceededReason, "Target is ready")})
			},
		},
		{
			name: "Target Ready with no secret",
			sa:   true,
			wantResult: func(t assert.TestingT, a any, b ...any) bool {
				target, ok := a.(*v1alpha1.Target)
				if !ok {
					assert.Fail(t, "a is not a *v1alpha1.Target")
				}

				return equalConditions(target.Status.Conditions,
					[]metav1.Condition{*conditions.TrueCondition(meta.ReadyCondition, meta.SucceededReason, "Target is ready")})
			},
		},
		{
			name: "Target Ready with a secret",
			sa:   true,
			secretLabels: map[string]string{
				"test-label": "test-value",
			},
			wantResult: func(t assert.TestingT, a any, b ...any) bool {
				target := a.(*v1alpha1.Target)

				// Check if the object status is valid.
				return equalConditions(target.Status.Conditions,
					[]metav1.Condition{*conditions.TrueCondition(meta.ReadyCondition, meta.SucceededReason, "Target is ready")})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns, err := env.envtest.CreateNamespace(ctx, "mpas-system")
			require.NoError(t, err)

			target := &v1alpha1.Target{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-target",
					Namespace: ns.Name,
				},
				Spec: v1alpha1.TargetSpec{
					Type: v1alpha1.Kubernetes,
					Access: &apiextensionsv1.JSON{
						Raw: []byte(`{"targetNamespace":"test-namespace"}`),
					},
				},
			}

			if tt.sa {
				target.Spec.ServiceAccountName = "test-service-account"
			}

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
				},
				Type: corev1.SecretTypeDockerConfigJson,
				Data: map[string][]byte{
					".dockerconfigjson": []byte(`{"auths":{"test-registry":{"username":"test-user","password":"test-password"}}}`),
				},
			}

			err = env.envtest.CreateAndWait(ctx, target)
			require.NoError(t, err)

			waitForReadyCondition(context.Background(), t, env.envtest, target)

			if tt.secretLabels != nil {
				secret.Labels = tt.secretLabels
				err := env.envtest.CreateAndWait(context.Background(), secret)
				require.NoError(t, err)
				target.Spec.SecretsSelector = &metav1.LabelSelector{
					MatchLabels: tt.secretLabels,
				}
				err = env.envtest.Update(context.Background(), target)
				require.NoError(t, err)
			}

			waitForReadyCondition(context.Background(), t, env.envtest, target)

			t.Log("target", target)
			tt.wantResult(t, target, fmt.Sprintf("Reconcile(%v)", tt.name))
		})
	}

}
