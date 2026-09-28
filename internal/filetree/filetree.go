// Package filetree centralizes boot-file namespaces. Actual IO uses os.Root,
// so symlinks and rename races cannot escape the configured directory.
package filetree

import (
	"os"
	"path/filepath"
	"strings"
)

const UploadTempPrefix = ".pxe-upload-"

func IsUploadTemp(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), UploadTempPrefix)
}

func Path(name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	if name == "" {
		name = "."
	}
	for _, part := range strings.Split(name, "/") {
		if IsUploadTemp(part) {
			return "", os.ErrPermission
		}
	}
	if !filepath.IsLocal(name) {
		return "", os.ErrPermission
	}
	return filepath.Clean(name), nil
}
