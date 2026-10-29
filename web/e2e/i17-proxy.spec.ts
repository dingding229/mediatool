import {test,expect,type Page} from '@playwright/test';
import {mkdirSync,readFileSync} from 'node:fs';
import {resolve,join,basename,extname} from 'node:path';
import {u3Fixture} from './u3-fixture';
const dir=resolve(import.meta.dirname,'../../scratchpad/i-phase/i17/screens');
const capture=process.env.I17_SCREENSHOTS==='1'&&!process.env.CI&&!process.env.GITHUB_ACTIONS,before=process.env.I17_BEFORE==='1';
const initial={protocol:'http',host:'proxy.fixture',port:7890,username:'fixture-user',configured:true,hasPassword:true,needsInput:false,tmdb:true,sites:false,revision:2};
async function fixture(page:Page,legacy=false){
 await u3Fixture(page);let stored={...initial,...legacy?{host:'',port:0,username:'',needsInput:true,hasPassword:false}:{}};const writes:any[]=[];
 await page.route('**/api/v1/settings/network-proxy',r=>{
  if(r.request().method()==='PUT'){
   const input=r.request().postDataJSON();writes.push(input);
   stored=input.clear?{...initial,protocol:'',host:'',port:0,username:'',configured:false,hasPassword:false,needsInput:false,tmdb:false,sites:false,revision:stored.revision+1}: {...stored,protocol:input.protocol,host:input.host,port:input.port,username:input.username,tmdb:input.tmdb,sites:input.sites,hasPassword:input.clearPassword?false:!!input.password||stored.hasPassword,needsInput:false,revision:stored.revision+1};
  }
  return r.fulfill({json:{data:before?{url:'http://fixture-user:synthetic-secret@proxy.fixture:7890',tmdb:true,sites:false,revision:2}:stored}});
 });return writes;
}
async function geometry(page:Page,width:number){
 const panel=page.getByRole('region',{name:'网络代理',exact:true}),fields=panel.locator('.proxy-fields');
 expect((await fields.evaluate(el=>getComputedStyle(el).gridTemplateColumns)).split(' ')).toHaveLength(width===1440?2:1);
 const rows=await panel.locator('.proxy-options>label').all();expect(rows).toHaveLength(4);
 for(let i=0;i<rows.length;i++){
  const b=await rows[i]!.boundingBox();expect(b!.height).toBeGreaterThanOrEqual(44);
  if(i){const a=await rows[i-1]!.boundingBox();expect(b!.y-a!.y-a!.height).toBeLessThanOrEqual(1)}
 }
 expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
}
test('I17 structured fields have two protocols, compact checks and secret-free keep/clear save contracts',async({page})=>{
 const writes=await fixture(page);await page.goto('/connections?tab=aux');const panel=page.getByRole('region',{name:'网络代理',exact:true});
 await expect(panel.getByLabel('代理密码',{exact:true})).toHaveValue('');await expect(page.getByRole('link',{name:'管理站点账号',exact:true})).toHaveCount(0);
 for(const width of [1440,390]){await page.setViewportSize({width,height:1000});await geometry(page,width)}
 await panel.locator('.el-select').click();await expect(page.getByRole('option')).toHaveCount(2);await expect(page.getByRole('option',{name:'SOCKS5',exact:true})).toBeVisible();await page.keyboard.press('Escape');
 await panel.getByRole('button',{name:'保存代理设置',exact:true}).click();await expect.poll(()=>writes.length).toBe(1);
 expect(writes[0]).toMatchObject({protocol:'http',host:'proxy.fixture',port:7890,username:'fixture-user',password:'',clearPassword:false,revision:2});expect(writes[0]).not.toHaveProperty('url');
 await panel.getByLabel('代理主机',{exact:true}).fill('new.fixture');await expect(panel.getByRole('button',{name:'保存代理设置',exact:true})).toBeDisabled();
 await panel.getByLabel('代理密码',{exact:true}).fill('discarded-synthetic');await panel.getByText('明确清除已保存的代理密码',{exact:true}).click();await expect(panel.getByLabel('代理密码',{exact:true})).toHaveValue('');
 await panel.getByRole('button',{name:'保存代理设置',exact:true}).click();await expect.poll(()=>writes.length).toBe(2);expect(writes[1].clearPassword).toBe(true);expect(writes[1].password).toBe('');
 await expect(panel.getByRole('checkbox',{name:'明确清除已保存的代理密码',exact:true})).toHaveCount(0);
});
test('I17 old payload exposes needsInput without disabling routes or silently saving empty settings',async({page})=>{
 const writes=await fixture(page,true);await page.goto('/connections?tab=aux');const panel=page.getByRole('region',{name:'网络代理',exact:true});
 await expect(panel.getByText(/旧格式或无法识别的配置已保留/)).toBeVisible();await expect(panel.getByRole('checkbox',{name:'TMDB查询与海报使用代理',exact:true})).toBeChecked();
 await expect(panel.getByRole('button',{name:'保存代理设置',exact:true})).toBeDisabled();expect(writes).toHaveLength(0);
 await panel.getByLabel('代理主机',{exact:true}).fill('replacement.fixture');await panel.getByLabel('代理用户名',{exact:true}).fill('new-user');await panel.getByLabel('代理密码',{exact:true}).fill('synthetic-new');
 await panel.getByRole('button',{name:'保存代理设置',exact:true}).click();await expect.poll(()=>writes.length).toBe(1);expect(writes[0].tmdb).toBe(true);await expect(panel.getByLabel('代理密码',{exact:true})).toHaveValue('');
});
test.describe('I17 proxy local screenshots @screenshots',()=>{
 test.skip(!capture,'Explicit I17_SCREENSHOTS=1 only, never CI');
 for(const width of [1440,390])for(const theme of ['light','dark'])test(`proxy-${width}-${theme}`,async({page})=>{
  mkdirSync(dir,{recursive:true});await fixture(page);
  if(before){const root=resolve(dir,'../before-assets');await page.route('**/connections{,?*}',r=>r.fulfill({contentType:'text/html',body:readFileSync(join(root,'index.html'))}));await page.route('**/assets/*',r=>{const file=basename(new URL(r.request().url()).pathname);return r.fulfill({contentType:extname(file)==='.js'?'application/javascript':'text/css',body:readFileSync(join(root,'assets',file))})})}
  await page.setViewportSize({width,height:1000});await page.addInitScript(t=>localStorage.setItem('media-tool:theme-mode:v1',t),theme);await page.goto('/connections?tab=aux');
  await expect(page.getByRole('region',{name:'网络代理',exact:true}).getByRole('button',{name:'保存代理设置',exact:true})).toBeVisible();await expect(page.locator('html')).toHaveAttribute('data-theme',theme);
  if(!before)await geometry(page,width);
  expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
  await page.screenshot({path:join(dir,`${before?'before':'after'}-proxy-${width}-${theme}.png`),fullPage:true,animations:'disabled'});
 });
});
