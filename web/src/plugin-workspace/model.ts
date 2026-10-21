import { ref } from 'vue';
import { APIError, responseStatus } from '../api/client';
import { operate, inspect, type PluginOperation, type PluginInstance } from '../api/plugins';
import { installIntent, completeInstallIntent } from '../plugins/install-intent';
import { operationError } from '../plugins/operation-error';
import { readPrivate, savePrivate } from '../organize/model';
import { requestUUID } from '../utils/request-uuid';
export const pluginState = (i: PluginInstance) => i.availability && i.availability !== 'available' ? '不可用' : ({ running: '运行中', stopped: '已停止', starting: '启动中', stopping: '停止中', backoff: '等待恢复', needs_action: '待处理' } as Record<string, string>)[i.state] || '状态待核对';
export const permissionName = (scope: string) => ({ 'downloads.add': '添加下载', 'sites.request': '访问站点', 'network.request': '发起网络请求', 'settings.write': '修改设置', 'ui.trusted': '受信界面', 'system.exec': '执行系统命令', 'auth.provider': '账号认证', 'agent.tools': '调用工具' } as Record<string, string>)[scope] || scope;
export const reasonName = (reason: string) => ({ permission_review_required: '权限已变化，插件已停用，请重新确认。', definition_missing: '当前内置目录已不提供此插件；配置和数据仍然保留。', definition_unsupported: '此插件当前不可用；配置和数据仍然保留。', worker_failed: '插件运行异常，请查看实例日志。', resource_limit: '插件达到资源限制，正在等待恢复。', data_unavailable: '插件数据暂不可用，请查看实例日志。' } as Record<string, string>)[reason] || '当前状态需要核对，请查看实例详情。';
export function usePluginOperation() {
  const storage = 'media-tool:u6:plugin-operation', pending = ref<any>(readPrivate(storage)), busy = ref(false), phase = ref<'queued' | 'reconciling' | null>(pending.value ? 'reconciling' : null), error = ref(''), notice = ref(''), technical = ref<any>(null), canReconcile = ref(false);
  let original: PluginOperation | undefined;
  function clear() {
    if (readPrivate(storage)?.key === pending.value?.key)
      savePrivate(storage, null);
    pending.value = null;
    phase.value = null;
    canReconcile.value = false;
    original = undefined;
  }
  function done(out?: PluginInstance) {
    const p = pending.value;
    if (p?.input.kind === 'install')
      completeInstallIntent(p.input.pluginId, p.input.fingerprint);
    notice.value = p?.input.kind === 'install' ? '已安装，默认停用；请核对权限和配置后再启用。' : p?.input.kind === 'review' ? '权限已确认，插件仍保持停用。' : '已读取操作回执，请以当前实例状态为准。';
    clear();
    if (typeof window !== 'undefined')
      window.dispatchEvent(new Event('plugins:changed'));
    return out;
  }
  async function reconcile(read: () => Promise<unknown>) {
    canReconcile.value = false;
    try {
      await read();
      if (!pending.value) {
        error.value = '';
        return;
      }
      const p = pending.value;
      if (p.receipt || p.input.instanceId) {
        try {
          const current = await inspect(p.receipt?.id || p.input.instanceId);
          if (p.input.kind === 'remove' ? current.instance.desiredState === 'absent' : current.instance.revision > p.input.expectedRevision && p.input.kind !== 'clone')
            return done(current.instance);
        }
        catch (e) {
          if (e instanceof APIError && e.status === 404 && p.input.kind === 'remove')
            return done();
          throw e;
        }
      }
      canReconcile.value = true;
    }
    catch {
      error.value = pending.value ? '核对未完成，请先重新读取；不会重复提交。' : notice.value ? '列表同步失败，操作回执仍保留；请重新读取。' : '插件列表读取失败，请重新读取。';
    }
  }
  async function send() {
    if (!pending.value || busy.value)
      return;
    busy.value = true;
    error.value = '';
    canReconcile.value = false;
    const ticket = pending.value;
    try {
      const out = await operate(original || { ...ticket.input, operationKey: ticket.key, confirm: true, ...(ticket.input.kind === 'install' ? { config: {} } : {}) });
      if (!out || typeof out.id !== 'string' || !out.id || out.id.length > 64 || !Number.isSafeInteger(out.revision) || out.revision < 1 || (ticket.input.kind === 'install' && out.pluginId !== ticket.input.pluginId) || (ticket.input.instanceId && ticket.input.kind !== 'clone' && out.id !== ticket.input.instanceId))
        throw new APIError(200, 'invalid_response', 'invalid_operation_receipt');
      if (responseStatus(out) === 202) {
        pending.value = { ...ticket, receipt: { id: out.id, revision: out.revision } };
        savePrivate(storage, pending.value);
        phase.value = 'queued';
      }
      else
        return done(out);
    }
    catch (e) {
      technical.value = operationError(e);
      if (e instanceof APIError && e.status === 499)
        technical.value.uncertain = true;
      error.value = technical.value.uncertain ? '上次操作结果未确认，请先核对，不会重复创建或卸载。' : '请求被拒绝，请重新读取当前定义与修订后再确认。';
      if (!technical.value.uncertain) {
        if (ticket.input.kind === 'install')
          completeInstallIntent(ticket.input.pluginId, ticket.input.fingerprint);
        clear();
      }
    }
    finally {
      busy.value = false;
    }
  }
  async function run(input: Omit<PluginOperation, 'operationKey' | 'confirm'>) {
    if (pending.value || busy.value)
      return;
    let key: string;
    try {
      JSON.stringify(input);
      key = input.kind === 'install' ? installIntent(input.pluginId!, input.fingerprint, requestUUID) : requestUUID();
    }
    catch {
      error.value = '请求尚未发出，无法准备此次操作，请稍后重新确认。';
      return;
    }
    technical.value = null;
    const { config, ...safe } = input;
    pending.value = { input: safe, key };
    savePrivate(storage, pending.value);
    original = { ...input, operationKey: key, confirm: true };
    phase.value = 'reconciling';
    notice.value = '';
    return send();
  }
  async function replay(read: () => Promise<unknown>) {
    await reconcile(read);
    if (pending.value && canReconcile.value && pending.value.input.kind !== 'configure')
      return send();
  }
  return { pending, busy, phase, error, notice, technical, canReconcile, run, reconcile, replay };
}
export function safePluginError(e: unknown) {
  if (!e)
    return null;
  return new APIError(e instanceof APIError ? e.status : 503, 'plugin_read_failed', '插件信息暂时无法读取，请重新读取核对。');
}
