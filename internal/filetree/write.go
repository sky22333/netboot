package filetree

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var mutations sync.Mutex
var ErrConflict = errors.New("文件已存在或已被修改，请刷新后重试")

func Revision(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }

// Write publishes only a complete, synced file. Creation never overwrites.
func Write(ctx context.Context, root *os.Root, name string, src io.Reader, size int64, replace bool) error {
	temp := filepath.Join(filepath.Dir(name), UploadTempPrefix+rand.Text())
	f, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(temp)
	defer f.Close()
	n, err := io.Copy(f, src)
	if err != nil {
		return err
	}
	if n != size {
		return io.ErrUnexpectedEOF
	}
	if err = f.Chmod(0644); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if replace {
		return root.Rename(temp, name)
	}
	return root.Link(temp, name)
}

func WriteText(ctx context.Context, root *os.Root, name, content, revision string) error {
	mutations.Lock()
	defer mutations.Unlock()
	if revision != "" {
		info, err := root.Lstat(name)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
			return ErrConflict
		}
		f, err := root.Open(name)
		if err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
		f.Close()
		if err != nil {
			return err
		}
		if Revision(data) != revision {
			return ErrConflict
		}
	}
	err := Write(ctx, root, name, strings.NewReader(content), int64(len(content)), revision != "")
	if errors.Is(err, os.ErrExist) {
		return ErrConflict
	}
	return err
}

func Remove(root *os.Root, name string) error {
	mutations.Lock()
	defer mutations.Unlock()
	return root.Remove(name)
}

func Rename(root *os.Root, from, to string) error {
	mutations.Lock()
	defer mutations.Unlock()
	if filepath.Dir(from) != filepath.Dir(to) {
		return fmt.Errorf("只能在同一目录内重命名")
	}
	parent, err := root.OpenRoot(filepath.Dir(from))
	if err != nil {
		return err
	}
	defer parent.Close()
	dir, err := parent.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	if err = renameNoReplace(dir, filepath.Base(from), filepath.Base(to)); err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	return nil
}
