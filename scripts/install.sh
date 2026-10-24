#!/bin/sh
# Puts herdr-auto-title in the plugin root: this version's release binary,
# else a Go build, else the latest published release. Herdr runs it on install;
# docs/architecture/installation.md says why in that order.
set -u

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd) || exit 1
cd "$root" || exit 1

name=herdr-auto-title
releases=${HERDR_AUTO_TITLE_RELEASES:-https://github.com/kryptamine/herdr-auto-title/releases}
installed=
go_missing=

say() { printf '%s: %s\n' "$name" "$*" >&2; }

version=$(sed -n 's/^version *= *"\([^"]*\)".*/\1/p' herdr-plugin.toml | head -n 1)

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
	elif command -v wget >/dev/null 2>&1; then
		wget -q -T 30 --tries=2 -O "$2" "$1"
	else
		say "neither curl nor wget is installed"
		return 1
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

# place file: moves a verified download into the plugin root once it runs
# there. Its checksum already says which release it is; running it says it
# works on this machine, from the root because $TMPDIR may be mounted noexec.
place() {
	staged=".$name.$$"
	if ! cp "$1" "$staged" || ! chmod 755 "$staged"; then
		rm -f "$staged"
		return 1
	fi
	if ! reported=$("./$staged" --version 2>/dev/null); then
		say "the downloaded binary does not run on this machine"
		rm -f "$staged"
		return 1
	fi
	mv -f "$staged" "$name" || {
		rm -f "$staged"
		return 1
	}
	installed=${reported#"$name "}
}

download() { # url
	tmp=$(mktemp -d 2>/dev/null || mktemp -d -t "$name") || return 1
	ok=1
	if fetch "$1/$asset" "$tmp/$asset" && fetch "$1/SHA256SUMS" "$tmp/SHA256SUMS"; then
		expected=$(awk -v asset="$asset" '$2 == asset || $2 == "*" asset { print $1; exit }' "$tmp/SHA256SUMS")
		actual=$(sha256 "$tmp/$asset") || actual=
		if [ -z "$expected" ] || [ "$expected" != "$actual" ]; then
			say "$asset does not match its checksum in SHA256SUMS"
		elif place "$tmp/$asset"; then
			ok=0
		fi
	fi
	rm -rf "$tmp"
	return $ok
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

if [ "${HERDR_AUTO_TITLE_BUILD:-}" = source ]; then
	build
	exit
fi

asset=$(asset_name) || {
	asset=
	say "no release binary is published for $(uname -s) $(uname -m)"
}

if [ -n "$asset" ] && [ -n "$version" ]; then
	say "downloading $asset $version"
	if download "$releases/download/v$version"; then
		say "installed release $installed"
		exit 0
	fi
	say "release $version could not be installed"
fi

if build; then
	exit 0
fi

# A release in the minutes before its binaries are attached is still a draft,
# and latest/download then serves the release before it.
if [ -n "$asset" ] && download "$releases/latest/download"; then
	say "installed release $installed, the latest published one, in place of $version;"
	say "if $version was released minutes ago, install again shortly to get it"
	exit 0
fi

say "could not install $version: no release binary could be downloaded, and"
if [ -n "$go_missing" ]; then
	say "building one from source needs Go 1.24 or newer, which is not installed"
else
	say "the source build above failed"
fi
exit 1
