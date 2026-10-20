#!/system/bin/sh
# Pasang collector statistik Hikari (jalan otomatis tiap boot). Jalankan sebagai root dari folder hasil extract.
# Override buat tes: STATS_DIR=/path SERVICE_D=/path sh install-stats.sh
set -e
SRC="$(cd "$(dirname "$0")" && pwd)/data/adb/box/stats"
DST="${STATS_DIR:-/data/adb/box/stats}"
SVC="${SERVICE_D:-/data/adb/service.d}"
[ -f "$SRC/hikari-stats" ] || { echo "binary nggak ada di $SRC"; exit 1; }

mkdir -p "$DST" "$SVC"
[ -f "$DST/stats-ctl.sh" ] && sh "$DST/stats-ctl.sh" stop || true
cp -f "$SRC/hikari-stats" "$DST/hikari-stats"
cp -f "$SRC/stats-ctl.sh" "$DST/stats-ctl.sh"
chmod 0755 "$DST/hikari-stats" "$DST/stats-ctl.sh"      # stats.json (data lama) nggak disentuh

cat > "$SVC/hikari-stats.sh" <<EOF
#!/system/bin/sh
# Jalankan collector statistik Hikari setelah boot selesai
(
  until [ "\$(getprop sys.boot_completed)" = "1" ]; do sleep 5; done
  sleep 20
  sh $DST/stats-ctl.sh start
) >/dev/null 2>&1 &
EOF
chmod 0755 "$SVC/hikari-stats.sh"

sh "$DST/stats-ctl.sh" start
echo "Laporan: buka http://127.0.0.1:9099/ di browser HP."
