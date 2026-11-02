import {test,expect} from '@playwright/test';
import {i42cFixture} from './i42c-fixture';
import {waitForLayout} from './layout-stability';
for(const width of [1440,375])test(`I43 about aligned controls and conditional idle actions ${width}`,async({page})=>{
 await page.setViewportSize({width,height:1000});await i42cFixture(page);await page.goto('/settings?tab=about');
 await expect(page.locator('.f8-auto .el-switch')).toBeVisible();await waitForLayout(page);
 const toggle=await page.locator('.f8-auto .el-switch').boundingBox(),interval=await page.getByRole('textbox',{name:'检查间隔（小时）',exact:true}).boundingBox();
 if(width===1440)expect(Math.abs(toggle!.y-interval!.y)).toBeLessThanOrEqual(1);
 const actions=page.locator('.f8-update-actions');await expect(actions.getByRole('button',{name:'检查更新',exact:true})).toBeVisible();await expect(actions.getByRole('button',{name:'更新详情',exact:true})).toBeVisible();
 expect((await page.locator('.f8-update').innerText()).match(/已是最新版本/g)).toHaveLength(1);
 await expect(page.getByRole('button',{name:'保存闲时设置',exact:true})).toHaveCount(0);await expect(page.getByRole('button',{name:'检查执行条件',exact:true})).toHaveCount(0);
 const idle=page.locator('.idle-toggle .el-switch');await idle.click();await expect(page.getByRole('button',{name:'保存闲时设置',exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'检查执行条件',exact:true})).toBeVisible();await idle.click();await expect(page.getByRole('button',{name:'保存闲时设置',exact:true})).toHaveCount(0);
 expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
});
