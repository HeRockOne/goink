// Goink Mobile — Service Worker
// 缓存网页资源 + API 数据，确保离线可用
// v2：预缓存补 idb-keyval.min.js；静态资源 cache-first 回填；API 只缓存 GET 且带 TTL
const CACHE = 'goink-v2';
const API_CACHE_TTL = 10 * 60 * 1000; // API 缓存 10 分钟过期（时间戳存在响应头）

self.addEventListener('install', (e) => {
  self.skipWaiting();
  e.waitUntil(
    caches.open(CACHE).then(c => c.addAll([
      './',
      './index.html',
      './style.css',
      './app.js',
      './marked.min.js',
      './jsQR.js',
      './idb-keyval.min.js',
      './wspulse.mjs',
      './manifest.json',
    ]))
  );
});

self.addEventListener('activate', (e) => {
  e.waitUntil(
    caches.keys().then(keys => Promise.all(keys.filter(k => k !== CACHE).map(k => caches.delete(k))))
      .then(() => self.clients.claim())
  );
});

self.addEventListener('fetch', (e) => {
  const req = e.request;
  // 只处理 GET；POST（如 /api/chat）等直接走网络
  if (req.method !== 'GET') return;

  // API 请求：网络优先，失败时用缓存兜底（带 TTL）
  if (req.url.includes('/api/')) {
    e.respondWith(
      fetch(req).then(r => {
        if (r.ok) {
          const clone = r.clone();
          const headers = new Headers(r.headers);
          headers.set('sw-cached-at', String(Date.now()));
          const body = r.body;
          const stamped = new Response(body, { status: r.status, statusText: r.statusText, headers });
          caches.open(CACHE).then(c => c.put(req, stamped));
        }
        return r;
      }).catch(() => caches.match(req).then(r => {
        if (!r) return new Response(JSON.stringify({ error: 'offline', _offline: true }), { headers: { 'Content-Type': 'application/json' } });
        const at = Number(r.headers.get('sw-cached-at') || 0);
        if (at && Date.now() - at > API_CACHE_TTL) {
          return new Response(JSON.stringify({ error: 'offline_stale', _offline: true }), { headers: { 'Content-Type': 'application/json' } });
        }
        return r;
      }))
    );
    return;
  }
  // 静态资源：缓存优先，未命中时回源并回填缓存
  e.respondWith(
    caches.match(req).then(r => r || fetch(req).then(resp => {
      if (resp.ok) {
        const clone = resp.clone();
        caches.open(CACHE).then(c => c.put(req, clone));
      }
      return resp;
    }))
  );
});
