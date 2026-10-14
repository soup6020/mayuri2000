package imaging

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"mayuri2000/assets"
	"mayuri2000/internal/sandbox"
)

// The support directory holds read-only files every ImageMagick job may
// read: the security policy and the caption font. It's created once per
// process and registered with the sandbox; nothing in it is secret.
var support struct {
	once sync.Once
	dir  string
	err  error
}

func supportDir() (string, error) {
	support.once.Do(func() {
		support.dir, support.err = writeSupportDir()
		if support.err == nil {
			sandbox.AllowReadOnly(support.dir)
			sandbox.SetEnv("MAGICK_CONFIGURE_PATH", support.dir)
		}
	})
	return support.dir, support.err
}

func writeSupportDir() (string, error) {
	dir, err := os.MkdirTemp("", "mayuri-magick-support-*")
	if err != nil {
		return "", err
	}
	font, err := assets.FS.ReadFile("fonts/arial.ttf")
	if err != nil {
		return "", errors.Join(err, os.RemoveAll(dir))
	}
	for name, data := range map[string][]byte{"arial.ttf": font, "policy.xml": magickPolicy} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o444); err != nil {
			return "", errors.Join(err, os.RemoveAll(dir))
		}
	}
	return dir, nil
}

// arialPath is the caption font inside the support directory.
func arialPath() (string, error) {
	dir, err := supportDir()
	return filepath.Join(dir, "arial.ttf"), err
}

// CleanupSupport removes the support directory; call it on shutdown.
func CleanupSupport() {
	if support.dir != "" {
		_ = os.RemoveAll(support.dir)
	}
}
