package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// checkDataDir fails early, with the fix, when the database directory isn't
// writable. A volume created by an older image that ran as root is the usual
// cause; SQLite would otherwise only fail at the first write, obscurely.
func checkDataDir(dbPath string) error {
	dir := filepath.Dir(dbPath)
	f, err := os.CreateTemp(dir, ".mayuri-write-check-*")
	if errors.Is(err, fs.ErrPermission) {
		uid, gid := os.Getuid(), os.Getgid()
		return fmt.Errorf("database directory %s is not writable by uid %d (gid %d); change its owner, e.g. chown -R %d:%d %s",
			dir, uid, gid, uid, gid, dir)
	}
	if err != nil {
		return fmt.Errorf("checking database directory: %w", err)
	}
	name := f.Name()
	return errors.Join(f.Close(), os.Remove(name))
}
