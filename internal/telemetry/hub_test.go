package telemetry

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const hubID = "ABCDEF01-2345-4678-89AB-CDEF01234567"

func hubConfig(enabled bool, version int, id string) string {
	return `{"telemetry":{"enabled":` + map[bool]string{true: "true", false: "false"}[enabled] +
		`,"decidedAt":"2026-10-02T00:00:00Z","promptVersion":` + strconv.Itoa(version) + `,"anonymousId":"` + id + `"}}`
}

func putHub(t *testing.T, o Options, text string) {
	t.Helper()
	put(t, filepath.Join(o.Home, ".caveman", "cloud.json"), text)
}

func putLegacy(t *testing.T, o Options, text string) {
	t.Helper()
	put(t, filepath.Join(o.Home, ".caveman-cloud", "config.json"), text)
}

func TestCavemanChoiceReadsCloudJSONFirst(t *testing.T) {
	t.Parallel()
	// A stale legacy yes never outlives a v4 opt-out.
	o := testOptions(t)
	putLegacy(t, o, hubConfig(true, 5, hubID))
	putHub(t, o, `{"telemetry":{"enabled":false,"decidedAt":"2026-10-03T00:00:00Z","promptVersion":6}}`)
	if d := Observe(o); d.Status != "off" || d.Reason != "the Caveman CLI opt-out" || d.HubCovered {
		t.Fatalf("stale legacy on + new off: %+v", d)
	}
	saved(t, o, State{Enabled: true, DecidedAt: "2026-10-01T00:00:00Z", Disclosure: 1, AnonymousID: strings.ToLower(hubID), Linked: true})
	if d := Decide(o); d.Status != "off" || d.Reason != "the Caveman CLI opt-out" {
		t.Fatalf("linked default, new file off: %+v", d)
	}

	// A new file that cannot be read is no choice: no fallback, no enrollment, the link does not hold.
	putHub(t, o, "{")
	if c := cavemanChoice(o); c.Enabled != nil {
		t.Fatalf("corrupt cloud.json fell back to the legacy file: %+v", c)
	}
	if d := Observe(o); d.Sendable || d.HubCovered {
		t.Fatalf("corrupt cloud.json: %+v", d)
	}

	// CAVEMAN_HOME moves the file; only without one is the legacy file read.
	c := testOptions(t)
	other := t.TempDir()
	c.Getenv = func(k string) string { return map[string]string{"CAVEMAN_HOME": other}[k] }
	putLegacy(t, c, hubConfig(true, 6, hubID))
	if got := cavemanChoice(c); got.AnonymousID != hubID {
		t.Fatalf("legacy fallback: %+v", got)
	}
	put(t, filepath.Join(other, "cloud.json"), hubConfig(false, 6, ""))
	if got := cavemanChoice(c); got.Enabled == nil || *got.Enabled {
		t.Fatalf("CAVEMAN_HOME ignored: %+v", got)
	}

	// An opt-out an older CLI left in the legacy file holds, as it does for that CLI.
	putHub(t, o, hubConfig(true, 6, hubID))
	putLegacy(t, o, `{"telemetry":{"enabled":false,"decidedAt":"2026-10-03T00:00:00Z","promptVersion":5}}`)
	if got := cavemanChoice(o); got.Enabled == nil || *got.Enabled {
		t.Fatalf("legacy opt-out lost: %+v", got)
	}
}

func TestHubCoveredEnrollment(t *testing.T) {
	t.Parallel()
	o := testOptions(t)
	putHub(t, o, hubConfig(true, 5, hubID))
	if d := Observe(o); d.Sendable || d.HubCovered {
		t.Fatalf("promptVersion 5 enrolled: %+v", d)
	}

	putHub(t, o, hubConfig(true, 6, hubID))
	d := Observe(o)
	s := d.State
	if !d.Sendable || !s.Linked || s.Explicit || s.AnonymousID != strings.ToLower(hubID) ||
		s.DecidedAt != o.now().Format(time.RFC3339) || s.Disclosure != DisclosureVersion || d.Reason != HubReason {
		t.Fatalf("hub-covered enrollment: %+v", d)
	}

	// The Caveman CLI replaces its ID: the old one stops at once and the day is marked; the new one
	// enrolls with a fresh decided_at.
	o.Now = func() time.Time { return time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC) }
	newID := "fedcba98-7654-4321-8fed-cba987654321"
	putHub(t, o, hubConfig(true, 6, newID))
	if d := Decide(o); d.Sendable || !d.HubCovered {
		t.Fatalf("hub ID change kept the old ID: %+v", d)
	}
	d = Observe(o)
	if !d.Sendable || d.State.AnonymousID != newID || d.State.DecidedAt != "2026-10-06T09:00:00Z" {
		t.Fatalf("re-enrollment: %+v", d)
	}
	if fi, err := os.Stat(filepath.Join(o.Base, "telemetry-off")); err != nil || fi.ModTime().Before(time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("hub ID change did not mark the day: %v", err)
	}

	// The Caveman CLI turns off: Blocks stops (control 6) and does not enroll.
	putHub(t, o, `{"telemetry":{"enabled":false,"decidedAt":"2026-10-06T10:00:00Z","promptVersion":6}}`)
	if d := Observe(o); d.Sendable || d.Status != "off" {
		t.Fatalf("hub off: %+v", d)
	}
}

func TestExplicitOffBeatsHubOn(t *testing.T) {
	o := testOptions(t)
	putHub(t, o, hubConfig(true, 6, hubID))
	saved(t, o, State{Enabled: false, Explicit: true, DecidedAt: "2026-10-01T00:00:00Z"})
	if d := Observe(o); d.Sendable || d.HubCovered || d.Status != "off" || d.State.AnonymousID != "" {
		t.Fatalf("explicit off overridden: %+v", d)
	}
}

func TestHubEnrollmentNeverUnderASwitch(t *testing.T) {
	for _, k := range []string{"CI", "DO_NOT_TRACK", "CAVEMAN_TELEMETRY", "CAVEMAN_BLOCKS_TELEMETRY"} {
		o := testOptions(t)
		putHub(t, o, hubConfig(true, 6, hubID))
		o.Getenv = func(key string) string { return map[string]string{k: "0x"}[key] }
		if k == "CI" || k == "DO_NOT_TRACK" {
			o.Getenv = func(key string) string { return map[string]string{k: "1"}[key] }
		}
		if d := Observe(o); d.Sendable || d.State.AnonymousID != "" {
			t.Fatalf("%s enrolled: %+v", k, d)
		}
	}
}

// Of processes enrolling at once, only the one whose save wrote the default reports it (first_run).
func TestConcurrentEnrollReportsOnce(t *testing.T) {
	t.Parallel()
	for i := range 10 {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			o := testOptions(t)
			putHub(t, o, hubConfig(true, 6, hubID))
			var wg sync.WaitGroup
			var mu sync.Mutex
			enrolled := 0
			for range 8 {
				wg.Go(func() {
					if d := Observe(o); d.Enrolled {
						mu.Lock()
						enrolled++
						mu.Unlock()
					}
				})
			}
			wg.Wait()
			if d := Observe(o); enrolled != 1 || d.Enrolled || !d.Sendable {
				t.Fatalf("enrolled %d times, then %+v", enrolled, d)
			}
		})
	}
}

// An explicit off saved while an Enroll is in flight (it decided before the off) is never replaced.
func TestExplicitOffSurvivesStaleEnroll(t *testing.T) {
	t.Parallel()
	o := testOptions(t)
	putHub(t, o, hubConfig(true, 6, hubID))
	if _, err := SaveChoice(o, false, true); err != nil {
		t.Fatal(err)
	}
	// What a stale Enroll does after its check passed: try to save the default. Nothing is written.
	if _, err := saveChoice(o, true, false, true); err == nil {
		t.Fatal("stale default save reported a write")
	}
	if d := Decide(o); d.Status != "off" || !d.State.Explicit || d.State.AnonymousID != "" {
		t.Fatalf("explicit off downgraded: %+v", d)
	}

	// Racing processes: whichever order, the off holds.
	for i := range 10 {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			o := testOptions(t)
			putHub(t, o, hubConfig(true, 6, hubID))
			var wg sync.WaitGroup
			for range 8 {
				wg.Go(func() { Enroll(o) })
			}
			wg.Go(func() {
				if _, err := SaveChoice(o, false, true); err != nil {
					t.Error(err)
				}
			})
			wg.Wait()
			if d := Decide(o); d.Status != "off" || !d.State.Explicit || d.State.AnonymousID != "" {
				t.Fatalf("racing enroll beat the off: %+v", d)
			}
			if _, err := os.Lstat(filepath.Join(o.Base, "telemetry.lock")); !os.IsNotExist(err) {
				t.Fatalf("lock left behind: %v", err)
			}
		})
	}
}

// Enroll saves only under the Caveman CLI's ID: once that link is gone it saves nothing, never a
// random ID of its own.
func TestEnrollNeverMints(t *testing.T) {
	for name, hub := range map[string]string{
		"hub off":       `{"telemetry":{"enabled":false,"decidedAt":"2026-10-03T00:00:00Z","promptVersion":6}}`,
		"hub v5":        hubConfig(true, 5, hubID),
		"no ID":         hubConfig(true, 6, ""),
		"unreadable":    "{",
		"no hub at all": "",
	} {
		o := testOptions(t)
		if hub != "" {
			putHub(t, o, hub)
		}
		if _, err := saveChoice(o, true, false, true); err != errNoLink {
			t.Fatalf("%s: %v", name, err)
		}
		if Enroll(o) {
			t.Fatalf("%s: enrolled", name)
		}
		if _, err := os.Lstat(filepath.Join(o.Base, "telemetry.json")); !os.IsNotExist(err) {
			t.Fatalf("%s: saved state: %v", name, err)
		}
	}
}

// A dangling cloud.json symlink is a cloud.json that cannot be read: no legacy fallback.
func TestDanglingCloudJSONFailsClosed(t *testing.T) {
	o := testOptions(t)
	putLegacy(t, o, hubConfig(true, 6, hubID))
	if err := os.MkdirAll(filepath.Join(o.Home, ".caveman"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(o.Home, "gone.json"), filepath.Join(o.Home, ".caveman", "cloud.json")); err != nil {
		t.Skip("no symlinks:", err)
	}
	if c := cavemanChoice(o); c.Enabled != nil {
		t.Fatalf("fell back to the legacy file: %+v", c)
	}
	if d := Observe(o); d.Sendable || d.HubCovered {
		t.Fatalf("dangling cloud.json enrolled: %+v", d)
	}
}

func TestStaleStateLockIsTakenOver(t *testing.T) {
	o := testOptions(t)
	lock := filepath.Join(o.Base, "telemetry.lock")
	put(t, lock, "")
	old := time.Now().Add(-time.Minute)
	if err := os.Chtimes(lock, old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveChoice(o, false, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(lock); !os.IsNotExist(err) {
		t.Fatalf("lock left behind: %v", err)
	}
}
