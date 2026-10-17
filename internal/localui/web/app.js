(() => {
  'use strict';
  const session = document.querySelector('meta[name="alpha-session"]')?.content || '';
  const $ = (id) => document.getElementById(id);
  const state = { data: null, sources: [], nodes: [], connection: null, view: 'overview', busy: false, lastCatalogRefresh: 0 };
  const pageTitles = { overview: 'مرکز کنترل شبکه', connect: 'اتصال هوشمند', nodes: 'کتابخانه کانفیگ‌ها', sources: 'منابع و همگام‌سازی', routes: 'مسیرها و موتور‌ها', diagnostics: 'عیب‌یابی سیستم' };
  const routeIcons = { 'diban-local': 'D', 'sing-box-local': 'S', 'xray-local': 'X', 'tor-local': 'T', 'psiphon-local': 'P', 'alpha-imported-sing-box': 'A' };

  function esc(value) {
    return String(value ?? '').replace(/[&<>"']/g, (char) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[char]);
  }
  function toast(message) {
    const element = $('toast');
    if (!element) return;
    element.textContent = message;
    element.classList.add('show');
    clearTimeout(toast.timer);
    toast.timer = setTimeout(() => element.classList.remove('show'), 3600);
  }
  function timeLabel(value) {
    if (!value) return 'هنوز بررسی نشده';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return 'نامشخص';
    return new Intl.DateTimeFormat('fa-IR', { hour: '2-digit', minute: '2-digit', month: 'short', day: 'numeric' }).format(date);
  }
  async function api(path, options = {}) {
    const headers = { 'Accept': 'application/json', ...(options.headers || {}) };
    if (options.method && options.method !== 'GET') {
      headers['X-Alpha-Session'] = session;
      headers['Content-Type'] = 'application/json';
    }
    const response = await fetch(path, { cache: 'no-store', ...options, headers });
    if (!response.ok) {
      let message = '';
      try { message = (await response.text()).slice(0, 240); } catch (_) { /* keep status */ }
      throw new Error(message || `HTTP ${response.status}`);
    }
    return response.json();
  }
  function routeCard(engine) {
    const healthy = !!engine.healthy;
    const label = healthy ? 'سالم' : (engine.last_check ? 'ناموفق' : 'در انتظار');
    const latency = healthy ? `${Math.max(1, Number(engine.latency_ms) || 1)} <small>ms</small>` : '—';
    const error = engine.error ? `<small class="error-detail">${esc(engine.error)}</small>` : '';
    return `<div class="route-row ${healthy ? 'healthy' : ''}">
      <div class="route-left"><span class="route-icon">${esc(routeIcons[engine.id] || '↔')}</span><div class="route-copy"><b>${esc(engine.name)}</b><small>${esc(engine.address)}</small>${error}</div></div>
      <div class="route-right"><span class="pill ${healthy ? 'good' : (engine.last_check ? 'bad' : '')}">${label}</span><span class="route-latency">${latency}</span></div></div>`;
  }
  function renderState(data) {
    state.data = data;
    $('sideVersion').textContent = `نسخه ${data.version}`;
    $('footerVersion').textContent = data.version;
    $('proxyAddress') && ($('proxyAddress').textContent = data.listen || '—');
    const engines = data.engines || [];
    const healthy = engines.filter((engine) => engine.healthy).length;
    const latencies = engines.filter((engine) => engine.healthy).map((engine) => Number(engine.latency_ms) || 0);
    $('healthyValue').innerHTML = `${healthy}<small id="totalValue">/ ${engines.length}</small>`;
    $('healthyFoot').textContent = engines.length ? `${healthy} از ${engines.length} مسیر آزموده‌شده سالم است` : 'موتور فعالی ثبت نشده است';
    $('healthBar').style.width = `${engines.length ? Math.round(100 * healthy / engines.length) : 0}%`;
    $('latencyValue').textContent = latencies.length ? `LATENCY ${Math.min(...latencies)} ms` : 'LATENCY —';
    $('routeList').innerHTML = engines.length ? engines.slice(0, 5).map(routeCard).join('') : '<div class="empty-state">هیچ موتور فعالی در تنظیمات وجود ندارد.</div>';
    $('allRoutes').innerHTML = engines.length ? engines.map(routeCard).join('') : '<div class="empty-state">موتوری تنظیم نشده است.</div>';
    $('probeTargets').innerHTML = (data.probe_targets || []).map((url) => `<span class="target-chip">${esc(url)}</span>`).join('') || '<span class="subtle-help">مقصدی تنظیم نشده است.</span>';
    $('systemPill').textContent = healthy ? `${healthy} مسیر سالم` : 'اتصال نیازمند بررسی';
    $('systemPill').className = `pill ${healthy ? 'good' : ''}`;
    $('diagSummary').textContent = data.safety_message || 'وضعیت زنده از هسته محلی دریافت شد.';
    $('diagLog').textContent = [
      `Alpha ${data.version}`, `Router SOCKS5: ${data.listen}`, `UI: ${data.ui_address}`,
      `Checked: ${data.checked_at || 'not checked'}`, `Healthy routes: ${healthy}/${engines.length}`, '',
      ...engines.map((engine) => `${engine.healthy ? '[OK]' : '[DOWN]'} ${engine.id} | ${engine.address} | ${engine.healthy ? `${engine.latency_ms}ms` : (engine.error || 'not checked')}`)
    ].join('\n');
    const hasHealthy = healthy > 0;
    $('headline').innerHTML = hasHealthy ? 'یک مسیر پاسخ‌گو.<br><em>مسیر جایگزین را هم آماده نگه دار.</em>' : 'یک نقطه ورود.<br><em>چندین مسیر جایگزین.</em>';
  }
  function renderCatalog(catalog) {
    state.sources = catalog.sources || [];
    state.nodes = catalog.nodes || [];
    $('nodeValue').textContent = String(catalog.node_count ?? state.nodes.length);
    $('navNodeCount').textContent = String(catalog.node_count ?? state.nodes.length);
    $('nodeTotalPill').textContent = `${catalog.node_count ?? state.nodes.length} کانفیگ`;
    $('nodeFoot').textContent = state.nodes.length ? 'برای اتصال هوشمند آمادهٔ بررسی' : 'لینک یا اشتراک وارد کن';
    $('nodeProgress').style.width = `${Math.min(100, state.nodes.length ? 100 : 0)}%`;
    $('sourceValue').textContent = String(state.sources.length);
    $('sourceTotalPill').textContent = `${state.sources.length} منبع`;
    $('sourceFoot').textContent = state.sources.length ? 'همگام‌سازی خودکار هر ۱۵ دقیقه' : 'هنوز منبعی دنبال نمی‌شود';
    renderNodes();
    renderSources();
    renderConnection(state.connection || {});
  }
  function renderNodes() {
    if (!state.nodes.length) {
      $('nodeList').innerHTML = '<div class="empty-state">هیچ کانفیگی وارد نشده است. از کادر بالا لینک‌های اشتراک یا متن Base64 را وارد کن.</div>';
      return;
    }
    $('nodeList').innerHTML = state.nodes.map((node) => `<article class="node-card"><div class="node-head"><div class="node-title">${esc(node.name || 'Unnamed node')}</div><span class="pill good">ذخیره‌شده</span></div><div class="node-meta"><span class="meta-chip protocol">${esc(String(node.protocol || 'unknown').toUpperCase())}</span><span class="meta-chip">${esc(node.server || 'server')}</span><span class="meta-chip">:${esc(node.port || '—')}</span></div><div class="node-source">منبع: ${esc(node.source || 'واردسازی دستی')} · به‌روزرسانی ${esc(timeLabel(node.updated_at))}</div><div class="node-actions"><button class="small-action" data-action="use-node">تلاش برای اتصال</button><button class="danger-btn" data-delete-node="${esc(node.id)}">حذف</button></div></article>`).join('');
  }
  function renderSources() {
    if (!state.sources.length) {
      $('sourceList').innerHTML = '<div class="empty-state">هنوز منبعی ثبت نشده است. کانال عمومی یا لینک اشتراک HTTPS خودت را اضافه کن.</div>';
      return;
    }
    $('sourceList').innerHTML = state.sources.map((source) => {
      const status = source.last_error ? `<small class="source-error">آخرین خطا: ${esc(source.last_error)}</small>` : `<small>آخرین بررسی: ${esc(timeLabel(source.last_fetch))} · ${Number(source.imported) || 0} نود جدید</small>`;
      return `<article class="source-card"><div class="source-info"><b>${esc(source.name)}</b><small>${esc(source.kind === 'telegram' ? `کانال عمومی تلگرام · @${source.host}` : `HTTPS · ${source.host}`)}</small>${status}</div><div class="source-actions"><span class="pill ${source.last_error ? 'bad' : 'good'}">${source.last_error ? 'نیازمند بررسی' : (source.last_fetch ? 'فعال' : 'در صف دریافت')}</span><button class="danger-btn" data-delete-source="${esc(source.id)}">حذف</button></div></article>`;
    }).join('');
  }
  function renderConnection(connection) {
    state.connection = connection;
    const active = !!connection.active;
    const statusText = active ? ({ starting: 'در حال راه‌اندازی موتور', connecting: 'در حال آزمایش مسیرها', reconnecting: 'اتصال قطع شد؛ تلاش مجدد', retrying: 'موتور اجرا نشد؛ تلاش مجدد', 'applying-updated-nodes': 'اعمال کانفیگ‌های به‌روز‌شده' }[connection.status] || 'اتصال هوشمند فعال است') : 'اتصال برقرار نیست';
    $('connectionValue').textContent = active ? 'فعال' : 'خاموش';
    $('connectionValue').style.color = active ? 'var(--mint)' : 'var(--muted)';
    $('connectionFoot').textContent = connection.engine_available ? `${connection.node_count || 0} نود در کتابخانه · ${connection.status || 'disconnected'}` : 'موتور sing-box نصب نشده است';
    $('connectionTitle').textContent = statusText;
    $('connectionDetail').textContent = connection.error || (connection.engine_available ? `${connection.node_count || 0} نود در گروه انتخاب خودکار · ورودی SOCKS5 محلی آماده می‌شود` : 'موتور sing-box در PATH یا پوشهٔ engines پیدا نشد.');
    $('connectionDot').className = `status-light ${active ? (connection.status === 'connecting' ? 'active' : 'connected') : ''}`;
    $('sideStatusDot').className = `status-dot ${active ? 'connected' : ''}`;
    $('sideStatusTitle').textContent = active ? 'اتصال هوشمند فعال' : 'هسته محلی فعال';
    $('heroLiveDot').classList.toggle('active', active);
    $('connectBtn').disabled = active || !(Number(connection.node_count) > 0) || !connection.engine_available;
    $('disconnectBtn').disabled = !active;
    $('heroConnectBtn').textContent = active ? 'اتصال در حال اجرا' : '⏻ اتصال هوشمند';
    $('connectProxy').textContent = connection.proxy || state.data?.listen || '127.0.0.1:1088';
    $('readyNodes').textContent = `${Number(connection.node_count) || 0} کانفیگ ذخیره‌شده`;
    $('readyEngine').textContent = connection.engine_available ? `یافت شد: ${connection.engine_path || 'sing-box'}` : 'نصب نیست یا در PATH وجود ندارد';
  }
  async function refreshState(manual = false) {
    try {
      const [data, connection] = await Promise.all([api('/api/state'), api('/api/connection')]);
      state.connection = connection;
      renderState(data);
      renderConnection(connection);
      if (Date.now() - state.lastCatalogRefresh > 25000) {
        const [sources, nodes] = await Promise.all([api('/api/sources'), api('/api/nodes')]);
        renderCatalog({ ...sources, ...nodes, sources: sources.sources, nodes: nodes.nodes, node_count: nodes.node_count });
        state.lastCatalogRefresh = Date.now();
      }
      if (manual) toast('وضعیت آلفا به‌روز شد.');
    } catch (error) {
      if (manual) toast(`خواندن وضعیت ناموفق بود: ${error.message}`);
    }
  }
  function showView(view) {
    if (!pageTitles[view]) return;
    state.view = view;
    document.querySelectorAll('.view').forEach((section) => section.classList.toggle('hidden', section.id !== `${view}View`));
    document.querySelectorAll('[data-view]').forEach((button) => button.classList.toggle('active', button.dataset.view === view && (button.classList.contains('nav-item') || button.classList.contains('mobile-nav-item'))));
    $('pageTitle').textContent = pageTitles[view];
    try { history.replaceState(null, '', `#${view}`); } catch (_) { /* optional in embedded browser */ }
    if (view === 'sources' || view === 'nodes' || view === 'connect') void refreshCatalog();
    if (view === 'routes' || view === 'diagnostics') void refreshState(false);
    window.scrollTo({ top: 0, behavior: 'smooth' });
  }
  async function refreshCatalog() {
    try {
      const [sources, nodes, connection] = await Promise.all([api('/api/sources'), api('/api/nodes'), api('/api/connection')]);
      state.connection = connection;
      renderCatalog({ sources: sources.sources, node_count: nodes.node_count, nodes: nodes.nodes, updated_at: nodes.updated_at });
    } catch (error) { toast(`به‌روزرسانی فهرست ناموفق بود: ${error.message}`); }
  }
  async function probe() {
    if (state.busy) return;
    state.busy = true;
    $('routesProbeBtn').disabled = true;
    try {
      const data = await api('/api/probe', { method: 'POST', body: '{}' });
      renderState(data);
      toast(`بررسی مسیرها تمام شد: ${data.healthy}/${data.total} سالم`);
    } catch (error) { toast(`آزمون مسیر ناموفق بود: ${error.message}`); }
    finally { state.busy = false; $('routesProbeBtn').disabled = false; }
  }
  async function connect(action = 'connect') {
    if (state.busy) return;
    state.busy = true;
    $('connectBtn').disabled = true;
    $('heroConnectBtn').disabled = true;
    try {
      const response = await api('/api/connect', { method: 'POST', body: JSON.stringify({ action }) });
      renderConnection(response);
      toast(action === 'connect' ? 'موتور شروع شد؛ آلفا در حال آزمودن مسیرهاست.' : 'اتصال هوشمند متوقف شد.');
      setTimeout(() => refreshState(false), 1200);
    } catch (error) { toast(`عملیات اتصال ناموفق بود: ${error.message}`); }
    finally { state.busy = false; $('heroConnectBtn').disabled = false; await refreshState(false); }
  }
  async function addSource(name, url) {
    const response = await api('/api/sources', { method: 'POST', body: JSON.stringify({ name, url }) });
    toast(response.message || 'منبع اضافه شد.');
    $('sourceName').value = '';
    $('sourceURL').value = '';
    await refreshCatalog();
    setTimeout(() => { void refreshCatalog(); }, 2500);
  }
  async function refreshSources() {
    if (state.busy) return;
    state.busy = true;
    $('refreshSourcesBtn').disabled = true;
    $('refreshSourcesBtn').textContent = 'در حال همگام‌سازی…';
    try {
      const result = await api('/api/sources/refresh', { method: 'POST', body: '{}' });
      toast(`همگام‌سازی انجام شد: ${result.added || 0} کانفیگ جدید، ${result.failed_sources || 0} منبع ناموفق`);
      await refreshCatalog();
    } catch (error) { toast(`همگام‌سازی ناموفق بود: ${error.message}`); }
    finally { state.busy = false; $('refreshSourcesBtn').disabled = false; $('refreshSourcesBtn').textContent = '↻ همگام‌سازی الآن'; }
  }
  async function copyText(value) {
    try { await navigator.clipboard.writeText(value); toast('کپی شد.'); }
    catch (_) { const area = document.createElement('textarea'); area.value = value; document.body.appendChild(area); area.select(); document.execCommand('copy'); area.remove(); toast('کپی شد.'); }
  }

  document.querySelectorAll('[data-view]').forEach((button) => button.addEventListener('click', (event) => { event.preventDefault(); showView(button.dataset.view); }));
  $('refreshBtn').addEventListener('click', () => { state.lastCatalogRefresh = 0; void refreshState(true); });
  $('probeBtn')?.addEventListener('click', probe);
  $('routesProbeBtn').addEventListener('click', probe);
  $('connectBtn').addEventListener('click', () => connect('connect'));
  $('heroConnectBtn').addEventListener('click', () => { if (!state.connection?.active) { showView('connect'); void connect('connect'); } else showView('connect'); });
  $('disconnectBtn').addEventListener('click', () => connect('disconnect'));
  $('copyProxyBtn').addEventListener('click', () => copyText(`SOCKS5\nHost: 127.0.0.1\nPort: ${state.data?.listen?.split(':').pop() || '1088'}`));
  $('diagCopyBtn').addEventListener('click', () => copyText($('diagLog').textContent));
  $('importForm').addEventListener('submit', async (event) => {
    event.preventDefault();
    const text = $('importText').value.trim();
    if (!text) return;
    $('importBtn').disabled = true;
    try {
      const result = await api('/api/nodes/import', { method: 'POST', body: JSON.stringify({ text }) });
      $('importText').value = '';
      toast(`واردسازی انجام شد: ${result.added || 0} کانفیگ جدید${result.skipped ? `، ${result.skipped} مورد رد شد` : ''}`);
      state.lastCatalogRefresh = 0;
      await refreshCatalog();
    } catch (error) { toast(`واردسازی ناموفق بود: ${error.message}`); }
    finally { $('importBtn').disabled = false; }
  });
  $('sourceForm').addEventListener('submit', async (event) => {
    event.preventDefault();
    $('addSourceBtn').disabled = true;
    try { await addSource($('sourceName').value.trim(), $('sourceURL').value.trim()); }
    catch (error) { toast(`افزودن منبع ناموفق بود: ${error.message}`); }
    finally { $('addSourceBtn').disabled = false; }
  });
  $('refreshSourcesBtn').addEventListener('click', refreshSources);
  document.querySelectorAll('[data-preset]').forEach((button) => button.addEventListener('click', async () => {
    const preset = button.dataset.preset;
    const data = preset === 'thefeed' ? { name: 'TheFeed Configs', url: '@thefeedconfig' } : { name: 'NetworkTI', url: '@networkti' };
    button.disabled = true;
    try { await addSource(data.name, data.url); toast(`منبع ${data.name} اضافه شد.`); }
    catch (error) { toast(`افزودن منبع ناموفق بود: ${error.message}`); }
    finally { button.disabled = false; }
  }));
  document.addEventListener('click', async (event) => {
    const nodeButton = event.target.closest('[data-delete-node]');
    if (nodeButton) {
      const id = nodeButton.dataset.deleteNode;
      nodeButton.disabled = true;
      try { await api(`/api/nodes?id=${encodeURIComponent(id)}`, { method: 'DELETE', body: '{}' }); state.lastCatalogRefresh = 0; await refreshCatalog(); toast('کانفیگ حذف شد.'); }
      catch (error) { toast(`حذف کانفیگ ناموفق بود: ${error.message}`); }
      return;
    }
    const sourceButton = event.target.closest('[data-delete-source]');
    if (sourceButton) {
      const id = sourceButton.dataset.deleteSource;
      sourceButton.disabled = true;
      try { await api(`/api/sources?id=${encodeURIComponent(id)}`, { method: 'DELETE', body: '{}' }); state.lastCatalogRefresh = 0; await refreshCatalog(); toast('منبع حذف شد؛ نودهای قبلاً واردشده باقی می‌مانند.'); }
      catch (error) { toast(`حذف منبع ناموفق بود: ${error.message}`); }
      return;
    }
    const useNodeButton = event.target.closest('[data-action="use-node"]');
    if (useNodeButton) { showView('connect'); await connect('connect'); }
  });

  const initialView = (location.hash || '').slice(1);
  showView(pageTitles[initialView] ? initialView : 'overview');
  void refreshState(false);
  setInterval(() => void refreshState(false), 10000);
  if ('serviceWorker' in navigator) navigator.serviceWorker.register('/sw.js').catch(() => {});
})();
