(() => {
  const $ = (id) => document.getElementById(id);
  const listEl = $('list');
  const crumbEl = $('breadcrumb');
  const previewEl = $('preview');
  const previewTopEl = $('preview-top');
  const previewNameEl = $('preview-name');
  const previewBodyEl = $('preview-body');
  const previewCloseEl = $('preview-close');
  const previewFullscreenEl = $('preview-fullscreen');
  const previewPrevEl = $('preview-prev');
  const previewNextEl = $('preview-next');
  const loadingEl = $('loading');
  const btnUpEl = $('btn-up');
  const btnRefreshEl = $('btn-refresh');
  const btnTopEl = $('btn-top');
  const backPeekEl = $('back-peek');
  const backPeekHeadEl = $('back-peek-head');
  const backPeekListEl = $('back-peek-list');
  const btnUploadEl = $('btn-upload');
  const fileInputEl = $('file-input');
  const previewDownloadEl = $('preview-download');
  const uploadProgressEl = $('upload-progress');
  const uploadTextEl = $('upload-text');
  const uploadBarEl = $('upload-bar');
  const toolbarEl = $('toolbar');
  const btnEditEl = $('btn-edit');
  const editBarEl = $('edit-bar');
  const editSelectAllEl = $('edit-select-all');
  const editCountEl = $('edit-count');
  const editMoveEl = $('edit-move');
  const editDeleteEl = $('edit-delete');
  const editCancelEl = $('edit-cancel');
  const editHideEl = $('edit-hide');
  const editRenameEl = $('edit-rename');
  const editCopyEl = $('edit-copy');
  const moveBarEl = $('move-bar');
  const moveHintEl = $('move-hint');
  const moveHereEl = $('move-here');
  const moveCancelEl = $('move-cancel');
  const copyBarEl = $('copy-bar');
  const copyHintEl = $('copy-hint');
  const copyHereEl = $('copy-here');
  const copyCancelEl = $('copy-cancel');
  const btnNewEl = $('btn-new');
  const newMenuEl = $('new-menu');
  const newFolderEl = $('new-folder');
  const newFileEl = $('new-file');
  const newCancelEl = $('new-cancel');
  const btnLogoutEl = $('btn-logout');
  const btnSleepEl = $('btn-sleep');
  const searchToggleEl = $('search-toggle');
  const searchOverlayEl = $('search-overlay');
  const searchInputEl = $('search-input');
  const searchCancelEl = $('search-cancel');
  const searchResultsEl = $('search-results');
  const editTextEl = $('edit-text');
  const textEditorEl = $('text-editor');
  const textEditorNameEl = $('text-editor-name');
  const textEditorAreaEl = $('text-editor-area');
  const textEditorCancelEl = $('text-editor-cancel');
  const textEditorSaveEl = $('text-editor-save');
  const btnChatEl = $('btn-chat');
  const chatEl = $('chat');
  const chatCloseEl = $('chat-close');
  const chatPeerEl = $('chat-peer');
  const chatMsgsEl = $('chat-msgs');
  const chatAddEl = $('chat-add');
  const chatInputEl = $('chat-input');
  const chatSendEl = $('chat-send');
  const chatFileInputEl = $('chat-file-input');

  const ICONS = { dir: '📁', image: '🖼️', video: '🎬', audio: '🎵', text: '📄', other: '📦' };

  let current = '/';
  let gallery = [];       // 当前目录里的图片/视频项，用于左右翻页
  let galleryIndex = -1;
  let previewType = '';   // 当前预览类型
  let previewPath = '';   // 当前预览文件的相对路径
  let galleryMode = false;
  let editMode = false;       // 编辑（多选）模式
  const selected = new Set(); // 选中的相对路径
  let moveSource = null;      // 移动来源路径数组；非 null 表示处于“选择目标目录”模式
  let copySource = null;      // 复制来源路径数组；非 null 表示处于“选择目标目录”模式
  let hiddenPaths = new Set(); // 已隐藏文件夹路径（仅管理员账号可见）
  let isAdmin = false;         // 当前是否以管理员账号登录
  let searchTimer = null;      // 搜索输入防抖
  let flashPath = '';          // 搜索跳转后要高亮的文件相对路径
  let editFilePath = null;     // 正在编辑的文本文件相对路径
  let chatTimer = null;        // 对话轮询定时器
  let chatPeer = '';           // 对方（手机端恒为设备名）
  let chatMe = '';             // 自己（用户名）
  let chatLastSeq = 0;         // 已见最大消息序号

  // 画廊层
  let viewportRef = null;
  let curLayer = null;
  let peekLayer = null;

  // 图片缩放（focal 锚点，绕两指中点）
  let zoomImg = null, scale = 1, tx = 0, ty = 0;

  // 触控状态机
  let touchMode = 'none'; // none | pending | swipe | pan | pinch | close
  let tStartX = 0, tStartY = 0, swipeDir = 0, swipeDX = 0, closeDY = 0;
  let swipeAllowed = true;
  let touchStartTarget = null;
  let pinchStart = null;
  let lastPanX = 0, lastPanY = 0;
  let hideTimer = null;
  let chromeHidden = false;
  let lastSave = 0;

  // 列表右滑返回上级
  let lsStartX = 0, lsStartY = 0, lsDX = 0, lsActive = false, lsTrack = false, lsParent = '', lsParentData = null;
  let lsLastX = 0, lsLastT = 0, lsVX = 0;

  function fromHash() {
    let p = '/';
    try { p = decodeURIComponent(location.hash.slice(1)) || '/'; } catch (e) {}
    return p.startsWith('/') ? p : '/' + p;
  }
  function apiURL(path) { return '/api/list?path=' + encodeURIComponent(path); }
  function fileURL(path) { return '/api/file?path=' + encodeURIComponent(path); }
  function joinPath(dir, name) { return dir === '/' ? '/' + name : dir + '/' + name; }
  function parentPath(path) {
    if (path === '/') return '/';
    const parts = path.split('/').filter(Boolean);
    parts.pop();
    return parts.length ? '/' + parts.join('/') : '/';
  }
  function thumbURL(base, e) {
    return '/api/thumb?path=' + encodeURIComponent(joinPath(base, e.name)) + '&v=' + e.modTime + '_' + e.size;
  }

  function humanSize(n) {
    if (n < 1024) return n + ' B';
    const units = ['KB', 'MB', 'GB', 'TB'];
    let i = -1;
    do { n /= 1024; i++; } while (n >= 1024 && i < units.length - 1);
    return n.toFixed(n >= 100 ? 0 : 1) + ' ' + units[i];
  }
  function humanDate(ts) {
    const d = new Date(ts * 1000);
    const p = (x) => String(x).padStart(2, '0');
    return d.getFullYear() + '-' + p(d.getMonth() + 1) + '-' + p(d.getDate()) + ' ' + p(d.getHours()) + ':' + p(d.getMinutes());
  }
  function escapeHtml(s) {
    return s.replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  }

  function showLoading(v) { loadingEl.classList.toggle('hidden', !v); }

  // 未登录（会话过期）时跳转到登录页
  function apiFetch(url, opts) {
    return fetch(url, opts).then((res) => {
      if (res.status === 401) {
        try { localStorage.removeItem('nas-last-path'); } catch (e) {}
        location.href = '/login.html';
        throw new Error('未登录');
      }
      return res;
    });
  }

  async function load(path) {
    if (path !== current) { selected.clear(); editMode = false; }
    current = path;
    btnUpEl.disabled = (path === '/');
    try { localStorage.setItem('nas-last-path', path); } catch (e) {}
    showLoading(true);
    try {
      const res = await apiFetch(apiURL(path));
      if (!res.ok) throw new Error('HTTP ' + res.status);
      const data = await res.json();
      hiddenPaths = new Set(data.hiddenPaths || []);
      isAdmin = !!data.admin;
      btnSleepEl.classList.toggle('hidden', !isAdmin);
      renderBreadcrumb(data.path);
      renderList(data.entries, data.path);
    } catch (err) {
      listEl.innerHTML = '<div class="empty">加载失败：' + escapeHtml(String(err)) + '</div>';
    } finally {
      showLoading(false);
      updateBars();
      applySelection();
    }
  }

  function navigate(path) {
    try { history.replaceState(null, '', '#' + encodeURIComponent(path)); } catch (e) {}
    load(path);
  }

  function renderBreadcrumb(path) {
    crumbEl.innerHTML = '';
    const root = document.createElement('a');
    root.href = '#';
    root.dataset.path = '/';
    root.className = 'crumb';
    root.textContent = '🏠';
    crumbEl.appendChild(root);

    let acc = '';
    for (const s of path.split('/').filter(Boolean)) {
      acc = acc === '' ? '/' + s : acc + '/' + s;
      const sep = document.createElement('span');
      sep.className = 'crumb-sep';
      sep.textContent = '›';
      crumbEl.appendChild(sep);
      const a = document.createElement('a');
      a.href = '#';
      a.dataset.path = acc;
      a.className = 'crumb';
      a.textContent = s;
      crumbEl.appendChild(a);
    }
  }

  function renderList(entries, path, target) {
    target = target || listEl;
    if (target === listEl) gallery = [];
    if (!entries.length) {
      target.innerHTML = '<div class="empty">（空目录）</div>';
      return;
    }
    const frag = document.createDocumentFragment();
    for (const e of entries) {
      const tile = document.createElement('div');
      tile.className = 'tile';
      tile.dataset.name = e.name;
      tile.dataset.dir = e.isDir ? '1' : '0';
      tile.dataset.type = e.type;
      const isTrash = e.isDir && e.name === '.nas_trash';
      const isHidden = e.isDir && hiddenPaths.has(joinPath(path, e.name));

      const thumb = document.createElement('div');
      thumb.className = 'tile-thumb';

      if (e.type === 'image' || e.type === 'video') {
        if (target === listEl) gallery.push({ name: e.name, type: e.type });
        thumb.classList.add('tile-media');
        const img = document.createElement('img');
        img.className = 'tile-img';
        img.loading = 'lazy';
        img.alt = e.name;
        img.src = thumbURL(path, e);
        img.onload = () => thumb.classList.remove('tile-media');
        img.onerror = () => {
          img.remove();
          thumb.classList.remove('tile-media');
          thumb.textContent = ICONS[e.type];
        };
        thumb.appendChild(img);
        if (e.type === 'video') {
          const badge = document.createElement('span');
          badge.className = 'tile-badge';
          badge.textContent = '▶';
          thumb.appendChild(badge);
        }
      } else {
        thumb.textContent = isTrash ? '🗑️' : (ICONS[e.type] || ICONS.other);
      }
      if (isHidden) {
        const hbadge = document.createElement('span');
        hbadge.className = 'tile-badge';
        hbadge.textContent = '🙈';
        thumb.appendChild(hbadge);
      }

      const name = document.createElement('div');
      name.className = 'tile-name';
      name.textContent = isTrash ? '回收站' : e.name;

      tile.appendChild(thumb);
      tile.appendChild(name);
      frag.appendChild(tile);
    }
    target.innerHTML = '';
    target.appendChild(frag);
    if (flashPath && target === listEl) {
      const fp = flashPath;
      flashPath = '';
      const tile = target.querySelector('.tile[data-name="' + CSS.escape(fp.split('/').filter(Boolean).pop()) + '"]');
      if (tile) { tile.classList.add('flash'); setTimeout(() => tile.classList.remove('flash'), 1900); }
    }
  }

  function previewKind(name, type) {
    if (type === 'image' || type === 'video' || type === 'audio') return type;
    if (type === 'text' && /\.(txt|md|markdown|log|json|xml|html?|csv|ini|ya?ml|js|css|py|go|sh|bat|conf|properties)$/i.test(name)) return 'text';
    return 'other';
  }

  // —— 缩放 ——
  function dist(a, b) { return Math.hypot(a.clientX - b.clientX, a.clientY - b.clientY); }
  function resetZoom() { zoomImg = null; scale = 1; tx = 0; ty = 0; }
  function applyZoom() {
    if (zoomImg) zoomImg.style.transform = 'translate(' + tx + 'px,' + ty + 'px) scale(' + scale + ')';
  }
  function setZoomTarget(layer) {
    zoomImg = layer ? layer.querySelector('img') : null;
    scale = 1; tx = 0; ty = 0;
    if (zoomImg) zoomImg.style.transformOrigin = '0 0';
  }

  // —— 画廊 ——
  function mediaLayer(item) {
    const layer = document.createElement('div');
    layer.className = 'gallery-layer';
    const path = joinPath(current, item.name);
    if (item.type === 'video') {
      const v = document.createElement('video');
      v.className = 'preview-media';
      v.controls = true;
      v.playsInline = true;
      v.preload = 'metadata';
      v.poster = '/api/thumb?path=' + encodeURIComponent(path);
      v.src = fileURL(path);
      attachResume(v, path);
      layer.appendChild(v);
    } else {
      const img = document.createElement('img');
      img.className = 'preview-media';
      img.alt = item.name;
      img.src = fileURL(path);
      layer.appendChild(img);
    }
    return layer;
  }

  function openGallery() {
    galleryMode = true;
    const ov = previewBodyEl.querySelector('video');
    if (ov) ov.pause();
    previewBodyEl.innerHTML = '';
    const it = gallery[galleryIndex];
    const viewport = document.createElement('div');
    viewport.className = 'gallery-viewport';
    curLayer = mediaLayer(it);
    viewport.appendChild(curLayer);
    previewBodyEl.appendChild(viewport);
    viewportRef = viewport;
    setZoomTarget(curLayer);
    updateGalleryHeader();
    updateNavVisibility();
    setChrome(false);
  }

  function updateGalleryHeader() {
    const it = gallery[galleryIndex];
    previewNameEl.textContent = it ? it.name : '';
    previewFullscreenEl.classList.toggle('hidden', !(it && it.type === 'video'));
    if (it) {
      previewPath = joinPath(current, it.name);
      updateDownload();
    }
  }

  function updateNavVisibility() {
    const on = galleryIndex >= 0 && gallery.length > 1;
    previewPrevEl.classList.toggle('hidden', !on || galleryIndex <= 0);
    previewNextEl.classList.toggle('hidden', !on || galleryIndex >= gallery.length - 1);
  }

  function setChrome(hidden) {
    chromeHidden = hidden;
    previewTopEl.classList.toggle('chrome-hidden', hidden);
    previewPrevEl.classList.toggle('chrome-hidden', hidden);
    previewNextEl.classList.toggle('chrome-hidden', hidden);
    if (hidden) clearTimeout(hideTimer);
    else if (galleryMode) scheduleChromeHide();
  }
  function scheduleChromeHide() {
    clearTimeout(hideTimer);
    hideTimer = setTimeout(() => setChrome(true), 3000);
  }
  function toggleChrome() { setChrome(!chromeHidden); }

  // —— 拖拽跟手翻页 ——
  function showPeek(dir) {
    removePeek();
    const idx = galleryIndex + dir;
    if (idx < 0 || idx >= gallery.length) return false;
    peekLayer = mediaLayer(gallery[idx]);
    peekLayer.style.transform = 'translateX(' + (dir > 0 ? 100 : -100) + '%)';
    viewportRef.appendChild(peekLayer);
    return true;
  }
  function applySwipe() {
    if (curLayer) {
      curLayer.style.transition = 'none';
      curLayer.style.transform = 'translateX(' + swipeDX + 'px)';
    }
    if (peekLayer) {
      peekLayer.style.transition = 'none';
      const base = swipeDir > 0 ? 100 : -100;
      peekLayer.style.transform = 'translateX(calc(' + base + '% + ' + swipeDX + 'px))';
    }
  }
  function animate(el, transform) {
    if (!el) return;
    el.style.transition = 'transform 0.18s ease';
    el.style.transform = transform;
  }
  function commitSwipe(dir) {
    const ov = curLayer && curLayer.querySelector('video');
    if (ov) ov.pause();
    const w = previewBodyEl.clientWidth;
    const targetDX = dir > 0 ? -w : w;
    if (curLayer) animate(curLayer, 'translateX(' + targetDX + 'px)');
    if (peekLayer) animate(peekLayer, 'translateX(0px)');
    galleryIndex += dir;
    const oldCur = curLayer, newCur = peekLayer;
    peekLayer = null;
    setTimeout(() => {
      if (oldCur && oldCur.parentNode) oldCur.parentNode.removeChild(oldCur);
      curLayer = newCur;
      if (curLayer) { curLayer.style.transition = 'none'; curLayer.style.transform = ''; }
      setZoomTarget(curLayer);
      updateGalleryHeader();
      updateNavVisibility();
      scheduleChromeHide();
    }, 190);
  }
  function cancelSwipe() {
    const pv = peekLayer && peekLayer.querySelector('video');
    if (pv) pv.pause();
    if (curLayer) animate(curLayer, 'translateX(0px)');
    if (peekLayer) animate(peekLayer, 'translateX(' + (swipeDir > 0 ? 100 : -100) + '%)');
    const p = peekLayer;
    peekLayer = null;
    setTimeout(() => { if (p && p.parentNode) p.parentNode.removeChild(p); }, 190);
  }
  function removePeek() {
    if (peekLayer && peekLayer.parentNode) peekLayer.parentNode.removeChild(peekLayer);
    peekLayer = null;
  }

  // —— 下滑关闭 ——
  function applyClose() {
    previewEl.style.transition = 'none';
    previewEl.style.transform = 'translateY(' + closeDY + 'px)';
    previewEl.style.opacity = Math.max(0.2, 1 - closeDY / 300);
  }
  function cancelClose() {
    previewEl.style.transition = 'transform 0.18s ease, opacity 0.18s ease';
    previewEl.style.transform = '';
    previewEl.style.opacity = '';
    setTimeout(() => { previewEl.style.transition = ''; }, 200);
  }

  // —— 视频续播 ——
  function attachResume(video, path) {
    const key = 'nas-pos:' + path;
    video.addEventListener('loadedmetadata', () => {
      try {
        const t = parseFloat(localStorage.getItem(key) || '0');
        if (t > 2) video.currentTime = t;
      } catch (e) {}
    });
    video.addEventListener('timeupdate', () => {
      const now = Date.now();
      if (now - lastSave > 1000) { lastSave = now; savePos(key, video.currentTime); }
    });
    video.addEventListener('pause', () => savePos(key, video.currentTime));
    video.addEventListener('ended', () => { try { localStorage.removeItem(key); } catch (e) {} });
  }
  function savePos(key, t) {
    try { if (t > 0) localStorage.setItem(key, String(t)); } catch (e) {}
  }

  function updateDownload() {
    const name = previewPath ? previewPath.split('/').filter(Boolean).pop() : '';
    previewDownloadEl.setAttribute('download', name || 'download');
    previewDownloadEl.href = previewPath ? fileURL(previewPath) : '#';
    const show = previewType === 'image' || previewType === 'video' || previewType === 'audio' || previewType === 'text';
    previewDownloadEl.classList.toggle('hidden', !show);
  }

  function openPreview(name, path, type) {
    previewType = type;
    previewPath = path;
    previewNameEl.textContent = name;
    previewBodyEl.innerHTML = '';
    resetZoom();
    if (type === 'image' || type === 'video') {
      openGallery();
    } else {
      galleryMode = false;
      let node;
      if (type === 'audio') {
        node = document.createElement('audio');
        node.src = fileURL(path);
        node.controls = true;
        node.autoplay = true;
        node.className = 'preview-audio';
      } else if (type === 'text') {
        node = document.createElement('pre');
        node.className = 'preview-text';
        node.textContent = '加载中…';
        loadText(path, node);
      } else {
        node = downloadPanel(name, path);
      }
      previewBodyEl.appendChild(node);
      previewFullscreenEl.classList.add('hidden');
      previewPrevEl.classList.add('hidden');
      previewNextEl.classList.add('hidden');
      setChrome(false);
    }
    updateDownload();
    previewEl.classList.remove('hidden');
  }

  function closePreview() {
    const ov = previewBodyEl.querySelector('video');
    if (ov) ov.pause();
    previewEl.style.transition = '';
    previewEl.style.transform = '';
    previewEl.style.opacity = '';
    previewEl.classList.add('hidden');
    previewBodyEl.innerHTML = '';
    galleryIndex = -1;
    galleryMode = false;
    resetZoom();
    clearTimeout(hideTimer);
  }

  async function loadText(path, pre) {
    try {
      const res = await apiFetch(fileURL(path));
      if (!res.ok) throw new Error('HTTP ' + res.status);
      pre.textContent = await res.text();
    } catch (err) {
      pre.textContent = '加载失败：' + err;
    }
  }

  function downloadPanel(name, path) {
    const box = document.createElement('div');
    box.className = 'preview-download';
    const icon = document.createElement('div');
    icon.className = 'preview-download-icon';
    icon.textContent = '📄';
    const a = document.createElement('a');
    a.href = fileURL(path);
    a.download = name;
    a.className = 'download-btn';
    a.textContent = '下载';
    box.appendChild(icon);
    box.appendChild(a);
    return box;
  }

  // —— 列表点击 ——
  listEl.addEventListener('click', (ev) => {
    const item = ev.target.closest('.tile');
    if (!item) return;
    const name = item.dataset.name;
    const child = joinPath(current, name);
    if (moveSource || copySource) {         // 移动/复制目标模式：目录可进入，文件忽略
      if (item.dataset.dir === '1') navigate(child);
      return;
    }
    if (editMode) return;   // 编辑模式：选择由 touch 拖拽/轻点处理
    if (item.dataset.dir === '1') { navigate(child); return; }
    const kind = previewKind(name, item.dataset.type);
    galleryIndex = (kind === 'image' || kind === 'video')
      ? gallery.findIndex((g) => g.name === name)
      : -1;
    openPreview(name, child, kind);
  });

  // —— 长按进入多选（在非编辑、非移动/复制目标模式下生效） ——
  let lpTimer = null, lpActive = false, lpTile = null, lpStartX = 0, lpStartY = 0;
  function clearLongPress() {
    if (lpTimer) { clearTimeout(lpTimer); lpTimer = null; }
    lpActive = false; lpTile = null;
  }
  listEl.addEventListener('touchstart', (e) => {
    if (editMode || moveSource || copySource) return;
    if (e.touches.length !== 1) return;
    const tile = e.target.closest('.tile');
    if (!tile) return;
    lpActive = true;
    lpTile = tile;
    lpStartX = e.touches[0].clientX; lpStartY = e.touches[0].clientY;
    lpTimer = setTimeout(() => {
      lpTimer = null;
      if (!lpActive || !lpTile) return;
      const path = joinPath(current, lpTile.dataset.name);
      setEditMode(true);
      selected.add(path);
      applySelection();
      if (navigator.vibrate) { try { navigator.vibrate(20); } catch (e) {} }
      lpActive = false; lpTile = null;
    }, 500);
  }, { passive: true });
  listEl.addEventListener('touchmove', (e) => {
    if (!lpActive || e.touches.length !== 1) return;
    const t = e.touches[0];
    if (Math.abs(t.clientX - lpStartX) > 10 || Math.abs(t.clientY - lpStartY) > 10) clearLongPress();
  }, { passive: true });
  listEl.addEventListener('touchend', clearLongPress, { passive: true });
  listEl.addEventListener('touchcancel', clearLongPress, { passive: true });

  // —— 列表右滑返回上级目录（跟手移页） ——
  async function prepareBackPeek() {
    const pp = parentPath(current);
    lsParent = pp;
    lsParentData = null;
    const tb = document.querySelector('.topbar');
    backPeekEl.style.paddingTop = (tb ? tb.getBoundingClientRect().height : 0) + 'px';
    backPeekHeadEl.textContent = '‹ ' + (pp === '/' ? '根目录' : pp.split('/').filter(Boolean).pop());
    backPeekListEl.innerHTML = '';
    try {
      const res = await apiFetch(apiURL(pp));
      if (!res.ok) throw new Error('HTTP ' + res.status);
      const data = await res.json();
      lsParentData = data;
      renderList(data.entries, data.path, backPeekListEl);
    } catch (err) {
      backPeekListEl.innerHTML = '<div class="empty">加载失败</div>';
    }
  }

  document.addEventListener('touchstart', (e) => {
    lsTrack = false; lsActive = false; lsDX = 0; lsVX = 0; lsParent = '';
    if (e.touches.length !== 1) return;
    if (!previewEl.classList.contains('hidden')) return;
    if (current === '/') return;
    if (editMode) return;   // 编辑模式下右滑让位给拖拽多选
    if (e.target && e.target.closest && e.target.closest('.toolbar, .breadcrumb, .topbar, .top-btn')) return;
    const t0 = e.touches[0];
    lsStartX = t0.clientX; lsStartY = t0.clientY;
    lsLastX = t0.clientX; lsLastT = e.timeStamp;
    lsTrack = true;
  }, { passive: true });

  document.addEventListener('touchmove', (e) => {
    if (!lsTrack || e.touches.length !== 1) return;
    const t = e.touches[0];
    const dx = t.clientX - lsStartX;
    const dy = t.clientY - lsStartY;
    if (!lsActive) {
      if (Math.abs(dx) < 8 || Math.abs(dx) <= Math.abs(dy) || dx <= 0) return;
      lsActive = true;
      prepareBackPeek();
    }
    lsDX = Math.max(0, dx);
    const now = t.timeStamp;
    if (now > lsLastT) lsVX = (t.clientX - lsLastX) / (now - lsLastT);
    lsLastX = t.clientX; lsLastT = now;
    listEl.style.transition = 'none';
    listEl.style.transform = 'translateX(' + lsDX + 'px)';
    backPeekEl.style.transition = 'none';
    backPeekEl.style.transform = 'translateX(calc(-100% + ' + lsDX + 'px))';
    if (e.cancelable) e.preventDefault();
  }, { passive: false });

  document.addEventListener('touchend', () => {
    if (!lsActive) { lsTrack = false; return; }
    lsActive = false;
    const w = window.innerWidth;
    // iOS 式：拖过屏宽 1/5，或快速右滑（速度 > 0.5px/ms 且位移 > 16px）即返回
    const commit = lsDX > w * 0.2 || (lsVX > 0.5 && lsDX > 16);
    listEl.style.transition = 'transform 0.18s ease';
    backPeekEl.style.transition = 'transform 0.18s ease';
    const cleanup = () => {
      listEl.style.transition = ''; listEl.style.transform = '';
      backPeekEl.style.transition = ''; backPeekEl.style.transform = 'translateX(-100%)';
      backPeekEl.style.paddingTop = ''; backPeekListEl.innerHTML = '';
      lsParent = ''; lsParentData = null; lsTrack = false;
    };
    if (commit) {
      listEl.style.transform = 'translateX(' + w + 'px)';
      backPeekEl.style.transform = 'translateX(0px)';
      setTimeout(() => {
        if (lsParentData) {
          renderList(lsParentData.entries, lsParentData.path, listEl);
          current = lsParent;
          btnUpEl.disabled = (lsParent === '/');
          try { localStorage.setItem('nas-last-path', lsParent); } catch (e) {}
          try { history.replaceState(null, '', '#' + encodeURIComponent(lsParent)); } catch (e) {}
          renderBreadcrumb(lsParent);
        } else if (lsParent) {
          navigate(lsParent);
        }
        cleanup();
      }, 190);
    } else {
      listEl.style.transform = 'translateX(0px)';
      backPeekEl.style.transform = 'translateX(-100%)';
      setTimeout(cleanup, 190);
    }
  }, { passive: true });

  crumbEl.addEventListener('click', (ev) => {
    const a = ev.target.closest('a[data-path]');
    if (!a) return;
    ev.preventDefault();
    navigate(a.dataset.path);
  });

  // —— 预览按钮 ——
  previewCloseEl.addEventListener('click', closePreview);
  previewFullscreenEl.addEventListener('click', () => {
    const v = previewBodyEl.querySelector('video');
    if (!v) return;
    if (v.webkitEnterFullscreen) v.webkitEnterFullscreen();
    else if (v.requestFullscreen) v.requestFullscreen();
  });
  previewPrevEl.addEventListener('click', () => {
    if (galleryIndex > 0) { galleryIndex--; openGallery(); }
  });
  previewNextEl.addEventListener('click', () => {
    if (galleryIndex < gallery.length - 1) { galleryIndex++; openGallery(); }
  });

  // —— 触控：拖拽翻页 / 双指缩放 / 下滑关闭 / 轻点切换 chrome ——
  previewBodyEl.addEventListener('touchstart', (e) => {
    touchStartTarget = e.target;
    const t = e.touches;
    if (t.length === 2 && previewType === 'image') {
      touchMode = 'pinch';
      const a = t[0], b = t[1];
      const fx = (a.clientX + b.clientX) / 2, fy = (a.clientY + b.clientY) / 2;
      const rect = zoomImg ? zoomImg.getBoundingClientRect() : { left: 0, top: 0 };
      pinchStart = {
        dist: dist(a, b),
        scale: scale,
        baseX: rect.left - tx,   // 图片布局左上角（不含 transform 的固定偏移）
        baseY: rect.top - ty,
        cx: (fx - rect.left) / (scale || 1),
        cy: (fy - rect.top) / (scale || 1),
      };
      return;
    }
    if (t.length === 1) {
      tStartX = t[0].clientX; tStartY = t[0].clientY;
      swipeDX = 0; closeDY = 0; swipeDir = 0;
      touchMode = 'pending';
      // 图片可左右滑；视频仅在“暂停”状态可左右滑（底部进度条区域留给原生拖动）
      let inBar = false;
      if (e.target && e.target.tagName === 'VIDEO') {
        const r = e.target.getBoundingClientRect();
        inBar = tStartY > r.bottom - 64;
      }
      if (!galleryMode) {
        swipeAllowed = false;
      } else if (previewType === 'image') {
        swipeAllowed = true;
      } else {
        const v = previewBodyEl.querySelector('video');
        swipeAllowed = v ? (v.paused && !inBar) : true;
      }
      return;
    }
  }, { passive: true });

  previewBodyEl.addEventListener('touchmove', (e) => {
    const t = e.touches;
    if (touchMode === 'pinch') {
      if (t.length === 2) {
        const a = t[0], b = t[1];
        const d = dist(a, b);
        const ns = Math.min(4, Math.max(1, pinchStart.scale * (d / pinchStart.dist)));
        const fx = (a.clientX + b.clientX) / 2, fy = (a.clientY + b.clientY) / 2;
        scale = ns;
        tx = fx - pinchStart.cx * ns - pinchStart.baseX;
        ty = fy - pinchStart.cy * ns - pinchStart.baseY;
        applyZoom();
        if (e.cancelable) e.preventDefault();
      }
      return;
    }
    if (touchMode === 'pan') {
      if (t.length === 1) {
        tx += t[0].clientX - lastPanX;
        ty += t[0].clientY - lastPanY;
        lastPanX = t[0].clientX; lastPanY = t[0].clientY;
        applyZoom();
      }
      return;
    }
    if (touchMode === 'swipe' && t.length === 1) {
      swipeDX = t[0].clientX - tStartX;
      applySwipe();
      return;
    }
    if (touchMode === 'close' && t.length === 1) {
      closeDY = t[0].clientY - tStartY;
      applyClose();
      return;
    }
    if (touchMode === 'pending' && t.length === 1) {
      const dx = t[0].clientX - tStartX;
      const dy = t[0].clientY - tStartY;
      if (Math.abs(dx) < 8 && Math.abs(dy) < 8) return;
      if (Math.abs(dx) > Math.abs(dy)) {
        if (!swipeAllowed) { touchMode = 'none'; return; }
        if (scale > 1) { touchMode = 'pan'; lastPanX = t[0].clientX; lastPanY = t[0].clientY; return; }
        swipeDir = dx < 0 ? 1 : -1;
        if (!showPeek(swipeDir)) { touchMode = 'none'; return; }
        touchMode = 'swipe';
      } else {
        if (scale > 1) { touchMode = 'pan'; lastPanX = t[0].clientX; lastPanY = t[0].clientY; return; }
        touchMode = (previewType !== 'text' && dy > 0) ? 'close' : 'none';
      }
      return;
    }
  }, { passive: false });

  previewBodyEl.addEventListener('touchend', () => {
    if (touchMode === 'pinch' || touchMode === 'pan') { touchMode = 'none'; return; }
    if (touchMode === 'swipe') {
      const w = previewBodyEl.clientWidth;
      if (Math.abs(swipeDX) > w * 0.25) commitSwipe(swipeDir);
      else cancelSwipe();
      touchMode = 'none';
      return;
    }
    if (touchMode === 'close') {
      if (closeDY > 90) closePreview();
      else cancelClose();
      touchMode = 'none';
      return;
    }
    if (touchMode === 'pending') {
      onTap();
      touchMode = 'none';
      return;
    }
    touchMode = 'none';
  }, { passive: true });

  function onTap() {
    const el = touchStartTarget;
    if (el && el.closest && el.closest('button')) return; // 按钮点击
    if (previewType !== 'image' && previewType !== 'video') return;
    if (previewType === 'video' && el && el.tagName === 'VIDEO') return; // 视频原生控制
    toggleChrome();
  }

  // —— 工具栏 ——
  btnUpEl.addEventListener('click', () => { if (current !== '/') navigate(parentPath(current)); });
  btnRefreshEl.addEventListener('click', () => load(current));
  btnTopEl.addEventListener('click', () => window.scrollTo({ top: 0, behavior: 'smooth' }));
  window.addEventListener('scroll', () => btnTopEl.classList.toggle('hidden', window.scrollY < 400));

  // —— 上传 ——
  btnUploadEl.addEventListener('click', () => fileInputEl.click());
  fileInputEl.addEventListener('change', () => {
    if (fileInputEl.files && fileInputEl.files.length) uploadFiles(Array.from(fileInputEl.files));
    fileInputEl.value = '';
  });

  function uploadFiles(files) {
    if (!files.length) return;
    const total = files.length;
    let idx = 0;
    const errors = [];
    uploadProgressEl.classList.remove('hidden');
    uploadBarEl.style.width = '0';
    uploadTextEl.textContent = '上传 1/' + total + ' · ' + files[0].name;
    function next() {
      if (idx >= total) {
        if (errors.length) {
          uploadTextEl.textContent = '⚠ ' + errors.length + ' 个失败：' + errors.join('；');
          uploadBarEl.style.width = '0';
          setTimeout(() => uploadProgressEl.classList.add('hidden'), 4000);
        } else {
          uploadProgressEl.classList.add('hidden');
        }
        load(current);
        return;
      }
      const file = files[idx];
      const fd = new FormData();
      fd.append('file', file, file.name);
      const xhr = new XMLHttpRequest();
      xhr.open('POST', '/api/upload?path=' + encodeURIComponent(current));
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable) {
          const pct = Math.round((e.loaded / e.total) * 100);
          uploadTextEl.textContent = '上传 ' + (idx + 1) + '/' + total + ' · ' + pct + '% · ' + file.name;
          uploadBarEl.style.width = pct + '%';
        }
      };
      xhr.onload = () => {
        if (xhr.status === 401) { location.href = '/login.html'; return; }
        let ok = xhr.status >= 200 && xhr.status < 300;
        if (ok) {
          try {
            const data = JSON.parse(xhr.responseText);
            const r = data && data.results && data.results[0];
            if (r && r.error) { ok = false; errors.push(file.name + ': ' + r.error); }
          } catch (e) {}
        } else {
          errors.push(file.name + ': HTTP ' + xhr.status);
        }
        idx++;
        next();
      };
      xhr.onerror = () => { errors.push(file.name + ': 网络错误'); idx++; next(); };
      xhr.send(fd);
    }
    next();
  }

  // —— 编辑模式：多选 + 删除/移动 ——
  function updateBars() {
    const move = moveSource !== null;
    const copy = copySource !== null;
    toolbarEl.classList.toggle('hidden', editMode || move || copy);
    editBarEl.classList.toggle('hidden', !editMode || move || copy);
    moveBarEl.classList.toggle('hidden', !move);
    copyBarEl.classList.toggle('hidden', !copy);
  }

  function applySelection() {
    const tiles = listEl.querySelectorAll('.tile');
    tiles.forEach((t) => {
      t.classList.toggle('selected', selected.has(joinPath(current, t.dataset.name)));
    });
    const n = selected.size;
    editCountEl.textContent = '已选 ' + n + ' 项';
    editRenameEl.disabled = n !== 1;
    editCopyEl.disabled = n === 0;
    editDeleteEl.disabled = n === 0;
    editMoveEl.disabled = n === 0;
    const all = tiles.length > 0 && Array.from(tiles).every((t) => selected.has(joinPath(current, t.dataset.name)));
    editSelectAllEl.textContent = all ? '全不选' : '全选';
    syncEditHide();
    syncEditText();
  }

  // 隐藏/取消隐藏按钮：仅管理员可见，且恰好选中一个文件夹时可用。
  function syncEditHide() {
    if (!isAdmin) { editHideEl.classList.add('hidden'); return; }
    editHideEl.classList.remove('hidden');
    let singleDir = null;
    if (selected.size === 1) {
      for (const t of listEl.querySelectorAll('.tile')) {
        const p = joinPath(current, t.dataset.name);
        if (selected.has(p) && t.dataset.dir === '1') { singleDir = p; break; }
      }
    }
    editHideEl.disabled = !singleDir;
    editHideEl.textContent = singleDir && hiddenPaths.has(singleDir) ? '👁 取消隐藏' : '🙈 隐藏';
  }

  function toggleSelect(path) {
    if (selected.has(path)) selected.delete(path);
    else selected.add(path);
    applySelection();
  }

  function selectAll() {
    const tiles = listEl.querySelectorAll('.tile');
    const all = tiles.length > 0 && Array.from(tiles).every((t) => selected.has(joinPath(current, t.dataset.name)));
    if (all) tiles.forEach((t) => selected.delete(joinPath(current, t.dataset.name)));
    else tiles.forEach((t) => selected.add(joinPath(current, t.dataset.name)));
    applySelection();
  }

  function setEditMode(on) {
    editMode = on;
    if (!on) selected.clear();
    updateBars();
    applySelection();
  }

  function startMove() {
    moveSource = Array.from(selected);
    editMode = false;
    selected.clear();
    moveHintEl.textContent = '已选 ' + moveSource.length + ' 项，选择目标文件夹';
    updateBars();
    applySelection();
  }

  function cancelMove() {
    moveSource = null;
    updateBars();
    applySelection();
  }

  async function doMove() {
    if (!moveSource || !moveSource.length) return;
    const paths = moveSource;
    const dest = current;
    moveSource = null;
    updateBars();
    try {
      const res = await apiFetch('/api/move', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ paths: paths, dest: dest }),
      });
      const data = await res.json();
      const errs = (data.results || []).filter((r) => r.error);
      if (errs.length) alert('移动失败：\n' + errs.map((r) => r.path + ': ' + r.error).join('\n'));
    } catch (e) {
      alert('移动失败：' + e);
    }
    load(current);
  }

  function startCopy() {
    copySource = Array.from(selected);
    editMode = false;
    selected.clear();
    copyHintEl.textContent = '已选 ' + copySource.length + ' 项，选择目标文件夹';
    updateBars();
    applySelection();
  }

  function cancelCopy() {
    copySource = null;
    updateBars();
    applySelection();
  }

  async function doCopy() {
    if (!copySource || !copySource.length) return;
    const paths = copySource;
    const dest = current;
    copySource = null;
    updateBars();
    try {
      const res = await apiFetch('/api/copy', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ paths: paths, dest: dest }),
      });
      const data = await res.json();
      const errs = (data.results || []).filter((r) => r.error);
      if (errs.length) alert('复制失败：\n' + errs.map((r) => r.path + ': ' + r.error).join('\n'));
    } catch (e) {
      alert('复制失败：' + e);
    }
    load(current);
  }

  async function doDelete() {
    if (!selected.size) return;
    const paths = Array.from(selected);
    const names = paths.map((p) => p.split('/').filter(Boolean).pop());
    const preview = names.slice(0, 6).join('\n') + (names.length > 6 ? '\n…' : '');
    if (!confirm('确定删除 ' + names.length + ' 项？\n\n' + preview + '\n\n此操作不可恢复。')) return;
    setEditMode(false);
    try {
      const res = await apiFetch('/api/delete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ paths: paths }),
      });
      const data = await res.json();
      const errs = (data.results || []).filter((r) => r.error);
      if (errs.length) alert('删除失败：\n' + errs.map((r) => r.path + ': ' + r.error).join('\n'));
    } catch (e) {
      alert('删除失败：' + e);
    }
    load(current);
  }

  // 隐藏/取消隐藏当前选中的文件夹（仅管理员账号可用）。
  async function doHide() {
    if (!isAdmin || selected.size !== 1) return;
    const p = Array.from(selected)[0];
    const hidden = hiddenPaths.has(p);
    try {
      const res = await apiFetch('/api/hidden', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: p, hidden: !hidden }),
      });
      const data = await res.json();
      if (!data.ok) { alert('操作失败：' + (data.error || 'HTTP ' + res.status)); return; }
    } catch (e) {
      alert('操作失败：' + e);
    }
    setEditMode(false);
    load(current);
  }

  // 重命名当前选中的单个文件/文件夹。
  async function doRename() {
    if (selected.size !== 1) return;
    const p = Array.from(selected)[0];
    const oldName = p.split('/').filter(Boolean).pop() || '';
    const n = prompt('重命名：', oldName);
    if (!n || !n.trim() || n.trim() === oldName) return;
    try {
      const res = await apiFetch('/api/rename', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: p, name: n.trim() }),
      });
      const data = await res.json();
      if (!data.ok) { alert('重命名失败：' + (data.error || 'HTTP ' + res.status)); return; }
    } catch (e) {
      alert('重命名失败：' + e);
    }
    setEditMode(false);
    load(current);
  }

  btnEditEl.addEventListener('click', () => setEditMode(!editMode));
  editSelectAllEl.addEventListener('click', selectAll);
  editHideEl.addEventListener('click', doHide);
  editRenameEl.addEventListener('click', doRename);
  editCopyEl.addEventListener('click', startCopy);
  editMoveEl.addEventListener('click', startMove);
  editDeleteEl.addEventListener('click', doDelete);
  editCancelEl.addEventListener('click', () => setEditMode(false));
  moveHereEl.addEventListener('click', doMove);
  moveCancelEl.addEventListener('click', cancelMove);
  copyHereEl.addEventListener('click', doCopy);
  copyCancelEl.addEventListener('click', cancelCopy);

  // —— 编辑模式下拖拽滑动多选（iOS 相册式：拖动框选矩形，拖到屏幕边缘自动滚动） ——
  let dsActive = false, dsDrag = false, dsPolarity = true;
  let dsStartX = 0, dsStartY = 0, dsStartTile = null;
  let dsContentStartX = 0, dsContentStartY = 0, dsLastX = 0, dsLastY = 0;
  let dsSnapshot = null, dsTiles = null, dsRaf = null;

  function dsSyncBar() {
    const n = selected.size;
    editCountEl.textContent = '已选 ' + n + ' 项';
    editRenameEl.disabled = n !== 1;
    editCopyEl.disabled = n === 0;
    editDeleteEl.disabled = n === 0;
    editMoveEl.disabled = n === 0;
    syncEditHide();
    syncEditText();
  }
  // 用“内容坐标”（视口 + 滚动量）缓存所有格子位置，滚动时无需重算
  function dsBuildTiles() {
    const sy = window.scrollY;
    dsContentStartX = dsStartX;
    dsContentStartY = dsStartY + sy;
    dsTiles = Array.from(listEl.querySelectorAll('.tile')).map((tile) => {
      const r = tile.getBoundingClientRect();
      return { tile: tile, path: joinPath(current, tile.dataset.name), left: r.left, top: r.top + sy, right: r.right, bottom: r.bottom + sy };
    });
    dsSnapshot = new Set(selected);
  }
  function dsApplyRect(x1, y1, x2, y2) {
    const left = Math.min(x1, x2), right = Math.max(x1, x2);
    const top = Math.min(y1, y2), bottom = Math.max(y1, y2);
    for (const it of dsTiles) {
      const inRect = it.right >= left && it.left <= right && it.bottom >= top && it.top <= bottom;
      const want = inRect ? dsPolarity : dsSnapshot.has(it.path);
      if (want) selected.add(it.path); else selected.delete(it.path);
      it.tile.classList.toggle('selected', want);
    }
    dsSyncBar();
  }
  // 手指贴到屏幕顶部/底部边缘时自动滚动，并继续扩展框选
  function dsTick() {
    dsRaf = null;
    if (!dsDrag) return;
    const h = window.innerHeight;
    const margin = 72;
    let speed = 0;
    if (dsLastY < margin) speed = -Math.max(2, (margin - dsLastY) * 0.25);
    else if (dsLastY > h - margin) speed = Math.max(2, (dsLastY - (h - margin)) * 0.25);
    if (speed !== 0) {
      window.scrollBy(0, speed);
      dsApplyRect(dsContentStartX, dsContentStartY, dsLastX, dsLastY + window.scrollY);
    }
    dsRaf = requestAnimationFrame(dsTick);
  }
  function dsStop() {
    if (dsRaf) { cancelAnimationFrame(dsRaf); dsRaf = null; }
  }

  listEl.addEventListener('touchstart', (e) => {
    if (!editMode || e.touches.length !== 1) return;
    const tile = e.target.closest('.tile');
    if (!tile) return;
    dsActive = true; dsDrag = false;
    dsStartTile = tile;
    dsStartX = e.touches[0].clientX; dsStartY = e.touches[0].clientY;
    dsLastX = dsStartX; dsLastY = dsStartY;
    dsPolarity = !selected.has(joinPath(current, tile.dataset.name));
  }, { passive: true });

  listEl.addEventListener('touchmove', (e) => {
    if (!dsActive || e.touches.length !== 1) return;
    const t = e.touches[0];
    dsLastX = t.clientX; dsLastY = t.clientY;
    if (!dsDrag) {
      if (Math.abs(t.clientX - dsStartX) < 8 && Math.abs(t.clientY - dsStartY) < 8) return;
      dsDrag = true;
      dsBuildTiles();
      dsRaf = requestAnimationFrame(dsTick);
    }
    dsApplyRect(dsContentStartX, dsContentStartY, dsLastX, dsLastY + window.scrollY);
    if (e.cancelable) e.preventDefault();
  }, { passive: false });

  listEl.addEventListener('touchend', () => {
    if (dsActive && !dsDrag && dsStartTile) {
      // 轻点：切换单个
      const path = joinPath(current, dsStartTile.dataset.name);
      if (dsPolarity) selected.add(path); else selected.delete(path);
      dsStartTile.classList.toggle('selected', dsPolarity);
      dsSyncBar();
    }
    dsActive = false; dsDrag = false; dsStartTile = null;
    dsSnapshot = null; dsTiles = null;
    dsStop();
  }, { passive: true });

  listEl.addEventListener('touchcancel', () => {
    dsActive = false; dsDrag = false; dsStartTile = null;
    dsSnapshot = null; dsTiles = null;
    dsStop();
  }, { passive: true });

  // —— 新建文件夹 / 文本文件 ——
  function hideNewMenu() { newMenuEl.classList.add('hidden'); }
  async function createItem(type, name) {
    try {
      const res = await apiFetch('/api/create', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: current, type: type, name: name }),
      });
      const data = await res.json();
      if (!data.ok) { alert('创建失败：' + (data.error || 'HTTP ' + res.status)); return; }
    } catch (e) {
      alert('创建失败：' + e);
    }
    load(current);
  }
  btnNewEl.addEventListener('click', () => newMenuEl.classList.remove('hidden'));
  newCancelEl.addEventListener('click', hideNewMenu);
  newMenuEl.addEventListener('click', (e) => { if (e.target === newMenuEl) hideNewMenu(); });
  newFolderEl.addEventListener('click', () => {
    hideNewMenu();
    const n = prompt('文件夹名称：', '新建文件夹');
    if (n && n.trim()) createItem('dir', n.trim());
  });
  newFileEl.addEventListener('click', () => {
    hideNewMenu();
    const n = prompt('文件名：', '新建文本.txt');
    if (n && n.trim()) createItem('file', n.trim());
  });

  // —— 搜索 ——
  function openSearch() {
    searchOverlayEl.classList.remove('hidden');
    searchResultsEl.innerHTML = '';
    searchInputEl.value = '';
    searchInputEl.focus();
  }
  function closeSearch() {
    searchOverlayEl.classList.add('hidden');
    searchInputEl.value = '';
    searchResultsEl.innerHTML = '';
  }
  async function doSearch(q) {
    try {
      const res = await apiFetch('/api/search?q=' + encodeURIComponent(q));
      if (!res.ok) throw new Error('HTTP ' + res.status);
      const data = await res.json();
      renderSearchResults(data.results || [], q);
    } catch (err) {
      searchResultsEl.innerHTML = '<div class="search-empty">搜索失败：' + escapeHtml(String(err)) + '</div>';
    }
  }
  function renderSearchResults(results, q) {
    searchResultsEl.innerHTML = '';
    if (!results.length) {
      searchResultsEl.innerHTML = '<div class="search-empty">未找到 “' + escapeHtml(q) + '”</div>';
      return;
    }
    const frag = document.createDocumentFragment();
    for (const r of results) {
      const item = document.createElement('div');
      item.className = 'search-item';
      const icon = document.createElement('div');
      icon.className = 'search-item-icon';
      icon.textContent = r.isDir ? '📁' : (ICONS[r.type] || ICONS.other);
      const main = document.createElement('div');
      main.className = 'search-item-main';
      const nm = document.createElement('div');
      nm.className = 'search-item-name';
      nm.textContent = r.name;
      const pt = document.createElement('div');
      pt.className = 'search-item-path';
      pt.textContent = r.parent || '/';
      main.appendChild(nm);
      main.appendChild(pt);
      const arrow = document.createElement('div');
      arrow.className = 'search-item-arrow';
      arrow.textContent = '→';
      item.appendChild(icon);
      item.appendChild(main);
      item.appendChild(arrow);
      item.addEventListener('click', () => {
        const targetDir = r.isDir ? r.path : r.parent;
        flashPath = r.isDir ? '' : r.path;
        closeSearch();
        navigate(targetDir);
      });
      frag.appendChild(item);
    }
    searchResultsEl.appendChild(frag);
  }
  searchToggleEl.addEventListener('click', openSearch);
  searchCancelEl.addEventListener('click', closeSearch);
  searchInputEl.addEventListener('input', () => {
    const q = searchInputEl.value.trim();
    clearTimeout(searchTimer);
    if (!q) { searchResultsEl.innerHTML = ''; return; }
    searchTimer = setTimeout(() => doSearch(q), 250);
  });
  searchInputEl.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') {
      const q = searchInputEl.value.trim();
      if (q) doSearch(q);
    }
  });

  // —— 编辑文本文件 ——
  function isEditableText(name) {
    return /\.(txt|md|markdown|log|json|xml|html?|csv|ini|ya?ml|js|css|py|go|sh|bat|conf|properties)$/i.test(name);
  }
  function syncEditText() {
    let singleText = null;
    if (selected.size === 1) {
      const p = Array.from(selected)[0];
      const name = p.split('/').filter(Boolean).pop() || '';
      if (isEditableText(name)) {
        for (const t of listEl.querySelectorAll('.tile')) {
          if (t.dataset.dir === '0' && joinPath(current, t.dataset.name) === p) { singleText = p; break; }
        }
      }
    }
    editTextEl.disabled = !singleText;
  }
  async function openTextEditor(path) {
    editFilePath = path;
    textEditorNameEl.textContent = path.split('/').filter(Boolean).pop() || '';
    textEditorAreaEl.value = '加载中…';
    textEditorAreaEl.disabled = true;
    textEditorEl.classList.remove('hidden');
    try {
      const res = await apiFetch(fileURL(path));
      if (!res.ok) throw new Error('HTTP ' + res.status);
      textEditorAreaEl.value = await res.text();
    } catch (err) {
      textEditorAreaEl.value = '加载失败：' + err;
    }
    textEditorAreaEl.disabled = false;
    textEditorAreaEl.focus();
  }
  function closeTextEditor() {
    textEditorEl.classList.add('hidden');
    editFilePath = null;
    textEditorAreaEl.value = '';
  }
  async function saveTextEditor() {
    if (!editFilePath) return;
    try {
      const res = await apiFetch('/api/save', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: editFilePath, content: textEditorAreaEl.value }),
      });
      const data = await res.json();
      if (!data.ok) { alert('保存失败：' + (data.error || 'HTTP ' + res.status)); return; }
    } catch (err) {
      alert('保存失败：' + err);
      return;
    }
    closeTextEditor();
    setEditMode(false);
    load(current);
  }
  editTextEl.addEventListener('click', () => {
    if (selected.size !== 1) return;
    openTextEditor(Array.from(selected)[0]);
  });
  textEditorCancelEl.addEventListener('click', closeTextEditor);
  textEditorSaveEl.addEventListener('click', saveTextEditor);

  // 后退/前进按钮时同步
  window.addEventListener('hashchange', () => {
    const p = fromHash();
    if (p !== current) load(p);
  });

  // 深色/浅色模式切换
  const themeBtn = $('theme-toggle');
  function applyTheme(dark) {
    document.documentElement.classList.toggle('dark', dark);
    document.documentElement.style.colorScheme = dark ? 'dark' : 'light';
    themeBtn.textContent = dark ? '☀️' : '🌙';
    try { localStorage.setItem('nas-theme', dark ? 'dark' : 'light'); } catch (e) {}
  }
  themeBtn.addEventListener('click', () => {
    applyTheme(!document.documentElement.classList.contains('dark'));
  });
  applyTheme(document.documentElement.classList.contains('dark'));

  // 退出登录：清除会话并回到登录页
  btnLogoutEl.addEventListener('click', () => {
    if (!confirm('确定退出登录？')) return;
    apiFetch('/api/logout', { method: 'POST' })
      .catch(() => {})
      .then(() => {
        try { localStorage.removeItem('nas-last-path'); } catch (e) {}
        location.href = '/login.html';
      });
  });

  // 睡眠 PC：仅管理员账号可见，点击后二次确认。
  btnSleepEl.addEventListener('click', () => {
    if (!confirm('确定让电脑进入睡眠？\n\n手机端将暂时无法访问，唤醒后自动恢复。')) return;
    apiFetch('/api/sleep', { method: 'POST' }).catch(() => {});
  });

  // —— 对话（文件传输助手） ——
  function chatTime(ts) {
    const d = new Date((ts || 0) * 1000);
    const p = (x) => String(x).padStart(2, '0');
    return p(d.getHours()) + ':' + p(d.getMinutes());
  }
  function chatScrollBottom() { chatMsgsEl.scrollTop = chatMsgsEl.scrollHeight; }

  function addChatBubble(m) {
    const mine = m.from === chatMe;
    const wrap = document.createElement('div');
    wrap.className = 'chat-msg ' + (mine ? 'me' : 'peer');
    if (!mine) {
      const sender = document.createElement('div');
      sender.className = 'chat-sender';
      sender.textContent = m.from;
      wrap.appendChild(sender);
    }
    if (m.type === 'text') {
      const b = document.createElement('div');
      b.className = 'chat-bubble';
      b.textContent = m.text;
      wrap.appendChild(b);
    } else if (m.type === 'image') {
      const img = document.createElement('img');
      img.className = 'chat-img';
      img.src = fileURL(m.file);
      img.alt = m.fileName || '';
      img.addEventListener('click', () => openChatMedia(m.file, m.fileName || '图片', 'image'));
      wrap.appendChild(img);
    } else if (m.type === 'video') {
      const v = document.createElement('video');
      v.className = 'chat-video';
      v.controls = true;
      v.playsInline = true;
      v.preload = 'metadata';
      v.poster = '/api/thumb?path=' + encodeURIComponent(m.file);
      v.src = fileURL(m.file);
      wrap.appendChild(v);
    } else {
      const a = document.createElement('a');
      a.className = 'chat-file';
      a.href = fileURL(m.file);
      a.download = m.fileName || '';
      a.textContent = '📄 ' + (m.fileName || '文件') + (m.size ? ' · ' + humanSize(m.size) : '');
      wrap.appendChild(a);
    }
    const t = document.createElement('div');
    t.className = 'chat-time';
    t.textContent = chatTime(m.time);
    wrap.appendChild(t);
    chatMsgsEl.appendChild(wrap);
  }

  function appendChatMessages(msgs) {
    if (!msgs || !msgs.length) return;
    let max = chatLastSeq;
    for (const m of msgs) {
      if (m.seq <= chatLastSeq) continue;
      addChatBubble(m);
      if (m.seq > max) max = m.seq;
    }
    chatLastSeq = max;
    chatScrollBottom();
  }

  async function chatPoll() {
    if (!chatPeer) return;
    try {
      const res = await apiFetch('/api/chat/messages?peer=' + encodeURIComponent(chatPeer) + '&after=' + chatLastSeq);
      if (!res.ok) throw new Error('HTTP ' + res.status);
      const data = await res.json();
      appendChatMessages(data.messages || []);
    } catch (e) {}
  }

  async function openChat() {
    try {
      const res = await apiFetch('/api/chat/state');
      if (!res.ok) throw new Error('HTTP ' + res.status);
      const st = await res.json();
      chatMe = st.me || '';
      chatPeer = st.deviceName || 'HomeBox';
    } catch (e) {
      chatMe = '';
      chatPeer = 'HomeBox';
    }
    chatLastSeq = 0;
    chatMsgsEl.innerHTML = '';
    chatPeerEl.textContent = chatPeer;
    chatInputEl.placeholder = '发送给 ' + chatPeer + '…';
    chatEl.classList.remove('hidden');
    chatPoll();
    chatTimer = setInterval(chatPoll, 3000);
    chatInputEl.focus();
  }

  function closeChat() {
    chatEl.classList.add('hidden');
    if (chatTimer) { clearInterval(chatTimer); chatTimer = null; }
    chatPeer = '';
    chatMe = '';
    chatLastSeq = 0;
    chatMsgsEl.innerHTML = '';
  }

  // 对话里的图片点开全屏预览（不走目录画廊）。
  function openChatMedia(path, name, type) {
    previewType = type;
    previewPath = path;
    previewNameEl.textContent = name;
    previewBodyEl.innerHTML = '';
    resetZoom();
    galleryMode = false;
    let node;
    if (type === 'image') {
      node = document.createElement('img');
      node.className = 'preview-media';
      node.src = fileURL(path);
    } else {
      node = document.createElement('video');
      node.className = 'preview-media';
      node.controls = true;
      node.playsInline = true;
      node.src = fileURL(path);
    }
    previewBodyEl.appendChild(node);
    previewFullscreenEl.classList.add('hidden');
    previewPrevEl.classList.add('hidden');
    previewNextEl.classList.add('hidden');
    setChrome(false);
    updateDownload();
    previewEl.classList.remove('hidden');
  }

  async function sendChatText() {
    const text = chatInputEl.value.trim();
    if (!text || !chatPeer) return;
    chatInputEl.value = '';
    try {
      const res = await apiFetch('/api/chat/text', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ to: chatPeer, text: text }),
      });
      const data = await res.json();
      if (data.ok && data.message) appendChatMessages([data.message]);
      else if (data.error) alert('发送失败：' + data.error);
    } catch (e) {
      alert('发送失败：' + e);
    }
  }

  async function sendChatFiles(files) {
    if (!files || !files.length || !chatPeer) return;
    for (const file of files) {
      const fd = new FormData();
      fd.append('file', file, file.name);
      try {
        const res = await apiFetch('/api/chat/upload?to=' + encodeURIComponent(chatPeer), {
          method: 'POST',
          body: fd,
        });
        const data = await res.json();
        if (data.ok) appendChatMessages(data.messages || []);
        else if (data.error) alert('发送失败：' + data.error);
      } catch (e) {
        alert('发送失败：' + e);
      }
    }
  }

  btnChatEl.addEventListener('click', openChat);
  chatCloseEl.addEventListener('click', closeChat);
  chatSendEl.addEventListener('click', sendChatText);
  chatInputEl.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') { e.preventDefault(); sendChatText(); }
  });
  chatAddEl.addEventListener('click', () => chatFileInputEl.click());
  chatFileInputEl.addEventListener('change', () => {
    if (chatFileInputEl.files && chatFileInputEl.files.length) sendChatFiles(Array.from(chatFileInputEl.files));
    chatFileInputEl.value = '';
  });

  // 启动：优先 URL hash，其次上次访问的目录
  let start = fromHash();
  if (start === '/') {
    try { start = localStorage.getItem('nas-last-path') || '/'; } catch (e) { start = '/'; }
  }
  navigate(start);

  // 心跳：页面打开期间周期刷新会话在线状态（管理面板的移动端指示灯据此判断在线）
  setInterval(() => {
    fetch('/api/ping').then((res) => {
      if (res.status === 401) location.href = '/login.html';
    }).catch(() => {});
  }, 30000);
})();
