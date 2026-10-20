export interface Intent {
  phase: 'queued' | 'reconciling';
  id?: string;
}
export function readIntent(key: string): Intent | null {
  try {
    const v = JSON.parse(sessionStorage.getItem('media-tool:intent:' + key) || 'null');
    return ['queued', 'reconciling'].includes(v?.phase) ? v : null;
  }
  catch {
    return null;
  }
}
export function writeIntent(key: string, value: Intent | null) {
  try {
    if (value)
      sessionStorage.setItem('media-tool:intent:' + key, JSON.stringify(value));
    else
      sessionStorage.removeItem('media-tool:intent:' + key);
  }
  catch {
  }
}
