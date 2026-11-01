//go:build unix

// Package main, not main_test: run is unexported and package main can't be
// imported. The tests re-run the test binary as the server (see TestMain), so
// they can send it real signals.
package main

import (
	"bufio"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runServerEnv makes the test binary run the server instead of the tests.
const runServerEnv = "HL_TEST_RUN_SERVER"

func TestMain(m *testing.M) {
	if os.Getenv(runServerEnv) == "1" {
		os.Exit(run())
	}
	os.Exit(m.Run())
}

// server is the server running in a child process.
type server struct {
	cmd    *exec.Cmd
	dbPath string
	exited chan struct{}
	err    error

	mu    sync.Mutex
	lines []string
	added chan struct{}
}

// startServer starts the server in a child process, with a broker that never
// answers and no HL_* variables from the environment. With slowStartup, it
// compiles a script whose top level loops until HL_SCRIPTS_TIMEOUT, so
// startup takes about 2s after the database is opened.
func startServer(t *testing.T, slowStartup bool) *server {
	t.Helper()
	dir := t.TempDir()
	scriptsDir := filepath.Join(dir, "scripts")
	require.NoError(t, os.Mkdir(scriptsDir, 0o700))
	if slowStartup {
		require.NoError(t, os.WriteFile(filepath.Join(scriptsDir, "slow.js"), []byte(`while (true) {}`), 0o600))
	}

	srv := &server{
		dbPath: filepath.Join(dir, "homeland.db"),
		exited: make(chan struct{}),
		added:  make(chan struct{}, 1),
	}
	env := []string{
		runServerEnv + "=1",
		"LOG_LEVEL=info",
		"HL_HTTP_PORT=" + strconv.Itoa(freePort(t)),
		"HL_Z2M_MQTT_BROKER=tcp://127.0.0.1:1",
		"HL_DB_PATH=" + srv.dbPath,
		"HL_SCRIPTS_DIR=" + scriptsDir,
		"HL_SCRIPTS_TIMEOUT=2s",
	}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "HL_") && !strings.HasPrefix(kv, "LOG_LEVEL=") {
			env = append(env, kv)
		}
	}

	pr, pw := io.Pipe()
	srv.cmd = exec.Command(os.Args[0], "-test.run=^$") //nolint:gosec // re-runs this test binary
	srv.cmd.Env = env
	srv.cmd.Stdout = pw
	srv.cmd.Stderr = pw
	require.NoError(t, srv.cmd.Start())

	go func() {
		scanner := bufio.NewScanner(pr)
		for scanner.Scan() {
			srv.mu.Lock()
			srv.lines = append(srv.lines, scanner.Text())
			srv.mu.Unlock()
			select {
			case srv.added <- struct{}{}:
			default:
			}
		}
	}()
	go func() {
		srv.err = srv.cmd.Wait()
		_ = pw.Close()
		close(srv.exited)
	}()
	t.Cleanup(func() {
		_ = srv.cmd.Process.Kill()
		<-srv.exited
	})
	return srv
}

// freePort returns a TCP port that was free a moment ago.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())
	return port
}

// output returns everything the server has logged so far.
func (srv *server) output() string {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	return strings.Join(srv.lines, "\n")
}

// waitForLog waits until the server logs a line containing text.
func (srv *server) waitForLog(t *testing.T, text string) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for !strings.Contains(srv.output(), text) {
		select {
		case <-srv.added:
		case <-srv.exited:
			if strings.Contains(srv.output(), text) {
				return
			}
			t.Fatalf("server exited before logging %q:\n%s", text, srv.output())
		case <-deadline:
			t.Fatalf("server did not log %q:\n%s", text, srv.output())
		}
	}
}

// waitForDB waits until the server has opened its database, which happens
// before the scripts are compiled.
func (srv *server) waitForDB(t *testing.T) {
	t.Helper()
	require.Eventually(t, func() bool {
		_, err := os.Stat(srv.dbPath)
		return err == nil
	}, 10*time.Second, 5*time.Millisecond, "the server did not open its database")
}

// signal sends sig to the server.
func (srv *server) signal(t *testing.T, sig syscall.Signal) {
	t.Helper()
	require.NoError(t, srv.cmd.Process.Signal(sig))
}

// wait waits for the server to exit and returns how it exited.
func (srv *server) wait(t *testing.T) error {
	t.Helper()
	select {
	case <-srv.exited:
		return srv.err
	case <-time.After(15 * time.Second):
		t.Fatalf("server did not exit:\n%s", srv.output())
		return nil
	}
}

func TestSignalShutsDownGracefully(t *testing.T) {
	srv := startServer(t, false)
	srv.waitForLog(t, "web: starting server")

	srv.signal(t, syscall.SIGTERM)
	require.NoError(t, srv.wait(t), "a graceful shutdown exits 0:\n%s", srv.output())
	out := srv.output()
	assert.Contains(t, out, "web: shutting down")
	assert.Contains(t, out, "svc/event: shutting down")
}

func TestSignalDuringStartupShutsDownWithoutServing(t *testing.T) {
	srv := startServer(t, true)
	srv.waitForDB(t)

	srv.signal(t, syscall.SIGTERM)
	require.NoError(t, srv.wait(t), "startup finishes, then everything shuts down:\n%s", srv.output())
	out := srv.output()
	assert.Contains(t, out, "main: received shutdown signal")
	assert.Contains(t, out, "svc/event: shutting down")
	assert.NotContains(t, out, "web: starting server", "nothing is served after the signal")
}

func TestSecondSignalExitsImmediately(t *testing.T) {
	srv := startServer(t, true)
	srv.waitForDB(t)

	srv.signal(t, syscall.SIGTERM)
	srv.waitForLog(t, "main: received shutdown signal")
	srv.signal(t, syscall.SIGINT)

	err := srv.wait(t)
	exitErr, ok := errors.AsType[*exec.ExitError](err)
	require.True(t, ok, "the server must be killed by the signal, got %v:\n%s", err, srv.output())
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	require.True(t, ok)
	assert.True(t, status.Signaled(), "the server must be killed by the signal")
	assert.Equal(t, syscall.SIGINT, status.Signal())
	assert.NotContains(t, srv.output(), "svc/event: shutting down", "startup is cut short")
}
