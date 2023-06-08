package rego

import (
	"context"
	"testing"

	"github.com/open-component-model/mpas-product-controller/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidationSuccess(t *testing.T) {
	m := map[string]any{
		"backend": map[string]any{
			"replicas": 1,
		},
	}
	rules := v1alpha1.ValidationData{
		Data: []byte(`package main

deny[msg] {
  not input.replicas

  msg := "Replicas must be set"
}

deny[msg] {
  input.replicas != 1

  msg := "Replicas must equal 1"
}`),
		Name: "backend",
	}

	outcome, reason, err := ValidateRules(context.Background(), rules, m)
	require.NoError(t, err)

	assert.True(t, outcome)
	assert.Empty(t, reason)
}

func TestValidationFails(t *testing.T) {
	m := map[string]any{
		"backend": map[string]any{
			"message": "msg",
		},
	}
	rules := v1alpha1.ValidationData{
		Data: []byte(`package main

deny[msg] {
  not input.replicas

  msg := "Replicas must be set"
}

deny[msg] {
  input.replicas != 1

  msg := "Replicas must equal 1"
}`),
		Name: "backend",
	}

	outcome, reason, err := ValidateRules(context.Background(), rules, m)
	require.NoError(t, err)

	assert.False(t, outcome)
	assert.Equal(t, "result from binding: Replicas must be set", reason)
}
