package updates

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
)

// releaseWithAssets answers like GitHub for a release that carries the files
// the release workflow uploads, plus one that is not ours.
func releaseWithAssets(tag string, names ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		assets := ""
		for i, name := range names {
			if i > 0 {
				assets += ","
			}
			assets += fmt.Sprintf(`{"name":%q,"browser_download_url":"https://example.com/elsewhere/%s"}`, name, name)
		}
		_, _ = fmt.Fprintf(w, `{"tag_name":%q,"name":%q,"published_at":"2026-09-05T16:02:10Z","assets":[%s]}`, tag, tag, assets)
	}
}

func TestLatestKeepsTheNamesOfTheReleaseFiles(t *testing.T) {
	c := github(t, releaseWithAssets("v1.2.0",
		"k8sdockside-1.2.0-linux-amd64.AppImage",
		"checksums.txt",
		"../escape",
		"k8sdockside-1.2.0-darwin-arm64.dmg",
	))

	got, err := c.Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	want := []string{"k8sdockside-1.2.0-linux-amd64.AppImage", "k8sdockside-1.2.0-darwin-arm64.dmg"}
	if fmt.Sprint(got.Assets) != fmt.Sprint(want) {
		t.Errorf("assets = %q, want only the app's own files: %q", got.Assets, want)
	}
}

func TestAssetNameFollowsTheReleaseWorkflow(t *testing.T) {
	cases := []struct {
		install Install
		want    string
	}{
		{Install{"linux", "amd64", FormatAppImage}, "k8sdockside-1.2.0-linux-amd64.AppImage"},
		{Install{"linux", "arm64", FormatDeb}, "k8sdockside-1.2.0-linux-arm64.deb"},
		{Install{"linux", "amd64", FormatRPM}, "k8sdockside-1.2.0-linux-amd64.rpm"},
		{Install{"linux", "amd64", FormatArch}, "k8sdockside-1.2.0-linux-amd64.pkg.tar.zst"},
		{Install{"linux", "amd64", FormatTarball}, "k8sdockside-1.2.0-linux-amd64.tar.gz"},
		{Install{"darwin", "arm64", FormatDMG}, "k8sdockside-1.2.0-darwin-arm64.dmg"},
		{Install{"windows", "amd64", FormatInstaller}, "k8sdockside-1.2.0-windows-amd64-installer.exe"},
		{Install{"windows", "amd64", FormatZip}, "k8sdockside-1.2.0-windows-amd64.zip"},
		{Install{"linux", "amd64", ""}, ""},
		{Install{"freebsd", "amd64", FormatTarball}, ""},
	}
	for _, tc := range cases {
		if got := tc.install.AssetName("v1.2.0"); got != tc.want {
			t.Errorf("%+v.AssetName(v1.2.0) = %q, want %q", tc.install, got, tc.want)
		}
	}
	// A pre-release keeps its full tag on the file, without the v.
	if got := (Install{"linux", "amd64", FormatDeb}).AssetName("v1.3.0-rc.1"); got != "k8sdockside-1.3.0-rc.1-linux-amd64.deb" {
		t.Errorf("pre-release asset = %q", got)
	}
}

func TestDownloadIsTheReleaseFileForThisInstall(t *testing.T) {
	r := Release{
		Version: "v1.2.0",
		Assets:  []string{"k8sdockside-1.2.0-linux-amd64.deb", "k8sdockside-1.2.0-darwin-arm64.dmg"},
	}
	want := ReleasesPage + "/download/v1.2.0/k8sdockside-1.2.0-linux-amd64.deb"
	if got := r.Download(Install{"linux", "amd64", FormatDeb}); got != want {
		t.Errorf("download = %q, want %q", got, want)
	}
	// Nothing published for this install: no link, rather than a guess.
	if got := r.Download(Install{"linux", "amd64", FormatRPM}); got != "" {
		t.Errorf("download for an absent file = %q, want none", got)
	}
}

// files is a fake filesystem for detect: the paths that exist.
func files(paths ...string) func(string) []string {
	return func(pattern string) []string {
		var out []string
		for _, p := range paths {
			if ok, _ := filepath.Match(pattern, p); ok {
				out = append(out, p)
			}
		}
		return out
	}
}

func TestDetectTellsAnAppImageByItsEnvironment(t *testing.T) {
	got := detect(probe{
		os: "linux", arch: "amd64",
		getenv: func(k string) string {
			if k == "APPIMAGE" {
				return "/home/me/Apps/k8sdockside.AppImage"
			}
			return ""
		},
		exe:  "/tmp/.mount_k8sdocABC/usr/bin/k8sdockside",
		glob: files(),
	})
	if got != (Install{"linux", "amd64", FormatAppImage}) {
		t.Errorf("detect = %+v", got)
	}
}

func TestDetectTellsPackagesByTheirDatabases(t *testing.T) {
	none := func(string) string { return "" }
	cases := []struct {
		name string
		exe  string
		have []string
		want string
	}{
		{"deb", "/usr/local/bin/k8sdockside", []string{"/var/lib/dpkg/info/k8sdockside.list"}, FormatDeb},
		{"arch", "/usr/local/bin/k8sdockside", []string{"/var/lib/pacman/local/k8sdockside-1.2.0-1/desc"}, FormatArch},
		{"rpm", "/usr/local/bin/k8sdockside", []string{"/usr/lib/sysimage/rpm/rpmdb.sqlite"}, FormatRPM},
		// An rpm database on the machine says nothing about a binary that was
		// not installed by it.
		{"tarball beside rpm", "/home/me/bin/k8sdockside", []string{"/var/lib/rpm/rpmdb.sqlite"}, FormatTarball},
		{"tarball", "/opt/k8sdockside/k8sdockside", nil, FormatTarball},
	}
	for _, tc := range cases {
		got := detect(probe{os: "linux", arch: "amd64", getenv: none, exe: tc.exe, glob: files(tc.have...)})
		if got.Format != tc.want {
			t.Errorf("%s: format = %q, want %q", tc.name, got.Format, tc.want)
		}
	}
}

func TestDetectOnMacIsTheDiskImage(t *testing.T) {
	got := detect(probe{os: "darwin", arch: "arm64", getenv: func(string) string { return "" },
		exe: "/Applications/k8sdockside.app/Contents/MacOS/k8sdockside", glob: files()})
	if got != (Install{"darwin", "arm64", FormatDMG}) {
		t.Errorf("detect = %+v", got)
	}
}

func TestDetectOnWindowsLooksForTheUninstaller(t *testing.T) {
	none := func(string) string { return "" }
	// Forward slashes, which Windows accepts and which let this run on the
	// other platforms, where a backslash is an ordinary character.
	exe := "C:/Program Files/k8sdockside/k8sdockside.exe"
	got := detect(probe{os: "windows", arch: "amd64", getenv: none, exe: exe,
		glob: files("C:/Program Files/k8sdockside/uninstall.exe")})
	if got.Format != FormatInstaller {
		t.Errorf("with uninstall.exe beside it: format = %q, want installer", got.Format)
	}
	got = detect(probe{os: "windows", arch: "amd64", getenv: none, exe: "C:/Tools/k8sdockside.exe", glob: files()})
	if got.Format != FormatZip {
		t.Errorf("on its own: format = %q, want zip", got.Format)
	}
}

func TestInstallDescribesItselfForTheAboutPage(t *testing.T) {
	cases := map[Install]string{
		{"linux", "amd64", FormatAppImage}:    "Linux AppImage, amd64",
		{"linux", "arm64", FormatDeb}:         "Debian package, arm64",
		{"linux", "amd64", FormatRPM}:         "RPM package, amd64",
		{"linux", "amd64", FormatArch}:        "Arch package, amd64",
		{"linux", "amd64", FormatTarball}:     "Linux tarball, amd64",
		{"darwin", "arm64", FormatDMG}:        "macOS, arm64",
		{"windows", "amd64", FormatInstaller}: "Windows installer, amd64",
		{"windows", "amd64", FormatZip}:       "Windows portable, amd64",
		{"linux", "amd64", ""}:                "linux/amd64",
	}
	for install, want := range cases {
		if got := install.String(); got != want {
			t.Errorf("%+v = %q, want %q", install, got, want)
		}
	}
}
