package helmcli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fakeHelm writes a helm that is a shell script, so a test can see exactly what
// the app asked it to do.
//
// The argv is what these tests are really about. Every one of these commands
// changes somebody's cluster, and the flags that decide *which* cluster are
// added here rather than by helm's own defaults -- so an argument silently
// dropped is a release upgraded somewhere it was not meant to be.
func fakeHelm(t *testing.T, body string) *Helm {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake helm is a shell script")
	}

	path := filepath.Join(t.TempDir(), "helm")
	script := "#!/bin/sh\n" + body
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	helm, err := New(Tool{Found: true, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	return helm
}

// echoArgs prints one argument per line, so a test can look for a flag and the
// value that followed it.
const echoArgs = `printf '%s\n' "$@"`

const prod = "/home/u/.kube/prod"

func target() Target {
	return Target{Kubeconfig: prod, Context: "admin@prod", Namespace: "rook-ceph"}
}

// argAfter returns the argument following flag, so a test can assert on a value
// without depending on where in the line it landed.
func argAfter(t *testing.T, out, flag string) string {
	t.Helper()
	args := strings.Split(strings.TrimSpace(out), "\n")
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	t.Fatalf("%s was not passed to helm:\n%s", flag, out)
	return ""
}

func hasArg(out, flag string) bool {
	for _, arg := range strings.Split(strings.TrimSpace(out), "\n") {
		if arg == flag {
			return true
		}
	}
	return false
}

// The mistake this app must not make: helm reading $KUBECONFIG and
// current-context, and upgrading whichever cluster the user last touched in a
// shell rather than the one whose tab they are looking at.
func TestEveryCommandNamesTheClusterItRunsAgainst(t *testing.T) {
	helm := fakeHelm(t, echoArgs)

	upgrade, err := helm.Upgrade(context.Background(), target(),
		UpgradeRequest{Release: "rook-ceph", Chart: "rook-release/rook-ceph"}, Options{})
	if err != nil {
		t.Fatalf("Upgrade: %v", err)
	}
	rollback, err := helm.Rollback(context.Background(), target(), "rook-ceph", 2, Options{})
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	uninstall, err := helm.Uninstall(context.Background(), target(), "rook-ceph", false, Options{})
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	for name, out := range map[string]string{"upgrade": upgrade, "rollback": rollback, "uninstall": uninstall} {
		if got := argAfter(t, out, "--kubeconfig"); got != prod {
			t.Errorf("%s: kubeconfig = %q, want %q", name, got, prod)
		}
		if got := argAfter(t, out, "--kube-context"); got != "admin@prod" {
			t.Errorf("%s: context = %q, want admin@prod", name, got)
		}
		if got := argAfter(t, out, "--namespace"); got != "rook-ceph" {
			t.Errorf("%s: namespace = %q, want rook-ceph", name, got)
		}
	}
}

func TestUpgradeSendsTheChartTheVersionAndTheValues(t *testing.T) {
	helm := fakeHelm(t, `printf '%s\n' "$@"; echo "---"; cat "$(echo "$@" | tr ' ' '\n' | grep -A1 -- --values | tail -1)"`)

	out, err := helm.Upgrade(context.Background(), target(), UpgradeRequest{
		Release: "rook-ceph",
		Chart:   "rook-release/rook-ceph",
		Version: "v1.19.9",
		Values:  "csi:\n  provisionerReplicas: 3\n",
	}, Options{})
	if err != nil {
		t.Fatalf("Upgrade: %v", err)
	}

	args := strings.Split(out, "---")[0]
	if got := argAfter(t, args, "--version"); got != "v1.19.9" {
		t.Errorf("version = %q, want v1.19.9", got)
	}
	// The chart is a positional argument after the release, which is helm's
	// own shape: `helm upgrade <release> <chart>`.
	lines := strings.Split(strings.TrimSpace(args), "\n")
	if lines[0] != "upgrade" || lines[1] != "rook-ceph" || lines[2] != "rook-release/rook-ceph" {
		t.Errorf("the command did not begin `upgrade <release> <chart>`: %v", lines[:3])
	}
	// The values reach helm as the document itself, not squeezed through --set,
	// where a dot or a comma in a value would have to be escaped.
	if !strings.Contains(out, "provisionerReplicas: 3") {
		t.Errorf("the values file did not carry the document:\n%s", out)
	}
}

// A release with no overrides must send no values file at all. Helm keeps the
// previous release's config only when no values are supplied, so sending an
// empty file would quietly reset the release to the chart's defaults.
func TestAnUpgradeWithNoValuesSendsNoValuesFile(t *testing.T) {
	helm := fakeHelm(t, echoArgs)

	out, err := helm.Upgrade(context.Background(), target(),
		UpgradeRequest{Release: "app", Chart: "repo/app", Values: "   \n"}, Options{})
	if err != nil {
		t.Fatalf("Upgrade: %v", err)
	}

	if hasArg(out, "--values") {
		t.Errorf("an empty values document was still sent as a file:\n%s", out)
	}
}

// --reuse-values would merge the old values back over the new ones, so a value
// deleted in the editor would come back and the save would look like it had
// silently failed.
func TestAnUpgradeNeverReusesTheOldValues(t *testing.T) {
	helm := fakeHelm(t, echoArgs)

	out, err := helm.Upgrade(context.Background(), target(),
		UpgradeRequest{Release: "app", Chart: "repo/app", Values: "replicas: 2\n"}, Options{})
	if err != nil {
		t.Fatalf("Upgrade: %v", err)
	}

	if hasArg(out, "--reuse-values") {
		t.Errorf("the upgrade reused the old values:\n%s", out)
	}
}

func TestWaitingAndAtomicAreAskedForOnlyWhenChosen(t *testing.T) {
	helm := fakeHelm(t, echoArgs)

	plain, err := helm.Upgrade(context.Background(), target(),
		UpgradeRequest{Release: "app", Chart: "repo/app"}, Options{})
	if err != nil {
		t.Fatalf("Upgrade: %v", err)
	}
	if hasArg(plain, "--wait") || hasArg(plain, "--atomic") {
		t.Errorf("an unasked-for wait was added:\n%s", plain)
	}

	careful, err := helm.Upgrade(context.Background(), target(),
		UpgradeRequest{Release: "app", Chart: "repo/app"}, Options{Atomic: true})
	if err != nil {
		t.Fatalf("Upgrade: %v", err)
	}
	// --atomic waits whether or not --wait was asked for, so both are sent:
	// what helm is told matches what it will do.
	if !hasArg(careful, "--atomic") || !hasArg(careful, "--wait") {
		t.Errorf("atomic did not imply waiting:\n%s", careful)
	}
}

func TestTheTimeoutIsPassedToHelmRatherThanOnlyEnforcedHere(t *testing.T) {
	helm := fakeHelm(t, echoArgs)

	out, err := helm.Rollback(context.Background(), target(), "app", 3,
		Options{Timeout: 90 * time.Second})
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	// helm's own timeout, so a command that runs out says what it was waiting
	// for rather than being killed silently from outside.
	if got := argAfter(t, out, "--timeout"); got != "1m30s" {
		t.Errorf("timeout = %q, want 1m30s", got)
	}
}

func TestUninstallKeepsHistoryOnlyWhenAsked(t *testing.T) {
	helm := fakeHelm(t, echoArgs)

	plain, err := helm.Uninstall(context.Background(), target(), "app", false, Options{})
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if hasArg(plain, "--keep-history") {
		t.Errorf("history was kept without being asked for:\n%s", plain)
	}

	kept, err := helm.Uninstall(context.Background(), target(), "app", true, Options{})
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if !hasArg(kept, "--keep-history") {
		t.Errorf("history was not kept when asked:\n%s", kept)
	}
}

// helm's own words are the answer to what went wrong. "UPGRADE FAILED: timed
// out waiting for the condition" is more use than any wording this app could
// invent for it.
func TestAFailureReportsWhatHelmSaid(t *testing.T) {
	helm := fakeHelm(t, `echo "Error: UPGRADE FAILED: timed out waiting for the condition" >&2; exit 1`)

	_, err := helm.Rollback(context.Background(), target(), "app", 1, Options{})

	if err == nil {
		t.Fatal("a failing helm reported success")
	}
	if !strings.Contains(err.Error(), "timed out waiting for the condition") {
		t.Errorf("error = %q, want helm's own message", err)
	}
}

func TestASilentFailureStillSaysSomething(t *testing.T) {
	helm := fakeHelm(t, `exit 3`)

	_, err := helm.Rollback(context.Background(), target(), "app", 1, Options{})

	if err == nil || !strings.Contains(err.Error(), "helm failed") {
		t.Errorf("error = %v, want a report of the exit status", err)
	}
}

// A chart the repositories do not cover is an answer, not a fault: Helm's
// release record does not say where a chart came from, so an OCI or local chart
// simply will not be found.
func TestAChartNothingOffersIsAnEmptyListRatherThanAnError(t *testing.T) {
	helm := fakeHelm(t, `echo "Error: no results found" >&2; exit 1`)

	found, err := helm.Versions(context.Background(), "some/chart")

	if err != nil {
		t.Fatalf("Versions: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("versions = %+v, want none", found)
	}
}

func TestVersionsAreReadFromWhatHelmFound(t *testing.T) {
	helm := fakeHelm(t,
		`echo '[{"name":"rook-release/rook-ceph","version":"v1.19.9","app_version":"v1.19.9","description":"Rook"}]'`)

	found, err := helm.Versions(context.Background(), "rook-release/rook-ceph")

	if err != nil {
		t.Fatalf("Versions: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("versions = %d, want 1", len(found))
	}
	if found[0].Version != "v1.19.9" || found[0].AppVersion != "v1.19.9" {
		t.Errorf("version = %+v", found[0])
	}
}

// The values document is the part of a release most likely to hold a password.
func TestTheValuesFileIsNotReadableByAnyoneElse(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits mean something else here")
	}

	path, cleanup, err := valuesFile("password: hunter2\n")
	if err != nil {
		t.Fatalf("valuesFile: %v", err)
	}
	defer cleanup()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("values file mode = %o, want 600", perm)
	}

	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("the values file outlived the command that needed it")
	}
}

func TestAReleaseNameHelmWouldRefuseIsRefusedHere(t *testing.T) {
	helm := fakeHelm(t, echoArgs)

	for _, name := range []string{"", "Rook-Ceph", "--kubeconfig", "-rf", strings.Repeat("a", 54)} {
		if _, err := helm.Rollback(context.Background(), target(), name, 1, Options{}); err == nil {
			t.Errorf("%q was accepted as a release name", name)
		}
	}
}

// Nothing here goes through a shell, so a leading dash -- an argument helm
// would read as a flag -- is the whole of what a chart reference has to be
// checked for.
func TestAChartReferenceMayNotArriveAsAFlag(t *testing.T) {
	helm := fakeHelm(t, echoArgs)

	if _, err := helm.Upgrade(context.Background(), target(),
		UpgradeRequest{Release: "app", Chart: "--set=x=y"}, Options{}); err == nil {
		t.Error("a chart reference beginning with a dash was accepted")
	}

	// The forms a chart really takes, none of which a narrower rule would allow.
	for _, chart := range []string{
		"ingress-nginx/ingress-nginx",
		"oci://registry-1.docker.io/bitnamicharts/nginx",
		"https://example.test/charts/app-1.0.0.tgz",
		"./charts/app",
	} {
		if _, err := helm.Upgrade(context.Background(), target(),
			UpgradeRequest{Release: "app", Chart: chart}, Options{}); err != nil {
			t.Errorf("chart %q was refused: %v", chart, err)
		}
	}
}

func TestARevisionMustBeARevision(t *testing.T) {
	helm := fakeHelm(t, echoArgs)

	for _, revision := range []int{0, -1} {
		if _, err := helm.Rollback(context.Background(), target(), "app", revision, Options{}); err == nil {
			t.Errorf("%d was accepted as a revision", revision)
		}
	}
}

// A configured path that is not there must not fall back to whatever helm is on
// PATH: somebody who typed a path meant that helm, and quietly running a
// different one is how the wrong thing gets upgraded.
func TestAConfiguredPathThatIsNotThereIsAnErrorRatherThanAFallback(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nowhere", "helm")

	tool := Locate(missing)

	if tool.Found {
		t.Fatal("a helm was found at a path with nothing on it")
	}
	if !tool.Configured {
		t.Error("the tool did not report that the path came from settings")
	}
	if !strings.Contains(tool.Reason, missing) {
		t.Errorf("reason = %q, want it to name the path that was tried", tool.Reason)
	}
}

func TestAConfiguredPathIsUsedAndDescribed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake helm is a shell script")
	}

	path := filepath.Join(t.TempDir(), "helm")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho v3.16.2+g13654a5\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	tool := Locate(path)

	if !tool.Found || tool.Path != path {
		t.Fatalf("tool = %+v, want the configured path", tool)
	}
	if !tool.Configured {
		t.Error("the tool did not report that the path came from settings")
	}
	if tool.Version != "v3.16.2+g13654a5" {
		t.Errorf("version = %q, want what helm said", tool.Version)
	}
}

// A directory is not a helm, however much its name suggests it.
func TestADirectoryIsNotAHelm(t *testing.T) {
	if executable(t.TempDir()) {
		t.Error("a directory was taken for something runnable")
	}
}

func TestNoHelmAtAllExplainsWhatToDoAboutIt(t *testing.T) {
	_, err := New(Tool{})

	if err == nil {
		t.Fatal("a missing helm was accepted")
	}
	// The fix has to be in the message: on a Mac the usual cause is not a
	// missing helm at all, it is a PATH the app cannot see.
	if !strings.Contains(err.Error(), "Settings") {
		t.Errorf("error = %q, want it to name where the path is set", err)
	}
}
