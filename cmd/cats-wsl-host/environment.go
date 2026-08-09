//go:build linux

package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/rohanthewiz/cats/internal/shellenv"
)

const (
	wslPathMarker  = "__CATS_WSL_PATH__"
	wslPathTimeout = 5 * time.Second
)

type userIdentity struct {
	uid      int
	username string
	home     string
	shell    string
}

func hydratedUserEnvironment(logWriter io.Writer) ([]string, userIdentity, error) {
	current, err := user.Current()
	if err != nil {
		return nil, userIdentity{}, fmt.Errorf("current user: %w", err)
	}
	uid, err := uidNumber(current)
	if err != nil {
		return nil, userIdentity{}, err
	}
	if !filepath.IsAbs(current.HomeDir) {
		return nil, userIdentity{}, fmt.Errorf("current user home is not absolute: %q", current.HomeDir)
	}
	shell := chooseShell(os.Getenv("SHELL"), passwdShell(current.Uid))
	identity := userIdentity{uid: uid, username: current.Username, home: current.HomeDir, shell: shell}
	env := os.Environ()
	env = setEnvironment(env, "HOME", identity.home)
	env = setEnvironment(env, "USER", identity.username)
	env = setEnvironment(env, "LOGNAME", identity.username)
	env = setEnvironment(env, "SHELL", identity.shell)

	derived, err := loginShellPATH(identity, env)
	if err != nil {
		fmt.Fprintf(logWriter, "cats-wsl-host: login shell PATH unavailable: %v; using inherited PATH\n", err)
		return env, identity, nil
	}
	env = setEnvironment(env, "PATH", shellenv.MergePATH(derived, environmentValue(env, "PATH")))
	return env, identity, nil
}

func chooseShell(candidates ...string) string {
	for _, candidate := range candidates {
		if !filepath.IsAbs(candidate) {
			continue
		}
		st, err := os.Stat(candidate)
		if err == nil && st.Mode().IsRegular() && st.Mode().Perm()&0o111 != 0 {
			return candidate
		}
	}
	return "/bin/sh"
}

func passwdShell(uid string) string {
	f, err := os.Open("/etc/passwd")
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), ":")
		if len(fields) >= 7 && fields[2] == uid {
			return fields[6]
		}
	}
	return ""
}

func loginShellPATH(identity userIdentity, env []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), wslPathTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, identity.shell, "-ilc",
		`printf '`+wslPathMarker+`%s`+wslPathMarker+`' "$PATH"`)
	cmd.Env = env
	cmd.Dir = identity.home
	out, err := cmd.Output()
	value := shellenv.BetweenMarkers(string(out), wslPathMarker)
	if value == "" {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("%s returned no marked PATH: %w", identity.shell, err)
	}
	return value, nil
}

func setEnvironment(env []string, key, value string) []string {
	prefix := key + "="
	result := make([]string, 0, len(env)+1)
	replaced := false
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			if !replaced {
				result = append(result, prefix+value)
				replaced = true
			}
			continue
		}
		result = append(result, entry)
	}
	if !replaced {
		result = append(result, prefix+value)
	}
	return result
}

func environmentValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}
