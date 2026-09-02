package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/roger/k8sdockside/internal/appconfig"
	"github.com/roger/k8sdockside/internal/kube"
)

const sampleConfig = `apiVersion: v1
kind: Config
current-context: one
clusters:
- name: one
  cluster:
    server: https://10.0.0.1:6443
contexts:
- name: one
  context:
    cluster: one
    user: admin
users:
- name: admin
`

// service returns a KubeconfigService backed by a settings file in a temp
// directory, with HOME pointed somewhere empty so the real ~/.kube plays no
// part in what the tests see.
func service(t *testing.T) *KubeconfigService {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("KUBECONFIG", "")

	store, err := appconfig.Open()
	if err != nil {
		t.Fatal(err)
	}
	return NewKubeconfigService(store)
}

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAddFilesKeepsTheGoodOnesWhenOneIsBad(t *testing.T) {
	s := service(t)
	dir := t.TempDir()

	good1 := write(t, dir, "a.config", sampleConfig)
	good2 := write(t, dir, "b.config", sampleConfig)
	bad := write(t, dir, "notes.txt", "not a kubeconfig\n")

	files, err := s.AddFiles([]string{good1, bad, good2})
	if err == nil {
		t.Fatal("want an error naming the file that failed")
	}
	if !strings.Contains(err.Error(), "notes.txt") {
		t.Errorf("error = %q, want it to name notes.txt", err)
	}
	if !strings.Contains(err.Error(), "added 2 of 3") {
		t.Errorf("error = %q, want it to say how many were added", err)
	}

	// The point of the exercise: a bad file among good ones must not throw the
	// good ones away, or the user has to pick them all again.
	if len(files) != 2 {
		t.Fatalf("got %d files, want the 2 good ones: %+v", len(files), files)
	}
}

func TestAddFilesReportsPlainlyWhenNoneWork(t *testing.T) {
	s := service(t)
	dir := t.TempDir()
	bad := write(t, dir, "notes.txt", "nope\n")

	files, err := s.AddFiles([]string{bad})
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), "added") {
		t.Errorf("error = %q, want no partial-success wording when nothing worked", err)
	}
	if len(files) != 0 {
		t.Errorf("got %d files, want none", len(files))
	}
}

func TestAddFolderTakesEveryConfigWhateverItIsNamed(t *testing.T) {
	s := service(t)
	dir := t.TempDir()

	write(t, dir, "hrw.config", sampleConfig)
	write(t, dir, "kubeconfig-test01-af3e", sampleConfig)
	write(t, dir, "readme.md", "# notes\n")

	files, err := s.AddFolder(dir)
	if err != nil {
		t.Fatalf("AddFolder: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2: %+v", len(files), files)
	}
	for _, f := range files {
		if f.Source != kube.SourceFolder {
			t.Errorf("%s: source = %q", filepath.Base(f.Path), f.Source)
		}
	}
	if got := s.Folders(); len(got) != 1 || got[0] != dir {
		t.Errorf("Folders() = %v, want [%s]", got, dir)
	}
}

func TestAddFolderRefusesAFolderWithNothingInIt(t *testing.T) {
	s := service(t)
	dir := t.TempDir()
	write(t, dir, "readme.md", "# no configs here\n")

	if _, err := s.AddFolder(dir); err == nil {
		t.Fatal("want an error rather than a folder that silently does nothing")
	}
	if got := s.Folders(); len(got) != 0 {
		t.Errorf("Folders() = %v, want it not to have been stored", got)
	}
}

func TestAddFolderRefusesAFile(t *testing.T) {
	s := service(t)
	path := write(t, t.TempDir(), "a.config", sampleConfig)

	if _, err := s.AddFolder(path); err == nil {
		t.Fatal("want an error")
	}
}

func TestRemoveFolderTakesItsConfigsWithIt(t *testing.T) {
	s := service(t)
	dir := t.TempDir()
	write(t, dir, "a.config", sampleConfig)

	if _, err := s.AddFolder(dir); err != nil {
		t.Fatal(err)
	}
	files, err := s.RemoveFolder(dir)
	if err != nil {
		t.Fatalf("RemoveFolder: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("got %d files, want none: %+v", len(files), files)
	}
	if got := s.Folders(); len(got) != 0 {
		t.Errorf("Folders() = %v, want empty", got)
	}
}

func TestAFileFromAWatchedFolderCanBeRemovedOnItsOwn(t *testing.T) {
	s := service(t)
	dir := t.TempDir()
	keep := write(t, dir, "a.config", sampleConfig)
	drop := write(t, dir, "b.config", sampleConfig)

	if _, err := s.AddFolder(dir); err != nil {
		t.Fatal(err)
	}

	files, err := s.RemoveFile(drop)
	if err != nil {
		t.Fatalf("RemoveFile: %v", err)
	}
	if len(files) != 1 || files[0].Path != keep {
		t.Fatalf("got %+v, want only %s", files, keep)
	}
	if got := s.Excluded(); len(got) != 1 || got[0] != drop {
		t.Errorf("Excluded() = %v, want [%s]", got, drop)
	}
}

func TestAnExcludedFileStaysGoneAcrossSyncs(t *testing.T) {
	s := service(t)
	dir := t.TempDir()
	write(t, dir, "a.config", sampleConfig)
	drop := write(t, dir, "b.config", sampleConfig)

	if _, err := s.AddFolder(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RemoveFile(drop); err != nil {
		t.Fatal(err)
	}

	// The whole reason exclusions exist: a rescan finds the file on disk again
	// and must not bring it back.
	if files := s.Sync(); len(files) != 1 {
		t.Errorf("got %d files after sync, want 1: %+v", len(files), files)
	}
}

func TestRestoreFileBringsAHiddenConfigBack(t *testing.T) {
	s := service(t)
	dir := t.TempDir()
	write(t, dir, "a.config", sampleConfig)
	drop := write(t, dir, "b.config", sampleConfig)

	if _, err := s.AddFolder(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RemoveFile(drop); err != nil {
		t.Fatal(err)
	}

	files, err := s.RestoreFile(drop)
	if err != nil {
		t.Fatalf("RestoreFile: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("got %d files, want 2: %+v", len(files), files)
	}
	if got := s.Excluded(); len(got) != 0 {
		t.Errorf("Excluded() = %v, want empty", got)
	}
}

func TestRemovingAManualFileForgetsItRatherThanHidingIt(t *testing.T) {
	s := service(t)
	path := write(t, t.TempDir(), "a.config", sampleConfig)

	if _, err := s.AddFile(path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RemoveFile(path); err != nil {
		t.Fatalf("RemoveFile: %v", err)
	}

	// A file the user added is theirs to forget; recording it as an exclusion
	// would leave junk behind that re-adding the same file would trip over.
	if got := s.Excluded(); len(got) != 0 {
		t.Errorf("Excluded() = %v, want empty", got)
	}
	if files := s.Files(); len(files) != 0 {
		t.Errorf("got %d files, want none", len(files))
	}

	// ...and re-adding it works, rather than being silently hidden.
	files, err := s.AddFile(path)
	if err != nil {
		t.Fatalf("re-adding: %v", err)
	}
	if len(files) != 1 {
		t.Errorf("got %d files after re-adding, want 1", len(files))
	}
}

func TestUnwatchingAFolderForgetsWhatWasHiddenInIt(t *testing.T) {
	s := service(t)
	dir := t.TempDir()
	write(t, dir, "a.config", sampleConfig)
	drop := write(t, dir, "b.config", sampleConfig)

	if _, err := s.AddFolder(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RemoveFile(drop); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RemoveFolder(dir); err != nil {
		t.Fatal(err)
	}

	// Re-adding the folder must show everything in it. Keeping the exclusion
	// would mean quietly returning fewer files with nothing explaining why.
	files, err := s.AddFolder(dir)
	if err != nil {
		t.Fatalf("re-adding the folder: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("got %d files, want both back: %+v", len(files), files)
	}
}

func TestWatchedFolderIsRescannedOnSync(t *testing.T) {
	s := service(t)
	dir := t.TempDir()
	write(t, dir, "a.config", sampleConfig)

	if _, err := s.AddFolder(dir); err != nil {
		t.Fatal(err)
	}

	// A config dropped in later is the reason folders are stored as folders
	// rather than expanded into their files when they are added.
	write(t, dir, "b.config", sampleConfig)

	if files := s.Sync(); len(files) != 2 {
		t.Errorf("got %d files after sync, want 2: %+v", len(files), files)
	}
}
