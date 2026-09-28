// Package filetree centralizes boot-file namespaces. Actual IO uses os.Root,
// so symlinks and rename races cannot escape the configured directory.
package filetree

import (
	"os"
	"path/filepath"
	"strings"
)

func Path(name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	if name == "" {
		name = "."
	}
	if !filepath.IsLocal(name) {
		return "", os.ErrPermission
	}
	return filepath.Clean(name), nil
}

func Resolve(root, downloadDir, name string) (string, string, error) {
	name = strings.TrimLeft(strings.ReplaceAll(name, "\\", "/"), "/")
	if rel, ok := strings.CutPrefix(name, "netboot/"); ok {
		root = downloadDir
		name = rel
	}
	rel, err := Path(name)
	return root, rel, err
}
