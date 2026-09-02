// Package kube reads kubeconfig files from local disk and serves the cluster
// data the UI renders.
//
// Resource listings (pods, deployments, ...) are currently stubs: they are
// shaped exactly like the real thing and are deterministic per context, so the
// UI can be built and exercised before a live API client is wired in. Swapping
// in client-go means replacing the bodies in stub.go, not the types here.
package kube

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Where a kubeconfig file came from. Shown in the sidebar so the user can tell
// an auto-discovered file from one they added by hand.
const (
	SourceDefault = "default" // ~/.kube/config
	SourceEnv     = "env"     // listed in $KUBECONFIG
	SourceScan    = "scan"    // found by scanning ~/.kube
	SourceManual  = "manual"  // added by the user
)

// maxConfigSize caps how much of a candidate file we are willing to read. A
// kubeconfig is a few KB; anything larger is not one and should not be slurped
// into memory just because it matched a glob.
const maxConfigSize = 4 << 20

// Context is one kubeconfig context flattened with the details the UI needs, so
// the frontend never has to cross-reference the clusters and users lists.
type Context struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Cluster   string `json:"cluster"`
	User      string `json:"user"`
	Namespace string `json:"namespace"`
	Server    string `json:"server"`
	File      string `json:"file"`
	Current   bool   `json:"current"`
}

// File is a kubeconfig file on disk together with the contexts parsed out of
// it. A file that failed to parse is still returned, with Error set, so the
// sidebar can show it as broken rather than silently dropping it.
type File struct {
	Path     string    `json:"path"`
	Source   string    `json:"source"`
	Contexts []Context `json:"contexts"`
	Error    string    `json:"error"`
}

// rawConfig is the subset of the kubeconfig schema we care about. Everything
// else in the file (credentials, extensions, proxy settings) is deliberately
// not parsed: we only need enough to list and label contexts.
type rawConfig struct {
	Kind           string `yaml:"kind"`
	CurrentContext string `yaml:"current-context"`
	Clusters       []struct {
		Name    string `yaml:"name"`
		Cluster struct {
			Server string `yaml:"server"`
		} `yaml:"cluster"`
	} `yaml:"clusters"`
	Contexts []struct {
		Name    string `yaml:"name"`
		Context struct {
			Cluster   string `yaml:"cluster"`
			User      string `yaml:"user"`
			Namespace string `yaml:"namespace"`
		} `yaml:"context"`
	} `yaml:"contexts"`
}

// ContextID is the stable key a context is known by across syncs, and the key
// its user settings (alias, colour) are stored under. Context names are only
// unique within a single file, so the file path has to be part of the identity.
func ContextID(path, name string) string {
	return path + "::" + name
}

// ParseFile reads one kubeconfig and flattens its contexts. A read or parse
// failure is reported in the returned File rather than as an error, because the
// caller is building a list and one bad file should not sink the rest.
func ParseFile(path, source string) File {
	f := File{Path: path, Source: source, Contexts: []Context{}}

	info, err := os.Stat(path)
	if err != nil {
		f.Error = err.Error()
		return f
	}
	if info.IsDir() {
		f.Error = "not a file"
		return f
	}
	if info.Size() > maxConfigSize {
		f.Error = fmt.Sprintf("file is too large to be a kubeconfig (%d bytes)", info.Size())
		return f
	}

	data, err := os.ReadFile(path)
	if err != nil {
		f.Error = err.Error()
		return f
	}

	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		f.Error = "not valid YAML: " + err.Error()
		return f
	}
	if raw.Kind != "" && raw.Kind != "Config" {
		f.Error = "not a kubeconfig (kind: " + raw.Kind + ")"
		return f
	}
	if len(raw.Contexts) == 0 {
		f.Error = "no contexts defined"
		return f
	}

	servers := make(map[string]string, len(raw.Clusters))
	for _, c := range raw.Clusters {
		servers[c.Name] = c.Cluster.Server
	}

	for _, c := range raw.Contexts {
		if c.Name == "" {
			continue
		}
		f.Contexts = append(f.Contexts, Context{
			ID:        ContextID(path, c.Name),
			Name:      c.Name,
			Cluster:   c.Context.Cluster,
			User:      c.Context.User,
			Namespace: c.Context.Namespace,
			Server:    servers[c.Context.Cluster],
			File:      path,
			Current:   c.Name == raw.CurrentContext,
		})
	}
	return f
}

// candidate is a path we intend to try, tagged with how we found it.
type candidate struct {
	path   string
	source string
}

// Discover finds every kubeconfig the app should offer: ~/.kube/config, the
// entries in $KUBECONFIG, the paths the user added by hand, and anything in
// ~/.kube that looks like a kubeconfig. Files are returned in that order, which
// is also the precedence used when the same file turns up twice.
func Discover(manual []string) []File {
	var candidates []candidate

	home, err := os.UserHomeDir()
	if err == nil {
		candidates = append(candidates, candidate{filepath.Join(home, ".kube", "config"), SourceDefault})
	}

	for _, p := range filepath.SplitList(os.Getenv("KUBECONFIG")) {
		if p = strings.TrimSpace(p); p != "" {
			candidates = append(candidates, candidate{p, SourceEnv})
		}
	}

	// User-added paths come before the directory scan so that a file the user
	// chose explicitly is labelled "manual" even if the scan would also find it.
	for _, p := range manual {
		if p = strings.TrimSpace(p); p != "" {
			candidates = append(candidates, candidate{p, SourceManual})
		}
	}

	if err == nil {
		for _, p := range scanKubeDir(filepath.Join(home, ".kube")) {
			candidates = append(candidates, candidate{p, SourceScan})
		}
	}

	files := make([]File, 0, len(candidates))
	seen := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		resolved := resolve(c.path)
		if resolved == "" || seen[resolved] {
			continue
		}
		seen[resolved] = true

		parsed := ParseFile(resolved, c.source)
		// A glob hit that turns out not to be a kubeconfig is not an error the
		// user needs to see -- they never asked for that file. Paths they named
		// (or that $KUBECONFIG names) are reported even when broken.
		if parsed.Error != "" && c.source == SourceScan {
			continue
		}
		files = append(files, parsed)
	}
	return files
}

// scanKubeDir returns the entries of ~/.kube that are plausibly kubeconfigs.
// Sub-directories (notably ~/.kube/cache) are skipped, as is the well-known
// "config" file, which is always added ahead of the scan as SourceDefault.
func scanKubeDir(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var found []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "config" || strings.HasPrefix(name, ".") {
			continue
		}
		if !looksLikeKubeconfigName(name) {
			continue
		}
		found = append(found, filepath.Join(dir, name))
	}
	sort.Strings(found)
	return found
}

// looksLikeKubeconfigName is a cheap name filter run before opening a file. It
// only decides what is worth parsing; ParseFile has the final say.
func looksLikeKubeconfigName(name string) bool {
	lower := strings.ToLower(name)
	switch filepath.Ext(lower) {
	case ".yaml", ".yml", ".kubeconfig", ".conf":
		return true
	}
	return strings.HasPrefix(lower, "config") || strings.HasPrefix(lower, "kubeconfig")
}

// resolve turns a user-supplied path into the absolute, symlink-free form used
// to detect duplicates. A leading "~" is expanded because paths typed into the
// UI are not shell-expanded on the way in.
func resolve(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		path = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/"))
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	// A symlink and its target are the same config; collapse them so the file
	// is not listed twice. Broken symlinks keep the abs path so the failure is
	// still reported to the user.
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real
	}
	return abs
}
