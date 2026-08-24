# NVR EZVIZ

Self-hosted NVR untuk kamera EZVIZ multi-lokasi: workspace multi-tenant, live view multiview, upload rekaman ke
Google Drive (prioritas) atau S3/MinIO, retention otomatis, audit log, dan notifikasi webhook.

## Struktur

```
apps/
  api/          Go (Fiber + GORM + MySQL) — REST API, auth, RBAC, OAuth Google, retention job, notifikasi
  edge-agent/   Go — jalan di tiap lokasi (site), rekam RTSP kamera, push live view, upload ke storage
  web/          TanStack Start (React) — dashboard admin, storage, live view multiview, audit log, health
infra/
  mediamtx.yml  Config relay live view (RTSP in, HLS out), auth per-request lewat API
docker-compose.yml   MySQL + MinIO + MediaMTX + api + web untuk dev lokal
```

Model akses: **superadmin** mengelola users, sites, kamera secara global. **Workspace admin** (role per-workspace)
mengelola member, binding kamera↔storage, storage target, dan notifikasi di workspace-nya. **Viewer** hanya melihat.
Kamera bisa dipasang ke banyak workspace; user bisa jadi anggota banyak workspace — sesuai skema `camera_workspaces`
dan `user_workspaces` di [apps/api/internal/models/models.go](apps/api/internal/models/models.go).

Edge agent di tiap gedung hanya melakukan koneksi **outbound** (heartbeat, live push, upload) ke API pusat / MediaMTX
— tidak perlu buka port atau IP publik di lokasi kamera, jadi 3 gedung dengan 3 ISP berbeda tetap bisa dipantau dari
satu domain VPS, termasuk dari HP via data seluler.

## Jalan di lokal

Butuh: Docker, Go 1.25+, Node 22+.

```bash
cp .env.example .env
docker compose up -d mysql minio minio-init mediamtx
cd apps/api && go run ./cmd/api
```

Di terminal lain, buat akun superadmin pertama (wajib — tidak ada self-registration):

```bash
cd apps/api && go run ./cmd/seed --email admin@example.com --password ganti-ini --name Admin
```

Jalankan frontend:

```bash
cd apps/web && npm install && npm run dev
```

Buka `http://localhost:3000`, login pakai akun yang baru dibuat.

### Uji coba edge agent + live view (multiview) secara lokal

1. Login sebagai superadmin → tab **Admin → Sites & Kamera** → buat site baru, simpan `agent_token` yang muncul (hanya ditampilkan sekali).
2. Tambahkan kamera di site itu. Isi `local_rtsp_url` dengan RTSP kamera EZVIZ (aktifkan RTSP lokal di app EZVIZ) —
   atau untuk tes tanpa kamera fisik, jalankan RTSP server dummy dan push video sample ke situ dengan
   `ffmpeg -re -stream_loop -1 -i sample.mp4 -c copy -f rtsp rtsp://localhost:8554/test`.
3. Di workspace, tab **Kamera**, pasangkan kamera ke sebuah **Storage** target (lihat bagian Google Drive di bawah,
   atau untuk tes cepat pakai MinIO: endpoint `localhost:9000`, access/secret key `nvr-admin`/`nvr-admin-secret`,
   bucket `nvr-recordings`, `use_ssl: false`).
4. Jalankan agent (perlu `ffmpeg` terpasang):
   ```bash
   cd apps/edge-agent
   AGENT_TOKEN=<token dari langkah 1> \
   API_BASE_URL=http://localhost:8080 \
   MEDIAMTX_HOST=localhost:8554 \
   go run ./cmd/agent
   ```
   Publish live view otentikasi pakai `AGENT_TOKEN` yang sama (site itu juga) — tidak ada secret terpisah.
5. Buka workspace di dashboard → tombol **Live View** → grid multiview (1x1/2x2/3x3) menampilkan tiap kamera
   sebagai tile HLS (lihat [apps/web/src/routes/workspaces.$workspaceId.live.tsx](apps/web/src/routes/workspaces.$workspaceId.live.tsx)).
   Agent men-push satu kali decode RTSP ke dua tujuan sekaligus (segmen lokal + live ke MediaMTX) lewat `ffmpeg`
   dua-output, jadi tidak ada beban pull RTSP tambahan ke kamera.

**Live view auth**: setiap publish/read ke MediaMTX divalidasi *per-request* lewat webhook
`/api/mediamtx/auth` ([apps/api/internal/handlers/mediamtx_auth_handler.go](apps/api/internal/handlers/mediamtx_auth_handler.go))
— bukan shared secret. Publish valid kalau password = `agent_token` milik site pemilik kamera; read valid kalau
password = JWT user yang memang anggota workspace kamera itu. Jadi tidak ada satu secret bocor yang membuka semua
kamera di semua workspace.

### Hubungkan Google Drive (storage utama)

1. Di [Google Cloud Console](https://console.cloud.google.com/): buat project → aktifkan **Google Drive API** →
   buat **OAuth client ID** tipe "Web application" → set Authorized redirect URI ke
   `http://localhost:8080/api/oauth/google/callback` (atau domain API produksi kamu).
2. Isi `GOOGLE_OAUTH_CLIENT_ID` dan `GOOGLE_OAUTH_CLIENT_SECRET` di `.env` API, restart `apps/api`.
3. Di dashboard, workspace → tab **Storage** → **Hubungkan Google Drive** → login & consent lewat layar Google resmi.
   Refresh token otomatis tersimpan (dienkripsi, lihat di bawah) sebagai storage target baru — tidak perlu tempel
   token manual.
4. Pasangkan kamera ke storage target itu di tab **Kamera**. Edge agent akan upload tiap segmen selesai rekam ke
   folder Drive akun tersebut (lihat [apps/edge-agent/internal/uploader/gdrive.go](apps/edge-agent/internal/uploader/gdrive.go)).

Kalau `GOOGLE_OAUTH_CLIENT_ID` belum diisi, tombol "Hubungkan Google Drive" akan gagal dengan pesan jelas
(412 Precondition Failed) — form manual (isi `client_id`/`client_secret`/`refresh_token` sendiri) tetap tersedia
sebagai fallback di bawahnya.

### Retention & enkripsi

- `STORAGE_ENCRYPTION_KEY` (API): kunci AES-256-GCM untuk mengenkripsi `StorageTarget.Config` (access key S3,
  refresh token Google Drive) sebelum disimpan ke DB. Kalau kosong, dipakai fallback turunan dari `JWT_SECRET` —
  cukup untuk dev, **wajib diisi eksplisit** untuk produksi (lihat [apps/api/internal/cryptoutil](apps/api/internal/cryptoutil)).
- `RETENTION_INTERVAL_MINUTES` (API, default 60): API menjalankan job periodik yang menghapus `Recording` lebih tua
  dari `retain_days` milik storage target-nya — baik dari Google Drive/S3/MinIO maupun baris metadatanya
  (lihat [apps/api/internal/retention](apps/api/internal/retention)).

### Notifikasi & audit log

- **Notifikasi webhook** (tab workspace → Notifikasi): daftarkan URL webhook (Slack/Discord incoming webhook, atau
  endpoint HTTP kustom) dan pilih event (`camera_offline`, `upload_failed`). API mengirim POST JSON
  `{event, message, timestamp}` — lihat [apps/api/internal/notify](apps/api/internal/notify). Notifikasi kamera
  offline di-debounce di sisi agent (maks 1x/15 menit per kamera) supaya tidak spam saat retry upload terus gagal.
- **Audit log** (tab Admin → Audit Log, superadmin): mencatat siapa melakukan apa — create/delete user, workspace,
  site, kamera, storage target, notification channel, assign kamera, ubah membership — lihat
  [apps/api/internal/handlers/audit_handler.go](apps/api/internal/handlers/audit_handler.go).
- **Health** (tab Admin → Health): status online/offline tiap site (berdasar heartbeat terakhir) dan ringkasan
  status kamera. `GET /healthz` (di luar `/api`) untuk liveness probe infra (dipakai `docker-compose.yml`'s
  healthcheck).

## Deploy ke VPS (dengan CloudPanel)

CloudPanel tidak punya tipe site "Go app" bawaan. Caranya:

1. Jalankan `docker compose up -d --build` di VPS untuk service `mysql` (atau pakai MySQL yang sudah ada di
   CloudPanel — ganti `MYSQL_DSN` supaya arahkan ke situ, tidak perlu container `mysql` lagi), `minio` (kalau masih
   dipakai selain Google Drive), `mediamtx`, `api`, dan `web`.
2. Di CloudPanel, buat site baru tipe **Reverse Proxy** untuk domain API → arahkan ke `127.0.0.1:8080`.
3. Buat site **Reverse Proxy** lagi untuk domain dashboard → arahkan ke `127.0.0.1:3000`.
4. MediaMTX butuh port RTSP (`8554`) & HLS (`8888`) terbuka langsung di firewall VPS (bukan lewat CloudPanel reverse
   proxy, karena RTSP bukan HTTP) — edge agent publish ke `8554`, browser baca HLS dari `8888`. MediaMTX memanggil
   balik `api:8080/api/mediamtx/auth` di jaringan Docker internal, tidak perlu diekspos publik.
5. Update `GOOGLE_OAUTH_REDIRECT_URL` & `WEB_BASE_URL` di env API ke domain produksi, dan tambahkan redirect URI itu
   di Google Cloud Console OAuth client.
6. Edge agent dijalankan **di tiap lokasi/gedung** (bukan di VPS) — build binary (`go build ./cmd/agent`) atau
   jalankan sebagai container, set `API_BASE_URL`/`MEDIAMTX_HOST` ke domain publik VPS dan `AGENT_TOKEN` milik site
   tersebut. Tidak perlu VPN, tidak perlu port terbuka di lokasi kamera.

## Belum diimplementasikan

- **EZVIZ Cloud API fallback** — tidak dikerjakan (butuh langganan EZVIZ Open Platform berbayar untuk kuota yang
  layak). Agent hanya mendukung `local_rtsp_url` (RTSP lokal, gratis, tanpa batas kuota) — cukup selama kamera EZVIZ
  diaktifkan RTSP lokalnya di app EZVIZ.
- Audit log & notification channel saat ini terbatas untuk aksi yang mengubah data lewat dashboard/API; belum
  mencakup semua kemungkinan (mis. login gagal berulang, perubahan lewat query DB langsung).

## Sudah selesai

- Workspace multi-tenant, RBAC (superadmin/workspace admin/viewer), edge agent outbound-only multi-lokasi
- Live view multiview (MediaMTX + grid 1x1/2x2/3x3), auth per-request tanpa shared secret
- Google Drive "Connect" via OAuth resmi (storage utama) + fallback S3/MinIO manual
- Enkripsi kredensial storage at rest (AES-256-GCM)
- Retention/cleanup job otomatis (Drive/S3/MinIO + metadata)
- Picker pencarian untuk assign kamera ke workspace & tambah anggota (superadmin)
- Notifikasi webhook (kamera offline, upload gagal) + audit log + health/status tab
