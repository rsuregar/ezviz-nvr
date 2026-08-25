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
scripts/
  dev.sh                  Jalankan seluruh stack lokal dengan satu perintah
  deploy-central.sh       Build + pasang api & web sebagai systemd service di VPS
  deploy-edge-agent.sh    Build + pasang edge agent sebagai systemd service di lokasi kamera
docker-compose.yml   MySQL + MinIO + MediaMTX + api + web (opsional, lihat "Tanpa Docker" di bawah)
```

Model akses: **superadmin** mengelola users, sites, kamera secara global. **Workspace admin** (role per-workspace)
mengelola member, binding kamera↔storage, storage target, dan notifikasi di workspace-nya. **Viewer** hanya melihat.
Kamera bisa dipasang ke banyak workspace; user bisa jadi anggota banyak workspace — sesuai skema `camera_workspaces`
dan `user_workspaces` di [apps/api/internal/models/models.go](apps/api/internal/models/models.go).

Edge agent di tiap gedung hanya melakukan koneksi **outbound** (heartbeat, live push, upload) ke API pusat / MediaMTX
— tidak perlu buka port atau IP publik di lokasi kamera, jadi 3 gedung dengan 3 ISP berbeda tetap bisa dipantau dari
satu domain VPS, termasuk dari HP via data seluler.

Semua service (`api`, `edge-agent`, `web`) adalah proses biasa — Go compile jadi satu binary, `web` cuma Node
server. **Docker itu opsional**, cuma dipakai untuk menyalakan MySQL/MinIO/MediaMTX dengan cepat. Kalau kamu sudah
punya (atau mau install native) MySQL, semuanya bisa jalan tanpa Docker sama sekali — lihat Opsi B di bawah.

### Site vs Workspace — dua konsep yang sengaja dipisah

Sumber kebingungan paling umum saat pertama pakai, jadi ditulis eksplisit di sini:

- **Site** = satu lokasi fisik (gedung) dengan satu edge agent yang menetap permanen di LAN yang sama dengan
  kamera-kameranya. Site menentukan **siapa yang merekam** — dan itu wajib satu jaringan, karena RTSP adalah
  protokol lokal, bukan sesuatu yang bisa "dipanggil" dari internet seperti app EZVIZ resmi (kamera EZVIZ asli
  punya firmware cloud-connect sendiri; kamera yang kita treat di sini murni RTSP, makanya butuh edge agent sebagai
  jembatan). Kamera tidak bisa direkam lintas site kecuali dua site itu memang satu LAN yang sama — yang bisa
  dilakukan adalah **memindahkan** kamera ke site lain (tombol "Pindahkan ke site..." di Admin → Sites & Kamera),
  tapi RTSP kamera itu sendiri tetap harus bisa dijangkau dari LAN site tujuan setelah dipindah.
- **Workspace** = pengelompokan logis untuk **siapa yang boleh melihat** — lintas site sepenuhnya. Satu workspace
  bisa berisi kamera dari site manapun sekaligus (superadmin bebas assign kamera dari site mana pun ke satu
  workspace), dan Live View-nya sudah punya filter per-site untuk kasus itu.

Yang membuat live view/rekaman bisa diakses dari jaringan manapun (persis seperti app EZVIZ resmi) adalah kombinasi
**workspace + VPS pusat**: browser viewer tidak pernah connect langsung ke kamera, dia connect ke domain VPS (lihat
"Deploy ke VPS" di bawah), yang meneruskan feed dari edge agent lokasi tersebut. Yang wajib tetap satu LAN dengan
kamera hanya mesin edge agent-nya — device viewer (laptop/HP siapa pun yang buka dashboard) bebas di jaringan
manapun, termasuk data seluler.

## Jalan di lokal

Butuh (kedua opsi): Go 1.25+, Node 22+, `ffmpeg` (buat edge agent).

**Setelah setup awal (langkah 1 salah satu opsi di bawah + `.env` sudah terisi), untuk hari-hari berikutnya cukup
satu perintah**: `./scripts/dev.sh` — menyalakan API, MediaMTX (kalau terpasang), dan web dev server sekaligus,
Ctrl+C mematikan semuanya. Lihat komentar di [scripts/dev.sh](scripts/dev.sh) untuk opsi env (`WITH_MEDIAMTX=0`,
`WITH_AGENT=1`). Langkah manual di bawah ini tetap didokumentasikan untuk setup awal / kalau mau jalankan tiap
service terpisah.

### Opsi A — dengan Docker (paling cepat)

```bash
cp .env.example .env
docker compose up -d mysql minio minio-init mediamtx
cd apps/api && go run ./cmd/api
```

### Opsi B — tanpa Docker (native)

Semua service database/relay dipasang langsung di mesin kamu lewat Homebrew (macOS) — tidak perlu Docker Desktop
sama sekali.

1. **MySQL/MariaDB** — kalau belum ada:
   ```bash
   brew install mariadb
   brew services start mariadb
   ```
   Buat database & user (ganti `-u root -p` jadi `-u root` saja kalau root belum punya password, mis. instalasi baru):
   ```bash
   mysql -u root -p -e "
   CREATE DATABASE nvr CHARACTER SET utf8mb4;
   CREATE USER 'nvr'@'localhost' IDENTIFIED BY 'nvr';
   GRANT ALL PRIVILEGES ON nvr.* TO 'nvr'@'localhost';
   "
   ```
   Default `MYSQL_DSN` di `.env.example` sudah mengarah ke `127.0.0.1:3306` dengan user/password itu, jadi tidak
   perlu diubah kalau kamu ikuti nama di atas persis. Kalau MySQL native kamu pakai auth socket dan `nvr@127.0.0.1`
   ditolak, jalankan `CREATE USER 'nvr'@'127.0.0.1' ...` juga (localhost dan 127.0.0.1 dihitung beda user oleh MySQL).

2. **ffmpeg** (wajib untuk edge agent, dan MinIO/MediaMTX di bawah ini opsional):
   ```bash
   brew install ffmpeg
   ```

3. **MediaMTX** (opsional — cuma perlu kalau mau coba live view/multiview):
   ```bash
   brew install mediamtx
   MTX_AUTHHTTPADDRESS=http://localhost:8080/api/mediamtx/auth mediamtx infra/mediamtx.yml
   ```
   `infra/mediamtx.yml` defaultnya mengarah ke `http://api:8080/...` (nama service di jaringan Docker) — env var
   `MTX_AUTHHTTPADDRESS` di atas meng-override itu ke `localhost` supaya nyambung ke API yang jalan native di
   langkah 5. Ini jalan sebagai proses foreground biasa (bisa juga `brew services start mediamtx` kalau mau di
   background, tapi itu pakai config default bawaan Homebrew, bukan `infra/mediamtx.yml` kita — untuk dev lebih
   gampang jalankan foreground dengan config kita langsung seperti di atas).

4. **MinIO** (opsional — cuma perlu kalau mau tes storage S3-compatible; **kalau storage utamamu Google Drive,
   lewati langkah ini**, tidak perlu service apa pun untuk Google Drive):
   ```bash
   brew install minio
   mkdir -p /tmp/nvr-minio-data
   MINIO_ROOT_USER=nvr-admin MINIO_ROOT_PASSWORD=nvr-admin-secret minio server /tmp/nvr-minio-data --console-address ":9001"
   ```
   Lalu buat bucket sekali lewat console-nya di `http://localhost:9001` (login `nvr-admin`/`nvr-admin-secret`) →
   buat bucket bernama `nvr-recordings`.

5. **Jalankan API** (di terminal baru, setelah MySQL siap):
   ```bash
   cp .env.example .env
   cd apps/api && go run ./cmd/api
   ```

Tidak ada `docker compose` sama sekali di jalur ini — MySQL, MediaMTX, dan MinIO semuanya proses native lewat
Homebrew, langsung `go run`/`npm run dev` untuk sisanya. Kalau kamu hanya mau coba workspace/user/kamera/Google
Drive tanpa live view, cukup langkah 1, 2, dan 5 saja (skip MediaMTX & MinIO).

### Lanjutan (sama untuk kedua opsi)

Buat akun superadmin pertama (wajib — tidak ada self-registration):

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
   Publish live view otentikasi pakai `AGENT_TOKEN` yang sama (site itu juga) — tidak ada secret terpisah. Kalau
   MediaMTX tidak dijalankan (baik Opsi A maupun B), cukup hapus baris `MEDIAMTX_HOST=...` — recording tetap jalan,
   cuma live view yang tidak aktif.

   **Alternatif tanpa copy-paste token ke `.env`**: jalankan `go run ./cmd/agent` tanpa `AGENT_TOKEN` sama sekali.
   Agent akan membuka halaman setup sendiri di jaringan lokal (alamatnya dicetak di log, mis.
   `http://192.168.1.5:8091`) dan menunggu di sana. Di Admin → Sites & Kamera, klik **Buat kode pairing** pada site
   yang dituju (berlaku 15 menit, sekali pakai), lalu masukkan kode itu di halaman setup tadi — agent otomatis
   mendapatkan token asli dan menyimpannya secara lokal (`agent_token.json`, ganti lewat env `TOKEN_FILE`), jadi
   restart berikutnya langsung jalan tanpa pairing ulang.
5. Buka workspace di dashboard → tombol **Live View** → grid multiview (1x1/2x2/3x3) menampilkan tiap kamera
   sebagai tile HLS (lihat [apps/web/src/routes/workspaces.$workspaceId.live.tsx](apps/web/src/routes/workspaces.$workspaceId.live.tsx)).
   Agent men-push satu kali decode RTSP ke dua tujuan sekaligus (segmen lokal + live ke MediaMTX) lewat `ffmpeg`
   dua-output, jadi tidak ada beban pull RTSP tambahan ke kamera.

**Live view auth**: setiap publish/read ke MediaMTX divalidasi *per-request* lewat webhook
`/api/mediamtx/auth` ([apps/api/internal/handlers/mediamtx_auth_handler.go](apps/api/internal/handlers/mediamtx_auth_handler.go))
— bukan shared secret. Publish valid kalau password = `agent_token` milik site pemilik kamera; read valid kalau
password = JWT user yang memang anggota workspace kamera itu. Jadi tidak ada satu secret bocor yang membuka semua
kamera di semua workspace.

**Kontrol per-tile di Live View**: jeda/lanjut (lanjut otomatis lompat ke live edge, bukan mengejar dari titik
jeda), bisukan, snapshot ke PNG, Picture-in-Picture (kalau browser mendukung), dan toggle resolusi HD/SD. Toggle
resolusi butuh `local_rtsp_url_sub` diisi di kamera (RTSP sub-stream resolusi rendah dari kamera yang sama, khusus
live view — tidak pernah direkam, rekaman selalu pakai stream utama). Klik tile = fullscreen; panah keyboard/D-pad
pindah fokus antar tile (wrap ke halaman berikutnya di tepi grid), Enter/Space fullscreen tile yang fokus — pola
navigasinya sengaja meniru remote-control app EZVIZ TV resmi, jadi kalau nanti dibungkus jadi app Android TV,
navigasinya sudah langsung kompatibel tanpa kode tambahan.

Shortcut **Live View**/**Rekaman** selalu tersedia di header, dari halaman manapun (termasuk Admin) — otomatis
lompat ke workspace yang terakhir dibuka, atau tampilkan pilihan kalau ada beberapa workspace.

### Label "kamera - site" di rekaman

Tiap rekaman punya teks `<nama kamera> - <nama site>` yang di-burn-in langsung ke pixel video (pojok kiri atas,
kotak semi-transparan) — jadi tetap kebaca kalau file diunduh dan dibuka di luar aplikasi ini (bukan cuma overlay
UI browser yang tidak ikut file-nya). Ini **cuma di jalur rekaman**, live view tetap `-c copy` tanpa beban decode/
encode tambahan (lihat pertimbangannya di [recorder.go](apps/edge-agent/internal/recorder/recorder.go)).

Butuh ffmpeg yang di-build dengan `--enable-libfreetype` (untuk filter `drawtext`) — **tidak semua build punya
ini** (Homebrew ffmpeg standar di macOS misalnya tidak, dikonfirmasi langsung sesi ini). Kalau tidak ada, agent
otomatis fallback ke rekaman tanpa label (bukan berhenti merekam) dan mencatat satu baris log sekali saja. Cek:
```bash
ffmpeg -hide_banner -filters | grep drawtext
```
Kalau kosong tapi tetap mau pakai fitur ini, install ffmpeg dari sumber yang menyertakan freetype (build sendiri
dengan `--enable-libfreetype`, atau paket distro yang sudah menyertakannya — kebanyakan `apt install ffmpeg` di
Ubuntu/Debian sudah termasuk).

Kecepatan encode/kualitas bisa diatur lewat env `RECORDING_PRESET` (default `veryfast`) dan `RECORDING_CRF`
(default `23`, makin kecil makin bagus kualitasnya tapi makin berat).

### Hubungkan Google Drive (storage utama)

Tidak butuh service lokal apa pun (Docker maupun native) — cukup kredensial OAuth dari Google.

1. Di [Google Cloud Console](https://console.cloud.google.com/): buat project → aktifkan **Google Drive API** →
   buat **OAuth client ID** tipe "Web application" → set Authorized redirect URI ke
   `http://localhost:8080/api/oauth/google/callback` (atau domain API produksi kamu).
2. Isi `GOOGLE_OAUTH_CLIENT_ID` dan `GOOGLE_OAUTH_CLIENT_SECRET` di `.env` API, restart `apps/api`.
3. Di dashboard, workspace → tab **Storage** → **Hubungkan Google Drive** → login & consent lewat layar Google resmi.
   Refresh token otomatis tersimpan (dienkripsi, lihat di bawah) sebagai storage target baru — tidak perlu tempel
   token manual.
4. Pasangkan kamera ke storage target itu di tab **Kamera**. Edge agent akan upload tiap segmen selesai rekam ke
   folder Drive akun tersebut (lihat [apps/edge-agent/internal/uploader/gdrive.go](apps/edge-agent/internal/uploader/gdrive.go)).

### Rekaman: playback, seek, hapus

- Playback S3/MinIO redirect langsung ke presigned URL (tidak ada byte yang lewat server kita). Google Drive tidak
  punya presigned URL, jadi di-relay lewat API — termasuk meneruskan `Range` header dari browser
  ([storage/gdrive.go](apps/api/internal/storage/gdrive.go)). **Ini yang sebenarnya membuat rekaman bisa diputar
  sama sekali** — dibuktikan langsung: sebuah rekaman lama yang tadinya macet total di `readyState: 0` (sebelum
  dukungan Range ada) langsung terbaca metadatanya begitu Range aktif, meski moov atom-nya tetap di posisi asli
  (lihat poin faststart di bawah). Chrome ternyata bisa mengambil moov dari ujung file lewat Range request kedua,
  jadi Range sendiri sudah cukup — tidak wajib moov ada di depan.
- `-movflags +faststart` tetap dipasang di command ffmpeg agent ([recorder.go](apps/edge-agent/internal/recorder/recorder.go))
  sebagai praktik yang benar, tapi **catatan jujur dari pengecekan langsung**: flag ini rupanya tidak benar-benar
  memindahkan moov ke depan file saat dipakai bersama muxer `-f segment` (diverifikasi lewat inspeksi struktur box
  MP4 langsung pada rekaman asli — moov tetap muncul di akhir walau flag-nya sudah diset). Belum digali lebih jauh
  *kenapa* kombinasi itu tidak bekerja seperti pada remux satu-file biasa, karena toh Range sudah menutup dampaknya
  terhadap playability. Manfaat yang tersisa dari benar-benar merapikan moov murni soal performa (satu round-trip
  lebih sedikit saat load pertama), bukan soal rekaman bisa diputar atau tidak.
- `cmd/remux-backfill` — tool pemeliharaan (bisa dijalankan ulang kapan saja, aman/idempotent) yang mendeteksi
  posisi moov lewat pembacaan 64KB pertama file (bukan flag di database) dan merapikannya di tempat kalau perlu,
  lewat `Replacer` di [storage](apps/api/internal/storage) (S3: `PutObject` menimpa key yang sama; Drive:
  `Files.Update` menimpa isi file yang sama, ID-nya tidak berubah). Sudah diverifikasi ujung-ke-ujung pada rekaman
  Drive asli: berhasil merapikan, hasil tetap bisa diputar, dan dry-run berikutnya benar-benar melewatkannya
  (tidak diproses ulang).
  ```bash
  cd apps/api && go run ./cmd/remux-backfill --dry-run   # lihat dulu mana yang perlu
  cd apps/api && go run ./cmd/remux-backfill --limit=5   # coba beberapa dulu
  cd apps/api && go run ./cmd/remux-backfill             # proses semua
  ```
- Timestamp mulai/selesai tiap rekaman diambil dari nama file segment (`%Y%m%d-%H%M%S.mp4`), bukan waktu upload —
  durasi yang tampil di halaman Rekaman jadi akurat, bukan selalu "0 detik".
- Tombol **Hapus** di halaman Rekaman menghapus dari storage (Drive/S3/MinIO) sekaligus metadatanya — hanya workspace
  admin yang bisa.
- Struktur folder di Google Drive: `recordings/<nama kamera>/<YYYY-MM-DD>/<file>.mp4`, dibuat otomatis kalau belum
  ada. Root folder `recordings` bisa dioverride lewat `folder_id` di config storage target — dengan catatan scope
  OAuth `drive.file` yang dipakai hanya bisa melihat/mengelola folder yang **dibuat oleh app ini sendiri**, jadi
  folder pre-existing yang dibuat manual di Drive tidak akan kelihatan.
- **Diketahui belum beres**: properti `duration` internal file MP4 hasil segmen kadang salah total (mis. terbaca
  puluhan jam, bukan ~10 menit) — sudah dikonfirmasi ini bukan akibat `remux-backfill` (muncul juga di rekaman yang
  belum pernah disentuh tool itu). Tidak menghalangi pemutaran (konten & seek tetap benar, cuma metadata durasi
  totalnya yang salah), belum digali root cause-nya.

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
  healthcheck kalau pakai Opsi A).

## Deploy ke VPS

Dua-duanya cocok jalan di belakang **CloudPanel** (yang tidak punya tipe site "Go app" bawaan) — CloudPanel cuma
urus domain/TLS lewat reverse proxy, proses sebenarnya jalan sendiri di baliknya. Pilih salah satu:

### Opsi A — Docker Compose

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

### Opsi B — Tanpa Docker (native + systemd)

Cocok kalau VPS-nya sudah dikelola CloudPanel dengan MySQL sendiri (seperti yang kamu pakai) — tinggal jalankan
binary Go dan Node langsung, tanpa lapisan Docker sama sekali.

1. Build binary di VPS (atau build lokal lalu upload — `CGO_ENABLED=0` supaya statis, gampang dipindah antar mesin):
   ```bash
   cd apps/api && CGO_ENABLED=0 go build -o /opt/nvr/api ./cmd/api
   cd apps/edge-agent && CGO_ENABLED=0 go build -o /opt/nvr/agent ./cmd/agent   # hanya kalau site-nya juga di VPS ini
   cd apps/web && npm ci && npm run build   # hasil: apps/web/.output — jalankan dengan `node .output/server/index.mjs`
   ```
2. `apt install mariadb-server ffmpeg` (atau pakai MySQL yang sudah ada di CloudPanel), buat database & user seperti
   di langkah "Opsi B — tanpa Docker" bagian lokal di atas.
3. (Opsional, untuk live view) unduh binary MediaMTX dari [rilis GitHub-nya](https://github.com/bluenviron/mediamtx/releases)
   (satu file, tidak perlu dependency lain — tidak ada paket `apt` resmi), lalu jalankan dengan `infra/mediamtx.yml`.
   `authHTTPAddress` di config itu default-nya `http://api:8080/...` (nama service Docker) — kalau `api` jalan
   native di VPS yang sama, override dengan env var `MTX_AUTHHTTPADDRESS=http://localhost:8080/api/mediamtx/auth`
   seperti di langkah lokal di atas.
4. Jalankan `api`, `agent` (kalau perlu), dan `web` sebagai **systemd service** masing-masing (`ExecStart` ke
   binary/`node .output/server/index.mjs`, `EnvironmentFile` ke file `.env` versi produksi) supaya otomatis restart
   dan jalan saat boot.
5. CloudPanel: sama seperti Opsi A langkah 2–5 di atas (reverse proxy ke `127.0.0.1:8080` dan `127.0.0.1:3000`,
   buka port RTSP/HLS MediaMTX di firewall kalau dipakai, update `GOOGLE_OAUTH_REDIRECT_URL`/`WEB_BASE_URL`).

### Opsi C — Script otomatis (`scripts/deploy-central.sh`)

Membungkus Opsi B di atas jadi satu perintah: build `api` + `web`, tulis/perbarui systemd unit, `daemon-reload`,
`enable --now`, restart. Jalankan **di VPS itu sendiri**:

```bash
INSTALL_DIR=/opt/nvr RUN_USER=$(whoami) ./scripts/deploy-central.sh
```

Butuh sudo (menulis ke `/etc/systemd/system` dan `/opt`). Kalau `$INSTALL_DIR/.env` belum ada, script menyalin dari
`.env.example` dan **berhenti** — isi dulu dengan nilai produksi (JWT_SECRET, STORAGE_ENCRYPTION_KEY, MYSQL_DSN,
domain di GOOGLE_OAUTH_REDIRECT_URL/WEB_BASE_URL) sebelum jalankan ulang. Setelahnya, tinggal `git pull` lalu
jalankan script ini lagi tiap kali mau deploy update.

Ini hanya menangani `api` + `web` (service pusat di VPS) — **tidak** menyalakan edge agent, karena itu memang tidak
boleh jalan di VPS (lihat "Site vs Workspace" di atas).

### Edge agent di tiap lokasi (berlaku untuk semua opsi deploy)

Edge agent **selalu jalan di lokasi/gedung** masing-masing, di sebuah mesin yang menetap permanen di LAN kamera
(lihat rekomendasi hardware di bawah) — bukan di VPS, kecuali kameranya kebetulan satu jaringan dengan VPS.

**Cara tercepat**: `scripts/deploy-edge-agent.sh`, dijalankan di mesin site itu sendiri:

```bash
API_BASE_URL=https://api.domain-kamu.com ./scripts/deploy-edge-agent.sh
```

Build binary, pasang sebagai systemd service, **tanpa perlu `AGENT_TOKEN` sama sekali** — begitu service jalan
pertama kali, dia otomatis masuk mode pairing (lihat penjelasan "Alternatif tanpa copy-paste token" di bagian
"Uji coba edge agent" di atas): buat kode pairing di Admin → Sites & Kamera, buka halaman setup si mesin itu di
browser (alamatnya ada di `journalctl -u nvr-agent -f`), masukkan kode. Restart berikutnya langsung jalan normal.

Tidak perlu VPN, tidak perlu port terbuka ke internet di lokasi kamera — cuma butuh internet outbound biasa (dan,
untuk pairing pertama kali, satu perangkat lain di LAN yang sama untuk buka halaman setup-nya).

### Rekomendasi hardware untuk edge agent (mesin yang menetap 24/7 di tiap lokasi)

Mesin ini menjalankan ffmpeg terus-menerus (decode + 2 output) dan menyimpan buffer lokal sebelum ter-upload —
butuh sesuatu yang **stabil dijalankan 24/7**, bukan laptop kerja harian:

- **Mini PC** (Intel N100/N150 generasi terbaru, mis. Beelink/GMKtec/Minisforum kelas "N100 mini PC") — pilihan
  paling praktis: hemat daya (~6-10W idle), performa CPU cukup untuk `-c copy` (stream copy, bukan transcode, jadi
  ringan) di banyak kamera sekaligus, ada port Ethernet gigabit (**pakai kabel, bukan WiFi**, untuk stabilitas RTSP
  jangka panjang), dan umumnya sudah fanless/fan tenang cocok nyala terus. Pasang SSD (bukan cuma eMMC kecil) kalau
  mau simpan buffer lokal yang agak besar.
- **Raspberry Pi 4/5** (4GB RAM ke atas) — juga bisa, lebih murah, tapi dua catatan: (1) **jangan boot dari
  microSD** untuk beban tulis terus-menerus (buffer rekaman lokal = banyak write) — microSD cepat rusak/corrupt
  kena beban seperti ini; boot dari **SSD via USB 3** (Pi 4/5 mendukung ini resmi) jauh lebih tahan lama. (2) pakai
  **kabel Ethernet**, bukan WiFi bawaan Pi, untuk RTSP yang stabil.
- **Hindari**: laptop/PC harian yang berpindah jaringan (persis kasus yang barusan kita alami — begitu jaringan
  laptop berubah, edge agent kehilangan akses ke kamera), dan microSD sebagai satu-satunya storage untuk beban
  tulis 24/7.
- **Opsional tapi disarankan**: UPS kecil (mis. UPS USB untuk router/ONT) supaya mesin ini tidak mati mendadak saat
  listrik padam — restart otomatis (systemd `Restart=on-failure` sudah di-set oleh `deploy-edge-agent.sh`) begitu
  listrik/mesin nyala lagi, tanpa perlu pairing ulang (token sudah tersimpan di `agent_token.json`).

### Penyimpanan eksternal (HDD) untuk buffer rekaman lokal

`RECORD_DIR` (default `./recordings`) cuma path filesystem biasa — arahkan ke HDD/SSD eksternal yang di-mount kalau
storage internal mesin edge agent-nya kecil:

```bash
# Linux: mount HDD eksternal permanen lewat /etc/fstab, lalu:
RECORD_DIR=/mnt/hdd-eksternal/recordings
```

Ini **hanya buffer lokal sementara** — tiap segmen otomatis terhapus dari `RECORD_DIR` begitu berhasil ter-upload ke
storage utama (Google Drive/S3/MinIO), lihat [recorder.go](apps/edge-agent/internal/recorder/recorder.go). Jadi HDD
eksternal di sini fungsinya sebagai "jaring pengaman" kalau upload sempat gagal/lambat (mis. internet lokasi lambat
sementara), bukan penyimpanan permanen jangka panjang — penyimpanan permanennya tetap di storage target yang
dikonfigurasi per-kamera (Drive/S3/MinIO), yang punya retention policy sendiri (`retain_days`). Kalau mau tetap
simpan salinan lokal jangka panjang di HDD itu juga (di luar apa yang sistem ini kelola), itu di luar cakupan
aplikasi — perlu proses backup terpisah yang membaca dari storage target, bukan dari `RECORD_DIR`.

## Belum diimplementasikan

- **EZVIZ Cloud API fallback** — tidak dikerjakan (butuh langganan EZVIZ Open Platform berbayar untuk kuota yang
  layak). Agent hanya mendukung `local_rtsp_url` (RTSP lokal, gratis, tanpa batas kuota) — cukup selama kamera EZVIZ
  diaktifkan RTSP lokalnya di app EZVIZ.
- Audit log & notification channel masih terbatas untuk aksi yang mengubah data lewat dashboard/API (login gagal
  sekarang sudah tercatat — lihat "Sudah selesai"); **perubahan lewat query DB langsung tidak bisa diaudit dari
  kode aplikasi sama sekali** — itu butuh binlog/trigger di level database (proyek ops terpisah), bukan sesuatu
  yang bisa "diselesaikan" di sisi app.
- **App Android TV (WebView)** — sengaja ditunda sampai versi web ini benar-benar settle. Satu syarat yang sudah
  disepakati untuk versi itu nanti: **kiosk mode** (auto-login, langsung buka Live View saat boot, tanpa layar
  login tiap kali TV nyala ulang) — navigasi keyboard/D-pad di Live View sekarang sudah dirancang kompatibel untuk
  itu tanpa kode tambahan.
- **Durasi internal rekaman kadang salah** — lihat catatan di bagian "Rekaman" di atas; belum digali root cause-nya
  (playback & seek tetap benar, cuma metadata durasi total yang keliru).

## Sudah selesai

- Workspace multi-tenant, RBAC (superadmin/workspace admin/viewer), edge agent outbound-only multi-lokasi
- **Site vs Workspace dipisah tegas**: site menentukan siapa yang merekam (wajib satu LAN dengan kamera), workspace
  menentukan siapa yang boleh melihat (lintas site bebas) — kamera bisa dipindah antar site tanpa kehilangan
  riwayat rekaman (Admin → Sites & Kamera → "Pindahkan ke site...")
- **Onboarding site baru tanpa CLI**: kode pairing sekali pakai (15 menit) yang ditukar lewat halaman setup lokal
  milik edge agent itu sendiri — tidak perlu copy-paste token ke `.env` secara manual. Halaman setup itu juga bisa
  ditemukan lewat `http://nvr-agent.local:<port>` (mDNS) di jaringan yang mendukungnya, tanpa perlu tahu IP mesin
  ([apps/edge-agent/internal/pairing](apps/edge-agent/internal/pairing))
- Live view multiview (MediaMTX + grid 1x1 s/d 5x5), auth per-request tanpa shared secret, navigasi keyboard/D-pad
  penuh, kontrol per-tile (jeda, bisukan, snapshot, PiP, toggle resolusi HD/SD lewat sub-stream)
- **Playback rekaman** dari Drive/S3/MinIO dengan dukungan seek (HTTP Range — ini yang benar-benar membuat rekaman
  bisa diputar, lihat bagian "Rekaman"), timestamp mulai/selesai akurat, hapus rekaman per-item, dan
  `cmd/remux-backfill` untuk merapikan posisi moov atom rekaman lama (optimisasi, bukan syarat playability)
- Google Drive "Connect" via OAuth resmi (storage utama) + fallback S3/MinIO manual, folder rekaman terstruktur
  `recordings/<kamera>/<tanggal>/<file>`
- Enkripsi kredensial storage at rest (AES-256-GCM)
- Retention/cleanup job otomatis (Drive/S3/MinIO + metadata)
- Picker pencarian untuk assign kamera ke workspace & tambah anggota (superadmin), dengan navigasi keyboard penuh
- Notifikasi webhook (kamera offline, upload gagal) + audit log (termasuk percobaan login gagal) + health/status tab
- **Aksesibilitas WCAG 2.1 AA**: skip-link, landmark semantik, kontras warna, label form, pola ARIA untuk tab/menu
  dropdown/combobox, dan grid Live View yang benar-benar bisa dinavigasi keyboard (bukan cuma ring visual)
- Shortcut navigasi global (Live View/Rekaman) yang selalu terlihat dari halaman manapun
- Jalan tanpa Docker sama sekali (MySQL/MediaMTX/MinIO native lewat Homebrew, atau `apt` di Linux), plus script
  otomatis untuk dev lokal (`scripts/dev.sh`) dan deploy produksi (`scripts/deploy-central.sh`,
  `scripts/deploy-edge-agent.sh`)
