package kube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	utilexec "k8s.io/client-go/util/exec"
)

func TestMissingExecutableRecognisesAShellThatIsNotInTheImage(t *testing.T) {
	// What a container runtime says when the command does not exist. Both the
	// exit status and the prose are matched, because which one arrives depends
	// on the runtime rather than on anything this app does.
	cases := []struct {
		name string
		err  error
	}{
		{"the runtime's own words", errors.New(`OCI runtime exec failed: exec failed: unable to start container process: exec: "bash": executable file not found in $PATH`)},
		{"a not-found exit status", utilexec.CodeExitError{Err: errors.New("command terminated with exit code 127"), Code: 127}},
		{"a found-but-not-executable status", utilexec.CodeExitError{Err: errors.New("command terminated with exit code 126"), Code: 126}},
		{"wrapped, as the stream layer hands it back", fmt.Errorf("opening a shell: %w", utilexec.CodeExitError{Err: errors.New("boom"), Code: 127})},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !missingExecutable(tc.err) {
				t.Fatalf("missingExecutable(%v) = false, want true", tc.err)
			}
		})
	}
}

func TestMissingExecutableLeavesRealFailuresAlone(t *testing.T) {
	// These must not fall through to the next shell: retrying a denied exec
	// with sh would ask the user's permissions the same question twice and
	// report the second refusal instead of the first.
	cases := []struct {
		name string
		err  error
	}{
		{"a refusal", errors.New(`pods "web" is forbidden: User "dev" cannot create resource "pods/exec"`)},
		{"a pod that has gone", errors.New(`pods "web" not found`)},
		{"a shell that ran and then failed", utilexec.CodeExitError{Err: errors.New("command terminated with exit code 1"), Code: 1}},
		{"nothing at all", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if missingExecutable(tc.err) {
				t.Fatalf("missingExecutable(%v) = true, want false", tc.err)
			}
		})
	}
}

func TestCountedRemembersWhetherAnythingWasDrawn(t *testing.T) {
	screen := &counted{to: discard{}}
	if screen.written.Load() {
		t.Fatal("a terminal nothing has been written to reports that it has")
	}

	// An empty write is not output: a stream that closes without saying
	// anything must still be allowed to fall through to the next shell.
	if _, err := screen.Write(nil); err != nil {
		t.Fatalf("writing nothing: %v", err)
	}
	if screen.written.Load() {
		t.Fatal("an empty write counted as output")
	}

	if _, err := screen.Write([]byte("$ ")); err != nil {
		t.Fatalf("writing a prompt: %v", err)
	}
	if !screen.written.Load() {
		t.Fatal("a prompt on screen did not count as output")
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// The reason the relays exist, checked directly: a shell that was not in the
// image leaves client-go's stdin copier sitting in Read on the reader it was
// given. Handed the same reader, that dead copier is first in the queue and
// takes the next thing the user types -- which is then written to a closed
// stream and lost, leaving the shell that did open looking hung.
func TestStdinGoesToTheAttemptThatIsLive(t *testing.T) {
	source, typing := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	relay := newStdinRelay(ctx, source)

	// The attempt that failed, with a copier still on it.
	failed := relay.attach()
	stolen := readInto(failed)
	relay.detach()

	// The one that opened.
	live := relay.attach()
	got := readInto(live)

	if _, err := typing.Write([]byte("whoami\n")); err != nil {
		t.Fatalf("typing: %v", err)
	}

	if text := waitFor(t, got); text != "whoami\n" {
		t.Fatalf("the live shell received %q", text)
	}
	// The dead one's reader is closed rather than left waiting, so its copier
	// ends instead of lurking.
	if text := waitFor(t, stolen); text != "" {
		t.Fatalf("the failed attempt took %q from the live one", text)
	}
}

func TestStdinTypedBetweenAttemptsIsNotLost(t *testing.T) {
	source, typing := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	relay := newStdinRelay(ctx, source)
	if _, err := typing.Write([]byte("ls\n")); err != nil {
		t.Fatalf("typing: %v", err)
	}

	// Typed while one attempt had ended and the next had not started. A
	// fallback takes milliseconds, but a keystroke inside them is still a
	// keystroke.
	got := readInto(relay.attach())
	if text := waitFor(t, got); text != "ls\n" {
		t.Fatalf("what was typed between attempts came out as %q", text)
	}
}

func TestSizesFollowTheLiveAttemptAndEndWithIt(t *testing.T) {
	from := make(chan TerminalSize, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	relay := newSizeRelay(ctx, from)
	sizes := relay.attach()

	from <- TerminalSize{Cols: 120, Rows: 40}
	select {
	case got := <-sizes:
		if got.Cols != 120 || got.Rows != 40 {
			t.Fatalf("the live attempt was told %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("the live attempt was never told the size")
	}

	// A second attempt is told the size it should already be, rather than being
	// left at the 80x24 a shell assumes until the window is dragged.
	relay.detach()
	again := relay.attach()
	select {
	case got := <-again:
		if got.Cols != 120 {
			t.Fatalf("the next attempt started at %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("the next attempt was not told the size it should be")
	}

	// Closing is how client-go's resize loop is told there will be no more.
	relay.detach()
	if _, open := <-again; open {
		t.Fatal("a detached attempt's size channel is still open")
	}
}

// readInto drains a reader on its own goroutine, the way client-go's copier
// does, and reports what it got.
func readInto(r io.Reader) <-chan string {
	out := make(chan string, 1)
	go func() {
		var got []byte
		buf := make([]byte, 64)
		for {
			n, err := r.Read(buf)
			got = append(got, buf[:n]...)
			if err != nil {
				break
			}
			if n > 0 {
				break
			}
		}
		out <- string(got)
	}()
	return out
}

func waitFor(t *testing.T, got <-chan string) string {
	t.Helper()
	select {
	case text := <-got:
		return text
	case <-time.After(2 * time.Second):
		t.Fatal("nothing arrived, and nothing said so")
		return ""
	}
}
