import {beijingTime} from '../presentation/time';
import { APIError } from '../api/client';
export interface Probe {
  state: string;
  reason: string;
  nextAction: string;
}
export interface Readiness {
  components: Record<string, Probe>;
  scope?: string;
  delivery?: string;
}
export const services = [{ key: 'open115', name: '115 官方授权', tab: 'storage' }, { key: 'clouddrive2', name: 'CloudDrive2', tab: 'storage' }, { key: 'strm', name: 'STRM 播放入口', tab: 'storage' }];
const labels: Record<string, string> = { not_configured: '未配置', configured: '已配置', authorization_required: '需要授权', disabled: '未启用', check_required: '需要检测', check_failed: '检测异常', ready: '检测有效' };
const reasons: Record<string, string> = { connection_missing: '尚未添加连接', official_grant_missing: '请完成自己的 115 应用授权', administrator_enable_required: '配置已保留，等待管理员启用', probe_missing_or_stale: '连接已配置，需要检测或更新过期的检测记录', last_probe_failed: '上次连接检测失败，请检查连接', fresh_saved_probe: '最近检测有效', resolver_configuration_invalid: '请检查固定解析地址和允许的目录', stable_resolver_configured_not_player_verified: '入口已配置，仍需在播放器中试播确认。' };
export function readinessLabel(probe?: Probe) {
  return probe?.state === 'configured' && probe.nextAction === 'verify_player_302' ? '已配置，待播放器验证' : probe ? labels[probe.state] || '状态待核对' : '—';
}
export const readinessReason = (p?: Probe) => p ? reasons[p.reason] || '当前状态需进一步核对，请前往对应连接查看详情' : '尚未读取服务状态';
export function stateFor(e: unknown) {
  return e instanceof APIError && [401, 403].includes(e.status) ? 'unauthorized' : e instanceof APIError && e.status === 501 ? 'unsupported' : 'error';
}
export const errorText = (e: unknown) => e instanceof Error ? e.message : '读取失败，请稍后重新读取';
export const date=beijingTime;
export { formatBytes as bytes } from '../utils/format-bytes';
export const guidePreference = 'media-tool:setup-guide-hidden:v1';
export function readGuideHidden() {
  try {
    return localStorage.getItem(guidePreference) === '1';
  }
  catch {
    return false;
  }
}
export function writeGuideHidden(hidden: boolean) {
  try {
    localStorage.setItem(guidePreference, hidden ? '1' : '0');
  }
  catch {
  }
}
