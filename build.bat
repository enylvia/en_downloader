@echo off
setlocal
echo ========================================================
echo   EnylviaDownloader - Build Script (GUI Executable)
echo ========================================================
echo.
echo Mengompilasi EnylviaDownloader.exe...
echo Command: go build -ldflags="-H=windowsgui -s -w" -o EnylviaDownloader.exe .
echo.

go build -ldflags="-H=windowsgui -s -w" -o EnylviaDownloader.exe .

if %ERRORLEVEL% EQU 0 (
    echo.
    echo ========================================================
    echo   [BERHASIL] EnylviaDownloader.exe siap digunakan!
    echo   Aplikasi berjalan murni GUI tanpa console hitam.
    echo ========================================================
) else (
    echo.
    echo [GAGAL] Kompilasi gagal. Pastikan compiler Go (Golang) sudah terpasang.
)
echo.
pause
