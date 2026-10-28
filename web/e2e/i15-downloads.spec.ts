import {test,expect} from '@playwright/test';
import {u3DownloadsFixture} from './u3-fixture';
import {capture,screens,prepareScreen,shot} from './i15-fixture';

test.beforeEach(async({page})=>u3DownloadsFixture(page));
test('I15 downloader management has a single retained entry in connections',async({page})=>{
 await page.goto('/downloads');await expect(page.locator('.t8-download-row')).toHaveCount(1);
 await expect(page.getByRole('link',{name:'管理下载器',exact:true})).toHaveCount(0);
 await page.goto('/connections?tab=downloader');
 await expect(page.getByRole('heading',{name:'下载器配置',exact:true})).toBeVisible();
 await expect(page.getByRole('button',{name:'添加连接',exact:true})).toBeVisible();
});
test.describe('I15 downloads local screenshots @screenshots',()=>{
 test.skip(!capture,'Explicit local I15_SCREENSHOTS=1 only, never CI');
 for(const {width,theme} of screens)test(`downloads-${width}-${theme}`,async({page})=>{
  await prepareScreen(page,width,theme,'/downloads');await expect(page.locator('.t8-download-row')).toHaveCount(1);
  await shot(page,'downloads',width,theme);
 });
});
