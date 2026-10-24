#!/bin/sh
# Puts herdr-auto-title in the plugin root: the release binary when this
# checkout is that release's commit, else a Go build;
# docs/architecture/installation.md says why.
set -u

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd) || exit 1
cd "$root" || exit 1

name=herdr-auto-title
releases=${HERDR_AUTO_TITLE_RELEASES:-https://github.com/kryptamine/herdr-auto-title/releases}
stable="herdr plugin install kryptamine/herdr-auto-title --ref stable"
released=
hinted=
go_missing=

say() { printf '%s: %s\n' "$name" "$*" >&2; }

version=$(sed -n 's/^version *= *"\([^"]*\)".*/\1/p' herdr-plugin.toml | head -n 1)
head=$(git rev-parse HEAD 2>/dev/null) || head=

tmp=$(mktemp -d 2>/dev/null || mktemp -d -t "$name") || exit 1
trap 'rm -rf "$tmp"' EXIT

asset_name() {
	case $(uname -s) in
	Linux) os=linux ;;
	Darwin) os=macos ;;
	*) return 1 ;;
	esac
	arch=$(uname -m)
	# A shell under Rosetta reports x86_64 on Apple silicon.
	if [ "$os" = macos ] && [ "$(sysctl -n hw.optional.arm64 2>/dev/null)" = 1 ]; then
		arch=arm64
	fi
	case $arch in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*) return 1 ;;
	esac
	printf '%s-%s-%s\n' "$name" "$os" "$arch"
}

fetch() { # url file
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL --retry 2 --connect-timeout 15 --max-time 300 -o "$2" "$1"
	else
		wget -q -T 30 --tries=2 -O "$2" "$1"
	fi
}

sha256() { # file
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{ print $1 }'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{ print $1 }'
	else
		say "neither sha256sum nor shasum is installed"
		return 1
	fi
}

# place file: moves a verified download into the plugin root once it reports
# the manifest's version. It runs from the root because $TMPDIR may be noexec.
place() {
	staged=".$name.$$"
	if ! cp "$1" "$staged" || ! chmod 755 "$staged"; then
		say "the downloaded binary could not be copied into the plugin root"
		rm -f "$staged"
		return 1
	fi
	if ! reported=$("./$staged" --version 2>/dev/null); then
		say "the downloaded binary does not run on this machine"
		rm -f "$staged"
		return 1
	fi
	if [ "$reported" != "$name $version" ]; then
		say "the downloaded binary reports '$reported', not $version"
		rm -f "$staged"
		return 1
	fi
	mv -f "$staged" "$name" || {
		rm -f "$staged"
		return 1
	}
}

download() { # url
	if ! fetch "$1/$asset" "$tmp/$asset" || ! fetch "$1/SHA256SUMS" "$tmp/SHA256SUMS"; then
		say "$asset could not be downloaded"
		return 1
	fi
	expected=$(awk -v asset="$asset" '$2 == asset || $2 == "*" asset { print $1; exit }' "$tmp/SHA256SUMS")
	actual=$(sha256 "$tmp/$asset") || actual=
	if [ -z "$expected" ] || [ "$expected" != "$actual" ]; then
		say "$asset does not match its checksum in SHA256SUMS"
		return 1
	fi
	place "$tmp/$asset"
}

# release_commit url: the commit the release was cut from. Herdr's checkout
# keeps .git but no tags, so that commit is all it can be matched by.
release_commit() {
	fetch "$1/COMMIT" "$tmp/COMMIT" 2>/dev/null && tr -d ' \r\n' <"$tmp/COMMIT"
}

build() {
	if ! command -v go >/dev/null 2>&1; then
		go_missing=1
		say "Go is not installed, so it cannot be built from source"
		return 1
	fi
	say "building from source with $(go version)"
	go build -o "$name" ./cmd/herdr-auto-title && return 0
	say "it could not be built from source, which needs Go 1.24 or newer"
	return 1
}

asset=$(asset_name) || {
	asset=
	say "no release binary is published for $(uname -s) $(uname -m)"
}
if [ -n "$asset" ] && ! command -v curl >/dev/null 2>&1 && ! command -v wget >/dev/null 2>&1; then
	asset=
	say "neither curl nor wget is installed, so no release binary can be downloaded"
fi

if [ -n "$asset" ] && [ -n "$version" ]; then
	base="$releases/download/v$version"
	if ! commit=$(release_commit "$base"); then
		hinted=1
		say "release $version could not be fetched, so it is built from source;"
		say "for the latest release instead: $stable"
	elif [ "$commit" != "$head" ]; then
		hinted=1
		say "this checkout is not release $version, so it is built from source;"
		say "for the latest release instead: $stable"
	else
		released=1
		say "downloading $asset $version"
		if download "$base"; then
			say "installed release $version"
			exit 0
		fi
		say "release $version could not be installed"
	fi
fi

if build; then
	exit 0
fi

if [ -n "$go_missing" ]; then
	if [ -n "$released" ]; then
		say "could not install $version: its release binary could not be installed,"
		say "and building it from source needs Go 1.24 or newer"
	elif [ -n "$hinted" ]; then
		say "could not install: building from source needs Go 1.24 or newer;"
		say "to install the latest release without Go: $stable"
	else
		say "could not install: building from source needs Go 1.24 or newer"
	fi
else
	say "could not install: the source build above failed"
fi
exit 1
