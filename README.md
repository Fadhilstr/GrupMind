# 🤖 AI Agent Telegram Bot & Pengingat Absensi Monev Otomatis

Aplikasi AI Agent Telegram berbasis **Go (Golang)** dan **Google Gemini API** yang berjalan otomatis, terhubung ke grup Telegram tim, mampu menganalisis jalannya percakapan, mengekstrak kesimpulan serta tugas, mencatat riwayat perubahan, dan secara konsisten mengirimkan **Pengingat Absensi Monev** langsung ke grup Telegram yang telah diverifikasi pada jadwal tetap.

---

## 🌟 Fitur Utama

1. **🔔 Pengingat Absensi Monev Terjadwal di Grup (Fitur Wajib)**
   - **Jadwal Pengingat:**
     - **Senin – Sabtu pukul 15.00 WIB** (Pengingat awal pengisian).
     - **Senin – Sabtu pukul 20.00 WIB** (Pengingat kedua / evaluasi sebelum hari berakhir).
     - **Hari Minggu libur total** (tidak ada pengingat yang dikirim).
   - **Tujuan Pengiriman:** Dikirim langsung ke **grup Telegram** yang ditentukan dan diizinkan, bukan ke chat pribadi.
   - **Zona Waktu:** Wajib dan konsisten menggunakan `Asia/Jakarta` (WIB).
   - **Idempotensi & Pencegahan Duplikasi:** Kunci unik `(group_id, reminder_date, slot)` di PostgreSQL menjamin pesan tidak pernah terkirim ganda.
   - **Pemulihan Setelah Restart (Recovery Policy):** Jika server mati atau direstart, scheduler memeriksa apakah restart terjadi dalam batas toleransi (*grace period* 30 menit). Jika masih dalam toleransi, pengingat segera dikirim. Jika sudah lewat jauh, pengingat ditandai *missed* untuk mencegah spam pesan basi ke grup.

2. **🧠 Analisis Percakapan Grup Otomatis (Gemini AI)**
   - Menggunakan Google GenAI SDK resmi untuk Go (`google.golang.org/genai`).
   - Menganalisis alur diskusi multi-pesan (menghubungkan pesan-pesan terkait).
   - *Structured Output* dengan JSON Schema tervalidasi.
   - Mengekstrak:
     - Topik utama diskusi.
     - Keputusan final vs usulan (dibedakan secara tegas).
     - Tugas baru dan penanggung jawab (*assignee*).
     - Tenggat waktu (*deadline*) dengan interpretasi tanggal relatif ("besok", "lusa", "Jumat sore") berbasis kalender `Asia/Jakarta`.
     - Agenda dan jadwal rapat.
     - Pertanyaan yang belum terjawab.
   - **Keamanan:** Memfilter input teks grup sebagai data tidak tepercaya untuk mencegah *prompt injection*.

3. **📋 Manajemen Tugas, Jadwal, & Keputusan**
   - Melihat daftar seluruh tugas tim (`/tasks`), tugas hari ini (`/today`), dan agenda mendatang (`/upcoming`).
   - Menyelesaikan tugas dengan validasi (`/done <ID_TUGAS>`).
   - Audit trail lengkap mencatat riwayat perubahan status dan tenggat waktu tugas.
   - Melihat daftar keputusan penting hasil rapat (`/decisions`).

4. **📊 Rangkuman Harian Percakapan Otomatis**
   - Merangkum percakapan grup harian secara berkala (default pukul 21:00 WIB) atau sesuai permintaan (`/summary`).
   - Memisahkan fakta diskusi dari interpretasi AI.

5. **🛡️ Otorisasi & Keamanan Grup**
   - Hanya memproses grup yang terdaftar pada konfigurasi atau disetujui admin.
   - Perintah sensitif (`/setgroup`, `/testreminder`) hanya dapat dijalankan oleh Admin.
   - Tidak pernah mengekspos token atau rahasia ke dalam log maupun respons pengguna.

---

## 🏗️ Struktur Direktori Proyek

```
botteleabsensi/
├── cmd/
│   └── bot/
│       └── main.go                  # Entrypoint aplikasi, koneksi, & lifecycle
├── internal/
│   ├── config/                      # Konfigurasi & parsing environment variables
│   │   ├── config.go
│   │   └── config_test.go
│   ├── database/                    # Koneksi PostgreSQL & embedded migrations
│   │   ├── postgres.go
│   │   ├── migrations.go
│   │   └── migrations/
│   │       └── 000001_init_schema.up.sql
│   ├── gemini/                      # Klien Gemini AI & ekstraksi percakapan
│   │   ├── analyzer.go
│   │   ├── schema.go
│   │   ├── mock_analyzer.go
│   │   └── analyzer_test.go
│   ├── handler/                     # Handler perintah dan routing bot Telegram
│   │   ├── handler.go
│   │   └── handler_test.go
│   ├── model/                       # Definisi domain struct & enum
│   │   └── models.go
│   ├── repository/                  # Akses database PostgreSQL & in-memory mock
│   │   ├── interfaces.go
│   │   ├── postgres_impl.go
│   │   ├── mock_impl.go
│   │   └── mock_impl_test.go
│   ├── scheduler/                   # Penjadwal otomatis Monev & rangkuman
│   │   ├── scheduler.go
│   │   └── scheduler_test.go
│   ├── service/                     # Logika bisnis Monev & Percakapan
│   │   ├── monev_service.go
│   │   ├── monev_service_test.go
│   │   └── conversation_service.go
│   └── telegram/                    # Telegram Bot API client & types
│       ├── types.go
│       ├── client.go
│       └── client_test.go
├── test/
│   └── integration_test.go          # Pengujian integrasi alur lengkap (E2E)
├── Dockerfile                       # Multi-stage Docker build
├── compose.yaml                     # Konfigurasi Docker Compose (App + PostgreSQL)
├── .env.example                     # Contoh konfigurasi environment
├── go.mod
├── go.sum
└── README.md
```

---

## ⚙️ Variabel Lingkungan (.env)

Salin `.env.example` menjadi `.env`:
```bash
cp .env.example .env
```

| Variabel | Tipe / Default | Keterangan |
|---|---|---|
| `APP_PORT` | `8080` | Port HTTP untuk health check (`/healthz`) |
| `TELEGRAM_BOT_TOKEN` | *Wajib* | Token bot Telegram dari `@BotFather` |
| `TELEGRAM_ALLOWED_GROUPS` | String (koma) | ID grup yang diizinkan (contoh: `-1001234567890`) |
| `TELEGRAM_ADMIN_USER_IDS` | String (koma) | ID user Telegram yang bertindak sebagai Admin |
| `TELEGRAM_MONEV_TARGET_GROUP_ID` | Angka | ID grup target utama pengingat Monev otomatis |
| `GEMINI_API_KEY` | *Wajib* | API Key Google Gemini dari Google AI Studio |
| `GEMINI_MODEL` | `gemini-2.5-flash` | Model Gemini yang digunakan |
| `DATABASE_HOST` | `localhost` / `postgres` | Host database PostgreSQL |
| `DATABASE_PORT` | `5432` | Port database PostgreSQL |
| `DATABASE_USER` | `bottele` | Username PostgreSQL |
| `DATABASE_PASSWORD` | `secretpassword` | Password PostgreSQL |
| `DATABASE_NAME` | `botteleabsensi` | Nama database |
| `DATABASE_SSLMODE` | `disable` | SSL mode database |
| `TIMEZONE` | `Asia/Jakarta` | Zona waktu wajib pengingat Monev |
| `MONEV_REMINDER_TIME_1` | `15:00` | Jam pengingat awal (Senin–Sabtu) |
| `MONEV_REMINDER_TIME_2` | `20:00` | Jam pengingat kedua (Senin–Sabtu) |
| `MONEV_RECOVERY_GRACE_MINUTES` | `30` | Toleransi pemulihan jadwal restart (menit) |
| `DAILY_SUMMARY_TIME` | `21:00` | Jadwal rangkuman percakapan harian otomatis |

---

## 🚀 Panduan Setup & Menjalankan Aplikasi

### 1. Membuat Bot di Telegram (@BotFather)
1. Buka Telegram dan cari akun resmi `@BotFather`.
2. Kirim perintah `/newbot` dan ikuti instruksi untuk memberi nama dan username bot.
3. Simpan **Bot Token** yang diberikan, lalu masukkan ke dalam variabel `TELEGRAM_BOT_TOKEN` di `.env`.
4. **Penting (Group Privacy):**
   - Secara default, bot Telegram hanya menerima pesan yang me-mention bot atau berupa perintah.
   - Agar bot dapat membaca seluruh pesan diskusi tim untuk dianalisis oleh AI:
     - Kirim `/setprivacy` ke `@BotFather`.
     - Pilih bot Anda, lalu pilih **Disable**.
     - Atau alternatifnya, jadikan bot sebagai **Administrator** di dalam grup Telegram tim Anda.

### 2. Mengundang Bot ke Grup & Menentukan Target Pengingat
1. Buat grup Telegram atau buka grup tim Anda yang sudah ada.
2. Tambahkan bot ke dalam grup tersebut.
3. Di dalam grup, ketik perintah:
   ```text
   /groupid
   ```
   Bot akan membalas dengan menampilkan ID Grup (misalnya: `-1002345678901`).
4. Untuk menetapkan grup tersebut sebagai target pengingat Monev resmi, admin cukup mengetik:
   ```text
   /setgroup
   ```
   *Atau* Anda dapat menyalin ID grup tersebut ke dalam `TELEGRAM_MONEV_TARGET_GROUP_ID` di file `.env`.
5. Uji koneksi pengiriman pesan pengingat dengan mengetik:
   ```text
   /testreminder
   ```
   Bot akan langsung mengirimkan pesan simulasi pengingat Monev ke grup.

### 3. Memperoleh Google Gemini API Key
1. Buka [Google AI Studio](https://aistudio.google.com/).
2. Masuk menggunakan akun Google Anda.
3. Klik tombol **Get API key**, lalu buat kunci baru (**Create API key**).
4. Salin API key dan tempelkan ke `GEMINI_API_KEY` di file `.env`.

---

## 💻 Menjalankan Aplikasi

### Cara 1: Menggunakan Docker Compose (Direkomendasikan untuk Deployment)

Pastikan Docker & Docker Compose sudah terpasang, lalu jalankan:

```bash
# 1. Pastikan file .env sudah siap
cp .env.example .env
nano .env

# 2. Jalankan PostgreSQL dan Aplikasi secara otomatis
docker compose up -d --build

# 3. Periksa status kontainer
docker compose ps

# 4. Lihat log aplikasi real-time
docker compose logs -f app
```

Container aplikasi akan menunggu PostgreSQL sehat (*healthy*) terlebih dahulu, lalu secara otomatis menjalankan seluruh migrasi skema database sebelum memulai scheduler dan polling Telegram.

### Cara 2: Menjalankan Secara Lokal (Pengembangan)

1. Pastikan PostgreSQL lokal sedang berjalan dan database `botteleabsensi` telah dibuat:
   ```bash
   createdb -U postgres botteleabsensi
   ```
2. Jalankan aplikasi menggunakan Go:
   ```bash
   go run ./cmd/bot
   ```

---

## 🧪 Menjalankan Pengujian Otomatis

Seluruh pengujian unit dan integrasi dapat dijalankan dengan satu perintah:

```bash
go test -v ./...
```

Pengujian mencakup:
- ✅ Konfigurasi & validasi zona waktu `Asia/Jakarta`.
- ✅ Pengingat Senin–Sabtu pukul 15.00 WIB & 20.00 WIB.
- ✅ Pengecualian mutlak pengingat pada hari Minggu.
- ✅ Pengiriman pengingat Monev langsung ke grup (bukan chat pribadi).
- ✅ Pencegahan pesan ganda (*idempotency log*).
- ✅ Pemulihan jadwal setelah restart (*recovery within grace period* vs *missed*).
- ✅ Penanganan kegagalan Telegram API (*retry, backoff, no false success*).
- ✅ Ekstraksi percakapan Gemini AI multi-pesan (tugas, PIC, deadline, keputusan).
- ✅ Otorisasi perintah sensitif (hanya admin yang dapat menjalankan `/setgroup` dan `/testreminder`).
- ✅ Penolakan grup yang tidak terdaftar.

---

## 📜 Daftar Perintah Bot Telegram

| Perintah | Otorisasi | Keterangan |
|---|---|---|
| `/start` | Semua | Memulai bot dan menampilkan ringkasan fitur |
| `/help` | Semua | Menampilkan panduan lengkap penggunaan perintah |
| `/groupid` | Semua | Menampilkan ID chat saat ini dan status target Monev |
| `/monev` | Semua | Menampilkan jadwal dan status pengingat Monev hari ini |
| `/tasks` | Anggota Grup | Menampilkan daftar seluruh tugas aktif tim |
| `/today` | Anggota Grup | Menampilkan tugas dan jadwal untuk hari ini |
| `/upcoming` | Anggota Grup | Menampilkan tugas dan jadwal agenda mendatang |
| `/decisions` | Anggota Grup | Menampilkan daftar keputusan dan kesimpulan diskusi tim |
| `/done <ID>` | Anggota Grup | Menandai tugas tertentu sebagai selesai |
| `/summary` | Anggota Grup | Meminta rangkuman percakapan harian terbaru |
| `/setgroup` | **Admin** | Menetapkan grup saat ini sebagai target pengingat Monev |
| `/testreminder` | **Admin** | Menguji pengiriman pengingat Monev langsung ke grup |
| `/status` | Semua | Menampilkan uptime, status database, AI, dan jadwal berikutnya |

---

## 🔒 Catatan Keamanan & Kepatuhan

- **Data Tidak Tepercaya:** Seluruh pesan dari grup diperlakukan sebagai input teks yang tidak tepercaya. AI prompt dilengkapi proteksi instruksi sistem untuk mencegah *prompt injection*.
- **Tanpa Kredensial Hardcoded:** Seluruh token rahasia, API key, dan kata sandi disimpan di `.env` yang terdaftar pada `.gitignore`.
- **Eksekusi Aman:** Bot tidak mengeksekusi perintah shell yang dihasilkan AI dan tidak memberikan akses langsung database kepada model AI.
- **Redaksi Log:** Token Telegram otomatis disamarkan (*redacted*) dari pesan log dan error.


