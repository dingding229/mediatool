import {test,expect,type Page} from '@playwright/test'
if(process.env.E2E_ISOLATED!=='1')throw Error('isolated test stack required')
const base=process.env.R_INSTALL_UI_ORIGIN??'http://mediatool-insecure.test:18084'
async function fixture(page:Page){
 await page.route(base+'/**',async r=>{const u=new URL(r.request().url());await r.fulfill({response:await r.fetch({url:'http://127.0.0.1:'+u.port+u.pathname+u.search})})})
 await page.route('**/api/v1/**',r=>r.fulfill({json:{data:[]}}))
 await page.route('**/api/v1/auth/session',r=>r.fulfill({json:{data:{authenticated:true,account:{id:'loading-fixture',username:'fixture'}}}}))
 await page.route('**/api/v1/system/status',r=>r.fulfill({json:{data:{version:'0.27.0',dependencies:[]}}}))
}
function gate(){let release!:()=>void;const promise=new Promise<void>(r=>release=r);return {promise,release}}
test('initial slow plugin lists never claim zero or no instances, including insecure HTTP',async({page},info)=>{
 await fixture(page);const pending=gate();let writes=0
 await page.route('**/api/v1/plugins/operations',r=>{writes++;return r.abort()})
 await page.route('**/api/v1/plugins/catalog?*',async r=>{await pending.promise;await r.fulfill({json:{data:{items:[],total:0}}})})
 await page.route('**/api/v1/plugins?*',async r=>{await pending.promise;await r.fulfill({json:{data:[]}})})
 await page.goto(base+'/plugins');expect(await page.evaluate(()=>isSecureContext)).toBe(false)
 try{
  await expect(page.locator('.state-view.state-loading')).toBeVisible()
  await expect(page.getByText('共 0 项',{exact:false})).toHaveCount(0)
  await expect(page.getByText('尚未安装匹配的插件',{exact:true})).toHaveCount(0)
  await page.setViewportSize({width:390,height:1000});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  await page.screenshot({path:info.outputPath('plugin-loading-390.png'),fullPage:true})
 }finally{pending.release()}
 await expect(page.getByText('尚未安装匹配的插件',{exact:true})).toBeVisible();expect(writes).toBe(0)
})
test('initial failure is an error with retry, never empty; successful retry can show empty',async({page})=>{
 await fixture(page);let fail=true
 await page.route('**/api/v1/plugins/catalog?*',r=>fail?r.fulfill({status:503,json:{error:{code:'plugins_unavailable',message:'private-error-sentinel'}}}):r.fulfill({json:{data:{items:[],total:0}}}))
 await page.route('**/api/v1/plugins?*',r=>r.fulfill({json:{data:[]}}))
 await page.goto(base+'/plugins?tab=catalog');await expect(page.getByRole('alert').filter({hasText:'插件信息暂时无法读取'})).toBeVisible();await expect(page.locator('main')).not.toContainText('private-error-sentinel')
 await expect(page.getByText('尚未安装匹配的插件',{exact:true})).toHaveCount(0)
 await expect(page.getByText('共 0 项',{exact:false})).toHaveCount(0)
 fail=false;await page.getByRole('button',{name:'重新读取',exact:true}).click()
 await expect(page.getByRole('alert')).toHaveCount(0);await expect(page.locator('.state-view.state-empty')).toBeVisible()
})

async function installedFixture(page:Page,running=false){
 await fixture(page)
 const instance={id:'11111111-1111-4111-8111-111111111111',pluginId:'hello-go',desiredState:running?'enabled':'disabled',state:running?'running':'stopped',revision:22,availability:'available',reason:''}
 const definition={manifest:{id:'hello-go',name:'Hello Go',version:'1.9.0',description:'Synthetic loading fixture',permissions:[]},fingerprint:'fixture',installedCount:1}
 await page.route('**/api/v1/plugins/catalog?*',r=>r.fulfill({json:{data:{items:[definition],total:1}}}))
 await page.route('**/api/v1/plugins?*',r=>r.fulfill({json:{data:[instance]}}))
 await page.route('**/api/v1/plugins/'+instance.id+'?*',r=>r.fulfill({json:{data:{instance,config:{value:{},revision:22},entries:[],next:'',fingerprint:'fixture'}}}));
 return instance
}
for(const sample of [
 {button:'工作流管理',dialog:'工作流管理',endpoint:'**/api/v1/plugins/workflows?*',caption:'工作流列表',empty:'尚无工作流'},
 {button:'数据快照',dialog:'实例数据快照',endpoint:'**/api/v1/plugins/*/state-snapshots',caption:'实例快照',empty:'尚无实例快照'},
 {button:'认证状态',dialog:'认证状态',endpoint:'**/api/v1/plugins/*/auth/status',caption:'认证状态',empty:'此定义没有认证 provider'},
 {button:'工具测试',dialog:'人工工具测试',endpoint:'**/api/v1/plugins/*/tools',caption:'工具列表',empty:'没有已授权且运行中的工具'},
]){
 test(`${sample.caption}: pending/error never empty; retry is read only`,async({page})=>{
  const instance=await installedFixture(page,sample.button==='工具测试');const pending=gate();let phase='slow';const methods:string[]=[]
  await page.route(sample.endpoint,async r=>{methods.push(r.request().method());if(phase==='slow')await pending.promise;await r.fulfill(phase==='empty'?{json:{data:[]}}:{status:503,json:{error:{code:'plugins_unavailable',message:'private-error-sentinel'}}})})
  await page.goto(base+(sample.button==='工作流管理'?'/plugins':'/plugins/'+instance.id));if(sample.button!=='工作流管理')await page.getByRole('button',{name:'实例数据',exact:true}).click();await page.getByRole('button',{name:sample.button,exact:true}).click();const dialog=page.getByRole('dialog',{name:sample.dialog,exact:true})
  try{await expect(dialog.getByRole('status').filter({hasText:sample.caption+'加载中'})).toBeVisible();await expect(dialog.getByText(sample.empty,{exact:true})).toHaveCount(0)}finally{pending.release()}
  await expect(dialog.getByRole('alert').filter({hasText:'plugins_unavailable'})).toBeVisible();await expect(dialog.getByText(sample.empty,{exact:true})).toHaveCount(0);await expect(dialog).not.toContainText('private-error-sentinel')
  phase='empty';await dialog.getByRole('button',{name:'重试读取'+sample.caption,exact:true}).click();await expect(dialog.getByText(sample.empty,{exact:true})).toBeVisible();expect(methods).toEqual(['GET','GET'])
 })
}
test('filtered log count is unknown during loading/error and retry keeps module and instance',async({page})=>{
 await fixture(page);const pending=gate();let phase='slow';const urls:string[]=[]
 await page.route('**/api/v1/logs/summary',r=>r.fulfill({json:{data:{totalBytes:0,limitBytes:1024,level:'info',files:[]}}}))
 await page.route('**/api/v1/logs/query?*',async r=>{urls.push(r.request().url());if(phase==='slow')await pending.promise;await r.fulfill(phase==='empty'?{json:{data:{items:[],total:0,page:1,pageSize:200,modules:['plugins'],before:'',truncated:false}}}:{status:503,json:{error:{code:'plugins_unavailable',message:'synthetic unavailable'}}})})
 await page.goto(base+'/logs?module=plugins&instance=owned')
 try{await expect(page.getByText('日志数量加载中…',{exact:true})).toBeVisible();await expect(page.getByText('没有符合筛选条件的日志',{exact:true})).toHaveCount(0);await expect(page.getByText('当前检索窗口共 0 条',{exact:true})).toHaveCount(0)}finally{pending.release()}
 await expect(page.getByText('读取失败，数量未知',{exact:true})).toBeVisible();await expect(page.getByText('没有符合筛选条件的日志',{exact:true})).toHaveCount(0)
 phase='empty';await page.getByRole('button',{name:'刷新日志',exact:true}).click();await expect(page.getByText('当前检索窗口共 0 条',{exact:true})).toBeVisible();expect(urls).toHaveLength(2);expect(urls.every(u=>new URL(u).searchParams.get('module')==='plugins'&&new URL(u).searchParams.get('instance')==='owned')).toBe(true)
})
