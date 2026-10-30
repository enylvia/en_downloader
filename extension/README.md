# EnylviaDownloader - Ekstensi Microsoft Edge

Ekstensi ini mengintegrasikan browser Microsoft Edge dengan aplikasi desktop **EnylviaDownloader** untuk menangkap file unduhan secara otomatis (mirip IDM) maupun lewat menu klik kanan (*context menu*).

## Extension ID Tetap
- ID: `leeinjkmocmbogffanjegflpmbfmhogj` (ditentukan otomatis via RSA public key di `manifest.json`)
- Native Messaging Host: `com.enylvia.downloader`

## Cara Memasang / Memperbarui di Microsoft Edge:
1. Buka browser **Microsoft Edge**.
2. Buka URL: `edge://extensions` pada address bar.
3. Aktifkan sakelar **Mode pengembang (Developer mode)** di sebelah kiri bawah.
4. Jika ekstensi lama sudah terpasang:
   - Klik ikon **Muat Ulang / Reload (panah melingkar)** pada kartu ekstensi EnylviaDownloader, atau klik **Hapus (Remove)** lalu pasang ulang.
5. Jika memasang baru:
   - Klik tombol **Muat ekstensi yang belum dibongkar (Load unpacked)** di bagian atas.
   - Arahkan ke folder `extension` di dalam folder project EnylviaDownloader hasil clone Anda.
6. Pastikan status ekstensi menyala (Aktif).
7. Sekarang, setiap kali Anda mengunduh file apa pun di Microsoft Edge atau klik kanan tautan > "Download with EnylviaDownloader", unduhan akan langsung masuk ke aplikasi EnylviaDownloader dan jendela aplikasi akan langsung terbuka menampilkan proses pengunduhan secara *real-time*.
