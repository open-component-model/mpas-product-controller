package controllers

import (
	"fmt"
	"strings"

	v1 "github.com/fluxcd/source-controller/api/v1"
	projectv1 "github.com/open-component-model/mpas-project-controller/api/v1alpha1"
)

// FetchGitRepositoryFromProjectInventory looks for the GitRepository in the project's inventory.
// There should ever only be one.
func FetchGitRepositoryFromProjectInventory(project *projectv1.Project) (string, string, error) {
	// Entry ID: <namespace>_<name>_<group>_<kind>. Just look for a postfix of gitrepository
	if project.Status.Inventory == nil {
		return "", "", fmt.Errorf("project inventory is empty")
	}

	var repoName, repoNamespace string
	for _, e := range project.Status.Inventory.Entries {
		split := strings.Split(e.ID, "_")
		splitLength := 2
		if len(split) < splitLength {
			return "", "", fmt.Errorf("failed to split ID: %s", e.ID)
		}

		if split[len(split)-1] == v1.GitRepositoryKind {
			repoName = split[1]
			repoNamespace = split[0]

			break
		}
	}

	if repoName == "" {
		return "", "", fmt.Errorf("gitrepository not found in the project inventory")
	}

	return repoName, repoNamespace, nil
}
