// ==UserScript==
// @name         OKX Copy Trade Monitor 07 (18771)
// @namespace    https://kunai.vip
// @version      1.0.17
// @description  拦截 OKX 带单/跟单页持仓 API + follow-detail DOM 多仓解析
// @author       auto
// @match        https://www.okx.com/*
// @match        https://okx.com/*
// @match        https://www.okx.com/zh-hans/*
// @match        https://www.okx.com/zh-cn/*
// @grant        GM_xmlhttpRequest
// @connect      kunai.vip
// @connect      *
// @connect      127.0.0.1
// @connect      localhost
// @connect      8.218.56.54
// @run-at       document-start
// @inject-into  page
// ==/UserScript==

(function () {
  'use strict';

  const SERVER = 'https://kunai.vip/api/okx-scraper-relay?route=07';
  const RELAY_TOKEN = 'e6428f6dbb9652b9727b0f0473646a27897e896324721d832cf1fa3f0a526ecf';
  const POST_MIN_MS = 5000;
  const KEEPALIVE_POST_MS = 25000;
  const SCRIPT_VER = '1.0.17';
  const LEAD_MGMT_URL = 'https://www.okx.com/zh-hans/copy-trading/account/manage';
  const POS_CACHE_KEY = 'okx-api-pos-v117-' + SERVER;
  const POS_LOCAL_PREFIX = 'okx-api-pos-local-v117-';
  const POS_LOCAL_TTL_MS = 7 * 24 * 3600 * 1000;
  const POLL_MS = 8000;
  const DOM_POLL_MS = 5000;
  const STALE_DATA_RELOAD_MS = 300000;
  const PHANTOM_MARGIN_USDT = 12500;

  const SWAP_CT_DEFAULT = {
    'ETH-USDT-SWAP': 0.1,
    'BTC-USDT-SWAP': 0.01,
  };

  const SRC_PRIORITY = { dom: 100, private: 80, lead: 60, public: 10 };

  const nativeFetch = window.fetch.bind(window);
  let downstreamFetch = nativeFetch;
  let fetchTrapOk = false;

  let lastSnapshotComplete = false;

  function purgePhantomPositions(reason) {
    let n = 0;
    Object.keys(positions).forEach(function (sym) {
      const row = positions[sym];
      if (!row) return;
      if (sym === 'HYPEUSDT') {
        delete positions[sym];
        delete positionMeta[sym];
        n++;
        return;
      }
      const margin = readPx(row.margin_used);
      const qty = readPx(row.quantity);
      const px = readPx(row.mark_price || row.entry_price);
      if (margin > PHANTOM_MARGIN_USDT) {
        delete positions[sym];
        delete positionMeta[sym];
        n++;
      }
    });
    if (n > 0) {
      log('清除 phantom ' + n + ' 仓' + (reason ? ' (' + reason + ')' : ''));
      savePosCache();
    }
  }
  let positions = {};
  let positionMeta = {};
  let ctValMap = {};
  let lastPayloadSig = '';
  let apiHits = 0;
  let emptyStreak = 0;
  let domEmptyStreak = 0;
  let lastPostAt = 0;
  let lastPostHash = '';
  let postTimer = null;
  let sendInFlight = false;
  let resolvedUniqueCode = '';
  let lastDataChangeAt = Date.now();

  function log(msg) {
    console.log('[OKX-API v' + SCRIPT_VER + '] ' + msg);
  }

  function isFollowDetailPage() {
    return /follow-detail/i.test(location.pathname || '');
  }

  function isLeadManagePage() {
    return /\/account\/manage/i.test(location.pathname || '');
  }

  function instIdToSymbol(instId) {
    const parts = String(instId || '').split('-');
    if (parts.length >= 2) return (parts[0] + parts[1]).toUpperCase();
    return String(instId || '').replace(/-/g, '').toUpperCase();
  }

  function parseNum(raw) {
    if (raw == null || raw === '') return 0;
    const n = Number(String(raw).replace(/,/g, '').trim());
    return isFinite(n) ? n : 0;
  }

  /** 保留 API 原始小数，不做四舍五入 */
  function readPx(raw) {
    return parseNum(raw);
  }

  let debugUrlLogged = 0;

  function isSubPositionsUrl(url) {
    const u = String(url || '').toLowerCase();
    if (/current-subpositions/i.test(u)) return true;
    if (/current-sub-positions/i.test(u)) return true;
    if (/public-current-subpositions/i.test(u)) return true;
    if (/performance-current-subpositions/i.test(u)) return true;
    if (/copytrading/i.test(u) && /subposition/i.test(u)) return true;
    if (/ecotrade/i.test(u) && /subposition/i.test(u)) return true;
    return false;
  }

  function isOkxHost(url) {
    return /okx\.com/i.test(String(url || ''));
  }

  function isCopyTradingUrl(url) {
    if (!isOkxHost(url)) return false;
    const u = String(url || '').toLowerCase();
    return /copytrading|ecotrade|\/api\/v5\/|priapi\/v5/i.test(u);
  }

  function maybeLogCopyUrl(url) {
    if (!isCopyTradingUrl(url) || debugUrlLogged >= 24) return;
    debugUrlLogged++;
    log('OKX API: ' + String(url).slice(0, 180));
  }

  function isProductsUrl(url) {
    const u = String(url || '').toLowerCase();
    return /\/products\b/.test(u) && /insttype=swap/i.test(u);
  }

  function urlSourceKind(url) {
    const u = String(url || '').toLowerCase();
    if (/public-current-subpositions/i.test(u)) return 'public';
    if (/priapi.*private|performance-current/i.test(u)) return 'private';
    if (/current-subpositions/i.test(u)) return 'lead';
    if (/ecotrade/i.test(u) && !/public\//i.test(u)) return 'private';
    return 'lead';
  }

  function looksLikeSubPosRow(item) {
    if (!item || typeof item !== 'object') return false;
    const instId = item.instId || item.instID;
    if (!instId) return false;
    if ('subPos' in item || 'subPosId' in item) return true;
    if ('pos' in item || 'availPos' in item) {
      return !!(item.posSide || item.side || item.mgnMode);
    }
    return false;
  }

  function extractSubPosList(data) {
    if (!data || typeof data !== 'object') return null;
    if (String(data.code) !== '0' || !Array.isArray(data.data)) return null;
    if (data.data.length === 0) return [];
    if (looksLikeSubPosRow(data.data[0])) return data.data;
    return null;
  }

  function deepFindSubPosList(data, depth) {
    depth = depth || 0;
    if (!data || depth > 4) return null;
    const direct = extractSubPosList(data);
    if (direct !== null) return direct;
    if (Array.isArray(data)) {
      for (let i = 0; i < data.length; i++) {
        const hit = deepFindSubPosList(data[i], depth + 1);
        if (hit !== null) return hit;
      }
      return null;
    }
    if (typeof data === 'object') {
      const keys = ['data', 'list', 'positions', 'subPositions', 'subPosList', 'currentSubPos'];
      for (let i = 0; i < keys.length; i++) {
        const hit = deepFindSubPosList(data[keys[i]], depth + 1);
        if (hit !== null) return hit;
      }
    }
    return null;
  }

  function coinsFromSubPos(instId, rawPos, item) {
    const abs = Math.abs(rawPos);
    const fromSubPos = item && ('subPos' in item || 'subPosId' in item);
    if (!fromSubPos || instId.indexOf('SWAP') < 0) return abs;
    const cv = ctValMap[instId] || parseFloat(item.ctVal || '0') || SWAP_CT_DEFAULT[instId] || 0.1;
    return abs * cv;
  }

  function mapSubPosRow(item, url) {
    const instId = String(item.instId || item.instID || '');
    const sym = instIdToSymbol(instId);
    const rawPos = parseFloat(item.subPos || item.pos || item.availSubPos || item.availPos || '0');
    if (!sym || !rawPos) return null;

    const ps = String(item.posSide || item.side || '').toLowerCase();
    let side = 'long';
    if (ps === 'short' || ps === 'sell') side = 'short';
    else if (ps === 'long' || ps === 'buy') side = 'long';
    else if (ps === 'net') side = rawPos >= 0 ? 'long' : 'short';
    else if (rawPos < 0) side = 'short';

    const qty = coinsFromSubPos(instId, rawPos, item);
    const entry = readPx(item.openAvgPx || item.avgPx || item.openPx);
    const mark = readPx(item.markPx);
    const pnl = readPx(item.upl);
    let margin = readPx(item.margin || item.imr || item.mgn);
    let lev = parseInt(String(item.lever || '0').replace(/[^\d]/g, ''), 10) || 0;
    const px = mark > 0 ? mark : entry;

    if (lev <= 0 && margin > 0 && qty > 0 && px > 0) {
      lev = Math.round((qty * px) / margin);
    }
    if (lev <= 0) lev = 20;
    if (lev > 125) lev = 125;
    if (margin <= 0 && qty > 0 && px > 0 && lev > 0) margin = (qty * px) / lev;
    if (entry <= 0 || qty <= 0) return null;
    if (margin < 0.01 && qty * px < 1) return null;
    if (sym === 'HYPEUSDT') return null;
    if (margin > PHANTOM_MARGIN_USDT) return null;

    if (isFollowDetailPage() && urlSourceKind(url) === 'public') {
      return null;
    }

    return {
      symbol: sym,
      side: side,
      entry_price: entry,
      quantity: qty,
      leverage: lev,
      margin_used: margin,
      mark_price: px,
      unrealized_pnl: pnl,
      update_time: Math.floor(Date.now() / 1000),
      okx_inst_id: instId,
      okx_sub_pos_id: String(item.subPosId || ''),
    };
  }

  /** 从「当前仓位」区块取全文（不截断，避免漏掉下方第二仓） */
  function domSliceAfterPositionTitle(text) {
    const blocks = text.split(/当前仓位|Current position/i);
    if (blocks.length <= 1) return text.slice(0, 16000);
    let chunk = blocks[1];
    const endMatch = chunk.match(/(?:^|\n)\s*(历史仓位|Historical|成交记录|Trade history|跟单记录)/im);
    if (endMatch && endMatch.index > 0) chunk = chunk.slice(0, endMatch.index);
    return chunk.slice(0, 16000);
  }

  function domConfirmsNoCurrentPositions() {
    if (!isFollowDetailPage()) return false;
    const text = document.body ? (document.body.innerText || "") : "";
    if (!/当前仓位|Current position/i.test(text)) return false;
    const chunk = domSliceAfterPositionTitle(text).slice(0, 2000);
    if (/[A-Z0-9]{2,20}USDT/i.test(chunk)) return false;
    return /暂无(?:数据|持仓|仓位|带单)|当前暂无|无(?:当前)?(?:持仓|仓位|带单)|No (?:open |current )?positions?|No data/i.test(chunk);
  }

  /** 按 *USDT 永续标题拆成多块（同页多仓） */
  function splitDomPositionBlocks(chunk) {
    const re = /(?:^|\n)\s*([A-Z0-9]{2,20}USDT)\s*(?:永续|Perpetual|SWAP)?/gi;
    const matches = [];
    let m;
    while ((m = re.exec(chunk)) !== null) {
      matches.push({ sym: m[1].toUpperCase(), start: m.index });
    }
    if (matches.length === 0) return [];
    const out = [];
    for (let i = 0; i < matches.length; i++) {
      const coin = matches[i].sym.replace(/USDT$/i, '');
      const start = matches[i].start;
      const end = i + 1 < matches.length ? matches[i + 1].start : chunk.length;
      out.push({ coin: coin, chunk: chunk.slice(start, end) });
    }
    return out;
  }

  function parseOneDomBlock(coin, blockChunk) {
    coin = (coin || 'ETH').toUpperCase();
    const sym = coin + 'USDT';
    const chunk = blockChunk || '';

    const qty = domQtyCoins(chunk, coin);
    let entry = domValueAfterLabel(chunk, '开仓均价', 80) || domValueAfterLabel(chunk, 'Avg\\. entry', 80);
    let mark = domValueAfterLabel(chunk, '标记价格', 80) || domValueAfterLabel(chunk, 'Mark price', 80);
    const pnl = domFloatPnl(chunk);
    const sideLevM = chunk.match(/(?:全仓\s*)?(空|多|Short|Long)\s*(\d+)\s*[xX×]/i);

    let side = 'long';
    let lev = 20;
    if (sideLevM) {
      const sd = sideLevM[1].toLowerCase();
      side = (sd === '空' || sd === 'short') ? 'short' : 'long';
      lev = parseInt(sideLevM[2], 10) || 20;
    }

    if (!isPlausiblePrice(coin, entry)) entry = 0;
    if (!isPlausiblePrice(coin, mark)) mark = entry > 0 ? entry : 0;
    if (!isPlausibleQty(coin, qty)) return null;

    const margin = resolveDomMargin(qty, mark, entry, lev, chunk);
    if (!qty || !entry || margin <= 0) return null;

    return {
      symbol: sym,
      side: side,
      entry_price: entry,
      quantity: qty,
      leverage: lev,
      margin_used: margin,
      mark_price: mark > 0 ? mark : entry,
      unrealized_pnl: pnl,
      update_time: Math.floor(Date.now() / 1000),
      okx_inst_id: coin + '-USDT-SWAP',
      okx_sub_pos_id: '',
    };
  }

  /** 标签后紧跟的数值（不跳过第一个数；避开「维持保证金率」里的「保证金」） */
  function domValueAfterLabel(chunk, label, maxLen, skipIfPrefix) {
    let pos = 0;
    const lab = String(label);
    while (pos < chunk.length) {
      const idx = chunk.indexOf(lab, pos);
      if (idx < 0) return 0;
      if (skipIfPrefix && idx >= skipIfPrefix.length && chunk.slice(idx - skipIfPrefix.length, idx) === skipIfPrefix) {
        pos = idx + lab.length;
        continue;
      }
      const slice = chunk.slice(idx + lab.length, idx + lab.length + (maxLen || 100));
      const m = slice.match(/^\s*[:：]?\s*([+-]?[\d,]+(?:\.\d+)?)/);
      if (m) return parseNum(m[1]);
      pos = idx + lab.length;
    }
    return 0;
  }

  function domQtyCoins(chunk, coin) {
    const idx = chunk.indexOf('持仓量');
    if (idx < 0) return domValueAfterLabel(chunk, 'Position size', 80) || domValueAfterLabel(chunk, 'Size', 80);
    const slice = chunk.slice(idx + 3, idx + 90);
    const m = slice.match(/^\s*[:：]?\s*([+-]?[\d,]+(?:\.\d+)?)\s*(?:[A-Z0-9]{2,20}|张)?/i);
    return m ? parseNum(m[1]) : 0;
  }

  function domMarginUsdt(chunk, mark, entry) {
    const raw = domValueAfterLabel(chunk, '保证金', 60, '维持');
    if (raw > 0) return raw;
    const m = chunk.match(/Margin(?! Ratio)[\s\S]{0,20}?([\d,]+\.?\d*)\s*USDT/i);
    return m ? parseNum(m[1]) : 0;
  }

  function domFloatPnl(chunk) {
    const m = chunk.match(/浮动(?:收益|盈亏)[\s\S]{0,40}?([+-]?[\d,]+\.?\d*)\s*USDT/i);
    if (m) return parseNum(m[1]);
    const m2 = chunk.match(/Unrealized P&L[\s\S]{0,40}?([+-]?[\d,]+\.?\d*)\s*USDT/i);
    return m2 ? parseNum(m2[1]) : 0;
  }

  function isPlausiblePrice(coin, px) {
    if (px <= 0) return false;
    if (coin === 'BTC') return px >= 5000 && px <= 500000;
    if (coin === 'ETH') return px >= 200 && px <= 50000;
    return px >= 0.01 && px <= 500000;
  }

  function isPlausibleQty(coin, qty) {
    if (qty <= 0) return false;
    if (coin === 'BTC') return qty >= 0.0001 && qty <= 500;
    if (coin === 'ETH') return qty >= 0.001 && qty <= 50000;
    return qty >= 0.0001 && qty <= 1e7;
  }

  function resolveDomMargin(qty, mark, entry, lev, chunk) {
    const px = mark > 0 ? mark : entry;
    const computed = (qty > 0 && px > 0 && lev > 0) ? (qty * px / lev) : 0;
    let labeled = domMarginUsdt(chunk, mark, entry);
    if (labeled > 0 && px > 0 && labeled < px * 1.15) labeled = 0;
    if (labeled > 0 && computed > 0 && Math.abs(labeled - computed) / computed <= 0.12) {
      return labeled;
    }
    return computed > 0 ? computed : labeled;
  }

  function parseDomPositions() {
    if (!isFollowDetailPage()) return [];
    const text = document.body ? (document.body.innerText || '') : '';
    if (!text || !/当前仓位|Current position/i.test(text)) return [];

    const chunk = domSliceAfterPositionTitle(text);
    if (!/[A-Z0-9]{2,20}USDT/i.test(chunk)) return [];

    const blocks = splitDomPositionBlocks(chunk);
    const rows = [];
    if (blocks.length > 0) {
      for (let i = 0; i < blocks.length; i++) {
        const row = parseOneDomBlock(blocks[i].coin, blocks[i].chunk);
        if (row) rows.push(row);
      }
    } else {
      const coinM = chunk.match(/([A-Z0-9]{2,20})USDT/i);
      const coin = coinM ? coinM[1].toUpperCase() : 'ETH';
      const row = parseOneDomBlock(coin, chunk);
      if (row) rows.push(row);
    }
    return rows;
  }

  function formatDomRowsBrief(rows) {
    if (!rows || !rows.length) return '0 仓';
    return rows.length + ' 仓: ' + rows.map(formatPosBrief).join(' | ');
  }

  function formatPosBrief(row) {
    if (!row) return '';
    return row.symbol + ' ' + row.side + ' qty=' + row.quantity + ' margin=' + row.margin_used + ' entry=' + row.entry_price + ' ' + row.leverage + 'x';
  }

  function applyPositions(rows, sourceKind, via, opts) {
    opts = opts || {};
    sourceKind = sourceKind || 'lead';
    const pri = SRC_PRIORITY[sourceKind] || 50;
    const updates = {};
    const updatesMeta = {};
    for (let i = 0; i < rows.length; i++) {
      const row = rows[i];
      if (!row || !row.symbol) continue;
      const sym = row.symbol;
      const oldPri = (positionMeta[sym] && positionMeta[sym].priority) || 0;
      if (pri < oldPri && positions[sym]) {
        updates[sym] = positions[sym];
        updatesMeta[sym] = positionMeta[sym];
        continue;
      }
      updates[sym] = row;
      updatesMeta[sym] = { priority: pri, via: via, kind: sourceKind };
    }
    if (Object.keys(updates).length === 0 && Object.keys(positions).length > 0 && pri < 80) return false;

    const prevCount = Object.keys(positions).length;
    const updateCount = Object.keys(updates).length;
    // follow-detail DOM 解析到多仓：视为当前页全量可见列表，可删已平品种
    const replaceAll =
      (isLeadManagePage() && sourceKind !== 'dom' && updateCount > 0 && updateCount >= prevCount) ||
      (sourceKind === 'private' && updateCount > 1) ||
      (opts.domFullList && sourceKind === 'dom' && updateCount >= 1);

    const prevSig = JSON.stringify(positions);
    if (replaceAll) {
      positions = updates;
      positionMeta = updatesMeta;
      lastSnapshotComplete = true;
    } else {
      // follow-detail DOM / 单行局部更新：只增改，不删其它 symbol（避免误平 BTC 只留 ETH）
      const merged = Object.assign({}, positions);
      const mergedMeta = Object.assign({}, positionMeta);
      Object.keys(updates).forEach(function (sym) {
        merged[sym] = updates[sym];
        mergedMeta[sym] = updatesMeta[sym];
      });
      positions = merged;
      positionMeta = mergedMeta;
      lastSnapshotComplete = false;
    }
    purgePhantomPositions('apply');
    savePosCache();
    if (JSON.stringify(positions) !== prevSig) markDataChanged();
    schedulePost();
    return true;
  }

  function markDataChanged() {
    lastDataChangeAt = Date.now();
  }

  function reloadOkxPage() {
    log('5分钟无新数据 → 刷新页面');
    location.reload();
  }

  function checkStaleDataReload() {
    if (Date.now() - lastDataChangeAt < STALE_DATA_RELOAD_MS) return;
    markDataChanged();
    reloadOkxPage();
  }

  function startStaleDataReloadWatcher() {
    setInterval(checkStaleDataReload, 30000);
  }

  function processProductsPayload(data) {
    const list = data && data.data;
    if (!Array.isArray(list)) return;
    let n = 0;
    for (let i = 0; i < list.length; i++) {
      const item = list[i];
      const id = item && item.instId;
      const cv = parseFloat((item && item.ctVal) || '0');
      if (id && cv > 0) {
        ctValMap[id] = cv;
        n++;
      }
    }
    if (n > 0) log('products 缓存 ctVal ' + n + ' 个合约');
  }

  function localCacheKey() {
    const id = parseUniqueNameFromPage() || resolvedUniqueCode || 'global';
    return POS_LOCAL_PREFIX + SERVER + '-' + id;
  }

  function mergePosMaps(into, metaInto, from, metaFrom) {
    Object.keys(from || {}).forEach(function (sym) {
      const oldPri = (metaInto[sym] && metaInto[sym].priority) || 0;
      const newPri = (metaFrom[sym] && metaFrom[sym].priority) || 0;
      if (!into[sym] || newPri >= oldPri) {
        into[sym] = from[sym];
        metaInto[sym] = metaFrom[sym];
      }
    });
  }

  function savePosCache() {
    const payload = { positions: positions, ctValMap: ctValMap, positionMeta: positionMeta, savedAt: Date.now() };
    try {
      sessionStorage.setItem(POS_CACHE_KEY, JSON.stringify(payload));
    } catch (e) {}
    try {
      localStorage.setItem(localCacheKey(), JSON.stringify(payload));
    } catch (e) {}
  }

  function loadPosCache() {
    let loaded = 0;
    try {
      const raw = sessionStorage.getItem(POS_CACHE_KEY);
      if (raw) {
        const obj = JSON.parse(raw);
        if (obj.positions) {
          positions = obj.positions;
          loaded = Object.keys(positions).length;
        }
        if (obj.ctValMap) ctValMap = Object.assign({}, ctValMap, obj.ctValMap);
        if (obj.positionMeta) positionMeta = obj.positionMeta;
      }
    } catch (e) {}

    try {
      const rawLocal = localStorage.getItem(localCacheKey());
      if (rawLocal) {
        const obj = JSON.parse(rawLocal);
        if (obj.savedAt && Date.now() - obj.savedAt > POS_LOCAL_TTL_MS) {
          localStorage.removeItem(localCacheKey());
        } else if (obj.positions) {
          const merged = Object.assign({}, obj.positions, positions);
          const mergedMeta = Object.assign({}, obj.positionMeta || {}, positionMeta);
          const localCount = Object.keys(obj.positions).length;
          positions = merged;
          positionMeta = mergedMeta;
          if (obj.ctValMap) ctValMap = Object.assign({}, obj.ctValMap, ctValMap);
          if (localCount > loaded) {
            log('合并 localStorage 缓存，共 ' + Object.keys(positions).length + ' 仓');
            loaded = Object.keys(positions).length;
          }
        }
      }
    } catch (e) {}

    if (loaded > 0) log('恢复缓存 ' + loaded + ' 仓');
    purgePhantomPositions('cache');
  }

  function schedulePost() {
    if (postTimer) return;
    postTimer = setTimeout(function () {
      postTimer = null;
      postToServer(false);
    }, 300);
  }

  function buildRelayHeaders() {
    const headers = { 'Content-Type': 'application/json' };
    if (RELAY_TOKEN) headers['X-OKX-Scraper-Token'] = RELAY_TOKEN;
    headers['X-OKX-Scraper-Route'] = '07';
    return headers;
  }

  function postToServer(heartbeatOnly, retryCount) {
    retryCount = retryCount || 0;
    const now = Date.now();
    if (!heartbeatOnly && now - lastPostAt < POST_MIN_MS) return;
    if (sendInFlight && retryCount === 0) return;

    const list = heartbeatOnly ? [] : Object.keys(positions).map(function (k) { return positions[k]; });
    const body = {
      source: heartbeatOnly ? 'okx-api-intercept-v' + SCRIPT_VER + '-ping' : 'okx-api-intercept-v' + SCRIPT_VER,
      positions: list,
      heartbeat_only: !!heartbeatOnly,
      snapshot_complete: !heartbeatOnly && (lastSnapshotComplete || list.length >= 2),
      ts: now,
      api_hits: apiHits,
      unique_name: parseUniqueNameFromPage() || resolvedUniqueCode || '',
      page_url: location.href,
    };
    const hash = JSON.stringify(body);
    if (hash === lastPostHash && !heartbeatOnly && retryCount === 0) return;

    sendInFlight = true;
    GM_xmlhttpRequest({
      method: 'POST',
      url: SERVER + (SERVER.indexOf('/api/') >= 0 ? '' : '/data'),
      headers: buildRelayHeaders(),
      data: JSON.stringify(body),
      timeout: 15000,
      onload: function (r) {
        sendInFlight = false;
        if (r.status === 200) {
          lastPostHash = hash;
          lastPostAt = now;
          if (!heartbeatOnly) markDataChanged();
          if (heartbeatOnly) log('keepalive ping OK');
          else {
            const brief = list.length ? formatPosBrief(list[0]) : '0 仓';
            log('POST OK | ' + list.length + ' 仓 | ' + brief);
          }
        } else {
          log('POST HTTP ' + r.status + ' ' + String(r.responseText || '').slice(0, 120));
        }
      },
      onerror: function (e) {
        sendInFlight = false;
        const err = (e && e.error) ? String(e.error) : 'network';
        if (retryCount < 4 && /background|shutdown|context|invalidated/i.test(err)) {
          log('POST 重试(' + (retryCount + 1) + '/4): ' + err);
          setTimeout(function () { postToServer(heartbeatOnly, retryCount + 1); }, 1500 * (retryCount + 1));
          return;
        }
        log('POST 失败: ' + err);
      },
      ontimeout: function () {
        sendInFlight = false;
        if (retryCount < 2) {
          setTimeout(function () { postToServer(heartbeatOnly, retryCount + 1); }, 2000);
          return;
        }
        log('POST 超时');
      },
    });
  }

  function ingestSubPosList(list, url, via) {
    if (!list || !list.length) {
      if (isSubPositionsUrl(url)) {
        emptyStreak++;
        log('API 空仓 [' + via + '] streak=' + emptyStreak);
        if (emptyStreak >= 2 && (positionMeta['__empty__'] || 0) < SRC_PRIORITY.dom) {
          positions = {};
          positionMeta = {};
          savePosCache();
          schedulePost();
        }
      }
      return false;
    }

    const kind = urlSourceKind(url);
    if (isFollowDetailPage() && kind === 'public') return false;

    apiHits++;
    const rows = [];
    for (let i = 0; i < list.length; i++) {
      const row = mapSubPosRow(list[i], url);
      if (row) rows.push(row);
    }
    if (!rows.length) return false;

    emptyStreak = 0;
    const ok = applyPositions(rows, kind, via);
    if (ok) {
      log('API#' + apiHits + ' [' + via + '/' + kind + ']: ' + rows.length + ' 仓 | ' + formatPosBrief(rows[0]));
    }
    return ok;
  }

  function tryProcessAnyJson(data, url, via) {
    const urlStr = String(url || '');
    const isPrivateFullList =
      urlSourceKind(urlStr) === 'private' && /current-sub-positions/i.test(urlStr);
    // follow-detail 只信 DOM；但 private 全量持仓 API 仍要（补全多仓，避免单页 DOM 丢 BTC）
    if (isFollowDetailPage() && via !== 'dom' && !isPrivateFullList) return false;
    if (isFollowDetailPage() && /public-current-subpositions/i.test(urlStr)) return false;
    if (!isCopyTradingUrl(url) && !isSubPositionsUrl(url)) return false;

    let list = extractSubPosList(data);
    if (list === null && isOkxHost(url)) list = deepFindSubPosList(data);
    if (list === null) return false;

    const sig = JSON.stringify(list);
    const confirmedEmpty = Array.isArray(list) && list.length === 0 && isSubPositionsUrl(url);
    if (!confirmedEmpty && sig === lastPayloadSig && via !== 'dom') return false;
    lastPayloadSig = sig;

    if (!isSubPositionsUrl(url)) {
      log('从 JSON 识别持仓 [' + via + ']: ' + String(url || '').slice(-100));
    }
    return ingestSubPosList(list, url, via);
  }

  function ingestDomOnce() {
    if (!isFollowDetailPage()) return;
    const rows = parseDomPositions();
    if (!rows.length) {
      if (!domConfirmsNoCurrentPositions()) {
        domEmptyStreak = 0;
        return;
      }
      domEmptyStreak++;
      if (domEmptyStreak < 2) return;
      positions = {};
      positionMeta = {};
      lastSnapshotComplete = true;
      apiHits++;
      savePosCache();
      schedulePost();
      if (domEmptyStreak === 2) log("DOM 连续确认空仓，发送全量空仓快照");
      return;
    }
    domEmptyStreak = 0;
    const sig = JSON.stringify(rows);
    if (sig === lastPayloadSig) return;
    lastPayloadSig = sig;
    apiHits++;
    const domFullList = rows.length >= 2;
    if (applyPositions(rows, 'dom', 'dom', { domFullList: domFullList })) {
      log('DOM#' + apiHits + ': ' + formatDomRowsBrief(rows));
    }
  }

  function onNetworkResponse(urlStr, res, via) {
    maybeLogCopyUrl(urlStr);
    if (isProductsUrl(urlStr) && res.ok) {
      res.clone().json().then(function (j) { processProductsPayload(j); }).catch(function () {});
    }
    if (!res.ok) return;
    res.clone().json().then(function (j) {
      scanUniqueCodeInJson(j);
      if (isProductsUrl(urlStr)) return;
      tryProcessAnyJson(j, urlStr, via);
    }).catch(function () {});
  }

  function requestUrl(input) {
    if (!input) return '';
    if (typeof input === 'string') return input;
    if (input instanceof Request) return input.url || '';
    return String(input);
  }

  function ourFetch(input, init) {
    const urlStr = requestUrl(input);
    return downstreamFetch.call(window, input, init).then(function (res) {
      onNetworkResponse(urlStr, res, 'fetch');
      return res;
    });
  }

  function installFetchTrap() {
    if (fetchTrapOk) return;
    try {
      Object.defineProperty(window, 'fetch', {
        configurable: true,
        enumerable: true,
        get: function () { return ourFetch; },
        set: function (fn) {
          if (typeof fn === 'function') downstreamFetch = fn.bind(window);
        },
      });
      fetchTrapOk = true;
      log('fetch 陷阱 OK');
    } catch (e) {
      log('fetch 陷阱失败: ' + e.message);
    }
  }

  function hookResponseJson() {
    if (Response.prototype.__okx_json_v1__) return;
    const orig = Response.prototype.json;
    Response.prototype.json = function () {
      const res = this;
      return orig.apply(this, arguments).then(function (data) {
        const url = res.url || '';
        maybeLogCopyUrl(url);
        scanUniqueCodeInJson(data);
        if (isProductsUrl(url) && res.ok) processProductsPayload(data);
        else if (res.ok) tryProcessAnyJson(data, url, 'response.json');
        return data;
      });
    };
    Response.prototype.__okx_json_v1__ = true;
  }

  function installXhrHook() {
    if (XMLHttpRequest.prototype.__okx_xhr_v1__) return;
    const origOpen = XMLHttpRequest.prototype.open;
    const origSend = XMLHttpRequest.prototype.send;
    XMLHttpRequest.prototype.open = function (method, url) {
      this._okx_url = url || '';
      return origOpen.apply(this, arguments);
    };
    XMLHttpRequest.prototype.send = function () {
      const xhr = this;
      const url = xhr._okx_url || '';
      xhr.addEventListener('load', function () {
        if (xhr.status < 200 || xhr.status >= 300) return;
        try {
          const data = JSON.parse(xhr.responseText);
          if (isProductsUrl(url)) processProductsPayload(data);
          else tryProcessAnyJson(data, url, 'xhr');
        } catch (e) {}
      });
      return origSend.apply(this, arguments);
    };
    XMLHttpRequest.prototype.__okx_xhr_v1__ = true;
  }

  function parseUniqueNameFromPage() {
    const m = (location.pathname || '').match(/follow-detail\/([A-Za-z0-9]{16,18})/i);
    return m ? m[1].toUpperCase() : '';
  }

  function parseCopyRelIdFromPage() {
    try {
      return new URLSearchParams(location.search || '').get('copyRelId') || '';
    } catch (e) {
      return '';
    }
  }

  function rememberUniqueCode(code) {
    const c = String(code || '').trim().toUpperCase();
    if (/^[A-Z0-9]{16,18}$/.test(c) && c !== resolvedUniqueCode) {
      resolvedUniqueCode = c;
      log('解析到 uniqueCode=' + c);
    }
  }

  function scanUniqueCodeInJson(data, depth) {
    depth = depth || 0;
    if (!data || depth > 5) return;
    if (Array.isArray(data)) {
      for (let i = 0; i < data.length; i++) scanUniqueCodeInJson(data[i], depth + 1);
      return;
    }
    if (typeof data !== 'object') return;
    if (data.uniqueCode) rememberUniqueCode(data.uniqueCode);
    const keys = ['data', 'list', 'portraitList', 'itemList'];
    for (let i = 0; i < keys.length; i++) scanUniqueCodeInJson(data[keys[i]], depth + 1);
  }

  function pollPositionUrls() {
    // follow-detail：只读 DOM + 拦截页面自然请求，不主动打 priapi（该页会 404）
    if (isFollowDetailPage()) return [];
    if (isLeadManagePage()) {
      return [
        '/priapi/v5/ecotrade/current-sub-positions?instType=SWAP',
        '/api/v5/copytrading/current-sub-positions?instType=SWAP',
      ];
    }
    return [];
  }

  function pollPositionsOnce() {
    ingestDomOnce();
    const urls = pollPositionUrls();
    if (!urls.length) return;
    const base = location.origin || 'https://www.okx.com';
    urls.forEach(function (rel) {
      const full = base + rel;
      downstreamFetch(full, { credentials: 'include', cache: 'no-store' }).then(function (res) {
        if (!res.ok) return null;
        return res.json();
      }).then(function (j) {
        if (j) tryProcessAnyJson(j, full, 'poll');
      }).catch(function () {});
    });
  }

  function warnMultiPositionHint() {
    if (!isFollowDetailPage()) return;
    const n = Object.keys(positions).length;
    const instN = Object.keys(ctValMap).length;
    if (instN >= 2 && n < 2) {
      log('多合约提示：products 有 ' + instN + ' 个 SWAP 但只采到 ' + n + ' 仓，请滚动到「当前仓位」确保 ETH/BTC 均可见后刷新');
    }
  }

  function startPositionPoller() {
    if (isFollowDetailPage()) {
      const nm = parseUniqueNameFromPage();
      log('follow-detail：仅 DOM 读持仓（不轮询 priapi，避免 404）');
      if (nm) log('uniqueName=' + nm + '；多仓请开带单管理页: ' + LEAD_MGMT_URL);
    } else if (isLeadManagePage()) {
      log('带单管理页：轮询 current-sub-positions 全量持仓');
    }
    setTimeout(function () {
      pollPositionsOnce();
      warnMultiPositionHint();
    }, 1500);
    setInterval(pollPositionsOnce, POLL_MS);
    if (isFollowDetailPage()) {
      setInterval(function () {
        ingestDomOnce();
        warnMultiPositionHint();
      }, DOM_POLL_MS);
    }
  }

  function ensureLeadTab() {
    const nodes = document.querySelectorAll('div, span, button, a, [role="tab"]');
    for (let i = 0; i < nodes.length; i++) {
      const t = (nodes[i].innerText || nodes[i].textContent || '').trim();
      if (t === '当前带单' || t === '当前持仓' || t === 'Current lead trades') {
        nodes[i].click();
        return;
      }
    }
  }

  function warnPageContext() {
    if (isFollowDetailPage()) {
      log('follow-detail：持仓量/保证金/开仓价读页面 DOM，与 UI 一致');
    } else if (!isLeadManagePage()) {
      log('带单员主控推荐: ' + LEAD_MGMT_URL);
    }
  }

  function boot() {
    loadPosCache();
    installFetchTrap();
    hookResponseJson();
    installXhrHook();
    log('OKX 带单 API 拦截已启动 → ' + SERVER);
    log('保证金算法: qty×标记价÷杠杆；follow-detail 读标签后首值（跳过维持保证金率）');
    warnPageContext();
    postToServer(true);
    setInterval(function () { postToServer(true); }, KEEPALIVE_POST_MS);
    startStaleDataReloadWatcher();
    startPositionPoller();
    setTimeout(ensureLeadTab, 2500);
    setTimeout(ensureLeadTab, 8000);
  }

  boot();
})();
