package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/caveman-ai/blocks/internal/scan"
	"github.com/caveman-ai/blocks/internal/telemetry"
	"github.com/spf13/cobra"
)

// All tests, including existing hook tests and testscript children, must never detach the test binary.
func init() { spawnTelemetry = func([]telemetry.Event) {} }

func cliTelemetry(t *testing.T) (telemetry.Options, *bytes.Buffer, *[][]telemetry.Event) {
	t.Helper()
	o := telemetry.Options{Base: filepath.Join(t.TempDir(), "state"), Home: t.TempDir(), Executable: "/bin/blocks", Version: "1.2.3",
		FirstParty: []string{"json-peek"}, Getenv: func(string) string { return "" },
		Now: func() time.Time { return time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC) }}
	var stderr bytes.Buffer
	var calls [][]telemetry.Event
	oldSpawn, oldOptions := spawnTelemetry, telemetryOptions
	spawnTelemetry = func(events []telemetry.Event) { calls = append(calls, events) }
	telemetryOptions = func() telemetry.Options { return o }
	t.Cleanup(func() { hookBackground.Wait(); spawnTelemetry, telemetryOptions = oldSpawn, oldOptions })
	return o, &stderr, &calls
}

func TestTelemetryLifecycle(t *testing.T) {
	o, stderr, calls := cliTelemetry(t)
	for i := range 2 {
		root := newRoot()
		session := beginTelemetry(o, true, stderr, []string{"stats"})
		session.end(root.Commands()[0], 0) // resolved command has no private positional args
		want := 1
		if i == 0 {
			want = 2
		}
		if len(*calls) != i+1 || len((*calls)[i]) != want || (*calls)[i][want-1].Event != "command_run" {
			t.Fatalf("first/second events: %+v", *calls)
		}
		if i == 0 && (*calls)[0][0].Event != "first_run" {
			t.Fatal("missing first_run")
		}
	}
	if stderr.String() != telemetry.Disclosure+"\n" {
		t.Fatalf("disclosure not exactly once: %q", stderr.String())
	}
	d := telemetry.Decide(o)
	if !d.Sendable || d.State.Explicit || d.State.Disclosure != 1 {
		t.Fatalf("default: %+v", d)
	}
	stderr.Reset()
	root := newRoot()
	root.SetArgs([]string{"telemetry", "off"})
	root.SetErr(stderr)
	session := beginTelemetry(o, true, stderr, []string{"telemetry", "off"})
	c, err := root.ExecuteC()
	if err != nil {
		t.Fatal(err)
	}
	session.end(c, 0)
	if len(*calls) != 2 || strings.TrimSpace(stderr.String()) != telemetry.OffNotice(d.State.AnonymousID) {
		t.Fatalf("off sent or lost notice: %+v %q", *calls, stderr.String())
	}
	stderr.Reset()
	if err := root.Execute(); err != nil || stderr.Len() != 0 {
		t.Fatalf("repeat off notice: %q %v", stderr.String(), err)
	}
}

func TestTelemetryNonInteractiveAndExcluded(t *testing.T) {
	for _, argv := range [][]string{nil, {"hook"}, {"help"}, {"version"}, {"--version"}, {"-v"}, {"--help"}, {"add", "--help"}, {"run", "-h"}, {"telemetry", "flush"}} {
		o, stderr, calls := cliTelemetry(t)
		if beginTelemetry(o, true, stderr, argv) != nil || stderr.Len() != 0 || len(*calls) != 0 || telemetry.Decide(o).Saved {
			t.Fatalf("excluded invocation did telemetry: %v", argv)
		}
	}
	o, stderr, calls := cliTelemetry(t)
	if beginTelemetry(o, false, stderr, []string{"scan"}) != nil || telemetry.Decide(o).Saved || len(*calls) != 0 {
		t.Fatal("non-interactive run saved or sent")
	}
	if _, err := telemetry.SaveChoice(o, true, true); err != nil {
		t.Fatal(err)
	}
	if beginTelemetry(o, false, stderr, []string{"scan"}) != nil || len(*calls) != 0 {
		t.Fatal("non-interactive saved yes sent command telemetry")
	}
}

func TestTelemetryEnvOffPersists(t *testing.T) {
	for _, key := range []string{"DO_NOT_TRACK", "CAVEMAN_BLOCKS_TELEMETRY", "CAVEMAN_TELEMETRY", "CI"} {
		t.Run(key, func(t *testing.T) {
			o, stderr, calls := cliTelemetry(t)
			s, err := telemetry.SaveChoice(o, true, false)
			if err != nil {
				t.Fatal(err)
			}
			o.Getenv = func(k string) string {
				if k == key {
					return "off"
				}
				return ""
			}
			session := beginTelemetry(o, true, stderr, []string{"stats"})
			session.end(nil, 1)
			o.Getenv = func(string) string { return "" }
			d := telemetry.Decide(o)
			if key == "CI" {
				if !d.Sendable || stderr.Len() != 0 {
					t.Fatal("CI persisted an off")
				}
			} else if d.Status != "off" || !d.State.Explicit || d.State.AnonymousID != "" || strings.TrimSpace(stderr.String()) != telemetry.OffNotice(s.AnonymousID) {
				t.Fatalf("env off: %+v %q", d, stderr.String())
			}
			if len(*calls) != 0 {
				t.Fatal("sent after env off")
			}
		})
	}
}

func TestCommandTelemetryFields(t *testing.T) {
	for _, tc := range []struct {
		argv                  []string
		command, sub, block   string
		exit                  int
		exitClass, errorClass string
	}{
		{[]string{"add", "json-peek"}, "add", "", "json-peek", 0, "ok", ""},
		{[]string{"run", "json-peek", "--path", "/private"}, "run", "", "json-peek", 1, "error", "failed"},
		{[]string{"run", "acme-secret-report"}, "run", "", "", 2, "error", "usage"},
		{[]string{"hooks", "status"}, "hooks", "status", "", 0, "ok", ""},
		{[]string{"unknown-secret"}, "unknown", "", "", 2, "error", "usage"},
	} {
		t.Run(strings.Join(tc.argv, " "), func(t *testing.T) {
			o, stderr, calls := cliTelemetry(t)
			root := newRoot()
			c, pos, err := root.Find(tc.argv)
			if err != nil {
				c = root
			} else if !c.DisableFlagParsing {
				if err := c.ParseFlags(pos); err != nil {
					t.Fatal(err)
				}
			}
			s := beginTelemetry(o, true, stderr, tc.argv)
			s.options.Now = func() time.Time { return o.Now().Add(123 * time.Millisecond) }
			s.end(c, tc.exit)
			e := (*calls)[0][1]
			if e.Command != tc.command || e.Subcommand != tc.sub || e.Block != tc.block || e.ExitClass != tc.exitClass || e.ErrorClass != tc.errorClass || e.DurationMS != 123 {
				t.Fatalf("fields: %+v", e)
			}
		})
	}
}

func TestScanTelemetryTotals(t *testing.T) {
	o, stderr, calls := cliTelemetry(t)
	root := newRoot()
	c, _, _ := root.Find([]string{"scan"})
	s := beginTelemetry(o, true, stderr, []string{"scan"})
	attachTelemetry(c, s)
	recordScanTelemetry(c, scan.Report{Commands: 20, Scripts: 13, Edits: 2, Groups: []scan.Group{
		{FP: "secret-fingerprint", Count: 5, CoveredBy: []string{"private-block"}}, {Count: 1}, {Count: 4},
	}}, 36*time.Hour)
	s.end(c, 0)
	e := (*calls)[0][1]
	if e.ScanWindowDays != 1 || e.ScanCommands != 20 || e.ScanScripts != 13 || e.ScanEdits != 2 || e.ScanShapes != 3 ||
		e.ScanRepeatedShapes != 2 || e.ScanCoveredShapes != 1 || e.ScanCoveredScripts != 5 || e.ScanTopCount != 5 {
		t.Fatalf("scan counts: %+v", e)
	}
	s.end(c, 1)
	if (*calls)[1][1].ScanCounts != (telemetry.ScanCounts{}) {
		t.Fatal("failed scan included counts")
	}
}

func TestTelemetryStatusNeverSaves(t *testing.T) {
	o, stderr, _ := cliTelemetry(t)
	root := newRoot()
	var stdout bytes.Buffer
	root.SetArgs([]string{"telemetry", "status"})
	root.SetOut(&stdout)
	root.SetErr(stderr)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "not started") || !strings.Contains(stdout.String(), "install ID: none") || telemetry.Decide(o).Saved {
		t.Fatalf("status: %q", stdout.String())
	}
}

func TestNoNetworkDependencies(t *testing.T) {
	t.Parallel()
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool is not on PATH")
	}
	cmd := exec.Command(goTool, "list", "-deps", ".")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps .: %v: %s", err, stderr.String())
	}
	for _, dep := range strings.Fields(string(out)) {
		if dep == "net/http" || dep == "crypto/tls" {
			t.Errorf("hook budget forbids linking %s", dep)
		}
	}
}

func TestDisclosureSaveFailure(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root writes into a read-only directory")
	}
	o, stderr, calls := cliTelemetry(t)
	if err := os.MkdirAll(o.Base, 0o500); err != nil { // the choice cannot be saved here
		t.Fatal(err)
	}
	s := beginTelemetry(o, true, stderr, []string{"stats"})
	s.end(nil, 1)
	if stderr.Len() != 0 || len(*calls) != 0 || telemetry.Decide(o).Saved {
		t.Fatalf("failed save disclosed or sent: %q %+v", stderr.String(), *calls)
	}
}

// A damaged telemetry.json may have been an opt-out: no disclosure, no default written over it.
func TestDamagedChoiceIsNotOverwritten(t *testing.T) {
	o, stderr, calls := cliTelemetry(t)
	file := filepath.Join(o.Base, "telemetry.json")
	const damaged = `{"enabled": false, "explicit": true,}`
	if err := os.MkdirAll(o.Base, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(damaged), 0o600); err != nil {
		t.Fatal(err)
	}
	beginTelemetry(o, true, stderr, []string{"lint"}).end(nil, 0)
	if b, _ := os.ReadFile(file); string(b) != damaged || stderr.Len() != 0 || len(*calls) != 0 {
		t.Fatalf("damaged choice: file %q, stderr %q, sent %+v", b, stderr.String(), *calls)
	}
}

// A yes comes from a person. Without one `telemetry on` refuses; with one it shows the disclosure
// and the new install ID's first_run goes out with the command.
func TestTelemetryOnNeedsAPerson(t *testing.T) {
	o, stderr, calls := cliTelemetry(t)
	old := telemetryInteractive
	t.Cleanup(func() { telemetryInteractive = old })
	on := func() (*cobra.Command, error) {
		root := newRoot()
		root.SetArgs([]string{"telemetry", "on"})
		root.SetErr(stderr)
		return root.ExecuteC()
	}
	telemetryInteractive = func() bool { return false }
	if _, err := on(); err == nil || !isUsage(err) || telemetry.Decide(o).Saved || stderr.Len() != 0 {
		t.Fatalf("an agent turned telemetry on: %v %q", err, stderr.String())
	}
	telemetryInteractive = func() bool { return true }
	session := beginTelemetry(o, true, stderr, []string{"telemetry", "on"})
	c, err := on()
	if err != nil {
		t.Fatal(err)
	}
	session.end(c, 0)
	d := telemetry.Decide(o)
	if !d.Sendable || !d.State.Explicit || stderr.String() != telemetry.Disclosure+"\n" {
		t.Fatalf("on: %+v %q", d, stderr.String())
	}
	if len(*calls) != 1 || len((*calls)[0]) != 2 || (*calls)[0][0].Event != "first_run" ||
		(*calls)[0][1].Command != "telemetry" || (*calls)[0][1].Subcommand != "on" {
		t.Fatalf("events: %+v", *calls)
	}
}

// run() end to end: the scan's totals reach the event through the command context, and the exit
// code run() returns is the one classified.
func TestRunWiring(t *testing.T) {
	o, _, calls := cliTelemetry(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	for _, k := range []string{"DO_NOT_TRACK", "CAVEMAN_BLOCKS_TELEMETRY", "CAVEMAN_TELEMETRY", "CI"} {
		t.Setenv(k, "")
	}
	oldInteractive, oldArgs := telemetryInteractive, os.Args
	t.Cleanup(func() { telemetryInteractive, os.Args = oldInteractive, oldArgs })
	telemetryInteractive = func() bool { return true }

	os.Args = []string{"caveman-blocks", "scan", "--since", "2d", "--harness", "claude"}
	if code := run(); code != 0 {
		t.Fatalf("scan exit %d", code)
	}
	if len(*calls) != 1 || len((*calls)[0]) != 2 || (*calls)[0][0].Event != "first_run" {
		t.Fatalf("scan events: %+v", *calls)
	}
	if e := (*calls)[0][1]; e.Command != "scan" || e.ExitClass != "ok" || e.ScanWindowDays != 2 || e.AnonymousID != telemetry.Decide(o).State.AnonymousID {
		t.Fatalf("scan event: %+v", e)
	}
	os.Args = []string{"caveman-blocks", "run"}
	if code := run(); code != 2 {
		t.Fatalf("run without a name: exit %d", code)
	}
	if len(*calls) != 2 || len((*calls)[1]) != 1 {
		t.Fatalf("second command events: %+v", *calls)
	}
	if e := (*calls)[1][0]; e.Command != "run" || e.ExitClass != "error" || e.ErrorClass != "usage" {
		t.Fatalf("usage error event: %+v", e)
	}
}

// A command that ran while telemetry was off marks the day, so what it counted is never sent.
func TestRunMarksOffDay(t *testing.T) {
	o, _, calls := cliTelemetry(t)
	if _, err := telemetry.SaveChoice(o, true, true); err != nil {
		t.Fatal(err)
	}
	o.Getenv = func(k string) string { return map[string]string{"DO_NOT_TRACK": "1"}[k] }
	telemetryOptions = func() telemetry.Options { return o }
	oldInteractive, oldArgs := telemetryInteractive, os.Args
	t.Cleanup(func() { telemetryInteractive, os.Args = oldInteractive, oldArgs })
	telemetryInteractive = func() bool { return false }
	os.Args = []string{"caveman-blocks", "version"}
	if code := run(); code != 0 || !marked(o) || len(*calls) != 0 {
		t.Fatalf("exit %d, marked %v, sent %+v", code, marked(o), *calls)
	}
}

// The Caveman CLI's promptVersion 6 notice covers Blocks: an interactive command prints no second
// disclosure, run's Observe enrolls under the shared ID, and the command's events follow.
func TestHubCoveredCommandPrintsNothing(t *testing.T) {
	o, stderr, calls := cliTelemetry(t)
	if err := os.MkdirAll(filepath.Join(o.Home, ".caveman"), 0o700); err != nil {
		t.Fatal(err)
	}
	hub := `{"telemetry":{"enabled":true,"decidedAt":"2026-10-01T00:00:00Z","promptVersion":6,"anonymousId":"ABCDEF01-2345-4678-89AB-CDEF01234567"}}`
	if err := os.WriteFile(filepath.Join(o.Home, ".caveman", "cloud.json"), []byte(hub), 0o600); err != nil {
		t.Fatal(err)
	}
	session := beginTelemetry(o, true, stderr, []string{"stats"})
	if d := telemetry.Observe(o); !d.Sendable || !d.State.Linked || d.State.AnonymousID != "abcdef01-2345-4678-89ab-cdef01234567" {
		t.Fatalf("not enrolled: %+v", d)
	}
	session.end(newRoot().Commands()[0], 0)
	if stderr.Len() != 0 {
		t.Fatalf("printed a second disclosure: %q", stderr.String())
	}
	if len(*calls) != 1 || len((*calls)[0]) != 2 || (*calls)[0][0].Event != "first_run" {
		t.Fatalf("events: %+v", *calls)
	}
}

// A switch seen by a command is saved before a hook without it (a GUI app) can enroll under the hub's yes.
func TestEnvOffPersistsBeforeHubEnrollment(t *testing.T) {
	for _, key := range []string{"DO_NOT_TRACK", "CAVEMAN_BLOCKS_TELEMETRY", "CAVEMAN_TELEMETRY", "CI"} {
		t.Run(key, func(t *testing.T) {
			o, stderr, calls := cliTelemetry(t)
			hub := `{"telemetry":{"enabled":true,"decidedAt":"2026-10-01T00:00:00Z","promptVersion":6,"anonymousId":"ABCDEF01-2345-4678-89AB-CDEF01234567"}}`
			if err := os.MkdirAll(filepath.Join(o.Home, ".caveman"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(o.Home, ".caveman", "cloud.json"), []byte(hub), 0o600); err != nil {
				t.Fatal(err)
			}
			value := "off"
			if key == "DO_NOT_TRACK" || key == "CI" {
				value = "1"
			}
			withSwitch := o
			withSwitch.Getenv = func(k string) string { return map[string]string{key: value}[k] }
			beginTelemetry(withSwitch, true, stderr, []string{"stats"}).end(nil, 0)
			d := telemetry.Observe(o) // the hook, without the switch
			if key == "CI" {
				if !d.Sendable || !d.State.Linked {
					t.Fatalf("CI was saved as a choice: %+v", d)
				}
				return
			}
			if d.Sendable || d.Status != "off" || !d.State.Explicit || d.State.AnonymousID != "" || stderr.Len() != 0 || len(*calls) != 0 {
				t.Fatalf("hook enrolled after %s: %+v %q", key, d, stderr.String())
			}
		})
	}
}
