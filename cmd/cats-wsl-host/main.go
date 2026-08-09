//go:build linux

// Command cats-wsl-host is the narrow Linux-side owner of a local Windows
// CATS session. It is launched directly by wsl.exe and speaks desktopproto on
// stdout/stdin; all human and child-process logs stay on stderr.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rohanthewiz/cats/internal/buildinfo"
	"github.com/rohanthewiz/cats/internal/desktopproto"
)

const defaultIdleTimeout = 10 * time.Minute

var osReleaseFiles = []string{"/proc/sys/kernel/osrelease", "/proc/version"}

type options struct {
	port        int
	launchID    string
	startDir    string
	idleTimeout time.Duration
	healthJSON  bool
	allowNonWSL bool
}

type healthReport struct {
	Architecture  string `json:"architecture"`
	HelperVersion string `json:"helper_version"`
	Home          string `json:"home"`
	PayloadPath   string `json:"payload_path"`
}

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func runCLI(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	opts, err := parseOptions(args, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "cats-wsl-host:", err)
		return 2
	}
	if err := requireWSL(opts.allowNonWSL); err != nil {
		fmt.Fprintln(stderr, "cats-wsl-host:", err)
		return 1
	}
	payloadDir, err := siblingPayloadDir()
	if err != nil {
		fmt.Fprintln(stderr, "cats-wsl-host:", err)
		return 1
	}

	if opts.healthJSON {
		if err := verifyPayload(payloadDir); err != nil {
			fmt.Fprintln(stderr, "cats-wsl-host:", err)
			return 1
		}
		current, err := user.Current()
		if err != nil {
			fmt.Fprintln(stderr, "cats-wsl-host: current user:", err)
			return 1
		}
		report := healthReport{
			Architecture:  runtime.GOARCH,
			HelperVersion: appVersion(),
			Home:          current.HomeDir,
			PayloadPath:   payloadDir,
		}
		if err := json.NewEncoder(stdout).Encode(report); err != nil {
			fmt.Fprintln(stderr, "cats-wsl-host: write health report:", err)
			return 1
		}
		return 0
	}

	protocol := protocolOutput{w: stdout}
	if err := protocol.write(desktopproto.NewStarting(appVersion(), os.Getpid())); err != nil {
		fmt.Fprintln(stderr, "cats-wsl-host: write starting record:", err)
		return 1
	}
	if err := verifyPayload(payloadDir); err != nil {
		return reportFailure(protocol, stderr, "payload", err)
	}

	env, identity, err := hydratedUserEnvironment(stderr)
	if err != nil {
		return reportFailure(protocol, stderr, "environment", err)
	}
	startDir, err := resolveStartDir(opts.startDir, identity.home)
	if err != nil {
		return reportFailure(protocol, stderr, "start_dir", err)
	}
	runtimeDir, err := allocateRuntimeDir(opts.launchID, identity.uid)
	if err != nil {
		return reportFailure(protocol, stderr, "runtime_dir", err)
	}
	defer func() {
		if err := runtimeDir.cleanup(); err != nil {
			fmt.Fprintln(stderr, "cats-wsl-host: cleanup runtime directory:", err)
		}
	}()

	control := readControl(stdin)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)

	s := session{
		opts:       opts,
		payloadDir: payloadDir,
		startDir:   startDir,
		env:        env,
		paths:      runtimeDir,
		stderr:     stderr,
		control:    control,
		signals:    signals,
	}
	result := s.run(func(addr string) error {
		return protocol.write(desktopproto.NewReady(appVersion(), addr, os.Getpid()))
	})
	if result.err != nil {
		if err := protocol.write(desktopproto.NewError(result.stage, result.err.Error())); err != nil {
			fmt.Fprintln(stderr, "cats-wsl-host: write error record:", err)
		}
		fmt.Fprintf(stderr, "cats-wsl-host: %s: %v\n", result.stage, result.err)
	}
	if err := protocol.write(desktopproto.NewStopped(result.reason)); err != nil {
		fmt.Fprintln(stderr, "cats-wsl-host: write stopped record:", err)
		return 1
	}
	if result.err != nil {
		return 1
	}
	return 0
}

func parseOptions(args []string, stderr io.Writer) (options, error) {
	var opts options
	fs := flag.NewFlagSet("cats-wsl-host", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.IntVar(&opts.port, "port", 0, "Windows-selected loopback port")
	fs.StringVar(&opts.launchID, "launch-id", "", "bounded per-launch ASCII token")
	fs.StringVar(&opts.startDir, "start-dir", "", "absolute Linux directory for new panes")
	fs.DurationVar(&opts.idleTimeout, "idle-timeout", defaultIdleTimeout, "persistent cathost idle timeout (0 disables)")
	fs.BoolVar(&opts.healthJSON, "health-json", false, "print payload health as one JSON object and exit")
	fs.BoolVar(&opts.allowNonWSL, "allow-non-wsl", false, "development only: permit an ordinary Linux host")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() != 0 {
		return options{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if opts.idleTimeout < 0 {
		return options{}, errors.New("--idle-timeout must not be negative")
	}
	if opts.healthJSON {
		return opts, nil
	}
	if opts.port < 1 || opts.port > 65535 {
		return options{}, errors.New("--port must be between 1 and 65535")
	}
	if !validLaunchID(opts.launchID) {
		return options{}, errors.New("--launch-id must be 1-64 ASCII letters, digits, underscores, or hyphens")
	}
	if opts.startDir != "" && !filepath.IsAbs(opts.startDir) {
		return options{}, errors.New("--start-dir must be an absolute Linux path")
	}
	return opts, nil
}

func validLaunchID(id string) bool {
	if len(id) < 1 || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func requireWSL(override bool) error {
	if override {
		return nil
	}
	var diagnostics []string
	for _, path := range osReleaseFiles {
		data, err := os.ReadFile(path)
		if err != nil {
			diagnostics = append(diagnostics, filepath.Base(path)+": "+err.Error())
			continue
		}
		lower := strings.ToLower(string(data))
		if strings.Contains(lower, "microsoft") || strings.Contains(lower, "wsl") {
			return nil
		}
	}
	detail := strings.Join(diagnostics, "; ")
	if detail != "" {
		detail = " (" + detail + ")"
	}
	return fmt.Errorf("this helper must run inside WSL2%s; use --allow-non-wsl only for development tests", detail)
}

func siblingPayloadDir() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate helper executable: %w", err)
	}
	if !filepath.IsAbs(self) {
		return "", fmt.Errorf("helper executable path is not absolute: %q", self)
	}
	return filepath.Dir(self), nil
}

func verifyPayload(dir string) error {
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("payload directory must be absolute: %q", dir)
	}
	for _, name := range []string{"catway", "cathost", "catctl"} {
		path := filepath.Join(dir, name)
		st, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("payload binary %s: %w", name, err)
		}
		if !st.Mode().IsRegular() || st.Mode().Perm()&0o111 == 0 {
			return fmt.Errorf("payload binary %s is not a regular executable", name)
		}
	}
	return nil
}

func appVersion() string {
	info := buildinfo.Get()
	version := info.Hash
	if version == "" {
		version = "dev"
	}
	if info.Dirty {
		version += "-dirty"
	}
	return version
}

func resolveStartDir(requested, home string) (string, error) {
	dir := requested
	if dir == "" {
		dir = home
	}
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("start directory must be absolute: %q", dir)
	}
	st, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("start directory %s: %w", dir, err)
	}
	if !st.IsDir() {
		return "", fmt.Errorf("start directory %s is not a directory", dir)
	}
	return filepath.Clean(dir), nil
}

func reportFailure(protocol protocolOutput, stderr io.Writer, stage string, err error) int {
	if writeErr := protocol.write(desktopproto.NewError(stage, err.Error())); writeErr != nil {
		fmt.Fprintln(stderr, "cats-wsl-host: write error record:", writeErr)
	}
	_ = protocol.write(desktopproto.NewStopped("startup_error"))
	fmt.Fprintf(stderr, "cats-wsl-host: %s: %v\n", stage, err)
	return 1
}

type protocolOutput struct{ w io.Writer }

func (p protocolOutput) write(record desktopproto.HelperRecord) error {
	return desktopproto.EncodeHelper(p.w, record)
}

type controlEvent struct {
	reason string
	err    error
}

func readControl(r io.Reader) <-chan controlEvent {
	events := make(chan controlEvent, 1)
	go func() {
		record, err := desktopproto.NewDecoder(r).ReadLauncher()
		switch {
		case errors.Is(err, io.EOF):
			events <- controlEvent{reason: "stdin_eof"}
		case err != nil:
			events <- controlEvent{reason: "protocol_error", err: err}
		case record.Type == desktopproto.TypeStop:
			events <- controlEvent{reason: "requested"}
		default:
			events <- controlEvent{reason: "protocol_error", err: fmt.Errorf("unexpected request %q", record.Type)}
		}
	}()
	return events
}

func uidNumber(current *user.User) (int, error) {
	uid, err := strconv.Atoi(current.Uid)
	if err != nil || uid < 0 {
		return 0, fmt.Errorf("invalid numeric uid %q", current.Uid)
	}
	return uid, nil
}

func init() {
	log.SetOutput(os.Stderr)
}
