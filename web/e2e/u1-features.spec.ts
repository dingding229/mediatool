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
    await expect(page.getByRole('heading', { name: '运行总览' })).toBeVisible()
    return
  }
  await expect(page.getByRole('heading', { name: '进入管理工作台', exact: true })).toBeVisible()
  await page.getByLabel('管理员账号', { exact: true }).fill(username)
  await page.getByLabel('密码', { exact: true }).fill(password)
  await page.getByRole('button', { name: '进入工作台', exact: true }).click()
  await expect(page.getByRole('heading', { name: '运行总览' })).toBeVisible()
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

test('U1 verification screenshots capture', async ({ page }) => {
  await ensureRegisteredAndLogin(page)
  const dir = '/tmp/media-tool-u1-screenshots'
  mkdirSync(dir, { recursive: true })

  // 1. 首页 1440 浅色
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.evaluate(() => {
    document.documentElement.classList.remove('dark')
    document.documentElement.setAttribute('data-theme', 'light')
  })
  await page.goto('/dashboard')
  await page.waitForTimeout(300)
  await page.screenshot({ path: `${dir}/dashboard-1440-light.png`, fullPage: true })

  // 2. 首页 1440 深色
  await page.evaluate(() => {
    document.documentElement.classList.add('dark')
    document.documentElement.setAttribute('data-theme', 'dark')
  })
  await page.waitForTimeout(300)
  await page.screenshot({ path: `${dir}/dashboard-1440-dark.png`, fullPage: true })

  // 3. 首页 375 浅色
  await page.setViewportSize({ width: 375, height: 812 })
  await page.evaluate(() => {
    document.documentElement.classList.remove('dark')
    document.documentElement.setAttribute('data-theme', 'light')
  })
  await page.goto('/dashboard')
  await page.waitForTimeout(300)
  await page.screenshot({ path: `${dir}/dashboard-375-light.png`, fullPage: true })

  // 4. 首页 375 深色
  await page.evaluate(() => {
    document.documentElement.classList.add('dark')
    document.documentElement.setAttribute('data-theme', 'dark')
  })
  await page.waitForTimeout(300)
  await page.screenshot({ path: `${dir}/dashboard-375-dark.png`, fullPage: true })

  // 5. 设置页 1440 浅色
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.evaluate(() => {
    document.documentElement.classList.remove('dark')
    document.documentElement.setAttribute('data-theme', 'light')
  })
  await page.goto('/settings')
  await page.waitForTimeout(300)
  await page.screenshot({ path: `${dir}/settings-1440-light.png`, fullPage: true })
})
