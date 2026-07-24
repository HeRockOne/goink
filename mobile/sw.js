// Goink Mobile — Service Worker
// 缓存网页资源 + API 数据，确保离线可用
const CACHE = 'goink-v1';

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
      './wspulse.mjs',
      './manifest.json',
    ]))
  );
});

self.addEventListener('activate', (e) => {
  e.waitUntil(
    caches.keys().then(keys => Promise.all(keys.filter(k => k !== CACHE).map(k => caches.delete(k))))
  );
});

self.addEventListener('fetch', (e) => {
  // API 请求：网络优先，失败时用缓存兜底
  if (e.request.url.includes('/api/')) {
    e.respondWith(
      fetch(e.request).then(r => {
        const clone = r.clone();
        caches.open(CACHE).then(c => c.put(e.request, clone));
        return r;
      }).catch(() => caches.match(e.request).then(r => r || new Response(JSON.stringify({ error: 'offline', _offline: true }), { headers: { 'Content-Type': 'application/json' } })))
    );
    return;
  }
  // 静态资源：缓存优先
  e.respondWith(
    caches.match(e.request).then(r => r || fetch(e.request))
  );
});
