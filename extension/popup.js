const API_BASE = "http://127.0.0.1:4242";

// Check server status
async function checkStatus() {
  const statusEl = document.getElementById("server-status");
  try {
    const res = await fetch(`${API_BASE}/api/config`);
    if (res.ok) {
      statusEl.innerHTML = '<span class="status-dot online"></span>Online';
    } else {
      statusEl.innerHTML = '<span class="status-dot offline"></span>Offline';
    }
  } catch (err) {
    statusEl.innerHTML = '<span class="status-dot offline"></span>Offline';
  }
}

// Load intercept setting
chrome.storage.local.get(["interceptEnabled"], (res) => {
  document.getElementById("toggle-intercept").checked = res.interceptEnabled !== false;
});

document.getElementById("toggle-intercept").addEventListener("change", (e) => {
  chrome.storage.local.set({ interceptEnabled: e.target.checked });
});

document.getElementById("btn-open-dashboard").addEventListener("click", () => {
  chrome.tabs.create({ url: API_BASE });
});

function showStatusMessage(msg, isSuccess) {
  let el = document.getElementById("msg-box");
  if (!el) {
    el = document.createElement("div");
    el.id = "msg-box";
    el.style.cssText = "margin-top: 10px; font-size: 11px; text-align: center; padding: 6px; border-radius: 4px; transition: all 0.2s;";
    document.body.appendChild(el);
  }
  el.style.display = "block";
  el.style.backgroundColor = isSuccess ? "rgba(16, 185, 129, 0.15)" : "rgba(239, 68, 68, 0.15)";
  el.style.color = isSuccess ? "#10b981" : "#ef4444";
  el.style.border = `1px solid ${isSuccess ? "#059669" : "#dc2626"}`;
  el.innerText = msg;
  setTimeout(() => { if (el) el.style.display = "none"; }, 4000);
}

document.getElementById("btn-quick-download").addEventListener("click", async () => {
  const url = document.getElementById("quick-url").value.trim();
  if (!url) return;

  try {
    const res = await fetch(`${API_BASE}/api/tasks/add`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ url, auto_start: true })
    });
    const data = await res.json();
    if (data.task) {
      document.getElementById("quick-url").value = "";
      showStatusMessage("Unduhan berhasil diteruskan!", true);
    } else {
      showStatusMessage("Gagal: " + (data.error || "Unknown error"), false);
    }
  } catch (err) {
    showStatusMessage("EnylviaDownloader belum aktif.", false);
  }
});

checkStatus();
