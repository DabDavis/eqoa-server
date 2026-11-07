#!/bin/bash
# Swaps the staged game server binary in WITHOUT ending anyone's session (internal/network/udp/keep.go;
# the user, 2026-11-06: "is there a way to keep folks in world and not boot with a restart"). The running
# server is signalled (SIGUSR2): it tells everyone in the world, saves them as a shutdown does, writes its
# sessions down beside the database and, finding keep.hold there, writes keep.saved and waits. This script
# then copies the database (after the save: a rollback keeps everyone's places), runs the staging's
# before.sh under `timeout` (the clients wait about 30 s for a silent server: a data step must be short),
# moves the new binary into place and removes keep.hold; the server re-execs into the new binary with the
# game's socket open, and the new process puts every session back. Players see the server stand still for a
# few seconds and a dump where they stand.
#
# Staging is swap_when_empty.sh's: $SWAP/pending/{gameserver,sha256} and an optional before.sh. A staging
# with a file named `cold` in it is refused here (its data step is long, or it needs the server stopped): swap
# it the old way. A server that does not answer the signal within 10 s (a binary from before this, on which
# SIGUSR2 does nothing) is left as it was, and nothing is swapped (exit 3): the first swap INTO a binary that
# keeps sessions is a cold one.
#
# If before.sh fails or runs past $BEFORE_MAX seconds: the database goes back to the copy made after the save,
# the old binary stays, keep.hold is removed, and the server re-execs into the same binary (the sessions
# kept). If the new binary does not listen within a minute of the exec: it is stopped, the old binary goes
# back (and the database, when a before.sh ran) and is started: the sessions end, as a cold swap ends them.
#
#   EQOA_UNIT=eqoa-game ./swap_keep.sh        (on the box; logs to $SWAP/swap.log)
set -uo pipefail
DIR=${EQOA_DIR:-$HOME/eqoa-server}
UNIT=${EQOA_UNIT:-eqoa-gameserver}
AUTH_PORT=${EQOA_AUTH_PORT:-9735}
SWAP=${EQOA_SWAP_DIR:-$HOME/eqoa-swap}
GAME_LOG=${EQOA_GAME_LOG:-$DIR/gameserver.log}
BEFORE_MAX=${EQOA_BEFORE_MAX:-12}
PENDING=$SWAP/pending
HOLD=$DIR/keep.hold
SAVED=$DIR/keep.saved

exec 9>"$SWAP/.lock"
flock -w 120 9 || { echo "the swap lock is held"; exit 1; }
log() { echo "$(date -u '+%Y-%m-%d %H:%M:%S') $*" | tee -a "$SWAP/swap.log"; }
[ -f "$PENDING/gameserver" ] || { echo "nothing staged"; exit 1; }
if [ -e "$PENDING/cold" ]; then
	log "keep: the staging says cold (pending/cold): swap it the old way"
	exit 2
fi
when=$(date -u '+%Y%m%d-%H%M%S')
got=$(sha256sum "$PENDING/gameserver" | cut -c1-64)
want=$(tr -d ' \n' < "$PENDING/sha256" 2>/dev/null)
[ "$want" = "$got" ] || { log "keep: the staged binary is $got, its sha256 file says '${want:-nothing}': refused"; exit 1; }
pid=$(systemctl show -p MainPID --value "$UNIT" 2>/dev/null)
[ -n "$pid" ] && [ "$pid" -gt 0 ] || { log "keep: $UNIT has no running process"; exit 1; }
cp -p "$DIR/gameserver" "$DIR/gameserver.bak-$when-pre-swap" || exit 1

listening() {
	for _ in $(seq 1 120); do
		ss -Hltn "( sport = :$AUTH_PORT )" | grep -q . && return 0
		sleep 0.5
	done
	return 1
}
db_back() {
	if cp -p "$DIR/eqoa.db.bak-$when-pre-swap" "$DIR/eqoa.db"; then
		log "keep: the database is back as the server saved it"
	else
		log "keep: THE DATABASE COULD NOT BE PUT BACK from $DIR/eqoa.db.bak-$when-pre-swap"
	fi
}

rm -f "$SAVED"
touch "$HOLD"
from=$(wc -l < "$GAME_LOG")
log "keep: swapping in ${got:0:16} keeping the sessions: SIGUSR2 to $pid"
kill -USR2 "$pid"
for _ in $(seq 1 100); do
	[ -f "$SAVED" ] && break
	sleep 0.1
done
if [ ! -f "$SAVED" ]; then
	rm -f "$HOLD"
	rm -f "$DIR/gameserver.bak-$when-pre-swap"
	log "keep: the server did not answer SIGUSR2 in 10 s (a binary from before keeping sessions?): nothing swapped; swap it the old way"
	exit 3
fi
held=$(date +%s.%N)
if ! sqlite3 -cmd ".timeout 10000" "$DIR/eqoa.db" ".backup $DIR/eqoa.db.bak-$when-pre-swap"; then
	rm -f "$HOLD"
	log "keep: the database could not be copied: nothing swapped (the server goes on with the same binary)"
	exit 1
fi
before=no
if [ -x "$PENDING/before.sh" ]; then
	if (cd "$PENDING" && timeout "$BEFORE_MAX" ./before.sh) > "$SWAP/before-$when.log" 2>&1; then
		log "keep: before.sh done (before-$when.log)"
		before=yes
	else
		log "keep: before.sh FAILED or ran past ${BEFORE_MAX}s (before-$when.log): not swapped, the server goes on with the same binary"
		db_back
		rm -f "$HOLD"
		mv "$PENDING" "$SWAP/failed-keep-$when"
		exit 1
	fi
fi
cp "$PENDING/gameserver" "$DIR/gameserver.swap" && mv "$DIR/gameserver.swap" "$DIR/gameserver"
rm -f "$HOLD"
log "keep: the new binary is in place after $(awk -v a="$held" -v b="$(date +%s.%N)" 'BEGIN { printf "%.1f", b - a }') s of hold; the server execs into it"
# the new process says what it put back; a binary that never gets that far is rolled back as a cold swap is
ok=no
for _ in $(seq 1 120); do
	# not grep -q: it stops at the match, and under pipefail the tail it leaves with more than a pipe's
	# worth of log to write dies of SIGPIPE, so the pipeline fails and a server that came up is rolled
	# back (2026-11-07: 92 KB of dumps followed the line within the second; every player booted)
	if tail -n +"$from" "$GAME_LOG" | grep -a "Keep: .* sessions resumed\|Keep: no sessions resumed" > /dev/null; then
		ok=yes
		break
	fi
	sleep 0.5
done
if [ "$ok" = yes ] && listening; then
	tail -n +"$from" "$GAME_LOG" | grep -a "Keep: " | tail -4 | while IFS= read -r l; do log "  $l"; done
	mv "$PENDING" "$SWAP/done-keep-$when"
	for pat in "gameserver.bak-*-pre-swap" "eqoa.db.bak-*-pre-swap"; do
		ls -1t "$DIR"/$pat 2>/dev/null | tail -n +4 | xargs -r rm -f
	done
	log "keep: swapped: ${got:0:16} is running, the sessions kept"
	exit 0
fi
log "keep: the new binary did not come up: the old one goes back (the sessions end)"
sudo -n systemctl stop "$UNIT"
cp -p "$DIR/gameserver.bak-$when-pre-swap" "$DIR/gameserver"
[ "$before" = yes ] && db_back
sudo -n systemctl start "$UNIT"
listening && log "keep: the old binary is back and listening" || log "keep: THE OLD BINARY DOES NOT LISTEN EITHER"
mv "$PENDING" "$SWAP/failed-keep-$when"
exit 1
