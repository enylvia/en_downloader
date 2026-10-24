// EnylviaDownloader - Smart Click & Download Interceptor
// Intercepts downloadable file links directly from web pages (IDM-style)

const DOWNLOAD_EXTENSIONS = new Set([
  // Archives
  "zip", "rar", "7z", "tar", "gz", "bz2", "xz", "iso", "img", "bin",
  // Executables & Installers
  "exe", "msi", "apk", "dmg", "pkg", "deb", "rpm", "appimage",
  // Video & Audio Media
  "mp4", "mkv", "avi", "mov", "wmv", "flv", "webm", "mp3", "flac", "wav", "aac", "m4a",
  // Documents & eBooks
  "pdf", "epub", "djvu"
]);

document.addEventListener("click", (e) => {
  // If user holds Alt key, bypass interception (standard IDM behavior)
  if (e.altKey) return;

  const anchor = e.target.closest("a");
  if (!anchor || !anchor.href) return;

  const href = anchor.href;
  if (!href.startsWith("http://") && !href.startsWith("https://")) return;

  let isDownloadable = false;
  let filename = anchor.getAttribute("download") || "";

  try {
    const parsed = new URL(href);
    const pathParts = parsed.pathname.split("/").filter(Boolean);
    if (pathParts.length > 0) {
      const lastSegment = pathParts[pathParts.length - 1];
      const dotIdx = lastSegment.lastIndexOf(".");
      if (dotIdx !== -1) {
        const ext = lastSegment.substring(dotIdx + 1).toLowerCase();
        if (DOWNLOAD_EXTENSIONS.has(ext)) {
          isDownloadable = true;
          if (!filename) filename = decodeURIComponent(lastSegment);
        }
      }
    }
  } catch (err) {}

  if (anchor.hasAttribute("download")) {
    isDownloadable = true;
  }

  if (isDownloadable) {
    if (anchor.target === "_blank") {
      anchor.target = "_self";
    }
    e.preventDefault();
    e.stopPropagation();
    e.stopImmediatePropagation();

    showOnPageToast(`Mengalihkan unduhan ${filename || "file"} ke EnylviaDownloader...`);

    try {
      chrome.runtime.sendMessage({
        action: "download",
        url: href,
        filename: filename,
        referrer: window.location.href
      });
    } catch (err) {
      console.warn("Failed to message EnylviaDownloader extension:", err);
    }
  }
}, true);

function showOnPageToast(msg) {
  const existing = document.getElementById("enylvia-page-toast");
  if (existing) existing.remove();

  const toast = document.createElement("div");
  toast.id = "enylvia-page-toast";
  toast.innerText = msg;
  toast.style.cssText = `
    position: fixed;
    bottom: 24px;
    right: 24px;
    background: #0d1424;
    color: #22d3ee;
    padding: 12px 20px;
    border-radius: 8px;
    border: 1px solid #0284c7;
    box-shadow: 0 10px 25px rgba(0,0,0,0.6);
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    font-size: 13px;
    font-weight: 600;
    z-index: 2147483647;
    pointer-events: none;
    transition: opacity 0.3s ease;
  `;
  document.body.appendChild(toast);
  setTimeout(() => {
    toast.style.opacity = "0";
    setTimeout(() => toast.remove(), 300);
  }, 3500);
}
