import {fixtureFulfill} from './collection-fixture'
import { test, expect, type Page } from '@playwright/test'
import { mkdirSync } from 'node:fs'
const sample = { id: 'trial-sub', title: '葬送的芙莉莲', media: { id: 209867, mediaType: 'series', title: '葬送的芙莉莲', year: '2023' }, enabled: true, source: 'both', revision: 3,
  rules: { accountIds: ['trial-site'], keyword: 'Frieren', regexKeywords: true, include: ['1080p'], exclude: [], minSize: 2, maxSize: 20, minSeeders: 3, freeOnly: true, rejectHR: true, season: 1, episodes: [], includeHistory: false, resolution: '1080p', source: 'web-dl', codec: 'h265' }, lastState: 'succeeded', lastMessage: '已覆盖的季集不会重复下载', nextRunAt: '2026-10-21T02:30:00Z' }
const matches = [{ id: 42, mediaType: 'movie', title: '星际穿越', year: '2014' }]
async function fixture(page: Page) {
  await page.route('**/api/v1/**', r => fixtureFulfill(r,{ json: { data: [] } }))
  await page.route('**/api/v1/auth/session', r => fixtureFulfill(r,{ json: { data: { authenticated: true, account: { id: 'trial-admin', username: '试做管理员' } } } }))
  await page.route('**/api/v1/system/status', r => fixtureFulfill(r,{ json: { data: { version: '试做环境', dependencies: [] } } }))
  await page.route('**/api/v1/integrations', r => fixtureFulfill(r,{ json: { data: [{ id: 'trial-site', name: '测试站点', kind: 'ptsite', enabled: true, secretFields: [] }] } }))
  await page.route('**/api/v1/subscriptions?*', r => fixtureFulfill(r,{ json: { data: [sample, { ...sample, id: 'trial-paused', title: '星际穿越', media: matches[0], enabled: false, source: 'search', lastState: '', nextRunAt: '' }] } }))
  await page.route('**/api/v1/subscriptions/*/history{,?*}', r => fixtureFulfill(r,{ json: { data: [{ id: 'trial-run', state: 'succeeded', preview: true, createdAt: '2026-10-20T12:00:00Z' }] } }))
  await page.route('**/api/v1/subscription-runs/trial-run', r => fixtureFulfill(r,{ json: { data: { id: 'trial-run', subscriptionId: 'trial-sub', state: 'succeeded', preview: true, decisions: [{ id: 'decision', title: '测试资源 1080p WEB-DL', action: 'would_download', reasons: ['符合订阅规则'], episodes: [1], checks: [{ key: 'resolution', label: '分辨率', state: 'matched', actual: '1080p', expected: '1080p', message: '分辨率符合要求' }] }] } } }))
  await page.route('**/api/v1/subscriptions', r => { const input = r.request().postDataJSON(); return fixtureFulfill(r,{ status: 201, json: { data: { ...sample, ...input, id: 'draft-sub', title: '星际穿越', media: matches[0], revision: 1 } } }) })
  await page.route('**/api/v1/subscriptions/draft-sub', r => fixtureFulfill(r,{ json: { data: { ...sample, id: 'draft-sub', revision: 2 } } }))
  await page.route('**/api/v1/subscriptions/draft-sub/evaluate', r => fixtureFulfill(r,{ status: 202, json: { data: { id: 'draft-run' } } }))
  await page.route('**/api/v1/subscription-runs/draft-run', r => fixtureFulfill(r,{ json: { data: { id: 'draft-run', state: 'succeeded', preview: true, decisions: [] } } }))
  await page.route('**/api/v1/search/media?*', r => fixtureFulfill(r,{ json: { data: { items: matches, cached: false, cacheSeconds: 3600 } } }))
}
const tabs = (page: Page) => page.locator('.mt-subscriptions .page-tabs')
async function chooseMedia(page: Page) {
  const dialog = page.getByRole('dialog', { name: '新建订阅', exact: true })
  await dialog.getByLabel('订阅TMDB关键词', { exact: true }).fill('星际穿越'); await dialog.getByRole('button', { name: '查询TMDB', exact: true }).click()
  await dialog.getByRole('group', { name: '匹配影视' }).getByRole('button').click(); await dialog.getByRole('button', { name: '选择站点与规则' }).click()
  return dialog
}
test.beforeEach(async ({ page }) => { await fixture(page) })
test('four-step composer saves a paused draft, runs real preview, then saves the returned revision', async ({ page }) => {
  const writes: any[] = []; let previews = 0
  await page.route('**/api/v1/subscriptions', r => { const input = r.request().postDataJSON(); writes.push(input); return fixtureFulfill(r,{ status: 201, json: { data: { ...sample, ...input, id: 'draft-sub', title: '星际穿越', media: matches[0], revision: 1 } } }) })
  await page.route('**/api/v1/subscriptions/draft-sub', r => { writes.push(r.request().postDataJSON()); return fixtureFulfill(r,{ json: { data: { ...sample, id: 'draft-sub', revision: 2 } } }) })
  await page.route('**/api/v1/subscriptions/draft-sub/evaluate', r => { previews++; return fixtureFulfill(r,{ status: 202, json: { data: { id: 'draft-run' } } }) })
  await page.goto('/subscriptions'); await page.getByRole('button', { name: '新建订阅', exact: true }).click()
  await expect(page.getByRole('button', { name: '选择站点与规则' })).toBeDisabled(); const dialog = await chooseMedia(page)
  await dialog.getByLabel('订阅包含关键词', { exact: true }).fill('WEB[-.]DL\n1080p'); await expect(dialog.getByRole('checkbox', { name: '拒绝下载H&R资源', exact: true })).toBeChecked()
  await dialog.getByRole('button', { name: '进入预演', exact: true }).click(); await expect(dialog.getByRole('button', { name: '确认并继续', exact: true })).toBeDisabled()
  await dialog.getByRole('button', { name: '保存停用草稿并预演', exact: true }).click(); await expect(dialog.getByRole('button', { name: '确认并继续', exact: true })).toBeEnabled()
  expect(writes[0]).toMatchObject({ tmdbId: 42, enabled: false, rules: { accountIds: ['trial-site'], include: ['WEB[-.]DL','1080p'], freeOnly: true, rejectHR: true } }); expect(previews).toBe(1)
  await dialog.getByRole('button', { name: '确认并继续', exact: true }).click(); await expect(dialog.getByRole('checkbox', { name: '保存后启用自动下载', exact: true })).not.toBeChecked()
  await dialog.getByRole('button', { name: '保存订阅', exact: true }).click(); await expect(dialog).toBeHidden(); expect(writes[1]).toMatchObject({ revision: 1, enabled: false })
})
test('verified metadata prefill opens directly into rules and defaults to paused', async ({ page }) => {
  await page.route('**/api/v1/metadata/details?*', r => fixtureFulfill(r,{ json: { data: { item: matches[0] } } }))
  await page.goto('/subscriptions?create=1&mediaType=movie&tmdbId=42&tab=list')
  const dialog = page.getByRole('dialog', { name: '新建订阅', exact: true })
  await expect(dialog.locator('.composer-target')).toContainText('星际穿越'); await expect(dialog.getByLabel('订阅包含关键词', { exact: true })).toBeVisible()
  await expect(page).not.toHaveURL(/create=1/); await dialog.getByRole('button', { name: '进入预演', exact: true }).click()
  await expect(dialog).toContainText('退出向导后草稿仍保留')
})
test('direct run URL opens inline decision review and a business-only detail dialog', async ({ page }) => {
  await page.goto('/subscriptions?tab=history&subscription=trial-sub&run=trial-run')
  await expect(tabs(page).getByRole('button', { name: '运行记录', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByRole('region', { name: '运行结果', exact: true })).toContainText('分辨率 · 通过')
  await page.getByRole('button', { name: '完整运行详情', exact: true }).click(); const drawer = page.getByRole('dialog', { name: '运行详情', exact: true })
  await expect(drawer.locator('.technical-json')).toHaveCount(0);
  await expect(drawer.getByRole('button',{name:/技术细节与原始记录/})).toHaveCount(0);await expect(drawer).toContainText('候选决策');
  await drawer.getByRole('button', { name: '关闭此对话框', exact: true }).click(); await tabs(page).getByRole('button', { name: '订阅列表', exact: true }).click(); await page.reload(); await expect(page.locator('.subscription-row')).toHaveCount(2)
})
test('202 stays queued, navigates to the accepted run and failed readback never repeats execution', async ({ page }) => {
  let posts = 0, reads = 0, fail = false
  let accept!: () => void; const acceptance = new Promise<void>(resolve => { accept = resolve })
  await page.route('**/api/v1/subscriptions/*/evaluate', async r => { posts++; await acceptance; return fixtureFulfill(r,{ status: 202, json: { data: { id: 'accepted' } } }) })
  await page.route('**/api/v1/subscription-runs/accepted', r => { reads++; return fixtureFulfill(r,fail ? { status: 503, json: { error: { code: 'unavailable', message: '读取暂不可用' } } } : { json: { data: { id: 'accepted', state: 'pending', preview: false, decisions: [] } } }) })
  await page.goto('/subscriptions'); await page.getByRole('button', { name: '立即运行', exact: true }).first().click()
  try { await expect(page.locator('.async-result-banner')).toContainText('尚未确认排队') } finally { accept() }
  await expect(page).toHaveURL(/run=accepted/); await expect(page.locator('.async-result-banner')).toContainText('运行已排队，尚未完成'); await expect(page.locator('.el-message--success')).toHaveCount(0)
  fail = true; await page.locator('.async-result-banner').getByRole('button', { name: '核对结果' }).click(); await expect(page.locator('.async-result-banner')).toContainText('回执尚未确认')
  await tabs(page).getByRole('button', { name: '订阅列表', exact: true }).click(); await expect(page.getByRole('button', { name: '立即运行', exact: true }).first()).toBeDisabled(); expect(posts).toBe(1); expect(reads).toBeGreaterThanOrEqual(2)
})
test('lost response persists across reload and reads history without reposting', async ({ page }) => {
  let posts = 0, histories = 0
  await page.route('**/api/v1/subscriptions/*/evaluate', r => { posts++; return r.abort('failed') })
  await page.route('**/api/v1/subscriptions/*/history{,?*}', r => { histories++; return fixtureFulfill(r,{ json: { data: [] } }) })
  await page.goto('/subscriptions'); await page.getByRole('button', { name: '立即运行', exact: true }).first().click(); await expect(page.locator('.async-result-banner')).toContainText('回执尚未确认'); await expect.poll(() => histories).toBeGreaterThanOrEqual(1)
  await page.reload(); await expect(page.locator('.async-result-banner')).toContainText('回执尚未确认'); await tabs(page).getByRole('button', { name: '订阅列表', exact: true }).click(); await expect(page.getByRole('button', { name: '立即运行', exact: true }).first()).toBeDisabled(); expect(posts).toBe(1); await expect.poll(() => histories).toBeGreaterThanOrEqual(2)
})
test('edit uses a review step and stale revision is not silently overwritten', async ({ page }) => {
  let puts = 0
  await page.route('**/api/v1/subscriptions/trial-sub', r => { puts++; return fixtureFulfill(r,{ status: 400, json: { error: { code: 'invalid_subscription', message: 'subscription changed; reload before saving' } } }) })
  await page.goto('/subscriptions'); await page.getByRole('button', { name: '葬送的芙莉莲', exact: true }).click(); await page.getByRole('button', { name: '编辑规则', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '编辑订阅', exact: true }); await expect(dialog).toContainText('媒体身份已绑定'); await dialog.getByRole('button', { name: '进入预演', exact: true }).click(); await dialog.getByRole('button', { name: '保存停用草稿并预演', exact: true }).click(); await expect(dialog).toContainText('刷新列表后重新编辑'); await expect(dialog.getByRole('button', { name: '保存停用草稿并预演', exact: true })).toBeDisabled(); expect(puts).toBe(1)
})
test('delete is managed from subscription detail and requires a scoped second confirmation', async ({ page }) => {
  let deletes = 0
  await page.route('**/api/v1/subscriptions/trial-sub', r => { deletes++; return fixtureFulfill(r,{ json: { data: { archived: true } } }) })
  await page.goto('/subscriptions'); await page.getByRole('button', { name: '葬送的芙莉莲', exact: true }).click(); await page.getByRole('button', { name: '归档订阅', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '归档订阅', exact: true }); await expect(dialog).toContainText('不删除已有下载、云盘文件或做种源文件'); expect(deletes).toBe(0)
  await dialog.getByRole('button', { name: '取消', exact: true }).click(); await page.getByRole('button', { name: '归档订阅', exact: true }).click(); await dialog.getByRole('button', { name: '确认归档订阅', exact: true }).click(); await expect(dialog).toBeHidden(); expect(deletes).toBe(1)
})
for (const [status, code, state] of [[503, 'unavailable', 'error'], [403, 'forbidden', 'unauthorized'], [501, 'not_supported', 'unsupported']] as const) {
  test(`subscription ${state} is distinct from empty`, async ({ page }) => { await page.route('**/api/v1/subscriptions?*', r => fixtureFulfill(r,{ status, json: { error: { code, message: '测试服务暂不可用' } } })); await page.goto('/subscriptions'); await expect(page.locator(`.state-view.state-${state}`)).toBeVisible(); await expect(page.locator('.state-view.state-empty')).toHaveCount(0) })
}
test('empty state has one creation entry, not a recycled long form', async ({ page }) => { await page.route('**/api/v1/subscriptions?*', r => fixtureFulfill(r,{ json: { data: [] } })); await page.goto('/subscriptions'); await expect(page.locator('.state-view.state-empty')).toContainText('还没有订阅'); await expect(page.getByRole('button', { name: '新建订阅', exact: true })).toHaveCount(1); await page.getByRole('button', { name: '新建订阅', exact: true }).click(); await expect(page.getByRole('navigation', { name: '创建订阅步骤' })).toBeVisible(); await expect(page.getByLabel('订阅包含关键词', { exact: true })).toHaveCount(0) })
test('four review screenshots and all creation steps fit 375px and 1440px in both themes', async ({ page }, info) => {
  const errors: string[] = []; page.on('pageerror', e => errors.push(e.message)); const directory = '/tmp/mt-trial-shots/sol'; mkdirSync(directory, { recursive: true })
  for (const width of [375, 1440]) for (const theme of ['light', 'dark']) {
    await page.setViewportSize({ width, height: width === 375 ? 900 : 1000 }); await page.addInitScript(value => localStorage.setItem('media-tool:theme-mode:v1', value), theme)
    await page.goto('/subscriptions'); await expect(page.locator('.subscription-row')).toHaveCount(2); await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    expect(await page.locator('.mt-subscriptions button:visible').evaluateAll(nodes => nodes.filter(n => n.getBoundingClientRect().height < 44).map(n => n.textContent))).toEqual([])
    await page.screenshot({ path: `${directory}/subscriptions-${width}-${theme}.png`, fullPage: true, animations: 'disabled' })
    await page.getByRole('button', { name: '新建订阅', exact: true }).click(); const dialog = await chooseMedia(page)
    expect(await dialog.evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true); await dialog.getByRole('button', { name: '进入预演', exact: true }).click(); await dialog.getByRole('button', { name: '保存停用草稿并预演', exact: true }).click(); await expect(dialog.getByRole('button', { name: '确认并继续', exact: true })).toBeEnabled(); await dialog.getByRole('button', { name: '确认并继续', exact: true }).click(); expect(await dialog.evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true)
    if (width === 375) { const rect = await dialog.evaluate(el => ({ top: el.getBoundingClientRect().top, bottom: el.getBoundingClientRect().bottom })); expect(rect.top).toBeGreaterThanOrEqual(0); expect(rect.bottom).toBeLessThanOrEqual(900); await expect(dialog.getByRole('navigation', { name: '创建订阅步骤' })).toBeInViewport(); await expect(dialog.getByRole('button', { name: '保存订阅', exact: true })).toBeInViewport() }
    expect(await dialog.locator('button:visible').evaluateAll(nodes => nodes.filter(n => n.getBoundingClientRect().height < 44).map(n => n.className))).toEqual([])
    await page.screenshot({ path: info.outputPath(`composer-${width}-${theme}.png`), fullPage: true, animations: 'disabled' }); await dialog.getByRole('button', { name: '关闭此对话框', exact: true }).click()
    await tabs(page).getByRole('button', { name: '运行记录', exact: true }).click(); await expect(page.getByRole('region', { name: '运行结果', exact: true })).toContainText('分辨率 · 通过'); expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await expect(page.getByText('分辨率 · 通过', { exact: true })).toBeVisible()
    await page.screenshot({ path: info.outputPath(`run-review-${width}-${theme}.png`), fullPage: true, animations: 'disabled' })
  }
  expect(errors).toEqual([])
})
test('desktop and mobile rows keep only run, edit and more, with secondary actions in the dropdown', async ({ page }) => {
  for (const width of [1440, 375]) {
    await page.setViewportSize({ width, height: 900 }); await page.goto('/subscriptions')
    const row = page.locator('.subscription-row').first()
    await expect(row.getByRole('button')).toHaveCount(4)
    expect(await row.getByRole('button').allTextContents()).toEqual(['葬送的芙莉莲','立即运行','编辑','更多'])
    await expect(row.getByRole('button', { name: '立即运行', exact: true })).toHaveClass(/el-button--primary/)
    await row.getByRole('button', { name: '葬送的芙莉莲更多操作', exact: true }).click()
    const menu = page.getByRole('menu').filter({ hasText: '归档' })
    await expect(menu.getByRole('menuitem')).toHaveCount(4)
    expect(await menu.getByRole('menuitem').allTextContents()).toEqual(['运行记录','预演','停用','归档'])
    await menu.getByRole('menuitem', { name: '归档', exact: true }).click()
    const confirm = page.getByRole('dialog', { name: '归档订阅', exact: true })
    await expect(confirm).toContainText('保留规则、运行记录及下载关联'); await confirm.getByRole('button', { name: '取消', exact: true }).click()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  }
})
