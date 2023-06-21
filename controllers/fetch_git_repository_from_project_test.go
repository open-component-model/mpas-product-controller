package controllers

import (
	"testing"

	projectv1 "github.com/open-component-model/mpas-project-controller/api/v1alpha1"
	"github.com/stretchr/testify/assert"
)

func TestFetchGitRepositoryFromProjectInventory(t *testing.T) {
	testcases := []struct {
		name          string
		project       *projectv1.Project
		err           string
		repoName      string
		repoNamespace string
	}{
		{
			name: "git repository exists",
			project: &projectv1.Project{
				Status: projectv1.ProjectStatus{
					Inventory: &projectv1.ResourceInventory{
						Entries: []projectv1.ResourceRef{
							{
								ID:      "mpas-system_repo_group_GitRepository",
								Version: "v0.0.1",
							},
						},
					},
				},
			},
			repoName:      "repo",
			repoNamespace: "mpas-system",
		},
		{
			name: "git repository doesn't exist",
			project: &projectv1.Project{
				Status: projectv1.ProjectStatus{
					Inventory: &projectv1.ResourceInventory{
						Entries: []projectv1.ResourceRef{},
					},
				},
			},
			err: "gitrepository not found in the project inventory",
		},
		{
			name: "entry is in incorrect format",
			project: &projectv1.Project{
				Status: projectv1.ProjectStatus{
					Inventory: &projectv1.ResourceInventory{
						Entries: []projectv1.ResourceRef{
							{
								ID:      "mpas-GitRepository",
								Version: "v0.0.1",
							},
						},
					},
				},
			},
			err: "failed to split ID: mpas-GitRepository",
		},
		{
			name: "inventory is empty",
			project: &projectv1.Project{
				Status: projectv1.ProjectStatus{},
			},
			err: "project inventory is empty",
		},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Helper()

			name, namespace, err := FetchGitRepositoryFromProjectInventory(tc.project)
			if tc.err != "" {
				assert.EqualError(t, err, tc.err)
			} else {
				assert.Equal(t, tc.repoName, name)
				assert.Equal(t, tc.repoNamespace, namespace)
			}
		})
	}
}
