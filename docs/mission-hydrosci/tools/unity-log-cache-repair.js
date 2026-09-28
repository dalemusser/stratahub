// Reference implementation, verified 2026-09-28 on v2.8.1 (see mhs-game-logging-silent-failure-plan.md §0).
// Removes empty {} entries from the game's queued-log cache so the old build's sender resumes.
// Replace __HASH__ with the MD5 of the play page URL up to its last '/' (Unity's persistentDataPath).
// Run in the page before the Unity loader starts; it never creates the database and aborts on any
// unexpected layout. Not wired into any page yet.
(async function () {
  // Remove empty entries ({}) from the game's queued-log cache in Unity's
  // PlayerPrefs file. Everything else in the file is copied byte for byte.
  // Never creates the database; any unexpected layout aborts without writing.
  var KEY = '/idbfs/__HASH__/PlayerPrefs';
  if (!window.indexedDB || typeof indexedDB.databases !== 'function') return 'no-databases-api';
  var list = await indexedDB.databases();
  if (!list.some(function (d) { return d && d.name === '/idbfs'; })) return 'no-db';
  var db = await new Promise(function (res, rej) { var r = indexedDB.open('/idbfs'); r.onsuccess = function () { res(r.result); }; r.onerror = function () { rej(r.error); }; });
  try {
    if (!db.objectStoreNames.contains('FILE_DATA')) return 'no-store';
    var rec = await new Promise(function (res, rej) { var g = db.transaction('FILE_DATA', 'readonly').objectStore('FILE_DATA').get(KEY); g.onsuccess = function () { res(g.result); }; g.onerror = function () { rej(g.error); }; });
    if (!rec || !rec.contents) return 'no-file';
    var u8 = rec.contents instanceof Uint8Array ? rec.contents : new Uint8Array(rec.contents);
    if (String.fromCharCode.apply(null, u8.subarray(0, 8)) !== 'UnityPrf') return 'bad-magic';
    function readLen(p) { var b = u8[p]; if (b < 0x80) return [b, p + 1]; if (b === 0x80) return [(u8[p+1] | (u8[p+2] << 8) | (u8[p+3] << 16) | (u8[p+4] << 24)) >>> 0, p + 5]; return null; }
    var dec = new TextDecoder('utf-8', { fatal: true }), enc = new TextEncoder();
    var i = 16, parts = [u8.subarray(0, 16)], removed = 0, total = 0;
    while (i < u8.length) {
      var kl = readLen(i); if (!kl) return 'bad-key-at-' + i;
      var kEnd = kl[1] + kl[0]; var key = dec.decode(u8.subarray(kl[1], kEnd));
      var t = u8[kEnd], vStart, vEnd;
      if (t === 0xfd || t === 0xfe) { vStart = kEnd + 1; vEnd = vStart + 4; }
      else { var vl = readLen(kEnd); if (!vl) return 'bad-type-at-' + kEnd; vStart = vl[1]; vEnd = vStart + vl[0]; }
      if (vEnd > u8.length) return 'overrun-at-' + i;
      if (key === 'game_logs_cache.json' && t !== 0xfd && t !== 0xfe) {
        var doc = JSON.parse(dec.decode(u8.subarray(vStart, vEnd)));
        if (doc && Array.isArray(doc.logs)) {
          total = doc.logs.length;
          var kept = doc.logs.filter(function (s) { try { var o = JSON.parse(s); return !!o && typeof o === 'object' && Object.keys(o).length > 0; } catch (e) { return false; } });
          removed = total - kept.length;
          if (removed > 0) {
            var vb = enc.encode(JSON.stringify({ logs: kept })), n = vb.length;
            parts.push(u8.subarray(i, kEnd));
            parts.push(n < 0x80 ? new Uint8Array([n]) : new Uint8Array([0x80, n & 255, (n >>> 8) & 255, (n >>> 16) & 255, (n >>> 24) & 255]));
            parts.push(vb); i = vEnd; continue;
          }
        }
      }
      parts.push(u8.subarray(i, vEnd)); i = vEnd;
    }
    if (removed === 0) return 'nothing-to-repair (' + total + ' queued)';
    var size = parts.reduce(function (a, p) { return a + p.length; }, 0), out = new Uint8Array(size), o = 0;
    parts.forEach(function (p) { out.set(p, o); o += p.length; });
    await new Promise(function (res, rej) { var tx = db.transaction('FILE_DATA', 'readwrite'); tx.objectStore('FILE_DATA').put({ timestamp: new Date(), mode: rec.mode, contents: out }, KEY); tx.oncomplete = res; tx.onerror = function () { rej(tx.error); }; });
    return 'repaired: removed ' + removed + ' of ' + total + ' queued entries; ' + u8.length + ' -> ' + size + ' bytes';
  } finally { db.close(); }
})()
