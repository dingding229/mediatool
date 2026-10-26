export interface Connection {
  id: string;
  kind: string;
  name: string;
  enabled: boolean;
  settings: Record<string, any>;
  secretFields: string[];
  revision: number;
  lastCheckState: string;
  lastCheckMessage: string;
  lastCheckedAt?: string;
  readiness?: {
    state: string;
    nextAction: string;
    reason: string;
  };
}
export interface Field {
  key: string;
  label: string;
  secret?: boolean;
  hint?: string;
}
export const kinds: Record<string, {
  name: string;
  fields: Field[];
}> = {
  open115: { name: '115 授权', fields: [{ key: 'appId', label: '自己的115开放平台 AppID', hint: '使用自己的开放平台应用，不借用其他服务凭据。' }, { key: 'cloudMountPrefix', label: 'CloudDrive2中的115挂载前缀' }, { key: 'accessToken', label: 'Access Token（可选手动录入）', secret: true }, { key: 'refreshToken', label: 'Refresh Token（可选手动录入）', secret: true }] },
  clouddrive2: { name: 'CloudDrive2', fields: [{ key: 'endpoint', label: '原生 gRPC 地址' }, { key: 'cloudRoot', label: '允许的网盘根目录' }, { key: 'token', label: 'CloudDrive2 API Token', secret: true }, { key: 'username', label: 'CloudDrive2 用户名（可选账号模式）', secret: true }, { key: 'password', label: 'CloudDrive2 密码', secret: true }, { key: 'caFile', label: '自定义 CA 文件路径（可选）' }] },
  telegram: { name: 'Telegram', fields: [{ key: 'botToken', label: 'Bot Token', secret: true }, { key: 'chatId', label: '接收 Chat ID' }] },
  tmdb: { name: 'TMDB', fields: [{ key: 'token', label: 'TMDB API Token', secret: true }, { key: 'language', label: '查询语言' }] },
  qbittorrent: { name: '下载器', fields: [{ key: 'endpoint', label: 'Web API 地址' }, { key: 'username', label: '用户名', secret: true }, { key: 'password', label: '密码', secret: true }, { key: 'downloadPath', label: '下载器看到的保存目录' }, { key: 'localPath', label: '本程序可读取的对应共享目录' }, { key: 'category', label: '任务分类' }] },
};
export function secretPayload(values: Record<string, string>, clear: string[]) {
  return { ...Object.fromEntries(Object.entries(values).filter(([, value]) => value !== '')), ...Object.fromEntries(clear.map(key => [key, ''])) };
}
export function changedEndpointMissing(form: {
  settings: Record<string, any>;
  secrets: Record<string, string>;
  clear: string[];
}, saved: Connection | null) {
  return saved && saved.settings.endpoint !== form.settings.endpoint ? saved.secretFields.filter(k => !form.secrets[k] && !form.clear.includes(k)) : [];
}
export const isConflict = (e: any) => e?.status === 409 || /changed|revision/.test(e?.code ?? '');
export function safeSettings(value: Record<string, any>) {
  return Object.fromEntries(Object.entries(value ?? {}).map(([key, v]) => {
    if (typeof v === 'string' && /url|endpoint/i.test(key)) {
      try {
        const u = new URL(v);
        if (u.username || u.password) {
          u.username = '';
          u.password = '';
          return [key, u.toString()];
        }
      }
      catch {
      }
    }
    return [key, v];
  }));
}
