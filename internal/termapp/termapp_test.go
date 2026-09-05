package termapp

import (
	"os"
	"strings"
	"testing"
)

func TestShellQuoteSurvivesWhatAPathCanContain(t *testing.T) {
	// The terminals that take a command line as one string are the reason this
	// exists: without it a kubeconfig in "My Cluster Configs" becomes two
	// arguments, and one holding an apostrophe ends the quoting early.
	cases := []struct {
		in   string
		want string
	}{
		{"kubectl", "'kubectl'"},
		{"/home/a b/config", "'/home/a b/config'"},
		{"it's", `'it'\''s'`},
	}

	for _, tc := range cases {
		if got := shellQuote(tc.in); got != tc.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestJoinedQuotesEveryArgument(t *testing.T) {
	got := joined([]string{"kubectl", "--kubeconfig", "/home/a b/config", "exec", "-it", "web"})
	want := `'kubectl' '--kubeconfig' '/home/a b/config' 'exec' '-it' 'web'`
	if got != want {
		t.Fatalf("joined() = %q, want %q", got, want)
	}
}

func TestEveryTerminalKnowsHowToRunACommand(t *testing.T) {
	// The whole point of the table is that each terminal wants its command
	// differently. An entry with no way to run one is an entry that would open
	// an empty window -- except on macOS, where the two applications are opened
	// with a script instead and deliberately have no argv.
	for _, table := range [][]spec{unixTerminals, windowsTerminals} {
		for _, s := range table {
			if s.id == "" || s.name == "" || s.bin == "" {
				t.Errorf("a terminal is missing its id, name or program: %+v", s)
			}
			if s.argv == nil {
				t.Errorf("%s has no way to run a command", s.id)
				continue
			}
			argv := s.argv("a title", []string{"kubectl", "exec", "-it", "web"})
			if len(argv) == 0 {
				t.Errorf("%s builds an empty argument list", s.id)
			}
			if !strings.Contains(strings.Join(argv, " "), "kubectl") {
				t.Errorf("%s drops the command it was given: %v", s.id, argv)
			}
		}
	}
}

func TestMacApplicationsAreOpenedWithAScriptRatherThanArguments(t *testing.T) {
	// An .app is a directory: it is started with `open -a`, which takes a
	// document. The absent argv is what routes them down that path in Launch.
	for _, s := range macTerminals {
		if strings.HasPrefix(s.bin, "/") && s.argv != nil {
			t.Errorf("%s is an application bundle but carries arguments", s.id)
		}
	}
}

func TestPreferredFollowsTheEnvironmentBeforeTheTable(t *testing.T) {
	found := []Terminal{
		{ID: "kitty", Name: "kitty", Path: "/usr/bin/kitty"},
		{ID: "xterm", Name: "xterm", Path: "/usr/bin/xterm"},
	}

	// $TERMINAL is what a desktop session or a dotfile sets to answer exactly
	// this question, so it wins where it is set.
	t.Setenv("TERMINAL", "/usr/bin/kitty")
	if got := preferred(found); got != "kitty" {
		t.Fatalf("preferred() = %q, want kitty", got)
	}

	// One that is set but not installed here falls through rather than naming
	// something that cannot be launched.
	t.Setenv("TERMINAL", "wezterm")
	if got := preferred(found); got == "wezterm" {
		t.Fatal("preferred() named a terminal that is not installed")
	}

	// Nothing found at all is an answer too, and Launch reports it in words.
	t.Setenv("TERMINAL", "")
	if got := preferred(nil); got != "" {
		t.Fatalf("preferred() = %q with nothing installed, want empty", got)
	}
}

func TestScriptRemovesItselfAndRunsWhatItWasGiven(t *testing.T) {
	path, err := script([]string{"kubectl", "exec", "-it", "web"})
	if err != nil {
		t.Fatalf("writing the script: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	raw, err := os.ReadFile(path) // #nosec G304 -- a file this test just wrote
	if err != nil {
		t.Fatalf("reading it back: %v", err)
	}
	body := string(raw)
	// It deletes itself first: nothing should be left in the temporary
	// directory once the terminal has read it.
	if !strings.Contains(body, "rm -f "+shellQuote(path)) {
		t.Errorf("the script does not remove itself:\n%s", body)
	}
	if !strings.Contains(body, "exec 'kubectl' 'exec' '-it' 'web'") {
		t.Errorf("the script does not run the command:\n%s", body)
	}
}
