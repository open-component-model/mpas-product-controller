package controllers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/fluxcd/pkg/untar"
	v1 "github.com/fluxcd/source-controller/api/v1"
)

// FetchValuesFileContent takes a product name and a GitRepository artifact to fetch a values file if it exists.
func FetchValuesFileContent(ctx context.Context, productName string, artifact *v1.Artifact) (_ []byte, err error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, artifact.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to construct request: %w", err)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("failed to download artifact from git repository: %w", err)
	}

	defer func() {
		if berr := response.Body.Close(); berr != nil {
			err = errors.Join(err, berr)
		}
	}()

	temp, err := os.MkdirTemp("", "artifact")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp folder: %w", err)
	}

	defer func() {
		if oerr := os.RemoveAll(temp); oerr != nil {
			err = errors.Join(err, oerr)
		}
	}()

	if _, err := untar.Untar(response.Body, temp); err != nil {
		return nil, fmt.Errorf("failed to untar artifact content: %w", err)
	}

	path := filepath.Join(temp, "products", productName, "values.yaml")

	if _, err := os.Stat(path); os.IsNotExist(err) {
		// return the error as is without wrapping. this is intentional.
		return nil, err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read values yaml: %w", err)
	}

	return content, nil
}
