import {test,expect} from '@playwright/test';
import {mkdirSync} from 'node:fs';
import {resolve} from 'node:path';
import {u5PlaybackFixture} from './u5-fixture';
const dir=resolve(import.meta.dirname,'../../scratchpad/h1');mkdirSync(dir,{recursive:true});
const baseline=process.env.H1_BASELINE==='1';
const current={version:'0.29.0',commit:'a'.repeat(40),buildTime:'2026-10-24T08:00:00Z',schema:79,minimumSchema:79,pluginPin:'v0.0.0-fixture'};
const latest={...current,version:'0.29.0',tag:'v0.29.0',digest:'sha256:'+'b'.repeat(64),repository:'dingding229/mediatool',platform:'linux/amd64',notes:'# 本次更新\n\n- **关于页**更加简洁\n- 使用 `稳定解析地址`，保留 302 跳转\n\n[查看发布说明](https://github.com/dingding229/mediatool/releases/tag/v0.29.0)\n\n<script>window.h1Injected=true</script>\n<img src="https://evil.invalid/tracker" onerror="window.h1Injected=true">\n![不加载图片](https://evil.invalid/image.png)',publishedAt:'2026-10-24T08:30:00Z',url:'https://github.com/dingding229/mediatool/releases/tag/v0.29.0'};
for(const width of [1440,390])for(const theme of ['light','dark'])test(`H1 about and pagination ${width} ${theme}`,async({page})=>{
 await page.setViewportSize({width,height:950});await u5PlaybackFixture(page);await page.addInitScript(mode=>localStorage.setItem('media-tool:theme-mode:v1',mode),theme);
 await page.route('**/api/v1/system/version',r=>r.fulfill({json:{data:current}}));
 await page.route('**/api/v1/system/updates',r=>r.fulfill({json:{data:{enabled:true,current,hasUpdate:false,intervalHours:12,result:{status:'verified',checkedAt:'2026-10-24T09:00:00Z',latest}}}}));
 await page.route('**/api/v1/system/online-update',r=>r.fulfill({json:{data:{recoveryRequired:[],available:true,reason:'',socketMounted:true,policyVerified:true,job:null}}}));
 await page.route('**/api/v1/downloads?*',r=>r.fulfill({json:{data:{items:[{id:'h1-download',title:'分页验收任务',state:'imported',progress:1}],total:1,page:1,pageSize:25}}}));
 await page.goto('/settings?tab=about');await expect(page.getByRole('region',{name:'关于 Media Tool',exact:true})).toBeVisible();
 await expect(page.locator('.f8-about')).toContainText('0.29.0');
 await page.screenshot({path:resolve(dir,`${baseline?'before':'after'}-about-${width}-${theme}.png`),fullPage:true,animations:'disabled'});
 if(!baseline){
  await expect(page.locator('.f8-about > .settings-card')).toHaveCount(3);
  await expect(page.locator('.f8-about')).not.toContainText(current.commit);
  await expect(page.locator('.f8-about')).not.toContainText(current.pluginPin);
  await expect(page.locator('.f8-about')).not.toContainText('IP 归属地');
  await expect(page.locator('.release-notes h1')).toHaveText('本次更新');await expect(page.locator('.release-notes strong')).toHaveText('关于页');await expect(page.locator('.release-notes code')).toHaveText('稳定解析地址');
  await expect(page.locator('.release-notes img, .release-notes script')).toHaveCount(0);await expect(page.locator('.release-notes a')).toHaveAttribute('rel','noopener noreferrer');await expect(page.locator('.release-notes a')).toHaveAttribute('target','_blank');
  expect(await page.evaluate(()=>Boolean((window as unknown as {h1Injected?:boolean}).h1Injected))).toBe(false);
  if(width===1440){await expect(page.locator('.workbench-version')).toHaveText('v0.29.0');await expect(page.locator('.workbench-version [title]')).toHaveCount(0)}
 }
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
 await page.goto('/downloads?tab=tasks');const pager=page.getByRole('navigation',{name:'下载任务分页',exact:true});await expect(pager).toBeVisible();
 if(!baseline){await expect(pager).toContainText('第 1 页 / 共 1 页');const boxes=await pager.locator('button').evaluateAll(nodes=>nodes.map(node=>{const box=node.getBoundingClientRect();return {height:box.height,top:box.top}}));expect(boxes.every(box=>box.height>=44)).toBe(true);expect(new Set(boxes.map(box=>box.top)).size).toBe(1)}
 await page.screenshot({path:resolve(dir,`${baseline?'before':'after'}-pagination-${width}-${theme}.png`),fullPage:true,animations:'disabled'});
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
});

test('H1 older release notes stay collapsed without changing execution control',async({page})=>{
 await u5PlaybackFixture(page);const older={...latest,version:'0.28.5',tag:'v0.28.5'};
 await page.route('**/api/v1/system/version',r=>r.fulfill({json:{data:current}}));
 await page.route('**/api/v1/system/updates',r=>r.fulfill({json:{data:{enabled:true,current,hasUpdate:false,intervalHours:12,result:{status:'verified',latest:older}}}}));
 await page.route('**/api/v1/system/online-update',r=>{expect(r.request().method()).toBe('GET');return r.fulfill({json:{data:{recoveryRequired:[],available:true,socketMounted:true,policyVerified:true,job:null}}})});
 await page.goto('/settings?tab=about');await expect(page.locator('.release-notes summary')).toContainText('v0.28.5');await expect(page.locator('.release-notes-body')).not.toBeVisible();await expect(page.getByRole('button',{name:'一键更新',exact:true})).toBeDisabled();await expect(page.getByRole('button',{name:'更新详情',exact:true})).toBeEnabled();await expect(page.getByRole('button',{name:/开启执行|关闭执行/})).toHaveCount(0);
});
