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
	"github.com/fluxcd/pkg/runtime/object"
	"github.com/open-component-model/mpas-product-controller/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestTargetReconciler_Reconcile(t *testing.T) {
	target := &v1alpha1.Target{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-target",
		},
		Spec: v1alpha1.TargetSpec{
			Type: v1alpha1.Kubernetes,
			Access: &apiextensionsv1.JSON{
				Raw: []byte(`{"targetNamespace":"test-namespace"}`),
			},
			ServiceAccountName: "test-service-account",
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test-namespace",
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			".dockercfg": []byte(`{"auths":{"test-registry":{"username":"test-user","password":"test-password"}}}`),
		},
	}

	tests := []struct {
		name         string
		secretLabels map[string]string
		secretName   string
		wantErr      assert.ErrorAssertionFunc
		wantResult   assert.ValueAssertionFunc
	}{
		{
			name: "Target Ready with no secret",
			wantResult: func(t assert.TestingT, a any, b ...any) bool {
				target := a.(*v1alpha1.Target)
				// Check if the object status is valid.
				return assert.Equal(t,
					[]metav1.Condition{*conditions.TrueCondition(meta.ReadyCondition, meta.SucceededReason, "Target is ready")},
					target.Status.Conditions,
				)
			},
			wantErr: assert.NoError,
		},
		{
			name: "Target Ready with a secret",
			secretLabels: map[string]string{
				"test-label": "test-value",
			},
			wantResult: func(t assert.TestingT, a any, b ...any) bool {
				target := a.(*v1alpha1.Target)
				// Check if the object status is valid.

				return assert.Equal(t,
					[]metav1.Condition{*conditions.TrueCondition(meta.ReadyCondition, meta.SucceededReason, "Target is ready")},
					target.Status.Conditions,
				)
			},
			wantErr: assert.NoError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := env.FakeKubeClient(WithObjects(target))
			r := &TargetReconciler{
				Client:        fakeClient,
				Scheme:        env.scheme,
				EventRecorder: record.NewFakeRecorder(32),
			}

			_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: target.Name}})

			waitForReadyCondition(context.Background(), t, fakeClient, target)

			if tt.secretLabels != nil {
				secret.Labels = tt.secretLabels
				fakeClient.Create(context.Background(), secret)
				target.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: tt.secretLabels,
				}
				_, err = r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: target.Name}})
				waitForReadyCondition(context.Background(), t, fakeClient, target)
			}

			tt.wantErr(t, err, fmt.Sprintf("Reconcile(%v)", tt.name))
			tt.wantResult(t, target, fmt.Sprintf("Reconcile(%v)", tt.name))
		})
	}

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
	}, env.timeout, env.pollInterval)
}
