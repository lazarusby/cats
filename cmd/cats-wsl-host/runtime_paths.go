//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Linux sockaddr_un.sun_path has 108 bytes including its terminating NUL.
const maxUnixSocketPathBytes = 107

type runtimePaths struct {
	dir  string
	th   string
	ctl  string
	hook string
}

func allocateRuntimeDir(launchID string, uid int) (runtimePaths, error) {
	if !validLaunchID(launchID) {
		return runtimePaths{}, fmt.Errorf("invalid launch id %q", launchID)
	}
	if uid < 0 {
		return runtimePaths{}, fmt.Errorf("invalid uid %d", uid)
	}

	if root := os.Getenv("XDG_RUNTIME_DIR"); root != "" {
		if err := validatePrivateOwnedDir(root, uid); err == nil {
			parent := filepath.Join(root, "cats")
			if err := ensurePrivateOwnedDir(parent, uid); err == nil {
				candidate := filepath.Join(parent, launchID)
				paths := newRuntimePaths(candidate)
				if paths.socketPathsFit() {
					if err := os.Mkdir(candidate, 0o700); err == nil {
						return paths, nil
					}
				}
			}
		}
	}

	prefix := "cats-wsl-" + strconv.Itoa(uid) + "-"
	tempRoot := filepath.Clean(os.TempDir())
	if !filepath.IsAbs(tempRoot) || tempRoot == string(filepath.Separator) {
		return runtimePaths{}, fmt.Errorf("temporary runtime root is unsafe: %q", tempRoot)
	}
	dir, err := os.MkdirTemp(tempRoot, prefix)
	if err != nil {
		return runtimePaths{}, fmt.Errorf("create private temporary runtime directory: %w", err)
	}
	paths := newRuntimePaths(dir)
	if !paths.socketPathsFit() {
		_ = os.Remove(dir)
		return runtimePaths{}, fmt.Errorf("temporary runtime path is too long for Unix sockets: %s", dir)
	}
	return paths, nil
}

func newRuntimePaths(dir string) runtimePaths {
	return runtimePaths{
		dir:  filepath.Clean(dir),
		th:   filepath.Join(dir, "th.sock"),
		ctl:  filepath.Join(dir, "ctl.sock"),
		hook: filepath.Join(dir, "hook.sock"),
	}
}

func (p runtimePaths) socketPathsFit() bool {
	return len(p.th) <= maxUnixSocketPathBytes &&
		len(p.ctl) <= maxUnixSocketPathBytes &&
		len(p.hook) <= maxUnixSocketPathBytes
}

func validatePrivateOwnedDir(path string, uid int) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("runtime root is not absolute: %q", path)
	}
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return fmt.Errorf("runtime root is not a directory")
	}
	if st.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("runtime root has group/world permissions %04o", st.Mode().Perm())
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok || int(sys.Uid) != uid {
		return fmt.Errorf("runtime root is not owned by uid %d", uid)
	}
	return nil
}

func ensurePrivateOwnedDir(path string, uid int) error {
	if err := os.Mkdir(path, 0o700); err != nil && !os.IsExist(err) {
		return err
	}
	return validatePrivateOwnedDir(path, uid)
}

// cleanup removes only the three resolved socket paths and then the exact
// directory created for this launch. It intentionally does not recurse.
func (p runtimePaths) cleanup() error {
	if err := p.validateForCleanup(); err != nil {
		return err
	}
	var errs []error
	for _, socket := range []string{p.th, p.ctl, p.hook} {
		if err := os.Remove(socket); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("remove %s: %w", filepath.Base(socket), err))
		}
	}
	if err := os.Remove(p.dir); err != nil && !os.IsNotExist(err) {
		errs = append(errs, fmt.Errorf("remove runtime directory: %w", err))
	}
	return joinErrors(errs)
}

func (p runtimePaths) validateForCleanup() error {
	if p.dir == "" || !filepath.IsAbs(p.dir) || filepath.Clean(p.dir) != p.dir || p.dir == string(filepath.Separator) {
		return fmt.Errorf("refusing unsafe runtime cleanup path %q", p.dir)
	}
	components := strings.Split(strings.Trim(p.dir, string(filepath.Separator)), string(filepath.Separator))
	if len(components) < 2 {
		return fmt.Errorf("refusing broad runtime cleanup path %q", p.dir)
	}
	want := newRuntimePaths(p.dir)
	if p.th != want.th || p.ctl != want.ctl || p.hook != want.hook {
		return fmt.Errorf("refusing runtime cleanup with unresolved socket paths")
	}
	return nil
}

func joinErrors(errs []error) error {
	return errors.Join(errs...)
}
