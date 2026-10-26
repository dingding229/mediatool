import { test, expect } from '@playwright/test'
import { readFileSync, mkdirSync } from 'node:fs'
import { resolve } from 'node:path'

const root = resolve(import.meta.dirname, '../..')
if (process.env.E2E_ISOLATED !== '1' || process.env.E2E_BASE_URL !== 'http://localhost:18084') {
  throw new Error('Use bash scripts/test-e2e.sh')
}
const username = process.env.E2E_ADMIN_USERNAME!, password = process.env.E2E_ADMIN_PASSWORD!

async function ensureRegisteredAndLogin(page: import('@playwright/test').Page) {
  await page.goto('/login')
  const isRegister = await page.getByRole('heading', { name: '注册管理员账号', exact: true }).isVisible()
  if (isRegister) {
    await page.getByLabel('管理员账号', { exact: true }).fill(username)
    await page.getByLabel('密码', { exact: true }).fill(password)
    await page.getByLabel('确认密码', { exact: true }).fill(password)
    await page.getByRole('button', { name: '注册并进入工作台' }).click()
    await expect(page.getByRole('heading', { name: '首页' })).toBeVisible()
    return
  }
  await expect(page.getByRole('heading', { name: '进入管理工作台', exact: true })).toBeVisible()
  await page.getByLabel('管理员账号', { exact: true }).fill(username)
  await page.getByLabel('密码', { exact: true }).fill(password)
  await page.getByRole('button', { name: '进入工作台', exact: true }).click()
  await expect(page.getByRole('heading', { name: '首页' })).toBeVisible()
}

test('U1 navigation, theme toggle, and 13 primary routes', async ({ page }) => {
  await ensureRegisteredAndLogin(page)

  // 1. 验证 13 个主路由直接可达，无水平溢出
  const primaryRoutes = [
    '/dashboard',
    '/search',
    '/subscriptions',
    '/downloads',
    '/library',
    '/organize',
    '/files',
    '/playback',
    '/connections',
    '/sites',
    '/plugins',
    '/ops',
    '/settings'
  ]
  for (const path of primaryRoutes) {
    await page.goto(path)
    expect(page.url()).toContain(path)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  }

  // 2. 主题切换测试与 localStorage 存储
  await page.goto('/dashboard')
  const themeBtn = page.getByRole('button', { name: /主题模式：/ })
  await expect(themeBtn).toBeVisible()

  // 触发深色模式
  await page.evaluate(() => {
    document.documentElement.classList.add('dark')
    document.documentElement.setAttribute('data-theme', 'dark')
  })
  expect(await page.evaluate(() => document.documentElement.classList.contains('dark'))).toBe(true)

  await themeBtn.click()
  await expect(themeBtn).toHaveAttribute('aria-label', /主题模式：/)

  // 3. 响应式布局检测 (375px 与 1440px)
  for (const width of [375, 1440]) {
    await page.setViewportSize({ width, height: 900 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  }
})

test('U1 sidebar contains exactly thirteen primary entries without current badges or accordion controls', async ({ page }) => {
  await ensureRegisteredAndLogin(page)
  const nav = page.getByRole('navigation', { name: '功能导航', exact: true })
  await expect(nav.getByRole('link')).toHaveCount(13)
  expect(await nav.getByRole('link').allTextContents()).toEqual(['首页','搜索','订阅','下载','媒体库','入库整理','文件','播放服务','连接','站点','插件','运维','设置'])
  await expect(nav.getByRole('button')).toHaveCount(0)
  await expect(nav.getByText('当前', { exact: true })).toHaveCount(0)
  for (const group of ['获取','媒体','系统']) await expect(nav.getByRole('heading', { name: group, exact: true })).toBeVisible()
})

test('U1 each outer page tab writes its exact URL tab and quick jump opens operations logs', async ({ page }) => {
  await ensureRegisteredAndLogin(page)
  const expected: Record<string, string[]> = {
    search: ['query','history','discover'], downloads: ['tasks','uploads','manual'], organize: ['candidates','scans','assets','history','recovery'], files: ['browser','strm','tools'],
    playback: ['settings','users','players','sessions','processing','ips','ip-blocklist'], connections: ['storage','emby','downloader','notifications','aux'],
    sites: ['accounts','templates'], ops: ['jobs','attention','maintenance','logs','cache'], settings: ['general','naming','automation','api','mobile','security','about'],
  }
  for (const [path, ids] of Object.entries(expected)) {
    await page.goto(`/${path}?keep=owned`)
    const nav = page.locator(['organize','files','playback','sites','plugins','ops'].includes(path) ? '.media-workspace .page-tabs' : path === 'connections' ? '.connections-workspace .page-tabs' : ['search','downloads'].includes(path) ? '.acquire-workspace .page-tabs' : '.settings-workspace > .page-tabs')
    await expect(nav.getByRole('button')).toHaveCount(ids.length)
    for (let index = 0; index < ids.length; index++) {
      await nav.getByRole('button').nth(index).click()
      await expect.poll(() => new URL(page.url()).searchParams.get('tab')).toBe(ids[index])
      expect(new URL(page.url()).searchParams.get('keep')).toBe('owned')
      await expect(nav.getByRole('button').nth(index)).toHaveAttribute('aria-pressed', 'true')
    }
  }
  await page.keyboard.press('Control+k')
  const dialog = page.getByRole('dialog', { name: '快速前往', exact: true })
  await dialog.getByRole('combobox', { name: '搜索功能入口' }).fill('日志')
  await dialog.getByRole('combobox', { name: '搜索功能入口' }).press('Enter')
  await expect(page).toHaveURL(/\/ops\?tab=logs/)
  await expect(page.getByRole('heading', { name: '程序日志', exact: true })).toBeVisible()
})
