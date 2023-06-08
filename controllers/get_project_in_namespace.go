package controllers

import (
	"context"
	"fmt"

	projectv1 "github.com/open-component-model/mpas-project-controller/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetProjectInNamespace returns the Project in the current namespace.
func GetProjectInNamespace(ctx context.Context, c client.Client, namespace string) (*projectv1.Project, error) {
	projectList := &projectv1.ProjectList{}
	if err := c.List(ctx, projectList, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to find project in namespace: %w", err)
	}

	if v := len(projectList.Items); v != 1 {
		return nil, fmt.Errorf("exactly one Project should have been found in namespace %s; got: %d", namespace, v)
	}

	project := &projectList.Items[0]

	return project, nil
}
