#!/bin/bash
# Swaps the game server's binary in when no one is connected (the user, 2026-10-29: "is there a
# way to deploy without kicking the players", then "swap when empty"). Every session's DRDP state
# and the World live in the process, so a restart ends them all; a data fix needs none (the server
# reads its tables as it goes), a code change does, and this runs it only once the server has
# been empty a while. Run it every minute (cron); it does nothing until a binary is staged.
#
# Staging: put the new binary at $SWAP/pending/gameserver, written under another name and moved
# there (a half-written file must not be taken), with its sha256 in $SWAP/pending/sha256
# (`sha256sum gameserver | cut -c1-64`; a binary that does not match it is refused). An
# executable $SWAP/pending/after.sh runs once the new server listens (a data step that needs the
# new code; it makes its own backup). A newer staging replaces an older one.
#
# Empty means all of these, again just before the stop (any check that cannot be made counts as
# someone there):
#   - no UDP flow to the game port in the kernel's connection table (/proc/net/nf_conntrack,
#     read with sudo; a client's flow ends about 120 s after its last packet, a relayed browser
#     player's too, from the relay on this machine);
#   - no Station login open on the auth port;
#   - no "UDP received" line in the last $QUIET seconds of the game server's log;
#   - for $QUIET seconds on end (the first empty check starts the clock).
# Then: the binary and the database are copied (*.bak-<when>-pre-swap, the last three kept),
# the service is stopped, the binary moved in, the service started; if it does not listen within
# a minute the old binary goes back and is started, and the staging is moved to failed-<when>.
# A swap done moves the staging to done-<when>. Everything is said in $SWAP/swap.log.
#
#   * * * * * EQOA_UNIT=eqoa-game /path/to/swap_when_empty.sh >> $HOME/eqoa-swap/swap.log 2>&1
set -uo pipefail
DIR=${EQOA_DIR:-$HOME/eqoa-server}
UNIT=${EQOA_UNIT:-eqoa-gameserver}
GAME_PORT=${EQOA_GAME_PORT:-10070}
AUTH_PORT=${EQOA_AUTH_PORT:-9735}
SWAP=${EQOA_SWAP_DIR:-$HOME/eqoa-swap}
QUIET=${EQOA_QUIET_SECONDS:-120}
GAME_LOG=${EQOA_GAME_LOG:-$DIR/gameserver.log}
PENDING=$SWAP/pending
STATE=$SWAP/empty-since

mkdir -p "$SWAP"
exec 9>"$SWAP/.lock"
flock -n 9 || exit 0
log() { echo "$(date -u '+%Y-%m-%d %H:%M:%S') $*"; }

if [ ! -f "$PENDING/gameserver" ]; then
	rm -f "$STATE"
	exit 0
fi

# who is there: prints why the server is not empty and succeeds, or fails when it is empty
someone() {
	local table flows logins last
	if ! table=$(sudo -n cat /proc/net/nf_conntrack 2>/dev/null) || [ -z "$table" ]; then
		echo "the connection table cannot be read"
		return 0
	fi
	flows=$(grep -c -E "^ipv[46] +[0-9]+ +udp .* dport=$GAME_PORT " <<<"$table")
	if [ "$flows" -gt 0 ]; then
		echo "$flows UDP flows to $GAME_PORT"
		return 0
	fi
	if ! logins=$(ss -Htn state established "( sport = :$AUTH_PORT )" 2>/dev/null | wc -l); then
		echo "the auth port's connections cannot be read"
		return 0
	fi
	if [ "$logins" -gt 0 ]; then
		echo "$logins logins on $AUTH_PORT"
		return 0
	fi
	if [ -r "$GAME_LOG" ]; then
		last=$(tail -c 4000000 "$GAME_LOG" | grep "UDP received" | tail -1 | cut -c1-19)
		if [ -n "$last" ] && [ $(( $(date +%s) - $(date -d "$last" +%s 2>/dev/null || echo 0) )) -lt "$QUIET" ]; then
			echo "a packet at $last"
			return 0
		fi
	fi
	return 1
}

if why=$(someone); then
	if [ -f "$STATE" ]; then
		log "not empty any more ($why): waiting"
		rm -f "$STATE"
	fi
	exit 0
fi
now=$(date +%s)
if [ ! -f "$STATE" ]; then
	echo "$now" > "$STATE"
	log "empty: the staged binary goes in after $QUIET s more of it"
	exit 0
fi
since=$(cat "$STATE")
[ $((now - since)) -ge "$QUIET" ] || exit 0

when=$(date -u '+%Y%m%d-%H%M%S')
got=$(sha256sum "$PENDING/gameserver" | cut -c1-64)
want=$(tr -d ' \n' < "$PENDING/sha256" 2>/dev/null)
if [ "$want" != "$got" ]; then
	log "the staged binary is $got, its sha256 file says '${want:-nothing}': refused"
	mv "$PENDING" "$SWAP/failed-$when"
	rm -f "$STATE"
	exit 1
fi
cp -p "$DIR/gameserver" "$DIR/gameserver.bak-$when-pre-swap"
if ! sqlite3 "$DIR/eqoa.db" ".backup $DIR/eqoa.db.bak-$when-pre-swap"; then
	log "the database could not be copied: not swapped"
	rm -f "$DIR/gameserver.bak-$when-pre-swap" "$STATE"
	exit 1
fi
if why=$(someone); then
	log "someone came ($why): not swapped"
	rm -f "$DIR/gameserver.bak-$when-pre-swap" "$DIR/eqoa.db.bak-$when-pre-swap" "$STATE"
	exit 0
fi
log "empty since $(date -u -d "@$since" '+%H:%M:%S'): swapping in ${got:0:16}"
listening() {
	for _ in $(seq 1 120); do
		ss -Hltn "( sport = :$AUTH_PORT )" | grep -q . && return 0
		sleep 0.5
	done
	return 1
}
sudo -n systemctl stop "$UNIT"
cp "$PENDING/gameserver" "$DIR/gameserver.swap" && mv "$DIR/gameserver.swap" "$DIR/gameserver"
sudo -n systemctl start "$UNIT"
if ! listening; then
	log "the new binary did not listen on $AUTH_PORT: the old one goes back"
	sudo -n systemctl stop "$UNIT"
	cp -p "$DIR/gameserver.bak-$when-pre-swap" "$DIR/gameserver"
	sudo -n systemctl start "$UNIT"
	listening && log "the old binary is back and listening" || log "THE OLD BINARY DOES NOT LISTEN EITHER"
	mv "$PENDING" "$SWAP/failed-$when"
	rm -f "$STATE"
	exit 1
fi
if [ -x "$PENDING/after.sh" ]; then
	if (cd "$PENDING" && ./after.sh); then
		log "after.sh done"
	else
		log "after.sh FAILED (the new binary stays; see above)"
	fi
fi
mv "$PENDING" "$SWAP/done-$when"
rm -f "$STATE"
# the last three swaps' copies are kept
for pat in "gameserver.bak-*-pre-swap" "eqoa.db.bak-*-pre-swap"; do
	ls -1t "$DIR"/$pat 2>/dev/null | tail -n +4 | xargs -r rm -f
done
log "swapped: ${got:0:16} is running"
