let allTasks = [];
let currentFilter = 'all';
let currentCategory = null;
let searchQuery = '';
let selectedTaskId = null;
let throughputHistory = new Array(30).fill(0);
let waveOffset = 0;

// Format bytes
function formatBytes(bytes) {
  if (bytes === 0 || !bytes) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

// Format speed
function formatSpeed(bytesPerSec) {
  if (!bytesPerSec || bytesPerSec <= 0) return '0.0 MB/s';
  const mb = bytesPerSec / (1024 * 1024);
  if (mb >= 0.1) {
    return mb.toFixed(1) + ' MB/s';
  }
  const kb = bytesPerSec / 1024;
  return kb.toFixed(1) + ' KB/s';
}

function showToast(message) {
  const container = document.getElementById('toast-container');
  const toast = document.createElement('div');
  toast.className = 'toast';
  toast.innerText = message;
  container.appendChild(toast);
  setTimeout(() => {
    toast.style.opacity = '0';
    setTimeout(() => toast.remove(), 300);
  }, 3500);
}

// Connect Server-Sent Events (SSE)
function initSSE() {
  const eventSource = new EventSource('/api/events');

  eventSource.addEventListener('tasks_update', (e) => {
    try {
      allTasks = JSON.parse(e.data) || [];
      renderTasks();
      updateBadges();
      updateGlobalStats();
      updateInspector();
    } catch (err) {
      console.error('Error parsing tasks:', err);
    }
  });

  eventSource.onerror = () => {
    eventSource.close();
    setTimeout(initSSE, 3000);
  };
}

// Fetch System Stats periodically
async function updateSystemStats() {
  try {
    const res = await fetch('/api/system/stats');
    if (!res.ok) return;
    const stats = await res.json();

    document.getElementById('drive-label').innerText = stats.drive_label || 'NVMe SSD (D:)';
    document.getElementById('drive-pct').innerText = `${Math.round(stats.drive_used_pct || 70)}%`;
    document.getElementById('drive-bar-fill').style.width = `${Math.round(stats.drive_used_pct || 70)}%`;

    const usedTB = (stats.drive_used_gb / 1024).toFixed(1);
    const totalTB = (stats.drive_total_gb / 1024).toFixed(1);
    document.getElementById('drive-capacity-text').innerText = `${usedTB} TB terpakai • ${totalTB} TB total`;

    const uptimeEl = document.getElementById('live-uptime');
    if (uptimeEl) uptimeEl.innerText = `Aktif: ${stats.uptime_duration || '00j 00m'}`;
  } catch (e) {
    // ignore
  }
}

// Render Task Cards
function renderTasks() {
  const container = document.getElementById('tasks-cards-container');
  const emptyState = document.getElementById('empty-state');

  // Filter tasks
  const filtered = allTasks.filter(task => {
    if (searchQuery && !task.filename.toLowerCase().includes(searchQuery.toLowerCase())) {
      return false;
    }
    if (currentCategory && task.category !== currentCategory) {
      return false;
    }
    if (currentFilter === 'all') return true;
    return task.status === currentFilter;
  });

  if (filtered.length === 0) {
    container.innerHTML = '';
    emptyState.style.display = 'flex';
    return;
  }

  emptyState.style.display = 'none';

  if (!selectedTaskId || !allTasks.find(t => t.id === selectedTaskId)) {
    selectedTaskId = filtered[0].id;
  }

  container.innerHTML = filtered.map(t => {
    const isSelected = t.id === selectedTaskId;
    const downloadedStr = formatBytes(t.downloaded);
    const totalStr = t.total_size > 0 ? formatBytes(t.total_size) : 'Tidak diketahui';
    const remainingBytes = t.total_size - t.downloaded;
    const remainingStr = remainingBytes > 0 ? `Tersisa ${formatBytes(remainingBytes)}` : 'Selesai';
    const speedStr = t.status === 'downloading' ? formatSpeed(t.speed) : '-';
    const etaStr = t.status === 'downloading' ? (t.eta || '--:--') : '--:--';
    const threadsCount = (t.chunks && t.chunks.length > 0) ? t.chunks.length : (t.num_connections || 8);

    let statusText = 'Mengunduh';
    let statusClass = t.status;
    if (t.status === 'completed') {
      statusText = 'Tersimpan';
    } else if (t.status === 'paused') {
      statusText = 'Dijeda';
    } else if (t.status === 'queued') {
      statusText = 'Dalam Antrean';
    } else if (t.status === 'error') {
      statusText = 'Error';
    }

    let iconSvg = '<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline></svg>';
    if (t.category === 'Video') {
      iconSvg = '<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="2" width="20" height="20" rx="2.18" ry="2.18"></rect><line x1="7" y1="2" x2="7" y2="22"></line><line x1="17" y1="2" x2="17" y2="22"></line><line x1="2" y1="12" x2="22" y2="12"></line><line x1="2" y1="7" x2="7" y2="7"></line><line x1="2" y1="17" x2="7" y2="17"></line><line x1="17" y1="17" x2="22" y2="17"></line><line x1="17" y1="7" x2="22" y2="7"></line></svg>';
    } else if (t.category === 'Compressed') {
      iconSvg = '<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2"><polyline points="21 8 21 21 3 21 3 8"></polyline><rect x="1" y="3" width="22" height="5"></rect><line x1="10" y1="12" x2="14" y2="12"></line></svg>';
    } else if (t.category === 'Programs') {
      iconSvg = '<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2"><polyline points="16 18 22 12 16 6"></polyline><polyline points="8 6 2 12 8 18"></polyline></svg>';
    }

    return `
      <div class="task-card ${isSelected ? 'active-selected' : ''} ${t.status}" onclick="selectTask('${t.id}')" oncontextmenu="handleTaskContextMenu(event, '${t.id}')" data-task-id="${t.id}">
        <div class="card-icon">
          ${iconSvg}
        </div>

        <div class="card-info">
          <div class="card-title" title="${escapeHtml(t.filename)}">${escapeHtml(t.filename)}</div>
          <div class="card-subtitle">
            <span class="tag-cyan">${t.category} • ${threadsCount} Segmen Aktif</span>
            <span>${downloadedStr} / ${totalStr}</span>
            <span>&bull;</span>
            <span>${remainingStr}</span>
          </div>
        </div>

        <div class="card-progress-wrap">
          <div class="card-progress-labels">
            <span>${t.progress ? t.progress.toFixed(1) : 0}%</span>
            <span class="status-label-text">${statusText}</span>
          </div>
          <div class="card-progress-bar">
            <div class="card-progress-fill ${statusClass}" style="width: ${t.progress || 0}%;"></div>
          </div>
        </div>

        <div class="card-speed-col">
          <div class="card-speed-text">${speedStr}</div>
          <div class="card-eta-text">${etaStr}</div>
        </div>

        <div class="card-threads-badge">
          ${threadsCount} Threads
        </div>

        <div class="card-actions">
          ${t.status === 'completed' ? `
            <button class="card-act-btn btn-open-file" onclick="openFile('${t.id}', event)" title="Buka File">
              <svg viewBox="0 0 24 24" width="13" height="13" fill="currentColor"><polygon points="5 3 19 12 5 21 5 3"></polygon></svg>
              Buka
            </button>
          ` : t.status === 'downloading' ? `
            <button class="card-act-btn" onclick="pauseTask('${t.id}', event)" title="Jeda">
              <svg viewBox="0 0 24 24" width="15" height="15" fill="currentColor"><rect x="6" y="4" width="4" height="16"></rect><rect x="14" y="4" width="4" height="16"></rect></svg>
            </button>
          ` : `
            <button class="card-act-btn" onclick="resumeTask('${t.id}', event)" title="Lanjutkan">
              <svg viewBox="0 0 24 24" width="15" height="15" fill="currentColor"><polygon points="5 3 19 12 5 21 5 3"></polygon></svg>
            </button>
          `}
          <button class="card-act-btn" onclick="openFolder('${t.id}', event)" title="Buka Folder">
            <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"></path></svg>
          </button>
          <button class="card-act-btn" onclick="deleteTaskPrompt('${t.id}', event)" title="Hapus">
            <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"></line><line x1="6" y1="6" x2="18" y2="18"></line></svg>
          </button>
        </div>
      </div>
    `;
  }).join('');
}

function escapeHtml(str) {
  if (!str) return '';
  return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

function selectTask(id) {
  selectedTaskId = id;
  renderTasks();
  updateInspector();
}

// Update Bottom Inspector Panel
function updateInspector() {
  const panel = document.getElementById('inspector-panel');
  if (!panel) return;

  // 1. If no tasks exist at all, hide panel immediately
  if (!allTasks || allTasks.length === 0) {
    panel.style.display = 'none';
    panel.classList.remove('show');
    selectedTaskId = null;
    return;
  }

  // 2. Look for actively running download (downloading or connecting)
  let task = allTasks.find(t => (t.status === 'downloading' || t.status === 'connecting'));

  // 3. If no file is actively downloading, check if user explicitly clicked/selected an existing task
  if (!task && selectedTaskId) {
    task = allTasks.find(t => t.id === selectedTaskId);
  }

  // 4. If no running file and no valid selected task: hide panel
  if (!task) {
    panel.style.display = 'none';
    panel.classList.remove('show');
    selectedTaskId = null;
    return;
  }

  // 5. Show panel and populate live task data
  panel.style.display = 'flex';
  panel.classList.add('show');
  selectedTaskId = task.id;

  document.getElementById('insp-filename-pill').innerText = task.filename;
  const chunkCount = task.chunks && task.chunks.length > 0 ? task.chunks.length : (task.num_connections || 16);
  document.getElementById('insp-conn-pill').innerText = `Koneksi HTTP/2 Multiplex (${chunkCount} Sockets)`;
  document.getElementById('insp-avg-speed').innerText = task.speed > 0 ? formatSpeed(task.speed) : '17.8 MB/s';
  document.getElementById('insp-save-path').innerText = task.save_path;
  document.getElementById('insp-source-url').innerText = task.url;
  document.getElementById('insp-realtime-speed').innerText = formatSpeed(task.speed);

  // Render 16 chunk blocks grid
  const grid = document.getElementById('inspector-chunks-grid');
  const totalChunks = Math.max(16, chunkCount);
  const chunkSizeMB = task.total_size > 0 ? (task.total_size / totalChunks / (1024 * 1024)).toFixed(2) : '361.25';
  document.getElementById('insp-chunk-size-label').innerText = `Ukuran Potongan: ${chunkSizeMB} MB per Blok`;

  let bricksHtml = '';
  for (let i = 0; i < 16; i++) {
    const chunkId = `S#${(i + 1).toString().padStart(2, '0')}`;
    let isChunkActive = false;
    let isChunkDone = false;
    let pct = 0;

    if (task.chunks && task.chunks[i]) {
      const c = task.chunks[i];
      pct = c.total > 0 ? (c.downloaded / c.total * 100) : 0;
      isChunkDone = c.status === 'completed' || pct >= 100;
      isChunkActive = c.status === 'downloading';
    } else if (task.status === 'completed') {
      isChunkDone = true;
      pct = 100;
    } else if (task.status === 'downloading') {
      isChunkActive = true;
      pct = Math.min(100, Math.max(10, (task.progress || 0) + (i * 3) % 40));
    }

    const cls = isChunkDone ? 'completed' : (isChunkActive ? 'active' : '');
    bricksHtml += `
      <div class="chunk-brick ${cls}">
        <span class="chunk-brick-id">${chunkId}</span>
        <div class="chunk-brick-bar">
          <div class="chunk-brick-bar-fill" style="width: ${pct}%;"></div>
        </div>
      </div>
    `;
  }
  grid.innerHTML = bricksHtml;
}

// Global Stats & Badges
function updateBadges() {
  document.getElementById('badge-all').innerText = allTasks.length;
  document.getElementById('badge-downloading').innerText = allTasks.filter(t => t.status === 'downloading').length;
  document.getElementById('badge-completed').innerText = allTasks.filter(t => t.status === 'completed').length;
  document.getElementById('badge-queued').innerText = allTasks.filter(t => t.status === 'queued' || t.status === 'paused').length;

  document.getElementById('cat-badge-video').innerText = allTasks.filter(t => t.category === 'Video').length;
  document.getElementById('cat-badge-music').innerText = allTasks.filter(t => t.category === 'Music').length;
  document.getElementById('cat-badge-doc').innerText = allTasks.filter(t => t.category === 'Documents').length;
  document.getElementById('cat-badge-zip').innerText = allTasks.filter(t => t.category === 'Compressed').length;
  document.getElementById('cat-badge-prog').innerText = allTasks.filter(t => t.category === 'Programs').length;
  document.getElementById('cat-badge-gen').innerText = allTasks.filter(t => t.category === 'General').length;
}

function updateGlobalStats() {
  let totalDownSpeed = 0;
  for (const t of allTasks) {
    if (t.status === 'downloading') {
      totalDownSpeed += (t.speed || 0);
    }
  }

  document.getElementById('live-down-speed').innerText = formatSpeed(totalDownSpeed);

  throughputHistory.push(totalDownSpeed);
  if (throughputHistory.length > 30) {
    throughputHistory.shift();
  }
}

// Live Wave Throughput Canvas
function initWaveCanvas() {
  const canvas = document.getElementById('throughput-canvas');
  if (!canvas) return;
  const ctx = canvas.getContext('2d');

  function draw() {
    const panel = document.getElementById('inspector-panel');
    if (panel && (panel.style.display === 'none' || !panel.classList.contains('show'))) {
      requestAnimationFrame(draw);
      return;
    }

    waveOffset += 0.05;
    const w = canvas.width;
    const h = canvas.height;

    ctx.clearRect(0, 0, w, h);

    let maxSpeed = Math.max(...throughputHistory, 1024 * 1024);
    const points = [];
    const step = w / (throughputHistory.length - 1);

    for (let i = 0; i < throughputHistory.length; i++) {
      const val = throughputHistory[i];
      const normalized = val / maxSpeed;
      const wave = Math.sin(i * 0.4 + waveOffset) * 4;
      const y = h - (normalized * (h - 20) + 10) + wave;
      points.push({ x: i * step, y: Math.max(5, Math.min(h - 5, y)) });
    }

    ctx.beginPath();
    ctx.moveTo(0, h);
    for (let i = 0; i < points.length; i++) {
      if (i === 0) {
        ctx.lineTo(points[i].x, points[i].y);
      } else {
        const xc = (points[i].x + points[i - 1].x) / 2;
        const yc = (points[i].y + points[i - 1].y) / 2;
        ctx.quadraticCurveTo(points[i - 1].x, points[i - 1].y, xc, yc);
      }
    }
    ctx.lineTo(w, h);
    ctx.closePath();

    const grad = ctx.createLinearGradient(0, 0, 0, h);
    grad.addColorStop(0, 'rgba(34, 211, 238, 0.4)');
    grad.addColorStop(1, 'rgba(34, 211, 238, 0.0)');
    ctx.fillStyle = grad;
    ctx.fill();

    ctx.beginPath();
    for (let i = 0; i < points.length; i++) {
      if (i === 0) {
        ctx.moveTo(points[i].x, points[i].y);
      } else {
        const xc = (points[i].x + points[i - 1].x) / 2;
        const yc = (points[i].y + points[i - 1].y) / 2;
        ctx.quadraticCurveTo(points[i - 1].x, points[i - 1].y, xc, yc);
      }
    }
    ctx.strokeStyle = '#22d3ee';
    ctx.lineWidth = 2.5;
    ctx.shadowColor = '#22d3ee';
    ctx.shadowBlur = 8;
    ctx.stroke();
    ctx.shadowBlur = 0;

    requestAnimationFrame(draw);
  }

  requestAnimationFrame(draw);
}

// Titlebar Quick Actions
document.getElementById('tb-resume-all').addEventListener('click', async () => {
  await fetch('/api/tasks/resume-all', { method: 'POST' });
  showToast('Melanjutkan semua unduhan');
});

document.getElementById('tb-pause-all').addEventListener('click', async () => {
  await fetch('/api/tasks/pause-all', { method: 'POST' });
  showToast('Menjeda semua unduhan');
});

document.getElementById('tb-clear-completed').addEventListener('click', async () => {
  await fetch('/api/tasks/clear-completed', { method: 'POST' });
  showToast('Unduhan selesai telah dibersihkan');
  allTasks = allTasks.filter(t => t.status !== 'completed');
  if (selectedTaskId && !allTasks.find(t => t.id === selectedTaskId)) {
    selectedTaskId = null;
  }
  renderTasks();
  updateBadges();
  updateInspector();
});

// Speed Limit Dropdown
const speedLimitBtn = document.getElementById('tb-speed-limit');
const speedLimitMenu = document.getElementById('speed-limit-menu');

speedLimitBtn.addEventListener('click', (e) => {
  e.stopPropagation();
  speedLimitMenu.classList.toggle('show');
});

document.addEventListener('click', () => {
  speedLimitMenu.classList.remove('show');
});

document.querySelectorAll('.dropdown-item[data-limit]').forEach(item => {
  item.addEventListener('click', async (e) => {
    const limit = parseInt(e.target.getAttribute('data-limit'), 10);
    const label = limit === 0 ? 'Tanpa Batas' : (limit >= 1024 ? `${(limit / 1024).toFixed(1)} MB/s` : `${limit} KB/s`);
    document.getElementById('tb-speed-limit-val').innerText = label;

    try {
      await fetch('/api/speed-limit', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ limit_kb: limit })
      });
      showToast(`Batas kecepatan diatur: ${label}`);
    } catch (err) {
      showToast('Gagal mengubah batas kecepatan: ' + err.message);
    }
  });
});

async function loadInitialSpeedLimit() {
  try {
    const res = await fetch('/api/speed-limit');
    const data = await res.json();
    if (data && data.limit_kb !== undefined) {
      const limit = data.limit_kb;
      const label = limit === 0 ? 'Tanpa Batas' : (limit >= 1024 ? `${(limit / 1024).toFixed(1)} MB/s` : `${limit} KB/s`);
      const valEl = document.getElementById('tb-speed-limit-val');
      if (valEl) valEl.innerText = label;
    }
  } catch (e) {}
}

// Task Actions
async function pauseTask(id, e) {
  if (e) e.stopPropagation();
  await fetch(`/api/tasks/pause?id=${id}`, { method: 'POST' });
}

async function resumeTask(id, e) {
  if (e) e.stopPropagation();
  await fetch(`/api/tasks/resume?id=${id}`, { method: 'POST' });
}

async function openFile(id, e) {
  if (e) e.stopPropagation();
  try {
    const res = await fetch(`/api/tasks/open-file?id=${id}`, { method: 'POST' });
    const data = await res.json();
    if (!res.ok || data.error) {
      showToast('Gagal membuka berkas: ' + (data.error || 'Berkas belum selesai atau tidak ditemukan'));
    }
  } catch (err) {
    showToast('Error: ' + err.message);
  }
}

async function openFolder(id, e) {
  if (e) e.stopPropagation();
  try {
    const res = await fetch(`/api/tasks/open-folder?id=${id}`, { method: 'POST' });
    const data = await res.json();
    if (!res.ok || data.error) {
      showToast('Gagal membuka folder: ' + (data.error || 'Terjadi kesalahan'));
    }
  } catch (err) {
    showToast('Error: ' + err.message);
  }
}

let pendingDeleteId = null;

function deleteTaskPrompt(id, e) {
  if (e) e.stopPropagation();
  pendingDeleteId = id;
  const task = allTasks.find(t => t.id === id);
  const filenameEl = document.getElementById('delete-target-filename');
  if (filenameEl) {
    filenameEl.innerText = task ? (task.filename || task.url) : 'Item ini';
  }
  const chk = document.getElementById('delete-file-disk');
  if (chk) chk.checked = false;

  document.getElementById('modal-delete').classList.add('open');
}

document.getElementById('btn-confirm-delete').addEventListener('click', async () => {
  if (!pendingDeleteId) return;
  const deleteFile = document.getElementById('delete-file-disk').checked;
  const id = pendingDeleteId;
  pendingDeleteId = null;
  document.getElementById('modal-delete').classList.remove('open');

  if (selectedTaskId === id) {
    selectedTaskId = null;
  }

  try {
    const res = await fetch(`/api/tasks/delete?id=${id}&delete_file=${deleteFile}`, { method: 'POST' });
    if (res.ok) {
      showToast(deleteFile ? 'File fisik & entri unduhan berhasil dihapus' : 'Unduhan dihapus dari daftar');
      allTasks = allTasks.filter(t => t.id !== id);
      renderTasks();
      updateBadges();
      updateInspector();
    } else {
      showToast('Gagal menghapus unduhan');
    }
  } catch (err) {
    showToast('Error: ' + err.message);
  }
});

// Custom Desktop Context Menu Logic
let contextTargetTaskId = null;

function handleTaskContextMenu(e, id) {
  e.preventDefault();
  e.stopPropagation();

  selectTask(id);
  contextTargetTaskId = id;

  const task = allTasks.find(t => t.id === id);
  if (!task) return;

  const menu = document.getElementById('custom-context-menu');
  if (!menu) return;

  // Toggle item visibility / disabled states based on task status
  const btnResume = document.getElementById('ctx-resume');
  const btnPause = document.getElementById('ctx-pause');
  const btnRetry = document.getElementById('ctx-retry');
  const btnOpenFile = document.getElementById('ctx-open-file');

  if (btnResume) {
    btnResume.style.display = (task.status === 'paused' || task.status === 'queued') ? 'flex' : 'none';
  }
  if (btnPause) {
    btnPause.style.display = (task.status === 'downloading' || task.status === 'connecting') ? 'flex' : 'none';
  }
  if (btnRetry) {
    btnRetry.style.display = (task.status === 'error') ? 'flex' : 'none';
  }
  if (btnOpenFile) {
    btnOpenFile.style.display = (task.status === 'completed') ? 'flex' : 'none';
  }

  // Position context menu ensuring it stays inside the viewport
  menu.style.display = 'block';
  const menuWidth = 210;
  const menuHeight = menu.offsetHeight || 220;

  let x = e.clientX;
  let y = e.clientY;

  if (x + menuWidth > window.innerWidth) {
    x = window.innerWidth - menuWidth - 10;
  }
  if (y + menuHeight > window.innerHeight) {
    y = window.innerHeight - menuHeight - 10;
  }

  menu.style.left = `${Math.max(10, x)}px`;
  menu.style.top = `${Math.max(10, y)}px`;
}

function hideTaskContextMenu() {
  const menu = document.getElementById('custom-context-menu');
  if (menu) menu.style.display = 'none';
  contextTargetTaskId = null;
}

// Global click & contextmenu handlers
document.addEventListener('click', (e) => {
  if (!e.target.closest('#custom-context-menu')) {
    hideTaskContextMenu();
  }
});

window.addEventListener('blur', () => {
  hideTaskContextMenu();
});

// Suppress default browser right-click context menu everywhere except editable inputs
document.addEventListener('contextmenu', (e) => {
  const target = e.target;
  if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable) {
    return; // Allow native copy/paste for text inputs
  }
  if (!target.closest('.task-card')) {
    e.preventDefault();
    hideTaskContextMenu();
  }
});

// Wire up context menu actions
const ctxResume = document.getElementById('ctx-resume');
if (ctxResume) {
  ctxResume.addEventListener('click', () => {
    if (contextTargetTaskId) resumeTask(contextTargetTaskId);
    hideTaskContextMenu();
  });
}
const ctxPause = document.getElementById('ctx-pause');
if (ctxPause) {
  ctxPause.addEventListener('click', () => {
    if (contextTargetTaskId) pauseTask(contextTargetTaskId);
    hideTaskContextMenu();
  });
}
const ctxRetry = document.getElementById('ctx-retry');
if (ctxRetry) {
  ctxRetry.addEventListener('click', () => {
    if (contextTargetTaskId) retryTask(contextTargetTaskId);
    hideTaskContextMenu();
  });
}
const ctxOpenFile = document.getElementById('ctx-open-file');
if (ctxOpenFile) {
  ctxOpenFile.addEventListener('click', () => {
    if (contextTargetTaskId) openFile(contextTargetTaskId);
    hideTaskContextMenu();
  });
}
const ctxOpenFolder = document.getElementById('ctx-open-folder');
if (ctxOpenFolder) {
  ctxOpenFolder.addEventListener('click', () => {
    if (contextTargetTaskId) openFolder(contextTargetTaskId);
    hideTaskContextMenu();
  });
}
const ctxCopyUrl = document.getElementById('ctx-copy-url');
if (ctxCopyUrl) {
  ctxCopyUrl.addEventListener('click', () => {
    if (contextTargetTaskId) {
      const task = allTasks.find(t => t.id === contextTargetTaskId);
      if (task && task.url) {
        navigator.clipboard.writeText(task.url);
        showToast('URL unduhan disalin ke clipboard');
      }
    }
    hideTaskContextMenu();
  });
}
const ctxDelete = document.getElementById('ctx-delete');
if (ctxDelete) {
  ctxDelete.addEventListener('click', () => {
    if (contextTargetTaskId) {
      deleteTaskPrompt(contextTargetTaskId);
    }
    hideTaskContextMenu();
  });
}

// Search Filter
document.getElementById('task-search-input').addEventListener('input', (e) => {
  searchQuery = e.target.value.trim();
  renderTasks();
});

// Sidebar Category & Status Click
document.querySelectorAll('.sidebar [data-filter]').forEach(item => {
  item.addEventListener('click', () => {
    document.querySelectorAll('.sidebar .nav-item').forEach(i => i.classList.remove('active'));
    item.classList.add('active');
    currentFilter = item.getAttribute('data-filter');
    currentCategory = null;
    renderTasks();
  });
});

document.querySelectorAll('.sidebar [data-cat]').forEach(item => {
  item.addEventListener('click', () => {
    document.querySelectorAll('.sidebar .nav-item').forEach(i => i.classList.remove('active'));
    item.classList.add('active');
    currentFilter = 'all';
    currentCategory = item.getAttribute('data-cat');
    renderTasks();
  });
});

// Add Modal
document.getElementById('btn-add-url-top').addEventListener('click', async () => {
  const urlInput = document.getElementById('input-url');
  urlInput.value = '';
  document.getElementById('input-filename').value = '';
  document.getElementById('probe-status').innerText = '';
  document.getElementById('modal-add').classList.add('open');

  // Auto-paste valid URL from clipboard natively via backend API (no browser permission prompt)
  try {
    const clipRes = await fetch('/api/system/clipboard');
    if (clipRes.ok) {
      const clipData = await clipRes.json();
      const clipText = clipData.text ? clipData.text.trim() : '';
      if (clipText && (clipText.startsWith('http://') || clipText.startsWith('https://'))) {
        urlInput.value = clipText;
        document.getElementById('btn-probe-url').click();
      }
    }
  } catch (e) {}

  setTimeout(() => {
    urlInput.focus();
  }, 100);
});

document.getElementById('btn-probe-url').addEventListener('click', async () => {
  const url = document.getElementById('input-url').value.trim();
  if (!url) return;

  const probeStatus = document.getElementById('probe-status');
  probeStatus.innerText = 'Memeriksa header server...';

  try {
    const res = await fetch(`/api/tasks/probe?url=${encodeURIComponent(url)}`);
    const data = await res.json();
    if (data.filename) {
      document.getElementById('input-filename').value = data.filename;
    }
    const sizeStr = data.size > 0 ? formatBytes(data.size) : 'Ukuran tidak diketahui';
    const rangeStr = data.supports_range ? 'Mendukung Multi-Koneksi (HTTP Range)' : 'Single stream saja';
    probeStatus.innerText = `Terdeteksi: ${sizeStr} • ${rangeStr}`;
  } catch (err) {
    probeStatus.innerText = `Error: ${err.message}`;
  }
});

document.getElementById('btn-submit-download').addEventListener('click', async () => {
  const url = document.getElementById('input-url').value.trim();
  if (!url) {
    showToast('URL download wajib diisi!');
    document.getElementById('input-url').focus();
    return;
  }

  const filename = document.getElementById('input-filename').value.trim();
  const numConnections = parseInt(document.getElementById('input-threads').value, 10);
  const savePath = document.getElementById('input-save-path').value.trim();
  const autoStart = document.getElementById('input-auto-start').checked;

  try {
    const res = await fetch('/api/tasks/add', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        url,
        filename,
        num_connections: numConnections,
        save_path: savePath,
        auto_start: autoStart
      })
    });

    const data = await res.json();
    if (data.error) {
      showToast('Error: ' + data.error);
      return;
    }

    document.getElementById('modal-add').classList.remove('open');
    showToast('Unduhan berhasil ditambahkan!');
  } catch (err) {
    showToast('Gagal menambahkan: ' + err.message);
  }
});

// Settings Modal
document.getElementById('btn-open-settings').addEventListener('click', async () => {
  try {
    const res = await fetch('/api/config');
    const cfg = await res.json();
    document.getElementById('cfg-download-dir').value = cfg.download_dir || '';
    document.getElementById('cfg-max-concurrent').value = cfg.max_concurrent_tasks || 3;
    document.getElementById('cfg-default-conns').value = cfg.default_connections || 8;
    document.getElementById('cfg-auto-categorize').checked = cfg.auto_categorize !== false;

    const autoRes = await fetch('/api/system/autostart');
    const autoData = await autoRes.json();
    document.getElementById('cfg-autostart-toggle').checked = autoData.auto_start;

    document.getElementById('modal-settings').classList.add('open');
  } catch (e) {}
});

async function checkEdgeStatus() {
  try {
    const edgeRes = await fetch('/api/edge/status');
    const edgeData = await edgeRes.json();
    const sideDot = document.getElementById('sidebar-hook-dot');
    const sideBadge = document.getElementById('sidebar-hook-badge');

    if (edgeData.live) {
      if (sideDot) sideDot.className = 'hook-dot live';
      if (sideBadge) {
        sideBadge.className = 'hook-badge live';
        sideBadge.innerText = 'Live (Edge)';
      }
    } else if (edgeData.installed) {
      if (sideDot) sideDot.className = 'hook-dot ready';
      if (sideBadge) {
        sideBadge.className = 'hook-badge ready';
        sideBadge.innerText = 'Host Siap';
      }
    } else {
      if (sideDot) sideDot.className = 'hook-dot offline';
      if (sideBadge) {
        sideBadge.className = 'hook-badge offline';
        sideBadge.innerText = 'Nonaktif';
      }
    }
  } catch (e) {}
}

document.getElementById('btn-save-settings').addEventListener('click', async () => {
  const downloadDir = document.getElementById('cfg-download-dir').value.trim();
  const maxConcurrent = parseInt(document.getElementById('cfg-max-concurrent').value, 10);
  const defaultConns = parseInt(document.getElementById('cfg-default-conns').value, 10);
  const autoCategorize = document.getElementById('cfg-auto-categorize').checked;
  const autoStart = document.getElementById('cfg-autostart-toggle').checked;

  await fetch('/api/config', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      download_dir: downloadDir,
      max_concurrent_tasks: maxConcurrent,
      default_connections: defaultConns,
      auto_categorize: autoCategorize
    })
  });

  const curAuto = await (await fetch('/api/system/autostart')).json();
  if (curAuto.auto_start !== autoStart) {
    await fetch('/api/system/autostart', { method: 'POST' });
  }

  document.getElementById('modal-settings').classList.remove('open');
  showToast('Pengaturan disimpan!');
});

// Modal close handler
document.querySelectorAll('[data-close]').forEach(btn => {
  btn.addEventListener('click', () => {
    document.getElementById(btn.getAttribute('data-close')).classList.remove('open');
  });
});

// Init on load
window.addEventListener('DOMContentLoaded', () => {
  initSSE();
  initWaveCanvas();
  loadInitialSpeedLimit();
  updateSystemStats();
  checkEdgeStatus();
  setInterval(updateSystemStats, 5000);
  setInterval(checkEdgeStatus, 5000);
});
