# EnylviaDownloader v2.4 Pro

Aplikasi **Desktop Download Manager** mandiri berperforma tinggi berbasis **Go (Golang)** yang dikompilasi menjadi satu biner executable (`EnylviaDownloader.exe`). Menampilkan antarmuka desktop modern bergaya cyberpunk/dark mode dengan visualisasi segmen koneksi real-time, grafik throughput jaringan dinamis, integrasi System Tray (xtray), dan integrasi khusus **Microsoft Edge**.

---

## ✨ Fitur Utama

* **Aplikasi Desktop Mandiri (Standalone Window):**
  * Tampil sebagai jendela desktop mandiri tanpa address bar atau tab browser.
  * Dilengkapi tombol navigasi window kustom: Minimize (`—`), Maximize (`▢`), dan Tutup (`✕`).
* **Integrasi System Tray (Xtray):**
  * Berjalan senyap di latar belakang (*system notification area* di sebelah jam Windows).
  * Menu klik kanan System Tray: *Buka Aplikasi, Lanjutkan Semua, Jeda Semua, Buka Folder Unduhan, Auto-Start Startup, dan Keluar*.
  * Ketika tombol tutup (`✕`) diklik, aplikasi tetap aktif mengunduh di system tray tanpa terputus.
* **Auto-Start Windows Booting:**
  * Opsi otomatis berjalan saat komputer pertama kali dinyalakan dan langsung terminimalkan ke system tray tanpa mengganggu layar.
* **Visualisasi Alokasi Blok Multi-Thread (Gaya Gambar 1):**
  * Grid alokasi segmen `S#01` hingga `S#16` yang memperlihatkan progres tiap koneksi HTTP Range secara visual.
* **Grafik Gelombang Throughput Real-Time:**
  * Animasi kurva gelombang (*wave chart*) neon cyan 60fps untuk memantau fluktuasi kecepatan transfer data langsung.
* **Widget Kapasitas Drive Lokal:**
  * Memantau penggunaan disk (misal: `NVMe SSD (D:)`) lengkap dengan kapasitas terpakai dan total dalam GB/TB.
* **Integrasi Khusus Microsoft Edge:**
  * Mendukung ekstensi Microsoft Edge (Manifest V3) untuk menangkap link unduhan otomatis dan via menu klik kanan (*context menu*).
  * Pendaftaran *Native Messaging Host* otomatis ke registry Windows.

---

## 🚀 Panduan Cepat (Untuk Pengguna Baru / Setelah Clone)

Hanya perlu **2 langkah mudah** agar aplikasi dan integrasi browser langsung berjalan 100%:

### Langkah 1: Jalankan Aplikasi
Cukup klik ganda (double-click) file:
```
EnylviaDownloader.exe
```
* Jendela desktop cyberpunk EnylviaDownloader akan langsung terbuka.
* Secara otomatis, aplikasi akan mendaftarkan *Native Messaging Host* ke registry Windows Anda secara senyap (tanpa perlu membuka CMD/Terminal).
* Aplikasi juga otomatis aktif di System Tray (dekat jam Windows).

---

### Langkah 2: Pasang Ekstensi di Microsoft Edge
1. Buka browser **Microsoft Edge**, lalu ketik di address bar: `edge://extensions`
2. Aktifkan sakelar **Mode Pengembang (*Developer mode*)** di panel kiri bawah.
3. Klik tombol **Muat ekstensi yang belum dibongkar (*Load unpacked*)**.
4. Pilih folder `extension` yang ada di dalam repositori hasil clone Anda:
   ```
   <folder-clone-anda>\extension
   ```
5. **Selesai!** Ekstensi akan otomatis terhubung ke EnylviaDownloader (indikator di dashboard akan berubah menjadi hijau *"Live (Edge)"*). Setiap unduhan di Edge kini otomatis ditangkap oleh EnylviaDownloader.

---

## 🛠️ Build dari Source (Untuk Developer / Pengguna Go)

Jika Anda ingin mengompilasi ulang kode sumber Go sendiri:

### Cara Praktis:
Jalankan file batch otomatis:
```cmd
build.bat
```

### Cara Manual via Terminal:
```powershell
go build -ldflags="-H=windowsgui -s -w" -o EnylviaDownloader.exe .
```
> **Catatan Penting:** Flag `-H=windowsgui` wajib disertakan agar aplikasi berjalan murni sebagai aplikasi GUI Windows tanpa memunculkan jendela console/terminal hitam saat dijalankan.

---

## ⚙️ Opsi Perintah Tambahan (CLI)

Aplikasi juga mendukung flag opsional jika dijalankan via terminal:

* **Mulai langsung di System Tray (Minimize):**
  ```powershell
  .\EnylviaDownloader.exe --minimized
  ```
* **Aktifkan Auto-Start saat Windows Booting:**
  ```powershell
  .\EnylviaDownloader.exe -install-autostart
  ```
* **Nonaktifkan Auto-Start:**
  ```powershell
  .\EnylviaDownloader.exe -uninstall-autostart
  ```
*(Pengaturan ini juga dapat diubah kapan saja langsung lewat menu klik kanan ikon System Tray atau Pengaturan di dalam aplikasi).*
