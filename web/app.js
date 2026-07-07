const API = 'http://localhost:8000'
let isRegisterMode = false

// ── Init ──────────────────────────────────────────
window.onload = () => {
  if (localStorage.getItem('token')) {
    showDashboard()
  }

  // Drag and drop wiring (elements exist in DOM even when hidden)
  const uploadArea = document.getElementById('upload-area')
  uploadArea.addEventListener('dragover', (e) => {
    e.preventDefault()
    uploadArea.classList.add('drag-over')
  })
  uploadArea.addEventListener('dragleave', () => uploadArea.classList.remove('drag-over'))
  uploadArea.addEventListener('drop', (e) => {
    e.preventDefault()
    uploadArea.classList.remove('drag-over')
    uploadFile(e.dataTransfer.files[0])
  })
}

// ── Toast ─────────────────────────────────────────
function showToast(message, type = 'info') {
  const icons = { success: '✓', error: '✕', info: 'ℹ' }
  const container = document.getElementById('toast-container')
  const toast = document.createElement('div')
  toast.className = `toast ${type}`
  toast.innerHTML = `<span>${icons[type]}</span> ${message}`
  container.appendChild(toast)

  setTimeout(() => {
    toast.classList.add('fade-out')
    setTimeout(() => toast.remove(), 300)
  }, 3000)
}

// ── Auth ──────────────────────────────────────────
function toggleMode() {
  isRegisterMode = !isRegisterMode
  document.getElementById('auth-title').textContent     = isRegisterMode ? 'Create account' : 'Welcome back'
  document.getElementById('auth-subtitle').textContent  = isRegisterMode ? 'Start storing your files' : 'Sign in to your storage vault'
  document.getElementById('auth-btn-text').textContent  = isRegisterMode ? 'Create Account' : 'Sign In'
  document.getElementById('toggle-text').innerHTML = isRegisterMode
    ? 'Already have an account? <a href="#" onclick="toggleMode()">Sign in</a>'
    : "Don't have an account? <a href=\"#\" onclick=\"toggleMode()\">Create one</a>"
  clearAuthError()
}

function setAuthLoading(loading) {
  document.getElementById('auth-btn-text').style.display  = loading ? 'none' : 'inline'
  document.getElementById('auth-btn-loader').style.display = loading ? 'inline-block' : 'none'
  document.getElementById('auth-btn').disabled = loading
}

function showAuthError(msg) {
  const el = document.getElementById('auth-error')
  el.textContent = msg
  el.classList.add('visible')
}

function clearAuthError() {
  const el = document.getElementById('auth-error')
  el.textContent = ''
  el.classList.remove('visible')
}

async function submitAuth() {
  const email    = document.getElementById('email').value.trim()
  const password = document.getElementById('password').value
  if (!email || !password) { showAuthError('Please fill in all fields'); return }

  clearAuthError()
  setAuthLoading(true)

  const endpoint = isRegisterMode ? '/auth/register' : '/auth/login'

  try {
    const res  = await fetch(API + endpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, password }),
    })
    const data = await res.json()

    if (!res.ok) {
      showAuthError(data.error || 'Invalid credentials')
      return
    }

    localStorage.setItem('token', data.token)
    showToast(isRegisterMode ? 'Account created!' : 'Welcome back!', 'success')
    showDashboard()
  } catch (err) {
    showAuthError('Could not connect to server')
  } finally {
    setAuthLoading(false)
  }
}

// Allow Enter key to submit
document.addEventListener('keydown', (e) => {
  if (e.key === 'Enter' && document.getElementById('login-view').style.display !== 'none') {
    submitAuth()
  }
})

function logout() {
  localStorage.removeItem('token')
  document.getElementById('dashboard-view').style.display = 'none'
  document.getElementById('login-view').style.display     = 'flex'
  showToast('Signed out', 'info')
}

// ── View Switch ───────────────────────────────────
function showDashboard() {
  document.getElementById('login-view').style.display     = 'none'
  document.getElementById('dashboard-view').style.display = 'flex'
  loadFiles()
}

// ── File List ─────────────────────────────────────
async function loadFiles() {
  const token = localStorage.getItem('token')
  showSkeleton(true)

  try {
    const res   = await fetch(API + '/objects', { headers: { Authorization: 'Bearer ' + token } })
    const files = await res.json()
    renderFiles(files)
  } catch {
    showToast('Failed to load files', 'error')
    showSkeleton(false)
  }
}

function showSkeleton(visible) {
  document.getElementById('skeleton-list').style.display = visible ? 'flex' : 'none'
  document.getElementById('file-table').style.display    = 'none'
  document.getElementById('empty-state').style.display   = 'none'
}

function renderFiles(files) {
  document.getElementById('skeleton-list').style.display = 'none'

  // Update stats
  const totalSize = files.reduce((sum, f) => sum + f.size_bytes, 0)
  document.getElementById('file-stats').textContent =
    files.length === 0 ? 'No files stored' :
    `${files.length} file${files.length !== 1 ? 's' : ''} · ${formatSize(totalSize)} total`

  if (files.length === 0) {
    document.getElementById('empty-state').style.display = 'block'
    document.getElementById('file-table').style.display  = 'none'
    return
  }

  document.getElementById('empty-state').style.display = 'none'
  document.getElementById('file-table').style.display  = 'table'

  const tbody = document.getElementById('file-list')
  tbody.innerHTML = ''

  files.forEach((f) => {
    const ext  = f.name.split('.').pop().toLowerCase()
    const icon = getFileIcon(ext)
    const tr   = document.createElement('tr')
    tr.innerHTML = `
      <td>
        <div class="file-name-cell">
          <div class="file-icon ${icon.cls}">${icon.emoji}</div>
          <span class="file-name">${escapeHTML(f.name)}</span>
        </div>
      </td>
      <td><span class="file-size">${formatSize(f.size_bytes)}</span></td>
      <td><span class="file-type-badge">${ext || 'file'}</span></td>
      <td>
        <div class="actions-cell">
          <button class="btn-action btn-download" onclick="downloadFile('${f.id}','${escapeHTML(f.name)}')">↓ Download</button>
          <button class="btn-action btn-delete"   onclick="deleteFile('${f.id}', this)">✕ Delete</button>
        </div>
      </td>
    `
    tbody.appendChild(tr)
  })
}

// ── Upload ────────────────────────────────────────
function uploadFile(file) {
  if (!file) return
  const token = localStorage.getItem('token')
  const xhr   = new XMLHttpRequest()

  xhr.open('POST', API + '/objects')
  xhr.setRequestHeader('Authorization', 'Bearer ' + token)
  xhr.setRequestHeader('X-Filename', file.name)
  xhr.setRequestHeader('Content-Type', file.type || 'application/octet-stream')

  // Show progress
  const wrapper = document.getElementById('progress-wrapper')
  document.getElementById('upload-filename').textContent = file.name
  document.getElementById('progress-bar').style.width    = '0%'
  document.getElementById('progress-text').textContent   = '0%'
  wrapper.style.display = 'block'

  xhr.upload.onprogress = (e) => {
    if (e.lengthComputable) {
      const pct = Math.round((e.loaded / e.total) * 100)
      document.getElementById('progress-bar').style.width  = pct + '%'
      document.getElementById('progress-text').textContent = pct + '%'
    }
  }

  xhr.onload = () => {
    wrapper.style.display = 'none'
    document.getElementById('file-input').value = ''
    if (xhr.status === 201) {
      showToast(`"${file.name}" uploaded successfully`, 'success')
      loadFiles()
    } else {
      showToast('Upload failed', 'error')
    }
  }

  xhr.onerror = () => {
    wrapper.style.display = 'none'
    showToast('Upload failed — check your connection', 'error')
  }

  xhr.send(file)
}

// ── Download ──────────────────────────────────────
async function downloadFile(id, name) {
  const token = localStorage.getItem('token')
  showToast(`Downloading "${name}"...`, 'info')

  try {
    const res  = await fetch(API + '/objects/' + id, { headers: { Authorization: 'Bearer ' + token } })
    const blob = await res.blob()
    const url  = URL.createObjectURL(blob)
    const a    = document.createElement('a')
    a.href = url
    a.download = name
    a.click()
    URL.revokeObjectURL(url)
    showToast('Download complete', 'success')
  } catch {
    showToast('Download failed', 'error')
  }
}

// ── Delete ────────────────────────────────────────
async function deleteFile(id, btn) {
  const token = localStorage.getItem('token')
  const originalText = btn.textContent
  btn.textContent = '...'
  btn.disabled    = true

  try {
    const res = await fetch(API + '/objects/' + id, {
      method: 'DELETE',
      headers: { Authorization: 'Bearer ' + token },
    })
    if (res.ok) {
      showToast('File deleted', 'info')
      loadFiles()
    } else {
      showToast('Delete failed', 'error')
      btn.textContent = originalText
      btn.disabled    = false
    }
  } catch {
    showToast('Delete failed', 'error')
    btn.textContent = originalText
    btn.disabled    = false
  }
}

// ── Helpers ───────────────────────────────────────
function formatSize(bytes) {
  if (bytes < 1024)          return bytes + ' B'
  if (bytes < 1024 * 1024)   return (bytes / 1024).toFixed(1) + ' KB'
  if (bytes < 1024 ** 3)     return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
  return (bytes / 1024 ** 3).toFixed(2) + ' GB'
}

function getFileIcon(ext) {
  const map = {
    pdf: { cls: 'doc', emoji: '📄' },
    doc: { cls: 'doc', emoji: '📝' }, docx: { cls: 'doc', emoji: '📝' },
    txt: { cls: 'doc', emoji: '📄' }, md: { cls: 'doc', emoji: '📄' },
    jpg: { cls: 'img', emoji: '🖼' }, jpeg: { cls: 'img', emoji: '🖼' },
    png: { cls: 'img', emoji: '🖼' }, gif: { cls: 'img', emoji: '🖼' },
    svg: { cls: 'img', emoji: '🖼' }, webp: { cls: 'img', emoji: '🖼' },
    mp4: { cls: 'vid', emoji: '🎬' }, mov: { cls: 'vid', emoji: '🎬' },
    avi: { cls: 'vid', emoji: '🎬' }, mkv: { cls: 'vid', emoji: '🎬' },
    zip: { cls: 'zip', emoji: '📦' }, tar: { cls: 'zip', emoji: '📦' },
    gz:  { cls: 'zip', emoji: '📦' }, rar: { cls: 'zip', emoji: '📦' },
    js:  { cls: 'code', emoji: '⟨⟩' }, ts: { cls: 'code', emoji: '⟨⟩' },
    go:  { cls: 'code', emoji: '⟨⟩' }, py: { cls: 'code', emoji: '⟨⟩' },
    html:{ cls: 'code', emoji: '⟨⟩' }, css: { cls: 'code', emoji: '⟨⟩' },
    json:{ cls: 'code', emoji: '⟨⟩' },
  }
  return map[ext] || { cls: 'misc', emoji: '📁' }
}

function escapeHTML(str) {
  return str.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;')
}
