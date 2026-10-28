import {test,expect,type Page} from '@playwright/test';
import {mkdirSync} from 'node:fs';
import {resolve} from 'node:path';
import {u4Fixture} from './u4-fixture';
import {fixtureFulfill} from './collection-fixture';
const dir=resolve(process.cwd(),'../scratchpad/t3');
async function fixture(page:Page){
 await u4Fixture(page);
 const item={id:'fixture-a',source:'managed',title:'海边的故事',year:'2026',mediaType:'movie',type:'Movie',overview:'最新刮削简介',rating:6.7,size:7960117968,resolutions:[{width:1920,height:872}],category:'电影 / 华语电影'};
 const target={mediaId:'root-fixture',kind:'movie',season:-1,episode:-1};
 let base:Record<string,unknown>={title:item.title,overview:item.overview,originalTitle:'Seaside',year:2026,communityRating:6.681,genres:['剧情']};let overrides:Record<string,unknown>={},revision=0,conflict=false;
 const record=()=>({target,targetKey:'root-fixture:movie:-1:-1',revision,base,effective:{...base,...overrides},overrides,matched:true,choices:[target],unmatched:[{...target,kind:'episode',season:1,episode:2}]});
 await page.route('**/api/v1/media/library?*',r=>fixtureFulfill(r,{json:{data:{items:[{...item,...record().effective}],total:1,categories:[]}}}));
 await page.route('**/api/v1/media/library/managed/fixture-a',r=>fixtureFulfill(r,{json:{data:{...item,...record().effective}}}));
 await page.route('**/api/v1/media/assets/*/subtitles',r=>fixtureFulfill(r,{json:{data:{assetId:'fixture-a',versions:[{id:'fixture-a',name:'当前物理版本'}],items:[]}}}));
 await page.route('**/api/v1/media/library/managed/*/metadata{,?*}',async r=>{
  if(r.request().method()==='PATCH'){
   const p=r.request().postDataJSON();
   if(conflict){conflict=false;overrides.originalTitle='并发原标题';revision++;return r.fulfill({status:409,json:{error:{code:'metadata_conflict',message:'信息已变化，请重新读取后比较保存'}}});}
   expect(p.revision).toBe(revision);expect(p.targetKey).toBe(record().targetKey);
   for(const k of p.restore)delete overrides[k];overrides={...overrides,...p.set};revision++;
  }
  return fixtureFulfill(r,{json:{data:record()}});
 });
 return {conflict:()=>{conflict=true},state:()=>record()};
}
test('T3 edit field locks, explicit restore, 409 keeps draft, unmatched notice',async({page})=>{
 const f=await fixture(page);await page.goto('/library');await page.getByRole('button',{name:'海边的故事',exact:true}).click();await page.getByRole('button',{name:'编辑信息',exact:true}).click();
 const form=page.locator('.metadata-editor');await expect(form.locator('#metadata-communityRating')).toHaveValue('6.7');await expect(form.getByText('暂无刮削值',{exact:false}).first()).toBeVisible();
 await form.locator('#metadata-title').fill('人工标题');await form.locator('#metadata-overview').fill('人工简介');
 await form.getByRole('button',{name:'保存信息',exact:true}).click();await expect(form.getByRole('status').filter({hasText:'信息已保存'})).toBeVisible();expect(f.state().overrides).toEqual({title:'人工标题',overview:'人工简介'});
 await expect(page.locator('.catalog-title-row').getByRole('heading',{name:'人工标题',exact:true})).toBeVisible();
 const row=form.locator('.metadata-row').filter({has:page.locator('#metadata-title')});await expect(row.getByText('人工锁定',{exact:true})).toBeVisible();
 await row.getByRole('button',{name:'恢复为刮削值',exact:true}).click();await form.getByRole('button',{name:'保存信息',exact:true}).click();expect(f.state().overrides).toEqual({overview:'人工简介'});
 await form.locator('#metadata-overview').fill('保留草稿');f.conflict();await form.getByRole('button',{name:'保存信息',exact:true}).click();await expect(form.getByRole('alert')).toContainText('信息已变化');await expect(form.locator('#metadata-overview')).toHaveValue('保留草稿');await expect(form.getByRole('button',{name:'保存信息',exact:true})).toBeDisabled();
 await form.getByRole('button',{name:'重新读取并核对'}).click();await expect(form.locator('#metadata-overview')).toHaveValue('保留草稿');await expect(form.getByRole('button',{name:'保存信息',exact:true})).toBeEnabled();await expect(form.locator('#metadata-originalTitle')).toHaveValue('并发原标题');
 await form.getByText('未匹配的人工信息（1）',{exact:true}).click();await expect(form.getByRole('button',{name:'第 1 季 · 第 2 集 · 未匹配'})).toBeVisible();
});
test('T3 metadata editor screenshots 1440/390 light/dark aligned without overflow',async({page})=>{
 test.skip(process.env.T3_SCREENSHOTS!=='1'||!!process.env.CI||!!process.env.GITHUB_ACTIONS,'Local explicit screenshot opt-in only');
 await fixture(page);mkdirSync(dir,{recursive:true});
 for(const width of [1440,390])for(const theme of ['light','dark']){
  await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);await page.goto('/library');await page.getByRole('button',{name:'海边的故事',exact:true}).click();
  await expect(page.locator('html')).toHaveAttribute('data-theme',theme);await expect(page.getByRole('button',{name:'编辑信息',exact:true})).toBeVisible();
  await page.screenshot({path:`${dir}/after-local-detail-${width}-${theme}.png`,animations:'disabled'});
  await page.getByRole('button',{name:'编辑信息',exact:true}).click();await expect(page.locator('#metadata-title')).toHaveValue('海边的故事');await expect(page.locator('.metadata-form')).toHaveCount(1);
  const d=page.getByRole('dialog',{name:'海边的故事',exact:true});await expect.poll(()=>d.evaluate(el=>el.getBoundingClientRect().left>=0&&el.getBoundingClientRect().right<=innerWidth)).toBe(true);
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);expect(await d.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true);
  expect(await page.locator('.metadata-row input,.metadata-origin .el-button').evaluateAll(els=>els.every(el=>el.getBoundingClientRect().height>=44))).toBe(true);
  await page.screenshot({path:`${dir}/after-local-editor-${width}-${theme}.png`,animations:'disabled'});
 }
});

test('T3 previous-release screenshot matrix for before/after comparison',async({page})=>{
 test.skip(process.env.T3_SCREENSHOTS!=='1'||!!process.env.CI||!!process.env.GITHUB_ACTIONS,'Local explicit screenshot opt-in only');
 test.skip(!process.env.T3_BEFORE_ASSETS,'only local baseline visual evidence');
 const {readFileSync}=await import('node:fs');const {basename,join,extname}=await import('node:path');const root=process.env.T3_BEFORE_ASSETS!;
 await fixture(page);
 await page.route('**/library{,?*}',r=>new URL(r.request().url()).pathname==='/library'?r.fulfill({contentType:'text/html',body:readFileSync(join(root,'index.html'))}):r.fallback());
 await page.route('**/assets/*',r=>{const name=basename(new URL(r.request().url()).pathname),type=extname(name)==='.js'?'application/javascript':extname(name)==='.css'?'text/css':'application/octet-stream';return r.fulfill({contentType:type,body:readFileSync(join(root,'assets',name))})});
 for(const width of [1440,390])for(const theme of ['light','dark']){
  await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);await page.goto('/library');await page.getByRole('button',{name:'海边的故事',exact:true}).click();const d=page.getByRole('dialog',{name:'海边的故事',exact:true});
  await expect(d.getByText('1080p（1920×872）',{exact:true})).toBeVisible();await expect.poll(()=>d.evaluate(el=>el.getBoundingClientRect().left>=0&&el.getBoundingClientRect().right<=innerWidth)).toBe(true);
  await expect(d.getByRole('button',{name:'编辑信息',exact:true})).toHaveCount(0);expect(await d.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true);
  await page.screenshot({path:`${dir}/before-local-detail-${width}-${theme}.png`,animations:'disabled'});
 }
});
