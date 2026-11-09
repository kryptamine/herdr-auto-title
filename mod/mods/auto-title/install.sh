#!/bin/sh
# Own the whole install even after the popup closes; report through $2/status.

set -u

version=$1
dir=$2
if command -v cygpath >/dev/null 2>&1; then dir=$(cygpath -m "$dir"); fi

close() {
	# The popup that would remove $2 may have closed, so drop the download here.
	rm -f "$dir/${asset:-none}" "$dir/SHA256SUMS"
	hc kill "${HC_TERMINAL_ID:-}" >/dev/null 2>&1
	exit "$1"
}

fail() {
	rm -f "$dir/${asset:-none}" "$dir/SHA256SUMS"
	printf '1\n%s\n' "$1" >"$dir/status"
	printf '\n%s\nPress Enter to close.' "$1"
	read -r _
	close 1
}

# A closed tab or Ctrl+C still answers the mod, which would otherwise wait.
trap 'rm -f "$dir/${asset:-none}" "$dir/SHA256SUMS"; printf "1\nThe install was stopped.\n" >"$dir/status"; exit 1' HUP INT TERM

system=$(uname -s)
suffix=
case "$system" in
Darwin) os=macos ;;
Linux) os=linux ;;
MINGW*_NT-* | MSYS_NT-*) os=windows; suffix=.exe ;;
*) fail "No release binary for $system." ;;
esac

arch=$(uname -m)
if [ "$os" = windows ]; then
	arch=${PROCESSOR_ARCHITEW6432:-${PROCESSOR_ARCHITECTURE:-$arch}}
	# Git Bash under x64 emulation reports the native CPU in this suffix.
	case $system in *-ARM64) arch=arm64 ;; esac
fi
if [ "$os" = macos ] && [ "$(sysctl -n hw.optional.arm64 2>/dev/null)" = 1 ]; then arch=arm64; fi
case "$arch" in
x86_64 | amd64 | AMD64) arch=amd64 ;;
arm64 | aarch64 | ARM64) arch=arm64 ;;
*) fail "No release binary for $arch." ;;
esac

asset="herdr-auto-title-$os-$arch$suffix"
releases=${HERDR_AUTO_TITLE_RELEASES:-https://github.com/kryptamine/herdr-auto-title/releases}
url="$releases/download/v$version"

echo "Downloading Auto Title $version"

if ! curl -fL --progress-bar -o "$dir/$asset" "$url/$asset" ||
	! curl -fsSL -o "$dir/SHA256SUMS" "$url/SHA256SUMS"; then
	fail "The download of $version failed."
fi

want=$(awk -v asset="$asset" '$2 == asset || $2 == "*" asset { print $1; exit }' "$dir/SHA256SUMS")

if command -v sha256sum >/dev/null 2>&1; then
	have=$(sha256sum "$dir/$asset" | cut -d' ' -f1)
else
	have=$(shasum -a 256 "$dir/$asset" | cut -d' ' -f1)
fi

if [ -z "$want" ] || [ "$want" != "$have" ]; then
	fail "$asset does not match SHA256SUMS."
fi

chmod 755 "$dir/$asset"

if ! "$dir/$asset" desktop install; then
	fail "desktop install failed."
fi

rm -f "$dir/$asset" "$dir/SHA256SUMS"
printf '0\n' >"$dir/status"
close 0
