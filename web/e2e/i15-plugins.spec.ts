import {test,expect,type Page} from '@playwright/test';
import {u6PluginsFixture,plugin} from './u6-fixture';
import {capture,screens,prepareScreen,shot} from './i15-fixture';

async function fixture(page:Page){
 await u6PluginsFixture(page);let saved:Record<string,unknown>|undefined;const writes:any[]=[];
 await page.route('**/api/v1/plugins/'+plugin.id,r=>r.fulfill({json:{data:{instance:{...plugin,pluginId:'library-refresh-notify',metadata:{...plugin.metadata,name:'入库刷新通知'},revision:saved?3:2},config:{value:saved||{active:false,provider:'builtin',intervalSeconds:60,retainedKey:'kept'},revision:saved?3:2},fingerprint:'i15-local-form'}}}));
 await page.route('**/api/v1/plugins/operations',r=>{const body=r.request().postDataJSON();writes.push(body);saved=body.config;return r.fulfill({json:{data:{...plugin,revision:3}}})});
 return writes;
}
async function geometry(page:Page,width:number){
 expect((await page.locator('.plugin-status').boundingBox())!.height).toBeLessThan(240);
 const config=page.getByRole('region',{name:'插件配置',exact:true}),grid=config.locator('.plugin-json');
 const columns=await grid.evaluate(el=>getComputedStyle(el).gridTemplateColumns.split(' ').length);
 expect(columns).toBe(width===1440?2:1);
 const bounds=await config.boundingBox(),save=await config.getByRole('button',{name:'保存插件配置',exact:true}).boundingBox(),form=await grid.boundingBox();
 expect(Math.abs(save!.x-form!.x)).toBeLessThan(1);expect(save!.y).toBeGreaterThanOrEqual(form!.y+form!.height);
 expect(bounds!.y+bounds!.height-(save!.y+save!.height)).toBeLessThanOrEqual(width===1440?25:17);
 expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
}
test('I15 host configuration is two columns / one column with a trailing left save, unchanged payload and confirmation',async({page})=>{
 const writes=await fixture(page);await page.goto('/plugins/'+plugin.id);
 await expect(page.getByRole('spinbutton',{name:'刷新间隔（秒）',exact:true})).toHaveValue('60');
 for(const width of [1440,390]){await page.setViewportSize({width,height:1000});await geometry(page,width)}
 await page.getByRole('spinbutton',{name:'刷新间隔（秒）',exact:true}).fill('90');
 await page.getByRole('spinbutton',{name:'刷新间隔（秒）',exact:true}).blur();
 await page.getByRole('button',{name:'保存插件配置',exact:true}).click();expect(writes).toHaveLength(0);
 await expect(page.getByRole('dialog',{name:'确认插件操作',exact:true})).toBeVisible();
 await page.getByRole('button',{name:'确认操作',exact:true}).click();await expect.poll(()=>writes.length).toBe(1);
 expect(writes[0]).toMatchObject({kind:'configure',expectedRevision:2,config:{active:false,provider:'builtin',intervalSeconds:90,retainedKey:'kept'}});
 await expect(page.getByRole('switch',{name:'启用插件',exact:true})).not.toBeChecked();
});
test.describe('I15 plugins local screenshots @screenshots',()=>{
 test.skip(!capture,'Explicit local I15_SCREENSHOTS=1 only, never CI');
 for(const {width,theme} of screens)test(`plugins-${width}-${theme}`,async({page})=>{
  await fixture(page);await prepareScreen(page,width,theme,'/plugins/'+plugin.id);
  await expect(page.getByRole('spinbutton',{name:'刷新间隔（秒）',exact:true})).toHaveValue('60');
  await shot(page,'plugins',width,theme);
 });
});
