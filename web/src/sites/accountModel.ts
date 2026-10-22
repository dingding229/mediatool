import {formatCompactNumber} from '../presentation/numbers';
export const secretLabels: Record<string, string> = { cookie: '登录 Cookie', rssUrl: '订阅地址', passkey: '订阅密钥', apiKey: '接口密钥', requestHeaders: '附加鉴权头' };
export function safeAccount(row: any) {
  const s = row.settings || {};
  return { id: row.id, definitionId: row.definitionId, name: row.name, revision: row.revision, state: row.state, settings: { endpoint: s.endpoint, userAgent: s.userAgent, userId: s.userId, rssState: s.rssState, rssSource: s.rssSource }, secretKeys: Object.keys(secretLabels).filter(k => typeof row.values?.[k] === 'string' && row.values[k].length > 0) };
}
export function accountPayload(form: any, savedRSS: string) {
  return { definitionId: form.definitionId, revision: form.revision, values: { userAgent: form.userAgent, ...(form.cookie ? { cookie: form.cookie } : {}) }, userId: form.userId, rssUrl: form.rediscover ? '' : form.rssUrl || savedRSS, rediscover: form.rediscover };
}
export const numberText = (n: any) => n == null || n === '' ? '—' : typeof n === 'number' ? formatCompactNumber(n) : String(n);
export function addSamples(store: Record<string, any[]>, rows: any[]) {
  for (const row of rows) {
    if (!row.collectedAt || !row.metrics)
      continue;
    const data = store[row.accountId] || (store[row.accountId] = []);
    if (!data.some(s => s.at === row.collectedAt)) {
      data.push({ at: row.collectedAt, uploaded: row.metrics.uploaded, downloaded: row.metrics.downloaded });
      data.sort((a, b) => Date.parse(a.at) - Date.parse(b.at));
      if (data.length > 60)
        data.shift();
    }
  }
}
export function chartPoints(samples: any[], key: string) {
  const valid = samples.map((v, i) => ({ i, value: v[key], time: Date.parse(v.at) })).filter(v => typeof v.value === 'number' && Number.isFinite(v.value) && Number.isFinite(v.time));
  if (valid.length < 2)
    return '';
  const times = valid.map(v => v.time), start = Math.min(...times), duration = Math.max(...times) - start || 1;
  const values = valid.map(v => v.value), lo = Math.min(...values), span = Math.max(...values) - lo || 1;
  return valid.map(v => `${10 + (v.time - start) / duration * 480},${135 - (v.value - lo) / span * 115}`).join(' ');
}
export function publicAccount(row: any) {
  return { id: row.id, kind: row.kind, name: row.name, definitionId: row.settings?.definitionId, revision: row.revision, enabled: row.enabled, settings: row.settings || {}, lastCheckState: row.lastCheckState, lastCheckedAt: row.lastCheckedAt, lastCheckMessage: row.lastCheckMessage, state: row.lastCheckState, secretKeys: (row.secretFields || []).filter((k: string) => k in secretLabels) };
}
