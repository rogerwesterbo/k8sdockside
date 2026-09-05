package themes

import (
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// write drops a theme file into dir and returns its path.
func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

// ids is the catalogue's theme ids, for comparing what loaded.
func ids(themes []Theme) []string {
	out := make([]string, 0, len(themes))
	for _, theme := range themes {
		out = append(out, theme.ID)
	}
	return out
}

func TestBuiltinThemesLoad(t *testing.T) {
	builtin := Builtin()
	if len(builtin) < 2 {
		t.Fatalf("got %d builtin themes, want the two defaults at least", len(builtin))
	}
	if builtin[0].ID != DefaultID {
		t.Errorf("first builtin is %q, want the default %q -- it is what the gallery opens on", builtin[0].ID, DefaultID)
	}

	seen := map[string]bool{}
	for _, theme := range builtin {
		if seen[theme.ID] {
			t.Errorf("builtin id %q appears twice", theme.ID)
		}
		seen[theme.ID] = true
		if theme.Name == "" || theme.Tagline == "" {
			t.Errorf("builtin %q has no name or tagline; both are shown in the gallery", theme.ID)
		}
		if !theme.Builtin() {
			t.Errorf("builtin %q has origin %q", theme.ID, theme.Origin)
		}
	}
}

// The two defaults are what every other theme inherits from, so they are the
// only ones that have to be complete. A gap in either would leave a token
// undefined for every theme that did not set it.
func TestDefaultThemesSetEveryToken(t *testing.T) {
	for _, id := range []string{DefaultID, LightID} {
		theme, ok := Catalogue{Themes: Builtin()}.Find(id)
		if !ok {
			t.Fatalf("default theme %q is missing", id)
		}
		for _, token := range TokenNames() {
			if theme.Tokens[token] == "" {
				t.Errorf("%s does not set %q", id, token)
			}
		}
	}
}

// Every palette that ships has to be readable, which is the one thing about a
// theme that is not a matter of taste.
func TestBuiltinThemesAreReadable(t *testing.T) {
	for _, theme := range Builtin() {
		for _, problem := range theme.Warnings {
			t.Errorf("%s: %s", theme.ID, problem)
		}
	}
}

// The gallery draws every theme from one call, so each theme has to arrive with
// its own tokens already filled in rather than needing a round trip per swatch.
func TestThemesArriveResolved(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "sparse.json", `{"id": "sparse", "name": "Sparse", "base": "light", "tokens": {"accent": "#ff0000"}}`)

	for _, theme := range Load(dir, nil).Themes {
		for _, token := range TokenNames() {
			if theme.Resolved[token] == "" {
				t.Errorf("%s arrived without a value for %q", theme.ID, token)
			}
		}
	}
}

func TestResolveInheritsFromBase(t *testing.T) {
	theme := Theme{ID: "sparse", Name: "Sparse", Base: BaseLight, Tokens: map[string]string{"accent": "#ff0000"}}
	resolved := Resolve(theme)

	if resolved["accent"] != "#ff0000" {
		t.Errorf("accent is %q, want the theme's own", resolved["accent"])
	}
	light, _ := Catalogue{Themes: Builtin()}.Find(LightID)
	if resolved["bg"] != light.Tokens["bg"] {
		t.Errorf("bg is %q, want the light default %q", resolved["bg"], light.Tokens["bg"])
	}
	for _, token := range TokenNames() {
		if resolved[token] == "" {
			t.Errorf("resolving left %q empty", token)
		}
	}
}

func TestIsColor(t *testing.T) {
	good := []string{
		"#fff", "#FFFF", "#4a86ff", "#4a86ffcc",
		"rgb(255, 0, 0)", "rgba(255, 255, 255, 0.08)", "hsl(210 40% 50%)",
		"oklch(0.7 0.1 250)", "color-mix(in oklab, #fff 20%, transparent)",
		"transparent", "TRANSPARENT",
	}
	for _, value := range good {
		if !IsColor(value) {
			t.Errorf("IsColor(%q) = false, want true", value)
		}
	}

	// The refusals are the point: a token's value is written straight into a
	// style declaration, so anything that could end that declaration and start
	// something else has to be turned away.
	bad := []string{
		"", "red", "#ff", "#1234567",
		"url(evil.png)", "var(--bg)", "expression(alert(1))",
		"rgb(0,0,0); background: url(x)", "#fff; }", "rgb(0,0,0)}", `"#fff"`,
	}
	for _, value := range bad {
		if IsColor(value) {
			t.Errorf("IsColor(%q) = true, want false", value)
		}
	}
}

func TestValidateRefusesBadThemes(t *testing.T) {
	cases := map[string]Theme{
		"no id":         {Name: "x", Base: BaseDark},
		"id with space": {ID: "my theme", Name: "x"},
		"id in caps":    {ID: "MyTheme", Name: "x"},
		"unknown base":  {ID: "x", Name: "x", Base: "sepia"},
		"unknown token": {ID: "x", Name: "x", Tokens: map[string]string{"backgorund": "#fff"}},
		"bad colour":    {ID: "x", Name: "x", Tokens: map[string]string{"bg": "url(x)"}},
	}
	for name, theme := range cases {
		if _, err := validate(theme); err == nil {
			t.Errorf("%s: validate accepted it", name)
		}
	}
}

func TestValidateFillsInWhatItCan(t *testing.T) {
	// No base and no name: both have a defensible answer, and refusing a theme
	// over either would be pedantry.
	theme, err := validate(Theme{ID: "minimal", Tokens: map[string]string{"--bg": " #101010 "}})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if theme.Base != BaseDark {
		t.Errorf("base is %q, want %q", theme.Base, BaseDark)
	}
	if theme.Name != "minimal" {
		t.Errorf("name is %q, want the id", theme.Name)
	}
	// The `--` prefix is how the token appears in CSS, so a theme author is
	// likely to write it; it is stripped rather than refused.
	if theme.Tokens["bg"] != "#101010" {
		t.Errorf("bg is %q, want the trimmed, unprefixed value", theme.Tokens["bg"])
	}
}

func TestLoadReadsThemesAndPacks(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "solo.json", `{"id": "solo", "name": "Solo", "base": "dark", "tokens": {"accent": "#ff0000"}}`)
	write(t, dir, "pack.json", `{
		"name": "Harbour Pack",
		"author": "someone",
		"themes": [
			{"id": "one", "name": "One", "base": "dark"},
			{"id": "two", "name": "Two", "base": "light"}
		]
	}`)
	// A pack cloned into a folder of its own, which is the other way one arrives.
	write(t, filepath.Join(dir, "cloned"), "theme.json", `{"id": "three", "name": "Three"}`)
	// Neither of these is a theme file and neither should be reported as one.
	write(t, dir, ".hidden.json", `nonsense`)
	write(t, dir, "notes.txt", `nonsense`)

	cat := Load(dir, nil)
	if len(cat.Problems) > 0 {
		t.Fatalf("problems: %v", cat.Problems)
	}
	for _, id := range []string{"solo", "one", "two", "three"} {
		if _, ok := cat.Find(id); !ok {
			t.Errorf("theme %q did not load; got %v", id, ids(cat.Themes))
		}
	}
	if theme, _ := cat.Find("one"); theme.Pack != "Harbour Pack" {
		t.Errorf("pack name is %q, want it carried onto the theme for grouping", theme.Pack)
	}
	if theme, _ := cat.Find("solo"); theme.Origin != filepath.Join(dir, "solo.json") {
		t.Errorf("origin is %q, want the file it came from", theme.Origin)
	}
	if !slices.Contains(ids(cat.Themes), DefaultID) {
		t.Error("loading a folder dropped the builtin themes")
	}
}

func TestLoadReportsBadFilesWithoutLosingGoodOnes(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "broken.json", `{ this is not json`)
	write(t, dir, "wrong.json", `{"id": "wrong", "tokens": {"bg": "url(x)"}}`)
	write(t, dir, "fine.json", `{"id": "fine", "name": "Fine"}`)

	cat := Load(dir, nil)
	if _, ok := cat.Find("fine"); !ok {
		t.Error("a broken file alongside a good one cost us the good one")
	}
	if len(cat.Problems) != 2 {
		t.Fatalf("got %d problems, want one per bad file: %v", len(cat.Problems), cat.Problems)
	}
	for _, problem := range cat.Problems {
		if problem.Path == "" || problem.Message == "" {
			t.Errorf("problem %+v does not say what or why", problem)
		}
	}
}

func TestLoadLetsAUserThemeReplaceABuiltin(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "mine.json", `{"id": "nord", "name": "My Nord", "base": "dark"}`)

	cat := Load(dir, nil)
	theme, ok := cat.Find("nord")
	if !ok {
		t.Fatal("nord is missing entirely")
	}
	if theme.Name != "My Nord" {
		t.Errorf("name is %q, want the user's own -- overriding a builtin by id is how you retheme it", theme.Name)
	}
	if got := slices.Contains(ids(cat.Themes), "nord"); !got {
		t.Error("nord is not in the catalogue")
	}
	// Replaced in place rather than appended, so the gallery does not show two.
	count := 0
	for _, id := range ids(cat.Themes) {
		if id == "nord" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("nord appears %d times, want 1", count)
	}
}

func TestLoadRefusesASecondThemeWithTheSameID(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	write(t, first, "a.json", `{"id": "clash", "name": "First"}`)
	write(t, second, "b.json", `{"id": "clash", "name": "Second"}`)

	cat := Load(first, []string{second})
	theme, _ := cat.Find("clash")
	if theme.Name != "First" {
		t.Errorf("name is %q, want the default folder's -- it is searched first", theme.Name)
	}
	if len(cat.Problems) != 1 {
		t.Fatalf("got %d problems, want the clash named: %v", len(cat.Problems), cat.Problems)
	}
	if !strings.Contains(cat.Problems[0].Message, "clash") {
		t.Errorf("problem %q does not name the id", cat.Problems[0].Message)
	}
}

// A themes folder that was never created is the normal state of a fresh
// install, not something to report.
func TestLoadIsQuietAboutAMissingFolder(t *testing.T) {
	cat := Load(filepath.Join(t.TempDir(), "never-created"), nil)
	if len(cat.Problems) != 0 {
		t.Errorf("problems: %v", cat.Problems)
	}
	if len(cat.Themes) != len(Builtin()) {
		t.Errorf("got %d themes, want the builtins", len(cat.Themes))
	}
}

func TestLoadWarnsAboutUnreadableThemes(t *testing.T) {
	dir := t.TempDir()
	// Text one shade off the background: it loads, because somebody's palette
	// is their business, but they should be told.
	write(t, dir, "murky.json", `{
		"id": "murky", "name": "Murky", "base": "dark",
		"tokens": {"bg": "#101010", "bg-sidebar": "#101010", "bg-panel": "#101010",
		           "bg-raised": "#101010", "text": "#151515", "text-dim": "#151515",
		           "text-faint": "#151515"}
	}`)

	cat := Load(dir, nil)
	theme, ok := cat.Find("murky")
	if !ok {
		t.Fatal("an unreadable theme should still load")
	}
	if len(theme.Warnings) == 0 {
		t.Error("no warnings on a theme whose text is invisible")
	}
}

func TestExampleIsAThemeThatLoads(t *testing.T) {
	dir := t.TempDir()
	path, err := WriteExample(dir)
	if err != nil {
		t.Fatalf("WriteExample: %v", err)
	}

	var theme Theme
	raw, err := os.ReadFile(path) // #nosec G304 -- a path this test just wrote
	if err != nil {
		t.Fatalf("reading it back: %v", err)
	}
	if err := json.Unmarshal(raw, &theme); err != nil {
		t.Fatalf("the starter theme is not valid JSON: %v", err)
	}
	if _, err := validate(theme); err != nil {
		t.Fatalf("the starter theme does not validate: %v", err)
	}

	// A second call must not overwrite the first, which may have been edited.
	again, err := WriteExample(dir)
	if err != nil {
		t.Fatalf("second WriteExample: %v", err)
	}
	if again == path {
		t.Errorf("both starters went to %s", path)
	}
}

// The frontend needs the default theme's id before it has asked the Go side for
// anything -- it is the fallback when a settings file names a theme that is not
// installed -- so it carries its own copy of the constant. This is the guard
// that the copy stays a copy.
func TestFrontendKnowsTheSameDefault(t *testing.T) {
	const file = "../../frontend/src/lib/theme/apply.ts"
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Skipf("frontend not present: %v", err)
	}
	want := "export const DEFAULT_THEME_ID = '" + DefaultID + "';"
	if !strings.Contains(string(raw), want) {
		t.Errorf("%s does not contain %q -- the two copies of the default theme id have drifted", file, want)
	}
}

// A pack is several themes in one file, so one bad theme inside it must not
// cost the user the rest -- nor go unmentioned.
func TestABadThemeInAPackDoesNotHideTheGoodOnes(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "pack.json", `{
		"name": "Mixed",
		"themes": [
			{"id": "good", "name": "Good"},
			{"id": "Bad Id", "name": "Bad"},
			{"id": "also-good", "name": "Also Good"}
		]
	}`)

	cat := Load(dir, nil)
	for _, id := range []string{"good", "also-good"} {
		if _, ok := cat.Find(id); !ok {
			t.Errorf("theme %q was lost to a bad sibling", id)
		}
	}
	if len(cat.Problems) != 1 {
		t.Fatalf("got %d problems, want the bad theme named: %v", len(cat.Problems), cat.Problems)
	}
	if !strings.Contains(cat.Problems[0].Message, "Bad Id") {
		t.Errorf("problem %q does not say which theme was refused", cat.Problems[0].Message)
	}
}

// The starter theme spells out every colour, which is what makes it editable a
// line at a time rather than a prompt to go and read the documentation.
func TestExampleSetsEveryToken(t *testing.T) {
	var theme Theme
	if err := json.Unmarshal(Example(), &theme); err != nil {
		t.Fatalf("the starter theme is not valid JSON: %v", err)
	}
	for _, token := range TokenNames() {
		if theme.Tokens[token] == "" {
			t.Errorf("the starter theme leaves %q out", token)
		}
	}
}
