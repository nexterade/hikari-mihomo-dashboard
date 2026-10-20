#!/system/bin/sh
# Pasang Hikari v4 (BFR) ke Box for Root. Jalankan sebagai root dari folder hasil extract.
# BOX_CLASH_DIR bisa di-override (buat tes): BOX_CLASH_DIR=/path sh install.sh
set -e
SRC="$(cd "$(dirname "$0")" && pwd)/data/adb/box/clash"
DST="${BOX_CLASH_DIR:-/data/adb/box/clash}"
TS="$(date +%Y%m%d-%H%M%S)"

[ -d "$SRC" ] || { echo "folder sumber $SRC nggak ada"; exit 1; }
[ -d "$DST" ] || { echo "$DST nggak ada. Box for Root belum terpasang?"; exit 1; }

[ -f "$DST/hikari.yaml" ] && cp -p "$DST/hikari.yaml" "$DST/hikari.yaml.bak-$TS" && echo "backup: $DST/hikari.yaml.bak-$TS"
mkdir -p "$DST/proxy" "$DST/ruleset"
cp -f "$SRC/hikari.yaml" "$DST/hikari.yaml"
cp -f "$SRC/proxy/"* "$DST/proxy/"
cp -f "$SRC/ruleset/"* "$DST/ruleset/"
# ui lama dibuang dulu biar file index-*.js versi lama nggak numpuk
rm -rf "$DST/ui" && mkdir -p "$DST/ui" && cp -rf "$SRC/ui/." "$DST/ui/"
chmod 0644 "$DST/hikari.yaml" "$DST/proxy/"* "$DST/ruleset/"*
echo "OK. Config: $DST/hikari.yaml"
echo "Pastikan Box baca hikari.yaml (cek path config di /data/adb/box/settings.ini), lalu:"
echo "  /data/adb/box/scripts/box.service restart"
