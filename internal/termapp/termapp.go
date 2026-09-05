// Package termapp finds the terminal emulators installed on this machine and
// runs a command in one of them.
//
// It exists because "open a shell" has two honest answers. The app can be the
// terminal itself, which is what the dock's terminal view is; or it can hand
// the job to the terminal the user already has set up -- their font, their
// colours, their scrollback, their tmux -- and get out of the way. This is the
// second answer.
//
// Every entry below is a name to look for on PATH and the arguments that
// terminal wants in order to run a command. There is no standard for the
// latter: -e means "run this" in xterm and konsole, "run this one string" in
// xfce4-terminal, and nothing at all in gnome-terminal, which wants -- instead.
// That variety is the whole reason this is a table.
package termapp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Terminal is one terminal emulator that was found on this machine.
type Terminal struct {
	ID string `json:"id"`
	// Name is what the settings view shows.
	Name string `json:"name"`
	// Path is where it was found, shown so that two installs of the same
	// terminal -- a system one and one from a package manager -- can be told
	// apart.
	Path string `json:"path"`
	// Default marks the one this machine would use if nobody chose: the
	// $TERMINAL a desktop session set, the Debian alternative, or the platform's
	// built-in.
	Default bool `json:"default"`
}

// spec is one terminal we know how to drive.
type spec struct {
	id   string
	name string
	// bin is the executable to look for, or the application bundle on macOS.
	bin string
	// argv builds the whole argument list -- everything after the program name
	// -- for running `command` with a window titled `title`.
	argv func(title string, command []string) []string
}

// shellQuote wraps one argument so a shell will pass it through unchanged. Used
// for the terminals that take a command line as a single string rather than as
// arguments, where the alternative is a filename with a space in it silently
// becoming two arguments.
func shellQuote(arg string) string {
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

func joined(command []string) string {
	parts := make([]string, len(command))
	for i, arg := range command {
		parts[i] = shellQuote(arg)
	}
	return strings.Join(parts, " ")
}

// unixTerminals are the emulators looked for on Linux and the BSDs, in the
// order they are offered. The order is a preference between things that are all
// installed: a terminal somebody installed themselves is more likely to be the
// one they want than the one their distribution shipped.
var unixTerminals = []spec{
	{id: "ghostty", name: "Ghostty", bin: "ghostty", argv: func(title string, c []string) []string {
		return append([]string{"--title=" + title, "-e"}, c...)
	}},
	{id: "kitty", name: "kitty", bin: "kitty", argv: func(title string, c []string) []string {
		return append([]string{"--title", title}, c...)
	}},
	{id: "wezterm", name: "WezTerm", bin: "wezterm", argv: func(_ string, c []string) []string {
		return append([]string{"start", "--"}, c...)
	}},
	{id: "alacritty", name: "Alacritty", bin: "alacritty", argv: func(title string, c []string) []string {
		return append([]string{"--title", title, "-e"}, c...)
	}},
	{id: "foot", name: "foot", bin: "foot", argv: func(title string, c []string) []string {
		return append([]string{"--title=" + title}, c...)
	}},
	{id: "gnome-terminal", name: "GNOME Terminal", bin: "gnome-terminal", argv: func(title string, c []string) []string {
		return append([]string{"--title=" + title, "--"}, c...)
	}},
	{id: "konsole", name: "Konsole", bin: "konsole", argv: func(title string, c []string) []string {
		return append([]string{"-p", "tabtitle=" + title, "-e"}, c...)
	}},
	{id: "tilix", name: "Tilix", bin: "tilix", argv: func(title string, c []string) []string {
		return append([]string{"-t", title, "-e"}, c...)
	}},
	{id: "terminator", name: "Terminator", bin: "terminator", argv: func(title string, c []string) []string {
		return append([]string{"-T", title, "-x"}, c...)
	}},
	// Takes its command as one string, not as arguments -- hence the quoting.
	{id: "xfce4-terminal", name: "Xfce Terminal", bin: "xfce4-terminal", argv: func(title string, c []string) []string {
		return []string{"--title=" + title, "--command=" + joined(c)}
	}},
	{id: "xterm", name: "xterm", bin: "xterm", argv: func(title string, c []string) []string {
		return append([]string{"-T", title, "-e"}, c...)
	}},
	// Debian's "whatever this system calls its terminal". Last, because when it
	// resolves it resolves to one of the above, which we would rather name.
	{id: "x-terminal-emulator", name: "System terminal", bin: "x-terminal-emulator", argv: func(_ string, c []string) []string {
		return append([]string{"-e"}, c...)
	}},
}

// macTerminals are the two macOS applications, plus whichever cross-platform
// terminals have put a binary on PATH.
//
// The applications are not run directly: an .app is a directory, and the way to
// start one with something to run is `open -a`, which takes a document rather
// than a command. So the command is written to a temporary script and the
// script is what gets opened -- the same trick every other tool on macOS uses
// for this, and the reason a window there appears a beat later than elsewhere.
var macTerminals = []spec{
	{id: "iterm", name: "iTerm", bin: "/Applications/iTerm.app"},
	{id: "terminal", name: "Terminal", bin: "/System/Applications/Utilities/Terminal.app"},
	{id: "kitty", name: "kitty", bin: "kitty", argv: func(title string, c []string) []string {
		return append([]string{"--title", title}, c...)
	}},
	{id: "wezterm", name: "WezTerm", bin: "wezterm", argv: func(_ string, c []string) []string {
		return append([]string{"start", "--"}, c...)
	}},
	{id: "alacritty", name: "Alacritty", bin: "alacritty", argv: func(title string, c []string) []string {
		return append([]string{"--title", title, "-e"}, c...)
	}},
}

var windowsTerminals = []spec{
	{id: "wt", name: "Windows Terminal", bin: "wt.exe", argv: func(title string, c []string) []string {
		return append([]string{"new-tab", "--title", title}, c...)
	}},
	{id: "powershell", name: "PowerShell", bin: "powershell.exe", argv: func(_ string, c []string) []string {
		return []string{"-NoExit", "-Command", strings.Join(c, " ")}
	}},
	{id: "cmd", name: "Command Prompt", bin: "cmd.exe", argv: func(title string, c []string) []string {
		return append([]string{"/c", "start", title, "cmd", "/k"}, c...)
	}},
}

// known is the table for this platform.
func known() []spec {
	switch runtime.GOOS {
	case "darwin":
		return macTerminals
	case "windows":
		return windowsTerminals
	default:
		return unixTerminals
	}
}

// find locates one spec's program, returning where it is and whether it is
// there at all. An absolute path is an application bundle and is checked as a
// file; anything else is looked for on PATH.
func find(s spec) (string, bool) {
	if filepath.IsAbs(s.bin) {
		if _, err := os.Stat(s.bin); err == nil {
			return s.bin, true
		}
		return "", false
	}
	path, err := exec.LookPath(s.bin)
	if err != nil {
		return "", false
	}
	return path, true
}

// preferred names the terminal this machine would use on its own.
//
// $TERMINAL is what a desktop session or a dotfile sets to answer exactly this
// question, so it wins where it is set. Otherwise the platform decides: the
// Debian alternative on Linux, Terminal on macOS, Windows Terminal where it is
// installed.
func preferred(found []Terminal) string {
	if chosen := strings.TrimSpace(os.Getenv("TERMINAL")); chosen != "" {
		name := filepath.Base(chosen)
		for _, t := range found {
			if t.ID == name || filepath.Base(t.Path) == name {
				return t.ID
			}
		}
	}

	var order []string
	switch runtime.GOOS {
	case "darwin":
		order = []string{"terminal", "iterm"}
	case "windows":
		order = []string{"wt", "powershell", "cmd"}
	default:
		order = []string{"x-terminal-emulator"}
	}
	for _, id := range order {
		for _, t := range found {
			if t.ID == id {
				return t.ID
			}
		}
	}
	if len(found) > 0 {
		return found[0].ID
	}
	return ""
}

// Available lists the terminal emulators found on this machine, with the one
// that would be used by default marked.
func Available() []Terminal {
	found := []Terminal{}
	for _, s := range known() {
		if path, ok := find(s); ok {
			found = append(found, Terminal{ID: s.id, Name: s.name, Path: path})
		}
	}

	def := preferred(found)
	for i := range found {
		if found[i].ID == def {
			found[i].Default = true
		}
	}
	return found
}

// specFor returns the table entry for an id, or the default one when the id is
// empty or names something that is no longer installed -- a terminal
// uninstalled since it was chosen should fall back rather than fail.
func specFor(id string) (spec, error) {
	table := known()
	if id != "" {
		for _, s := range table {
			if s.id != id {
				continue
			}
			if _, ok := find(s); ok {
				return s, nil
			}
			break
		}
	}

	def := preferred(Available())
	for _, s := range table {
		if s.id == def {
			return s, nil
		}
	}
	return spec{}, fmt.Errorf("no terminal emulator was found on this machine")
}

// script writes a command out as a shell script for macOS's `open` to run, and
// returns the path.
//
// The script removes itself as its first act, so nothing is left behind in the
// temporary directory once the terminal has read it.
func script(command []string) (string, error) {
	file, err := os.CreateTemp("", "k8sdockside-*.command")
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	body := "#!/bin/sh\nrm -f " + shellQuote(file.Name()) + "\nexec " + joined(command) + "\n"
	if _, err := file.WriteString(body); err != nil {
		return "", err
	}
	// Executable, because `open` runs it rather than reads it -- and by its
	// owner alone, because it names a kubeconfig and a context. 0700 is the
	// least this can be and still work.
	// #nosec G302 -- a script that is not executable is not a script.
	if err := os.Chmod(file.Name(), 0o700); err != nil {
		return "", err
	}
	return file.Name(), nil
}

// Launch runs a command in a terminal window and returns as soon as it has been
// started. An empty id means "whichever this machine uses by default".
func Launch(id, title string, command []string) error {
	if len(command) == 0 {
		return fmt.Errorf("nothing to run")
	}
	chosen, err := specFor(id)
	if err != nil {
		return err
	}
	path, ok := find(chosen)
	if !ok {
		return fmt.Errorf("%s is no longer installed", chosen.name)
	}

	// The macOS applications, which are opened with a document rather than run
	// with arguments.
	if chosen.argv == nil {
		file, err := script(command)
		if err != nil {
			return err
		}
		// #nosec G204 -- the arguments are this app's own: an application path
		// from the table above and a script it just wrote.
		return exec.Command("/usr/bin/open", "-a", path, file).Start()
	}

	// #nosec G204 -- the program is one of the terminals found on PATH above,
	// and the arguments are built by this package rather than passed through
	// from the frontend.
	cmd := exec.Command(path, chosen.argv(title, command)...)
	return cmd.Start()
}

// Kubectl is where kubectl is, or an error saying it is not here.
//
// An external terminal needs it: the app's own connection cannot be handed to
// another process, so what runs over there is kubectl with the same kubeconfig
// and context this window is using.
func Kubectl() (string, error) {
	path, err := exec.LookPath("kubectl")
	if err != nil {
		return "", fmt.Errorf(
			"kubectl was not found on your PATH, and an external terminal opens the shell by running it -- " +
				"install kubectl, or use the terminal built into this window",
		)
	}
	return path, nil
}
