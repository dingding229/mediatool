import {test,expect} from '@playwright/test';
import {mkdirSync} from 'node:fs';
import {resolve} from 'node:path';
import {u3Fixture,u3Run,u3Resource} from './u3-fixture';
const capture=process.env.I29_SCREENSHOTS==='1'&&!process.env.CI&&!process.env.GITHUB_ACTIONS;
const dir=resolve(import.meta.dirname,'../../scratchpad/i-phase/i29/screens');
const media={id:300146,mediaType:'series',title:'嫁金钗',originalTitle:'嫁金钗',year:'2026'};
test('I29 excluded results explain zero matches and browse-all keeps delivery identity',async({page})=>{
 await u3Fixture(page);await page.route('**/api/v1/metadata/details?*',r=>r.fulfill({json:{data:{item:media}}}));let bypass=false;let posted:any;
 await page.route('**/api/v1/search/run-u3?*',r=>{bypass=new URL(r.request().url()).searchParams.get('ignoreMediaIdentity')==='true';return r.fulfill({json:{data:{...u3Run,media,keyword:media.title,sites:[{accountId:'site-u3',name:'HDSKY',state:'succeeded',count:407,message:''}],excluded:407,total:bypass?407:0,resources:bypass?[{...u3Resource,title:'Unrelated.Show.S01.2026.1080p'}]:[]}}})});
 await page.route('**/api/v1/downloads',r=>{posted=r.request().postDataJSON();return r.fulfill({status:400,json:{error:{code:'invalid_download',message:'资源发行标题与所选媒体身份不匹配'}}})});
 await page.goto('/search/results/run-u3');await expect(page.getByText('另有 407 条因与所选媒体身份不匹配被排除',{exact:true})).toBeVisible();await expect(page.getByText('全部被身份过滤',{exact:true})).toBeVisible();
 if(capture){mkdirSync(dir,{recursive:true});await page.screenshot({path:resolve(dir,'excluded-1440-light.png'),fullPage:true})}
 await page.getByRole('button',{name:'显示全部（不按媒体身份过滤）',exact:true}).click();await expect(page.locator('.resource-entry')).toHaveCount(1);expect(bypass).toBe(true);await expect(page.getByText(/投递仍须通过身份校验/)).toBeVisible();
 if(capture){await page.screenshot({path:resolve(dir,'all-1440-light.png'),fullPage:true});await page.setViewportSize({width:390,height:1000});await page.screenshot({path:resolve(dir,'all-390-light.png'),fullPage:true})}
 await page.getByRole('button',{name:'确认并下载',exact:true}).click();const dialog=page.getByRole('dialog');await expect(dialog).toContainText('嫁金钗');
 await dialog.getByRole('button',{name:'创建下载任务',exact:true}).click();await expect.poll(()=>posted?.tmdbId).toBe(300146);expect(posted.searchRunId).toBe('run-u3');expect(posted.correctMediaIdentity).toBe(false);expect(posted).not.toHaveProperty('ignoreMediaIdentity');await expect(dialog).toContainText('资源发行标题与所选媒体身份不匹配');
});
test('I29 completed sites with no resources show site-empty state',async({page})=>{
 await u3Fixture(page);await page.route('**/api/v1/search/run-u3?*',r=>r.fulfill({json:{data:{...u3Run,total:0,excluded:0,resources:[],sites:u3Run.sites.map(s=>({...s,count:0}))}}}));
 await page.goto('/search/results/run-u3');await expect(page.getByText('站点无结果',{exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'显示全部（不按媒体身份过滤）',exact:true})).toHaveCount(0);
});
