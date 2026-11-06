#!/bin/sh
# Installs the release named in $1 from a tab of its own, since a mod's exec
# gives up after 30 s. Writes "0", or "1" and why, to $2/status for the mod,
# then closes the tab: Herdr would otherwise leave a shell in it.

set -u

version=$1
dir=$2

close() {
	# The popup that would remove $2 may have closed, so drop the download here.
	rm -f "$dir/${asset:-none}" "$dir/SHA256SUMS"
	hc kill "${HC_TERMINAL_ID:-}" >/dev/null 2>&1
	exit "$1"
}

fail() {
	printf '1\n%s\n' "$1" >"$dir/status"
	printf '\n%s\nPress Enter to close.' "$1"
	read -r _
	close 1
}

# A closed tab or Ctrl+C still answers the mod, which would otherwise wait.
trap 'printf "1\nThe install was stopped.\n" >"$dir/status"; rm -f "$dir/${asset:-none}" "$dir/SHA256SUMS"; exit 1' HUP INT TERM

case "$(uname -s)" in
Darwin) os=macos ;;
Linux) os=linux ;;
*) fail "No release binary for $(uname -s)." ;;
esac

case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) fail "No release binary for $(uname -m)." ;;
esac

asset="herdr-auto-title-$os-$arch"
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

printf '0\n' >"$dir/status"
close 0
