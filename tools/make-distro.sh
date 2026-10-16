#!/bin/bash
# make-distro.sh -- build the archive an operator unpacks and runs.
#
#   ./tools/make-distro.sh                  -> ~/go-eqoa-server-windows-<date>.zip
#   ./tools/make-distro.sh --linux          -> ~/go-eqoa-server-<date>.tar.gz
#   ./tools/make-distro.sh --out /tmp/x.zip
#
# This existed only as a staging directory and a memory of which files belonged
# in it, and what fell out of that is the reason it is a script now: the archive
# carries no `patches/.version`, so an operator who unpacked it into a new folder
# restarted the patch version below what his consoles had already applied, and
# the patcher — which gates the whole distribution on that one number — fetched
# the manifest and downloaded nothing at all. The binary now enforces a floor
# (versionFloor in internal/network/tcp/patchserver.go), but the packaging rule
# it depends on has to be written down somewhere that runs.
#
# TWO THINGS MUST NEVER SHIP, and both are dotfiles that a `cp -r patches` picks
# up without a word:
#
#   patches/.version   the operator's patch version. Shipping one number to
#                      everybody would push every other operator's consoles
#                      backwards, which is the failure above with more victims.
#   patches/.sdstate   which of THIS server's consoles hold which SD, keyed by
#                      address. Shipping it tells a stranger's server that a
#                      console it has never met is already up to date.
set -euo pipefail

HERE="$(cd "$(dirname "$0")/.." && pwd)"
STAGE_ROOT="${STAGE_ROOT:-$HOME/.eqoa-winbuild/stage}"
DATE="$(date +%Y%m%d)"
GOOS_TARGET=windows
OUT=

while [ $# -gt 0 ]; do
	case "$1" in
	--linux) GOOS_TARGET=linux; shift ;;
	--windows) GOOS_TARGET=windows; shift ;;
	--out) OUT="$2"; shift 2 ;;
	*) echo "unknown option: $1" >&2; exit 1 ;;
	esac
done

if [ "$GOOS_TARGET" = windows ]; then
	BIN=gameserver.exe
	OUT="${OUT:-$HOME/go-eqoa-server-windows-$DATE.zip}"
else
	BIN=gameserver
	OUT="${OUT:-$HOME/go-eqoa-server-$DATE.tar.gz}"
fi

STAGE="$STAGE_ROOT/go-eqoa-server"
rm -rf "$STAGE"
mkdir -p "$STAGE"

# ------------------------------------------------------------------ source --
# The whole server source ships, tests included: an operator debugging his own
# deployment is the person most likely to read them, and they are what says what
# the wire format is meant to be.
for d in cmd internal sql; do
	cp -r "$HERE/$d" "$STAGE/$d"
done
find "$STAGE" -name '*.exe' -o -name '*.log' | xargs -r rm -f

cp "$HERE/go.mod" "$HERE/go.sum" "$HERE/CLAUDE.md" "$HERE/config.yaml.example" "$STAGE/"
cp "$HERE/../wintest/README.md" "$STAGE/README.md"

# The module replace points outside this tree at a sibling checkout. Vendor just
# the packages it actually provides and rewrite the replace to a relative path,
# so `go build` works in the operator's unpacked folder with nothing else on his
# disk.
#
# Only the packages that are imported, and a go.mod written here rather than
# copied: the sibling checkout declares itself as github.com/DabDavis/... and is
# reached through the replace, so copying its go.mod verbatim gives the vendored
# copy a module path the replace does not name.
PKG_SRC="$HERE/../go-eqoa-esf-tools"
mkdir -p "$STAGE/_deps/eqoa-pkg/pkg"
printf 'module github.com/eqoa/pkg\n\ngo 1.24.0\n' > "$STAGE/_deps/eqoa-pkg/go.mod"
cp -r "$PKG_SRC/pkg/buffer" "$STAGE/_deps/eqoa-pkg/pkg/buffer"
sed -i 's|=> \.\./go-eqoa-esf-tools|=> ./_deps/eqoa-pkg|' "$STAGE/go.mod"

# ---------------------------------------------------------------- payloads --
# patches/serve/ WHOLE: it is the manifest, so shipping all of it is shipping
# exactly what this server offers, with no second list here to fall behind the
# folder. Its .delete goes too — those names are part of what is served.
mkdir -p "$STAGE/patches/serve"
cp "$HERE"/patches/serve/* "$STAGE/patches/serve/"
cp "$HERE/patches/serve/.delete" "$STAGE/patches/serve/.delete" 2>/dev/null || true

# The alternates config.yaml.example names, from patches/ itself. NOT the whole
# directory: it also holds bisect controls and retired builds, and "serve the
# older SD to fix a problem" is advice that takes the hard-disk worlds away with
# no error anywhere.
for f in SD-hdd-mc0 SD-hddnpc SD-worlds ss-s2boot.dat S2-race10-nohelm; do
	cp "$HERE/patches/$f" "$STAGE/patches/$f"
done
# Belt and braces: the loops above cannot pick up patches/.version or .sdstate,
# but a future edit that reaches for a wildcard can, and both of them fail
# silently in the field. serve/.delete is deliberately kept.
rm -f "$STAGE/patches/.version" "$STAGE/patches/.sdstate" "$STAGE/patches/.eqahosts"

# ------------------------------------------------------- the testers' ELFs --
# What an operator hands to players. Without these the archive is a server with
# nothing to connect to it: a tester has no way to reach it from a retail disc,
# no way to prepare a hard disk, and no way to say which end is broken.
#
# TWO OF THE THREE ARE THE SAME BYTES FOR EVERY OPERATOR, and that is a fact
# about those two rather than a hope: EQOAHDD and EQOADIAG both read the server
# address off the memory card at run time, out of eqahosts.txt. So they are
# copied verbatim.
#
# EQOABOOT IS NOT, AND SHIPS AS A TEMPLATE. It patches the LOGIN address into
# SLUS before the game starts -- the retail disc hardcodes that address and no
# card file redirects it -- so a prebuilt copy would aim every operator's
# testers at whichever machine built this archive. The server retargets it at
# startup from its own server_ip (internal/bootelf), which is the same field
# that renders eqahosts.txt and the manifest, so the loader a tester downloads
# and the server they are downloading it from cannot disagree.
#
# The .template.ELF name is load-bearing: it is not runnable-looking, so an
# operator poking around in the folder does not find a plausible EQOABOOT.ELF
# before the server has ever run and send that one out.
echo "== testers"
PLAYERS="$STAGE/players"
mkdir -p "$PLAYERS"
BOOT_SRC="$HERE/../eqoa-boot/out/EQOABOOT.ELF"
HDD_SRC="$HERE/../eqscene/dist/EQOAHDD.ELF"
DIAG_SRC="$HERE/../eqoa-diag/dist/EQOADIAG.ELF"
for f in "$BOOT_SRC" "$HDD_SRC" "$DIAG_SRC"; do
	[ -f "$f" ] || { echo "  MISSING $f -- build it before packaging" >&2; exit 1; }
done
cp "$BOOT_SRC" "$PLAYERS/EQOABOOT.template.ELF"
cp "$HDD_SRC"  "$PLAYERS/EQOAHDD.ELF"
cp "$DIAG_SRC" "$PLAYERS/EQOADIAG.ELF"
cp "$HERE/tools/players-README.txt" "$PLAYERS/README.txt"
cp "$HERE/../eqscene/dist/README.txt"   "$PLAYERS/EQOAHDD-README.txt"
cp "$HERE/../eqoa-diag/dist/README.txt" "$PLAYERS/EQOADIAG-README.txt"
cp "$HERE/../eqoa-boot/dist/README.txt" "$PLAYERS/EQOABOOT-README.txt"
cp "$HERE/../eqscene/dist/LICENSE.ps2sdk" "$PLAYERS/LICENSE.ps2sdk"

# --------------------------------------------------------------- database --
# NPCs only. An operator's own eqoa.db has his players in it and must never be
# overwritten by an unpack, which is why the README says to keep yours.
cp "$HERE/eqoa.db" "$STAGE/eqoa.db"
if command -v sqlite3 >/dev/null; then
	sqlite3 "$STAGE/eqoa.db" \
		"DELETE FROM characters; DELETE FROM accountinfo; VACUUM;" 2>/dev/null || true
fi

# ------------------------------------------------------------------ build --
echo "== build ($GOOS_TARGET)"
(cd "$STAGE" && GOOS="$GOOS_TARGET" GOARCH=amd64 CGO_ENABLED=0 \
	go build -trimpath -ldflags "-s -w" -o "$BIN" ./cmd/gameserver)

# ------------------------------------------------------------------ pack --
rm -f "$OUT"
case "$OUT" in
*.zip) (cd "$STAGE_ROOT" && zip -qr "$OUT" go-eqoa-server) ;;
*) (cd "$STAGE_ROOT" && tar czf "$OUT" go-eqoa-server) ;;
esac

# -------------------------------------------------------------- verify --
# Assert what the archive IS, not that the commands above exited 0. Every line
# here is a mistake that has actually shipped, or would have been invisible.
echo "== verify"
list=$(case "$OUT" in *.zip) unzip -l "$OUT" | awk '{print $4}' ;; *) tar tzf "$OUT" ;; esac)
fail=0
need="go-eqoa-server/$BIN go-eqoa-server/patches/serve/SD go-eqoa-server/patches/serve/SS
go-eqoa-server/patches/serve/S2 go-eqoa-server/patches/SD-hddnpc
go-eqoa-server/config.yaml.example
go-eqoa-server/README.md go-eqoa-server/_deps/eqoa-pkg/go.mod
go-eqoa-server/players/EQOABOOT.template.ELF go-eqoa-server/players/EQOAHDD.ELF
go-eqoa-server/players/EQOADIAG.ELF go-eqoa-server/players/README.txt
go-eqoa-server/players/LICENSE.ps2sdk"
for f in $need; do
	echo "$list" | grep -qx "$f" || { echo "  MISSING $f"; fail=1; }
done
for f in patches/.version patches/.sdstate config.yaml; do
	if echo "$list" | grep -qx "go-eqoa-server/$f"; then
		echo "  MUST NOT SHIP go-eqoa-server/$f"
		fail=1
	fi
done
# A runnable-looking EQOABOOT.ELF in the archive is one an operator can find and
# send out before the server has ever aimed it, and it would carry the address
# THIS machine was built for. The template is the only boot loader that ships.
if echo "$list" | grep -qx "go-eqoa-server/players/EQOABOOT.ELF"; then
	echo "  MUST NOT SHIP go-eqoa-server/players/EQOABOOT.ELF -- it would carry"
	echo "  this build machine's address to every operator who unpacked it"
	fail=1
fi
# The two server-independent tools must be the bytes their own builds verified.
# Checked rather than trusted: a cp from a stale dist/ is silent, and eqscene's
# own dist copy has been stale before -- it shipped an SD that could not send.
for pair in "EQOAHDD.ELF:$HDD_SRC" "EQOADIAG.ELF:$DIAG_SRC"; do
	n=${pair%%:*}; s=${pair#*:}
	[ "$(md5sum "$s" | cut -d' ' -f1)" = "$(md5sum "$PLAYERS/$n" | cut -d' ' -f1)" ] ||
		{ echo "  players/$n differs from $s"; fail=1; }
done
# And the template must be one the server can actually aim, proved by aiming it
# rather than by its presence. A template the retargeter cannot read leaves
# every operator who unpacks this with no boot loader at all, and the first
# person to find out is a tester.
if ! aimed=$(go run ./tools/bootaddr "$PLAYERS/EQOABOOT.template.ELF" 2>&1); then
	echo "  players/EQOABOOT.template.ELF: $aimed"
	fail=1
fi
# The SD in the archive must be the one this server serves, byte for byte. It
# used to be checked by NAME -- config.yaml.example had to spell "SD-hddnpc" and
# both copies had to match -- which is precisely how a distribution shipped the
# previous payload while every check passed: the name was right and the server
# had moved on. There is no name to get wrong now. serve/SD is what is served,
# so serve/SD is what is compared.
sd_want=$(md5sum "$HERE/patches/serve/SD" | cut -d' ' -f1)
sd_got=$(md5sum "$STAGE/patches/serve/SD" | cut -d' ' -f1)
[ "$sd_want" = "$sd_got" ] || { echo "  serve/SD differs from the served one"; fail=1; }
# An override left in the shipped config would serve something other than the
# folder to every operator who unpacks this, silently.
grep -qE '^  (ss|s2|sd)_file:' "$STAGE/config.yaml.example" &&
	{ echo "  config.yaml.example sets a payload override; serve/ must be the default"; fail=1; }
[ "$fail" = 0 ] || { echo "FAILED"; exit 1; }

echo "  serve/SD   $sd_got  $(stat -c%s "$STAGE/patches/serve/SD") bytes"
echo "  serve/     $(ls "$STAGE/patches/serve" | wc -l) files"
echo "  version floor $(grep -o 'versionFloor = [0-9]*' \
	"$HERE/internal/network/tcp/patchserver.go" | head -1 | tr -d '\n')"
ls -l "$OUT"
