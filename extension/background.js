const NATIVE_HOST = "com.enylvia.downloader";
const API_URL = "http://127.0.0.1:4242/api/tasks/add";
const PING_URL = "http://127.0.0.1:4242/api/edge/ping";

let interceptEnabled = true;
const recentlyIntercepted = new Set();

// Synchronize intercept setting immediately
chrome.storage.local.get(["interceptEnabled"], (res) => {
  if (res && res.interceptEnabled !== undefined) {
    interceptEnabled = res.interceptEnabled;
  } else {
    chrome.storage.local.set({ interceptEnabled: true });
  }
});

chrome.storage.onChanged.addListener((changes, area) => {
  if (area === "local" && changes.interceptEnabled !== undefined) {
    interceptEnabled = changes.interceptEnabled.newValue;
  }
});

// Heartbeat ping to EnylviaDownloader server
async function pingServer() {
  try {
    await fetch(PING_URL, { method: "POST" });
  } catch (e) {}
}
pingServer();
setInterval(pingServer, 10000);

// Listen to messages from content script
chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  if (msg && msg.action === "download" && msg.url) {
    sendDownloadToEnylvia(msg.url, msg.filename || "", msg.referrer || "");
    sendResponse({ ok: true });
  }
});

// Setup context menus
chrome.runtime.onInstalled.addListener(() => {
  chrome.contextMenus.removeAll(() => {
    chrome.contextMenus.create({
      id: "enylvia-download-link",
      title: "Download with EnylviaDownloader",
      contexts: ["link"]
    });

    chrome.contextMenus.create({
      id: "enylvia-download-media",
      title: "Download media with EnylviaDownloader",
      contexts: ["image", "video", "audio"]
    });
  });
});

chrome.contextMenus.onClicked.addListener((info, tab) => {
  let targetUrl = "";
  if (info.menuItemId === "enylvia-download-link" && info.linkUrl) {
    targetUrl = info.linkUrl;
  } else if (info.menuItemId === "enylvia-download-media" && info.srcUrl) {
    targetUrl = info.srcUrl;
  }

  if (targetUrl) {
    sendDownloadToEnylvia(targetUrl, "", tab ? tab.url : "");
  }
});

function extractFilename(item, url) {
  let name = (item && item.filename) ? item.filename : "";
  if (name.includes("\\")) {
    name = name.substring(name.lastIndexOf("\\") + 1);
  } else if (name.includes("/")) {
    name = name.substring(name.lastIndexOf("/") + 1);
  }

  if (!name || name === "download") {
    try {
      const u = new URL(url);
      const parts = u.pathname.split("/").filter(Boolean);
      if (parts.length > 0) {
        name = decodeURIComponent(parts[parts.length - 1]);
      }
    } catch (e) {}
  }
  return name;
}

function closeTabIfBlankDownload(url) {
  try {
    chrome.tabs.query({}, (tabs) => {
      if (!tabs) return;
      for (const t of tabs) {
        if (t.url === url || t.pendingUrl === url) {
          chrome.tabs.remove(t.id);
        }
      }
    });
  } catch (e) {}
}

// 1. Primary Interceptor: onCreated
chrome.downloads.onCreated.addListener((item) => {
  if (!interceptEnabled) return;

  // Ignore completed, interrupted, or stale downloads from previous browser sessions
  if (item.state && item.state !== "in_progress") return;
  if (item.startTime && (Date.now() - new Date(item.startTime).getTime() > 15000)) return;

  const url = item.finalUrl || item.url;
  if (!url || url.startsWith("data:") || url.startsWith("blob:") || url.startsWith("chrome:") || url.startsWith("edge:") || url.startsWith("file:")) {
    return;
  }

  if (recentlyIntercepted.has(item.id) || recentlyIntercepted.has(url)) {
    return;
  }

  recentlyIntercepted.add(item.id);
  recentlyIntercepted.add(url);
  setTimeout(() => {
    recentlyIntercepted.delete(item.id);
    recentlyIntercepted.delete(url);
  }, 1500);

  // Cancel Edge download immediately and close any popup/blank tab opened for this download
  chrome.downloads.cancel(item.id, () => {
    chrome.downloads.erase({ id: item.id }, () => {});
  });
  closeTabIfBlankDownload(url);

  const filename = extractFilename(item, url);
  sendDownloadToEnylvia(url, filename, item.referrer || "");
});

// 2. Secondary Interceptor: onDeterminingFilename
chrome.downloads.onDeterminingFilename.addListener((item, suggest) => {
  if (!interceptEnabled) {
    suggest();
    return;
  }

  // Ignore completed, interrupted, or stale downloads from previous browser sessions
  if (item.state && item.state !== "in_progress") {
    suggest();
    return;
  }
  if (item.startTime && (Date.now() - new Date(item.startTime).getTime() > 15000)) {
    suggest();
    return;
  }

  const url = item.finalUrl || item.url;
  if (!url || url.startsWith("data:") || url.startsWith("blob:") || url.startsWith("chrome:") || url.startsWith("edge:") || url.startsWith("file:")) {
    suggest();
    return;
  }

  if (recentlyIntercepted.has(item.id) || recentlyIntercepted.has(url)) {
    suggest();
    return;
  }

  recentlyIntercepted.add(item.id);
  recentlyIntercepted.add(url);
  setTimeout(() => {
    recentlyIntercepted.delete(item.id);
    recentlyIntercepted.delete(url);
  }, 1500);

  chrome.downloads.cancel(item.id, () => {
    chrome.downloads.erase({ id: item.id }, () => {});
  });
  closeTabIfBlankDownload(url);

  const filename = extractFilename(item, url);
  sendDownloadToEnylvia(url, filename, item.referrer || "");
  suggest(); // Tell browser we are done determining filename
});

async function sendDownloadToEnylvia(url, filename, referrer) {
  let cookieHeader = "";
  try {
    const cookies = await chrome.cookies.getAll({ url: url });
    if (cookies && cookies.length > 0) {
      cookieHeader = cookies.map(c => `${c.name}=${c.value}`).join("; ");
    }
  } catch (e) {}

  const payload = {
    action: "download",
    url: url,
    filename: filename || "",
    referrer: referrer || "",
    cookies: cookieHeader,
    headers: {
      "Referer": referrer || "",
      "Cookie": cookieHeader
    }
  };

  // 1. Direct HTTP API for maximum speed & reliability
  try {
    const res = await fetch(API_URL, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        url: payload.url,
        filename: payload.filename,
        headers: payload.headers,
        auto_start: true
      })
    });

    if (res.ok) {
      showNotification("EnylviaDownloader", `Mengunduh: ${filename || url}`);
      return;
    }
  } catch (err) {
    // If HTTP fails, proceed to Native Messaging Host
  }

  // 2. Fallback: Native Messaging Host
  try {
    chrome.runtime.sendNativeMessage(NATIVE_HOST, payload, (response) => {
      if (chrome.runtime.lastError || !response || response.status !== "ok") {
        console.warn("Native message error:", chrome.runtime.lastError || response);
      } else {
        showNotification("EnylviaDownloader", `Mengunduh: ${filename || url}`);
      }
    });
  } catch (e) {
    console.warn("Native messaging failed:", e);
  }
}

function showNotification(title, message) {
  try {
    chrome.notifications.create({
      type: "basic",
      iconUrl: "icons/icon48.png",
      title: title,
      message: message
    });
  } catch (e) {}
}
