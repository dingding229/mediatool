import {test,expect} from '@playwright/test'
import type {Page} from '@playwright/test'
import {readFileSync} from 'node:fs'
import {resolve} from 'node:path'
if(process.env.E2E_ISOLATED!=='1')throw Error('isolated test stack required')
// A non-loopback HTTP origin genuinely disables secure-context APIs. Route
// only static transport to loopback so DNS/system proxy settings are irrelevant;
// the browser document origin remains HTTP .test (not a spoofed isSecureContext).
const base=process.env.R_INSTALL_UI_ORIGIN??'http://mediatool-insecure.test:18084'
const contract=JSON.parse(readFileSync(resolve(import.meta.dirname,'../../internal/platform/httpapi/testdata/contracts/plugin-v2-instances.json'),'utf8'))
async function fixture(page:Page){
 await page.route(base+'/**',async route=>{const url=new URL(route.request().url());const response=await route.fetch({url:'http://127.0.0.1:'+url.port+url.pathname+url.search});await route.fulfill({response})})
 await page.route('**/api/**',route=>route.fulfill({json:{data:[]}}))
 await page.route('**/api/v1/auth/session',r=>r.fulfill({json:{data:{authenticated:true,account:{id:'r-ui-fixture',username:'fixture'}}}}))
 await page.route('**/api/v1/system/status',r=>r.fulfill({json:{data:{version:'R UI fixture',dependencies:[]}}}))
 const definition={manifest:{id:'hello-go',name:'Hello Go',version:'1.9.0',description:'R installation fixture',permissions:contract.request.scopes.map((scope:string)=>({scope}))},fingerprint:contract.request.fingerprint,installedCount:0}
 return definition
}
async function confirm(page:Page){const dialog=page.getByRole('dialog',{name:'插件权限确认',exact:true});await dialog.getByRole('checkbox').check();await dialog.getByRole('button',{name:'确认安装插件',exact:true}).click();return dialog}
test('R actual insecure HTTP: confirmation, rejected requestId and committed/list-failed states',async({page},info)=>{
 const definition=await fixture(page);let instance:any=null,failList=false,reject=true;const calls:any[]=[]
 await page.route('**/api/v1/plugins/catalog?*',async r=>{if(failList){failList=false;await r.fulfill({status:503,json:{error:{code:'plugins_unavailable',message:'private-list-sentinel'}}});return}await r.fulfill({json:{data:{items:[{...definition,installedCount:instance?1:0}],total:1}}})})
 await page.route('**/api/v1/plugins?*',r=>r.fulfill({json:{data:instance?[instance]:[]}}))
 await page.route('**/api/v1/plugins/operations',async r=>{calls.push(r.request().postDataJSON());if(reject){reject=false;await r.fulfill({status:409,headers:{'X-Request-ID':'abcdef0123456789abcdef01'},json:{error:{code:'plugin_conflict',message:'private-response-sentinel'}}});return}instance=contract.response;failList=true;await r.fulfill({json:{data:instance}})})
 await page.goto(base+'/plugins?tab=catalog');expect(await page.evaluate(()=>isSecureContext)).toBe(false);expect(await page.evaluate(()=>typeof crypto.randomUUID)).toBe('undefined')
 await page.getByRole('button',{name:'安装插件',exact:true}).click();const dialog=page.getByRole('dialog',{name:'插件权限确认',exact:true})
 await expect(dialog).toContainText('尚未提交');await expect(dialog.getByRole('button',{name:'确认安装插件',exact:true})).toBeDisabled();expect(calls).toHaveLength(0)
 await confirm(page);await dialog.getByText('技术详情',{exact:true}).click();await expect(dialog).toContainText('plugin_conflict');await expect(dialog).toContainText('abcdef0123456789abcdef01');await expect(dialog).not.toContainText('private-response-sentinel')
 await dialog.getByRole('button',{name:'确认安装插件',exact:true}).click();await expect(dialog).toBeHidden()
 await expect(page.getByRole('status')).toContainText('已安装');await expect(page.getByRole('alert').filter({hasText:'列表同步失败'})).toBeVisible()
 expect(calls).toHaveLength(2)
 await page.getByRole('button',{name:'刷新',exact:true}).click();await expect(page.getByRole('alert')).toHaveCount(0);expect(calls).toHaveLength(2)
 await expect(page.getByRole('button',{name:'新增独立实例',exact:true})).toBeVisible();await page.getByRole('button',{name:'已安装',exact:true}).click();await expect(page.locator('.u6-card')).toContainText('已停用')
 await page.setViewportSize({width:390,height:1000});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await page.screenshot({path:info.outputPath('r-install-feedback-390.png'),fullPage:true})
})
test('R actual insecure HTTP: lost reply survives reload, retries same intent, new instance is explicit',async({page})=>{
 const definition=await fixture(page);const instances:any[]=[],keys:string[]=[];const receipts=new Map<string,any>();let lose=true
 await page.route('**/api/v1/plugins/catalog?*',r=>r.fulfill({json:{data:{items:[{...definition,installedCount:instances.length}],total:1}}}))
 await page.route('**/api/v1/plugins?*',r=>r.fulfill({json:{data:instances}}))
 await page.route('**/api/v1/plugins/operations',async r=>{const input=r.request().postDataJSON();keys.push(input.operationKey);let instance=receipts.get(input.operationKey);if(!instance){instance={...contract.response,id:instances.length?'22222222-0000-4000-8000-000000000062':contract.response.id};instances.push(instance);receipts.set(input.operationKey,instance)}if(lose){lose=false;await r.abort('connectionreset');return}await r.fulfill({json:{data:instance}})})
 await page.goto(base+'/plugins?tab=catalog');await page.getByRole('button',{name:'安装插件',exact:true}).click();await confirm(page);await expect(page.getByRole('alert')).toContainText('结果未确认')
 await page.reload();await page.getByRole('button',{name:'核对原请求',exact:true}).click();await expect.poll(()=>keys.length).toBe(2);await expect(page.getByRole('dialog',{name:'插件权限确认',exact:true})).toBeHidden()
 expect(keys).toHaveLength(2);expect(keys[0]).toBe(keys[1]);expect(instances).toHaveLength(1)
 await page.getByRole('button',{name:'新增独立实例',exact:true}).click();await expect(page.getByRole('dialog',{name:'插件权限确认',exact:true})).toContainText('将新增独立实例');await confirm(page)
 await page.getByRole('button',{name:'已安装',exact:true}).click();await expect(page.locator('.u6-card')).toHaveCount(2);expect(keys[2]).not.toBe(keys[0]);expect(instances.every(i=>i.desiredState==='disabled')).toBe(true)
})
