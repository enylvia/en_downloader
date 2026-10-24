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

## 🚀 Cara Menjalankan

### 1. Menjalankan Biasa (Membuka Jendela Desktop):
```powershell
.\EnylviaDownloader.exe
```
Jendela aplikasi desktop mandiri akan langsung muncul di layar Anda.

### 2. Menjalankan Langsung Minimize ke System Tray:
```powershell
.\EnylviaDownloader.exe --minimized
```
atau
```powershell
.\EnylviaDownloader.exe --tray
```
Aplikasi akan langsung masuk ke System Tray di pojok kanan bawah tanpa membuka jendela.

### 3. Mengatur Auto-Start Windows Startup:
* **Aktifkan:**
  ```powershell
  .\EnylviaDownloader.exe -install-autostart
  ```
* **Nonaktifkan:**
  ```powershell
  .\EnylviaDownloader.exe -uninstall-autostart
  ```
*(Atau dapat diaktifkan/dinonaktifkan langsung lewat menu klik kanan ikon System Tray atau Pengaturan Aplikasi).*

### 4. Mendaftarkan Integrasi Microsoft Edge:
```powershell
.\EnylviaDownloader.exe -install-edge
```

---

## 🌐 Cara Memasang Ekstensi di Microsoft Edge:
1. Buka browser **Microsoft Edge** dan buka: `edge://extensions`
2. Aktifkan **Mode Pengembang (*Developer mode*)** di panel kiri bawah.
3. Klik tombol **Load unpacked** (Muat ekstensi yang belum dibongkar).
4. Pilih folder:
   ```
   D:\Project  Adit\EnylviaDownloader\extension
   ```
5. Ekstensi EnylviaDownloader kini aktif di Microsoft Edge Anda!
