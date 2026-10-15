import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import MaintenanceView from './MaintenanceView.vue'
import { flush, setupComponent } from '../test/setup-component'
import { request } from '../api/client'
vi.mock('../api/client', () => ({ request: vi.fn(), post: vi.fn() }))
vi.mock('element-plus', () => ({ ElMessage: { success: vi.fn() }, ElMessageBox: {} }))
beforeEach(() => { vi.useFakeTimers(); vi.mocked(request).mockReset() })
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals() })
it('reloads reactive policy snapshots into independent editable values', async () => {
  const snapshot = reactive({ policy: { value: { keepDays: 7, thresholds: { disk: 2 } }, revision: 1 } })
  vi.mocked(request).mockImplementation(async (path) => path === '/ops' ? snapshot : [])
  const view = setupComponent(MaintenanceView)
  await flush()
  snapshot.policy.revision = 2
  snapshot.policy.value.keepDays = 30
  await view.state.reloadPolicy()
  expect(view.state.error).toBe('')
  expect(view.state.policyRevision).toBe(2)
  expect(view.state.policy.keepDays).toBe(30)
  view.state.policy.thresholds.disk = 9
  expect(snapshot.policy.value.thresholds.disk).toBe(2)
  view.unmount()
})
it('displays clone failures instead of leaving a rejected reload handler', async () => {
  vi.mocked(request).mockResolvedValue({ policy: { value: { keepDays: 7 }, revision: 1 } })
  const view = setupComponent(MaintenanceView)
  await flush()
  vi.stubGlobal('structuredClone', () => { throw new Error('clone failed') })
  await expect(view.state.reloadPolicy()).resolves.toBeUndefined()
  expect(view.state.error).toContain('clone failed')
  view.unmount()
})
it('saves and reclones reactive server policy responses', async () => {
  const policy = reactive({ value: { keepDays: 15 }, revision: 2 })
  vi.mocked(request).mockImplementation(async (path) => path === '/ops/policy' ? policy : path === '/ops' ? { policy } : [])
  const view = setupComponent(MaintenanceView)
  await flush()
  await view.state.save()
  expect(view.state.error).toBe('')
  expect(view.state.policyRevision).toBe(2)
  view.state.policy.keepDays = 99
  expect(policy.value.keepDays).toBe(15)
  view.unmount()
})

it('marks deleting backups as in progress and hides destructive actions', async () => {
  vi.mocked(request).mockResolvedValue({ policy: { value: {}, revision: 1 } })
  const view = setupComponent(MaintenanceView)
  await flush()
  expect(view.state.labels.deleting).toBe('删除中')
  expect(view.state.labels.deleted).toBe('已删除')
  expect(view.state.canDeleteBackup('deleting')).toBe(false)
  expect(view.state.canDeleteBackup('deleted')).toBe(false)
  expect(view.state.canDeleteBackup('ready')).toBe(true)
  expect(view.state.canDeleteBackup('failed')).toBe(true)
  expect(view.state.canDeleteBackup('interrupted')).toBe(true)
  expect(view.state.canPinBackup('deleting')).toBe(false)
  expect(view.state.canPinBackup('deleted')).toBe(false)
  expect(view.state.canPinBackup('ready')).toBe(true)
  view.unmount()
})
