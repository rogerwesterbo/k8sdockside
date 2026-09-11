package plugins

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// A plugin kept in a repository of its own is installed by cloning it into the
// plugins folder, and updated by pulling. Nothing more clever than that: the
// clone is an ordinary folder the loader reads like any other, so a plugin
// installed this way and one copied in by hand are the same thing afterwards,
// and `git` itself is the version manager.
//
// The repository's plugin.json has to be at its root, because the loader reads
// one level into the plugins folder and no deeper.

// gitTimeout bounds a clone or a pull. A plugin is a manifest and some static
// files; one that takes longer than this is not one.
const gitTimeout = 2 * time.Minute

// gitURL is what a repository address may look like: https, ssh, or the scp
// form git@host:owner/repo. Deliberately not file://, ext:: or anything else
// git would accept -- those run things, or read things, this app should not.
var gitURL = regexp.MustCompile(`^(https://[^\s]+|ssh://[^\s]+|[A-Za-z0-9._-]+@[A-Za-z0-9.-]+:[^\s]+)$`)

// ValidGitURL reports whether a repository address is one this app will clone.
func ValidGitURL(url string) bool {
	return gitURL.MatchString(url) && !strings.HasPrefix(url, "-")
}

// RepoFolder is the folder a repository is cloned into: its last path element,
// without .git, kept to what a plugin id may contain.
func RepoFolder(url string) (string, error) {
	url = strings.TrimSuffix(strings.TrimRight(url, "/"), ".git")
	at := strings.LastIndexAny(url, "/:")
	name := strings.ToLower(url[at+1:])
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		case r == '_' || r == '.':
			b.WriteRune('-')
		}
	}
	folder := strings.Trim(b.String(), "-")
	if folder == "" {
		return "", fmt.Errorf("cannot tell what to call the folder for %s", url)
	}
	return folder, nil
}

// gitPath finds git, with the reason in words when it cannot.
func gitPath() (string, error) {
	path, err := exec.LookPath("git")
	if err != nil {
		return "", errors.New("installing a plugin from a repository needs git, and git is not on this machine's PATH")
	}
	return path, nil
}

func runGit(dir string, args ...string) error {
	git, err := gitPath()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, git, args...) // #nosec G204 -- the URL is checked by ValidGitURL and passed after --
	cmd.Dir = dir
	// Never stop to ask for a password: there is no terminal to ask in, and a
	// clone waiting on one would hang until the timeout.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=")
	out, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if text == "" {
			text = err.Error()
		}
		return fmt.Errorf("git %s: %s", args[0], text)
	}
	return nil
}

// Clone installs a plugin repository into dir and returns the folder it made.
// A folder already there is left alone: that is Update's job, and cloning over
// it would throw away whatever is in it.
func Clone(dir, url string) (string, error) {
	url = strings.TrimSpace(url)
	if !ValidGitURL(url) {
		return "", fmt.Errorf("%q is not a repository address this app will clone -- use https://, ssh:// or git@host:owner/repo", url)
	}
	folder, err := RepoFolder(url)
	if err != nil {
		return "", err
	}
	if err := EnsureDir(dir); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, folder)
	if _, err := os.Stat(dest); err == nil {
		return "", fmt.Errorf("%s is already there -- update it instead, or remove the folder first", dest)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if err := runGit(dir, "clone", "--depth", "1", "--", url, folder); err != nil {
		return "", err
	}
	return dest, nil
}

// RepoOf is the repository a plugin's file is in, if it is in one: its own
// folder, or the folder above it for a pack kept in a subfolder.
func RepoOf(p Plugin) (string, bool) {
	if p.Builtin() || p.Origin == "" {
		return "", false
	}
	dir := filepath.Dir(p.Origin)
	for range 2 {
		if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil && info.IsDir() {
			return dir, true
		}
		dir = filepath.Dir(dir)
	}
	return "", false
}

// Pull updates a cloned plugin to what its repository has now. Fast-forward
// only: a clone someone has edited in place is theirs, and merging into it is
// not a decision to make for them.
func Pull(repo string) error {
	return runGit(repo, "pull", "--ff-only")
}
