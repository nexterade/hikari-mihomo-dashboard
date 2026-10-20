#!/system/bin/sh
# Kontrol collector statistik Hikari.  Pakai: sh stats-ctl.sh start|stop|restart|status
D="$(cd "$(dirname "$0")" && pwd)"
CLASH="${BOX_CLASH_DIR:-/data/adb/box/clash}"
CFG="$CLASH/hikari.yaml"
BIN="$D/hikari-stats"; PIDF="$D/stats.pid"; LOG="$D/stats.log"
LISTEN="${STATS_LISTEN:-127.0.0.1:9099}"

secret() { sed -n "s/^secret: *['\"]\{0,1\}\([^'\" #]*\).*/\1/p" "$CFG" 2>/dev/null | head -n 1; }
port()   { sed -n "s/^external-controller: *[^:]*:\([0-9]*\).*/\1/p" "$CFG" 2>/dev/null | head -n 1; }
running() {
  [ -f "$PIDF" ] || return 1
  p="$(cat "$PIDF" 2>/dev/null)"; [ -n "$p" ] && [ -d "/proc/$p" ] && grep -q hikari-stats "/proc/$p/cmdline" 2>/dev/null
}

case "$1" in
start)
  running && { echo "sudah jalan (pid $(cat "$PIDF")): http://$LISTEN/"; exit 0; }
  [ -x "$BIN" ] || { echo "$BIN nggak ada / bukan executable"; exit 1; }
  [ -f "$LOG" ] && [ "$(wc -c < "$LOG")" -gt 1048576 ] && mv -f "$LOG" "$LOG.old"
  HIKARI_SECRET="$(secret)" "$BIN" -api "http://127.0.0.1:${STATS_API_PORT:-$(port)}" \
    -data "$D/stats.json" -wifi-file "$CLASH/ruleset/wifi.yaml" -listen "$LISTEN" >> "$LOG" 2>&1 &
  echo $! > "$PIDF"; sleep 1
  running && echo "jalan: http://$LISTEN/" || { echo "gagal start, cek $LOG"; exit 1; }
  ;;
stop)
  running || { echo "nggak jalan"; exit 0; }
  p="$(cat "$PIDF")"; kill "$p"
  i=0; while [ -d "/proc/$p" ] && [ $i -lt 10 ]; do sleep 1; i=$((i+1)); done
  [ -d "/proc/$p" ] && kill -9 "$p"
  rm -f "$PIDF"; echo "berhenti (data tersimpan di $D/stats.json)"
  ;;
restart) sh "$0" stop; sh "$0" start ;;
status)
  if running; then echo "jalan (pid $(cat "$PIDF")): http://$LISTEN/"; else echo "mati"; fi
  [ -f "$D/stats.json" ] && echo "data: $D/stats.json ($(wc -c < "$D/stats.json") byte)"
  [ -f "$LOG" ] && { echo "--- log terakhir"; tail -n 5 "$LOG"; }
  ;;
*) echo "pakai: sh stats-ctl.sh start|stop|restart|status"; exit 1 ;;
esac
