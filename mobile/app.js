// ── Goink Mobile Web Client ──
const API = { base: location.origin, ws: null, connOk: false };
const state = {
  page: 'novels', novelId: 0, novelTitle: '', sessionId: null,
  models: [], selectedModel: '', isLoading: false, sessions: [],
  reader: { novelId: 0, chapterId: 0, idx: 0, chapters: [] },
  chaptersCache: {}, selfStreaming: false
};

// ── 离线缓存（idb-keyval + 内存 Map）──
// idb-keyval: 持久化层，基于 IndexedDB，大容量
// 内存缓存: 微秒级，存最近 200 条数据（LRU）
const memCache = new Map();
const MEM_MAX = 200;
function memSet(k, v) { memCache.set(k, v); if (memCache.size > MEM_MAX) { const first = memCache.keys().next().value; memCache.delete(first); } }
function memGet(k) { if (memCache.has(k)) { const v = memCache.get(k); memCache.delete(k); memCache.set(k, v); return v; } }

async function cacheResponse(path, data) {
  memSet(path, data);                          // 内存缓存（微秒级）
  await idbKeyval.set(path, data).catch(() => {}); // 持久化（IndexedDB）
}

async function offlineFallback(path) {
  // L1: 内存（微秒）
  const m = memGet(path);
  if (m !== undefined) return { ...m, _offline: true };
  // L2: idb-keyval（IndexedDB，首次读稍慢）
  const d = await idbKeyval.get(path).catch(() => null);
  if (d) { memSet(path, d); return { ...d, _offline: true }; }
  return { error: 'offline', _offline: true };
}

async function syncToOffline() {
  if (!API.connOk) return;
  try {
    await idbKeyval.clear().catch(() => {});
    memCache.clear();
    const novelsRes = await api('/api/novels');
    if (novelsRes.novels) {
      for (const novel of novelsRes.novels) {
        const chRes = await api(`/api/novels/${novel.id}/chapters`).catch(() => ({}));
        const chapters = chRes.chapters || [];
        // 预缓存每章正文内容
        for (const ch of chapters) {
          await api(`/api/chapters/${ch.id}`).catch(() => ({}));
        }
        // 缓存其他设定
        await Promise.all([
          api(`/api/characters?novel_id=${novel.id}`).catch(() => ({})),
          api(`/api/timeline?novel_id=${novel.id}`).catch(() => ({})),
          api(`/api/arcs?novel_id=${novel.id}`).catch(() => ({})),
          api(`/api/locations?novel_id=${novel.id}`).catch(() => ({})),
          api(`/api/preferences?novel_id=${novel.id}`).catch(() => ({})),
        ]);
      }
    }
  } catch (_) {}
}

// ── HTTP ──
let _useCache = false; // 连续失败后跳过网络直接读缓存

function getToken() { return localStorage.getItem('goink_api_token') || ''; }
function setToken(t) { localStorage.setItem('goink_api_token', t); }

async function api(path, opts = {}) {
  const headers = { 'Content-Type': 'application/json' };
  const token = getToken();
  if (token) headers['Authorization'] = 'Bearer ' + token;
  const isGet = !opts.method || opts.method === 'GET';
  // 离线或缓存模式：跳过网络直接读缓存
  if (!navigator.onLine || (_useCache && isGet)) {
    API.connOk = false;
    return offlineFallback(path);
  }
  // 网络请求带 1.5s 超时
  const ctrl = new AbortController();
  const timer = setTimeout(() => ctrl.abort(), 500);
  const signal = opts.signal || ctrl.signal;
  try {
    const res = await fetch(API.base + path, { method: opts.method || 'GET', headers, body: opts.body ? JSON.stringify(opts.body) : undefined, signal });
    clearTimeout(timer);
    if (res.status === 401) {
      if (!document.getElementById('tokenOverlay')) showTokenPrompt();
      return { error: 'unauthorized' };
    }
    API.connOk = true; _useCache = false; // 成功 → 取消缓存模式
    const data = await res.json();
    if (isGet) cacheResponse(path, data).catch(() => {});
    return data;
  } catch (_) {
    API.connOk = false; _useCache = true; // 失败 → 后续直接读缓存
    return offlineFallback(path);
  }
}

// Token 输入弹窗
function showTokenPrompt() {
  let overlay = document.getElementById('tokenOverlay');
  if (overlay) overlay.remove();
  overlay = tpl('tpl-token-prompt').firstElementChild;
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
  overlay = tpl('tpl-qr-scan').firstElementChild;
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
  if (mc) mc.content = theme === 'dark' ? '#0a0e17' : '#eef3f7';
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
    lore: '设定', items: '物品', scenes: '场景',
    no_chapters: '暂无章节', no_characters: '暂无角色', no_timeline: '暂无时间线',
    no_arcs: '暂无弧线', no_reader: '暂无读者认知', no_prefs: '暂无偏好', no_locations: '暂无地点',
    no_lore: '暂无设定', no_items: '暂无物品', no_scenes: '暂无场景',
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
    lore: 'Lore', items: 'Items', scenes: 'Scenes',
    no_chapters: 'No chapters yet', no_characters: 'No characters yet', no_timeline: 'No timeline yet',
    no_arcs: 'No arcs yet', no_reader: 'No reader perspectives yet', no_prefs: 'No preferences yet', no_locations: 'No locations yet',
    no_lore: 'No lore yet', no_items: 'No items yet', no_scenes: 'No scenes yet',
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

// ── Template Helpers ──
function tpl(id) { return document.getElementById(id).content.cloneNode(true); }
function qs(el, sel) { return el.querySelector(sel); }

// 空状态（template: tpl-empty）
function emptyState(text) {
  const el = tpl('tpl-empty'); const root = el.firstElementChild;
  qs(root, '.em-text').textContent = text;
  const hint = qs(root, '.em-hint');
  if (hint) hint.remove();
  return root;
}

// 设置值行（template: tpl-setting-row）
function settingRow(label, val, onclick, valStyle) {
  const el = tpl('tpl-setting-row'); const root = el.firstElementChild;
  qs(root, '.sv-label').textContent = label;
  const v = qs(root, '.sv-val');
  if (valStyle) v.style.cssText = valStyle;
  v.innerHTML = val;
  if (onclick) root.onclick = onclick;
  return root;
}

// 通用数据卡片构建
function dc({ badge, bg, color, title, sub, meta, onclick }) {
  const el = tpl('tpl-data-card'); const root = el.firstElementChild;
  const b = qs(root, '.dc-badge'); b.textContent = badge || '?';
  if (bg) b.style.background = bg; if (color) b.style.color = color;
  qs(root, '.dc-title').textContent = title || '';
  const subEl = qs(root, '.dc-sub');
  if (sub) subEl.textContent = sub; else subEl.remove();
  const metaEl = qs(root, '.dc-meta');
  if (meta) metaEl.innerHTML = meta; else metaEl.remove();
  if (onclick) root.onclick = onclick;
  return root;
}

// 小说卡片构建
function nvCard(n) {
  const el = tpl('tpl-novel-card'); const root = el.firstElementChild;
  if (n.id === state.novelId) root.classList.add('novel-active');
  root.onclick = () => openNovel(n.id, esc(n.title));
  const icon = qs(root, '.nv-icon'); icon.style.background = n.color || '#666'; icon.textContent = (n.title || '?')[0];
  qs(root, '.nv-title').textContent = n.title;
  let m = '';
  if (n.genre) m += `<span class="novel-tag novel-tag-ghost">${esc(n.genre)}</span>`;
  if (n.id === state.novelId) m += `<span class="novel-tag novel-tag-jade">◆ ${t('current')}</span>`;
  qs(root, '.nv-meta').innerHTML = m;
  const desc = qs(root, '.nv-desc');
  if (n.description) desc.textContent = n.description; else desc.remove();
  const wd = n.totalWords >= 10000 ? (n.totalWords / 10000).toFixed(1) + '万' : n.totalWords ? n.totalWords + '字' : '';
  qs(root, '.nv-stat-ch').innerHTML = `<strong>${n.chapterCount}</strong>${t('chapters')}`;
  qs(root, '.nv-stat-char').innerHTML = `<strong>${n.charCount}</strong>${t('characters')}`;
  const wdEl = qs(root, '.nv-stat-wd');
  if (wd) wdEl.innerHTML = `<strong>${wd}</strong>`; else wdEl.remove();
  const dtEl = qs(root, '.nv-stat-dt');
  if (n.lastUpdated) dtEl.textContent = '🕐' + n.lastUpdated; else dtEl.remove();
  return root;
}

// 分类折叠
function toggleGroup(el) {
  const group = el.closest('.collapse-group');
  if (!group) return;
  const body = group.querySelector('.collapse-body');
  const icon = group.querySelector('.collapse-icon');
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
  const titles = { novels: t('bookshelf'), chat: t('chat'), stats: t('stats_label', '统计'), settings: t('settings'), 'novel-detail': state.novelTitle || t('detail') };
  document.getElementById('pageTitle').textContent = titles[page] || 'Goink';
  const actions = document.getElementById('headerActions');
  if (page === 'chat') {
    actions.innerHTML = '<button onclick="newChat()" title="新对话"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg></button><button onclick="showSessions()" title="历史"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><polyline points="12,6 12,12 16,14"/></svg></button>';
    const banner = document.getElementById('chatBanner');
    if (state.novelTitle) { banner.innerHTML = '<span>📖</span> ' + esc(state.novelTitle); banner.style.display = 'flex'; } else { banner.style.display = 'none'; }
    loadModels(); loadSessions();
  } else { actions.innerHTML = ''; if (document.getElementById('chatBanner')) document.getElementById('chatBanner').style.display = 'none'; }
  if (page === 'novels') loadNovels();
  if (page === 'chat') { loadModels(); loadSessions(); syncWithDesktopState(); }
  if (page === 'settings') loadSettings();
  if (page === 'stats') loadStatsPage();
  if (page === 'novel-detail') loadNovelDetail();
}

// ── WebSocket (wspulse) ──
let wsStreamEl = null; // 桌面端对话流式消息的 DOM 元素
let wsThinking = '';   // 桌面端对话思考内容
let wsContent = '';    // 桌面端对话正文内容

async function connectWS() {
  try {
    const token = getToken();
    const room = state.sessionId || 'global';
    const params = new URLSearchParams();
    if (token) params.set('token', token);
    params.set('room', room);
    const wsUrl = API.base.replace('http', 'ws') + '/api/ws?' + params.toString();

    const client = await pulseConnect(wsUrl, {
      onMessage(msg) {
        console.log('[WS] 收到消息:', msg.event, msg.payload);
        try {
          const ev = typeof msg.payload === 'string' ? JSON.parse(msg.payload) : msg.payload;
          // 非 chat 频道的事件
          if (ev.type === 'model_changed') {
            state.selectedModel = ev.model_key || '';
            toast('模型已切换');
            if (state.page === 'settings') loadSettings();
            return;
          }
          // chat 频道事件
          if (ev.channel === 'chat') {
            // 同步状态事件：中途加入时桌面端推送当前流式状态
            if (ev.type === 'sync_state') {
              handleSyncState(ev);
              return;
            }
            // 如果桌面端在不同会话，只更新会话列表
            if (ev.session_id && ev.session_id !== state.sessionId) {
              if (ev.type === 'done' || ev.type === 'started') loadSessions();
              return;
            }
            handleChatEvent(ev);
          }
        } catch (_) {}
      },
      onDisconnect(err) {
        console.log('[WS] 断开连接:', err);
        API.connOk = false;
      },
      onTransportRestore() {
        console.log('[WS] 连接恢复');
        API.connOk = true;
      },
      autoReconnect: { maxRetries: 10, baseDelay: 1000, maxDelay: 30000 }
    });

    API.wsClient = client;
    API.connOk = true;

    // 连接成功后，查询桌面端当前流式状态
    syncWithDesktopState();
  } catch (_) {
    setTimeout(connectWS, 5000);
  }
}

// 处理同步状态：中途加入会话时，桌面端推送当前流式状态
function handleSyncState(ev) {
  if (state.page !== 'chat') return;

  const sessionId = ev.session_id;
  const thinking = ev.thinking || '';
  const content = ev.content || '';

  // 已在流式同步中（WS 事件流进行中），不重复创建气泡
  if (wsStreamEl) return;

  const createBubble = () => {
    wsThinking = thinking;
    wsContent = content;
    wsStreamEl = addMessage('assistant', content || '思考中...', '', true);
    if (thinking || content) {
      updateStreaming(wsStreamEl, content || '思考中...', thinking, true);
    }
    state.isLoading = true;
    setChatBusy(true);
  };

  // 切换到桌面端当前会话
  if (sessionId && sessionId !== state.sessionId) {
    state.sessionId = sessionId;
    // 加载该会话的历史消息
    const container = document.getElementById('chatMessages');
    container.innerHTML = '';
    api(`/api/sessions/${sessionId}/messages`).then(r => {
      (r.messages || []).forEach(m => {
        if ((m.role === 'user' || m.role === 'assistant') && (m.content || m.thinking_content)) {
          addMessage(m.role, m.content || '', m.thinking_content || '');
        }
      });
      createBubble();
    }).catch(() => {
      createBubble();
    });
    toast('已同步到当前会话');
  } else {
    // 同一会话：桌面端生成中途进入聊天页，直接补流式气泡
    createBubble();
  }
}

// 查询桌面端流式状态并同步按钮/气泡（WS 连接时与进入聊天页时调用）
function syncWithDesktopState() {
  api('/api/sync/state').then(r => {
    if (r.active && r.session_id) {
      if (wsStreamEl || state.selfStreaming) return;
      handleSyncState(r);
    } else if (!state.selfStreaming) {
      // 桌面端无活跃流：恢复发送按钮
      state.isLoading = false;
      setChatBusy(false);
    }
  }).catch(() => {});
}

// 处理桌面端对话事件，实时更新移动端 UI
function handleChatEvent(ev) {
  console.log('[Chat] handleChatEvent:', ev.type, 'page:', state.page, 'sessionId:', state.sessionId);
  // 自己发消息时走 SSE 通道，屏蔽 WS 全局广播，避免双气泡/清空自己的消息
  if (state.selfStreaming) {
    console.log('[Chat] 自己发消息中，忽略 WS 事件');
    return;
  }
  if (state.page !== 'chat') {
    console.log('[Chat] 非聊天页面，忽略');
    return;
  }

  switch (ev.type) {
    case 'started': {
      // 桌面端开始新对话，先加载历史消息
      const newSessionId = ev.session_id || state.sessionId;
      const prevSessionId = state.sessionId;
      console.log('[Chat] started:', { newSessionId, prevSessionId, currentPage: state.page });
      state.sessionId = newSessionId;
      wsThinking = '';
      wsContent = '';
      state.isLoading = true;
      setChatBusy(true);

      // 如果是同一个会话的新消息，先加载历史再追加流式内容
      if (newSessionId && newSessionId === prevSessionId) {
        console.log('[Chat] 同一会话，加载历史');
        const container = document.getElementById('chatMessages');
        container.innerHTML = '';
        // 异步加载历史消息，然后创建流式气泡
        api(`/api/sessions/${newSessionId}/messages`).then(r => {
          console.log('[Chat] 历史消息数量:', (r.messages || []).length);
          (r.messages || []).forEach(m => {
            if ((m.role === 'user' || m.role === 'assistant') && (m.content || m.thinking_content)) {
              addMessage(m.role, m.content || '', m.thinking_content || '');
            }
          });
          // 历史加载完后创建流式气泡
          wsStreamEl = addMessage('assistant', '思考中...', '', true);
          console.log('[Chat] 流式气泡已创建');
        }).catch((err) => {
          console.log('[Chat] 加载历史失败:', err);
          wsStreamEl = addMessage('assistant', '思考中...', '', true);
        });
      } else {
        // 新会话，清空并创建流式气泡
        console.log('[Chat] 新会话，清空并创建流式气泡');
        const container = document.getElementById('chatMessages');
        container.innerHTML = '';
        wsStreamEl = addMessage('assistant', '思考中...', '', true);
        console.log('[Chat] 流式气泡已创建:', wsStreamEl);
      }
      break;
    }

    case 'thinking':
      // 桌面端思考内容
      wsThinking += ev.data || '';
      // 如果流式气泡还未创建，立即创建
      if (!wsStreamEl) {
        wsStreamEl = addMessage('assistant', '思考中...', '', true);
      }
      scheduleStreamingRender(wsStreamEl, wsContent || '思考中...', wsThinking);
      break;

    case 'content':
      // 桌面端正文内容
      wsContent += ev.data || '';
      // 如果流式气泡还未创建，立即创建
      if (!wsStreamEl) {
        wsStreamEl = addMessage('assistant', '', '', true);
      }
      scheduleStreamingRender(wsStreamEl, wsContent, wsThinking);
      break;

    case 'done':
      // 桌面端对话完成
      cancelPendingStream();
      if (ev.text) wsContent = ev.text;
      if (wsStreamEl) {
        updateStreaming(wsStreamEl, wsContent, wsThinking, true);
        wsStreamEl.dataset.streaming = '';
        wsStreamEl = null;
      }
      wsThinking = '';
      wsContent = '';
      state.isLoading = false;
      setChatBusy(false);
      // 刷新会话列表
      loadSessions();
      break;

    case 'error':
      cancelPendingStream();
      if (wsStreamEl) {
        updateStreaming(wsStreamEl, '❌ ' + (ev.error || '未知错误'), '', true);
        wsStreamEl.dataset.streaming = '';
        wsStreamEl = null;
      }
      wsThinking = '';
      wsContent = '';
      state.isLoading = false;
      setChatBusy(false);
      break;

    case 'tool_call':
      // 工具调用提示
      if (wsStreamEl && ev.tool_name) {
        const toolHint = `\n\n🔧 ${ev.tool_name}...`;
        wsContent += toolHint;
        scheduleStreamingRender(wsStreamEl, wsContent, wsThinking);
      }
      break;

    case 'phase_gate':
      // 阶段门禁变化
      if (ev.phase_gate && ev.phase_gate.phase) {
        toast(`阶段: ${ev.phase_gate.phase}`);
      }
      break;
  }
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
    const colors = ['#7b4f9e','#c44a4a','#3d8b5e','#b8922e','#c44a4a','#7b4f9e','#3d8b5e','#d4a843'];
    const enriched = await Promise.all(novels.map(async (n, i) => {
      const [chRes, charRes] = await Promise.all([api(`/api/novels/${n.id}/chapters?page=1&size=9999`), api(`/api/characters?novel_id=${n.id}`)]);
      const chs = chRes.chapters || [];
      const chars = charRes.characters || [];
      return { ...n, color: colors[i % colors.length], chapterCount: chs.length, charCount: chars.length, totalWords: chs.reduce((s, c) => s + (c.word_count || 0), 0), lastUpdated: chs.length ? chs[0]?.updated_at?.slice(0, 10) : '' };
    }));
    el.innerHTML = '';
    enriched.forEach(n => el.appendChild(nvCard(n)));
  } catch (_) { document.getElementById('novelList').innerHTML = `<div class="empty-state"><p>${t('load_fail')}</p></div>`; }
}

function openNovel(id, title) { state.novelId = id; state.novelTitle = title; switchPage('novel-detail'); }

// ═══════════ 新建作品 ═══════════
function showCreateNovel() {
  document.getElementById('createNovelOverlay').classList.remove('hidden');
  document.getElementById('createNovelTitle').value = '';
  document.getElementById('createNovelGenre').value = '';
  document.getElementById('createNovelDesc').value = '';
  setTimeout(() => document.getElementById('createNovelTitle').focus(), 100);
}
function hideCreateNovel() { document.getElementById('createNovelOverlay').classList.add('hidden'); }
async function doCreateNovel() {
  const title = document.getElementById('createNovelTitle').value.trim();
  if (!title) { toast('请输入作品名称'); return; }
  hideCreateNovel();
  toast('创建中…');
  try {
    const r = await api('/api/novels', { method: 'POST', body: { title, genre: document.getElementById('createNovelGenre').value.trim(), description: document.getElementById('createNovelDesc').value.trim() } });
    if (r.novel) { toast('✅ 已创建'); loadNovels(); }
    else { toast('❌ ' + (r.error || '创建失败')); }
  } catch (_) { toast('❌ 创建失败'); }
}

// ═══════════ 小说详情 ═══════════
let novelTab = 'chapters';
const TABS = [{ id: 'chapters', label: () => '📖 ' + t('chapters') }, { id: 'characters', label: () => '👤 ' + t('characters') }, { id: 'timeline', label: () => '⏱ ' + t('timeline') }, { id: 'arcs', label: () => '🔮 ' + t('arcs') }, { id: 'reader', label: () => '👁 ' + t('reader') }, { id: 'preferences', label: () => '⚙ ' + t('preferences') }, { id: 'locations', label: () => '📍 ' + t('locations') }, { id: 'lore', label: () => '📜 ' + t('lore') }, { id: 'items', label: () => '⚔️ ' + t('items') }, { id: 'scenes', label: () => '🎬 ' + t('scenes') }];

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
        el.innerHTML = '';
        if (chs.length) {
          chs.forEach(c => el.appendChild(dc({
            badge: c.chapter_number, bg: 'var(--frost)', color: 'var(--ice)',
            title: c.title, sub: c.word_count + ' ' + t('words'),
            onclick: () => readChapter(nId, c.id)
          })));
        } else {
          el.innerHTML = `<div class="empty-state"><p>${t('no_chapters')}</p></div>`;
        }
        break;
      }
      case 'characters': {
        const r = await api(`/api/characters?novel_id=${nId}`);
        const chars = r.characters || [];
        el.innerHTML = '';
        if (chars.length) {
          chars.forEach(c => {
            let preview = c.role || '';
            if (!preview && c.personality) try { const p = JSON.parse(c.personality); const vals = Object.values(p).filter(v => typeof v === 'string' && v.length < 40); preview = vals.slice(0, 2).join(' · '); } catch (_) { preview = (c.personality||'').slice(0, 40) || ''; }
            el.appendChild(dc({
              badge: (c.name||'?')[0], bg: 'var(--frost)', color: 'var(--ice)',
              title: c.name, sub: preview || undefined,
              onclick: (ev) => cardClick(ev, () => showDetail(c.name, formatCharacter(c)))
            }));
          });
        } else {
          el.innerHTML = `<div class="empty-state"><p>${t('no_characters')}</p></div>`;
        }
        break;
      }
      case 'timeline': {
        const r = await api(`/api/timeline?novel_id=${nId}&page=1&size=500`);
        const items = extractItems(r);
        el.innerHTML = '';
        if (items.length) {
          const groups = [
            { key: t('resolved'), items: items.filter(i => i.status === 'resolved') },
            { key: t('pending'), items: items.filter(i => i.status === 'pending') },
            { key: t('other'), items: items.filter(i => i.status !== 'resolved' && i.status !== 'pending') }
          ];
          groups.forEach(g => {
            if (!g.items.length) return;
            const grp = tpl('tpl-collapse');
            const root = grp.firstElementChild;
            qs(root, '.c-title').textContent = g.key;
            qs(root, '.c-count').textContent = g.items.length;
            const body = qs(root, '.c-body');
            g.items.forEach(i => body.appendChild(timelineCard(i)));
            el.appendChild(root);
          });
        } else {
          el.innerHTML = `<div class="empty-state"><p>${t('no_timeline')}</p></div>`;
        }
        break;
      }
      case 'arcs': {
        const [arcRes, nodeRes] = await Promise.all([api(`/api/arcs?novel_id=${nId}&page=1&size=500`), api(`/api/arc-nodes?novel_id=${nId}`)]);
        const arcs = extractItems(arcRes);
        const allNodes = nodeRes.nodes || [];
        el.innerHTML = '';
        if (arcs.length) {
          arcs.forEach(a => {
            const nodes = allNodes.filter(n => n.story_arc_id === a.id);
            const completed = nodes.filter(n => n.status === 'completed').length;
            const sc = a.status === 'active' ? 'var(--ice)' : a.status === 'completed' ? 'var(--ice)' : a.status === 'paused' ? 'var(--text2)' : 'var(--text2)';
            let meta = '';
            if (nodes.length) meta += `<span class="tag tag-sm" style="background:var(--frost);color:var(--ice)">${completed}/${nodes.length}</span>`;
            if (a.arc_type) meta += `<span class="tag tag-sm" style="background:var(--surface2);color:var(--text2)">${esc(a.arc_type)}</span>`;
            if (a.status) meta += `<span class="tag tag-sm" style="background:${sc.replace(')','12)')};color:${sc}">${esc(a.status)}</span>`;
            el.appendChild(dc({
              badge: 'A', bg: 'var(--frost)', color: sc,
              title: a.name || '无名弧线', sub: (a.description||'').slice(0,50) || undefined,
              meta: meta || undefined,
              onclick: (ev) => cardClick(ev, () => showDetail(a.name||'', formatArc(a, nodes)))
            }));
          });
        } else {
          el.innerHTML = `<div class="empty-state"><p>${t('no_arcs')}</p></div>`;
        }
        break;
      }
      case 'reader': {
        const r = await api(`/api/reader?novel_id=${nId}&page=1&size=500`);
        const items = extractItems(r);
        el.innerHTML = '';
        if (items.length) {
          const groups = [
            { key: t('known'), tc: 'var(--ice)', tl: t('known'), items: items.filter(i => i.type === 'known') },
            { key: t('suspense'), tc: 'var(--ice)', tl: t('suspense'), items: items.filter(i => i.type === 'suspense') },
            { key: t('misconception'), tc: 'var(--ice)', tl: t('misconception'), items: items.filter(i => i.type === 'misconception') }
          ];
          groups.forEach(g => {
            if (!g.items.length) return;
            const grp = tpl('tpl-collapse');
            const root = grp.firstElementChild;
            qs(root, '.c-title').textContent = g.key;
            qs(root, '.c-count').textContent = g.items.length;
            const body = qs(root, '.c-body');
            g.items.forEach(i => {
              let meta = `<span class="tag tag-sm" style="background:${g.tc}15;color:${g.tc}">${g.tl}</span>`;
              if (i.planted_chapter) meta += `<span class="tag tag-sm" style="background:var(--surface2);color:var(--text2)">第${i.planted_chapter}章</span>`;
              if (i.revealed_chapter) meta += `<span class="tag tag-sm" style="background:var(--frost);color:var(--ice)">揭示${i.revealed_chapter}</span>`;
              body.appendChild(dc({
                badge: 'R', bg: g.tc + '15', color: g.tc,
                title: (i.content||'').slice(0,35), meta: meta,
                onclick: (ev) => cardClick(ev, () => showDetail(g.tl, formatReader(i)))
              }));
            });
            el.appendChild(root);
          });
        } else {
          el.innerHTML = `<div class="empty-state"><p>${t('no_reader')}</p></div>`;
        }
        break;
      }
      case 'preferences': {
        const r = await api(`/api/preferences?novel_id=${nId}&page=1&size=500`);
        const items = extractItems(r);
        el.innerHTML = '';
        if (items.length) {
          items.forEach(i => {
            const meta = `<span class="tag tag-sm" style="background:${i.is_global ? 'var(--frost)' : 'var(--surface2)'};color:${i.is_global ? 'var(--ice)' : 'var(--text2)'}">${i.is_global ? t('global') : t('novel_only')}</span>`;
            el.appendChild(dc({
              badge: 'P', bg: 'var(--frost)', color: 'var(--ice)',
              title: i.category || t('uncategorized'), sub: (i.content||'').slice(0,60) || undefined,
              meta: meta,
              onclick: (ev) => cardClick(ev, () => showDetail(i.category||t('uncategorized'), formatPreference(i)))
            }));
          });
        } else {
          el.innerHTML = `<div class="empty-state"><p>${t('no_prefs')}</p></div>`;
        }
        break;
      }
      case 'locations': {
        const r = await api(`/api/locations?novel_id=${nId}&page=1&size=500`);
        const items = extractItems(r);
        el.innerHTML = '';
        if (items.length) {
          items.forEach(i => {
            let preview = i.description ? i.description.slice(0,40) : (i.location_type || '');
            let meta = '';
            if (i.location_type) meta = `<span class="tag tag-sm" style="background:var(--frost);color:var(--ice)">${esc(i.location_type)}</span>`;
            el.appendChild(dc({
              badge: 'L', bg: 'var(--frost)', color: 'var(--ice)',
              title: i.name || '无名', sub: preview || undefined,
              meta: meta || undefined,
              onclick: (ev) => cardClick(ev, () => showDetail(i.name||'', formatLocation(i)))
            }));
          });
        } else {
          el.innerHTML = `<div class="empty-state"><p>${t('no_locations')}</p></div>`;
        }
        break;
      }
      case 'lore': {
        const r = await api(`/api/lore?novel_id=${nId}&page=1&size=9999`);
        const items = r.lore || [];
        el.innerHTML = '';
        if (items.length) {
          const groups = {};
          items.forEach(i => { const c = i.category || '未分类'; if (!groups[c]) groups[c] = []; groups[c].push(i); });
          Object.keys(groups).forEach(cat => {
            const grp = tpl('tpl-collapse');
            const root = grp.firstElementChild;
            qs(root, '.c-title').textContent = cat;
            qs(root, '.c-count').textContent = groups[cat].length;
            const body = qs(root, '.c-body');
            groups[cat].forEach(i => {
              body.appendChild(dc({
                badge: (i.title||'?')[0], bg: 'var(--frost)', color: 'var(--ice)',
                title: i.title, sub: i.summary || (i.content||'').slice(0,50) || undefined,
                meta: i.is_public === false ? '<span class="tag tag-sm" style="background:#c44a4a15;color:#c44a4a">隐藏</span>' : undefined,
                onclick: (ev) => cardClick(ev, () => showDetail(i.title, formatLore(i)))
              }));
            });
            el.appendChild(root);
          });
        } else {
          el.innerHTML = `<div class="empty-state"><p>${t('no_lore')}</p></div>`;
        }
        break;
      }
      case 'items': {
        const r = await api(`/api/items?novel_id=${nId}&page=1&size=9999`);
        const items = r.items || [];
        el.innerHTML = '';
        if (items.length) {
          items.forEach(i => {
            let meta = '';
            if (i.item_type) meta += `<span class="tag tag-sm" style="background:var(--frost);color:var(--ice)">${esc(i.item_type)}</span>`;
            if (i.grade) meta += `<span class="tag tag-sm" style="background:var(--surface2);color:var(--text2)">${esc(i.grade)}</span>`;
            if (i.status) meta += `<span class="tag tag-sm" style="${i.status === 'active' ? 'background:#3d8b5e15;color:#3d8b5e' : 'background:var(--surface2);color:var(--text2)'}">${esc(i.status)}</span>`;
            el.appendChild(dc({
              badge: (i.name||'?')[0], bg: 'var(--frost)', color: 'var(--ice)',
              title: i.name, sub: (i.description||'').slice(0,50) || undefined,
              meta: meta || undefined,
              onclick: (ev) => cardClick(ev, () => showDetail(i.name, formatItem(i)))
            }));
          });
        } else {
          el.innerHTML = `<div class="empty-state"><p>${t('no_items')}</p></div>`;
        }
        break;
      }
      case 'scenes': {
        const r = await api(`/api/scenes?novel_id=${nId}`);
        const scenes = r.scenes || [];
        el.innerHTML = '';
        if (scenes.length) {
          scenes.forEach(s => {
            let meta = '';
            if (s.arc_node_id) meta += `<span class="tag tag-sm" style="background:var(--frost);color:var(--ice)">节点${s.arc_node_id}</span>`;
            el.appendChild(dc({
              badge: (s.title||'?')[0], bg: 'var(--frost)', color: 'var(--ice)',
              title: s.title || '场景', sub: (s.content||'').slice(0,50) || undefined,
              meta: meta || undefined,
              onclick: (ev) => cardClick(ev, () => showDetail(s.title||'场景', formatScene(s)))
            }));
          });
        } else {
          el.innerHTML = `<div class="empty-state"><p>${t('no_scenes')}</p></div>`;
        }
        break;
      }
    }
  } catch (_) { el.innerHTML = '<div class="empty-state"><p>加载失败</p></div>'; }
}

// ═══════════ 统计页面 ═══════════
async function loadStatsPage() {
  const el = document.getElementById('statsContent');
  el.innerHTML = `<div class="empty-state"><div class="loading-dots">${t('loading')}</div></div>`;
  const nId = state.novelId;
  if (!nId) {
    el.innerHTML = '<div class="empty-state"><p>请先打开一本小说</p></div>';
    return;
  }
  try {
    const r = await api(`/api/stats?novel_id=${nId}`);
    const s = r.stats;
    if (!s) { el.innerHTML = '<div class="empty-state"><p>暂无数据</p></div>'; return; }
    el.innerHTML = '';
    const cards = [
      { label: '总章节', value: String(s.total_chapters || 0), badge: '📖' },
      { label: '总字数', value: fmt(s.total_words || 0), badge: '📝' },
      { label: '均章字数', value: fmt(s.avg_chapter_words || 0), badge: '📐' },
      { label: '弧线进度', value: `${s.arc_completed||0}/${s.arc_count||0}`, badge: '🔮' },
      { label: '伏笔回收', value: `${s.foreshadowing_resolved||0}/${s.foreshadowing_total||0}`, badge: '👁' },
      { label: '角色数', value: String(s.character_count || 0), badge: '👤' },
      { label: '地点数', value: String(s.location_count || 0), badge: '📍' },
    ];
    const grid = document.createElement('div'); grid.style.cssText = 'display:grid;grid-template-columns:1fr 1fr;gap:10px';
    cards.forEach(c => {
      const card = document.createElement('div');
      card.style.cssText = 'background:var(--surface-raised);border:1px solid var(--border);border-radius:var(--radius-lg);padding:14px;text-align:center;box-shadow:var(--shadow-xs)';
      card.innerHTML = `<div style="font-size:20px;margin-bottom:4px">${c.badge}</div><div style="font-size:20px;font-weight:700;color:var(--ice)">${esc(c.value)}</div><div style="font-size:11px;color:var(--text2);margin-top:2px">${esc(c.label)}</div>`;
      grid.appendChild(card);
    });
    el.appendChild(grid);
    if (s.latest_chapter_num > 0) {
      const footer = document.createElement('div');
      footer.style.cssText = 'margin-top:14px;font-size:12px;color:var(--text2);text-align:center';
      footer.textContent = `最新章节：第 ${s.latest_chapter_num} 章 ${s.latest_chapter_title || ''}`;
      el.appendChild(footer);
    }
  } catch (_) { el.innerHTML = '<div class="empty-state"><p>加载失败</p></div>'; }
}

function fmt(n) {
  if (n >= 10000) return (n / 10000).toFixed(1) + '万';
  if (n >= 1000) return (n / 1000).toFixed(1) + 'k';
  return String(n);
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
  // 背景文字颜色完全跟随全局主题，不设内联样式
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
  el.innerHTML = '';
  chapters.forEach((c, i) => {
    const item = tpl('tpl-ch-list'); const root = item.firstElementChild;
    if (i === idx) root.classList.add('ch-list-active');
    root.onclick = () => jumpToChapter(i);
    qs(root, '.ch-num').textContent = c.chapter_number;
    qs(root, '.ch-title').textContent = c.title || '';
    el.appendChild(root);
  });
  panel.classList.toggle('hidden');
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
  try { return JSON.parse(localStorage.getItem('reader_settings')) || { fontSize: 17, lineHeight: 1.8 }; } catch (_) { return { fontSize: 17, lineHeight: 1.8 }; }
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
}

// ═══════════ 时间线卡片 ═══════════
function timelineCard(t) {
  const sc = t.status === 'resolved' ? 'var(--ice)' : t.status === 'pending' ? 'var(--ice)' : 'var(--text2)';
  const sl = t.status === 'resolved' ? '已解决' : t.status === 'pending' ? '待处理' : t.status || '';
  let tags = `<span class="tag tag-sm" style="background:${sc.replace(')','15)')};color:${sc}">${sl}</span>`;
  if (t.target_chapter) tags += `<span class="tag tag-sm" style="background:var(--frost);color:var(--ice)">目标:第${t.target_chapter}</span>`;
  if (t.source_chapter_id) tags += `<span class="tag tag-sm" style="background:var(--surface2);color:var(--text2)">来源:第${t.source_chapter_id}</span>`;
  if (t.resolved_chapter_id) tags += `<span class="tag tag-sm" style="background:var(--frost);color:var(--ice)">解决:第${t.resolved_chapter_id}</span>`;
  if (t.importance) { let stars = ''; for (let i = 0; i < t.importance; i++) stars += '★'; for (let i = t.importance; i < 5; i++) stars += '☆'; tags += `<span class="tag tag-sm" style="background:var(--frost);color:var(--ice)">${stars}</span>`; }
  return dc({
    badge: 'T', bg: 'var(--frost)', color: sc,
    title: t.title || '', sub: (t.content||'').slice(0,50) || undefined,
    meta: tags,
    onclick: (ev) => cardClick(ev, () => showDetail(t.title||'', formatTimeline(t)))
  });
}

// ═══════════ 详情弹窗 ═══════════
function showDetail(title, body) {
  document.getElementById('detailTitle').textContent = title;
  document.getElementById('detailBody').innerHTML = body;
  openSheet('detailSheet');
}

// ═══════════ 详情格式化 ═══════════
// ir: 构造 info-row HTML（结构定义见 template#tpl-info-row）
function ir(l, v, s) { return `<div class="info-row"><span class="info-label">${esc(l)}</span><span${s?` style="${s}"`:''}>${v}</span></div>`; }
function escH(s) { return s || ''; }

function formatCharacter(c) {
  let h = '';
  if (c.name) h += `<div style="font-size:17px;font-weight:700;margin-bottom:8px;color:var(--ice)">${esc(c.name)}</div>`;
  if (c.role) h += ir('定位', esc(c.role));
  if (c.personality) {
    try { const p = JSON.parse(c.personality); Object.keys(p).forEach(k => { const v = String(p[k]); h += ir(k, esc(v)); }); } catch (_) { h += ir('性格', esc(c.personality)); }
  }
  if (c.background) h += `<div style="margin-top:8px;font-size:13px;line-height:1.6;color:var(--text2);padding:0 4px">${esc(c.background)}</div>`;
  return h;
}

function formatTimeline(t) {
  let h = '';
  if (t.title) h += `<div style="font-size:17px;font-weight:700;margin-bottom:6px;color:var(--ice)">${esc(t.title)}</div>`;
  if (t.category) h += ir('分类', esc(t.category));
  if (t.status) { const sc = t.status === 'resolved' ? 'var(--ice)' : t.status === 'pending' ? 'var(--ice)' : 'var(--text2)'; h += ir('状态', esc(t.status), `color:${sc};font-weight:600`); }
  if (t.target_chapter) h += ir('目标章节', '第' + t.target_chapter + '章');
  if (t.source_chapter_id) h += ir('来源章节', '第' + t.source_chapter_id + '章');
  if (t.resolved_chapter_id) h += ir('解决章节', '第' + t.resolved_chapter_id + '章');
  if (t.importance) { let s = ''; for (let i = 0; i < t.importance; i++) s += '★'; for (let i = t.importance; i < 5; i++) s += '☆'; h += ir('重要度', s); }
  if (t.source) h += ir('来源', esc(t.source));
  if (t.content) h += `<div style="margin-top:10px;font-size:13px;line-height:1.7;color:var(--text2)">${esc(t.content)}</div>`;
  if (t.detail_json) { try { const d = JSON.parse(t.detail_json); Object.keys(d).forEach(k => { h += ir(esc(k), esc(String(d[k]))); }); } catch (_) {} }
  return h;
}

function formatArc(a, nodes) {
  let h = '';
  if (a.name) h += `<div style="font-size:17px;font-weight:700;margin-bottom:6px;color:var(--ice)">${esc(a.name)}</div>`;
  if (a.arc_type) h += ir('类型', esc(a.arc_type));
  if (a.status) h += ir('状态', esc(a.status));
  if (a.description) h += `<div style="margin:8px 4px;font-size:13px;line-height:1.6;color:var(--text2)">${esc(a.description)}</div>`;
  if (nodes && nodes.length) {
    h += `<div style="font-size:14px;font-weight:600;margin:12px 0 6px;color:var(--ice)">节点 (${nodes.length})</div>`;
    nodes.sort((x, y) => (x.target_chapter||0) - (y.target_chapter||0)).forEach((n, i) => {
      const sc = n.status === 'completed' ? 'var(--ice)' : n.status === 'pending' ? 'var(--ice)' : 'var(--text2)';
      h += `<div style="background:var(--surface2);border:1px solid var(--border);border-radius:var(--radius-sm);padding:10px;margin:6px 0"><div style="display:flex;align-items:center;gap:8px"><span style="width:22px;height:22px;border-radius:50%;background:${sc};color:#fff;display:inline-flex;align-items:center;justify-content:center;font-size:11px;font-weight:700;flex-shrink:0">${i+1}</span><strong style="font-size:13px;flex:1;color:var(--text)">${esc(n.title||'')}</strong></div>${n.target_chapter ? `<div style="font-size:11px;color:var(--text2);margin-top:4px">目标: 第${n.target_chapter}章${n.actual_chapter ? ` | 实际: 第${n.actual_chapter}章` : ''}</div>` : ''}${n.description ? `<div style="font-size:12px;color:var(--text2);margin-top:4px;line-height:1.4">${esc(n.description)}</div>` : ''}</div>`;
    });
  }
  return h;
}

function formatReader(i) {
  let h = '';
  const tl = { known: '已知信息', suspense: '悬念', misconception: '误解' }[i.type] || i.type || '';
  if (tl) { const tc = i.type==='suspense'?'var(--ice)':i.type==='misconception'?'var(--ice)':'var(--ice)'; h += ir('类型', esc(tl), `color:${tc};font-weight:600`); }
  if (i.planted_chapter) h += ir('埋设章节', '第' + i.planted_chapter + '章');
  if (i.revealed_chapter) h += ir('揭示章节', '第' + i.revealed_chapter + '章');
  if (i.related_truth) h += ir('关联真相', esc(i.related_truth));
  if (i.content) h += `<div style="margin-top:10px;font-size:13px;line-height:1.7;color:var(--text2)">${esc(i.content)}</div>`;
  return h;
}

function formatPreference(i) {
  let h = '';
  h += ir('分类', esc(i.category || '未分类'), 'font-weight:600');
  h += ir('范围', i.is_global ? '全局' : '小说专属');
  if (i.content) h += `<div style="margin-top:10px;font-size:13px;line-height:1.7;color:var(--text2);white-space:pre-wrap">${esc(i.content)}</div>`;
  return h;
}

function formatLocation(l) {
  let h = '';
  if (l.name) h += `<div style="font-size:17px;font-weight:700;margin-bottom:6px;color:var(--ice)">${esc(l.name)}</div>`;
  if (l.location_type) h += ir('类型', esc(l.location_type));
  if (l.description) h += `<div style="margin:8px 4px;font-size:13px;line-height:1.6;color:var(--text2)">${esc(l.description)}</div>`;
  if (l.detail_json) { try { const d = JSON.parse(l.detail_json); Object.keys(d).forEach(k => { h += ir(esc(k), esc(String(d[k]))); }); } catch (_) {} }
  if (l.tags) h += `<div style="margin-top:8px"><span class="info-label" style="display:inline-block;min-width:60px;font-size:11px;color:var(--ice)">标签</span><div class="data-card-meta" style="display:inline-flex;gap:4px;margin-left:4px">${l.tags.split(',').map(t => `<span class="tag" style="background:var(--frost);color:var(--ice)">${esc(t.trim())}</span>`).join('')}</div></div>`;
  return h;
}

function formatLore(l) {
  let h = '';
  if (l.title) h += `<div style="font-size:17px;font-weight:700;margin-bottom:6px;color:var(--ice)">${esc(l.title)}</div>`;
  if (l.category) h += ir('分类', esc(l.category));
  if (l.is_public === false) h += ir('公开', '否（隐藏设定）', 'color:#c44a4a');
  if (l.reveal_chapter_id) h += ir('揭示章节', '第' + l.reveal_chapter_id + '章');
  if (l.arc_id) h += ir('关联弧线', 'ID: ' + l.arc_id);
  if (l.summary) h += `<div style="margin:8px 4px;font-size:13px;line-height:1.6;color:var(--text2)">${esc(l.summary)}</div>`;
  if (l.content) h += `<div style="margin:8px 4px;font-size:13px;line-height:1.7;color:var(--text2);white-space:pre-wrap">${esc(l.content)}</div>`;
  return h;
}

function formatItem(i) {
  let h = '';
  if (i.name) h += `<div style="font-size:17px;font-weight:700;margin-bottom:6px;color:var(--ice)">${esc(i.name)}</div>`;
  if (i.item_type) h += ir('类型', esc(i.item_type));
  if (i.grade) h += ir('品级', esc(i.grade));
  if (i.status) h += ir('状态', esc(i.status));
  if (i.narrative_role) h += ir('叙事角色', {key_prop:'关键道具',supporting:'重要',normal:'普通'}[i.narrative_role] || i.narrative_role);
  if (i.owner_id) h += ir('持有者', 'ID: ' + i.owner_id);
  if (i.description) h += `<div style="margin:8px 4px;font-size:13px;line-height:1.6;color:var(--text2)">${esc(i.description)}</div>`;
  if (i.ability) h += `<div style="margin:8px 4px"><span class="info-label" style="color:var(--ice)">能力</span><div style="font-size:13px;line-height:1.6;color:var(--text2);margin-top:4px">${esc(i.ability)}</div></div>`;
  if (i.lore) h += `<div style="margin:8px 4px"><span class="info-label" style="color:var(--ice)">来历</span><div style="font-size:13px;line-height:1.6;color:var(--text2);margin-top:4px">${esc(i.lore)}</div></div>`;
  return h;
}

function formatScene(s) {
  let h = '';
  if (s.title) h += `<div style="font-size:17px;font-weight:700;margin-bottom:6px;color:var(--ice)">${esc(s.title)}</div>`;
  if (s.chapter_id) h += ir('章节', 'ID: ' + s.chapter_id);
  if (s.location_id) h += ir('地点', 'ID: ' + s.location_id);
  if (s.arc_id) h += ir('关联弧线', 'ID: ' + s.arc_id);
  if (s.arc_node_id) h += ir('弧线节点', 'ID: ' + s.arc_node_id);
  if (s.content) h += `<div style="margin:8px 4px;font-size:13px;line-height:1.7;color:var(--text2);white-space:pre-wrap">${esc(s.content)}</div>`;
  return h;
}

// ═══════════ 对话 ═══════════
function addMessage(role, content, thinking, isStreaming) {
  const container = document.getElementById('chatMessages');
  const el = tpl('tpl-chat-msg'); const div = el.firstElementChild;
  div.className = 'msg ' + role; div.dataset.streaming = isStreaming || '';
  qs(div, '.cm-av').textContent = role === 'user' ? '我' : 'AI';
  qs(div, '.cm-bubble').innerHTML = marked.parse(content || '');
  if (thinking) {
    const body = qs(div, '.msg-body');
    const tog = document.createElement('div'); tog.className = 'thinking-toggle'; tog.onclick = function(){ toggleThinking(this); }; tog.textContent = `💭 思考 (${thinking.length}字) ▲`;
    const tc = document.createElement('div'); tc.className = 'thinking-content'; tc.textContent = thinking;
    body.appendChild(tog); body.appendChild(tc);
  }
  container.appendChild(div);
  container.scrollTop = container.scrollHeight;
  return div;
}

// 流式渲染节流：60ms 合并多次增量事件，避免每 token 全量 marked 重渲染卡顿
let pendingStream = null, streamTimer = null;
function cancelPendingStream() {
  if (streamTimer) { clearTimeout(streamTimer); streamTimer = null; }
  pendingStream = null;
}
function scheduleStreamingRender(el, content, thinking) {
  pendingStream = { el, content, thinking };
  if (streamTimer) return;
  streamTimer = setTimeout(() => {
    streamTimer = null;
    if (pendingStream) { const p = pendingStream; pendingStream = null; updateStreaming(p.el, p.content, p.thinking, false); }
  }, 60);
}

// final=true 渲染完整 markdown；final=false 流式期间用纯文本（未闭合 markdown 结构不会显示异常）
function updateStreaming(el, content, thinking, final) {
  const b = el.querySelector('.msg-bubble');
  if (b) {
    if (final) { b.classList.remove('streaming-plain'); b.innerHTML = marked.parse(content || ''); }
    else { b.classList.add('streaming-plain'); b.textContent = content || ''; }
  }
  if (thinking) {
    let t = el.querySelector('.thinking-toggle'), c = el.querySelector('.thinking-content');
    if (!t) {
      t = document.createElement('div'); t.className = 'thinking-toggle'; t.onclick = function(){ toggleThinking(this); };
      c = document.createElement('div'); c.className = 'thinking-content';
      const body = el.querySelector('.msg-body');
      body.insertBefore(t, body.children[1]); body.insertBefore(c, body.children[2]);
    }
    c.textContent = thinking; t.textContent = `💭 思考 (${thinking.length}字) ▲`;
  }
  el.closest('.chat-scroll').scrollTop = el.closest('.chat-scroll').scrollHeight;
}

function toggleThinking(el) { const c = el.nextElementSibling; if (c) { c.classList.toggle('hidden'); el.textContent = el.textContent.includes('▼') ? el.textContent.replace('▼', '▲') : el.textContent.replace('▲', '▼'); } }

// 发送/停止按钮状态：busy=true 显示停止按钮，false 恢复发送按钮
function setChatBusy(busy) {
  const send = document.getElementById('sendBtn');
  const stop = document.getElementById('stopBtn');
  if (send) send.classList.toggle('hidden', busy);
  if (stop) stop.classList.toggle('hidden', !busy);
}

let currentStreamEl = null, abortCtrl = null;
async function sendMessage(text) {
  if (!text.trim() || state.isLoading) return;
  const input = document.getElementById('msgInput'); input.value = ''; input.style.height = 'auto';
  state.isLoading = true; setChatBusy(true);
  state.selfStreaming = true; // 自己发消息期间屏蔽 WS 通道（同一事件流会经 WS 全局广播重复推送）
  addMessage('user', text);
  currentStreamEl = addMessage('assistant', '思考中...', '', true);
  abortCtrl = new AbortController(); let thinking = '', content = '', finished = false;
  try {
    const body = { message: text, novel_id: state.novelId };
    if (state.sessionId) body.session_id = state.sessionId;
    if (state.selectedModel) { const p = state.selectedModel.split('/', 2); if (p.length === 2) { body.provider_name = p[0]; body.model_id = p[1]; } }
    const headers = { 'Content-Type': 'application/json' };
    const token = getToken();
    if (token) headers['Authorization'] = 'Bearer ' + token;
    const res = await fetch(API.base + '/api/chat', { method: 'POST', headers, body: JSON.stringify(body), signal: abortCtrl.signal });
    const reader = res.body.getReader(), decoder = new TextDecoder(); let buf = '';
    while (true) {
      const { done, value } = await reader.read(); if (done) break;
      buf += decoder.decode(value, { stream: true });
      const lines = buf.split('\n'); buf = lines.pop();
      for (const line of lines) {
        if (!line.startsWith('data: ')) continue;
        const js = line.slice(6).trim(); if (!js) continue;
        try {
          const ev = JSON.parse(js);
          switch (ev.type) {
            case 'started': state.sessionId = ev.session_id; break;
            case 'thinking': thinking += ev.data||''; scheduleStreamingRender(currentStreamEl, content||'思考中...', thinking); break;
            case 'content': content += ev.data||''; scheduleStreamingRender(currentStreamEl, content, thinking); break;
            case 'tool_call':
              if (currentStreamEl && ev.tool_name) { content += `\n\n🔧 ${ev.tool_name}...`; scheduleStreamingRender(currentStreamEl, content, thinking); }
              break;
            case 'phase_gate':
              if (ev.phase_gate && ev.phase_gate.phase) toast(`阶段: ${ev.phase_gate.phase}`);
              break;
            case 'done':
              cancelPendingStream();
              if (ev.text) { content = ev.text; updateStreaming(currentStreamEl, content, thinking, true); }
              finished = true;
              break;
            case 'error':
              cancelPendingStream();
              if (currentStreamEl) { updateStreaming(currentStreamEl, '❌ ' + (ev.error||'未知错误'), '', true); currentStreamEl.dataset.streaming = ''; }
              currentStreamEl = null;
              finished = true;
              break;
          }
        } catch (_) {}
      }
      // done/error 是终止事件：服务端不关闭 SSE 连接（events channel 不 close），
      // 不跳出会永久挂在 reader.read() 上，恢复按钮的代码永不执行
      if (finished) break;
    }
    if (finished) abortCtrl.abort(); // 主动断开服务端残留连接，触发其 ctx.Done 退出
  } catch (e) {
    if (e.name !== 'AbortError') {
      cancelPendingStream();
      if (currentStreamEl) { updateStreaming(currentStreamEl, '❌ 连接失败: ' + e.message, thinking, true); currentStreamEl.dataset.streaming = ''; }
      currentStreamEl = null;
    }
  }
  if (currentStreamEl) { currentStreamEl.dataset.streaming = ''; currentStreamEl = null; }
  state.isLoading = false; setChatBusy(false); abortCtrl = null;
  state.selfStreaming = false;
}

function stopChat() { if (abortCtrl) abortCtrl.abort(); if (state.isLoading) { state.isLoading = false; setChatBusy(false); if (state.sessionId) api('/api/chat/cancel', { method: 'POST', body: { session_id: state.sessionId } }).catch(()=>{}); } }

// ═══════════ 会话 ═══════════
async function loadSessions() { if (!state.novelId) return; try { const r = await api(`/api/sessions?novel_id=${state.novelId}&page=1&size=20`); state.sessions = r.items || []; } catch (_) {} }
function showSessions() {
  const list = document.getElementById('sessionList'); list.innerHTML = '';
  if (state.sessions.length) {
    state.sessions.forEach(s => {
      const el = tpl('tpl-session-item'); const root = el.firstElementChild;
      root.dataset.sid = s.session_id;
      qs(root, '.s-title').textContent = s.title || s.session_id.slice(0,20);
      const ph = qs(root, '.s-phase');
      if (s.current_phase) ph.textContent = '阶段: ' + s.current_phase;
      else ph.remove();
      list.appendChild(root);
    });
  } else {
    list.innerHTML = '<div style="padding:16px;text-align:center;color:var(--text2)">暂无历史会话</div>';
  }
  openSheet('sessionSheet');
}
async function loadSession(sid) { state.sessionId = sid; document.getElementById('chatMessages').innerHTML = ''; try { const r = await api(`/api/sessions/${sid}/messages`); (r.messages||[]).forEach(m => { if ((m.role==='user'||m.role==='assistant') && (m.content||m.thinking_content)) addMessage(m.role, m.content||'', m.thinking_content||''); }); } catch (_) {} toast('已加载会话'); }
function newChat() { state.sessionId = null; const c = document.getElementById('chatMessages'); c.innerHTML = ''; c.appendChild(emptyState('开始新的对话')); qs(c, '.em-hint') && (qs(c, '.em-hint').textContent = '输入消息开始创作'); }

// ═══════════ 模型 ═══════════
async function loadModels() { try { const r = await api('/api/settings/model'); state.models = r.models||[]; state.selectedModel = r.selected_model_key||''; } catch (_) {} }
function showModels() {
  const list = document.getElementById('modelList'); list.innerHTML = '';
  state.models.forEach(m => {
    const el = tpl('tpl-model-item'); const root = el.firstElementChild;
    root.dataset.key = m.key;
    if (m.key === state.selectedModel) root.classList.add('selected');
    const displayName = m.provider ? `${m.provider} / ${m.name}` : (m.name || m.key);
    qs(root, '.mod-name').textContent = displayName;
    if (m.thinking) {
      const badge = document.createElement('span'); badge.className = 'model-badge'; badge.textContent = '思考';
      root.appendChild(badge);
    }
    list.appendChild(root);
  });
  openSheet('modelSheet');
}
async function selectModel(key) { try { await api('/api/settings/model', { method: 'POST', body: { model_key: key } }); state.selectedModel = key; closeSheet('modelSheet'); toast('已切换'); showModels(); } catch (_) { toast('切换失败'); } }

// ═══════════ 设置 ═══════════
function sg(label, rows) {
  const el = document.createElement('div'); el.className = 'setting-group';
  const lbl = document.createElement('div'); lbl.className = 'setting-label'; lbl.textContent = label;
  el.appendChild(lbl);
  rows.forEach(r => {
    if (typeof r === 'string') el.insertAdjacentHTML('beforeend', r);
    else el.appendChild(r);
  });
  return el;
}
function authBlock(token) {
  const d = document.createElement('div');
  d.innerHTML = `<div style="display:flex;gap:8px;margin-top:4px"><input id="tokenField" type="text" placeholder="输入 32 位令牌" value="${esc(token)}" style="flex:1;padding:7px 10px;border:1px solid var(--border);border-radius:var(--radius-sm);font-size:12px;font-family:monospace;outline:none;background:var(--bg);color:var(--text)"><button onclick="saveTokenFromSettings()" style="padding:7px 14px;border:none;border-radius:var(--radius-sm);background:linear-gradient(135deg,var(--ice),var(--ice-light));color:#fff;font-size:12px;font-weight:600;cursor:pointer">保存</button></div><button onclick="startQRScanFromSettings()" style="width:100%;margin-top:8px;padding:7px;border:1px solid var(--border);border-radius:var(--radius-sm);background:var(--surface);color:var(--text);font-size:11px;cursor:pointer">📷 扫描二维码</button>`;
  return d;
}

async function loadSettings() {
  const isDark = getTheme() === 'dark';
  const lang = getLang();
  const token = getToken();
  try {
    const r = await api('/api/settings/model');
    if (r.error === 'unauthorized') {
      const el = document.getElementById('settingsContent'); el.innerHTML = '';
      el.appendChild(sg('🔐 API 认证', [authBlock(token)]));
      el.appendChild(sg('🎨 ' + t('appearance'), [settingRow(t('dark_mode').replace('Mode','').replace('模式',''), isDark?t('dark_mode'):t('light_mode'), toggleTheme, 'color:var(--ice)')]));
      el.appendChild(sg('🌐 Language', [settingRow('切换到', lang==='zh'?'English →':'中文 →', toggleLang, 'color:var(--ice)')]));
      return;
    }
    state.models = r.models||[]; state.selectedModel = r.selected_model_key||'';
    const found = state.models.find(m => m.key === state.selectedModel);
    const name = found ? (found.provider ? `${found.provider} / ${found.name}` : found.name) : state.selectedModel.split('/').pop() || '未选择';
    const el = document.getElementById('settingsContent'); el.innerHTML = '';
    el.appendChild(sg('🤖 ' + t('model'), [settingRow(t('current_model'), esc(name), showModels, 'color:var(--ice)')]));
    el.appendChild(sg('🎨 ' + t('appearance'), [settingRow(t('dark_mode').replace('Mode','').replace('模式',''), isDark?t('dark_mode'):t('light_mode'), toggleTheme, 'color:var(--ice)')]));
    el.appendChild(sg('🌐 Language', [settingRow('切换到', lang==='zh'?'English →':'中文 →', toggleLang, 'color:var(--ice)')]));
    el.appendChild(sg('🔐 API 认证', [authBlock(token)]));
    el.appendChild(sg('🔗 ' + t('server'), [
      settingRow(t('server'), esc(API.base), null, 'color:var(--text);font-size:12px'),
      settingRow(t('status'), API.connOk?'🟢 已连接':'🔴 离线', null, `color:${API.connOk?'var(--ice)':'var(--ice)'}`),
      settingRow('缓存', '在线自动同步', null, 'color:var(--text3);font-size:11px')
    ]));
  } catch (_) {
    const el = document.getElementById('settingsContent'); el.innerHTML = '';
    el.appendChild(sg('🔐 API 认证', [authBlock(token)]));
    el.appendChild(sg('🎨 ' + t('appearance'), [settingRow(t('dark_mode').replace('Mode','').replace('模式',''), isDark?t('dark_mode'):t('light_mode'), toggleTheme, 'color:var(--ice)')]));
    el.appendChild(sg('🌐 Language', [settingRow('切换到', lang==='zh'?'English →':'中文 →', toggleLang, 'color:var(--ice)')]));
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
  overlay = tpl('tpl-qr-scan').firstElementChild;
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
  // 从离线恢复时（重新连上局域网），尝试增量同步
  window.addEventListener('online', () => {
    API.connOk = false;
    if (getToken()) {
      toast('📡 网络已恢复，正在同步数据...');
      syncToOffline().then(() => {
        toast('✅ 数据同步完成');
        if (state.page === 'novels') loadNovels();
      }).catch(() => {});
    }
  });
});
