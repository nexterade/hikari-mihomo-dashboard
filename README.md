# Hikari v4 — BFR (Box for Root / Clash for Magisk, Android)

Core: **Mihomo (Meta) build android**. Config: `hikari.yaml`, secret: `hikari`.

## Pasang (root shell / Termux `su`)
```sh
cd <folder-hasil-extract>
sh install.sh
/data/adb/box/scripts/box.service restart
```
Pastikan Box baca `hikari.yaml` (path config di `/data/adb/box/settings.ini`).

## Setting Box
`bin_name="clash"`, `xclash_option="mihomo"`, `network_mode="tproxy"`, `ipv6="false"`.

## Yang berubah dari config asli
- **Group & rantai routing tetap seperti semula** (MATCH → 🧩 Inject). Satu-satunya group baru: **📶 WIFI** (Inject / Direct / Reject), sumbernya `ruleset/wifi.yaml`.
- **Deteksi per-app:** `find-process-mode: always` → tiap koneksi dilabeli nama package app-nya di dashboard > Connections (kolom Process). Routing tidak diubah.
- Sniffer diperkuat (skip LAN/loopback; `skip-sniff`/`force-sniff` aktif).
- Dashboard zashboard + tema tambahan: synthwave, cyberpunk, luxury, coffee, aqua, valentine.
- Detail: `AUDIT.md`.

## Default group (v4.1)
| Group | Default |
|-------|---------|
| 🧩 Inject | 🔥 AUTO MODE |
| ✨ SMART ROUTING | 🧩 Inject |
| 🛑 BLOCKER | ✂️ Reject |
| 🔞 PLUSPLUS | ✂️ Reject |
| 📶 WIFI | 🎮 Direct |

`profile.store-selected: true` menyimpan pilihan lama di `cache.db`, jadi pilihan yang pernah lu klik di dashboard **mengalahkan default baru**. Biar default baru kepakai: hapus `cache.db` di folder kerja core (BFR: `/data/adb/box/clash/cache.db`, Nikki: `/etc/nikki/run/cache.db`) lalu restart.

DNS: `doh.mullvad.net` diganti `dns10.quad9.net` (Quad9 tanpa filter). Mullvad dulu juga nyangkut di SmartMode karena `mullvad.net` ada di list.

## Statistik permanen (hikari-stats)
Collector kecil yang jalan terus di belakang layar, jadi dashboard nggak perlu dibuka. Hitung total **harian, mingguan, bulanan**, dipisah **WiFi vs Data seluler**, lalu disajikan sebagai halaman bertema cyberpunk.

```sh
cd <folder-hasil-extract>
sh install-stats.sh        # pasang + start + autostart tiap boot (/data/adb/service.d/hikari-stats.sh)
```
Buka **http://127.0.0.1:9099/** di browser HP. Kontrol: `sh /data/adb/box/stats/stats-ctl.sh start|stop|restart|status`.

- **Data tersimpan** di `/data/adb/box/stats/stats.json` (ditulis tiap 30 dtk dan saat stop). Aman waktu Box/core restart, update zip, atau reboot. Backup/ekspor: tombol *Ekspor CSV* di halaman.
- **WiFi vs Data** ditentukan dari IP sumber koneksi, memakai CIDR yang sama dengan `ruleset/wifi.yaml` (default `192.168.0.0/16`). Ubah file itu, lalu `stats-ctl.sh restart`. Trafik internal (DNS DoH dari core, port proxy lokal) dipisah dan tidak masuk total.
- **Total akurat** (dari counter Mihomo, termasuk koneksi singkat yang nutup di antara dua polling). **Daftar teratas** (aplikasi, rute, jalur keluar) dan jumlah koneksi adalah perkiraan dari koneksi yang sempat terlihat.
- Halaman hanya bisa dibuka dari HP itu sendiri (`127.0.0.1`); client hotspot nggak bisa akses.
- Secret dan port controller dibaca otomatis dari `hikari.yaml`.
- Binary arm64 statis. Source ada di `stats-src/` (bisa dibuild ulang: `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build`). SHA-256 binary: `9be4bbcc33ce2b23dd4380c2121d4a3f1dbf725341d05e8064144a5f90c0403b`
