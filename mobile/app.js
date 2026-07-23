// ── Goink Mobile Web Client ──
const API = { base: location.origin, ws: null, connOk: false };
const state = {
  page: 'novels', novelId: 0, novelTitle: '', sessionId: null,
  models: [], selectedModel: '', isLoading: false, sessions: [],
  reader: { novelId: 0, chapterId: 0, idx: 0, chapters: [] },
  chaptersCache: {}
};

// ── IndexedDB 离线存储 ──
const DB_NAME = 'goink_offline';
const DB_VERSION = 1;
let dbInstance = null;

function openDB() {
  if (dbInstance) return Promise.resolve(dbInstance);
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION);
    req.onupgradeneeded = (e) => {
      const db = e.target.result;
      if (!db.objectStoreNames.contains('novels')) db.createObjectStore('novels', { keyPath: 'id' });
      if (!db.objectStoreNames.contains('chapters')) db.createObjectStore('chapters', { keyPath: 'id' });
      if (!db.objectStoreNames.contains('characters')) db.createObjectStore('characters', { keyPath: 'id' });
      if (!db.objectStoreNames.contains('timeline')) db.createObjectStore('timeline', { keyPath: 'id' });
      if (!db.objectStoreNames.contains('arcs')) db.createObjectStore('arcs', { keyPath: 'id' });
      if (!db.objectStoreNames.contains('locations')) db.createObjectStore('locations', { keyPath: 'id' });
      if (!db.objectStoreNames.contains('preferences')) db.createObjectStore('preferences', { keyPath: 'id' });
      if (!db.objectStoreNames.contains('settings')) db.createObjectStore('settings', { keyPath: 'key' });
    };
    req.onsuccess = (e) => { dbInstance = e.target.result; resolve(dbInstance); };
    req.onerror = () => reject(req.error);
  });
}

async function dbPut(store, data) {
  const db = await openDB();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(store, 'readwrite');
    const s = tx.objectStore(store);
    if (Array.isArray(data)) data.forEach(item => s.put(item));
    else s.put(data);
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
  });
}

async function dbGetAll(store) {
  const db = await openDB();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(store, 'readonly');
    const req = tx.objectStore(store).getAll();
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

async function dbGet(store, key) {
  const db = await openDB();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(store, 'readonly');
    const req = tx.objectStore(store).get(key);
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

// 从服务器同步数据到 IndexedDB
async function syncToOffline() {
  if (!API.connOk) return;
  try {
    // 同步小说列表
    const novelsRes = await api('/api/novels');
    if (novelsRes.novels) {
      await dbPut('novels', novelsRes.novels);
      // 同步每本小说的章节、角色等
      for (const novel of novelsRes.novels) {
        const [chRes, charRes, tlRes, arcRes, locRes, prefRes] = await Promise.all([
          api(`/api/novels/${novel.id}/chapters`).catch(() => ({})),
          api(`/api/characters?novel_id=${novel.id}`).catch(() => ({})),
          api(`/api/timeline?novel_id=${novel.id}`).catch(() => ({})),
          api(`/api/arcs?novel_id=${novel.id}`).catch(() => ({})),
          api(`/api/locations?novel_id=${novel.id}`).catch(() => ({})),
          api(`/api/preferences?novel_id=${novel.id}`).catch(() => ({})),
        ]);
        if (chRes.chapters) await dbPut('chapters', chRes.chapters);
        if (charRes.characters) await dbPut('characters', charRes.characters);
        if (tlRes.entries) await dbPut('timeline', tlRes.entries);
        if (arcRes.arcs) await dbPut('arcs', arcRes.arcs);
        if (locRes.locations) await dbPut('locations', locRes.locations);
        if (prefRes.preferences) await dbPut('preferences', prefRes.preferences);
      }
    }
  } catch (_) {}
}

// ── HTTP ──
function getToken() { return localStorage.getItem('goink_api_token') || ''; }
function setToken(t) { localStorage.setItem('goink_api_token', t); }

async function api(path, opts = {}) {
  const headers = { 'Content-Type': 'application/json' };
  const token = getToken();
  if (token) headers['Authorization'] = 'Bearer ' + token;
  try {
    const res = await fetch(API.base + path, { method: opts.method || 'GET', headers, body: opts.body ? JSON.stringify(opts.body) : undefined, signal: opts.signal });
    if (res.status === 401) {
      // token 无效，只在未弹窗时显示
      if (!document.getElementById('tokenOverlay')) showTokenPrompt();
      return { error: 'unauthorized' };
    }
    API.connOk = true;
    return res.json();
  } catch (_) {
    // 网络失败，尝试从 IndexedDB 读取
    API.connOk = false;
    return offlineFallback(path);
  }
}

// 离线回退：从 IndexedDB 读取缓存数据
async function offlineFallback(path) {
  try {
    if (path === '/api/novels') {
      const novels = await dbGetAll('novels');
      return { novels };
    }
    const novelMatch = path.match(/\/api\/novels\/(\d+)\/chapters/);
    if (novelMatch) {
      const all = await dbGetAll('chapters');
      return { chapters: all.filter(c => c.novel_id == novelMatch[1]) };
    }
    if (path.startsWith('/api/characters')) {
      const params = new URLSearchParams(path.split('?')[1] || '');
      const nid = params.get('novel_id');
      const all = await dbGetAll('characters');
      return { characters: nid ? all.filter(c => c.novel_id == nid) : all };
    }
    if (path.startsWith('/api/timeline')) {
      const params = new URLSearchParams(path.split('?')[1] || '');
      const nid = params.get('novel_id');
      const all = await dbGetAll('timeline');
      return { entries: nid ? all.filter(e => e.novel_id == nid) : all };
    }
    if (path.startsWith('/api/arcs')) {
      const params = new URLSearchParams(path.split('?')[1] || '');
      const nid = params.get('novel_id');
      const all = await dbGetAll('arcs');
      return { arcs: nid ? all.filter(a => a.novel_id == nid) : all };
    }
    if (path.startsWith('/api/locations')) {
      const params = new URLSearchParams(path.split('?')[1] || '');
      const nid = params.get('novel_id');
      const all = await dbGetAll('locations');
      return { locations: nid ? all.filter(l => l.novel_id == nid) : all };
    }
    if (path.startsWith('/api/preferences')) {
      const params = new URLSearchParams(path.split('?')[1] || '');
      const nid = params.get('novel_id');
      const all = await dbGetAll('preferences');
      return { preferences: nid ? all.filter(p => p.novel_id == nid) : all };
    }
    // 章节内容
    const chMatch = path.match(/\/api\/chapters\/(\d+)/);
    if (chMatch) {
      const ch = await dbGet('chapters', parseInt(chMatch[1]));
      return ch || { error: 'not_found' };
    }
  } catch (_) {}
  return { error: 'offline', _offline: true };
}

// Token 输入弹窗
function showTokenPrompt() {
  // 创建 token 输入弹窗
  let overlay = document.getElementById('tokenOverlay');
  if (overlay) overlay.remove();
  overlay = document.createElement('div');
  overlay.id = 'tokenOverlay';
  overlay.style.cssText = 'position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(0,0,0,.5);z-index:999;display:flex;align-items:center;justify-content:center';
  overlay.innerHTML = `
    <div style="background:var(--surface);border-radius:16px;padding:24px;width:85%;max-width:320px;box-shadow:0 8px 32px rgba(0,0,0,.2)">
      <h3 style="font-size:16px;font-weight:700;margin-bottom:8px;color:var(--accent)">🔐 访问验证</h3>
      <p style="font-size:13px;color:var(--text2);margin-bottom:16px">首次连接需输入令牌，请在桌面端「设置」中查看或扫描二维码。</p>
      <input id="tokenInput" type="text" placeholder="输入 32 位令牌" style="width:100%;padding:10px 14px;border:1px solid var(--border);border-radius:8px;font-size:14px;font-family:monospace;margin-bottom:12px;outline:none;box-sizing:border-box">
      <div style="display:flex;gap:8px;margin-bottom:12px">
        <button id="tokenSave" style="flex:1;padding:10px;border:none;border-radius:8px;background:var(--accent);color:#fff;font-size:14px;font-weight:600;cursor:pointer">连接</button>
        <button id="tokenCancel" style="padding:10px 16px;border:1px solid var(--border);border-radius:8px;background:var(--surface);color:var(--text);font-size:14px;cursor:pointer">跳过</button>
      </div>
      <button id="tokenScan" style="width:100%;padding:10px;border:1px solid var(--border);border-radius:8px;background:var(--surface);color:var(--text);font-size:13px;cursor:pointer;display:flex;align-items:center;justify-content:center;gap:6px">📷 扫描二维码</button>
    </div>`;
  document.body.appendChild(overlay);
  document.getElementById('tokenInput').focus();
  document.getElementById('tokenSave').onclick = () => {
    const val = document.getElementById('tokenInput').value.trim();
    if (val) {
      setToken(val);
      overlay.remove();
      switchPage(state.page);
      toast('令牌已保存');
    }
  };
  document.getElementById('tokenCancel').onclick = () => { overlay.remove(); };
  document.getElementById('tokenInput').onkeydown = (e) => {
    if (e.key === 'Enter') document.getElementById('tokenSave').click();
  };
  document.getElementById('tokenScan').onclick = () => { startQRScan(); };
}

// QR 码扫描
function startQRScan() {
  let overlay = document.getElementById('qrScanOverlay');
  if (overlay) overlay.remove();
  overlay = document.createElement('div');
  overlay.id = 'qrScanOverlay';
  overlay.style.cssText = 'position:fixed;top:0;left:0;right:0;bottom:0;background:#000;z-index:1000;display:flex;flex-direction:column';
  overlay.innerHTML = `
    <div style="flex:1;position:relative;overflow:hidden">
      <video id="qrVideo" style="width:100%;height:100%;object-fit:cover" autoplay playsinline></video>
      <canvas id="qrCanvas" style="display:none"></canvas>
      <div style="position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);width:200px;height:200px;border:2px solid rgba(255,255,255,.7);border-radius:12px;pointer-events:none"></div>
    </div>
    <div style="padding:16px;text-align:center;background:var(--surface)">
      <p style="font-size:14px;color:var(--text);margin-bottom:12px">将二维码放入框内</p>
      <button id="qrCancel" style="padding:10px 32px;border:1px solid var(--border);border-radius:8px;background:var(--surface);color:var(--text);font-size:14px;cursor:pointer">取消</button>
    </div>`;
  document.body.appendChild(overlay);

  const video = document.getElementById('qrVideo');
  const canvas = document.getElementById('qrCanvas');
  const ctx = canvas.getContext('2d', { willReadFrequently: true });
  let scanning = true;

  document.getElementById('qrCancel').onclick = () => {
    scanning = false;
    if (video.srcObject) {
      video.srcObject.getTracks().forEach(t => t.stop());
    }
    overlay.remove();
  };

  navigator.mediaDevices.getUserMedia({ video: { facingMode: 'environment' } })
    .then(stream => {
      video.srcObject = stream;
      video.play();
      scanFrame();
    })
    .catch(() => {
      toast('无法访问摄像头');
      overlay.remove();
    });

  function scanFrame() {
    if (!scanning) return;
    if (video.readyState === video.HAVE_ENOUGH_DATA) {
      canvas.width = video.videoWidth;
      canvas.height = video.videoHeight;
      ctx.drawImage(video, 0, 0, canvas.width, canvas.height);
      const imageData = ctx.getImageData(0, 0, canvas.width, canvas.height);
      const code = jsQR(imageData.data, imageData.width, imageData.height, { inversionAttempts: 'dontInvert' });
      if (code && code.data) {
        scanning = false;
        video.srcObject.getTracks().forEach(t => t.stop());
        overlay.remove();
        // 填入 token 并连接
        const tokenInput = document.getElementById('tokenInput');
        if (tokenInput) tokenInput.value = code.data;
        setToken(code.data);
        document.getElementById('tokenOverlay').remove();
        switchPage(state.page);
        toast('扫码成功，令牌已保存');
        return;
      }
    }
    requestAnimationFrame(scanFrame);
  }
}

// ── 全局主题 ──
function getTheme() { return localStorage.getItem('goink_theme') || 'light'; }
function applyTheme(theme) {
  document.documentElement.setAttribute('data-theme', theme);
  localStorage.setItem('goink_theme', theme);
  const mc = document.querySelector('meta[name="theme-color"]');
  if (mc) mc.content = theme === 'dark' ? '#121212' : '#F5F0E8';
}
function toggleTheme() {
  const next = getTheme() === 'dark' ? 'light' : 'dark';
  applyTheme(next);
  if (state.page === 'settings') loadSettings();
}
applyTheme(getTheme());

// ── 国际化 i18n ──
const LANGS = {
  zh: {
    bookshelf: '书架', chat: '对话', settings: '设置', detail: '小说详情',
    novels_empty: '书架空空如也', loading: '加载中', load_fail: '加载失败',
    chapters: '章节', characters: '角色', timeline: '时间线', arcs: '弧线',
    reader: '读者', preferences: '偏好', locations: '地点',
    no_chapters: '暂无章节', no_characters: '暂无角色', no_timeline: '暂无时间线',
    no_arcs: '暂无弧线', no_reader: '暂无读者认知', no_prefs: '暂无偏好', no_locations: '暂无地点',
    resolved: '已解决', pending: '待处理', other: '其他',
    known: '已知信息', suspense: '悬念', misconception: '误解',
    global: '全局', novel_only: '小说专属', uncategorized: '未分类',
    target_ch: '目标', source_ch: '来源', resolved_ch: '解决',
    prev_ch: '‹ 上一章', next_ch: '下一章 ›',
    chapter_list: '章节目录', close: '关闭', settings_title: '设置',
    model: '模型', current_model: '当前模型', appearance: '外观',
    dark_mode: '深色模式', light_mode: '浅色模式',
    server: '服务器', status: '状态', connected: '已连接', disconnected: '未连接',
    input_msg: '输入消息...', new_chat: '新对话', history: '历史',
    no_sessions: '暂无历史会话', start_chat: '开始新的对话', start_hint: '输入消息开始创作',
    copied: '已复制', copy_fail: '复制失败', switch_ok: '已切换', switch_fail: '切换失败',
    chapter: '章', words: '字', roles: '角色', current: '当前',
    thinking: '思考', cancel: '取消', stop: '停止',
    search: '搜索...', no_results: '无结果',
    position: '定位', personality: '性格', background: '背景',
    importance: '重要度', source_label: '来源',
    type: '类型', content: '内容', category: '分类', scope: '范围',
    planted_ch: '埋设章节', revealed_ch: '揭示章节', related_truth: '关联真相',
    arc_type: '弧线类型', status_label: '状态', nodes: '节点',
    location_type: '地点类型', tags: '标签', description: '描述',
  },
  en: {
    bookshelf: 'Bookshelf', chat: 'Chat', settings: 'Settings', detail: 'Novel Detail',
    novels_empty: 'Your bookshelf is empty', loading: 'Loading', load_fail: 'Failed to load',
    chapters: 'Chapters', characters: 'Characters', timeline: 'Timeline', arcs: 'Arcs',
    reader: 'Reader', preferences: 'Preferences', locations: 'Locations',
    no_chapters: 'No chapters yet', no_characters: 'No characters yet', no_timeline: 'No timeline yet',
    no_arcs: 'No arcs yet', no_reader: 'No reader perspectives yet', no_prefs: 'No preferences yet', no_locations: 'No locations yet',
    resolved: 'Resolved', pending: 'Pending', other: 'Other',
    known: 'Known', suspense: 'Suspense', misconception: 'Misconception',
    global: 'Global', novel_only: 'Novel only', uncategorized: 'Uncategorized',
    target_ch: 'Target', source_ch: 'Source', resolved_ch: 'Resolved',
    prev_ch: '‹ Prev', next_ch: 'Next ›',
    chapter_list: 'Chapter List', close: 'Close', settings_title: 'Settings',
    model: 'Model', current_model: 'Current Model', appearance: 'Appearance',
    dark_mode: 'Dark Mode', light_mode: 'Light Mode',
    server: 'Server', status: 'Status', connected: 'Connected', disconnected: 'Disconnected',
    input_msg: 'Type a message...', new_chat: 'New Chat', history: 'History',
    no_sessions: 'No sessions yet', start_chat: 'Start a new conversation', start_hint: 'Type to begin',
    copied: 'Copied', copy_fail: 'Copy failed', switch_ok: 'Switched', switch_fail: 'Switch failed',
    chapter: 'Ch', words: 'words', roles: 'roles', current: 'Current',
    thinking: 'Thinking', cancel: 'Cancel', stop: 'Stop',
    search: 'Search...', no_results: 'No results',
    position: 'Role', personality: 'Personality', background: 'Background',
    importance: 'Importance', source_label: 'Source',
    type: 'Type', content: 'Content', category: 'Category', scope: 'Scope',
    planted_ch: 'Planted', revealed_ch: 'Revealed', related_truth: 'Related Truth',
    arc_type: 'Arc Type', status_label: 'Status', nodes: 'Nodes',
    location_type: 'Location Type', tags: 'Tags', description: 'Description',
  }
};
function getLang() { return localStorage.getItem('goink_lang') || 'zh'; }
function setLang(lang) { localStorage.setItem('goink_lang', lang); }
function t(key) { const lang = getLang(); return (LANGS[lang] && LANGS[lang][key]) || LANGS.zh[key] || key; }
function toggleLang() {
  const next = getLang() === 'zh' ? 'en' : 'zh';
  setLang(next);
  // 刷新当前页面
  switchPage(state.page);
}

// ── Utils ──
function esc(s) { if (!s) return ''; const d = document.createElement('div'); d.textContent = s; return d.innerHTML; }
function toast(msg, dur = 2000) { const t = document.getElementById('toast'); t.textContent = msg; t.classList.remove('hidden'); t.classList.add('show'); setTimeout(() => { t.classList.remove('show'); setTimeout(() => t.classList.add('hidden'), 300); }, dur); }
function openSheet(id) { document.getElementById(id).classList.remove('hidden'); }
function closeSheet(id) { const el = document.getElementById(id); if (el) el.classList.add('hidden'); }

// 复制文本
function copyText(t) {
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(t).then(() => toast('已复制')).catch(() => fallbackCopy(t));
  } else { fallbackCopy(t); }
}
function fallbackCopy(t) {
  const ta = document.createElement('textarea');
  ta.value = t; ta.style.position = 'fixed'; ta.style.opacity = '0';
  document.body.appendChild(ta); ta.select();
  try { document.execCommand('copy'); toast('已复制'); } catch (_) { toast('复制失败'); }
  document.body.removeChild(ta);
}

// 分类折叠
function toggleGroup(el) {
  const body = el.querySelector('.collapse-body');
  const icon = el.querySelector('.collapse-icon');
  if (body) { body.classList.toggle('hidden'); icon.textContent = body.classList.contains('hidden') ? '▸' : '▾'; }
}
function cardClick(ev, fn) { ev.stopPropagation(); fn(); }

// 提取列表数据
function extractItems(r) {
  if (!r || typeof r !== 'object') return Array.isArray(r) ? r : [];
  for (const k of Object.keys(r)) { if (Array.isArray(r[k])) return r[k]; }
  for (const k of Object.keys(r)) { if (r[k] && typeof r[k] === 'object' && Array.isArray(r[k].items)) return r[k].items; }
  if (Array.isArray(r.items)) return r.items;
  return [];
}

// ── 页面切换 ──
function switchPage(page) {
  state.page = page;
  document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
  document.getElementById('page-' + page)?.classList.add('active');
  document.querySelectorAll('.nav-item').forEach(n => n.classList.toggle('active', n.dataset.page === page));
  const titles = { novels: t('bookshelf'), chat: t('chat'), settings: t('settings'), 'novel-detail': state.novelTitle || t('detail') };
  document.getElementById('pageTitle').textContent = titles[page] || 'Goink';
  const actions = document.getElementById('headerActions');
  if (page === 'chat') {
    actions.innerHTML = '<button onclick="newChat()" title="新对话"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg></button><button onclick="showSessions()" title="历史"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><polyline points="12,6 12,12 16,14"/></svg></button>';
    const banner = document.getElementById('chatBanner');
    if (state.novelTitle) { banner.innerHTML = '<span>📖</span> ' + esc(state.novelTitle); banner.style.display = 'flex'; } else { banner.style.display = 'none'; }
    loadModels(); loadSessions();
  } else { actions.innerHTML = ''; if (document.getElementById('chatBanner')) document.getElementById('chatBanner').style.display = 'none'; }
  if (page === 'novels') loadNovels();
  if (page === 'chat') { loadModels(); loadSessions(); }
  if (page === 'settings') loadSettings();
  if (page === 'novel-detail') loadNovelDetail();
}

// ── WebSocket ──
function connectWS() {
  try {
    const token = getToken();
    const wsUrl = API.base.replace('http', 'ws') + '/api/ws' + (token ? '?token=' + token : '');
    const ws = new WebSocket(wsUrl);
    API.ws = ws;
    ws.onopen = () => { API.connOk = true; };
    ws.onmessage = (e) => { try { const ev = JSON.parse(e.data); if (ev.type === 'model_changed') { state.selectedModel = ev.model_key || ''; toast('模型已切换'); if (state.page === 'settings') loadSettings(); } if (ev.type === 'chat:done' && state.page === 'chat') loadSessions(); } catch (_) {} };
    ws.onclose = () => { API.connOk = false; setTimeout(connectWS, 5000); };
  } catch (_) { setTimeout(connectWS, 5000); }
}

// ═══════════ 章节缓存 ═══════════
async function getChapters(novelId) {
  if (state.chaptersCache[novelId]) return state.chaptersCache[novelId];
  const r = await api(`/api/novels/${novelId}/chapters?page=1&size=999`);
  state.chaptersCache[novelId] = r.chapters || [];
  return state.chaptersCache[novelId];
}

// ═══════════ 小说列表（首页）═══════════
async function loadNovels() {
  try {
    const r = await api('/api/novels');
    const novels = r.novels || [];
    const el = document.getElementById('novelList');
    if (!novels.length) { el.innerHTML = `<div class="empty-state"><p>${t('novels_empty')}</p></div>`; return; }
    // 在线时后台同步到 IndexedDB
    if (API.connOk && !r._offline) syncToOffline();
    // 离线提示
    if (r._offline) toast('📡 离线模式，显示缓存数据');
    const colors = ['#6366F1','#EC4899','#10B981','#F59E0B','#EF4444','#8B5CF6','#14B8A6','#F97316'];
    const enriched = await Promise.all(novels.map(async (n, i) => {
      const [chRes, charRes] = await Promise.all([api(`/api/novels/${n.id}/chapters?page=1&size=9999`), api(`/api/characters?novel_id=${n.id}`)]);
      const chs = chRes.chapters || [];
      const chars = charRes.characters || [];
      return { ...n, color: colors[i % colors.length], chapterCount: chs.length, charCount: chars.length, totalWords: chs.reduce((s, c) => s + (c.word_count || 0), 0), lastUpdated: chs.length ? chs[0]?.updated_at?.slice(0, 10) : '' };
    }));
    el.innerHTML = enriched.map(n => {
      const isActive = n.id === state.novelId;
      const wordStr = n.totalWords >= 10000 ? (n.totalWords / 10000).toFixed(1) + '万字' : n.totalWords ? n.totalWords + '字' : '';
      return `<div class="novel-card${isActive ? ' novel-active' : ''}" onclick="openNovel(${n.id},'${esc(n.title)}')"><div class="novel-card-top"><div class="novel-icon" style="background:${n.color}">${(n.title||'?')[0]}</div><div class="novel-info"><div class="novel-title">${esc(n.title)}</div>${n.genre ? `<span class="novel-genre" style="background:${n.color}22;color:${n.color}">${esc(n.genre)}</span>` : ''}${isActive ? `<span class="novel-genre" style="background:rgba(139,105,20,.15);color:var(--accent)">${t('current')}</span>` : ''}${n.description ? `<div class="novel-desc">${esc(n.description)}</div>` : ''}</div></div><div class="novel-meta"><span>📖 ${n.chapterCount}${t('chapters')}</span><span>👤 ${n.charCount}${t('characters')}</span>${wordStr ? `<span>📝 ${wordStr}</span>` : ''}${n.lastUpdated ? `<span>🕐 ${n.lastUpdated}</span>` : ''}</div><span class="arrow">›</span></div>`;
    }).join('');
  } catch (_) { document.getElementById('novelList').innerHTML = `<div class="empty-state"><p>${t('load_fail')}</p></div>`; }
}

function openNovel(id, title) { state.novelId = id; state.novelTitle = title; switchPage('novel-detail'); }

// ═══════════ 小说详情 ═══════════
let novelTab = 'chapters';
const TABS = [{ id: 'chapters', label: () => '📖 ' + t('chapters') }, { id: 'characters', label: () => '👤 ' + t('characters') }, { id: 'timeline', label: () => '⏱ ' + t('timeline') }, { id: 'arcs', label: () => '🔮 ' + t('arcs') }, { id: 'reader', label: () => '👁 ' + t('reader') }, { id: 'preferences', label: () => '⚙ ' + t('preferences') }, { id: 'locations', label: () => '📍 ' + t('locations') }];

async function loadNovelDetail() {
  // 标题已在顶部导航栏显示，不再重复
  document.getElementById('novelDetailHeader').innerHTML = '';
  document.getElementById('novelDetailTabs').innerHTML = TABS.map(tab => `<button class="tab-item ${tab.id === novelTab ? 'active' : ''}" onclick="switchTab('${tab.id}')">${tab.label()}</button>`).join('');
  switchTab(novelTab);
}

function switchTab(tab) { novelTab = tab; document.querySelectorAll('.tab-item').forEach((t, i) => t.classList.toggle('active', TABS[i].id === tab)); loadTabContent(tab); }

// ═══════════ 小说详情 Tab 内容 ═══════════
async function loadTabContent(tab) {
  const el = document.getElementById('novelDetailContent');
  el.innerHTML = `<div class="empty-state"><div class="loading-dots">${t('loading')}</div></div>`;
  const nId = state.novelId;
  try {
    switch (tab) {
      case 'chapters': {
        const chs = await getChapters(nId);
        el.innerHTML = chs.length ? chs.map(c => `<div class="data-card" onclick="readChapter(${nId}, ${c.id})"><div class="data-card-header"><div class="data-card-icon" style="background:rgba(139,105,20,.1);color:var(--accent)">${c.chapter_number}</div><div class="data-card-body"><div class="data-card-title">${esc(c.title)}</div><div class="data-card-sub">${c.word_count} ${t('words')}</div></div><span class="data-card-arrow">›</span></div></div>`).join('') : `<div class="empty-state"><p>${t('no_chapters')}</p></div>`;
        break;
      }
      case 'characters': {
        const r = await api(`/api/characters?novel_id=${nId}`);
        const chars = r.characters || [];
        el.innerHTML = chars.length ? chars.map(c => { let preview = c.role || ''; if (!preview && c.personality) try { const p = JSON.parse(c.personality); const vals = Object.values(p).filter(v => typeof v === 'string' && v.length < 40); preview = vals.slice(0, 2).join(' · '); } catch (_) { preview = (c.personality||'').slice(0, 40) || ''; } return `<div class="data-card" onclick="cardClick(event, () => showDetail('${esc(c.name)}', formatCharacter(${JSON.stringify(c).replace(/"/g, '&quot;')})))"><div class="data-card-header"><div class="data-card-icon" style="background:rgba(59,130,246,.1);color:#3B82F6">${(c.name||'?')[0]}</div><div class="data-card-body"><div class="data-card-title">${esc(c.name)}</div>${preview ? `<div class="data-card-sub">${esc(preview)}</div>` : ''}</div><span class="data-card-arrow">›</span></div></div>`; }).join('') : `<div class="empty-state"><p>${t('no_characters')}</p></div>`;
        break;
      }
      case 'timeline': {
        const r = await api(`/api/timeline?novel_id=${nId}&page=1&size=500`);
        const items = extractItems(r);
        const resolved = items.filter(i => i.status === 'resolved');
        const pending = items.filter(i => i.status === 'pending');
        const other = items.filter(i => i.status !== 'resolved' && i.status !== 'pending');
        el.innerHTML = items.length ? `<div class="collapse-group" onclick="toggleGroup(this)"><div class="collapse-header"><span class="collapse-icon">▸</span><span class="collapse-title">${t('resolved')}</span><span class="collapse-count">${resolved.length}</span></div><div class="collapse-body hidden">${resolved.map(i => timelineCard(i)).join('')}</div></div><div class="collapse-group" onclick="toggleGroup(this)"><div class="collapse-header"><span class="collapse-icon">▸</span><span class="collapse-title">${t('pending')}</span><span class="collapse-count">${pending.length}</span></div><div class="collapse-body hidden">${pending.map(i => timelineCard(i)).join('')}</div></div><div class="collapse-group" onclick="toggleGroup(this)"><div class="collapse-header"><span class="collapse-icon">▸</span><span class="collapse-title">${t('other')}</span><span class="collapse-count">${other.length}</span></div><div class="collapse-body hidden">${other.map(i => timelineCard(i)).join('')}</div></div>` : `<div class="empty-state"><p>${t('no_timeline')}</p></div>`;
        break;
      }
      case 'arcs': {
        const [arcRes, nodeRes] = await Promise.all([api(`/api/arcs?novel_id=${nId}&page=1&size=500`), api(`/api/arc-nodes?novel_id=${nId}`)]);
        const arcs = extractItems(arcRes);
        const allNodes = nodeRes.nodes || [];
        el.innerHTML = arcs.length ? arcs.map(a => {
          const nodes = allNodes.filter(n => n.story_arc_id === a.id);
          const completed = nodes.filter(n => n.status === 'completed').length;
          const sc = a.status === 'active' ? 'var(--success)' : a.status === 'completed' ? '#3B82F6' : a.status === 'paused' ? 'var(--warning)' : 'var(--text2)';
          return `<div class="data-card" onclick="cardClick(event, () => showDetail('${esc(a.name||'')}', formatArc(${JSON.stringify(a).replace(/"/g, '&quot;')}, ${JSON.stringify(nodes).replace(/"/g, '&quot;')})))"><div class="data-card-header"><div class="data-card-icon" style="background:${sc}15;color:${sc}">A</div><div class="data-card-body"><div class="data-card-title">${esc(a.name||'无名弧线')}</div><div class="data-card-sub">${esc((a.description||'').slice(0,50))}</div><div class="data-card-tags">${nodes.length ? `<span class="tag tag-sm" style="background:rgba(99,102,241,.1);color:#6366F1">${completed}/${nodes.length}节点</span>` : ''}${a.arc_type ? `<span class="tag tag-sm" style="background:var(--surface2);color:var(--text2)">${esc(a.arc_type)}</span>` : ''}${a.status ? `<span class="tag tag-sm" style="background:${sc}15;color:${sc}">${esc(a.status)}</span>` : ''}</div></div><span class="data-card-arrow">›</span></div></div>`;
        }).join('') : `<div class="empty-state"><p>${t('no_arcs')}</p></div>`;
        break;
      }
      case 'reader': {
        const r = await api(`/api/reader?novel_id=${nId}&page=1&size=500`);
        const items = extractItems(r);
        const known = items.filter(i => i.type === 'known');
        const suspense = items.filter(i => i.type === 'suspense');
        const misconception = items.filter(i => i.type === 'misconception');
        const mkCard = (i, tc, tl) => `<div class="data-card" onclick="cardClick(event, () => showDetail('${esc(tl)}', formatReader(${JSON.stringify(i).replace(/"/g, '&quot;')})))"><div class="data-card-header"><div class="data-card-icon" style="background:${tc}15;color:${tc}">R</div><div class="data-card-body"><div class="data-card-title">${esc((i.content||'').slice(0,35))}</div><div class="data-card-tags"><span class="tag tag-sm" style="background:${tc}15;color:${tc}">${tl}</span>${i.planted_chapter ? `<span class="tag tag-sm" style="background:var(--surface2);color:var(--text2)">第${i.planted_chapter}章</span>` : ''}${i.revealed_chapter ? `<span class="tag tag-sm" style="background:rgba(91,140,90,.15);color:var(--success)">揭示${i.revealed_chapter}</span>` : ''}</div></div><span class="data-card-arrow">›</span></div></div>`;
        el.innerHTML = items.length ? `<div class="collapse-group" onclick="toggleGroup(this)"><div class="collapse-header"><span class="collapse-icon">▸</span><span class="collapse-title">${t('known')}</span><span class="collapse-count">${known.length}</span></div><div class="collapse-body hidden">${known.map(i => mkCard(i, 'var(--success)', t('known'))).join('')}</div></div><div class="collapse-group" onclick="toggleGroup(this)"><div class="collapse-header"><span class="collapse-icon">▸</span><span class="collapse-title">${t('suspense')}</span><span class="collapse-count">${suspense.length}</span></div><div class="collapse-body hidden">${suspense.map(i => mkCard(i, 'var(--warning)', t('suspense'))).join('')}</div></div><div class="collapse-group" onclick="toggleGroup(this)"><div class="collapse-header"><span class="collapse-icon">▸</span><span class="collapse-title">${t('misconception')}</span><span class="collapse-count">${misconception.length}</span></div><div class="collapse-body hidden">${misconception.map(i => mkCard(i, 'var(--error)', t('misconception'))).join('')}</div></div>` : `<div class="empty-state"><p>${t('no_reader')}</p></div>`;
        break;
      }
      case 'preferences': {
        const r = await api(`/api/preferences?novel_id=${nId}&page=1&size=500`);
        const items = extractItems(r);
        el.innerHTML = items.length ? items.map(i => `<div class="data-card" onclick="cardClick(event, () => showDetail('${esc(i.category||t('uncategorized'))}', formatPreference(${JSON.stringify(i).replace(/"/g, '&quot;')})))"><div class="data-card-header"><div class="data-card-icon" style="background:rgba(245,158,11,.1);color:var(--warning)">P</div><div class="data-card-body"><div class="data-card-title">${esc(i.category||t('uncategorized'))}</div><div class="data-card-sub">${esc((i.content||'').slice(0,60))}</div><div class="data-card-tags"><span class="tag tag-sm" style="background:${i.is_global ? 'rgba(139,105,20,.1)' : 'var(--surface2)'};color:${i.is_global ? 'var(--accent)' : 'var(--text2)'}">${i.is_global ? t('global') : t('novel_only')}</span></div></div><span class="data-card-arrow">›</span></div></div>`).join('') : `<div class="empty-state"><p>${t('no_prefs')}</p></div>`;
        break;
      }
      case 'locations': {
        const r = await api(`/api/locations?novel_id=${nId}&page=1&size=500`);
        const items = extractItems(r);
        el.innerHTML = items.length ? items.map(i => {
          let preview = i.description ? esc(i.description.slice(0,40)) : (i.location_type ? esc(i.location_type) : '');
          return `<div class="data-card" onclick="cardClick(event, () => showDetail('${esc(i.name||'')}', formatLocation(${JSON.stringify(i).replace(/"/g, '&quot;')})))"><div class="data-card-header"><div class="data-card-icon" style="background:rgba(16,185,129,.1);color:var(--success)">L</div><div class="data-card-body"><div class="data-card-title">${esc(i.name||'无名')}</div>${preview ? `<div class="data-card-sub">${preview}</div>` : ''}${i.location_type ? `<div class="data-card-tags"><span class="tag tag-sm" style="background:rgba(16,185,129,.1);color:var(--success)">${esc(i.location_type)}</span></div>` : ''}</div><span class="data-card-arrow">›</span></div></div>`;
        }).join('') : `<div class="empty-state"><p>${t('no_locations')}</p></div>`;
        break;
      }
    }
  } catch (_) { el.innerHTML = '<div class="empty-state"><p>加载失败</p></div>'; }
}

// ═══════════ 全屏沉浸阅读器 ═══════════
let scrollThrottleTimer = null;

async function readChapter(novelId, chapterId) {
  const chapters = await getChapters(novelId);
  if (!chapters.length) { toast('暂无章节'); return; }
  const idx = chapters.findIndex(c => c.id === chapterId);
  if (idx < 0) { toast('章节未找到'); return; }
  state.reader = { novelId, chapterId, idx, chapters };

  // 获取章节内容
  let content = '';
  try {
    const r = await api(`/api/chapters/${chapterId}`);
    content = r.content || '';
    if (!state.chaptersCache[novelId]) state.chaptersCache[novelId] = chapters;
  } catch (_) { toast('加载失败'); return; }

  // 渲染阅读器
  renderReader(content);
  // 隐藏导航栏
  document.body.classList.add('reader-active');
  // 打开全屏
  openSheet('readerSheet');
}

function renderReader(content) {
  const { novelId, idx, chapters } = state.reader;
  const total = chapters.length;
  const ch = chapters[idx];
  const title = ch ? `第${ch.chapter_number}章 ${ch.title}` : '';
  // 动态设置按钮文本
  document.getElementById('readerPrev').textContent = '‹ ' + t('prev_ch').replace('‹ ','').replace(' Prev','');
  document.getElementById('readerNext').textContent = t('next_ch').replace(' ›','').replace('Next ›','') + ' ›';

  // 读取阅读设置
  const rs = loadReaderSettings();
  // 读取上次进度
  const savedScroll = localStorage.getItem(`reader_progress_${novelId}_${ch?.id}`) || '0';

  document.getElementById('readerTitle').textContent = title;
  document.getElementById('readerContent').style.fontSize = rs.fontSize + 'px';
  document.getElementById('readerContent').style.lineHeight = rs.lineHeight;
  document.getElementById('readerContent').style.color = rs.textColor;
  document.getElementById('readerContent').style.backgroundColor = rs.bgColor;
  // 微信兼容
  document.getElementById('readerContent').style.setProperty('color', rs.textColor, 'important');
  document.getElementById('readerContent').style.setProperty('background-color', rs.bgColor, 'important');
  // 渲染内容
  document.getElementById('readerContent').innerHTML = marked.parse(content);
  // 更新进度
  updateReaderProgress(idx, total);
  // 更新按钮状态
  updateNavButtons(idx, total);
  // 更新设置控件
  updateSettingsUI(rs);
  // 恢复滚动
  setTimeout(() => {
    const scrollEl = document.getElementById('readerContent');
    if (scrollEl && savedScroll > 0) {
      scrollEl.scrollTop = scrollEl.scrollHeight * (parseFloat(savedScroll) / 100);
    }
  }, 100);
  // 绑定滚动保存
  const scrollEl = document.getElementById('readerContent');
  scrollEl.onscroll = () => { if (scrollThrottleTimer) return; scrollThrottleTimer = setTimeout(() => { scrollThrottleTimer = null; saveReaderScroll(); }, 500); };
  // 绑定翻页点击
  scrollEl.onclick = (e) => {
    const rect = scrollEl.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const w = rect.width;
    if (x < w * 0.3) prevChapter();
    else if (x > w * 0.7) nextChapter();
  };
}

function updateReaderProgress(idx, total) {
  const ch = state.reader.chapters[idx];
  const chNum = ch ? ch.chapter_number : (total - idx);
  document.getElementById('readerProgress').textContent = `${chNum}/${total}`;
  // 章节降序：idx=0 是最新章(最大号)，idx=total-1 是第1章(最小号)
  // 上一章（章节号减小）= idx+1，下一章（章节号增大）= idx-1
  document.getElementById('readerPrev').disabled = idx >= total - 1;
  document.getElementById('readerNext').disabled = idx <= 0;
}

function updateNavButtons(idx, total) {
  document.getElementById('readerPrev').disabled = idx >= total - 1;
  document.getElementById('readerNext').disabled = idx <= 0;
}

// 上一章 = 章节号减小 = idx+1
function prevChapter() {
  const { novelId, idx, chapters } = state.reader;
  if (idx >= chapters.length - 1) return;
  state.reader.idx = idx + 1;
  const ch = chapters[idx + 1];
  if (ch) readChapter(novelId, ch.id);
}

// 下一章 = 章节号增大 = idx-1
function nextChapter() {
  const { novelId, idx } = state.reader;
  if (idx <= 0) return;
  state.reader.idx = idx - 1;
  const ch = state.reader.chapters[idx - 1];
  if (ch) readChapter(novelId, ch.id);
}

// 章节目录（内嵌在阅读器内）
function showChapterList() {
  const panel = document.getElementById('chapterListPanel');
  const { chapters, idx } = state.reader;
  const el = document.getElementById('chapterListBody');
  el.innerHTML = chapters.map((c, i) => {
    const isCurrent = i === idx;
    return `<div class="ch-list-item${isCurrent ? ' ch-list-active' : ''}" onclick="jumpToChapter(${i})"><span class="ch-list-num">${c.chapter_number}</span><span class="ch-list-title">${esc(c.title||'')}</span></div>`;
  }).join('');
  panel.classList.toggle('hidden');
  // 滚动到当前章节
  if (!panel.classList.contains('hidden')) {
    setTimeout(() => { const active = el.querySelector('.ch-list-active'); if (active) active.scrollIntoView({ block: 'center' }); }, 100);
  }
}

function jumpToChapter(idx) {
  state.reader.idx = idx;
  const ch = state.reader.chapters[idx];
  document.getElementById('chapterListPanel').classList.add('hidden');
  if (ch) readChapter(state.reader.novelId, ch.id);
}

// 深浅模式切换（全局）
function toggleReaderTheme() {
  toggleTheme();
  // 同步阅读器背景
  const isDark = getTheme() === 'dark';
  const content = document.getElementById('readerContent');
  if (content) {
    const bgColor = isDark ? '#1E1E1E' : '#FFFEF9';
    const textColor = isDark ? '#E8E0D0' : '#3E3427';
    content.style.setProperty('background-color', bgColor, 'important');
    content.style.setProperty('color', textColor, 'important');
  }
  document.getElementById('readerSheet').style.backgroundColor = isDark ? '#121212' : 'var(--bg)';
}

function saveReaderScroll() {
  const { novelId, idx, chapters } = state.reader;
  const ch = chapters[idx];
  if (!ch) return;
  const scrollEl = document.getElementById('readerContent');
  if (!scrollEl || !scrollEl.scrollHeight) return;
  const pct = (scrollEl.scrollTop / scrollEl.scrollHeight * 100).toFixed(2);
  localStorage.setItem(`reader_progress_${novelId}_${ch.id}`, pct);
}

function closeReader() {
  saveReaderScroll();
  if (scrollThrottleTimer) { clearTimeout(scrollThrottleTimer); scrollThrottleTimer = null; }
  closeSheet('readerSheet');
  document.body.classList.remove('reader-active');
  state.reader = { novelId: 0, chapterId: 0, idx: 0, chapters: [] };
}

// 切换阅读器设置面板
function toggleReaderSettings() {
  const panel = document.getElementById('readerSettingsPanel');
  panel.classList.toggle('hidden');
}

// 阅读设置
function loadReaderSettings() {
  try { return JSON.parse(localStorage.getItem('reader_settings')) || { fontSize: 17, lineHeight: 1.8, bgColor: '#FFFEF9', textColor: '#3E3427' }; } catch (_) { return { fontSize: 17, lineHeight: 1.8, bgColor: '#FFFEF9', textColor: '#3E3427' }; }
}
function saveReaderSettings(rs) { localStorage.setItem('reader_settings', JSON.stringify(rs)); }
function updateSettingsUI(rs) {
  document.getElementById('rsFontSize').value = rs.fontSize;
  document.getElementById('rsFontSizeVal').textContent = rs.fontSize;
  document.getElementById('rsLineHeight').value = rs.lineHeight;
  document.getElementById('rsLineHeightVal').textContent = rs.lineHeight;
}
function updateReaderSetting(key, val) {
  const rs = loadReaderSettings();
  rs[key] = parseFloat(val) || val;
  saveReaderSettings(rs);
  const content = document.getElementById('readerContent');
  if (!content) return;
  if (key === 'fontSize') { content.style.fontSize = rs.fontSize + 'px'; content.style.setProperty('font-size', rs.fontSize + 'px', 'important'); document.getElementById('rsFontSizeVal').textContent = rs.fontSize; }
  if (key === 'lineHeight') { content.style.lineHeight = rs.lineHeight; content.style.setProperty('line-height', rs.lineHeight, 'important'); document.getElementById('rsLineHeightVal').textContent = rs.lineHeight; }
  if (key === 'bgColor') { content.style.backgroundColor = rs.bgColor; content.style.setProperty('background-color', rs.bgColor, 'important'); }
  if (key === 'textColor') { content.style.color = rs.textColor; content.style.setProperty('color', rs.textColor, 'important'); }
}

// ═══════════ 时间线卡片 ═══════════
function timelineCard(t) {
  const sc = t.status === 'resolved' ? 'var(--success)' : t.status === 'pending' ? 'var(--warning)' : 'var(--text2)';
  const sl = t.status === 'resolved' ? '已解决' : t.status === 'pending' ? '待处理' : t.status || '';
  let tags = '';
  if (t.target_chapter) tags += `<span class="tag tag-sm" style="background:rgba(139,105,20,.1);color:var(--accent)">目标:第${t.target_chapter}</span>`;
  if (t.source_chapter_id) tags += `<span class="tag tag-sm" style="background:var(--surface2);color:var(--text2)">来源:第${t.source_chapter_id}</span>`;
  if (t.resolved_chapter_id) tags += `<span class="tag tag-sm" style="background:rgba(91,140,90,.15);color:var(--success)">解决:第${t.resolved_chapter_id}</span>`;
  if (t.importance) { let stars = ''; for (let i = 0; i < t.importance; i++) stars += '★'; for (let i = t.importance; i < 5; i++) stars += '☆'; tags += `<span class="tag tag-sm" style="background:rgba(245,158,11,.1);color:var(--warning)">${stars}</span>`; }
  return `<div class="data-card" onclick="cardClick(event, () => showDetail('${esc(t.title||'')}', formatTimeline(${JSON.stringify(t).replace(/"/g, '&quot;')})))"><div class="data-card-header"><div class="data-card-icon" style="background:${sc}15;color:${sc}">T</div><div class="data-card-body"><div class="data-card-title">${esc(t.title||'')}</div><div class="data-card-sub">${esc((t.content||'').slice(0,50))}</div><div class="data-card-tags"><span class="tag tag-sm" style="background:${sc}15;color:${sc}">${sl}</span>${tags}</div></div><span class="data-card-arrow">›</span></div></div>`;
}

// ═══════════ 详情弹窗 ═══════════
function showDetail(title, body) {
  document.getElementById('detailTitle').textContent = title;
  document.getElementById('detailBody').innerHTML = body;
  openSheet('detailSheet');
}

// ═══════════ 详情格式化 ═══════════
function formatCharacter(c) {
  let h = '';
  if (c.name) h += `<div style="font-size:18px;font-weight:700;margin-bottom:8px">${esc(c.name)}</div>`;
  if (c.role) h += `<div class="info-row"><span class="info-label">定位</span><span>${esc(c.role)}</span></div>`;
  if (c.personality) {
    try { const p = JSON.parse(c.personality); Object.keys(p).forEach(k => { const v = String(p[k]); h += `<div class="info-row"><span class="info-label">${esc(k)}</span><span>${esc(v)}</span></div>`; }); } catch (_) { h += `<div class="info-row"><span class="info-label">性格</span><span>${esc(c.personality)}</span></div>`; }
  }
  if (c.background) h += `<div class="info-row"><span class="info-label">背景</span><span>${esc(c.background)}</span></div>`;
  return h;
}

function formatTimeline(t) {
  let h = '';
  if (t.title) h += `<div style="font-size:17px;font-weight:700;margin-bottom:8px">${esc(t.title)}</div>`;
  if (t.category) h += `<div class="info-row"><span class="info-label">分类</span><span>${esc(t.category)}</span></div>`;
  if (t.status) { const sc = t.status === 'resolved' ? 'var(--success)' : t.status === 'pending' ? 'var(--warning)' : 'var(--text2)'; h += `<div class="info-row"><span class="info-label">状态</span><span style="color:${sc};font-weight:600">${esc(t.status)}</span></div>`; }
  if (t.target_chapter) h += `<div class="info-row"><span class="info-label">目标章节</span><span>第${t.target_chapter}章</span></div>`;
  if (t.source_chapter_id) h += `<div class="info-row"><span class="info-label">来源章节</span><span>第${t.source_chapter_id}章</span></div>`;
  if (t.resolved_chapter_id) h += `<div class="info-row"><span class="info-label">解决章节</span><span>第${t.resolved_chapter_id}章</span></div>`;
  if (t.importance) { h += '<div class="info-row"><span class="info-label">重要度</span><span>'; for (let i = 0; i < t.importance; i++) h += '★'; for (let i = t.importance; i < 5; i++) h += '☆'; h += '</span></div>'; }
  if (t.source) h += `<div class="info-row"><span class="info-label">来源</span><span>${esc(t.source)}</span></div>`;
  if (t.content) h += `<div style="margin-top:10px;font-size:13px;line-height:1.6;color:var(--text2)">${esc(t.content)}</div>`;
  if (t.detail_json) { try { const d = JSON.parse(t.detail_json); Object.keys(d).forEach(k => { h += `<div class="info-row"><span class="info-label">${esc(k)}</span><span>${esc(String(d[k]))}</span></div>`; }); } catch (_) {} }
  return h;
}

function formatArc(a, nodes) {
  let h = '';
  if (a.name) h += `<div style="font-size:17px;font-weight:700;margin-bottom:6px">${esc(a.name)}</div>`;
  if (a.arc_type) h += `<div class="info-row"><span class="info-label">类型</span><span>${esc(a.arc_type)}</span></div>`;
  if (a.status) h += `<div class="info-row"><span class="info-label">状态</span><span>${esc(a.status)}</span></div>`;
  if (a.description) h += `<div style="margin:8px 0;font-size:13px;line-height:1.5;color:var(--text2)">${esc(a.description)}</div>`;
  if (nodes && nodes.length) {
    h += '<div style="font-size:14px;font-weight:600;margin:10px 0 6px">节点 (' + nodes.length + ')</div>';
    nodes.sort((x, y) => (x.target_chapter||0) - (y.target_chapter||0)).forEach((n, i) => {
      const sc = n.status === 'completed' ? 'var(--success)' : n.status === 'pending' ? 'var(--accent)' : 'var(--text2)';
      h += `<div style="background:var(--surface2);border:1px solid var(--border);border-radius:8px;padding:10px;margin:6px 0"><div style="display:flex;align-items:center;gap:8px"><span style="width:22px;height:22px;border-radius:50%;background:${sc};color:#fff;display:inline-flex;align-items:center;justify-content:center;font-size:11px;font-weight:700;flex-shrink:0">${i+1}</span><strong style="font-size:13px;flex:1">${esc(n.title||'')}</strong></div>${n.target_chapter ? `<div style="font-size:11px;color:var(--text2);margin-top:4px">目标: 第${n.target_chapter}章${n.actual_chapter ? ` | 实际: 第${n.actual_chapter}章` : ''}</div>` : ''}${n.description ? `<div style="font-size:12px;color:var(--text2);margin-top:4px;line-height:1.4">${esc(n.description)}</div>` : ''}</div>`;
    });
  }
  return h;
}

function formatReader(i) {
  let h = '';
  const tl = { known: '已知信息', suspense: '悬念', misconception: '误解' }[i.type] || i.type || '';
  if (tl) h += `<div class="info-row"><span class="info-label">类型</span><span style="color:${i.type==='suspense'?'var(--warning)':i.type==='misconception'?'var(--error)':'var(--success)'};font-weight:600">${esc(tl)}</span></div>`;
  if (i.planted_chapter) h += `<div class="info-row"><span class="info-label">埋设章节</span><span>第${i.planted_chapter}章</span></div>`;
  if (i.revealed_chapter) h += `<div class="info-row"><span class="info-label">揭示章节</span><span>第${i.revealed_chapter}章</span></div>`;
  if (i.related_truth) h += `<div class="info-row"><span class="info-label">关联真相</span><span>${esc(i.related_truth)}</span></div>`;
  if (i.content) h += `<div style="margin-top:10px;font-size:13px;line-height:1.6;color:var(--text2)">${esc(i.content)}</div>`;
  return h;
}

function formatPreference(i) {
  let h = '';
  h += `<div class="info-row"><span class="info-label">分类</span><span style="font-weight:600">${esc(i.category||'未分类')}</span></div>`;
  h += `<div class="info-row"><span class="info-label">范围</span><span>${i.is_global ? '全局' : '小说专属'}</span></div>`;
  if (i.id) h += `<div class="info-row"><span class="info-label">ID</span><span>${i.id}</span></div>`;
  if (i.content) h += `<div style="margin-top:10px;font-size:13px;line-height:1.6;color:var(--text2);white-space:pre-wrap">${esc(i.content)}</div>`;
  return h;
}

function formatLocation(l) {
  let h = '';
  if (l.name) h += `<div style="font-size:17px;font-weight:700;margin-bottom:6px">${esc(l.name)}</div>`;
  if (l.location_type) h += `<div class="info-row"><span class="info-label">类型</span><span>${esc(l.location_type)}</span></div>`;
  if (l.description) h += `<div style="margin:8px 0;font-size:13px;line-height:1.6;color:var(--text2)">${esc(l.description)}</div>`;
  if (l.detail_json) { try { const d = JSON.parse(l.detail_json); Object.keys(d).forEach(k => { h += `<div class="info-row"><span class="info-label">${esc(k)}</span><span>${esc(String(d[k]))}</span></div>`; }); } catch (_) {} }
  if (l.tags) h += `<div style="margin-top:8px"><span class="info-label">标签</span><div class="data-card-tags" style="margin-top:4px">${l.tags.split(',').map(t => `<span class="tag" style="background:rgba(16,185,129,.1);color:var(--success)">${esc(t.trim())}</span>`).join('')}</div></div>`;
  return h;
}

// ═══════════ 对话 ═══════════
function addMessage(role, content, thinking, isStreaming) {
  const container = document.getElementById('chatMessages');
  const div = document.createElement('div');
  div.className = 'msg ' + role; div.dataset.streaming = isStreaming || '';
  const av = role === 'user' ? '我' : 'AI';
  div.innerHTML = `<div class="msg-avatar">${av}</div><div class="msg-body"><div class="msg-bubble">${marked.parse(content || '')}</div>${role === 'assistant' ? `<div class="msg-actions"><button onclick="copyText(this.closest('.msg').querySelector('.msg-bubble').textContent)">复制</button></div>${thinking ? `<div class="thinking-toggle" onclick="toggleThinking(this)">💭 思考 (${thinking.length}字) ▼</div><div class="thinking-content hidden">${esc(thinking)}</div>` : ''}` : ''}</div>`;
  container.appendChild(div);
  container.scrollTop = container.scrollHeight;
  return div;
}

function updateStreaming(el, content, thinking) {
  const b = el.querySelector('.msg-bubble'); if (b) b.innerHTML = marked.parse(content || '');
  if (thinking) { let t = el.querySelector('.thinking-toggle'), c = el.querySelector('.thinking-content'); if (!t) { t = document.createElement('div'); t.className = 'thinking-toggle'; t.onclick = function(){ toggleThinking(this); }; c = document.createElement('div'); c.className = 'thinking-content hidden'; const body = el.querySelector('.msg-body'); body.insertBefore(t, body.children[1]); body.insertBefore(c, body.children[2]); } c.textContent = thinking; t.textContent = `💭 思考 (${thinking.length}字) ▼`; }
  el.closest('.chat-scroll').scrollTop = el.closest('.chat-scroll').scrollHeight;
}

function toggleThinking(el) { const c = el.nextElementSibling; if (c) { c.classList.toggle('hidden'); el.textContent = el.textContent.includes('▼') ? el.textContent.replace('▼', '▲') : el.textContent.replace('▲', '▼'); } }

let currentStreamEl = null, abortCtrl = null;
async function sendMessage(text) {
  if (!text.trim() || state.isLoading) return;
  const input = document.getElementById('msgInput'); input.value = ''; input.style.height = 'auto';
  state.isLoading = true; document.getElementById('sendBtn').classList.add('hidden'); document.getElementById('stopBtn').classList.remove('hidden');
  addMessage('user', text);
  currentStreamEl = addMessage('assistant', '思考中...', '', true);
  abortCtrl = new AbortController(); let thinking = '', content = '';
  try {
    const body = { message: text, novel_id: state.novelId };
    if (state.sessionId) body.session_id = state.sessionId;
    if (state.selectedModel) { const p = state.selectedModel.split('/', 2); if (p.length === 2) { body.provider_name = p[0]; body.model_id = p[1]; } }
    const headers = { 'Content-Type': 'application/json' };
    const token = getToken();
    if (token) headers['Authorization'] = 'Bearer ' + token;
    const res = await fetch(API.base + '/api/chat', { method: 'POST', headers, body: JSON.stringify(body), signal: abortCtrl.signal });
    const reader = res.body.getReader(), decoder = new TextDecoder(); let buf = '';
    while (true) { const { done, value } = await reader.read(); if (done) break; buf += decoder.decode(value, { stream: true }); const lines = buf.split('\n'); buf = lines.pop(); for (const line of lines) { if (!line.startsWith('data: ')) continue; const js = line.slice(6).trim(); if (!js) continue; try { const ev = JSON.parse(js); switch (ev.type) { case 'started': state.sessionId = ev.session_id; break; case 'thinking': thinking += ev.data||''; updateStreaming(currentStreamEl, content||'思考中...', thinking); break; case 'content': content += ev.data||''; updateStreaming(currentStreamEl, content, thinking); break; case 'done': if (ev.text) { content = ev.text; updateStreaming(currentStreamEl, content, thinking); } break; case 'error': addMessage('assistant', '❌ ' + (ev.error||'未知错误')); currentStreamEl = null; break; } } catch (_) {} } }
  } catch (e) { if (e.name !== 'AbortError') addMessage('assistant', '❌ 连接失败: ' + e.message); }
  if (currentStreamEl) { currentStreamEl.dataset.streaming = ''; currentStreamEl = null; }
  state.isLoading = false; document.getElementById('sendBtn').classList.remove('hidden'); document.getElementById('stopBtn').classList.add('hidden'); abortCtrl = null;
}

function stopChat() { if (abortCtrl) abortCtrl.abort(); if (state.isLoading) { state.isLoading = false; document.getElementById('sendBtn').classList.remove('hidden'); document.getElementById('stopBtn').classList.add('hidden'); if (state.sessionId) api('/api/chat/cancel', { method: 'POST', body: { session_id: state.sessionId } }).catch(()=>{}); } }

// ═══════════ 会话 ═══════════
async function loadSessions() { if (!state.novelId) return; try { const r = await api(`/api/sessions?novel_id=${state.novelId}&page=1&size=20`); state.sessions = r.items || []; } catch (_) {} }
function showSessions() { const list = document.getElementById('sessionList'); list.innerHTML = state.sessions.length ? state.sessions.map(s => `<div class="session-item" onclick="loadSession('${s.session_id}');closeSheet('sessionSheet')"><div class="session-title">${esc(s.title||s.session_id.slice(0,20))}</div>${s.current_phase ? `<div class="session-phase">阶段: ${s.current_phase}</div>` : ''}</div>`).join('') : '<div style="padding:16px;text-align:center;color:var(--text2)">暂无历史会话</div>'; openSheet('sessionSheet'); }
async function loadSession(sid) { state.sessionId = sid; document.getElementById('chatMessages').innerHTML = ''; try { const r = await api(`/api/sessions/${sid}/messages`); (r.messages||[]).forEach(m => { if ((m.role==='user'||m.role==='assistant') && (m.content||m.thinking_content)) addMessage(m.role, m.content||'', m.thinking_content||''); }); } catch (_) {} toast('已加载会话'); }
function newChat() { state.sessionId = null; document.getElementById('chatMessages').innerHTML = '<div class="empty-state"><p>开始新的对话</p><span class="hint">输入消息开始创作</span></div>'; }

// ═══════════ 模型 ═══════════
async function loadModels() { try { const r = await api('/api/settings/model'); state.models = r.models||[]; state.selectedModel = r.selected_model_key||''; } catch (_) {} }
function showModels() { document.getElementById('modelList').innerHTML = state.models.map(m => { const displayName = m.provider ? `${m.provider} / ${m.name}` : (m.name || m.key); return `<div class="model-item ${m.key===state.selectedModel?'selected':''}" onclick="selectModel('${esc(m.key)}')"><div class="model-name">${esc(displayName)}</div>${m.thinking ? '<span class="model-badge">思考</span>' : ''}</div>`; }).join(''); openSheet('modelSheet'); }
async function selectModel(key) { try { await api('/api/settings/model', { method: 'POST', body: { model_key: key } }); state.selectedModel = key; closeSheet('modelSheet'); toast('已切换'); showModels(); } catch (_) { toast('切换失败'); } }

// ═══════════ 设置 ═══════════
async function loadSettings() {
  const isDark = getTheme() === 'dark';
  const lang = getLang();
  const token = getToken();
  try {
    const r = await api('/api/settings/model');
    if (r.error === 'unauthorized') {
      // token 无效，显示设置页+token输入
      document.getElementById('settingsContent').innerHTML = `
        <div class="setting-group"><div class="setting-label">🔐 API 认证</div>
          <p style="font-size:12px;color:var(--text2);margin-bottom:10px">令牌无效或已过期，请在桌面端「设置」中查看令牌并重新输入。</p>
          <div style="display:flex;gap:8px;margin-top:8px">
            <input id="tokenField" type="text" placeholder="输入 32 位令牌" value="${esc(token)}" style="flex:1;padding:8px 12px;border:1px solid var(--border);border-radius:8px;font-size:13px;font-family:monospace;outline:none">
            <button onclick="saveTokenFromSettings()" style="padding:8px 16px;border:none;border-radius:8px;background:var(--accent);color:#fff;font-size:13px;font-weight:600;cursor:pointer">保存</button>
          </div>
          <button onclick="startQRScanFromSettings()" style="width:100%;margin-top:8px;padding:8px;border:1px solid var(--border);border-radius:8px;background:var(--surface);color:var(--text);font-size:12px;cursor:pointer">📷 扫描二维码</button>
        </div>
        <div class="setting-group"><div class="setting-label">🎨 ${t('appearance')}</div><div class="setting-value" onclick="toggleTheme()"><span>${t('dark_mode').replace('Mode','').replace('模式','')}</span><strong style="color:var(--accent)">${isDark?t('dark_mode'):t('light_mode')}</strong></div></div>
        <div class="setting-group"><div class="setting-label">🌐 Language</div><div class="setting-value" onclick="toggleLang()"><span>切换到</span><strong style="color:var(--accent)">${lang==='zh'?'English →':'中文 →'}</strong></div></div>`;
      return;
    }
    state.models = r.models||[]; state.selectedModel = r.selected_model_key||'';
    const found = state.models.find(m => m.key === state.selectedModel);
    const name = found ? (found.provider ? `${found.provider} / ${found.name}` : found.name) : state.selectedModel.split('/').pop() || '未选择';
    document.getElementById('settingsContent').innerHTML = `
      <div class="setting-group"><div class="setting-label">🤖 ${t('model')}</div><div class="setting-value" onclick="showModels()"><span>${t('current_model')}</span><strong style="color:var(--accent)">${esc(name)}</strong></div></div>
      <div class="setting-group"><div class="setting-label">🎨 ${t('appearance')}</div><div class="setting-value" onclick="toggleTheme()"><span>${t('dark_mode').replace('Mode','').replace('模式','')}</span><strong style="color:var(--accent)">${isDark?t('dark_mode'):t('light_mode')}</strong></div></div>
      <div class="setting-group"><div class="setting-label">🌐 Language</div><div class="setting-value" onclick="toggleLang()"><span>切换到</span><strong style="color:var(--accent)">${lang==='zh'?'English →':'中文 →'}</strong></div></div>
      <div class="setting-group"><div class="setting-label">🔐 API 认证</div>
        <div style="display:flex;gap:8px">
          <input id="tokenField" type="text" placeholder="输入 32 位令牌" value="${esc(token)}" style="flex:1;padding:8px 12px;border:1px solid var(--border);border-radius:8px;font-size:13px;font-family:monospace;outline:none">
          <button onclick="saveTokenFromSettings()" style="padding:8px 16px;border:none;border-radius:8px;background:var(--accent);color:#fff;font-size:13px;font-weight:600;cursor:pointer">保存</button>
        </div>
        <button onclick="startQRScanFromSettings()" style="width:100%;margin-top:8px;padding:8px;border:1px solid var(--border);border-radius:8px;background:var(--surface);color:var(--text);font-size:12px;cursor:pointer">📷 扫描二维码</button>
      </div>
      <div class="setting-group"><div class="setting-label">🔗 ${t('server')}</div><div class="setting-value"><span>${t('server')}</span><strong>${esc(API.base)}</strong></div><div class="setting-value"><span>${t('status')}</span><strong style="color:${API.connOk?'var(--success)':'var(--error)'}">${API.connOk?'🟢 已连接':'🔴 离线'}</strong></div><div class="setting-value"><span>缓存</span><strong style="color:var(--text2);font-size:12px">在线时自动同步到本地</strong></div></div>`;
  } catch (_) {
    // 网络错误也显示设置页
    document.getElementById('settingsContent').innerHTML = `
      <div class="setting-group"><div class="setting-label">🔐 API 认证</div>
        <p style="font-size:12px;color:var(--text2);margin-bottom:10px">连接失败，请输入令牌。</p>
        <div style="display:flex;gap:8px;margin-top:8px">
          <input id="tokenField" type="text" placeholder="输入 32 位令牌" value="${esc(token)}" style="flex:1;padding:8px 12px;border:1px solid var(--border);border-radius:8px;font-size:13px;font-family:monospace;outline:none">
          <button onclick="saveTokenFromSettings()" style="padding:8px 16px;border:none;border-radius:8px;background:var(--accent);color:#fff;font-size:13px;font-weight:600;cursor:pointer">保存</button>
        </div>
        <button onclick="startQRScanFromSettings()" style="width:100%;margin-top:8px;padding:8px;border:1px solid var(--border);border-radius:8px;background:var(--surface);color:var(--text);font-size:12px;cursor:pointer">📷 扫描二维码</button>
      </div>
      <div class="setting-group"><div class="setting-label">🎨 ${t('appearance')}</div><div class="setting-value" onclick="toggleTheme()"><span>${t('dark_mode').replace('Mode','').replace('模式','')}</span><strong style="color:var(--accent)">${isDark?t('dark_mode'):t('light_mode')}</strong></div></div>
      <div class="setting-group"><div class="setting-label">🌐 Language</div><div class="setting-value" onclick="toggleLang()"><span>切换到</span><strong style="color:var(--accent)">${lang==='zh'?'English →':'中文 →'}</strong></div></div>`;
  }
}

function saveTokenFromSettings() {
  const val = document.getElementById('tokenField').value.trim();
  if (val) {
    setToken(val);
    toast('令牌已保存');
    loadSettings();
  }
}

// 设置页 QR 码扫描
function startQRScanFromSettings() {
  let overlay = document.getElementById('qrScanOverlay');
  if (overlay) overlay.remove();
  overlay = document.createElement('div');
  overlay.id = 'qrScanOverlay';
  overlay.style.cssText = 'position:fixed;top:0;left:0;right:0;bottom:0;background:#000;z-index:1000;display:flex;flex-direction:column';
  overlay.innerHTML = `
    <div style="flex:1;position:relative;overflow:hidden">
      <video id="qrVideo" style="width:100%;height:100%;object-fit:cover" autoplay playsinline></video>
      <canvas id="qrCanvas" style="display:none"></canvas>
      <div style="position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);width:200px;height:200px;border:2px solid rgba(255,255,255,.7);border-radius:12px;pointer-events:none"></div>
    </div>
    <div style="padding:16px;text-align:center;background:var(--surface)">
      <p style="font-size:14px;color:var(--text);margin-bottom:12px">将二维码放入框内</p>
      <button id="qrCancel" style="padding:10px 32px;border:1px solid var(--border);border-radius:8px;background:var(--surface);color:var(--text);font-size:14px;cursor:pointer">取消</button>
    </div>`;
  document.body.appendChild(overlay);

  const video = document.getElementById('qrVideo');
  const canvas = document.getElementById('qrCanvas');
  const ctx = canvas.getContext('2d', { willReadFrequently: true });
  let scanning = true;

  document.getElementById('qrCancel').onclick = () => {
    scanning = false;
    if (video.srcObject) {
      video.srcObject.getTracks().forEach(t => t.stop());
    }
    overlay.remove();
  };

  navigator.mediaDevices.getUserMedia({ video: { facingMode: 'environment' } })
    .then(stream => {
      video.srcObject = stream;
      video.play();
      scanFrame();
    })
    .catch(() => {
      toast('无法访问摄像头');
      overlay.remove();
    });

  function scanFrame() {
    if (!scanning) return;
    if (video.readyState === video.HAVE_ENOUGH_DATA) {
      canvas.width = video.videoWidth;
      canvas.height = video.videoHeight;
      ctx.drawImage(video, 0, 0, canvas.width, canvas.height);
      const imageData = ctx.getImageData(0, 0, canvas.width, canvas.height);
      const code = jsQR(imageData.data, imageData.width, imageData.height, { inversionAttempts: 'dontInvert' });
      if (code && code.data) {
        scanning = false;
        video.srcObject.getTracks().forEach(t => t.stop());
        overlay.remove();
        // 填入 token 并保存
        const tokenField = document.getElementById('tokenField');
        if (tokenField) tokenField.value = code.data;
        setToken(code.data);
        toast('扫码成功，令牌已保存');
        loadSettings();
        return;
      }
    }
    requestAnimationFrame(scanFrame);
  }
}

// ═══════════ 输入 ═══════════
function autoResize(el) { el.style.height = 'auto'; el.style.height = Math.min(el.scrollHeight, 120) + 'px'; document.getElementById('sendBtn').disabled = !el.value.trim(); }

// 对话滚动到底部
function scrollToBottom() {
  const el = document.getElementById('chatMessages');
  el.scrollTo({ top: el.scrollHeight, behavior: 'smooth' });
}
// 监听滚动，显示/隐藏滚动到底部按钮
function setupChatScroll() {
  const el = document.getElementById('chatMessages');
  if (!el) return;
  el.addEventListener('scroll', () => {
    const btn = document.getElementById('scrollToBottom');
    const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 100;
    if (nearBottom) btn.classList.add('hidden');
    else btn.classList.remove('hidden');
  });
}

// ═══════════ 初始化 ═══════════
document.addEventListener('DOMContentLoaded', async () => {
  document.querySelectorAll('.nav-item').forEach(btn => btn.addEventListener('click', () => switchPage(btn.dataset.page)));
  const input = document.getElementById('msgInput');
  input.addEventListener('input', () => autoResize(input));
  input.addEventListener('keydown', (e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendMessage(input.value); } });
  document.getElementById('sendBtn').addEventListener('click', () => sendMessage(input.value));
  document.getElementById('stopBtn').addEventListener('click', stopChat);
  setupChatScroll();
  // 检查 token 是否有效
  const token = getToken();
  if (!token) {
    showTokenPrompt();
  } else {
    // 验证 token 是否有效
    try {
      const r = await api('/api/novels');
      if (r.error === 'unauthorized') {
        showTokenPrompt();
      }
    } catch (_) {
      // 网络错误不弹窗
    }
  }
  connectWS();
  switchPage('novels');
});
