package controllers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	v1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchValuesFileContent(t *testing.T) {
	testValues := `backend: {
		cacheAddr: tcp://redis:6379
		replicas: 1
	}
cache: {
		replicas: 1
}
frontend:n {
		color: red
		message: Hello, world!
		replicas: 1
}
`

	dir := t.TempDir()
	values, err := generateTestData(dir, filepath.Join("products", "test-values"), []byte(testValues))
	require.NoError(t, err)

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, string(values))
	}))
	defer testServer.Close()

	artifact := &v1.Artifact{
		Path: "./",
		URL:  testServer.URL,
	}

	content, err := FetchValuesFileContent(context.Background(), "test-values", artifact)
	require.NoError(t, err)
	assert.Equal(t, []byte(testValues), content)
}

func TestFetchValuesFileContentFileDoesNotExist(t *testing.T) {
	dir := t.TempDir()
	values, err := generateTestData(dir, filepath.Join("not-products", "test-values"), []byte(`something: value`))
	require.NoError(t, err)

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, string(values))
	}))
	defer testServer.Close()

	artifact := &v1.Artifact{
		Path: "./",
		URL:  testServer.URL,
	}

	_, err = FetchValuesFileContent(context.Background(), "test-values", artifact)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestFetchValuesFileContentContentIsCorrupted(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "not-tar")
	}))
	defer testServer.Close()

	artifact := &v1.Artifact{
		Path: "./",
		URL:  testServer.URL,
	}

	_, err := FetchValuesFileContent(context.Background(), "test-values", artifact)
	assert.EqualError(t, err, "failed to untar artifact content: requires gzip-compressed body: unexpected EOF")
}

func generateTestData(base, folder string, data []byte) ([]byte, error) {
	if err := os.MkdirAll(filepath.Join(base, folder), 0o777); err != nil {
		return nil, fmt.Errorf("failed to create folder: %w", err)
	}

	if err := os.WriteFile(filepath.Join(base, folder, "config.cue"), data, 0o777); err != nil {
		return nil, fmt.Errorf("failed to write values file: %w", err)
	}

	return buildTar(base)
}

// The source here is located at https://github.com/fluxcd/pkg/blob/2ee90dd5b2ec033f44881f160e29584cceda8f37/oci/client/build.go
// copying here to mimic the same archiving behaviour that the Artifact will
// Build archives the given directory as a tarball to the given local path.
// While archiving, any environment specific data (for example, the user and group name) is stripped from file headers.
// buildTar is a modified version of https://github.com/fluxcd/pkg/blob/2ee90dd5b2ec033f44881f160e29584cceda8f37/oci/client/build.go
func buildTar(sourceDir string) ([]byte, error) {
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("invalid source dir path: %s", sourceDir)
	}

	var b []byte
	buffer := bytes.NewBuffer(b)

	gw := gzip.NewWriter(buffer)
	tw := tar.NewWriter(gw)

	if err := filepath.Walk(sourceDir, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Ignore anything that is not a file or directories e.g. symlinks
		if m := fi.Mode(); !(m.IsRegular() || m.IsDir()) {
			return nil
		}

		header, err := tar.FileInfoHeader(fi, p)
		if err != nil {
			return err
		}
		// The name needs to be modified to maintain directory structure
		// as tar.FileInfoHeader only has access to the base name of the file.
		// Ref: https://golang.org/src/archive/tar/common.go?#L626
		relFilePath := p
		if filepath.IsAbs(sourceDir) {
			relFilePath, err = filepath.Rel(sourceDir, p)
			if err != nil {
				return err
			}
		}
		header.Name = relFilePath

		// Remove any environment specific data.
		header.Gid = 0
		header.Uid = 0
		header.Uname = ""
		header.Gname = ""
		header.ModTime = time.Time{}
		header.AccessTime = time.Time{}
		header.ChangeTime = time.Time{}

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if !fi.Mode().IsRegular() {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			f.Close()
			return err
		}
		if _, err := io.Copy(tw, f); err != nil {
			f.Close()
			return err
		}
		return f.Close()
	}); err != nil {
		tw.Close()
		gw.Close()
		return nil, err
	}
	if err := tw.Close(); err != nil {
		gw.Close()
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}
