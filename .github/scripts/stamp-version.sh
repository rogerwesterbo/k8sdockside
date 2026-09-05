#!/usr/bin/env bash
#
# Stamps a release version into the packaging metadata that Wails reads while
# building. These files carry a hardcoded 0.0.1 in the repository; the release
# workflow rewrites them in its own checkout and never commits the result.
#
# Two versions are used, because the fields want different things:
#
#   VERSION   the tag as written, e.g. 1.4.0 or 1.4.0-rc.1. Goes where a
#             free-form string is allowed (deb/rpm versions, artifact names).
#   NUMERIC   the same with any pre-release suffix removed, e.g. 1.4.0. Goes
#             where the format is fixed: NSIS builds VIProductVersion as
#             "${INFO_PRODUCTVERSION}.0" and refuses anything but four numbers,
#             and CFBundleVersion is dotted integers too.
#
# perl rather than sed, because sed -i differs between GNU and BSD and this
# runs on Linux, macOS and Windows runners alike.

set -euo pipefail

VERSION="${1:?usage: stamp-version.sh <version> [numeric-version]}"
NUMERIC="${2:-${VERSION%%-*}}"

cd "$(dirname "$0")/../.."

echo "Stamping version=${VERSION} numeric=${NUMERIC}"

# build/config.yml -- the source `wails3 update build-assets` regenerates from.
# Anchored to exactly two spaces of indent so it cannot hit the file's own
# top-level `version: '3'` (the Taskfile schema version) or the commented-out
# iOS block.
perl -pi -e "s/^(  version: \")[^\"]*\"/\${1}${NUMERIC}\"/" build/config.yml

# Windows: the .exe resource block, and the NSIS installer's fallback define.
# wails_tools.nsh guards it with !ifndef, so this is the value the installer
# gets unless makensis is given -D on the command line.
perl -pi -e "s/(\"file_version\": \")[^\"]*\"/\${1}${NUMERIC}\"/" build/windows/info.json
perl -pi -e "s/(\"ProductVersion\": \")[^\"]*\"/\${1}${NUMERIC}\"/" build/windows/info.json
perl -pi -e "s/(!define INFO_PRODUCTVERSION \")[^\"]*\"/\${1}${NUMERIC}\"/" build/windows/nsis/wails_tools.nsh

# macOS: the two version keys in the bundle's Info.plist. Slurp mode, because
# the value sits on the line after the key it belongs to.
perl -0pi -e "s{(<key>CFBundleShortVersionString</key>\s*<string>)[^<]*}{\${1}${NUMERIC}}s" build/darwin/Info.plist
perl -0pi -e "s{(<key>CFBundleVersion</key>\s*<string>)[^<]*}{\${1}${NUMERIC}}s" build/darwin/Info.plist

# Linux: nfpm builds the .deb, .rpm and Arch packages from this. Full version
# here -- dpkg and rpm both take a pre-release suffix.
perl -pi -e "s/^(version: \")[^\"]*\"/\${1}${VERSION}\"/" build/linux/nfpm/nfpm.yaml

# The one the running app shows: the About dialog under the app menu, and the
# About section in settings. Stamped into the source rather than injected with
# -ldflags because the Wails build tasks hardcode their own -ldflags and give
# no way to add -X to them; a CLI override of BUILD_FLAGS is ignored, because
# Task's task-level vars outrank it. Full version, so a pre-release reads as
# one in the UI.
perl -pi -e "s/^(var version = \")[^\"]*\"/\${1}${VERSION}\"/" settingsservice.go

echo "--- stamped ---"
grep -n '^  version:' build/config.yml
grep -n 'file_version\|ProductVersion' build/windows/info.json
grep -n 'INFO_PRODUCTVERSION "' build/windows/nsis/wails_tools.nsh
grep -n -A1 'CFBundleShortVersionString\|CFBundleVersion' build/darwin/Info.plist
grep -n '^version:' build/linux/nfpm/nfpm.yaml
grep -n '^var version' settingsservice.go
