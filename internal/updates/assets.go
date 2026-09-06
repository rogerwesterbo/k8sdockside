package updates

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// How this build got onto the machine, as far as can be told from where it is
// running. Each is one of the files the release workflow uploads, so knowing
// the format is knowing which file to point the user at.
const (
	FormatAppImage  = "appimage"
	FormatDeb       = "deb"
	FormatRPM       = "rpm"
	FormatArch      = "arch"
	FormatTarball   = "tarball"
	FormatDMG       = "dmg"
	FormatInstaller = "installer"
	FormatZip       = "zip"
)

// Install is what this build is: the platform it was built for and the
// package it arrived in. Format is empty when nothing could be told, which is
// not an error -- the release page is always there to fall back on.
type Install struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	Format string `json:"format"`
}

// suffixes is what each format's file is called after the common prefix, as
// the release workflow writes them.
var suffixes = map[string]string{
	FormatAppImage:  ".AppImage",
	FormatDeb:       ".deb",
	FormatRPM:       ".rpm",
	FormatArch:      ".pkg.tar.zst",
	FormatTarball:   ".tar.gz",
	FormatDMG:       ".dmg",
	FormatInstaller: "-installer.exe",
	FormatZip:       ".zip",
}

// platforms is which formats ship for which OS, so a format cannot be asked
// for on a platform it is never built for.
var platforms = map[string][]string{
	"linux":   {FormatAppImage, FormatDeb, FormatRPM, FormatArch, FormatTarball},
	"darwin":  {FormatDMG},
	"windows": {FormatInstaller, FormatZip},
}

// AssetName is the file the release workflow would have uploaded for this
// install of the given release, or empty when the format is unknown or not
// one that platform ships. It follows the workflow exactly: the tag without
// its v, then the platform, then the format's suffix.
func (i Install) AssetName(tag string) string {
	suffix, ok := suffixes[i.Format]
	if !ok || !contains(platforms[i.OS], i.Format) {
		return ""
	}
	version := strings.TrimPrefix(strings.TrimSpace(tag), "v")
	return "k8sdockside-" + version + "-" + i.OS + "-" + i.Arch + suffix
}

// String is the install as the About page and the bell describe it.
func (i Install) String() string {
	names := map[string]string{
		FormatAppImage:  "Linux AppImage",
		FormatDeb:       "Debian package",
		FormatRPM:       "RPM package",
		FormatArch:      "Arch package",
		FormatTarball:   "Linux tarball",
		FormatDMG:       "macOS",
		FormatInstaller: "Windows installer",
		FormatZip:       "Windows portable",
	}
	if name, ok := names[i.Format]; ok {
		return name + ", " + i.Arch
	}
	return i.OS + "/" + i.Arch
}

// Download is the address of this release's file for the given install, or
// empty when no such file was published. The address is built from the tag
// and the file's name rather than taken from the response, for the reason the
// release page's is: the browser goes only to this repository's downloads.
func (r Release) Download(i Install) string {
	name := i.AssetName(r.Version)
	if name == "" || !contains(r.Assets, name) {
		return ""
	}
	return ReleasesPage + "/download/" + url.PathEscape(r.Version) + "/" + url.PathEscape(name)
}

// ownAsset reports whether a file name from the release is one of the app's
// own downloads: something the workflow named, with nothing in it that could
// be read as a path.
func ownAsset(name string) bool {
	return strings.HasPrefix(name, "k8sdockside-") && !strings.ContainsAny(name, `/\`) && name == filepath.Base(name)
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

// ---- detection --------------------------------------------------------------

// probe is what detect looks at, gathered here so a test can describe any
// machine without being on it.
type probe struct {
	os, arch string
	getenv   func(string) string
	// exe is where this binary is running from.
	exe string
	// glob answers which paths exist, filepath.Glob in the real thing.
	glob func(pattern string) []string
}

// DetectInstall works out how this build was installed, from where it is and
// what package databases claim it. It reads a handful of paths and never runs
// anything.
func DetectInstall() Install {
	exe, _ := os.Executable()
	return detect(probe{
		os:     runtime.GOOS,
		arch:   runtime.GOARCH,
		getenv: os.Getenv,
		exe:    exe,
		glob: func(pattern string) []string {
			matches, _ := filepath.Glob(pattern)
			return matches
		},
	})
}

// detect is DetectInstall over a probe. The order on Linux matters: an
// AppImage says so in its environment and is certain; a package manager that
// lists the app is next; the rpm database cannot be read without rpm itself,
// so it is trusted only when the binary sits where the package would have put
// it; anything else is a tarball or a build of the user's own, which is the
// same download either way.
func detect(p probe) Install {
	install := Install{OS: p.os, Arch: p.arch}
	exists := func(pattern string) bool { return len(p.glob(pattern)) > 0 }

	switch p.os {
	case "linux":
		switch {
		case p.getenv("APPIMAGE") != "":
			install.Format = FormatAppImage
		case exists("/var/lib/dpkg/info/k8sdockside.list"):
			install.Format = FormatDeb
		case exists("/var/lib/pacman/local/k8sdockside-*/desc"):
			install.Format = FormatArch
		case p.exe == "/usr/local/bin/k8sdockside" &&
			(exists("/var/lib/rpm/*") || exists("/usr/lib/sysimage/rpm/*")):
			install.Format = FormatRPM
		default:
			install.Format = FormatTarball
		}
	case "darwin":
		install.Format = FormatDMG
	case "windows":
		if p.exe != "" && exists(filepath.Join(filepath.Dir(p.exe), "uninstall.exe")) {
			install.Format = FormatInstaller
		} else {
			install.Format = FormatZip
		}
	}
	return install
}
