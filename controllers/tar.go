package controllers

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Untar writes a tar stream to a filesystem.
func Untar(in io.Reader, dir string) error {
	tr := tar.NewReader(in)
	for {
		header, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		abs := filepath.Join(dir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(abs, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("unable to create directory %s: %w", header.Name, err)
			}
		case tar.TypeReg:
			file, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("unable to open file %s: %w", header.Name, err)
			}
			//nolint:gosec // We don't know what size limit we could set, the tar
			// archive can be an image layer and that can even reach the gigabyte range.
			// For now, we acknowledge the risk.
			//
			// We checked other softwares and tried to figure out how they manage this,
			// but it's handled the same way.
			if _, err := io.Copy(file, tr); err != nil {
				return fmt.Errorf("unable to copy tar file to filesystem: %w", err)
			}
			if err := file.Close(); err != nil {
				return fmt.Errorf("unable to close file %s: %w", header.Name, err)
			}
		}
	}
}
