# Audit Hikari → v4

Dites Mihomo v1.19.13: config load bersih; routing 📶 WIFI (Inject/Direct/Reject) dites live.

## Diperbaiki
| # | Masalah | Perbaikan |
|---|---------|-----------|
| 1 | Nama file config nggak konsisten (`hikari.yaml` vs `config.yaml` di install.sh) | Tetap `hikari.yaml`, install.sh disesuaikan |
| 2 | Dashboard config nunjuk yacd-meta + download GitHub, padahal UI yang dibundel **zashboard** | `external-ui: ui` aja; install.sh ikut copy `ui/` |
| 3 | `skip-sniff.yaml` & `force-sniff.yaml` ada tapi sniffer nggak pakai | Disambung ke `skip-domain`/`force-domain` |
| 4 | Provider `Blocker` update tanpa proxy | `proxy: "🧩 Inject"` (file lokal tetap cadangan) |
| 5 | Health-check/url-test tanpa `lazy`/`timeout`/`expected-status` | Ditambah (`lazy`, 5000ms, 204) |
| 6 | `geox-url` repo pihak ketiga (`qaz617`) | MetaCubeX resmi |

## Fitur
- Group 📶 WIFI (Inject/Direct/Reject) via `wifi.yaml`, ditaruh setelah Reject & sebelum Blocker.
- Sniffer diperkuat; BFR: `find-process-mode: always` (label app di Connections, tanpa ubah routing).
- Tema zashboard: synthwave, cyberpunk, luxury, coffee, aqua, valentine (JS di-rename `index-BSy1-gCl-v4t.js` biar cache PWA nggak nyangkut).

## Belum diubah — keputusan lu
- Secret `hikari` lemah (permintaan lu).
- `allow-lan: true` + `bind-address: "*"` tanpa auth: port proxy 7890–7892 terbuka ke client hotspot/LAN.
- Rule mati di `smartmode.yaml`: `drive.google.com`, `play.google.com`, `play-fe.googleapis.com`, `fonts.googleapis.com` ketimpa `google.com`/`googleapis.com` di `Inject`.
- `skip-cert-verify: true` di semua node.
- `plus18.mrs` memuat `reddit.com` & `gog.com` → ikut 🔞 PLUSPLUS.
