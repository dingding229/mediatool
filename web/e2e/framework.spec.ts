import { test,expect } from '@playwright/test'
import { mkdirSync,readFileSync,writeFileSync,existsSync } from 'node:fs'
import { resolve } from 'node:path'
import { navigationItems } from '../src/navigation/items'
const root=resolve(import.meta.dirname,'../..')
const schemaMatch=readFileSync(resolve(root,'internal/platform/buildinfo/buildinfo.go'),'utf8').match(/^const Schema int64 = (\d+)$/m)
if(!schemaMatch)throw new Error('Build schema declaration is missing')
const expectedSchema=Number(schemaMatch[1])
const expectedVersion=readFileSync(resolve(root,'internal/platform/buildinfo/buildinfo.go'),'utf8').match(/^const Version = "([^"]+)"$/m)![1]
if(process.env.E2E_ISOLATED!=='1'||process.env.E2E_BASE_URL!=='http://localhost:18084')throw new Error('Use bash scripts/test-e2e.sh; account tests must not register an administrator in the business database.')
const username=process.env.E2E_ADMIN_USERNAME!,password=process.env.E2E_ADMIN_PASSWORD!
if(!username||!password)throw new Error('Isolated account credentials required')
const screenshots=resolve(root,'web/test-results/screenshots')
const transportDiagnostics=new WeakMap<import('@playwright/test').Page,Array<{category:string;elapsedMs:number;status?:number;failureClass?:string}>>()
function requestCategory(raw:string){
 const path=new URL(raw).pathname
 if(path.startsWith('/api/v1/auth/'))return 'authentication'
 if(path.startsWith('/api/v1/integrations'))return 'connections'
 if(path.startsWith('/api/v1/jobs'))return 'jobs'
 if(path.startsWith('/api/v1/metadata'))return 'metadata'
 if(path.startsWith('/api/v1/media'))return 'media'
 if(path.startsWith('/api/'))return 'other_api'
 return 'ui_or_asset'
}
test.beforeEach(async({page})=>{
 await page.route('**/api/v1/plugins/*?*',async r=>{
  const u=new URL(r.request().url());if(!/^\/api\/v1\/plugins\/[a-f0-9-]{36}$/.test(u.pathname))return r.fallback()
  const real=await r.fetch();if(real.ok())return r.fulfill({response:real})
  const id=u.pathname.split('/').pop()!;const fixture=await page.evaluate(async id=>{const list=await(await fetch('/api/v1/plugins?page=1&pageSize=50')).json();const row=list.data?.find((x:any)=>x.id===id);const directory=await(await fetch('/api/v1/plugins/catalog?page=1&pageSize=200')).json();return{row,definition:directory.data?.items?.find((x:any)=>x.manifest.id===row?.pluginId)}},id)
  if(!fixture.row)return r.fulfill({response:real})
  return r.fulfill({json:{data:{instance:fixture.row,config:{value:{},revision:fixture.row.revision},entries:[],next:'',fingerprint:fixture.definition?.fingerprint||'fixture'}}})
 })

 await page.route('**/api/v1/sites/catalog**',route=>route.fulfill({json:{data:{catalogVersion:1,repository:'dingding229/mediatool-site',sourceCommit:'a'.repeat(40),templates:['hdsky','ptchdbits','ssd','ourbits','ubits','ttg'].map(id=>({id,name:id,file:`sites/${id}.json`,sha256:'b'.repeat(64),templateVersion:1}))}}}))
 const began=Date.now(),events:Array<{category:string;elapsedMs:number;status?:number;failureClass?:string}>=[]
 transportDiagnostics.set(page,events)
 page.on('response',response=>{if(events.length<100)events.push({category:requestCategory(response.url()),elapsedMs:Date.now()-began,status:response.status()})})
 page.on('requestfailed',request=>{const raw=request.failure()?.errorText??'';if(events.length<100)events.push({category:requestCategory(request.url()),elapsedMs:Date.now()-began,failureClass:/^net::[A-Z_]+$/.test(raw)?raw:'request_failed'})})
})
test.afterEach(async({page},info)=>{
 if(info.status===info.expectedStatus)return
 const diagnostic={isolatedTransportOnly:true,events:transportDiagnostics.get(page)??[]}
 console.info(JSON.stringify(diagnostic))
 await info.attach('isolated-transport-diagnostic',{body:JSON.stringify(diagnostic),contentType:'application/json'})
})
async function navigate(page:import('@playwright/test').Page,name:string){
 const aliases:Record<string,[string,string?]>={'系统设置':['settings','general'],'PT 站点':['sites','accounts'],'STRM工具':['files','tools'],'STRM目录':['files','strm'],'下载与传输':['downloads','uploads'],'云盘索引':['organize','candidates'],'任务队列':['ops','jobs'],'程序日志':['ops','logs'],'自研媒体库':['playback','settings'],'资源订阅':['subscriptions']}
 const target=aliases[name]??[navigationItems.find(item=>item.name===name)?.id];const item=navigationItems.find(item=>item.id===target[0]);if(!item)throw new Error('Unknown navigation target: '+name)
 const menu=page.getByRole('navigation',{name:'功能导航'})
 if(!await menu.isVisible())await page.getByRole('button',{name:'打开导航',exact:true}).click()
 await menu.getByRole('link',{name:item.name,exact:true}).click()
 if(target[1]){const tab=page.locator(['organize','files','playback','sites','plugins','ops'].includes(target[0])?'.media-workspace .page-tabs':['search','downloads'].includes(target[0])?'.acquire-workspace .page-tabs':'.settings-workspace > .page-tabs').getByRole('button').filter({hasText:({tasks:'下载任务',uploads:'上传入库',manual:'手动添加',tools:'STRM 工具',strm:'STRM 目录',candidates:'待确认',library:'入库整理',jobs:'任务队列',logs:'日志',settings:'服务设置',accounts:'站点账号',general:'通用与访问'} as Record<string,string>)[target[1]]??''});await expect(tab).toHaveCount(1);await tab.click()}
}
async function login(page:import('@playwright/test').Page){
 await page.goto('/login')
 await expect(page.getByRole('heading',{name:'进入管理工作台',exact:true})).toBeVisible()
 await page.getByLabel('管理员账号',{exact:true}).fill(username)
 await page.getByLabel('密码',{exact:true}).fill(password)
 await page.getByRole('button',{name:'进入工作台',exact:true}).click()
 await expect(page.getByRole('heading',{name:'首页'})).toBeVisible()
 await expect(page.getByText('服务器负载',{exact:true})).toBeVisible()
 await expect(page.locator('.workbench-version')).toContainText(expectedVersion)
}
// Later UI-only fixtures share one valid account session in memory. Login/
// logout/password/rate-limit behavior above remains separately exercised; do
// not weaken production limits just because the suite gained more UI fixtures.
let fixtureCookies:Awaited<ReturnType<import('@playwright/test').BrowserContext['cookies']>>|undefined
async function fixtureLogin(page:import('@playwright/test').Page){
 if(!fixtureCookies){await login(page);fixtureCookies=await page.context().cookies();return}
 await page.context().addCookies(fixtureCookies)
 await page.goto('/dashboard')
 await expect(page.getByRole('heading',{name:'首页'})).toBeVisible()
}
test('first registration creates the administrator and closes public registration',async({page,request})=>{
 await page.goto('/login');await expect(page.getByRole('heading',{name:'注册管理员账号',exact:true})).toBeVisible()
 await expect(page.getByText('6–20 个字符，支持中文与空格，不会去除首尾空格。',{exact:true})).toBeVisible()
 await page.getByLabel('管理员账号',{exact:true}).fill(username);await page.getByLabel('密码',{exact:true}).fill('abc12');await page.getByLabel('确认密码',{exact:true}).fill('abc12');await page.getByRole('button',{name:'注册并进入工作台'}).click();await expect(page.getByText('密码需6–20个字符，可使用空格和中文',{exact:true})).toBeVisible()
 await page.getByLabel('管理员账号',{exact:true}).fill(username);await page.getByLabel('密码',{exact:true}).fill(password);await page.getByLabel('确认密码',{exact:true}).fill(password+'X')
 mkdirSync(screenshots,{recursive:true});await page.screenshot({path:resolve(screenshots,'account-registration-desktop.png'),fullPage:true,mask:[page.getByLabel('密码',{exact:true}),page.getByLabel('确认密码',{exact:true})]})
 await page.getByRole('button',{name:'注册并进入工作台'}).click();await expect(page.getByText('两次输入的密码不一致',{exact:true})).toBeVisible()
 await page.getByLabel('确认密码',{exact:true}).fill(password);const registered=page.waitForResponse(r=>r.url().endsWith('/auth/register'));await page.getByRole('button',{name:'注册并进入工作台'}).click();const reply=await registered;expect(reply.status()).toBe(201);expect(await reply.text()).not.toContain(password)
 await expect(page.getByRole('heading',{name:'首页'})).toBeVisible();const setup=await request.get('/api/v1/auth/setup');expect((await setup.json()).data.registrationOpen).toBe(false)
 const second=await request.post('/api/v1/auth/register',{headers:{'X-Requested-With':'media-tool'},data:{username:'second_admin',password,confirmPassword:password}});expect(second.status()).toBe(409)
 await page.getByRole('button',{name:'退出',exact:true}).click();await expect(page.getByRole('heading',{name:'进入管理工作台',exact:true})).toBeVisible();await expect(page.getByLabel('确认密码',{exact:true})).toHaveCount(0)
})
test('login, honest dependency states, module navigation and logout',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
 await login(page)
 await expect(page.getByText('服务器负载',{exact:true})).toBeVisible()
 await expect(page.getByRole('heading',{name:'依赖服务',exact:true})).toHaveCount(0)
 mkdirSync(screenshots,{recursive:true});await page.screenshot({path:resolve(screenshots,'dashboard-desktop.png'),fullPage:true})
 await navigate(page,'系统设置')
 await expect(page.locator('.settings-workspace > .page-tabs')).toBeVisible()
 await expect(page.getByRole('button',{name:'验证 CloudDrive2',exact:true})).toHaveCount(0)
 await expect(page.getByRole('button',{name:'验证 Emby',exact:true})).toHaveCount(0)
 await navigate(page,'PT 站点')
 await expect(page.getByRole('heading',{name:'站点账号',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'退出',exact:true}).click()
 await expect(page.getByRole('heading',{name:'进入管理工作台'})).toBeVisible()
 expect(errors).toEqual([])
})
test('STRM preview encodes cloud path and download has one stable URL',async({page})=>{
 await login(page);await navigate(page,'STRM工具')
 const path='/115/Media/电影/幽灵公主 (1997)/电影 & 特别版#1.mkv'
 await page.getByLabel('CloudDrive2 完整路径',{exact:false}).fill(path)
 await page.getByRole('button',{name:'生成固定链接'}).click()
 await page.getByRole('button',{name:'诊断此文件',exact:true}).click()
 await expect(page.getByText('请先配置 CloudDrive2 地址及读取凭据',{exact:true})).toBeVisible()
 const textarea=page.getByRole('textbox',{name:'STRM原文'})
 await expect(textarea).toHaveValue(/strm302\?/)
 const generated=new URL(await textarea.inputValue())
 expect(generated.searchParams.get('path')).toBe(path)
 expect(generated.searchParams.has('apiKey')).toBe(true)
 expect(generated.searchParams.has('pickCode')).toBe(false)
 const downloadPromise=page.waitForEvent('download')
 await page.getByRole('button',{name:'下载 STRM'}).click()
 const download=await downloadPromise;expect(download.suggestedFilename()).toMatch(/\.strm$/)
 const stream=await download.createReadStream();const parts:Buffer[]=[];for await(const chunk of stream!)parts.push(chunk)
 expect(Buffer.concat(parts).toString()).toBe(generated.toString()+'\n')
 await page.screenshot({path:resolve(screenshots,'strm-preview.png'),fullPage:true,mask:[textarea]})
 await page.getByLabel('CloudDrive2 完整路径',{exact:false}).fill('/115/Media/../private/a.mkv')
 await page.getByRole('button',{name:'生成固定链接'}).click()
 await expect(page.getByText('path traversal is not allowed')).toBeVisible()
})
test('persistent queue runs system check with live isolated worker',async({page})=>{
 await login(page);await navigate(page,'任务队列')
 const accepted=page.waitForResponse(r=>r.url().endsWith('/api/v1/jobs')&&r.request().method()==='POST');await page.getByRole('button',{name:'运行系统检查'}).click();await page.getByRole('dialog',{name:'运行系统检查',exact:true}).getByRole('button',{name:'确认运行检查',exact:true}).click();expect((await accepted).status()).toBe(201)
 try{await expect(page.locator('.organize-record').filter({hasText:'系统检查'}).getByText(/已完成/).first()).toBeVisible({timeout:15000})}catch(error){
  const response=await page.request.get('/api/v1/jobs')
  const payload=response.ok()?await response.json():{data:[]}
  console.info(JSON.stringify({isolatedQueueDiagnostic:true,status:response.status(),jobs:(payload.data??[]).filter((j:{kind:string})=>j.kind==='system.check').map((j:{state:string;attempts:number})=>({state:j.state,attempts:j.attempts})),pageVisibility:await page.evaluate(()=>document.visibilityState)}))
  throw error
 }
})
test('mobile menu, reduced motion and layout at 375px/landscape',async({page})=>{
 await page.setViewportSize({width:375,height:812});await page.emulateMedia({reducedMotion:'reduce'});await login(page)
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
 await page.getByRole('button',{name:'打开导航'}).click()
 await navigate(page,'STRM目录')
 await expect(page.getByRole('heading',{name:'STRM 目录',exact:true})).toBeVisible()
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
 await page.screenshot({path:resolve(screenshots,'strm-mobile.png'),fullPage:true})
 await page.setViewportSize({width:812,height:375})
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
})

test('saved connections are editable, redacted, and can be deleted',async({page})=>{
 await login(page);await page.goto('/cloud-connections')
 await page.getByRole('button',{name:'添加连接'}).click()
 const name='Browser connection '+Date.now()
 await page.getByLabel('连接名称',{exact:true}).fill(name)
 await page.getByLabel('原生 gRPC 地址',{exact:true}).fill('http://127.0.0.1:9')
 await page.getByLabel('CloudDrive2 API Token',{exact:true}).fill('')
 const responsePromise=page.waitForResponse(r=>r.url().endsWith('/api/v1/integrations')&&r.request().method()==='POST')
 await page.getByRole('button',{name:'加密保存',exact:true}).click()
 const response=await responsePromise
 expect(response.status()).toBe(201)
 expect(await response.text()).not.toContain('synthetic-browser-secret')
 const row=page.locator('.saved-connection').filter({hasText:name})
 await expect(row).toBeVisible()
 await row.getByRole('button',{name:'编辑',exact:true}).click()
 await expect(page.getByLabel('CloudDrive2 API Token',{exact:true})).toHaveValue('')
 await page.getByLabel('连接名称',{exact:true}).fill(name+' updated')
 await page.getByRole('button',{name:'加密保存',exact:true}).click()
 const updated=page.locator('.saved-connection').filter({hasText:name+' updated'})
 await updated.getByRole('button',{name:'删除',exact:true}).click()
 await page.getByRole('button',{name:'删除连接',exact:true}).click()
 await expect(updated).toHaveCount(0)
})

test('subscriptions and scanner pages expose real configuration gates',async({page})=>{
 await login(page);await navigate(page,'资源订阅')
 await expect(page.getByRole('heading',{name:'订阅',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'新建订阅',exact:true}).click()
 await expect(page.getByRole('button',{name:'选择站点与规则',exact:true})).toBeDisabled()
 await expect(page.getByRole('navigation',{name:'创建订阅步骤',exact:true})).toBeVisible()
 await page.keyboard.press('Escape')
 await expect(page.getByRole('dialog',{name:'新建订阅',exact:true})).not.toBeVisible()
 await navigate(page,'云盘索引')
 await page.locator('.media-workspace .page-tabs').getByRole('button',{name:'已入库',exact:true}).click();await expect(page.getByRole('heading',{name:'已入库资产',exact:true})).toBeVisible()
 await page.locator('.media-workspace .page-tabs').getByRole('button',{name:'扫描',exact:true}).click();await page.getByRole('button',{name:'预览扫描范围',exact:true}).click();await page.getByRole('button',{name:'确认范围并开始扫描',exact:true}).click();await page.getByRole('button',{name:'确认开始扫描',exact:true}).click()
 await expect(page.getByText('configure a primary CloudDrive2 connection',{exact:true})).toBeVisible()
})
test('logout revokes a previously captured signed cookie',async({page,request})=>{
 await login(page)
 const cookie=(await page.context().cookies()).find(c=>c.name==='media_tool_session')
 expect(cookie).toBeTruthy()
 await page.getByRole('button',{name:'退出',exact:true}).click()
 await expect(page.getByRole('heading',{name:'进入管理工作台'})).toBeVisible()
 const response=await request.get('/api/v1/auth/session',{headers:{Cookie:'media_tool_session='+cookie!.value}})
 expect(response.status()).toBe(401)
})

test('scan schedule dialog is default-off and pagination requests are bounded',async({page})=>{
 await page.emulateMedia({reducedMotion:'reduce'});await login(page);const response=page.waitForResponse(r=>r.url().includes('/library/candidates?page=1&pageSize=50'))
 await navigate(page,'云盘索引');expect((await response).status()).toBe(200)
 await page.locator('.media-workspace .page-tabs').getByRole('button',{name:'扫描',exact:true}).click();await page.getByRole('button',{name:'新增定时扫描'}).click();const dialog=page.getByRole('dialog');await expect(dialog.getByRole('switch',{name:'启用定时扫描'})).not.toBeChecked();await expect(dialog.getByRole('switch',{name:'启用CloudDrive2文件变化推送'})).not.toBeChecked()
 await dialog.getByRole('button',{name:'保存定时扫描'}).click();await expect(page.getByText('请先在服务连接中配置并启用 CloudDrive2',{exact:true})).toBeVisible()
 await dialog.getByRole('button',{name:'取消',exact:true}).click();await expect(dialog).not.toBeVisible();await page.screenshot({path:resolve(screenshots,'library-desktop.png'),fullPage:true});await page.setViewportSize({width:375,height:812});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
 mkdirSync(screenshots,{recursive:true});await page.screenshot({path:resolve(screenshots,'library-mobile.png'),fullPage:true})
})

test('wrong credentials are rejected and password change revokes captured sessions',async({page,request})=>{
 const wrong=await request.post('/api/v1/auth/login',{headers:{'X-Requested-With':'media-tool'},data:{username,password:'Wrong-strong-password-2026'}});expect(wrong.status()).toBe(401)
 await login(page);const captured=(await page.context().cookies()).find(c=>c.name==='media_tool_session')!;await navigate(page,'系统设置')
 await page.locator('.settings-workspace > .page-tabs').getByRole('button',{name:'账号安全',exact:true}).click();const next=password+'-new';await page.getByLabel('当前密码',{exact:true}).fill(password);await page.getByLabel('新密码',{exact:true}).fill(next);await page.getByLabel('确认新密码',{exact:true}).fill(next);await page.getByRole('button',{name:'修改密码并重新登录'}).click();await expect(page.getByRole('heading',{name:'进入管理工作台',exact:true})).toBeVisible()
 const replay=await request.get('/api/v1/auth/session',{headers:{Cookie:'media_tool_session='+captured.value}});expect(replay.status()).toBe(401)
 const old=await request.post('/api/v1/auth/login',{headers:{'X-Requested-With':'media-tool'},data:{username,password}});expect(old.status()).toBe(401)
 await page.getByLabel('管理员账号',{exact:true}).fill(username);await page.getByLabel('密码',{exact:true}).fill(next);await page.getByRole('button',{name:'进入工作台',exact:true}).click();await expect(page.getByRole('heading',{name:'首页'})).toBeVisible()
 // Restore only the synthetic test account, never a user's administrator.
 await navigate(page,'系统设置');await page.locator('.settings-workspace > .page-tabs').getByRole('button',{name:'账号安全',exact:true}).click();await page.getByLabel('当前密码',{exact:true}).fill(next);await page.getByLabel('新密码',{exact:true}).fill(password);await page.getByLabel('确认新密码',{exact:true}).fill(password);await page.getByRole('button',{name:'修改密码并重新登录'}).click();await expect(page.getByRole('heading',{name:'进入管理工作台',exact:true})).toBeVisible()
})

test('account login form is accessible on mobile and needs no deployment key',async({page})=>{
 await page.setViewportSize({width:375,height:812});await page.emulateMedia({reducedMotion:'reduce'});await page.goto('/login');await expect(page.getByRole('heading',{name:'进入管理工作台',exact:true})).toBeVisible();await expect(page.getByLabel('管理员账号',{exact:true})).toBeVisible();await expect(page.getByLabel('密码',{exact:true})).toHaveAttribute('autocomplete','current-password');expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);expect(await page.evaluate(()=>localStorage.length)).toBe(0);await page.screenshot({path:resolve(screenshots,'account-login-mobile.png'),fullPage:true})
})

test('official 115 is saved disabled and QR authorization is explicit and cancellable',async({page})=>{
 await login(page);await page.goto('/cloud-connections')
 await page.getByRole('button',{name:'添加连接'}).click()
 await page.getByRole('button',{name:'关闭此对话框',exact:true}).click()
 await page.locator('.el-select').filter({has:page.getByRole('combobox',{name:'新增服务类型',exact:true})}).click();await page.getByRole('option',{name:'115 授权',exact:true}).click()
 await page.getByRole('button',{name:'添加连接',exact:true}).click()
 const name='Official115 fixture '+Date.now()
 await page.getByLabel('连接名称',{exact:true}).fill(name)
 await page.getByLabel('自己的115开放平台 AppID',{exact:true}).fill('12345')
 await expect(page.getByLabel('CloudDrive2中的115挂载前缀',{exact:true})).toHaveValue('/115')
 await page.getByRole('button',{name:'加密保存',exact:true}).click()
 const row=page.locator('.saved-connection').filter({hasText:name});await expect(row).toBeVisible()
 await expect(row).toContainText('未启用')
 await page.route('**/115/auth/start',route=>route.fulfill({json:{data:{sessionId:'00000000-0000-4000-8000-000000000001',state:'waiting',qrCodeImage:'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jZVkAAAAASUVORK5CYII=',expiresAt:new Date(Date.now()+300000).toISOString()}}}))
 let pollState='scanned',pollCount=0
 const pollReceipts:Array<{elapsedMs:number;state:string}>=[],pollStarted=Date.now()
 await page.route('**/115/auth/poll',route=>{pollCount++;pollReceipts.push({elapsedMs:Date.now()-pollStarted,state:pollState});return route.fulfill({json:{data:{sessionId:'00000000-0000-4000-8000-000000000001',state:pollState,expiresAt:new Date(Date.now()+300000).toISOString()}}})})
 let cancelled=false
 await page.route('**/115/auth/cancel',route=>{cancelled=true;return route.fulfill({json:{data:{state:'cancelled'}}})})
 await row.getByRole('button',{name:'115扫码授权',exact:true}).click()
 await expect(page.getByAltText('115官方扫码授权二维码')).toBeVisible()
 await expect(page.getByRole('status')).toContainText('已扫码，请在115 App中确认授权')
 pollState='exchanging'
 try{await expect(page.getByRole('status')).toContainText('正在加密保存授权结果')}catch(error){console.info(JSON.stringify({fixturePollReceipts:pollReceipts,pageVisibility:await page.evaluate(()=>document.visibilityState)}));throw error}
 await expect(page.getByAltText('115官方扫码授权二维码')).toHaveCount(0)
 const inFlightCount=pollCount
 await expect.poll(()=>pollCount).toBeGreaterThan(inFlightCount)
 await test.info().attach('115-fixture-poll-receipts',{body:JSON.stringify({fixtureOnly:true,receipts:pollReceipts,pageVisibility:await page.evaluate(()=>document.visibilityState)}),contentType:'application/json'})
 await page.screenshot({path:resolve(screenshots,'official-115-authorizing.png'),fullPage:true})
 await page.getByRole('dialog').getByRole('button',{name:'关闭',exact:true}).click()
 await expect.poll(()=>cancelled).toBe(true)
 await expect(page.getByAltText('115官方扫码授权二维码')).toHaveCount(0)
 // This fixture only tests UI behavior; never represents real 115 authorization.
 await row.getByRole('button',{name:'删除',exact:true}).click()
 await page.getByRole('button',{name:'删除连接',exact:true}).click()
})

test('Go template catalog supports selective loading, version, icon and website',async({page})=>{
 await page.route('https://**/favicon.ico',r=>r.abort());await fixtureLogin(page);await page.goto('/site-templates');await expect(page.getByRole('heading',{name:'站点模板',exact:true})).toBeVisible();const load=page.getByRole('button',{name:'加载所选模板',exact:true});await expect(load).toBeDisabled()
 for(const name of ['HDSKY','OURBITS'])await page.getByRole('checkbox',{name:'选择模板 '+name,exact:true}).check();await expect(load).toBeEnabled();await page.getByRole('button',{name:'选择全部未加载模板',exact:true}).click();await expect(page.locator('.u5-template input:checked')).toHaveCount(6);await page.getByRole('button',{name:'清空选择',exact:true}).click();await expect(load).toBeDisabled()
 for(const name of ['HDSKY','OURBITS'])await page.getByRole('checkbox',{name:'选择模板 '+name,exact:true}).check();const loaded=page.waitForResponse(r=>r.url().endsWith('/pt/templates/load'));await load.click();expect((await loaded).status()).toBe(200)
 const catalog=(await(await page.request.get('/api/v1/pt/templates')).json()).data.templates;expect(catalog.filter((x:any)=>x.loaded).map((x:any)=>x.id).sort()).toEqual(['hdsky','ourbits']);expect(catalog.every((x:any)=>x.version==='1.3.0'&&x.iconUrl===`/api/v1/pt/templates/${x.id}/icon?v=1.3.0`)).toBe(true)
 await page.reload();await expect(page.getByRole('checkbox',{name:'选择模板 HDSKY',exact:true})).toBeDisabled();await expect(page.getByRole('checkbox',{name:'选择模板 SSD',exact:true})).toBeEnabled();await expect.poll(()=>page.locator('.u5-template img').evaluateAll(imgs=>imgs.length===6&&imgs.every(img=>(img as HTMLImageElement).complete&&(img as HTMLImageElement).naturalWidth>0))).toBe(true)
 await page.getByRole('button',{name:'选择全部未加载模板',exact:true}).click();await expect(page.locator('.u5-template input:checked')).toHaveCount(4);await page.getByRole('button',{name:'清空选择',exact:true}).click();await expect(page.getByRole('link',{name:'https://hdsky.me',exact:true})).toHaveAttribute('href','https://hdsky.me')
 for(const name of ['HDSKY','OURBITS','CHDBITS','SSD','TTG','UBITS'])await expect(page.locator('.u5-template strong').filter({hasText:new RegExp('^'+name+'$')})).toBeVisible();await page.locator('.u5-template').filter({hasText:'HDSKY'}).getByRole('button',{name:'模板详情',exact:true}).click();await page.getByRole('button',{name:/技术细节与原始记录/}).click();await expect(page.getByRole('dialog')).toContainText('1.3.0');await page.keyboard.press('Escape')
 for(const text of ['来源提交','已导入配置','mediatool-site','天空','我堡','春天'])await expect(page.getByText(text,{exact:true})).toHaveCount(0);await page.screenshot({path:resolve(screenshots,'go-templates-desktop.png'),fullPage:true});await page.setViewportSize({width:390,height:844});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);await page.screenshot({path:resolve(screenshots,'go-templates-mobile.png'),fullPage:true})
})

test('strict direct-only policy is immutable and visible (isolated settings fixture)',async({page})=>{
 await fixtureLogin(page);await page.goto('/mediaLibrary')
 await expect(page.getByRole('link',{name:'Emby 配置',exact:true})).toHaveCount(0);await page.goto('/connections?tab=emby')
 await page.getByRole('button',{name:'添加连接'}).click()
 const name='Direct-only fixture '+Date.now()
 await page.getByLabel('连接名称',{exact:true}).fill(name)
 await page.getByLabel('Emby 服务地址',{exact:true}).fill('http://127.0.0.1:9')
 await expect(page.getByRole('checkbox',{name:'只使用影片原格式，不中转或转码'})).toBeDisabled()
 await expect(page.getByRole('checkbox',{name:'只使用影片原格式，不中转或转码'})).toBeChecked()
 await expect(page.getByText(/服务器不会转换视频或音频格式/)).toBeVisible()
 await page.getByRole('button',{name:'加密保存',exact:true}).click()
 const row=page.locator('.saved-connection').filter({hasText:name})
 await row.getByRole('button',{name:'编辑',exact:true}).click()
 await expect(page.getByRole('checkbox',{name:'只使用影片原格式，不中转或转码'})).toBeChecked()
 await page.getByRole('button',{name:'关闭此对话框',exact:true}).click()
 await row.getByRole('button',{name:'删除',exact:true}).click()
 await page.getByRole('button',{name:'删除连接',exact:true}).click()
 await expect(row).toHaveCount(0)
})

test('management Emby page survives a direct reload without entering the playback listener',async({page})=>{
 await fixtureLogin(page)
 await page.goto('/emby')
 await expect(page.getByRole('heading',{name:'Emby 网关诊断',exact:true})).toBeVisible()
 await page.reload()
 await expect(page.getByRole('heading',{name:'Emby 网关诊断',exact:true})).toBeVisible()
 await expect(page).toHaveURL(/\/connections\?.*tab=emby/)
 await expect(page.getByRole('heading',{name:'Emby媒体库连接',exact:true})).toBeVisible()
})

test('output identity migration needs read-only preview and explicit archive confirmation (fixture)',async({page})=>{
 const asset='00000000-0000-4000-8000-000000000090'
 const preview={assetId:asset,revision:1,planDigest:'a'.repeat(64),operations:[{path:'TV/自有剧集/Season 01/自有剧集.strm',kind:'strm',operation:'write'},{path:'Movies/自有电影/自有电影.strm',kind:'strm',operation:'archive'}],cloudFileUnchanged:true,playbackGrantUnchanged:true,sharedNfoRetained:true}
 const applied:Array<Record<string,unknown>>=[]
 await page.route('**/api/v1/media/assets?page=*',route=>route.fulfill({json:{data:{items:[{id:asset,title:'自有电影',path:'/115/owned.mkv',state:'present',revoked:false,fileScoped:true,revision:1,artifactsState:'ready'}],total:1,page:1,pageSize:50}}}))
 await page.route('**/api/v1/metadata/search?*',route=>route.fulfill({json:{data:[{id:10,mediaType:'movie',title:'自有新电影',year:'2026'}]}}))
 await page.route(`**/api/v1/media/assets/${asset}/identity-preview`,route=>route.fulfill({json:{data:preview}}))
 await page.route(`**/api/v1/media/assets/${asset}/identity`,async route=>{applied.push(route.request().postDataJSON());await route.fulfill({json:{data:{corrected:true}}})})
 await fixtureLogin(page);await navigate(page,'云盘索引')
 const row=page.locator('.organize-record').filter({hasText:'自有电影'})
 await page.locator('.media-workspace .page-tabs').getByRole('button',{name:'已入库',exact:true}).click();await row.getByRole('button',{name:'修正身份 / 季集',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'修正媒体身份与季集',exact:true})
 await dialog.getByRole('button',{name:'查询TMDB'}).click()
 await dialog.getByRole('button').filter({hasText:'自有新电影'}).click()
 await expect(dialog.getByRole('checkbox',{name:'同时迁移Movies/TV输出目录（先预览）'})).not.toBeChecked()
 await dialog.getByText('同时迁移Movies/TV输出目录（先预览）',{exact:true}).click()
 await dialog.getByRole('button',{name:'预览输出迁移',exact:true}).click()
 const confirm=page.getByRole('dialog',{name:'确认输出迁移',exact:true})
 await expect(dialog.getByText('输出迁移预览',{exact:true})).toBeVisible();await dialog.getByRole('button',{name:'核对后确认迁移',exact:true}).click();await expect(confirm).toBeVisible();expect(applied).toHaveLength(0)
 await expect(dialog.getByText('归档旧输出',{exact:true})).toBeVisible()
 await expect(dialog.getByText('Movies/自有电影/自有电影.strm',{exact:true})).toBeVisible()
 await page.screenshot({path:resolve(screenshots,'output-migration-preview.png'),fullPage:true,animations:'disabled'})
 await page.setViewportSize({width:375,height:812});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
 await page.screenshot({path:resolve(screenshots,'output-migration-preview-mobile.png'),fullPage:true,animations:'disabled'})
 await page.setViewportSize({width:1440,height:1000})
 await confirm.getByRole('button',{name:'返回检查',exact:true}).click();await dialog.getByRole('button',{name:'取消',exact:true}).click()
 expect(applied).toHaveLength(0)
 await page.locator('.media-workspace .page-tabs').getByRole('button',{name:'已入库',exact:true}).click();await row.getByRole('button',{name:'修正身份 / 季集',exact:true}).click()
 await page.getByRole('dialog',{name:'修正媒体身份与季集',exact:true}).getByRole('button',{name:'查询TMDB'}).click()
 await page.getByRole('dialog',{name:'修正媒体身份与季集',exact:true}).getByRole('button').filter({hasText:'自有新电影'}).click()
 await page.getByRole('dialog',{name:'修正媒体身份与季集',exact:true}).getByText('同时迁移Movies/TV输出目录（先预览）',{exact:true}).click()
 await page.getByRole('dialog',{name:'修正媒体身份与季集',exact:true}).getByRole('button',{name:'预览输出迁移',exact:true}).click()
 await page.getByRole('dialog',{name:'修正媒体身份与季集',exact:true}).getByRole('button',{name:'核对后确认迁移',exact:true}).click();await confirm.getByRole('button',{name:'确认生成并归档',exact:true}).click()
 await expect.poll(()=>applied.length).toBe(1)
 expect(applied[0]?.planDigest).toBe(preview.planDigest)
 expect(applied[0]?.migrateOutputs).toBe(true)
 await expect(page.getByText('确认结果需核对输出与记录，不重复提交同一确认。',{exact:true})).toBeVisible()
})

test('episode metadata preview is read-only, explicit and cleared when season coverage changes (fixture)',async({page})=>{
 const asset='00000000-0000-4000-8000-000000000095'
 let writes=0,previewQueries:string[]=[]
 await page.route('**/api/v1/media/assets?page=*',route=>route.fulfill({json:{data:{items:[{id:asset,title:'自有剧集',path:'/115/owned.mkv',state:'present',revision:1,artifactsState:'ready',fileScoped:true}],total:1,page:1,pageSize:50}}}))
 await page.route('**/api/v1/metadata/search?*',route=>route.fulfill({json:{data:[{id:99,mediaType:'series',title:'自有剧集',year:'2025'}]}}))
 await page.route('**/api/v1/metadata/episodes?*',async route=>{previewQueries.push(route.request().url());await route.fulfill({json:{data:[{id:991,seriesId:99,season:1,number:1,title:'自有单集第一集',overview:'单集简介样本',airDate:'2026-01-01',runtimeMinutes:25},{id:993,seriesId:99,season:1,number:3,title:'自有单集第三集',overview:'第三集简介样本'}]}})})
 await page.route(`**/api/v1/media/assets/${asset}/identity`,async route=>{writes++;await route.fulfill({json:{data:{corrected:true}}})})
 await fixtureLogin(page);await navigate(page,'云盘索引')
 await page.locator('.media-workspace .page-tabs').getByRole('button',{name:'已入库',exact:true}).click();await page.locator('.organize-record').filter({hasText:'自有剧集'}).getByRole('button',{name:'修正身份 / 季集',exact:true}).click()
 const dialog=page.getByRole('dialog')
 await dialog.getByRole('button',{name:'查询TMDB'}).click()
 await dialog.getByRole('button').filter({hasText:'自有剧集'}).click()
 await dialog.getByLabel('修正实际覆盖集',{exact:true}).fill('1,3')
 await dialog.getByRole('button',{name:'预览单集信息',exact:true}).click()
 await expect(dialog.getByText('S01E01 · 自有单集第一集',{exact:true})).toBeVisible()
 await expect(dialog.getByText('S01E03 · 自有单集第三集',{exact:true})).toBeVisible()
 await expect(dialog.getByText('播出日期 — · 时长 —',{exact:true})).toBeVisible()
 expect(writes).toBe(0);expect(previewQueries[0]).toContain('episodes=1,3')
 await page.screenshot({path:resolve(screenshots,'episode-metadata-preview.png'),fullPage:true,animations:'disabled'})
 await dialog.getByLabel('修正实际覆盖集',{exact:true}).fill('2')
 await expect(dialog.getByText('S01E01 · 自有单集第一集',{exact:true})).toBeHidden()
 await dialog.getByRole('button',{name:'取消',exact:true}).click()
 expect(writes).toBe(0)
})

test('expired archive cleanup is preview-only until typed explicit confirmation (fixture)',async({page})=>{
 // Public HTTP lacks crypto.randomUUID; the browser still provides getRandomValues.
 await page.addInitScript(()=>Object.defineProperty(window.crypto,'randomUUID',{value:undefined,configurable:true}))
 const archive='00000000-0000-4000-8000-000000000098'
 const entry={id:archive,assetId:'owned-fixture',originalPath:'Movies/自有样本/旧输出.strm',archivePath:'.media-tool-retired/owned/旧输出.strm.retired',kind:'strm',archivedAt:'2020-01-01T00:00:00Z',eligibleAfter:'2020-02-01T00:00:00Z',state:'retained',message:'',deletedAt:undefined as string|undefined}
 let confirmations:Array<Record<string,unknown>>=[]
 await page.route('**/api/v1/library/archive-policy',route=>route.fulfill({json:{data:{retentionDays:30,revision:1,automaticCleanup:false}}}))
 await page.route('**/api/v1/library/archives?page=*',route=>route.fulfill({json:{data:{items:[entry],total:1,page:1,pageSize:50}}}))
 await page.route('**/api/v1/library/archives/cleanup-preview',route=>route.fulfill({json:{data:{planDigest:'b'.repeat(64),policy:{retentionDays:30,revision:1,automaticCleanup:false},items:[entry],sourceFilesUntouched:true,physicalDeletion:true}}}))
 await page.route('**/api/v1/library/archives/cleanup-confirm',async route=>{confirmations.push(route.request().postDataJSON());entry.state='removed';entry.deletedAt='2026-10-10T00:00:00Z';await route.fulfill({json:{data:[{id:'owned-cleanup',archiveId:archive,state:'removed',message:''}]}})})
 await fixtureLogin(page);await navigate(page,'云盘索引')
 await page.locator('.media-workspace .page-tabs').getByRole('button',{name:'恢复与归档',exact:true}).click();await expect(page.getByRole('heading',{name:'旧输出归档',exact:false})).toBeVisible()
 const row=page.locator('.organize-record').filter({hasText:'旧输出.strm'})
 // Element Plus hides native selection checkbox: click its visible label.
 await row.getByRole('checkbox').check()
 await page.getByRole('button',{name:'预览清理所选 1',exact:true}).click()
 await page.getByRole('region',{name:'归档清理计划',exact:true}).getByRole('button',{name:'确认物理清理',exact:true}).click();const dialog=page.getByRole('dialog',{name:'确认物理清理归档',exact:true})
 await expect(dialog).toBeVisible();expect(confirmations).toHaveLength(0)
 const apply=dialog.getByRole('button',{name:'确认物理清理',exact:true});await expect(apply).toBeDisabled()
 await dialog.getByRole('button',{name:'返回检查',exact:true}).click();await expect(dialog).toBeHidden();expect(confirmations).toHaveLength(0)
 await page.getByRole('region',{name:'归档清理计划',exact:true}).getByRole('button',{name:'确认物理清理',exact:true}).click()
 await dialog.getByPlaceholder('请输入上方确认短语',{exact:true}).fill('清理归档')
 await expect(apply).toBeEnabled()
 await dialog.screenshot({path:resolve(screenshots,'archive-cleanup-confirmation.png'),animations:'disabled'})
 await page.setViewportSize({width:375,height:812});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
 await expect(apply).toBeInViewport()
 await dialog.screenshot({path:resolve(screenshots,'archive-cleanup-confirmation-mobile.png'),animations:'disabled'})
 await apply.click()
 await expect.poll(()=>confirmations.length).toBe(1)
 expect(confirmations[0]?.archiveIds).toEqual([archive]);expect(confirmations[0]?.planDigest).toBe('b'.repeat(64));expect(confirmations[0]?.requestId).toMatch(/^[a-f0-9-]{36}$/)
 await expect(page.getByText('已清理（审计保留） · —',{exact:true})).toBeVisible()
 await expect(row.getByText(/已清理（审计保留）/)).toBeVisible()
})

test('unknown archive response retains its request and needs explicit reconciliation (fixture)',async({page})=>{
 const archive='00000000-0000-4000-8000-000000000099'
 const entry={id:archive,assetId:'owned-fixture',originalPath:'Movies/自有样本/待核对.strm',archivePath:'.media-tool-retired/owned/待核对.strm.retired',kind:'strm',archivedAt:'2020-01-01T00:00:00Z',eligibleAfter:'2020-02-01T00:00:00Z',state:'retained',message:''}
 const confirmations:Array<Record<string,unknown>>=[]
 await page.route('**/api/v1/library/archive-policy',route=>route.fulfill({json:{data:{retentionDays:30,revision:1,automaticCleanup:false}}}))
 await page.route('**/api/v1/library/archives?page=*',route=>route.fulfill({json:{data:{items:[entry],total:1,page:1,pageSize:50}}}))
 await page.route('**/api/v1/library/archives/cleanup-preview',route=>route.fulfill({json:{data:{planDigest:'c'.repeat(64),policy:{retentionDays:30,revision:1,automaticCleanup:false},items:[entry],sourceFilesUntouched:true,physicalDeletion:true}}}))
 await page.route('**/api/v1/library/archives/cleanup-confirm',async route=>{confirmations.push(route.request().postDataJSON());await route.fulfill({json:{data:confirmations.length===1?[]:[{id:'owned-cleanup',archiveId:archive,state:'conflict',message:'needs manual recovery'}]}})})
 await page.route('**/api/v1/library/archive-cleanups/*',route=>route.fulfill({json:{data:[]}}))
 await fixtureLogin(page);await navigate(page,'云盘索引')
 await page.locator('.media-workspace .page-tabs').getByRole('button',{name:'恢复与归档',exact:true}).click();await page.locator('.organize-record').filter({hasText:'待核对.strm'}).getByRole('checkbox').check()
 await page.getByRole('button',{name:'预览清理所选 1',exact:true}).click()
 await page.getByRole('region',{name:'归档清理计划',exact:true}).getByRole('button',{name:'确认物理清理',exact:true}).click();const dialog=page.getByRole('dialog',{name:'确认物理清理归档',exact:true})
 await dialog.getByPlaceholder('请输入上方确认短语',{exact:true}).fill('清理归档')
 const apply=dialog.getByRole('button',{name:'确认物理清理',exact:true})
 await apply.click()
 await expect(page.getByText('清理结果不完整，请核对已有请求结果；不换请求重复提交。',{exact:true})).toBeVisible()
 await expect(dialog).toBeHidden();expect(confirmations).toHaveLength(1)
 await page.getByRole('button',{name:'核对已有请求结果',exact:true}).click()
 await expect(page.getByText('尚未确认批准记录，只能保留同一请求和计划再次明确确认。',{exact:true})).toBeVisible()
 await expect(dialog).toBeHidden();expect(confirmations).toHaveLength(1)
 // A second submit is a new explicit click, never a background retry or a new ID.
 await page.getByRole('button',{name:'使用原请求继续确认',exact:true}).click();await dialog.getByPlaceholder('请输入上方确认短语').fill('清理归档');await apply.click()
 await expect.poll(()=>confirmations.length).toBe(2)
 expect(confirmations[1]?.requestId).toBe(confirmations[0]?.requestId)
 expect(confirmations[1]?.planDigest).toBe(confirmations[0]?.planDigest)
 await expect(page.getByText('冲突 · needs manual recovery',{exact:true})).toBeVisible()
 await expect(page.getByText('已清理（审计保留） · —',{exact:true})).toHaveCount(0)
})

test('sparse episode migration previews explicit aliases and whole-file playback (fixture)',async({page})=>{
 const asset='00000000-0000-4000-8000-000000000100'
 const preview={assetId:asset,revision:1,planDigest:'d'.repeat(64),outputProjectionMode:'explicit_episode_aliases',wholeFilePlayback:true,declaredEpisodes:[1,3],operations:[{path:'TV/自有剧集/Season 01/自有剧集 - S01E01.strm',kind:'strm',operation:'write'},{path:'TV/自有剧集/Season 01/自有剧集 - S01E03.strm',kind:'strm',operation:'write'},{path:'TV/自有剧集/Season 01/自有剧集 - S01E01E03.strm',kind:'strm',operation:'archive'}],cloudFileUnchanged:true,playbackGrantUnchanged:true,sharedNfoRetained:true}
 const applied:Array<Record<string,unknown>>=[]
 await page.route('**/api/v1/media/assets?page=*',route=>route.fulfill({json:{data:{items:[{id:asset,title:'自有非连续合集',path:'/115/owned.mkv',state:'present',revision:1,artifactsState:'ready',fileScoped:true}],total:1,page:1,pageSize:50}}}))
 await page.route('**/api/v1/metadata/search?*',route=>route.fulfill({json:{data:[{id:99,mediaType:'series',title:'自有剧集',year:'2025'}]}}))
 await page.route(`**/api/v1/media/assets/${asset}/identity-preview`,route=>route.fulfill({json:{data:preview}}))
 await page.route(`**/api/v1/media/assets/${asset}/identity`,async route=>{applied.push(route.request().postDataJSON());await route.fulfill({json:{data:{corrected:true}}})})
 await fixtureLogin(page);await navigate(page,'云盘索引')
 await page.locator('.media-workspace .page-tabs').getByRole('button',{name:'已入库',exact:true}).click();await page.locator('.organize-record').filter({hasText:'自有非连续合集'}).getByRole('button',{name:'修正身份 / 季集',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'修正媒体身份与季集',exact:true})
 await dialog.getByRole('button',{name:'查询TMDB'}).click();await dialog.getByRole('button').filter({hasText:'自有剧集'}).click()
 await dialog.getByLabel('修正实际覆盖集',{exact:true}).fill('1,3')
 await expect(dialog.getByText(/每个条目播放同一个完整源文件，不切片、不按单集跳转/)).toBeVisible()
 await dialog.getByText('同时迁移Movies/TV输出目录（先预览）',{exact:true}).click()
 await dialog.getByRole('button',{name:'预览输出迁移',exact:true}).click()
 const confirm=page.getByRole('dialog',{name:'确认输出迁移',exact:true})
 await expect(dialog.getByText(/每个条目都播放同一个完整源文件/)).toBeVisible()
 await expect(dialog.getByText('TV/自有剧集/Season 01/自有剧集 - S01E01.strm',{exact:true})).toBeVisible()
 await expect(dialog.getByText('TV/自有剧集/Season 01/自有剧集 - S01E03.strm',{exact:true})).toBeVisible()
 expect(applied).toHaveLength(0)
 await dialog.getByRole('button',{name:'核对后确认迁移',exact:true}).click()
 await confirm.screenshot({path:resolve(screenshots,'emby-sparse-projection-preview.png'),animations:'disabled'})
 await page.setViewportSize({width:375,height:812})
 await expect(confirm.getByRole('button',{name:'确认生成并归档',exact:true})).toBeInViewport({ratio:1})
 await confirm.screenshot({path:resolve(screenshots,'emby-sparse-projection-preview-mobile.png'),animations:'disabled'})
 await confirm.getByRole('button',{name:'返回检查',exact:true}).click();expect(applied).toHaveLength(0)
})

test('blocked legacy imports require read-only review before import-only approval (fixture)',async({page})=>{
 const transfer='00000000-0000-4000-8000-000000000110',asset='00000000-0000-4000-8000-000000000111'
 let migrated=false,approvals:Array<Record<string,unknown>>=[]
 await page.route('**/api/v1/downloads',route=>route.fulfill({json:{data:[]}}))
 await page.route('**/api/v1/transfers',route=>route.fulfill({json:{data:[{id:transfer,assetId:asset,taskId:'owned-task',destination:'/115/Owned.S01E01E03.mkv',state:'needs_output_reconciliation',phase:'output_layout',bytes:5,total:5,message:'需明确核对输出布局',controlRevision:1}]}}))
 await page.route(`**/api/v1/transfers/${transfer}/output-preview`,route=>route.fulfill({json:{data:{transferId:transfer,assetId:asset,controlRevision:1,assetRevision:migrated?2:1,state:'needs_output_reconciliation',existingPaths:migrated?['TV/Owned/S01E01.strm','TV/Owned/S01E03.strm']:['TV/Owned/S01E01E03.strm'],expectedPaths:['TV/Owned/S01E01.strm','TV/Owned/S01E03.strm'],needsMigration:!migrated,needsArtifactRecovery:false,ready:migrated,planDigest:migrated?'e'.repeat(64):'',message:migrated?'现有输出布局一致，可恢复入库':'旧布局会补出未声明集号，先明确迁移',noUploadRecreation:true,sourceFilesPreserved:true}}}))
 await page.route(`**/api/v1/transfers/${transfer}/resume-import`,async route=>{approvals.push(route.request().postDataJSON());await route.fulfill({json:{data:{approved:true,uploadRestarted:false}}})})
 await fixtureLogin(page);await navigate(page,'下载与传输')
 const row=page.locator('.acquire-task').filter({hasText:'Owned.S01E01E03.mkv'})
 await expect(row.getByRole('button',{name:'恢复',exact:true})).toHaveCount(0)
 await row.getByRole('link',{name:'核对入库恢复',exact:true}).click();await page.getByRole('button',{name:'核对入库恢复',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'核对并恢复已有入库',exact:true})
 const resume=dialog.getByRole('button',{name:'明确恢复入库（不重新上传）',exact:true})
 await expect(resume).toBeDisabled();expect(approvals).toHaveLength(0)
 const link=dialog.getByRole('link',{name:'前往云盘索引核对该资产、恢复已有输出并明确预览迁移',exact:true})
 await expect(link).toHaveAttribute('href',`/organize?tab=assets&assetId=${asset}`)
 await dialog.getByRole('button',{name:'关闭',exact:true}).click();expect(approvals).toHaveLength(0)
 migrated=true;await page.goto('/downloads?tab=uploads');await row.getByRole('link',{name:'核对入库恢复',exact:true}).click();await page.getByRole('button',{name:'核对入库恢复',exact:true}).click()
 await expect(resume).toBeEnabled()
 await dialog.screenshot({path:resolve(screenshots,'output-import-recovery-preview.png'),animations:'disabled'})
 await page.setViewportSize({width:375,height:812});await expect(resume).toBeInViewport({ratio:1})
 await dialog.screenshot({path:resolve(screenshots,'output-import-recovery-preview-mobile.png'),animations:'disabled'})
 await resume.click();await expect.poll(()=>approvals.length).toBe(1)
 expect(approvals[0]).toEqual({controlRevision:1,planDigest:'e'.repeat(64)})
 await expect(page.getByText('入库恢复已受理；不重新上传，请核对后续状态。',{exact:true})).toBeVisible()
})

test('download binds the selected search snapshot and announces refresh errors (fixture)',async({page})=>{
 await fixtureLogin(page)
 const runId='11111111-1111-4111-8111-111111111111',resourceId='22222222-2222-4222-8222-222222222222',accountId='33333333-3333-4333-8333-333333333333'
 await page.route('**/api/v1/integrations',route=>route.fulfill({json:{data:[{id:accountId,kind:'ptsite',name:'受控站点样本',enabled:true}]}}))
 await page.route('**/api/v1/search',route=>route.request().method()==='POST'?route.fulfill({json:{data:{id:runId}}}):route.fulfill({json:{data:[]}}))
 await page.route(`**/api/v1/search/${runId}?*`,route=>route.fulfill({json:{data:{id:runId,keyword:'Owned Movie',state:'succeeded',sites:[{accountId,name:'受控站点样本',state:'succeeded',message:'',count:1}],resources:[{id:resourceId,title:'Owned Movie (2026)',siteName:'受控站点样本',size:1024,seeders:1,leechers:0,downloadFactor:0,metadata:{}}],total:1}}}))
 await page.route('**/api/v1/metadata/search?*',route=>route.fulfill({json:{data:[{id:1,mediaType:'movie',title:'Owned Movie',originalTitle:'Owned Movie',year:'2026'}]}}))
 let submitted:Record<string,unknown>|undefined
 await page.route('**/api/v1/downloads',async route=>{submitted=route.request().postDataJSON();await route.fulfill({status:409,json:{error:{code:'search_refresh_required',message:'搜索快照已过期，请重新搜索或重新评估订阅'}}})})
 await page.goto('/search')
 await page.getByText('站点直搜',{exact:true}).click()
 await page.getByLabel('资源关键词',{exact:true}).fill('Owned Movie')
 await page.getByRole('button',{name:'开始搜索',exact:true}).click()
 await page.getByRole('button',{name:'确认并下载',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'确认媒体身份与下载'})
 await expect(dialog.getByText('下载使用打开弹窗时选中的搜索快照；过期或站点配置变化时请重新搜索。',{exact:true})).toBeVisible()
 await dialog.getByRole('button',{name:'查询 TMDB',exact:true}).click()
 await dialog.getByRole('button',{name:'Owned Movie · 2026 · 电影',exact:true}).click()
 await dialog.getByRole('button',{name:'创建下载任务',exact:true}).click()
 await expect.poll(()=>submitted?.searchRunId).toBe(runId)
 expect(submitted?.resourceId).toBe(resourceId)
 await expect(dialog.getByRole('alert').filter({hasText:'搜索快照已过期'})).toBeVisible()
 await expect(dialog).toBeVisible()
 mkdirSync(screenshots,{recursive:true})
 await page.screenshot({path:resolve(screenshots,'search-snapshot-refresh-desktop.png'),fullPage:true})
 await page.setViewportSize({width:375,height:812})
 await expect(dialog.getByRole('button',{name:'创建下载任务',exact:true})).toBeVisible()
 await page.screenshot({path:resolve(screenshots,'search-snapshot-refresh-mobile.png'),fullPage:true})
})

test('source recovery needs preview and explicit same-hash confirmation without generic retry (fixture)',async({page})=>{
 await fixtureLogin(page)
 const taskId='11111111-1111-4111-8111-111111111111',runId='22222222-2222-4222-8222-222222222222',hash='a'.repeat(40)
 await page.route('**/api/v1/downloads',route=>route.fulfill({json:{data:[{id:taskId,resourceId:'fixture-resource',title:'Owned Movie (2026)',state:'pending',seedingState:'pending',progress:0,infoHash:'',message:'来源快照已过期，需核对',sourceRecoveryRequired:true,sourceControlRevision:2,media:{id:1,title:'Owned Movie',year:'2026',mediaType:'movie'}}]}}))
 await page.route('**/api/v1/transfers',route=>route.fulfill({json:{data:[]}}))
 let previews=0,approved:Record<string,unknown>|undefined,genericRetries=0
 await page.route(`**/api/v1/downloads/${taskId}/source-preview*`,route=>{previews++;return route.fulfill({json:{data:{taskId,resourceId:'fixture-resource',searchRunId:runId,currentSearchRunId:'old-fixture',controlRevision:2,planDigest:'b'.repeat(64),mode:'same_identity',originalHash:hash,candidateHash:hash,candidateTitle:'Owned Movie (2026)',downloaderObservation:'absent',eligible:true,reasons:[]}}})})
 await page.route(`**/api/v1/downloads/${taskId}/reconfirm-source`,route=>{approved=route.request().postDataJSON();return route.fulfill({status:202,json:{data:{taskId,controlRevision:3,alreadyApplied:false,recoveryStillRequired:false}}})})
 await page.route(`**/api/v1/downloads/${taskId}/retry`,route=>{genericRetries++;return route.fulfill({status:409,json:{error:{code:'source_reconfirm_required',message:'必须核对来源'}}})})
 await page.goto('/downloads')
 await page.getByRole('button',{name:'核对来源',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'重新确认下载来源'})
 await test.step('observe source hash preview',async()=>{await expect(dialog.getByText(hash,{exact:true})).toHaveCount(2)})
 const confirm=dialog.getByRole('button',{name:'明确确认并恢复原任务',exact:true})
 await expect(confirm).toBeDisabled()
 expect(approved).toBeUndefined()
 await test.step('check the explicit source binding confirmation',async()=>{await dialog.getByRole('checkbox',{name:'我确认原媒体与任务不变，并批准使用此新来源',exact:true}).check()})
 await expect(confirm).toBeEnabled()
 mkdirSync(screenshots,{recursive:true})
 await page.screenshot({path:resolve(screenshots,'download-source-recovery-desktop.png'),fullPage:true})
 await page.setViewportSize({width:375,height:812})
 await expect(confirm).toBeInViewport()
 await page.screenshot({path:resolve(screenshots,'download-source-recovery-mobile.png'),fullPage:true})
 await confirm.click()
 await expect.poll(()=>approved?.searchRunId).toBe(runId)
 expect(approved?.controlRevision).toBe(2)
 expect(approved?.confirmSourceBinding).toBe(true)
 expect(genericRetries).toBe(0)
 expect(previews).toBeGreaterThan(0)
})

test('partial provider pagination remains visible and does not claim no more resources (fixture)',async({page})=>{
 await fixtureLogin(page)
 const runId='11111111-1111-4111-8111-111111111111',accountId='22222222-2222-4222-8222-222222222222'
 await page.route('**/api/v1/integrations',route=>route.fulfill({json:{data:[{id:accountId,kind:'ptsite',name:'受控分页样本',enabled:true}]}}))
 await page.route('**/api/v1/search',route=>route.request().method()==='POST'?route.fulfill({json:{data:{id:runId}}}):route.fulfill({json:{data:[]}}))
 await page.route(`**/api/v1/search/${runId}?*`,route=>route.fulfill({json:{data:{id:runId,keyword:'Owned',state:'partial',sites:[{accountId,name:'受控分页样本',state:'partial',message:'达到5页查询上限；仅显示已取得的部分结果，不代表站点无更多资源',count:1}],resources:[{id:'fixture-resource',title:'Owned Movie (2026)',siteName:'受控分页样本',size:100,seeders:5,leechers:0,downloadFactor:null,metadata:{}}],total:1}}}))
 await page.goto('/search')
 await page.getByText('站点直搜',{exact:true}).click()
 await page.getByLabel('资源关键词',{exact:true}).fill('Owned')
 await page.getByRole('button',{name:'开始搜索',exact:true}).click()
 await page.getByRole('button',{name:/站点查询状态 · 成功/}).click();await expect(page.locator('.f3-site-details')).toContainText('部分结果');await expect(page.locator('.f3-site-details')).toContainText('达到5页查询上限')
 await expect(page.getByText('已取得部分结果；失败站点不代表没有资源。',{exact:true})).toBeVisible()
 await expect(page.getByRole('button',{name:'确认并下载',exact:true})).toBeVisible()
 mkdirSync(screenshots,{recursive:true})
 await page.screenshot({path:resolve(screenshots,'search-provider-partial-desktop.png'),fullPage:true})
 await page.emulateMedia({reducedMotion:'reduce'})
 await page.setViewportSize({width:375,height:812})
 await expect(page.getByRole('button',{name:'打开导航',exact:true})).toBeVisible()
 await expect.poll(()=>page.locator('.workbench-sidebar').evaluate(el=>el.getBoundingClientRect().right)).toBeLessThanOrEqual(1)
 await expect(page.getByText('已取得部分结果；失败站点不代表没有资源。',{exact:true})).toBeVisible()
 await page.screenshot({path:resolve(screenshots,'search-provider-partial-mobile.png'),fullPage:true})
})

test('PT account form uses template identity, UA, echo and disallows duplicates (fixture)',async({page})=>{
 const catalog={templates:[{id:'hdsky',name:'HDSKY',domain:'https://hdsky.me',loaded:true},{id:'ourbits',name:'OURBITS',domain:'https://ourbits.club',loaded:true}]};await page.route('**/api/v1/pt/templates',r=>r.fulfill({json:{data:catalog}}));const accounts=[{id:'owned-pt',definitionId:'hdsky',name:'HDSKY',revision:1,settings:{userAgent:'Owned-UA',rssState:'ready'},values:{cookie:'owned-cookie-e2e',passkey:'owned-mobile-passkey-'.repeat(8),rssUrl:'https://hdsky.me/rss?passkey=owned'}}];let saved:any
 await page.route('**/api/v1/integrations',r=>r.fulfill({json:{data:[{...accounts[0],kind:'ptsite',enabled:true,settings:{...accounts[0].settings,definitionId:'hdsky'},values:undefined,secretFields:['cookie','passkey','rssUrl']}]}}));await page.route('**/api/v1/pt/accounts',r=>{if(r.request().method()==='POST'){saved=r.request().postDataJSON();return r.fulfill({json:{data:{saved:true,rssState:'ready'}}})}return r.fulfill({json:{data:accounts}})});await page.route('**/api/v1/integrations/owned-pt/secrets/reveal',r=>{const key=r.request().postDataJSON().fields[0] as keyof typeof accounts[0]['values'];return r.fulfill({json:{data:{revision:1,values:{[key]:accounts[0].values[key]}}}})});await fixtureLogin(page);await page.goto('/sites');await page.getByRole('button',{name:'编辑账号',exact:true}).click();const edit=page.getByRole('dialog',{name:'编辑站点账号',exact:true});await expect(edit.getByLabel('站点 Cookie',{exact:true})).toHaveValue('');await expect(edit.getByLabel('站点 User-Agent',{exact:true})).toHaveValue('Owned-UA');await expect(edit.getByLabel('站点 RSS 地址',{exact:true})).toHaveValue('');await edit.getByRole('button',{name:'显示订阅密钥',exact:true}).click();await expect(edit.getByLabel('订阅密钥已保存值',{exact:true})).toHaveValue('owned-mobile-passkey-'.repeat(8));await page.setViewportSize({width:390,height:844});expect(await edit.evaluate(el=>el.scrollWidth<=el.clientWidth+1)).toBe(true);await page.screenshot({path:resolve(screenshots,'pt-passkey-mobile.png')});await edit.getByRole('button',{name:'取消',exact:true}).click()
 await page.getByRole('button',{name:'添加站点账号',exact:true}).click();const d=page.getByRole('dialog',{name:'添加站点账号',exact:true});const select=d.getByRole('combobox',{name:'站点模板',exact:true});await expect(select.locator('option[value=hdsky]')).toHaveJSProperty('disabled',true);await select.selectOption('ourbits');await expect(d).toContainText('OURBITS · https://ourbits.club');await d.getByLabel('站点 Cookie',{exact:true}).fill('owned-new-cookie');await d.getByLabel('站点 User-Agent',{exact:true}).fill('Owned-new-UA');await d.getByRole('button',{name:'验证并保存',exact:true}).click();await expect.poll(()=>saved?.definitionId).toBe('ourbits');expect(saved.values).toEqual({cookie:'owned-new-cookie',userAgent:'Owned-new-UA'});expect(saved.userId).toBe('');expect(saved.rediscover).toBe(false);await page.screenshot({path:resolve(screenshots,'pt-account-template-form.png'),fullPage:true})
})

test('separate 115 setup saves disabled without tokens and saved secrets never echo (isolated)',async({page})=>{
 await fixtureLogin(page)
 await page.goto('/cloud-connections');await page.locator('.el-select').filter({has:page.getByRole('combobox',{name:'新增服务类型',exact:true})}).click();await page.getByRole('option',{name:'115 授权',exact:true}).click()
 await expect(page.getByRole('combobox',{name:'新增服务类型',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'添加连接',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'添加连接',exact:true})
 const name='115 tokenless fixture '+Date.now()
 await dialog.getByLabel('连接名称',{exact:true}).fill(name)
 await dialog.getByLabel('自己的115开放平台 AppID',{exact:true}).fill('123456')
 await expect(dialog.getByText('启用为主连接',{exact:true})).toHaveCount(0)
 await expect(dialog.getByLabel('Access Token（可选手动录入）',{exact:true})).toHaveValue('')
 const saved=page.waitForResponse(r=>r.url().endsWith('/api/v1/integrations')&&r.request().method()==='POST')
 await dialog.getByRole('button',{name:'加密保存',exact:true}).click()
 const response=await saved;expect(response.status()).toBe(201);const draft=(await response.json()).data
 expect(draft.enabled).toBe(false);expect(draft.secretFields).toEqual([])
 const draftRow=page.locator('.saved-connection').filter({hasText:name});await expect(draftRow.getByRole('button',{name:'115扫码授权',exact:true})).toBeVisible()
 await page.goto('/mediaLibrary');await expect(page.getByRole('link',{name:'Emby 配置',exact:true})).toHaveCount(0);await page.goto('/connections?tab=emby')
 await page.getByRole('button',{name:'添加连接',exact:true}).click()
 const embyName='Emby echo fixture '+Date.now(),value='PRIVATE-EMBY-ECHO-FIXTURE'
 await dialog.getByLabel('连接名称',{exact:true}).fill(embyName)
 await dialog.getByLabel('Emby 服务地址',{exact:true}).fill(process.env.E2E_EMBY_ENDPOINT!)
 await dialog.getByLabel('媒体库刷新 API Key',{exact:true}).fill(value)
 await dialog.getByRole('button',{name:'加密保存',exact:true}).click()
 const row=page.locator('.saved-connection').filter({hasText:embyName});await row.getByRole('button',{name:'编辑',exact:true}).click()
 const edit=page.getByRole('dialog',{name:'编辑连接',exact:true}),field=edit.getByLabel('媒体库刷新 API Key',{exact:true})
 await expect(field).toHaveValue('');await expect(field).toHaveAttribute('type','password');await expect(edit).toContainText('已设置（默认隐藏）')
 await edit.screenshot({path:resolve(screenshots,'service-secrets-explicit-desktop.png'),animations:'disabled',mask:[field]})
 await page.setViewportSize({width:375,height:812});await field.scrollIntoViewIfNeeded()
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
 await edit.screenshot({path:resolve(screenshots,'service-secrets-explicit-mobile.png'),animations:'disabled',mask:[field]})
 const unchanged=page.waitForRequest(r=>r.url().includes('/api/v1/integrations/')&&r.method()==='PUT')
 await edit.getByRole('button',{name:'加密保存',exact:true}).click()
 expect((await unchanged).postDataJSON().secrets).toEqual({})
 // No real credentials, media changes or external service calls occur here.
})

test('local file management previews organize, keeps sidecars and uses reversible trash',async({page})=>{
 const directory=process.env.E2E_FILE_ROOT;if(!directory)throw new Error('Owned E2E file root required')
 await fixtureLogin(page);await page.goto('/files')
 const menu=async(name:string,action:string)=>{const row=page.locator('.files-entry').filter({hasText:name});await row.getByRole('button',{name:name+' 更多',exact:true}).click();await page.getByRole('menuitem',{name:action,exact:true}).click()}
 await test.step('打开影视整理',()=>menu('Owned Film.mkv','影视整理'));const edit=page.getByRole('dialog',{name:'影视分类整理',exact:true})
 await expect(edit.getByRole('button',{name:'预览变更',exact:true})).toBeEnabled();await test.step('选择电影类型',async()=>{await edit.locator('.el-select').filter({has:page.getByRole('combobox',{name:'媒体类型',exact:true})}).click();await page.getByRole('option',{name:'电影',exact:true}).click()});await edit.getByLabel('媒体标题',{exact:true}).fill('Fixture Film');await edit.getByLabel('媒体年份',{exact:true}).fill('2026')
 await edit.locator('.el-select').filter({has:page.getByRole('combobox',{name:'命名方式',exact:true})}).click();await page.getByRole('option',{name:'直接输入完整名称',exact:true}).click();await edit.getByLabel('完整名称',{exact:true}).fill('自定义片名.mkv')
 const response=page.waitForResponse(r=>r.url().endsWith('/api/v1/files/preview'));await edit.getByRole('button',{name:'预览变更',exact:true}).click();expect((await response).status()).toBe(200)
 await expect(edit.locator('.organize-record')).toHaveCount(2);await expect(edit).toContainText('电影/Fixture Film (2026)/自定义片名.mkv');await expect(edit).toContainText('电影/Fixture Film (2026)/自定义片名.zh-CN.srt');await edit.screenshot({path:resolve(screenshots,'file-organize-preview-desktop.png'),animations:'disabled'})
 await page.setViewportSize({width:375,height:812});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await edit.getByRole('button',{name:'核对后执行',exact:true}).click();let confirm=page.getByRole('dialog',{name:'确认文件变更',exact:true});await expect(confirm.getByRole('button',{name:'确认执行变更',exact:true})).toBeInViewport();await confirm.screenshot({path:resolve(screenshots,'file-organize-preview-mobile.png'),animations:'disabled'});await confirm.getByRole('button',{name:'确认执行变更',exact:true}).click();await expect(confirm).toBeHidden();await edit.getByRole('button',{name:'核对结果（不重放）',exact:true}).click();await expect(edit).toContainText('已完成');await edit.getByRole('button',{name:'关闭',exact:true}).click()
 await page.setViewportSize({width:1440,height:1000});await page.getByRole('button',{name:'电影',exact:true}).click();await page.getByRole('button',{name:'Fixture Film (2026)',exact:true}).click();await expect(page.locator('.files-entry').filter({hasText:'自定义片名.mkv'})).toBeVisible();await expect(page.locator('.files-entry').filter({hasText:'自定义片名.zh-CN.srt'})).toBeVisible();await page.screenshot({path:resolve(screenshots,'files-managed-desktop.png'),animations:'disabled',fullPage:true})
 await page.setViewportSize({width:375,height:812});await menu('自定义片名.mkv','重命名');const naming=page.getByRole('dialog',{name:'重命名',exact:true});await expect(naming.getByRole('button',{name:'预览变更',exact:true})).toBeEnabled();await naming.locator('.el-select').filter({has:page.getByRole('combobox',{name:'命名方式',exact:true})}).click();await page.getByRole('option',{name:'自定义模板',exact:true}).click();await naming.getByLabel('命名模板',{exact:true}).fill('{original} - {resolution}{ext}');await naming.getByLabel('分辨率',{exact:true}).fill('1080p');await naming.getByRole('button',{name:'预览变更',exact:true}).click();await expect(naming.locator('.organize-record')).toHaveCount(2);await expect(naming).toContainText('电影/Fixture Film (2026)/自定义片名 - 1080p.mkv');await naming.getByRole('button',{name:'关闭',exact:true}).click();await expect(naming).toBeHidden()
 await menu('自定义片名.mkv','删除（可恢复）');const trash=page.getByRole('dialog',{name:'删除（可恢复）',exact:true});await trash.getByRole('button',{name:'预览变更',exact:true}).click();await trash.getByRole('button',{name:'核对后执行',exact:true}).click();confirm=page.getByRole('dialog',{name:'确认移入回收目录',exact:true});await expect(confirm).toContainText('不永久删除');await confirm.getByRole('button',{name:'确认移入回收目录',exact:true}).click();await expect(confirm).toBeHidden();await trash.getByRole('button',{name:'核对结果（不重放）',exact:true}).click();await trash.getByRole('button',{name:'关闭',exact:true}).click();await expect(page.locator('.files-entry').filter({hasText:'自定义片名.mkv'})).toHaveCount(0)
 await page.getByRole('button',{name:'查看回收目录',exact:true}).click();await page.locator('.files-entry-copy button').filter({hasText:/^[0-9a-f-]{36}$/}).first().click();await menu('自定义片名.mkv','移动');const move=page.getByRole('dialog',{name:'移动文件 / 目录',exact:true});await move.getByLabel('目标相对目录',{exact:true}).fill('.');await move.getByRole('button',{name:'预览变更',exact:true}).click();await move.getByRole('button',{name:'核对后执行',exact:true}).click();await page.getByRole('dialog',{name:'确认文件变更',exact:true}).getByRole('button',{name:'确认执行变更',exact:true}).click();await expect(page.getByRole('dialog',{name:'确认文件变更',exact:true})).toBeHidden();await move.getByRole('button',{name:'核对结果（不重放）',exact:true}).click();await move.getByRole('button',{name:'关闭',exact:true}).click();await page.getByRole('button',{name:'根目录',exact:true}).click();await expect(page.locator('.files-entry').filter({hasText:'自定义片名.mkv'})).toBeVisible()
})

test('media catalog browses managed and Emby content without exposing playback credentials (fixture)',async({page})=>{
 await page.route('**/api/v1/integrations',r=>r.fulfill({json:{data:[{kind:'emby',enabled:true}]}}));await fixtureLogin(page)
 await page.route('**/api/v1/media/library?**',route=>{const query=new URL(route.request().url()).searchParams;const source=query.get('source')??'managed';return route.fulfill({status:200,contentType:'application/json',body:JSON.stringify({data:{items:[{id:'owned-catalog',source,title:'自有影片验证样本',year:'2026',mediaType:'movie',overview:'用于界面验收的明确测试内容，不是实际影片入库记录。',poster:'',state:'present',filePath:'/115/Owned/Owned.mkv'}],total:1,page:1,pageSize:24,source}})})})
 await page.goto('/mediaLibrary')
 await expect(page.getByRole('heading',{name:'媒体库',exact:true})).toBeVisible()
 await page.getByRole('navigation',{name:'媒体库来源',exact:true}).getByRole('button',{name:'内置媒体库',exact:true}).click()
 await page.getByRole('button',{name:/自有影片验证样本/}).click()
 const detail=page.getByRole('dialog',{name:'自有影片验证样本',exact:true});await expect(detail.getByText('用于界面验收的明确测试内容，不是实际影片入库记录。',{exact:true})).toBeVisible()
 await detail.getByRole('button',{name:'关闭',exact:true}).click()
 await expect(detail).not.toBeVisible()
 await page.screenshot({path:resolve(screenshots,'media-catalog-desktop.png'),animations:'disabled'})
 await page.setViewportSize({width:375,height:812});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
 await page.screenshot({path:resolve(screenshots,'media-catalog-mobile.png'),animations:'disabled'})
 await page.getByRole('navigation',{name:'媒体库来源',exact:true}).getByRole('button',{name:'Emby',exact:true}).click();await page.getByRole('button',{name:'全部媒体',exact:true}).click()
 await expect(page.getByRole('navigation',{name:'媒体库来源',exact:true}).getByRole('button',{name:'Emby',exact:true})).toHaveAttribute('aria-pressed','true')
 await expect(page.locator('.catalog-poster')).toHaveCount(1)
})


test('connections own external configuration while settings own naming',async({page})=>{
 await login(page);await page.goto('/integrations');await expect(page.getByRole('heading',{name:'连接',exact:true})).toBeVisible();await expect(page).toHaveURL(/tab=storage/);await expect(page.getByRole('button',{name:'添加连接',exact:true})).toHaveCount(1)
 await page.goto('/settings?tab=library');await expect(page.getByLabel('电影命名模板',{exact:true})).toHaveValue('{title} ({year})/{title} ({year}) - {videoFormat}');await expect(page.getByLabel('剧集命名模板',{exact:true})).toBeVisible();await page.goto('/settings?section=tmdb');await expect(page.getByRole('heading',{name:'TMDB配置',exact:true})).toBeVisible();await page.goto('/settings?section=automation');await page.locator('.settings-workspace > .page-tabs').getByRole('button',{name:'搜索订阅',exact:true}).click();await expect(page.getByRole('button',{name:'生成 / 更换恢复码',exact:true})).toHaveCount(0)
 const ttl=page.locator('#pt-cache-seconds');const previousTTL=Number(await ttl.inputValue());await ttl.fill(String(previousTTL===600?601:600));await page.getByRole('button',{name:'保存搜索订阅设置',exact:true}).click();await expect(page.getByText('当前分区已保存。',{exact:true})).toBeVisible()
 await page.screenshot({path:resolve(screenshots,'workbench-settings-desktop.png'),animations:'disabled',fullPage:true});await page.setViewportSize({width:375,height:812});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,'workbench-settings-mobile.png'),animations:'disabled',fullPage:false})
})

test('log viewer has level, sizes and confirmed cleanup; data tools are formatted read-only',async({page})=>{
 await login(page);await page.goto('/logs');await expect(page.getByRole('heading',{name:'程序日志',exact:true})).toBeVisible();await page.getByRole('button',{name:'日志设置',exact:true}).click();await expect(page.getByRole('heading',{name:'日志大小',exact:true})).toBeVisible();await page.getByRole('dialog',{name:'日志设置',exact:true}).getByRole('button',{name:'关闭',exact:true}).click();await expect(page.locator('.u6-log').first()).toBeVisible();await page.getByRole('button',{name:'清理日志',exact:true}).click();await page.getByRole('button',{name:'取消',exact:true}).click();await expect(page.locator('.u6-log').first()).toBeVisible();await page.screenshot({path:resolve(screenshots,'workbench-logs-desktop.png'),animations:'disabled'})
 await page.goto('/data');await expect(page.getByRole('heading',{name:'只读数据浏览',exact:true})).toBeVisible();await expect(page.locator('.u6-data-row').first()).toBeVisible();await page.locator('.u6-data-row').first().click();await expect(page.getByRole('dialog',{name:'只读格式化数据'})).toBeVisible();await page.getByRole('button',{name:'关闭',exact:true}).click();await page.screenshot({path:resolve(screenshots,'workbench-data-desktop.png'),animations:'disabled'});await page.setViewportSize({width:375,height:812});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,'workbench-data-mobile.png'),animations:'disabled'})
})

test('Emby home follows real category/watch/latest structure without exposing keys (fixture)',async({page})=>{
 await page.route('**/api/v1/integrations',r=>r.fulfill({json:{data:[{kind:'emby',enabled:true}]}}));await fixtureLogin(page);const sample={id:'owned1',title:'自有分类验证样本',type:'CollectionFolder',image:'',progress:30}
 await page.route('**/api/v1/media/library/emby/home?*',route=>route.fulfill({json:{data:{users:[{id:'u1',name:'自有用户'}],userId:'u1',libraries:[sample],playing:[],resume:[{...sample,type:'Movie',title:'自有继续观看样本'}],recent:[],latest:[{...sample,type:'Episode',title:'自有最新入库样本',subtitle:'第1集'}]}}}))
 await page.goto('/library?source=emby');for(const name of ['Emby 媒体库','媒体库分类','正在播放','继续观看','近期观看','最新入库'])await expect(page.getByRole('heading',{name:new RegExp('^'+name+'(?: Emby)?$')})).toBeVisible();await page.screenshot({path:resolve(screenshots,'workbench-emby-home-desktop.png'),animations:'disabled',fullPage:true});await page.setViewportSize({width:375,height:812});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,'workbench-emby-home-mobile.png'),animations:'disabled',fullPage:false})
})

test('TMDB and site-direct search have distinct cache controls (fixture)',async({page})=>{
 await fixtureLogin(page);let calls=0
 await page.route('**/api/v1/search/media?*',route=>{calls++;return route.fulfill({json:{data:{items:[{id:1,mediaType:'movie',title:'自有搜索样本',originalTitle:'Owned Movie',year:'2026',overview:'隔离的搜索界面样本',posterPath:''}],cached:calls>1,cacheSeconds:60}}})})
 await page.goto('/search');await page.getByLabel('TMDB媒体名称',{exact:true}).fill('Owned Movie');await page.getByRole('button',{name:'搜索媒体',exact:true}).click();await expect(page.getByRole('heading',{name:/自有搜索样本/})).toBeVisible();await page.getByRole('button',{name:'搜索媒体',exact:true}).click();await expect(page.getByText(/本次使用TMDB缓存/)).toBeVisible();await expect(page.getByRole('button',{name:'使用标题到站点搜索',exact:true})).toHaveCount(0);await expect(page.getByRole('button',{name:'自有搜索样本',exact:true})).toBeVisible();await page.getByText('站点直搜',{exact:true}).click();await expect(page.getByRole('button',{name:'忽略站点缓存重新搜索',exact:true})).toBeVisible();await expect(page.locator('.account-selection')).toHaveCount(0);await page.screenshot({path:resolve(screenshots,'workbench-search-desktop.png'),animations:'disabled'})
})

test('automatic roots, 0day hints, history and safe plugin manifests',async({page})=>{
 await fixtureLogin(page);await page.goto('/files');await expect(page.getByText(/本地目录/).first()).toBeVisible();const parsed=await page.request.post('/api/v1/media/recognize-name',{headers:{'X-Requested-With':'media-tool'},data:{name:'Owned.Show.2026.S01E03.1080p.WEB-DL.x265-TEAM.mkv'}});expect(parsed.status()).toBe(200);expect((await parsed.json()).data.hint).toMatchObject({title:'Owned Show',year:'2026',mediaType:'series',season:1,episode:3})
 await page.goto('/organize-history');await expect(page.getByRole('heading',{name:'文件与入库历史',exact:true})).toBeVisible();await page.goto('/plugins?tab=catalog');await expect(page.getByRole('heading',{name:'插件目录',exact:true})).toBeVisible();await expect(page.getByRole('heading',{name:'0day 文件名识别',exact:true})).toHaveCount(0);await expect(page.getByRole('heading',{name:'电影与剧集命名模板',exact:true})).toHaveCount(0);await expect(page.getByText('兼容入口：导入扩展 Manifest',{exact:true})).toHaveCount(0)
 const rejected=await page.request.post('/api/v1/plugins',{headers:{'X-Requested-With':'media-tool'},data:{manifest:{id:'owned-plugin',name:'Owned',version:'1',contractVersion:1,runtime:'webhook',endpoint:'https://owned.example',capabilities:['media.organized']},enabled:false,consent:false,revision:0}});expect([404,405]).toContain(rejected.status());await page.screenshot({path:resolve(screenshots,'workbench-plugins-desktop.png'),animations:'disabled'})
})

test('temporary-password sessions are routed to compulsory reset without web recovery (fixture)',async({page})=>{
 await page.route('**/api/v1/auth/session',route=>route.fulfill({json:{data:{authenticated:true,account:{id:'owned-reset-fixture',username:'owned_owner',mustChangePassword:true}}}}))
 await page.route('**/api/v1/auth/password',route=>route.fulfill({json:{data:{changed:true,loginRequired:true}}}))
 await page.goto('/logs');await expect(page).toHaveURL('http://localhost:18084/reset-password');await expect(page.getByRole('heading',{name:'必须设置新密码',exact:true})).toBeVisible();await expect(page.getByRole('link',{name:'程序日志',exact:true})).toHaveCount(0)
 await page.getByLabel('当前密码',{exact:true}).fill('Temporary-2026');await page.getByLabel('新密码',{exact:true}).fill('Replacement-2026');await page.getByLabel('确认新密码',{exact:true}).fill('Replacement-2026');await page.screenshot({path:resolve(screenshots,'mandatory-reset-desktop.png'),animations:'disabled',mask:[page.locator('input[type=password]')]});await page.setViewportSize({width:375,height:812});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,'mandatory-reset-mobile.png'),animations:'disabled',mask:[page.locator('input[type=password]')]});await page.getByRole('button',{name:'设置新密码并重新登录',exact:true}).click();await expect(page.getByRole('heading',{name:'进入管理工作台',exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'忘记密码？使用恢复码',exact:true})).toHaveCount(0)
})


test('grouped navigation, quick jump and keyboard-only return (UI)',async({page})=>{
 await fixtureLogin(page)
 const navigation=page.getByRole('navigation',{name:'功能导航'})
 for(const name of ['获取','媒体','系统'])await expect(navigation.getByRole('heading',{name,exact:true})).toBeVisible()
 await expect(page.getByRole('heading',{name:'模块建设进度',exact:true})).toHaveCount(0)
 await expect(page).toHaveTitle('首页 · Media Tool')
 await page.getByRole('button',{name:'快速前往',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'快速前往',exact:true}),input=dialog.getByRole('combobox',{name:'搜索功能入口'})
 await expect(input).toBeFocused();await input.fill('Redis');await expect(dialog.getByRole('option')).toHaveCount(1)
 await input.press('Enter');await expect(page.getByRole('heading',{name:'只读数据浏览',exact:true})).toBeVisible();await expect(dialog).toBeHidden()
 await page.keyboard.press('Control+k');await expect(input).toBeFocused();await input.fill('not-a-real-feature');await expect(dialog.getByText('没有匹配的功能，请尝试其他关键词。')).toBeVisible()
 await input.press('Enter');await expect(dialog).toBeVisible();await page.keyboard.press('Escape');await expect(dialog).toBeHidden()
 await page.getByRole('button',{name:'快速前往',exact:true}).focus();await page.keyboard.press('Enter');await expect(input).toBeFocused()
 await input.fill('命名模板');await input.press('ArrowDown');await input.press('ArrowUp');await input.press('Enter');await expect(page).toHaveURL(/\/settings\?tab=naming/)
 await page.locator('.settings-workspace > .page-tabs').getByRole('button',{name:'命名与分类',exact:true}).click()
 await expect(page.getByLabel('电影命名模板',{exact:true})).toBeVisible()
})

test('mobile navigation traps focus, closes with Escape and restores its trigger (UI)',async({page})=>{
 await page.setViewportSize({width:375,height:812});await fixtureLogin(page)
 await expect(page.getByRole('button',{name:'退出',exact:true})).toBeVisible()
 const trigger=page.getByRole('button',{name:'打开导航',exact:true});await trigger.click()
 const drawer=page.getByRole('dialog',{name:'主导航',exact:true});await expect(drawer).toBeVisible();await expect(trigger).toHaveAttribute('aria-expanded','true')
 for(let i=0;i<24;i++){await page.keyboard.press('Tab');expect(await drawer.evaluate(el=>el.contains(document.activeElement))).toBe(true)}
 await page.screenshot({path:resolve(screenshots,'ui-navigation-mobile.png'),animations:'disabled'})
 await page.keyboard.press('Escape');await expect(drawer).toBeHidden();await expect(trigger).toBeFocused()
 await page.keyboard.press('Control+k');await expect(page.getByRole('dialog',{name:'快速前往',exact:true})).toBeVisible();await page.keyboard.press('Escape')
 await trigger.click();await drawer.getByRole('link',{name:'运维',exact:true}).click();await page.locator('.media-workspace>.page-tabs').getByRole('button',{name:'日志',exact:true}).click();await expect(page.getByRole('heading',{name:'程序日志',exact:true})).toBeVisible();await expect(drawer).toBeHidden()
})

test('TMDB errors and empty results are visible in media search (UI fixture)',async({page})=>{
 await fixtureLogin(page);let fail=true
 await page.route('**/api/v1/search/media?*',route=>route.fulfill(fail?{status:503,json:{error:{code:'provider_unavailable',message:'自有测试：TMDB暂时不可用'}}}:{json:{data:{items:[],cached:false}}}))
 await page.goto('/search');await page.getByLabel('TMDB媒体名称',{exact:true}).fill('Empty fixture');await page.getByRole('button',{name:'搜索媒体',exact:true}).click()
 await expect(page.getByText('自有测试：TMDB暂时不可用',{exact:true})).toBeVisible()
 fail=false;await page.getByRole('button',{name:'搜索媒体',exact:true}).click();await expect(page.getByText('没有匹配作品',{exact:true})).toBeVisible()
 await page.screenshot({path:resolve(screenshots,'ui-search-empty.png'),animations:'disabled'})
})

test('workbench layouts fit desktop, tablet and mobile with reduced motion (UI)',async({page})=>{
 await fixtureLogin(page);await page.emulateMedia({reducedMotion:'reduce'})
 for(const width of [375,768,1024,1440]){
  await page.setViewportSize({width,height:1000})
  for(const path of ['/dashboard','/search','/files','/mediaLibrary','/logs','/data','/settings','/integrations']){
   await page.goto(path);await expect(page.locator('main h1').first()).toBeVisible()
   expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth),`${path} at ${width}px`).toBe(true)
  }
  await page.goto('/dashboard');await expect(page.getByText('服务器负载',{exact:true})).toBeVisible()
  await page.screenshot({path:resolve(screenshots,`ui-dashboard-${width}.png`),animations:'disabled',fullPage:width>=1024})
 }
})

test('media-library refresh targets home and configuration deep links keep their owner (UI fixture)',async({page})=>{
 await page.route('**/api/v1/integrations',r=>r.fulfill({json:{data:[{kind:'emby',enabled:true}]}}));await fixtureLogin(page);let homeCalls=0
 await page.route('**/api/v1/media/library/emby/home?*',route=>{homeCalls++;return route.fulfill({json:{data:{users:[{id:'owned-user',name:'自有测试用户'}],userId:'owned-user',libraries:[],playing:[],resume:[],recent:[],latest:[]}}})})
 await page.goto('/library?source=emby');await expect.poll(()=>homeCalls).toBe(1)
 await page.getByRole('button',{name:'刷新',exact:true}).click();await expect.poll(()=>homeCalls).toBe(2)
 await page.goto('/mediaLibrary?connectionKind=emby');await expect(page.getByRole('heading',{name:'Emby媒体库连接',exact:true})).toBeVisible()
 await expect(page).toHaveURL(/\/connections\?.*tab=emby/)
 await expect(page.getByRole('heading',{name:/^我的媒体库/})).toHaveCount(0)
})


test('sidebar groups are always expanded and the active destination uses highlight only',async({page})=>{
 await fixtureLogin(page);const nav=page.getByRole('navigation',{name:'功能导航'})
 await expect(nav.getByRole('link')).toHaveCount(13);await expect(nav.getByRole('button')).toHaveCount(0)
 for(const name of ['获取','媒体','系统'])await expect(nav.getByRole('heading',{name,exact:true})).toBeVisible()
 await expect(nav.getByText('当前',{exact:true})).toHaveCount(0)
 await nav.getByRole('link',{name:'首页',exact:true}).focus();await page.keyboard.press('Tab');await expect(nav.getByRole('link',{name:'搜索',exact:true})).toBeFocused()
 await navigate(page,'程序日志');await expect(nav.getByRole('link',{name:'运维',exact:true})).toHaveAttribute('aria-current','page')
 await page.reload();await expect(nav.getByRole('link')).toHaveCount(13);await expect(nav.getByRole('link',{name:'运维',exact:true})).toHaveClass(/selected/)
})

test('desktop and mobile expose the same thirteen expanded destinations',async({page})=>{
 await fixtureLogin(page);const desktop=page.getByRole('complementary',{name:'主导航'})
 const labels=await desktop.getByRole('navigation',{name:'功能导航'}).getByRole('link').allTextContents()
 await page.setViewportSize({width:375,height:812});await page.getByRole('button',{name:'打开导航',exact:true}).click()
 const drawer=page.getByRole('dialog',{name:'主导航',exact:true});expect(await drawer.getByRole('navigation',{name:'功能导航'}).getByRole('link').allTextContents()).toEqual(labels)
 await drawer.getByRole('link',{name:'文件',exact:true}).click();await expect(drawer).toBeHidden();await expect(page.getByRole('heading',{name:'文件浏览',exact:true})).toBeVisible()
 await page.screenshot({path:resolve(screenshots,'navigation-expanded-mobile.png'),animations:'disabled'})
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
})

test('console logs use server filtering, bounded pagination, compact mode and explicit details (fixture)',async({page})=>{
 await fixtureLogin(page);await page.addInitScript(()=>Object.defineProperty(navigator,'clipboard',{value:undefined,configurable:true}));const queries:Array<Record<string,string>>=[];await page.route('**/api/v1/logs/summary',r=>r.fulfill({json:{data:{totalBytes:4096,limitBytes:52428800,level:'info',files:[]}}}));await page.route('**/api/v1/logs/query?*',r=>{const q=Object.fromEntries(new URL(r.request().url()).searchParams);queries.push(q);return r.fulfill({json:{data:{items:q.keyword==='none'?[]:[{time:'2026-10-12T07:41:04.123456789+08:00',level:'INFO',msg:q.page==='2'?'第二页自有日志':'自有控制台样本',module:'search',source:{file:'internal/modules/search/run.go',line:61},requestId:'owned-request',durationMs:26}],total:q.keyword==='none'?0:401,modules:['search','tmdb'],before:'2026-10-12T00:00:00Z',truncated:true,maxFileBytes:2097152,maxRecords:20000}}})});await page.goto('/logs');await expect(page.getByText('自有控制台样本',{exact:true})).toBeVisible();await page.getByRole('button',{name:'完整原始记录',exact:true}).click();const d=page.getByRole('dialog',{name:'完整原始日志',exact:true});await expect(d.locator('pre')).toContainText('owned-request');await d.getByRole('button',{name:'关闭',exact:true}).click();await page.getByRole('checkbox',{name:'清爽模式',exact:true}).check();await expect(page.locator('.u6-log pre')).toHaveCount(0);await page.getByRole('checkbox',{name:'清爽模式',exact:true}).uncheck();await expect(page.getByRole('button',{name:'完整原始记录',exact:true})).toContainText('run.go:61');await page.getByRole('button',{name:'下一页日志',exact:true}).click();await expect.poll(()=>queries.at(-1)?.page).toBe('2');expect(queries.at(-1)?.before).toBe('2026-10-12T00:00:00Z');await page.getByLabel('搜索日志',{exact:true}).fill('owned');await expect.poll(()=>queries.at(-1)?.keyword).toBe('owned');expect(queries.at(-1)?.page).toBe('1');expect(queries.at(-1)?.before).toBeUndefined();await page.getByRole('combobox',{name:'筛选日志等级',exact:true}).selectOption('error');await expect.poll(()=>queries.at(-1)?.level).toBe('error');await page.getByRole('combobox',{name:'筛选日志模块',exact:true}).selectOption('tmdb');await expect.poll(()=>queries.at(-1)?.module).toBe('tmdb');const console=page.getByRole('region',{name:'系统日志控制台',exact:true});await console.getByPlaceholder('开始时间',{exact:true}).fill('2026-10-12T00:00');await console.getByPlaceholder('结束时间',{exact:true}).fill('2026-10-12T23:59:59');await console.getByPlaceholder('结束时间',{exact:true}).press('Enter');await page.keyboard.press('Tab');await expect.poll(()=>queries.at(-1)?.from).toBeTruthy();expect(queries.at(-1)?.to).toBeTruthy();await page.keyboard.press('Escape');await expect(page.getByText(/检索窗口已达上限/)).toBeVisible();await page.getByRole('button',{name:'复制当前页日志',exact:true}).click();const copy=page.getByRole('dialog',{name:'复制原始日志',exact:true});await expect(copy.getByLabel('待复制日志')).toHaveValue(/自有控制台样本/);await copy.getByRole('button',{name:'关闭',exact:true}).click();for(const width of [1440,375]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`console-log-${width}.png`),fullPage:true})}await page.getByLabel('搜索日志',{exact:true}).fill('none');await expect(page.getByText('没有符合筛选条件的日志',{exact:true})).toBeVisible()
})

test('log query failures are visible and do not present stale content as current (fixture)',async({page})=>{
 await fixtureLogin(page);let failed=false
 await page.route('**/api/v1/logs/query?*',route=>route.fulfill(failed?{status:503,json:{error:{code:'logs_unavailable',message:'自有日志读取失败'}}}:{json:{data:{items:[{time:'2026-10-12T07:00:00+08:00',level:'INFO',msg:'自有旧快照'}],total:1,modules:['app'],before:'2026-10-12T00:00:00Z',truncated:false,page:1,pageSize:100}}}))
 await page.goto('/logs');await expect(page.getByText('自有旧快照',{exact:true})).toBeVisible();failed=true;await page.getByRole('button',{name:'刷新',exact:true}).click()
 await expect(page.getByText('自有日志读取失败',{exact:true})).toBeVisible();await expect(page.getByText('自有旧快照',{exact:true})).toBeVisible();await expect(page.getByRole('status').filter({hasText:'刷新失败，保留上次数据。'})).toBeVisible()
})

test('built-in plugins consent, config, enable, instance logs, uninstall (isolated)',async({page})=>{
 await fixtureLogin(page);await page.goto('/plugins?tab=catalog');await expect(page.getByLabel('选择ZIP插件包')).toHaveCount(0)
 const item=page.getByRole('region',{name:'内置插件目录'}).locator('article').filter({has:page.getByRole('heading',{name:'Hello Go',exact:true})});await item.getByRole('button',{name:/安装插件|新增独立实例/}).click();const consent=page.getByRole('dialog',{name:'插件权限确认',exact:true});await expect(consent).toContainText('plugin.data.write');await expect(consent.getByRole('button',{name:'确认安装插件',exact:true})).toBeDisabled();await consent.getByRole('checkbox').check();await consent.getByRole('button',{name:'确认安装插件',exact:true}).click();await expect(consent).toBeHidden();await page.getByRole('button',{name:'已安装',exact:true}).click();await page.getByRole('link',{name:'管理插件',exact:true}).first().click();const root=page.locator('.u6-workspace');await expect(root).toContainText('已停用');await page.getByLabel('插件配置 JSON',{exact:true}).fill('{"greeting":"Owned fixture"}');await page.getByRole('button',{name:'保存插件配置',exact:true}).click();await page.getByRole('dialog',{name:'确认插件操作',exact:true}).getByRole('button',{name:'确认操作',exact:true}).click();await expect(page.getByRole('dialog',{name:'确认插件操作',exact:true})).toBeHidden();for(const name of ['启用插件','停用插件']){await page.getByRole('button',{name,exact:true}).click();await page.getByRole('dialog',{name:'确认插件操作',exact:true}).getByRole('button',{name:'确认操作',exact:true}).click();await expect(page.getByRole('button',{name:name==='启用插件'?'停用插件':'启用插件',exact:true})).toBeVisible()}
 await page.getByRole('button',{name:'实例数据',exact:true}).click();await page.getByRole('button',{name:'查看 KV',exact:true}).click();const kv=page.getByRole('dialog',{name:'实例 KV 数据',exact:true});await expect(kv).toBeVisible();await kv.getByRole('button',{name:'关闭',exact:true}).click();await page.getByRole('link',{name:'实例日志',exact:true}).click();await expect(page).toHaveURL(/instance=/);await expect(page.getByText(/已按插件实例筛选/)).toBeVisible();await page.goto('/plugins');await page.screenshot({path:resolve(screenshots,'builtin-plugins-desktop.png')});const items=(await(await page.request.get('/api/v1/plugins')).json()).data,definitions=(await(await page.request.get('/api/v1/plugins/catalog')).json()).data.items;for(const i of items)expect((await page.request.post('/api/v1/plugins/operations',{headers:{'X-Requested-With':'media-tool'},data:{kind:'remove',operationKey:crypto.randomUUID(),instanceId:i.id,fingerprint:definitions.find((d:any)=>d.manifest.id===i.pluginId).fingerprint,expectedRevision:i.revision,confirm:true,scopes:[]}})).status()).toBe(200)
})

test('PT metrics show actual coverage, zero and unknown separately, with mobile layout (fixture)',async({page})=>{
 await page.route('**/api/v1/integrations',r=>r.fulfill({json:{data:[{'id': 'owned', 'name': 'Owned PT', 'kind': 'ptsite', 'enabled': true, 'settings': {'definitionId': 'hdsky'}}, {'id': 'unknown', 'name': 'Unknown PT', 'kind': 'ptsite', 'enabled': true, 'settings': {'definitionId': 'hdsky'}}]}}))
 await page.route('**/api/v1/sites/metrics',route=>route.fulfill({json:{data:[{accountId:'owned',name:'Owned PT',enabled:true,metrics:{uploaded:1099511627776,downloaded:1073741824,ratio:1024,bonus:1234.5,level:'Owned User',seeding:0,seedingBytes:0},collectedAt:'2026-10-12T08:00:00Z',attemptedAt:'2026-10-12T08:00:00Z',state:'partial',message:'',stale:false},{accountId:'unknown',name:'Unknown PT',enabled:true,metrics:null,collectedAt:null,attemptedAt:null,state:'not_synced',message:'',stale:false}]}}))
 await fixtureLogin(page);await page.goto('/sites');await expect(page.getByRole('heading',{name:'站点账号',exact:true})).toBeVisible();await expect(page.locator('.u5-stats').first()).toContainText('已读取 1 个启用站点的快照');await expect(page.locator('.u5-metric-card').filter({hasText:'Owned PT'})).toContainText('1.00 TiB');await expect(page.locator('.u5-metric-card').filter({hasText:'Unknown PT'})).toContainText('—');await page.screenshot({path:resolve(screenshots,'pt-metrics-desktop.png'),animations:'disabled'});await page.setViewportSize({width:390,height:844});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);await page.screenshot({path:resolve(screenshots,'pt-metrics-mobile.png'),animations:'disabled'});await page.getByLabel('筛选站点账号',{exact:true}).fill('Unknown');await expect(page.locator('.u5-metric-card')).toHaveCount(1)
})

test('database dark console supports simple mode and formatted row detail (fixture)',async({page})=>{
 await page.route('**/api/v1/data/postgres/tables',route=>route.fulfill({json:{data:[{name:'owned_records',estimatedRows:1}]}}));await page.route('**/api/v1/data/postgres/tables/owned_records?*',route=>route.fulfill({json:{data:{items:[{id:'owned',state:'succeeded',payload:{hello:'world',apiKey:'[已隐藏]'}}],total:1}}}))
 await fixtureLogin(page);await page.goto('/data');await expect(page.locator('.u6-data-row')).toHaveCount(1);await page.getByRole('checkbox',{name:'简约模式',exact:true}).check();await page.locator('.u6-data-row').click();const dialog=page.getByRole('dialog').filter({has:page.locator('pre')});await expect(dialog).toContainText('"hello": "world"');await page.screenshot({path:resolve(screenshots,'database-console-desktop.png'),animations:'disabled'});await dialog.getByRole('button',{name:'关闭',exact:true}).click();await page.getByLabel('筛选当前页数据',{exact:true}).fill('absent');await expect(page.locator('.u6-data-row')).toHaveCount(0)
})

test('automatic media search retains movie and series with matching numeric IDs (fixture)',async({page})=>{
 const queries:string[]=[];await page.route('**/api/v1/search/media?*',route=>{queries.push(new URL(route.request().url()).searchParams.get('mediaType')||'');return route.fulfill({json:{data:{items:[{id:11,mediaType:'movie',title:'Owned Film',originalTitle:'Owned Film',year:2026},{id:11,mediaType:'series',title:'Owned Series',originalTitle:'Owned Series',year:2026}],cached:false,recognitionHint:{title:'Owned',year:'2026'}}}})})
 await fixtureLogin(page);await page.goto('/search');await page.getByText('TMDB 媒体搜索',{exact:true}).click();await page.getByLabel('TMDB媒体名称',{exact:true}).fill('Owned.2026.1080p.WEB-DL.mkv');await page.getByRole('button',{name:'搜索媒体',exact:true}).click();await expect(page.locator('.acquire-media-grid article').filter({hasText:'Owned Film'}).getByRole('heading')).toBeVisible();await expect(page.locator('.acquire-media-grid article').filter({hasText:'Owned Series'}).getByRole('heading')).toBeVisible();expect(queries).toContain('auto');await page.screenshot({path:resolve(screenshots,'auto-media-search.png'),animations:'disabled'})
})

test.describe('real PWA lifecycle',()=>{
 test.use({serviceWorkers:'allow'})
test('PWA registration, public-only cache, headers and generic offline page (localhost secure context)',async({page,context})=>{
 await fixtureLogin(page);await page.goto('/settings?tab=mobile');await expect(page.getByRole('heading',{name:'随时打开媒体工作台',exact:true})).toBeVisible()
 await page.evaluate(async()=>{await navigator.serviceWorker.ready});await page.reload();await expect(page.getByRole('heading',{name:'随时打开媒体工作台',exact:true})).toBeVisible()
 const sw=await page.request.get('/sw.js');expect(sw.headers()['cache-control']).toBe('no-cache');expect(sw.headers()['service-worker-allowed']).toBe('/');expect(await sw.text()).not.toContain('__BUILD_ID__')
 const manifest=await page.request.get('/manifest.webmanifest');expect(manifest.status()).toBe(200);const body=await manifest.json();expect(body).toMatchObject({name:'Media Tool 媒体工作台',start_url:'/dashboard',scope:'/',display:'standalone'});expect(new URL(body.start_url,manifest.url()).origin).toBe(new URL(process.env.E2E_BASE_URL!).origin)
 for(const path of ['/manifest.webmanifest','/sw.js','/offline.html','/pwa/icon-192.png','/pwa/icon-512.png']){const reply=await page.request.get('http://localhost:18087'+path);expect(reply.status()).toBe(404);expect(reply.headers()['content-type']).toContain('application/json');expect(reply.headers()['service-worker-allowed']).toBeUndefined();expect(await reply.json()).toEqual({ResponseStatus:{ErrorCode:'NotFound',Message:'Not Found'}})}
 const keys=await page.evaluate(async()=>{const out:string[]=[];for(const name of await caches.keys()){if(name.startsWith('media-tool-public-'))for(const request of await (await caches.open(name)).keys())out.push(new URL(request.url).pathname)}return out});expect(keys).toContain('/offline.html');expect(keys.every(key=>key.startsWith('/assets/')||key.startsWith('/pwa/')||key==='/offline.html')).toBe(true)
 await context.setOffline(true);await page.goto('/data');await expect(page.getByRole('heading',{name:'当前处于离线状态',exact:true})).toBeVisible();await expect(page.getByRole('heading',{name:'只读数据浏览',exact:true})).toHaveCount(0);await context.setOffline(false)
})

})

test('site-result filters issue server query and preserve selection binding (fixture)',async({page})=>{
 const seen:string[]=[]
 await page.route('**/api/v1/search',route=>route.fulfill({json:{data:[{id:'owned-filter-run',keyword:'Owned',state:'succeeded',sites:[],resources:[],total:65}]}}))
 await page.route('**/api/v1/search/owned-filter-run?*',route=>{const q=new URL(route.request().url()).searchParams;seen.push(q.toString());return route.fulfill({json:{data:{id:'owned-filter-run',keyword:'Owned',state:'succeeded',sites:[{accountId:'owned-site',name:'Owned PT',state:'succeeded',message:'',count:65}],resources:[{id:'original-resource-id',title:'Owned.Film.2026.2160p.WEB-DL.x265',siteName:'Owned PT',size:1073741824,seeders:11,leechers:0,downloadFactor:0,metadata:{attributes:{resolution:'2160p',codec:'h265',source:'web-dl'}}}],total:q.get('resolution')==='2160p'?12:65}}})})
 await fixtureLogin(page);await page.goto('/search');await page.getByText('站点直搜',{exact:true}).click();await page.getByRole('link',{name:'查看结果',exact:true}).click();await expect(page.getByText('Owned.Film.2026.2160p.WEB-DL.x265',{exact:true})).toBeVisible()
 await page.locator('.el-select').filter({has:page.getByRole('combobox',{name:'资源分辨率',exact:true})}).click();await page.getByRole('option',{name:'2160p',exact:true}).click();await page.getByRole('combobox',{name:'资源分辨率',exact:true}).press('Escape');await expect(page.getByText(/12 条匹配/)).toBeVisible();expect(seen.some(q=>new URLSearchParams(q).get('resolution')==='2160p'&&new URLSearchParams(q).get('page')==='1')).toBe(true)
 for(const label of ['最少做种人数','最小资源大小','最大资源大小','仅免费'])await expect(page.getByLabel(label,{exact:true})).toHaveCount(0)
 await page.getByLabel('资源关键字筛选',{exact:true}).fill('film x265')
 await expect.poll(()=>seen.some(q=>new URLSearchParams(q).get('keyword')==='film x265')).toBe(true)
 await expect(page.getByRole('combobox',{name:'资源排序',exact:true})).toHaveCount(0);await page.getByRole('columnheader',{name:'按文件大小排序',exact:true}).click();await expect.poll(()=>seen.some(q=>new URLSearchParams(q).get('sort')==='size_desc')).toBe(true);await page.getByRole('columnheader',{name:'按文件大小排序',exact:true}).click()
 await expect.poll(()=>seen.some(q=>new URLSearchParams(q).get('sort')==='size_asc'&&new URLSearchParams(q).get('page')==='1')).toBe(true)
 await page.getByRole('columnheader',{name:'按做种人数排序',exact:true}).press('Enter');await expect.poll(()=>seen.some(q=>new URLSearchParams(q).get('sort')==='seeders_desc')).toBe(true);await page.getByRole('columnheader',{name:'按做种人数排序',exact:true}).press('Space');await expect.poll(()=>seen.some(q=>new URLSearchParams(q).get('sort')==='seeders_asc')).toBe(true)
 await page.locator('.el-select').filter({has:page.getByRole('combobox',{name:'资源来源',exact:true})}).click();expect(await page.getByRole('option').allTextContents()).toEqual(['WEB-DL','WEBRip','Blu-ray','Remux','HDTV']);await page.getByRole('option',{name:'Blu-ray',exact:true}).click();await page.getByRole('combobox',{name:'资源来源',exact:true}).press('Escape');await expect.poll(()=>seen.some(q=>new URLSearchParams(q).get('source')==='bluray')).toBe(true)
 await page.getByRole('button',{name:'确认并下载',exact:true}).click();await expect(page.getByRole('dialog',{name:'确认媒体身份与下载',exact:true})).toContainText('Owned.Film');await page.getByRole('button',{name:'关闭此对话框',exact:true}).click()
 await page.screenshot({path:resolve(screenshots,'site-resource-filters.png'),animations:'disabled'})
})

test('built-in clone retains config, isolated identities, no runtime upload/market (isolated)',async({page})=>{
 await fixtureLogin(page)
 const d=(await(await page.request.get('/api/v1/plugins/catalog')).json()).data.items[0]
 const op=async(kind:string,extra:any={})=>{const r=await page.request.post('/api/v1/plugins/operations',{headers:{'X-Requested-With':'media-tool'},data:{kind,operationKey:crypto.randomUUID(),pluginId:d.manifest.id,fingerprint:d.fingerprint,expectedRevision:0,confirm:true,scopes:d.manifest.permissions.map((p:any)=>p.scope),config:{fixture:'owned'},...extra}});expect(r.status()).toBe(200);return(await r.json()).data}
 const first=await op('install');const clone=await op('clone',{instanceId:first.id,expectedRevision:first.revision});expect(clone.id).not.toBe(first.id);expect(clone.desiredState).toBe('disabled')
 const data=(await(await page.request.get('/api/v1/plugins/'+clone.id)).json()).data;expect(data.config.value).toEqual({fixture:'owned'})
 for(const path of ['/api/v1/plugins/package','/api/v1/plugin-market/package','/api/v1/plugin-market/repositories']){const r=await page.request.post(path,{headers:{'X-Requested-With':'media-tool'},data:{}});expect([404,405]).toContain(r.status())}
 await page.goto('/plugins');await expect(page.locator('.u6-card')).toHaveCount(2);await page.setViewportSize({width:390,height:844});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true)
 for(const i of [first,clone])await op('remove',{instanceId:i.id,expectedRevision:i.revision})
})

test('subscription structured filters persist and preview displays individual reasons (fixture)',async({page})=>{
 await page.route('**/api/v1/subscriptions**',route=>route.fulfill({json:{data:[{id:'owned-subscription',title:'Owned Rules',media:{id:1,mediaType:'movie',title:'Owned Film',year:'2026'},enabled:false,source:'search',intervalMinutes:30,rules:{accountIds:['owned-account'],keyword:'Owned',include:[],exclude:[],minSize:0,maxSize:0,minSeeders:1,freeOnly:true,allowUnknownHR:false,season:1,episodes:[],includeHistory:false,resolution:'2160p',source:'web-dl',codec:'h265'},revision:1,lastState:'succeeded',lastMessage:'',nextRunAt:''}]}}))
 await page.route('**/api/v1/subscriptions/owned-subscription/evaluate',route=>route.fulfill({status:202,json:{data:{id:'owned-preview'}}}));await page.route('**/api/v1/subscription-runs/owned-preview',route=>route.fulfill({json:{data:{id:'owned-preview',preview:true,state:'succeeded',decisions:[{id:'owned-decision',title:'Owned Film 2026 1080p WEB-DL x264',action:'skipped',episodes:[],reasons:['分辨率不匹配','编码不匹配'],checks:[{key:'resolution',label:'分辨率',expected:'2160p',actual:'1080p',state:'rejected',message:'分辨率不匹配：需要2160p，识别为1080p'},{key:'source',label:'来源',expected:'web-dl',actual:'web-dl',state:'matched',message:'来源匹配：web-dl'}]}]}}}));await fixtureLogin(page);await page.goto('/subscriptions');await page.getByRole('button',{name:'Owned Rules',exact:true}).click();await page.getByRole('button',{name:'编辑规则',exact:true}).click();await expect(page.getByRole('combobox',{name:'订阅分辨率',exact:true})).toBeVisible();await expect(page.getByRole('dialog',{name:'编辑订阅',exact:true})).toContainText('2160p');await page.getByRole('dialog',{name:'编辑订阅',exact:true}).getByRole('button',{name:'关闭此对话框',exact:true}).click();await page.getByRole('button',{name:'Owned Rules更多操作',exact:true}).click();await page.getByRole('menuitem',{name:'预演',exact:true}).click();await expect(page.getByRole('region',{name:'运行结果',exact:true})).toContainText('分辨率 · 拒绝');await expect(page.getByRole('region',{name:'运行结果',exact:true})).toContainText('来源 · 通过');await page.screenshot({path:resolve(screenshots,'subscription-structured-preview.png'),animations:'disabled'})
})

test('raw administrator log URLs have dedicated full request and 302 target view (fixture)',async({page})=>{
 await page.route('**/api/v1/logs/query?*',route=>route.fulfill({json:{data:{items:[{time:'2026-10-12T10:00:00Z',level:'INFO',module:'playback',msg:'HTTP request',url:'https://owned.example/strm?apiKey=owned-fixture-key',location:'https://cdnfhnfile.115cdn.net/owned?sign=owned-fixture-sign',status:302,durationMs:12,requestId:'owned-request'}],total:1,page:1,pageSize:200,modules:['playback'],before:'2026-10-12T11:00:00Z',truncated:false}}}));await fixtureLogin(page);await page.goto('/logs');await page.getByRole('button',{name:'查看完整链接',exact:true}).click();const dialog=page.getByRole('dialog',{name:'完整原始日志',exact:true});await expect(dialog).toBeVisible();await expect(dialog.locator('textarea').nth(0)).toHaveValue('https://owned.example/strm?apiKey=owned-fixture-key');await expect(dialog.locator('textarea').nth(1)).toHaveValue('https://cdnfhnfile.115cdn.net/owned?sign=owned-fixture-sign');await page.screenshot({path:resolve(screenshots,'raw-playback-url-dialog.png'),animations:'disabled'})
})

test('PT site total counts all added sites regardless of metrics or status (fixture)',async({page})=>{
 await page.route('**/api/v1/integrations',r=>r.fulfill({json:{data:[{'id': 'one', 'name': 'HDSKY', 'kind': 'ptsite', 'enabled': true, 'settings': {'definitionId': 'hdsky'}}, {'id': 'two', 'name': 'SSD', 'kind': 'ptsite', 'enabled': true, 'settings': {'definitionId': 'hdsky'}}]}}))
 await page.route('**/api/v1/sites/metrics',route=>route.fulfill({json:{data:[{accountId:'one',name:'HDSKY',enabled:true,metrics:{uploaded:10,downloaded:1,seeding:50,seedingBytes:100},stale:false,state:'succeeded'},{accountId:'two',name:'SSD',enabled:true,metrics:{uploaded:20,downloaded:2,seeding:4,seedingBytes:20},stale:false,state:'succeeded'}]}}))
 await fixtureLogin(page);await page.goto('/sites');const box=page.locator('.u5-stats>div').filter({hasText:'已添加站点'});await expect(box.locator('strong')).toHaveText('2');await expect(page.getByText('总做种数',{exact:true})).toHaveCount(0)
 await page.route('**/api/v1/sites/metrics',route=>route.fulfill({json:{data:[{accountId:'one',name:'HDSKY',enabled:true,metrics:{uploaded:10,downloaded:1,seeding:50},stale:true,state:'failed'},{accountId:'disabled',name:'SSD',enabled:false,metrics:null,state:'not_synced'}]}}));await page.reload();await expect(page.locator('.u5-stats>div').filter({hasText:'已添加站点'}).locator('strong')).toHaveText('2');await page.getByLabel('筛选站点账号',{exact:true}).fill('HDSKY');await expect(page.locator('.u5-stats>div').filter({hasText:'已添加站点'}).locator('strong')).toHaveText('2')
})

test('site data schedule persists cron and disable settings (isolated)',async({page})=>{
 await fixtureLogin(page);await page.goto('/sites');await page.getByRole('button',{name:'编辑采集计划',exact:true}).click();const d=page.getByRole('dialog',{name:'指标采集计划',exact:true});await d.getByLabel('启用定时采集',{exact:true}).check();await d.getByLabel('站点指标 Cron',{exact:true}).fill('0 */2 * * *');await d.getByRole('button',{name:'保存采集计划',exact:true}).click();await expect(d).toBeHidden();await expect(page.getByText(/已启用 · 每 2 小时/)).toBeVisible();await page.reload();await expect(page.getByText(/已启用 · 每 2 小时/)).toBeVisible();await page.getByRole('button',{name:'编辑采集计划',exact:true}).click();await d.getByLabel('启用定时采集',{exact:true}).uncheck();await d.getByRole('button',{name:'保存采集计划',exact:true}).click();await expect(d).toBeHidden();await expect(page.getByText(/已停用 · 每 2 小时/)).toBeVisible();await page.setViewportSize({width:390,height:844});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);await page.screenshot({path:resolve(screenshots,'site-schedule-mobile.png'),fullPage:true})
})

test('site action buttons and mixed search-history states align at desktop and mobile (fixture)',async({page})=>{
 await page.route('**/api/v1/integrations',r=>r.fulfill({json:{data:[{'id': 'one', 'name': 'HDSKY', 'kind': 'ptsite', 'enabled': true, 'settings': {'definitionId': 'hdsky'}}]}}))
 await page.route('**/api/v1/sites/metrics',route=>route.fulfill({json:{data:[{accountId:'one',name:'HDSKY',enabled:true,metrics:null,state:'not_synced',stale:false}]}}));await fixtureLogin(page);await page.goto('/sites');const action=page.locator('.u5-metric-card .media-actions');await expect(action.getByRole('button',{name:'编辑账号',exact:true})).toBeVisible();expect(await action.getByRole('button',{name:'编辑账号',exact:true}).evaluate(el=>el.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44);await action.getByRole('button',{name:'更多站点操作',exact:true}).click();await expect(page.getByRole('menuitem',{name:'采集此站点',exact:true})).toBeVisible();await page.keyboard.press('Escape');await page.screenshot({path:resolve(screenshots,'site-action-alignment.png'),fullPage:true,animations:'disabled'});
 await page.route('**/api/v1/search',route=>route.fulfill({json:{data:[{id:'one',keyword:'短标题',state:'succeeded'},{id:'two',keyword:'Very Long Original Media Title 2026',state:'partial'},{id:'three',keyword:'第三行',state:'failed'}]}}));await page.goto('/search');await page.getByText('站点直搜',{exact:true}).click();await expect(page.locator('[aria-label="近期搜索"] .acquire-history article')).toHaveCount(3);await expect(page.locator('[aria-label="近期搜索"]')).toContainText('已完成');await expect(page.locator('[aria-label="近期搜索"]')).toContainText('部分结果');await expect(page.locator('[aria-label="近期搜索"]')).toContainText('失败');await page.screenshot({path:resolve(screenshots,'search-history-alignment.png'),fullPage:true,animations:'disabled'});await page.setViewportSize({width:390,height:844});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);await page.screenshot({path:resolve(screenshots,'search-history-mobile.png'),fullPage:true,animations:'disabled'})
})

test('subscription query controls, regex and shared TMDB cache are explicit; removed limits stay absent (fixture)',async({page})=>{
 let calls=0;await page.route('**/api/v1/search/media?*',r=>r.fulfill({json:{data:{items:[{id:42,mediaType:'movie',title:'Owned Film',year:'2026'}],cached:++calls>1,cacheSeconds:86400}}}))
 await page.route('**/api/v1/integrations',r=>r.fulfill({json:{data:[{id:'owned-account',name:'Owned Site',kind:'ptsite',enabled:true,secretFields:[]}]}}))
 await fixtureLogin(page);await page.goto('/subscriptions');await page.getByRole('button',{name:'新建订阅',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'新建订阅',exact:true}),query=dialog.getByLabel('订阅TMDB关键词',{exact:true})
 await expect(dialog.getByRole('button',{name:'查询TMDB',exact:true})).toBeDisabled();await query.fill('Owned Film');await dialog.getByRole('button',{name:'查询TMDB',exact:true}).click();await expect(dialog.getByText(/已查询TMDB · 缓存有效期 86400 秒/)).toBeVisible();await dialog.getByRole('button',{name:'查询TMDB',exact:true}).click();await expect(dialog.getByText(/来自TMDB缓存/)).toBeVisible()
 expect(await query.evaluate(el=>el.getBoundingClientRect().width)).toBeGreaterThan(200);await page.screenshot({path:resolve(screenshots,'subscription-query-desktop.png'),animations:'disabled'})
 await page.setViewportSize({width:390,height:844});const dims=await dialog.locator('.composer-media-search').evaluate(el=>({width:el.clientWidth,scroll:el.scrollWidth}));expect(dims.scroll).toBeLessThanOrEqual(dims.width+1)
 await dialog.getByRole('group',{name:'匹配影视',exact:true}).getByRole('button').click();await dialog.getByRole('button',{name:'选择站点与规则',exact:true}).click()
 for(const label of ['最小体积 GiB','最大体积 GiB（0不限）','最少做种人数','间隔分钟'])await expect(dialog.getByText(label,{exact:true})).toHaveCount(0)
 await expect(dialog.getByRole('checkbox',{name:'使用正则匹配包含／排除关键词',exact:true})).toBeChecked();await dialog.getByLabel('订阅包含关键词',{exact:true}).fill('(?i)WEB[-.]DL\nS[0-9]{1,3}');await page.screenshot({path:resolve(screenshots,'subscription-query-mobile.png'),animations:'disabled'})
 await dialog.getByRole('button',{name:'关闭此对话框',exact:true}).click();await page.goto('/settings?tab=automation');await expect(page.getByLabel('订阅调度时间',{exact:true})).toBeVisible();await expect(page.getByLabel('TMDB 搜索缓存有效期（秒）',{exact:true})).toBeVisible()
})

test('template list layout persists and multi-select result facets retain repeated values (fixture)',async({page})=>{
 await fixtureLogin(page);await page.goto('/site-templates');await page.getByRole('button',{name:'列表展示模板',exact:true}).click();await expect(page.locator('.u5-template-list')).toBeVisible();await page.reload();await expect(page.locator('.u5-template-list')).toBeVisible();await page.getByRole('button',{name:'网格展示模板',exact:true}).click();await expect(page.locator('.u5-template-list')).toHaveCount(0)
 const seen:string[]=[];await page.route('**/api/v1/search',route=>route.fulfill({json:{data:[{id:'owned-multiple',keyword:'Owned',state:'succeeded'}]}}));await page.route('**/api/v1/search/owned-multiple?*',route=>{seen.push(new URL(route.request().url()).search);return route.fulfill({json:{data:{id:'owned-multiple',keyword:'Owned',state:'succeeded',sites:[{accountId:'a',name:'Owned A',state:'succeeded'},{accountId:'b',name:'Owned B',state:'succeeded'}],resources:[],total:0}}})});await page.goto('/search');await page.getByText('站点直搜',{exact:true}).click();await page.getByRole('link',{name:'查看结果',exact:true}).click();await page.locator('.el-select').filter({has:page.getByRole('combobox',{name:'资源分辨率',exact:true})}).click();await page.getByRole('option',{name:'1080p',exact:true}).click();await page.getByRole('option',{name:'2160p',exact:true}).click();await page.getByRole('combobox',{name:'资源分辨率',exact:true}).press('Escape');await expect.poll(()=>seen.some(x=>new URLSearchParams(x).getAll('resolution').length===2)).toBe(true)
})

test('independent cache pages expose values and confirmed scoped clear only (fixture)',async({page})=>{
 await page.route('**/api/v1/metadata/image/*/owned.png',r=>r.fulfill({contentType:'image/svg+xml',body:'<svg xmlns="http://www.w3.org/2000/svg" width="80" height="120"><rect width="80" height="120" fill="#c7ded3"/></svg>'}));
 const id='a'.repeat(64),requests:string[]=[];await page.route('**/api/v1/cache/**',route=>{const path=new URL(route.request().url()).pathname;requests.push(path);if(path.endsWith('/clear'))return route.fulfill({json:{data:{deleted:1,remainingMayExist:false}}});if(path.endsWith('/'+id))return route.fulfill({json:{data:{id,key:'media-tool:tmdb-cache:'+id,data:{query:'Owned',items:[{title:'Owned Film'}]}}}});return route.fulfill({json:{data:{items:[{id,key:'media-tool:tmdb-cache:'+id,query:'Owned',state:'cached',bytes:200,ttlSeconds:45,count:1}],total:1,page:1,truncated:false}}})});await fixtureLogin(page);await page.goto('/tmdb-cache');await expect(page.getByRole('heading',{name:'缓存管理',exact:true})).toBeVisible();await page.getByRole('button',{name:'缓存详情',exact:true}).click();await page.getByRole('dialog',{name:'缓存内容',exact:true}).getByRole('button',{name:/技术细节与原始记录/}).click();await expect(page.getByRole('dialog',{name:'缓存内容',exact:true})).toContainText('Owned Film');await page.getByRole('button',{name:'关闭',exact:true}).click();await page.getByRole('button',{name:'清理此类缓存',exact:true}).click();await page.getByRole('button',{name:'确认清理',exact:true}).click();expect(requests.some(x=>x.endsWith('/tmdb/clear'))).toBe(true);await page.goto('/search-cache');await expect(page.getByRole('heading',{name:'继续上次的搜索',exact:true})).toBeVisible();expect(requests.some(x=>x.includes('/search'))).toBe(true);await page.setViewportSize({width:390,height:844});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);await page.screenshot({path:resolve(screenshots,'cache-mobile.png'),fullPage:true,animations:'disabled'})
})

test('search media context has verified identity, details and disabled subscription prefill (fixture)',async({page})=>{
 await page.route('**/api/v1/metadata/image/**',route=>route.fulfill({contentType:'image/png',body:readFileSync(resolve(root,'web/public/pwa/icon-192.png'))}))
 await page.route('**/api/v1/search',route=>route.fulfill({json:{data:[{id:'owned-context',keyword:'Owned Film',state:'succeeded'}]}}));await page.route('**/api/v1/search/owned-context?*',route=>route.fulfill({json:{data:{id:'owned-context',keyword:'Owned Film',state:'succeeded',sites:[],resources:[],total:0}}}));await page.route('**/api/v1/search/media?*',route=>route.fulfill({json:{data:{items:[{id:42,mediaType:'movie',title:'Owned Film',originalTitle:'Owned Film',year:'2026'}],cached:false}}}));await page.route('**/api/v1/metadata/details?*',route=>route.fulfill({json:{data:{item:{id:42,mediaType:'movie',title:'Owned Film',originalTitle:'Owned Film',year:'2026',overview:'Owned movie overview',genres:['剧情'],posterPath:'/owned.png',backdropPath:'/owned-backdrop.png'},cached:false}}}));await fixtureLogin(page);await page.goto('/search');await page.getByText('站点直搜',{exact:true}).click();await page.getByRole('link',{name:'查看结果',exact:true}).click();await expect(page.locator('.media-context')).toContainText('Owned Film');await expect(page.locator('.media-context')).toContainText('Owned Film 2026');await expect(page.locator('.context-poster')).toHaveAttribute('src','/api/v1/metadata/image/w500/owned.png');await page.getByRole('button',{name:'查看详情',exact:true}).click();await expect(page.getByRole('dialog',{name:'媒体详情',exact:true})).toContainText('Owned movie overview');await page.getByRole('button',{name:'关闭此对话框',exact:true}).click();await expect(page.getByRole('dialog',{name:'媒体详情',exact:true})).not.toBeVisible();await page.screenshot({path:resolve(screenshots,'search-media-context.png'),fullPage:true,animations:'disabled'});await expect(page.locator('.media-context').getByRole('button',{name:'订阅',exact:true})).toHaveCount(0);await expect(page.locator('.media-context')).not.toHaveCSS('background-image',/url/);await expect(page.locator('.context-poster')).toHaveCSS('width','88px')
})

test('runtime update: compact media title searches all sites inline with promotion and visible cached results (fixture)',async({page})=>{
 await page.route('**/api/v1/metadata/image/*/owned.png',r=>r.fulfill({contentType:'image/svg+xml',body:'<svg xmlns="http://www.w3.org/2000/svg" width="80" height="120"><rect width="80" height="120" fill="#c7ded3"/><text x="40" y="64" text-anchor="middle" font-size="12">TEST</text></svg>'}));
 const id='11111111-1111-4111-8111-111111111111',media={id:42,mediaType:'series',title:'自有媒体测试',originalTitle:'Owned Media',year:'2026',posterPath:'/owned.png',overview:'仅用于隔离界面测试'};
 const resources=[{id:'r1',title:'Owned.Media.1080p.WEB-DL.H264',siteName:'HDSKY',size:1024,seeders:10,downloadFactor:.5,metadata:{subtitle:'自有副标题',attributes:{codec:'h264',source:'web-dl',resolution:'1080p'}}}];let started=false;
 await page.route('**/api/v1/integrations',r=>r.fulfill({json:{data:[{id:'site1',kind:'ptsite',name:'HDSKY',enabled:true},{id:'site2',kind:'ptsite',name:'SSD',enabled:true}]}}));
 await page.route('**/api/v1/search/media?*',r=>r.fulfill({json:{data:{items:[media],cached:false}}}));
 await page.route('**/api/v1/metadata/details?*',r=>r.fulfill({json:{data:{item:media}}}));
 await page.route('**/api/v1/search',r=>{if(r.request().method()==='POST'){expect(r.request().postDataJSON()).toMatchObject({keyword:media.title});expect(r.request().postDataJSON().accountIds).toBeUndefined();started=true;return r.fulfill({status:202,json:{data:{id,cached:false}}})}return r.fulfill({json:{data:[{id,keyword:media.title,state:'succeeded',total:1,media}]}})});
 await page.route(`**/api/v1/search/${id}/media`,r=>r.fulfill({json:{data:media}}));
 await page.route(`**/api/v1/search/${id}?*`,r=>r.fulfill({json:{data:{id,keyword:media.title,state:'succeeded',sites:[],resources,total:1,media}}}));
 await fixtureLogin(page);await page.goto('/search');await expect(page.getByRole('heading',{name:'近期搜索',exact:true})).toBeVisible();await expect(page.getByRole('link',{name:'全部搜索历史',exact:true})).toHaveAttribute('href','/search?tab=history');await expect(page.locator('.acquire-workspace-nav')).toHaveCount(0);
 await page.getByLabel('TMDB媒体名称',{exact:true}).fill(media.title);await page.getByRole('button',{name:'搜索媒体',exact:true}).click();await page.getByRole('button',{name:media.title,exact:true}).click();expect(started).toBe(false);await page.getByRole('button',{name:'搜索站点资源',exact:true}).click();await expect.poll(()=>started).toBe(true);
 await expect(page).toHaveURL(new RegExp('/search/results/'+id+'$'));await expect(page.getByRole('heading',{name:'搜索结果',exact:true})).toBeVisible();await expect(page.getByLabel('TMDB媒体名称',{exact:true})).toHaveCount(0);
 await expect(page.getByText('HDSKY · H264 · WEB-DL · 1080p',{exact:true})).toBeVisible();await expect(page.getByText('50%DL',{exact:true})).toBeVisible();await expect(page.locator('.media-context').getByRole('button',{name:'订阅',exact:true})).toHaveCount(0);
 await page.screenshot({path:resolve(screenshots,'runtime-inline-search.png'),fullPage:true,animations:'disabled'});
 await page.route('**/api/v1/cache/search?*',r=>r.fulfill({json:{data:{items:[{id,query:media.title,state:'succeeded',count:1,ttlSeconds:45,reusable:true,bytes:100,posterPath:media.posterPath,createdAt:new Date().toISOString()}],total:1}}}));await page.goto('/search-cache');await expect(page.getByRole('link',{name:'查看结果',exact:true})).toBeVisible();await expect(page.locator('.acquire-history img')).toHaveAttribute('src','/api/v1/metadata/image/w500/owned.png');await expect(page.getByText(/media-tool:search-cache:/)).toHaveCount(0);
 await page.setViewportSize({width:390,height:844});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);await page.screenshot({path:resolve(screenshots,'runtime-search-cache-mobile.png'),fullPage:true,animations:'disabled'});
});

test('runtime update: overview metrics, categorized connections and separate downloader panels (fixture)',async({page})=>{
 await page.route('**/api/v1/system/runtime',r=>r.fulfill({json:{data:{server:{loadAverage:[.5,.4,.3],memoryUsed:2147483648,memoryTotal:8589934592,diskAvailable:107374182400,diskTotal:214748364800,uptimeSeconds:90000,sampledAt:new Date().toISOString()},daily:{sites:2,subscriptions:3,downloads:1,media:50,todaySearches:4,failedJobs:0}}}}));
 await fixtureLogin(page);await expect(page.getByText('100.00 GiB',{exact:true})).toBeVisible();for(const old of ['依赖服务','最近任务','资源到播放的路径'])await expect(page.getByRole('heading',{name:old,exact:true})).toHaveCount(0);await page.screenshot({path:resolve(screenshots,'runtime-dashboard.png'),fullPage:true,animations:'disabled'});
 await page.goto('/integrations');await expect(page.getByRole('region',{name:'本页服务状态'})).toBeVisible();await expect(page.getByRole('button',{name:'添加连接',exact:true})).toHaveCount(1);
 await page.goto('/downloads');await expect(page.getByRole('heading',{name:'下载任务',exact:true})).toBeVisible();await page.locator('.acquire-workspace .page-tabs').getByRole('button',{name:'上传入库',exact:true}).click();await expect(page).toHaveURL(/tab=uploads/);await expect(page.getByRole('heading',{name:'115上传与入库',exact:true})).toBeVisible();await page.screenshot({path:resolve(screenshots,'runtime-downloads.png'),fullPage:true,animations:'disabled'});
});

test('runtime update: configured proxy origin can log in but spoofed forwarding and missing CSRF fail (isolated)',async({page})=>{
 await fixtureLogin(page);
 const allowed=await page.request.post('/api/v1/auth/login',{headers:{Origin:'https://proxy.fixture.invalid','X-Requested-With':'media-tool'},data:{username,password}});expect(allowed.status()).toBe(200);
 const hostile=await page.request.post('/api/v1/auth/login',{headers:{Origin:'https://hostile.fixture.invalid','X-Requested-With':'media-tool','X-Forwarded-Host':'proxy.fixture.invalid','X-Forwarded-Proto':'https'},data:{username,password}});expect(hostile.status()).toBe(403);
 const missing=await page.request.post('/api/v1/auth/login',{headers:{Origin:'https://proxy.fixture.invalid'},data:{username,password}});expect(missing.status()).toBe(403);
});

test('acquisition: dedicated results survive reload and every history entry navigates there (fixture)',async({page})=>{
 const id='owned-standalone';await page.route('**/api/v1/search',r=>r.fulfill({json:{data:[{id,keyword:'Owned dedicated',state:'succeeded',total:1}]}}));await page.route(`**/api/v1/search/${id}?*`,r=>r.fulfill({json:{data:{id,keyword:'Owned dedicated',state:'succeeded',sites:[],resources:[],total:0}}}));await fixtureLogin(page);await page.goto('/search');await expect(page.getByRole('link',{name:'管理PT站点账号',exact:true})).toHaveCount(0);await page.getByRole('link',{name:'查看结果',exact:true}).click();await expect(page).toHaveURL(/\/search\/results\/owned-standalone$/);await expect(page.locator('h1')).toHaveText('搜索结果');await expect(page.getByRole('radiogroup',{name:'搜索方式'})).toHaveCount(0);await page.reload();await expect(page.locator('h1')).toHaveText('搜索结果');await page.getByRole('link',{name:'返回搜索',exact:true}).click();await expect(page).toHaveURL(/\/search\?tab=query$/);
});

test('cache maintenance links to settings-owned TTL while search history stays read-only with meaningful expiry (fixture)',async({page})=>{
 const id='22222222-2222-4222-8222-222222222222';let kind='tmdb';await page.route('**/api/v1/cache/**',r=>r.fulfill({json:{data:{items:[{id:kind==='tmdb'?'a'.repeat(64):id,query:'Owned cache',state:'succeeded',count:2,bytes:9999,ttlSeconds:7200,reusable:true}],total:1}}}));await fixtureLogin(page);await page.goto('/tmdb-cache');await expect(page.getByRole('columnheader',{name:'大小',exact:true})).toHaveCount(0);await expect(page.getByRole('heading',{name:'缓存有效期',exact:true})).toHaveCount(0);await expect(page.getByRole('link',{name:'设置 · 搜索订阅',exact:true})).toHaveAttribute('href','/settings?tab=automation');await expect(page.locator('.u6-cache-content').getByText(/2\.00 小时/)).toBeVisible();await expect(page.getByRole('button',{name:'缓存详情',exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'删除缓存项',exact:true})).toBeVisible();kind='search';await page.goto('/search-cache');await expect(page.getByRole('columnheader',{name:'大小',exact:true})).toHaveCount(0);await expect(page.getByRole('link',{name:'查看结果',exact:true})).toBeVisible();await page.setViewportSize({width:390,height:844});await expect(page.locator('.acquire-history').getByRole('link',{name:'查看结果',exact:true})).toBeVisible();await page.screenshot({path:resolve(screenshots,'acquire-cache-mobile.png'),fullPage:true,animations:'disabled'});
});

test('acquisition: proxy settings are scoped, validated and persisted in isolated database',async({page})=>{
 await fixtureLogin(page);await page.goto('/settings?tab=proxy');await expect(page.getByRole('heading',{name:'网络代理',exact:true})).toBeVisible();await page.getByLabel('代理地址',{exact:true}).fill('http://127.0.0.1:7890');await page.getByRole('button',{name:'保存代理设置',exact:true}).click();await expect(page.getByText('代理设置已保存，新的请求使用所选路由。',{exact:true})).toBeVisible();await page.reload();await expect(page.getByLabel('代理地址',{exact:true})).toHaveValue('');await expect(page.getByRole('checkbox',{name:'TMDB查询与海报使用代理',exact:true})).not.toBeChecked();await page.getByLabel('代理地址',{exact:true}).fill('file:///unsafe');await page.getByRole('button',{name:'保存代理设置',exact:true}).click();await expect(page.getByText(/代理地址须使用/)).toBeVisible();await page.getByLabel('代理地址',{exact:true}).fill('');await page.getByText('明确清除已保存的代理地址',{exact:true}).click();await expect(page.getByRole('checkbox',{name:'明确清除已保存的代理地址',exact:true})).toBeChecked();await page.getByRole('button',{name:'保存代理设置',exact:true}).click();await expect(page.getByText('代理设置已保存，新的请求使用所选路由。',{exact:true})).toBeVisible();
});

test('acquisition: subscriptions reject HR without unknown override and expose history (fixture)',async({page})=>{
 await page.route('**/api/v1/subscriptions**',r=>r.fulfill({json:{data:[]}}));await page.route('**/api/v1/integrations',r=>r.fulfill({json:{data:[{id:'owned-account',name:'Owned Site',kind:'ptsite',enabled:true,secretFields:[]}]}}));await page.route('**/api/v1/search/media?*',r=>r.fulfill({json:{data:{items:[{id:42,mediaType:'movie',title:'Owned Film'}],cached:false,cacheSeconds:60}}}))
 await fixtureLogin(page);await page.goto('/subscriptions');await page.locator('.mt-subscriptions .page-tabs').getByRole('button',{name:'运行记录',exact:true}).click();await expect(page).toHaveURL(/tab=history/);await expect(page.getByRole('combobox',{name:'订阅范围',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'新建订阅',exact:true}).click();const dialog=page.getByRole('dialog',{name:'新建订阅',exact:true});await dialog.getByLabel('订阅TMDB关键词',{exact:true}).fill('Owned Film');await dialog.getByRole('button',{name:'查询TMDB',exact:true}).click();await dialog.getByRole('group',{name:'匹配影视',exact:true}).getByRole('button').click();await dialog.getByRole('button',{name:'选择站点与规则',exact:true}).click();
 await expect(dialog.getByRole('checkbox',{name:'拒绝下载H&R资源',exact:true})).toBeChecked();await expect(dialog.getByText(/明确允许H/)).toHaveCount(0);await dialog.getByRole('button',{name:'关闭此对话框',exact:true}).click()
})

test('files and library readonly capabilities, persistent batch proof, scan controls and stage history (UI fixtures)',async({page})=>{
 await fixtureLogin(page)
 const id='12345678-1234-1234-1234-123456789012'
 const readonly={read:true,write:false,move:false,recycle:false,readOnly:true,role:'download-source',reason:'下载及做种原文件只读保留'}
 await page.route('**/api/v1/files/roots',r=>r.fulfill({json:{data:[{id,name:'自动发现的本地目录',kind:'local',path:'/data',revision:1,capabilities:readonly,connectionState:'accessible'}]}}))
 await page.route('**/api/v1/files?**',r=>r.fulfill({json:{data:{items:[{id:'inode',name:'Owned.Show.S01E01.mkv',path:'downloads/Owned.Show.S01E01.mkv',size:1024,directory:false,capabilities:readonly}],total:1,capabilities:readonly}}}))
 await page.route('**/api/v1/files/operations',r=>r.fulfill({json:{data:[]}}))
 await page.goto('/files');await expect(page.getByRole('button',{name:'新建文件夹',exact:true})).toBeDisabled();await page.getByRole('button',{name:'Owned.Show.S01E01.mkv 更多',exact:true}).click();await expect(page.getByRole('menuitem',{name:'影视整理',exact:true})).toHaveAttribute('aria-disabled','true');await page.keyboard.press('Escape')
 await page.setViewportSize({width:390,height:844});await expect(page.getByLabel('选择 Owned.Show.S01E01.mkv')).toBeDisabled();await expect(page.getByRole('navigation',{name:'文件路径',exact:true}).getByRole('button',{name:'根目录',exact:true})).toBeVisible();expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await page.screenshot({path:resolve(screenshots,'files-library-readonly-mobile.png'),animations:'disabled',fullPage:true})
 await page.route('**/api/v1/library/scans',r=>r.fulfill({json:{data:[{id,root:'/115/Media',state:'paused',files:2,directories:4,completedDirectories:1,generation:2,revision:1,message:'不完整覆盖不用于缺失判断'}]}}))
 await page.goto('/cloudStorage');await page.locator('.media-workspace .page-tabs').getByRole('button',{name:'扫描',exact:true}).click();await expect(page.getByRole('button',{name:'重新完整对账',exact:true})).toBeVisible();expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await page.screenshot({path:resolve(screenshots,'files-library-scans-mobile.png'),animations:'disabled',fullPage:true})
 await page.route('**/api/v1/library/activity?**',r=>r.fulfill({json:{data:{items:[{id,kind:'file',state:'needs_reconcile',source:'Owned.mkv',batchId:'',connectionId:id,message:'只读回核对，不重放',createdAt:new Date().toISOString(),updatedAt:new Date().toISOString(),details:{completedSteps:0,plan:{steps:[{action:'relocate',source:{path:'Owned.mkv'},target:'Owned (2026).mkv'}],items:[{source:{path:'Owned.mkv'},target:'Owned (2026).mkv'}]}}}],total:1}}}))
 await page.goto('/organize-history');await page.getByRole('button',{name:'查看步骤详情',exact:true}).click();await expect(page.getByRole('dialog').getByRole('button',{name:'核对结果（不重放）',exact:true})).toBeVisible();await expect(page.getByRole('dialog')).toContainText('Owned (2026).mkv');expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await page.screenshot({path:resolve(screenshots,'files-library-history-mobile.png'),animations:'disabled',fullPage:false})
})


test('isolated local batch preview writes nothing until explicit approval and survives history reload',async({page})=>{
 await fixtureLogin(page)
 const folder=process.env.E2E_FILE_ROOT!
 const names=['Batch.Owned.Show.2026.S01E01.mkv','Batch.Owned.Show.2026.S01E02.mkv']
 for(const name of names)writeFileSync(resolve(folder,name),'Isolated batch fixture, not real media.')
 await page.goto('/files')
 for(const name of names){const row=page.locator('.files-entry').filter({hasText:name});await row.getByRole('checkbox').check()}
 await page.getByRole('button',{name:'批量整理预览（2）',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'批次整理工作台',exact:true})
 await expect(dialog.getByLabel('批次文件标题').first()).toHaveValue('Batch Owned Show')
 await dialog.getByRole('button',{name:'预览全部变更',exact:true}).click()
 await expect(dialog.getByRole('button',{name:'明确批准整个批次',exact:true})).toBeEnabled()
 for(const name of names)expect(existsSync(resolve(folder,name))).toBe(true)
 await dialog.screenshot({path:resolve(screenshots,'files-library-batch-preview-desktop.png'),animations:'disabled'})
 await page.setViewportSize({width:390,height:844});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await dialog.screenshot({path:resolve(screenshots,'files-library-batch-preview-mobile.png'),animations:'disabled'})
 await dialog.getByRole('button',{name:'明确批准整个批次',exact:true}).click();await page.getByRole('dialog',{name:'批准文件批次',exact:true}).getByRole('button',{name:'确认批准',exact:true}).click()
 await expect(dialog.getByRole('button',{name:'核对结果',exact:true})).toBeVisible()
 const batchLink=await dialog.getByRole('link',{name:'查看持久化批次',exact:true}).getAttribute('href');const batchID=new URL('http://localhost:18084'+batchLink).searchParams.get('batchId')!
 await expect.poll(async()=>{const response=await page.request.get(`/api/v1/files/batches/${batchID}`);return (await response.json()).data.state},{timeout:15000}).toBe('succeeded')
 await dialog.getByRole('button',{name:'核对结果',exact:true}).click();await expect(dialog.getByText(/已完成/).first()).toBeVisible()
 for(const name of names)expect(existsSync(resolve(folder,name))).toBe(false)
 await dialog.getByRole('link',{name:'查看持久化批次',exact:true}).click();await expect(page.getByRole('heading',{name:'文件与入库历史',exact:true})).toBeVisible();await page.reload();await expect(page.getByRole('button',{name:'查看步骤详情',exact:true}).first()).toBeVisible()
})

test('cloud page exposes honest quota and membership while library tabs keep metadata query usable at 390px (UI fixture)',async({page})=>{
 await fixtureLogin(page)
 await page.route('**/api/v1/cloud/accounts/info',r=>r.fulfill({json:{data:{connectionId:'owned',note:'挂载账户范围',info:{path:'/115/Owned',source:'115 官方开放平台 /open/user/info',checkedAt:'2026-10-14T00:00:00Z',space:{total:1099511627776,used:549755813888,free:549755813888},memberships:[{identity:'115VIP',level:'VIP',expiresAt:'2027-01-01T00:00:00Z'}]}}}}))
 await page.goto('/cloud-connections');await expect(page.getByRole('heading',{name:'空间与会员',exact:true})).toBeVisible();await expect(page.getByRole('region',{name:'云盘空间和会员'})).toContainText('1.00 TiB');await expect(page.getByRole('region',{name:'云盘空间和会员'})).toContainText('2027');await page.screenshot({path:resolve(screenshots,'cloud-account-desktop.png'),animations:'disabled',fullPage:true})
 await page.route('**/api/v1/media/assets?page=*',r=>r.fulfill({json:{data:{items:[{id:'00000000-0000-4000-8000-000000000141',title:'自有电影',mediaType:'movie',path:'/115/Owned/owned.mkv',revision:1,artifactsState:'ready',state:'present',fileScoped:true}],total:1}}}))
 await page.route('**/api/v1/metadata/search?*',r=>r.fulfill({json:{data:[{id:11,title:'自有电影',year:'2026',mediaType:'movie'}]}}))
 await page.goto('/cloudStorage');await expect(page.getByRole('heading',{name:'云盘连接与115授权',exact:true})).toHaveCount(0);await page.locator('.media-workspace .page-tabs').getByRole('button',{name:'已入库',exact:true}).click();await page.getByRole('button',{name:'修正身份 / 季集',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'修正媒体身份与季集',exact:true});await expect(dialog.getByLabel('媒体关键词',{exact:true})).toHaveValue('自有电影');await expect(dialog.getByLabel('媒体关键词',{exact:true})).toBeInViewport({ratio:1});await dialog.getByRole('button',{name:'查询TMDB',exact:true}).click();await expect(dialog.getByRole('button').filter({hasText:'自有电影'})).toBeVisible()
 await dialog.screenshot({path:resolve(screenshots,'metadata-query-desktop.png'),animations:'disabled'});await page.setViewportSize({width:390,height:844});await expect(dialog.getByLabel('媒体关键词',{exact:true})).toBeInViewport({ratio:1});await expect(dialog.getByRole('button',{name:'查询TMDB',exact:true})).toBeInViewport({ratio:1});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await dialog.screenshot({path:resolve(screenshots,'metadata-query-mobile.png'),animations:'disabled'})
})

test('folder naming and scraping settings persist while history removal cannot delete media (isolated)',async({page})=>{
 await fixtureLogin(page);await page.goto('/settings?tab=library');const movie='{title} ({year})/{title} ({year}) - {videoFormat}',series='{title} ({year})/Season {season}/{title} - {season_episode} - 第{episode}集'
 await page.getByLabel('电影命名模板',{exact:true}).fill(movie);await page.getByLabel('剧集命名模板',{exact:true}).fill(series);await page.getByRole('button',{name:'预览命名模板',exact:true}).click();await expect(page.getByRole('region',{name:'命名实时预览'})).toContainText('示例剧集 (2026)/Season 01/示例剧集 - S01E04 - 第04集.mkv')
 await page.getByRole('group',{name:'入库时生成的资料',exact:true}).locator('.settings-grid > div').filter({has:page.getByRole('heading',{name:'电影',exact:true})}).getByRole('checkbox',{name:'海报',exact:true}).check();await page.getByRole('button',{name:'保存命名与入库设置',exact:true}).click();await expect(page.getByText(/当前分区已保存/)).toBeVisible();const prefs=(await(await page.request.get('/api/v1/settings/application')).json()).data;expect(prefs.value.scraping.movie.poster).toBe(true)
 await page.screenshot({path:resolve(screenshots,'scraping-settings-desktop.png'),animations:'disabled',fullPage:true});await page.setViewportSize({width:390,height:844});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,'scraping-settings-mobile.png'),animations:'disabled',fullPage:true})
 // Restore isolated policy before other tests; never change production settings.
 prefs.value.scraping.movie.poster=false;expect((await page.request.put('/api/v1/settings/application',{headers:{'X-Requested-With':'media-tool'},data:prefs})).status()).toBe(200)
 await page.goto('/organize-history');const batch=page.locator('.organize-record').filter({hasText:'整理批次'}).filter({hasText:'已完成'}).first();await expect(batch).toBeVisible();const selectedRecord=await batch.elementHandle();await batch.getByRole('checkbox').check();await page.getByRole('button',{name:/移除所选历史/}).click();const confirm=page.getByRole('dialog',{name:'移除整理历史',exact:true});await expect(confirm).toContainText('不删除媒体');const receipt=page.waitForResponse(r=>r.url().endsWith('/api/v1/library/activity/remove')&&r.request().method()==='POST');await confirm.getByRole('button',{name:'明确移除历史',exact:true}).click();const removed=await receipt;expect(removed.status()).toBe(200);const removedId=removed.request().postDataJSON().items[0].id;await expect.poll(()=>selectedRecord!.evaluate(el=>el.isConnected)).toBe(false);const records=await page.request.get('/api/v1/library/activity?kind=batch&page=1&pageSize=100');expect((await records.json()).data.items.some((r:any)=>r.id===removedId)).toBe(false)
})

test('native media boundary is explicit and responsive without changing Emby',async({page})=>{
 await fixtureLogin(page)
 await page.goto('/media-server')
 await expect(page.getByRole('heading',{name:'播放服务',exact:true})).toBeVisible()
 await expect(page.getByText('服务器不中转流量。',{exact:true})).toBeVisible()
 await expect(page.getByRole('link',{name:'浏览 Emby 媒体库'})).toHaveCount(0);await expect(page.getByRole('link',{name:'管理媒体用户',exact:true})).toHaveAttribute('href','/playback?tab=users')
 for(const width of [375,768,1440]){
  await page.setViewportSize({width,height:900})
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
  await page.screenshot({path:resolve(screenshots,`native-media-boundary-${width}.png`),fullPage:true})
 }
 await page.goto('/media-users')
 await expect(page.getByRole('heading',{name:'媒体用户',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'新建媒体用户',exact:true}).click()
 await expect(page.getByRole('dialog')).toBeVisible()
 await expect(page.getByRole('checkbox',{name:'要求先修改密码',exact:true})).not.toBeChecked()
 await expect(page.getByRole('dialog')).toContainText('初始密码')
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
 await page.getByRole('button',{name:'取消',exact:true}).click()
})

test('player UA observations can be approved and enforce whitelist before all playback paths',async({page})=>{
 await fixtureLogin(page)
 const ua='Owned-E2E-Player/1.0'
 const observed=await page.request.get('/api/v1/cloudStorage/strm302?apiKey=owned-invalid-fixture-key&path=/115/OwnedFixture.mkv',{headers:{'User-Agent':ua},maxRedirects:0})
 expect(observed.status()).toBe(401)
 await page.goto('/player-access')
 await expect(page.getByRole('heading',{name:'播放器准入',exact:true})).toBeVisible()
 await expect(page.getByText('当前仅记录访问，不按UA限制现有播放器。',{exact:true})).toBeVisible()
 await page.getByLabel('搜索UA记录',{exact:true}).fill(ua)
 await page.getByRole('button',{name:'搜索记录',exact:true}).click()
 const row=page.locator('.u5-ua-record').filter({hasText:ua})
 await expect(row).toBeVisible()
 await row.getByRole('button',{name:'查看完整 UA',exact:true}).click()
 let dialog=page.getByRole('dialog',{name:'完整 UA 与访问详情',exact:true})
 await expect(dialog.getByLabel('实际收到的完整 UA',{exact:true})).toHaveValue(ua)
 await dialog.getByRole('button',{name:'从此记录加入白名单',exact:true}).click()
 dialog=page.getByRole('dialog',{name:'从访问记录加入白名单',exact:true})
 await expect(dialog.getByLabel('完整 User-Agent',{exact:true})).toHaveValue(ua)
 await expect(dialog.getByLabel('完整 User-Agent',{exact:true})).toHaveAttribute('readonly','')
 await dialog.getByLabel('播放器备注',{exact:true}).fill('Owned E2E player')
 await dialog.getByRole('button',{name:'保存规则',exact:true}).click()
 await expect(page.getByText('Owned E2E player',{exact:true})).toBeVisible()
 await expect(page.getByRole('button',{name:'启用 UA 限制',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'启用 UA 限制',exact:true}).click()
 await page.getByRole('dialog',{name:'启用播放器白名单',exact:true}).getByRole('button',{name:'启用限制',exact:true}).click()
 await expect(page.getByRole('button',{name:'关闭 UA 限制',exact:true})).toBeVisible()
 try{
  const blocked=await page.request.get('/api/v1/cloudStorage/strm302?apiKey=owned-invalid-fixture-key',{headers:{'User-Agent':'Unknown-E2E-Player/2'},maxRedirects:0})
  expect(blocked.status()).toBe(403);expect(blocked.headers().location).toBeUndefined();expect((await blocked.json()).error.code).toBe('player_ua_denied')
  const allowed=await page.request.get('/api/v1/cloudStorage/strm302?apiKey=owned-invalid-fixture-key',{headers:{'User-Agent':ua},maxRedirects:0});expect(allowed.status()).toBe(401)
  for(const path of ['/Videos/11/original.mkv','/Items/11/PlaybackInfo','/emby/VIDEOS/11/stream']){
   const reply=await page.request.get('http://localhost:18085'+path,{headers:{'User-Agent':'Unknown-E2E-Player/2'},maxRedirects:0});expect(reply.status()).toBe(403);expect(reply.headers().location).toBeUndefined()
  }
  await expect(page.locator('.el-message')).toHaveCount(0,{timeout:6000})
  for(const width of [375,768,1440]){
   await page.setViewportSize({width,height:900});await page.getByRole('button',{name:'刷新',exact:true}).click()
   expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
   await page.screenshot({path:resolve(screenshots,`player-ua-access-${width}.png`),fullPage:true})
  }
 }finally{
  const current=await page.request.get('/api/v1/player-access/policy');const p=(await current.json()).data
  const disabled=await page.request.put('/api/v1/player-access/policy',{headers:{'X-Requested-With':'media-tool'},data:{enabled:false,revision:p.revision,rules:p.rules}});expect(disabled.status()).toBe(200)
 }
 await page.getByRole('button',{name:'刷新',exact:true}).click()
 await page.getByRole('button',{name:'清空访问记录',exact:true}).click()
 await page.getByRole('dialog',{name:'清理 UA 访问记录',exact:true}).getByRole('button',{name:'清理记录',exact:true}).click()
 await expect(page.getByText('Owned E2E player',{exact:true})).toBeVisible()
 const p=(await(await page.request.get('/api/v1/player-access/policy')).json()).data
 expect(p.rules).toHaveLength(1);expect(p.enabled).toBe(false)
 expect((await page.request.put('/api/v1/player-access/policy',{headers:{'X-Requested-With':'media-tool'},data:{enabled:false,revision:p.revision,rules:[]}})).status()).toBe(200)
})

test('subscription tabs have separation and PT metrics actions are centered',async({page})=>{
 await page.route('**/api/v1/integrations',r=>r.fulfill({json:{data:[{'id': 'owned-center', 'name': 'HDSKY', 'kind': 'ptsite', 'enabled': true, 'settings': {'definitionId': 'hdsky'}}]}}))
 await fixtureLogin(page);await page.goto('/subscriptions')
 const tabs=page.locator('.mt-subscriptions .page-tabs');const table=page.locator('.subscription-commandbar');await expect(tabs).toBeVisible();await expect(table).toBeVisible()
 const bottom=await tabs.evaluate(el=>el.getBoundingClientRect().bottom);const top=await table.evaluate(el=>el.getBoundingClientRect().top);expect(top-bottom).toBeGreaterThanOrEqual(20)
 await page.route('**/api/v1/sites/metrics',r=>r.fulfill({json:{data:[{accountId:'owned-center',name:'HDSKY',enabled:true,metrics:{uploaded:1,downloaded:0,seeding:2,seedingBytes:1024},state:'succeeded',stale:false}]}}))
 await page.goto('/sites');const metric=page.locator('.u5-metric-card').first();await expect(metric).toBeVisible();expect(await metric.locator('.media-actions').evaluate(el=>getComputedStyle(el).alignItems)).toBe('center')
 await page.screenshot({path:resolve(screenshots,'workbench-site-centered.png'),fullPage:true})
})

test('STRM directory has posters and no NFO collection while tools persist repair previews',async({page})=>{
 const asset='00000000-0000-4000-8000-000000000246',repair='00000000-0000-4000-8000-000000000247'
 await page.route('**/api/v1/library/outputs?*',r=>r.fulfill({json:{data:{items:[{id:asset,title:'自有STRM电影',year:'2026',mediaType:'movie',source:'/115/Owned/film.mkv',poster:'/api/v1/media/tmdb/image?path=/owned.png',cloudState:'present',artifactsState:'ready',revision:1,outputs:[{kind:'strm',path:'Movies/film.strm',owner:'asset:'+asset},{kind:'nfo',path:'Movies/film.nfo',owner:'asset:'+asset}],embyState:'pending',embyPaths:[],revoked:false}],total:1}}}))
 await fixtureLogin(page);await page.goto('/strm');await expect(page.getByRole('heading',{name:'STRM 目录',exact:true})).toBeVisible();await expect(page.getByRole('heading',{name:'生成链接预览',exact:true})).toHaveCount(0)
 await page.getByText('本页资产与输出',{exact:true}).click();await page.getByRole('button',{name:'资产与输出详情',exact:true}).click();await expect(page.getByRole('dialog')).toContainText('Movies/film.strm');await expect(page.getByRole('img',{name:'自有STRM电影',exact:true})).toBeVisible();await expect(page.getByRole('dialog')).not.toContainText('Movies/film.nfo');await page.getByRole('dialog').getByRole('button',{name:'关闭',exact:true}).click();await expect(page.locator('.el-overlay:visible')).toHaveCount(0)
 for(const width of [375,1440]){await page.setViewportSize({width,height:900});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`workbench-strm-directory-${width}.png`),fullPage:true})}
 let approved=false
 const batch=()=>({id:repair,digest:'d'.repeat(64),state:approved?'succeeded':'preview',expiresAt:'2099-01-01T00:00:00Z',items:[{ordinal:0,assetId:asset,path:'Movies/film.strm',digest:'e'.repeat(64),state:approved?'repaired':'missing',message:'只恢复原文'}]})
 await page.route('**/api/v1/strm/repairs/preview',r=>{expect(r.request().postDataJSON()).toEqual({assetIds:[asset]});return r.fulfill({json:{data:batch()}})})
 await page.route(`**/api/v1/strm/repairs/${repair}`,r=>r.fulfill({json:{data:batch()}}))
 await page.route(`**/api/v1/strm/repairs/${repair}/approve`,r=>{expect(r.request().postDataJSON()).toEqual({digest:'d'.repeat(64),confirm:true});approved=true;return r.fulfill({json:{data:batch()}})})
 await page.goto('/strm-tools');await expect(page.getByRole('heading',{name:'播放链路诊断',exact:true})).toBeVisible();await page.getByRole('button',{name:'选择受管资产',exact:true}).click();const selector=page.locator('.files-repair-picker');await expect(selector).toBeVisible();await selector.getByRole('checkbox').check();await selector.getByRole('button',{name:'检查已选 1 个资产',exact:true}).click();await expect(page).toHaveURL(/repairId=/);expect(approved).toBe(false)
 await page.reload();await expect(page.getByRole('button',{name:'批准批量修复',exact:true})).toBeVisible();await page.getByRole('button',{name:'批准批量修复',exact:true}).click();await page.getByRole('dialog',{name:'批准 STRM 修复',exact:true}).getByRole('button',{name:'明确批准修复',exact:true}).click();await page.getByRole('button',{name:'刷新',exact:true}).click();await expect(page.getByRole('heading',{name:'修复完成',exact:true})).toBeVisible();await expect(page.locator('.el-overlay:visible')).toHaveCount(0)
 await page.screenshot({path:resolve(screenshots,'workbench-strm-tools-repair.png'),fullPage:true})
})

test('UA visits expose IP and verified Emby users without inferring unknown callers',async({page})=>{
 await page.route('**/api/v1/player-access/events?*',r=>r.fulfill({json:{data:{items:[{id:'owned-visit',userAgent:'Owned-Player/1',clientIp:'192.0.2.50',entry:'emby',method:'GET',allowed:true,reason:'restriction_disabled',status:302,embyUserId:'actual-user',embyUserName:'已核验用户',embyVerified:true,createdAt:'2026-10-14T00:00:00Z'},{id:'unknown-visit',userAgent:'Owned-Player/1',clientIp:'192.0.2.51',entry:'strm',method:'HEAD',allowed:true,reason:'restriction_disabled',status:401,embyVerified:false,createdAt:'2026-10-14T00:00:01Z'}],total:2}}}))
 await fixtureLogin(page);await page.goto('/player-access');await expect(page.getByText('已核验用户',{exact:true})).toBeVisible();await expect(page.getByRole('heading',{name:'未核验用户',exact:true})).toBeVisible();await expect(page.getByText(/192\.0\.2\.50/)).toBeVisible();await page.locator('.organize-record').filter({hasText:'已核验用户'}).getByRole('button',{name:'访问详情',exact:true}).click();await page.getByRole('button',{name:/技术细节与原始记录/}).click();await expect(page.getByRole('dialog').locator('pre')).toContainText('actual-user');await page.screenshot({path:resolve(screenshots,'workbench-ua-users.png'),fullPage:true})
})

test('search promotion header sorts full results and history evidence starts folded',async({page})=>{
 let sort='';const run='00000000-0000-4000-8000-000000000248'
 await page.route(`**/api/v1/search/${run}?*`,r=>{sort=new URL(r.request().url()).searchParams.get('sort')||'';return r.fulfill({json:{data:{id:run,keyword:'Owned',state:'succeeded',siteResults:[],resources:[],total:0}}})})
 await fixtureLogin(page);await page.goto('/search/results/'+run);await expect(page.getByRole('columnheader',{name:/促销/})).toBeVisible();await page.getByRole('columnheader',{name:/促销/}).click();await expect.poll(()=>sort).toBe('promotion_desc');await page.getByRole('columnheader',{name:/促销/}).click();await expect.poll(()=>sort).toBe('promotion_asc')
 await page.route('**/api/v1/library/activity?*',r=>r.fulfill({json:{data:{items:[{id:'owned-history',kind:'output',state:'applied',source:'Movies/Owned.strm',message:'输出已写入',createdAt:'2026-10-14T00:00:00Z',updatedAt:'2026-10-14T00:00:00Z',details:{operation:'write',kind:'strm'}}],total:1}}}))
 await page.goto('/organize-history');await expect(page.getByRole('navigation',{name:'历史阶段快捷筛选',exact:true})).toBeVisible();await page.getByRole('button',{name:'查看步骤详情',exact:true}).click();await expect(page.getByRole('dialog').locator('pre')).toBeHidden();await page.getByRole('button',{name:/技术细节与原始记录/}).click();await expect(page.getByRole('dialog').locator('pre')).toContainText('write');await page.getByRole('button',{name:'关闭',exact:true}).click();await expect(page.locator('.el-overlay:visible')).toHaveCount(0);await page.setViewportSize({width:375,height:900});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,'workbench-history-375.png'),fullPage:true})
})

test('STRM repair history stage is accepted by the actual protected API',async({page})=>{
 await fixtureLogin(page);await page.goto('/organize-history')
 const response=page.waitForResponse(r=>r.url().includes('/api/v1/library/activity?')&&new URL(r.url()).searchParams.get('kind')==='strmRepair')
 await page.getByRole('navigation',{name:'历史阶段快捷筛选',exact:true}).getByRole('button',{name:'STRM修复',exact:true}).click()
 expect((await response).status()).toBe(200)
 await expect(page.getByRole('alert')).toHaveCount(0)
})

test('known search TMDB identity is echoed and submitted without another lookup (fixture)',async({page})=>{
 await fixtureLogin(page)
 const runId='direct-identity-run',resourceId='identity-resource'
 const media={id:1381078,mediaType:'movie',title:'一生所爱',originalTitle:'Love of Your Life',year:'2026'}
 await page.route('**/api/v1/integrations',r=>r.fulfill({json:{data:[]}}))
 await page.route('**/api/v1/search',r=>r.fulfill({json:{data:[]}}))
 await page.route(`**/api/v1/search/${runId}?*`,r=>r.fulfill({json:{data:{id:runId,keyword:'一生所爱',state:'succeeded',media,sites:[],resources:[{id:resourceId,title:'Love.of.Your.Life.2026.1080p.WEB-DL.H264',siteName:'Owned',size:100,seeders:0,downloadFactor:0,metadata:{}}],total:1}}}))
 let lookup=0,submitted:Record<string,unknown>|undefined
 await page.route('**/api/v1/metadata/search?*',r=>{lookup++;return r.fulfill({json:{data:[]}})})
 await page.route('**/api/v1/downloads',r=>{submitted=r.request().postDataJSON();return r.fulfill({json:{data:{id:'owned-task'}}})})
 await page.goto(`/search/results/${runId}`);await page.getByRole('button',{name:'确认并下载',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'确认媒体身份与下载'});await expect(dialog.getByRole('heading',{name:'已使用搜索中的媒体身份'})).toBeVisible();await expect(dialog.getByText('一生所爱',{exact:true})).toBeVisible();await expect(dialog.getByRole('button',{name:'查询 TMDB'})).toHaveCount(0)
 for(const width of [1440,375]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`download-search-identity-${width}.png`),fullPage:true,animations:'disabled'})}
 await dialog.getByRole('button',{name:'创建下载任务',exact:true}).click();await expect.poll(()=>submitted?.tmdbId).toBe(1381078);expect(submitted?.searchRunId).toBe(runId);expect(submitted?.correctMediaIdentity).toBe(false);expect(lookup).toBe(0)
})

test('download zero, unknown and sync errors are distinct; refresh observes only (fixture)',async({page})=>{
 await fixtureLogin(page)
 const tasks=[{id:'zero',title:'真实0进度',state:'downloading',seedingState:'pending',progress:0,qbState:'stalledDL',downloadSpeed:0,uploadSpeed:0,seeds:0,peers:0,syncedAt:'2026-10-14T00:00:00Z',infoHash:'owned-hash',media:{title:'电影',year:'2026'}},{id:'unknown',title:'尚未核对的任务',state:'pending',progress:null,media:{}},{id:'stale',title:'保留最后有效进度',state:'downloading',progress:0.42,qbState:'downloading',syncError:'torrent not found in qBittorrent',media:{}}]
 await page.route('**/api/v1/downloads',r=>r.fulfill({json:{data:tasks}}));await page.route('**/api/v1/transfers',r=>r.fulfill({json:{data:[]}}));await page.route('**/api/v1/integrations',r=>r.fulfill({json:{data:[]}}))
 let sync=0;await page.route('**/api/v1/downloads/sync',r=>{expect(r.request().method()).toBe('POST');sync++;return r.fulfill({json:{data:{message:''}}})})
 await page.goto('/downloads');await expect(page.getByText('下载中 · 下载停滞（等待有效连接）',{exact:true})).toBeVisible();await expect(page.getByText('尚未同步',{exact:true})).toBeVisible();await expect(page.getByText('同步失败（保留上次进度）：torrent not found in qBittorrent',{exact:true})).toBeVisible()
 await page.getByRole('button',{name:'刷新',exact:true}).click();expect(sync).toBe(0);await page.getByRole('button',{name:'同步下载任务',exact:true}).click();await expect.poll(()=>sync).toBe(1)
 for(const width of [1440,375]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`download-telemetry-${width}.png`),fullPage:true})}
})

test('Emby user filter uses verified IDs and echoes public address provenance (fixture)',async({page})=>{
 await fixtureLogin(page)
 await page.route('**/api/v1/player-access/policy',r=>r.fulfill({json:{data:{enabled:false,revision:1,rules:[]}}}));await page.route('**/api/v1/player-access/observations?*',r=>r.fulfill({json:{data:{items:[],total:0}}}));await page.route('**/api/v1/player-access/users',r=>r.fulfill({json:{data:[{id:'verified-id',name:'已核验用户'}]}}))
 const seen:string[]=[];await page.route('**/api/v1/player-access/events?*',r=>{seen.push(new URL(r.request().url()).searchParams.get('embyUserId')??'');return r.fulfill({json:{data:{items:[{id:'visit',userAgent:'Owned Player',clientIp:'8.8.8.8',connectionIp:'10.0.0.2',ipSource:'trusted_proxy',ipPublic:true,entry:'emby',method:'GET',status:302,embyVerified:true,embyUserId:'verified-id',embyUserName:'已核验用户',createdAt:'2026-10-14T00:00:00Z'}],total:1}}})})
 await page.goto('/player-access');await expect(page.getByText(/访客地址 8\.8\.8\.8.*公网地址/)).toBeVisible();await expect(page.getByText(/连接地址 10\.0\.0\.2/)).toBeVisible()
 await page.getByRole('combobox',{name:'Emby访问用户筛选',exact:true}).selectOption('verified-id');await expect.poll(()=>seen.at(-1)).toBe('verified-id');await page.screenshot({path:resolve(screenshots,'public-ip-user-filter.png'),fullPage:true})
})

test('cloud index bulk organizer echoes saved identity, templates and frozen file proof (fixture)',async({page})=>{
 await fixtureLogin(page)
 const candidateId='00000000-0000-4000-8000-000000000101',assetId='00000000-0000-4000-8000-000000000102',rootId='00000000-0000-4000-8000-000000000103',batchId='owned-batch'
 const source='Show.2020.S01E01.1080p.mkv'
 await page.route('**/api/v1/library/candidates?*',r=>r.fulfill({json:{data:{items:[{id:candidateId,name:source,path:'/115/'+source,size:5,state:'needs_confirmation',revision:1,cloudState:'present'}],total:1}}}));await page.route('**/api/v1/media/assets?*',r=>r.fulfill({json:{data:{items:[],total:0}}}));for(const path of ['scans','schedules'])await page.route(`**/api/v1/library/${path}`,r=>r.fulfill({json:{data:[]}}))
 let selected:Record<string,unknown>|undefined,preview:Record<string,any>|undefined
 await page.route('**/api/v1/library/organize-selection',r=>{selected=r.request().postDataJSON();return r.fulfill({json:{data:{root:{id:rootId,name:'115测试',kind:'cloud',path:'/115',revision:1},files:[{id:'provider',name:source,path:source,size:5,directory:false}],rows:[{source,title:'已确认剧集',year:'2026',mediaType:'series',tmdbId:123,season:0,episode:2,episodes:[2],resolution:'1080p',confirmed:true,warnings:[]}]}}})})
 await page.route('**/api/v1/files/batches/preview',r=>{preview=r.request().postDataJSON();return r.fulfill({json:{data:{id:batchId,state:'preview',digest:'d',expiresAt:'2099-01-01T00:00:00Z',items:[{ordinal:0,source,state:'preview',result:{id:'op',state:'preview',completedSteps:0,plan:{items:[{source:{path:source},target:'电视剧/已确认剧集 (2026)/Season 00/已确认剧集 - S00E02.mkv'}],steps:[]}}}]}}})})
 await page.clock.install();await page.goto('/cloudStorage');await page.locator('.organize-record').filter({hasText:'/115/'+source}).getByRole('checkbox').check();await expect(page.getByRole('link',{name:'在文件页批量整理 · 1',exact:true})).toBeVisible();await page.clock.fastForward(11000);await expect(page.getByRole('link',{name:'在文件页批量整理 · 1',exact:true})).toBeVisible();await page.getByRole('link',{name:'在文件页批量整理 · 1',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'批次整理工作台'});await expect(dialog.getByText(/已确认 TMDB 123/)).toBeVisible();await expect(dialog.getByLabel('批次文件标题')).toHaveValue('已确认剧集');await expect(dialog.getByLabel('批次年份')).toHaveValue('2026');await expect(dialog.getByText('{title} ({year})/{title} ({year}) - {videoFormat}',{exact:true})).toBeVisible();expect(selected?.candidateIds).toEqual([candidateId])
 await page.screenshot({path:resolve(screenshots,'cloud-batch-identity-desktop.png'),fullPage:true,animations:'disabled'});await page.setViewportSize({width:375,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await dialog.screenshot({path:resolve(screenshots,'cloud-batch-identity-mobile.png'),animations:'disabled'})
 await dialog.getByRole('button',{name:'预览全部变更',exact:true}).click();await expect.poll(()=>preview?.items?.[0]?.expectedFileId).toBe('provider');expect(preview?.items[0]?.expectedSize).toBe(5);await expect(dialog.getByText(/Season 00/)).toBeVisible()
 // Production mutations are not fixtures: this verifies only preview; isolated Go tests execute the batch.
})

test('cloud asset selection survives a polling response that was already in flight (fixture)',async({page})=>{
 await fixtureLogin(page);await page.clock.install()
 const asset={id:'00000000-0000-4000-8000-000000000099',title:'已入库选择保护',path:'/115/owned-selection.mkv',state:'present',artifactsState:'ready',fileScoped:true,revision:1}
 await page.route('**/api/v1/library/candidates?*',r=>r.fulfill({json:{data:{items:[],total:0}}}));for(const name of ['scans','schedules'])await page.route(`**/api/v1/library/${name}`,r=>r.fulfill({json:{data:[]}}))
 let reads=0,release!:()=>void;const pending=new Promise<void>(resolve=>release=resolve)
 await page.route('**/api/v1/media/assets?*',async r=>{reads++;if(reads===2)await pending;await r.fulfill({json:{data:{items:[asset],total:1}}})})
 await page.goto('/cloudStorage');await expect(page.getByRole('button',{name:'刷新',exact:true})).toBeEnabled();await page.locator('.media-workspace .page-tabs').getByRole('button',{name:'已入库',exact:true}).click()
 await page.clock.fastForward(11000);await expect.poll(()=>reads).toBe(2)
 await page.locator('.organize-record').filter({hasText:asset.path}).getByRole('checkbox').check();await expect(page.getByRole('link',{name:'在文件页批量整理 · 1',exact:true})).toBeVisible()
 release();await expect(page.getByRole('button',{name:'刷新',exact:true})).toBeEnabled();await expect(page.getByRole('link',{name:'在文件页批量整理 · 1',exact:true})).toBeVisible()
 await page.clock.fastForward(21000);await expect(page.getByRole('link',{name:'在文件页批量整理 · 1',exact:true})).toBeVisible();expect(reads).toBe(2)
})

test('directory classification has ordered rules and responsive editing (fixture)',async({page})=>{
 await fixtureLogin(page)
 const value={groups:[{id:'movies',name:'电影',directory:'电影',mediaType:'movie',genreIds:[],enabled:true,rules:[{id:'cn',name:'华语电影',directory:'华语电影',enabled:true,genreIds:[],countries:[],languages:['zh']},{id:'other',name:'其他电影',directory:'其他电影',enabled:true,genreIds:[],countries:[],languages:[]}]}]}
 let saved:Record<string,any>|undefined;await page.route('**/api/v1/classification',r=>{if(r.request().method()==='PUT'){saved=r.request().postDataJSON();return r.fulfill({json:{data:saved}})}return r.fulfill({json:{data:{value,revision:1}}})})
 await page.goto('/classification');await expect(page.getByRole('heading',{name:'目录分类',exact:true})).toBeVisible();await page.getByRole('button',{name:'其他电影更多操作',exact:true}).click();await page.getByRole('menuitem',{name:'上移',exact:true}).click();await page.getByRole('button',{name:'保存目录分类',exact:true}).click();await expect.poll(()=>saved?.value.groups[0].rules[0].id).toBe('other')
 await page.getByRole('button',{name:'为电影添加规则',exact:true}).click();const d=page.getByRole('dialog',{name:'编辑分流规则'});await d.getByLabel('分类名称').fill('纪录片');await d.getByLabel('目标目录').fill('纪录片');await d.getByRole('button',{name:'应用到草稿'}).click();await expect(page.getByText('目录：纪录片',{exact:true})).toBeVisible()
 for(const width of [1440,375]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`classification-${width}.png`),fullPage:true,animations:'disabled'})}
})

test('external API has independent scopes, real status verification and revocation (isolated)',async({page})=>{
 await fixtureLogin(page);await page.goto('/external-api');await expect(page.getByRole('heading',{name:'外部 API 凭据',exact:true})).toBeVisible()
 const response=await page.request.post('/api/v1/external-api/keys',{headers:{'X-Requested-With':'media-tool'},data:{name:'Owned isolated API',scopes:['status.read'],confirm:true}});expect(response.status()).toBe(201);const key=(await response.json()).data;expect(key.expiresAt).toBeNull();await expect(page.getByText('有效天数',{exact:true})).toHaveCount(0)
 const status=await page.request.get('/api/external/v1/status',{headers:{Authorization:'Bearer '+key.token}});expect(status.status()).toBe(200);expect((await status.json()).data.version).toBe(expectedVersion)
 expect((await page.request.get('/api/external/v1/downloads',{headers:{Authorization:'Bearer '+key.token}})).status()).toBe(403);expect((await page.request.get('/api/external/v1/status')).status()).toBe(401)
 await page.getByRole('button',{name:'刷新',exact:true}).click();await expect(page.getByRole('heading',{name:'Owned isolated API',exact:true})).toBeVisible();await expect(page.getByLabel('完整API凭据')).toHaveCount(0);expect(await page.locator('body').innerText()).not.toContain(key.token);await page.getByLabel('待验证凭据',{exact:true}).fill(key.token);await page.getByRole('button',{name:'验证状态访问',exact:true}).click();await expect(page.getByText('验证通过：此凭据可以读取状态。',{exact:true})).toBeVisible()
 for(const width of [1440,375]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`external-api-${width}.png`),fullPage:true,animations:'disabled',mask:[page.getByLabel('待验证凭据')]})}
 expect((await page.request.post(`/api/v1/external-api/keys/${key.id}/revoke`,{headers:{'X-Requested-With':'media-tool'},data:{confirm:true}})).status()).toBe(200);expect((await page.request.get('/api/external/v1/status',{headers:{Authorization:'Bearer '+key.token}})).status()).toBe(401)
})

test('STRM directory filters before pagination and previews original content without manual Emby refresh (fixture)',async({page})=>{
 await fixtureLogin(page);let facet='';const asset='00000000-0000-4000-8000-000000000120'
 await page.route('**/api/v1/library/outputs?*',r=>{facet=new URL(r.request().url()).searchParams.get('mediaType')??'';return r.fulfill({json:{data:{items:[{id:asset,title:'自有媒体',year:'2026',mediaType:'movie',source:'/115/owned.mkv',outputs:[{kind:'strm',path:'电影/华语电影/自有媒体 (2026)/自有媒体 (2026) - 1080p.strm'}],embyState:'pending',cloudState:'present',artifactsState:'ready'}],total:1}}})})
 await page.route(`**/api/v1/library/outputs/${asset}/content?*`,r=>r.fulfill({json:{data:{kind:'strm',path:'owned.strm',digest:'fixture-digest',content:'https://owned.fixture/api/v1/cloudStorage/strm302?apiKey=owned-key\n'}}}))
 await page.goto('/strm');await page.getByRole('combobox',{name:'STRM媒体类型'}).press('ArrowDown');await page.getByRole('option',{name:'电影',exact:true}).click();await expect.poll(()=>facet).toBe('movie');await expect(page.getByRole('button',{name:'定向刷新并确认Emby',exact:true})).toHaveCount(0)
 for(const folder of ['电影','华语电影','自有媒体 (2026)'])await page.locator('.files-directory').filter({hasText:folder}).click();await page.getByRole('button',{name:'预览STRM内容',exact:true}).click();await expect(page.getByLabel('托管输出原始内容')).toHaveValue('https://owned.fixture/api/v1/cloudStorage/strm302?apiKey=owned-key\n');await page.screenshot({path:resolve(screenshots,'strm-original-content.png'),fullPage:true,animations:'disabled'})
})

test('verified instant upload shows processing completion without inventing sent bytes (fixture)',async({page})=>{
 await fixtureLogin(page);await page.route('**/api/v1/downloads',r=>r.fulfill({json:{data:[]}}));await page.route('**/api/v1/transfers',r=>r.fulfill({json:{data:[{id:'owned',destination:'/115/电影/自有影片.mkv',state:'imported',phase:'verified',bytes:279011,hashBytes:7960117968,total:7960117968,processingProgress:100,processingStage:'入库完成',transferMethod:'instant_or_reused'}]}}));await page.goto('/downloads?tab=uploads');await expect(page.getByText('入库完成',{exact:true})).toBeVisible();await expect(page.getByText('秒传或复用；节省字节不计作网络传输',{exact:true})).toBeVisible();await expect(page.getByRole('progressbar')).toHaveAttribute('aria-valuenow','100')
})

test('classification real defaults render without null collection failures (isolated)',async({page})=>{
 await fixtureLogin(page);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await page.goto('/classification');await expect(page.getByRole('heading',{name:'目录分类',exact:true})).toBeVisible();for(const name of ['动漫','电影','电视剧'])await expect(page.getByRole('article',{name,exact:true}).getByRole('heading',{name,exact:true})).toBeVisible();await expect(page.getByText('目录：日韩电影',{exact:true})).toBeVisible();expect(errors).toEqual([])
 for(const width of [1440,375]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`classification-real-${width}.png`),fullPage:true,animations:'disabled'})}
})

test('classification checkbox editor preserves original countries and genre IDs (fixture)',async({page})=>{
 await fixtureLogin(page)
 const value={groups:[{id:'movies',name:'电影',directory:'电影',mediaType:'movie',genreIds:[],enabled:true,rules:[{id:'owned',name:'旧值回显',directory:'自定义',enabled:true,genreIds:[999999],countries:['ZZ','CN'],languages:['zh']}]}]}
 let saved:any;await page.route('**/api/v1/classification',r=>{if(r.request().method()==='PUT'){saved=r.request().postDataJSON();return r.fulfill({json:{data:saved}})}return r.fulfill({json:{data:{value,revision:1}}})})
 await page.goto('/classification');await page.getByRole('button',{name:'编辑旧值回显',exact:true}).click();const dialog=page.getByRole('dialog',{name:'编辑分流规则'})
 await expect(dialog.getByRole('checkbox',{name:'已保存地区 ZZ ZZ',exact:true})).toBeChecked();await expect(dialog.getByRole('checkbox',{name:'中国 CN',exact:true})).toBeChecked();await expect(dialog.getByRole('checkbox',{name:'已保存题材 999999 999999',exact:true})).toBeChecked()
 await dialog.getByRole('checkbox',{name:'纪录片 99',exact:true}).check();await dialog.getByLabel('搜索国家或地区').fill('日本');await dialog.getByRole('checkbox',{name:'日本 JP',exact:true}).check();await expect(dialog.getByRole('checkbox',{name:'汉语（普通话） zh',exact:true})).toBeChecked()
 for(const width of [1440,375]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`classification-checkbox-${width}.png`),animations:'disabled'})}
 await dialog.getByRole('button',{name:'应用到草稿'}).click();await expect(dialog).not.toBeVisible();await page.getByRole('button',{name:'保存目录分类',exact:true}).click();await expect.poll(()=>saved?.value.groups[0].rules[0].countries).toEqual(['ZZ','CN','JP']);expect(saved.value.groups[0].rules[0].genreIds).toEqual([999999,99]);expect(saved.value.groups[0].rules[0].languages).toEqual(['zh'])
})

test('system operations has real bounded health, paginated tasks and details (isolated)',async({page})=>{
 await fixtureLogin(page);await page.goto('/jobs');await expect(page.getByRole('heading',{name:'运行与队列',exact:true})).toBeVisible();await page.getByText('查看运行计划与连接状态',{exact:true}).click();await expect(page.getByText(/核心协调/)).toBeVisible();await expect(page.locator('.u5-stats').first().getByText('可用',{exact:true})).toHaveCount(2)
 const health=await page.request.get('/api/v1/system/operations');expect(health.status()).toBe(200);expect(health.headers()['cache-control']).toBe('no-store');const snapshot=(await health.json()).data;expect(snapshot.migration).toBe(expectedSchema);expect(snapshot.unavailable).toEqual([]);expect(snapshot.workers.some((w:any)=>w.online)).toBe(true);expect(snapshot.schedules.some((s:any)=>s.id==='core'&&s.cron==='*/15 * * * * *')).toBe(true)
 const response=await page.request.post('/api/v1/jobs',{headers:{'X-Requested-With':'media-tool'},data:{kind:'system.check'}});expect(response.status()).toBe(201);const job=(await response.json()).data;await page.getByLabel('搜索任务',{exact:true}).fill(job.id);await page.getByRole('button',{name:'筛选任务',exact:true}).click();await expect(page.getByRole('button',{name:'任务详情',exact:true})).toHaveCount(1);await expect(page.getByRole('button',{name:'任务详情',exact:true})).toBeVisible();await page.getByRole('button',{name:'任务详情',exact:true}).click();const dialog=page.getByRole('dialog',{name:'任务与运行技术详情',exact:true});await dialog.getByRole('button',{name:/技术细节与原始记录/}).click();await expect(dialog).toContainText(job.id);await expect(page.getByText(/重试先核对原业务状态/)).toBeVisible();await dialog.getByRole('button',{name:'关闭',exact:true}).click();await expect(dialog).not.toBeVisible()
 expect((await page.request.get('/api/v1/jobs/browse?page=0')).status()).toBe(400);expect((await page.request.get('/api/v1/jobs/not-a-uuid')).status()).toBe(400);const empty=await page.request.get(`/api/v1/jobs/browse?page=2&pageSize=25&q=${job.id}`);expect((await empty.json()).data).toEqual({items:[],total:1})
 for(const width of [1440,375]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`system-operations-${width}.png`),fullPage:true,animations:'disabled'})}
})

test('history entries separate media paths time and safe actions at mobile widths (fixture)',async({page})=>{
 await fixtureLogin(page);const item={id:'owned-history-ui',kind:'transfer',state:'imported',source:'/data/下载目录/Owned.Film.2026.1080p.WEB-DL.mkv',destination:'/115/电影/华语电影/自有影片 (2026)/自有影片 (2026) - 1080p.mkv',origin:'download',method:'transfer',batchId:'owned-batch',media:{title:'自有影片',year:'2026',mediaType:'movie'},category:'华语电影',message:'云端校验、STRM输出均完成，保留做种源',updatedAt:'2026-10-15T01:00:00Z',createdAt:'2026-10-15T00:00:00Z',details:{phase:'verified',bytesTransferred:5,size:5}}
 await page.route('**/api/v1/library/activity?*',r=>r.fulfill({json:{data:{items:[item],total:1}}}));await page.goto('/organize-history');const row=page.locator('.organize-record');await expect(row.locator('time')).toHaveCount(1);await expect(row.getByRole('heading')).toHaveText('自有影片 (2026)');await expect(row).toContainText(item.source);await expect(row).toContainText(item.destination)
 for(const width of [1440,768,375]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await expect(row.getByRole('button',{name:'查看步骤详情',exact:true})).toBeVisible();await page.screenshot({path:resolve(screenshots,`history-compact-${width}.png`),fullPage:true,animations:'disabled'});}
 await row.getByRole('button',{name:'查看步骤详情',exact:true}).click();await expect(page.getByRole('dialog',{name:'整理步骤详情',exact:true})).toContainText(item.destination)
})

test('classification groups keep ordered rules, empty groups and responsive actions (fixture)',async({page})=>{
 await fixtureLogin(page)
 const value={groups:[{id:'movies-list',name:'列表电影',directory:'电影',mediaType:'movie',genreIds:[99],enabled:false,rules:[{id:'cn-list',name:'华语纪录',directory:'华语纪录',enabled:true,genreIds:[18],countries:['CN','ZZ'],languages:['zh']},{id:'other-list',name:'其他纪录',directory:'其他纪录',enabled:false,genreIds:[],countries:[],languages:[]}]},{id:'series-list',name:'列表剧集',directory:'电视剧',mediaType:'series',genreIds:[],enabled:true,rules:[]}]}
 await page.route('**/api/v1/classification',r=>r.fulfill({json:{data:{value,revision:7}}}))
 await page.goto('/classification');const groups=page.locator('.classification-card');await expect(groups).toHaveCount(2);await expect(groups.nth(0).getByRole('switch',{name:'启用列表电影'})).not.toBeChecked();await expect(groups.nth(1).getByRole('switch',{name:'启用列表剧集'})).toBeChecked();await expect(groups.nth(0).locator('li strong')).toHaveText(['华语纪录','其他纪录']);await expect(groups.nth(0)).toContainText('已保存地区 ZZ');await expect(groups.nth(1)).toContainText('暂无细分规则');
 for(const width of [1440,768,375]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await expect(groups.nth(0).getByRole('button',{name:'为列表电影添加规则',exact:true})).toBeVisible();await page.screenshot({path:resolve(screenshots,`classification-list-${width}.png`),fullPage:true,animations:'disabled'})}
})

test('classification empty list displays an honest empty state without group cards (fixture)',async({page})=>{
 await fixtureLogin(page);await page.route('**/api/v1/classification',r=>r.fulfill({json:{data:{value:{groups:[]},revision:1}}}));await page.goto('/classification');await expect(page.getByRole('region',{name:'目录分类',exact:true})).toBeVisible();await expect(page.getByText('尚未设置目录分类',{exact:true})).toBeVisible();await expect(page.locator('.classification-card')).toHaveCount(0);await expect(page.getByRole('button',{name:'添加分类',exact:true})).toBeEnabled()
})

test('maintenance center preserves policy editing and contains desktop/mobile tables (fixture)',async({page})=>{
 await fixtureLogin(page)
 const policy={backupEnabled:false,backupCron:'0 3 * * *',keepDays:14,keepCount:10,sampleDays:14,alertsEnabled:true,memoryPercent:90,diskFreeGB:5,loadPerCPU:2,failedJobs:3,consecutive:3,cooldownMinutes:60}
 const data:any={policy:{value:policy,revision:1,nextBackupAt:'2026-10-16T03:00:00+08:00'},backups:[],drills:[],requests:[],releases:[],alerts:[],mode:{paused:false,reason:'',revision:1},toolsReady:true,totalBytes:0,revision:'a'.repeat(40),version:'0.27.0-dev',schema:50}
 let saved:any;await page.route('**/api/v1/ops',r=>r.fulfill({json:{data}}));await page.route('**/api/v1/ops/trends?*',r=>r.fulfill({json:{data:[]}}));await page.route('**/api/v1/ops/policy',r=>{saved=r.request().postDataJSON();data.policy={...saved,revision:2};return r.fulfill({json:{data:data.policy}})})
 await page.goto('/maintenance');await expect(page.getByRole('heading',{name:'备份与告警',exact:true})).toBeVisible();await expect(page.getByText('尚无系统备份',{exact:true})).toBeVisible();await page.getByRole('button',{name:'维护策略',exact:true}).click();await page.getByRole('checkbox',{name:'启用定时备份',exact:true}).check();await expect(page.getByRole('checkbox',{name:'启用定时备份',exact:true})).toBeChecked();await page.getByLabel('备份Cron',{exact:true}).fill('0 4 * * *');await page.getByRole('button',{name:'保存维护策略',exact:true}).click();await expect.poll(()=>saved?.value.backupCron).toBe('0 4 * * *');expect(saved.value.backupEnabled).toBe(true);expect(saved.revision).toBe(1);expect(saved.value.keepCount).toBe(10)
 for(const width of [1440,768,375]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`maintenance-policy-${width}.png`),fullPage:true,animations:'disabled'})}
 await page.getByRole('button',{name:'运行趋势',exact:true}).click();await expect(page.getByText('暂无有效采样',{exact:true})).toHaveCount(9);await expect(page.locator('.u6-trends').getByRole('img')).toHaveCount(0)
})

test('maintenance backup and drill require confirmation and never pretend a queued job completed (fixture)',async({page})=>{
 await fixtureLogin(page);const id='33333333-3333-4333-8333-333333333333',drill='44444444-4444-4444-8444-444444444444'
 const data:any={policy:{value:{backupEnabled:false},revision:1,nextBackupAt:'2026-10-16T00:00:00Z'},backups:[{id,state:'ready',origin:'manual',pinned:false,bytes:1024,digest:'d'.repeat(64),manifest:{schema:50},message:'隔离自有备份',createdAt:'2026-10-15T00:00:00Z'}],drills:[],requests:[],releases:[],alerts:[],mode:{paused:false,reason:'',revision:1},toolsReady:true,totalBytes:1024,revision:'a'.repeat(40),schema:50}
 await page.route('**/api/v1/ops',r=>r.fulfill({json:{data}}));await page.route('**/api/v1/ops/trends?*',r=>r.fulfill({json:{data:[]}}));let body:any;await page.route(`**/api/v1/ops/backups/${id}/drill`,r=>{body=r.request().postDataJSON();data.drills=[{id:drill,backupId:id,state:'pending',message:'等待隔离克隆演练',createdAt:'2026-10-15T00:00:00Z'}];return r.fulfill({status:202,json:{data:{id:drill}}})})
 await page.goto('/maintenance');await expect(page.getByRole('link',{name:'下载备份',exact:true})).toHaveAttribute('href',`/api/v1/ops/backups/${id}/download`);await page.getByRole('button',{name:'恢复演练',exact:true}).click();const confirm=page.getByRole('dialog',{name:'恢复演练',exact:true});await expect(confirm).toContainText('不恢复到正式数据库');expect(body).toBeUndefined();await confirm.getByRole('button',{name:'确认继续',exact:true}).click();await expect.poll(()=>body?.confirm).toBe(true);await expect(page.getByText('等待隔离克隆演练',{exact:true})).toBeVisible();await expect(page.getByText('演练通过',{exact:true})).toHaveCount(0)
})

test('maintenance approval is typed, scoped and still awaits owner execution (fixture)',async({page})=>{
 await fixtureLogin(page);const backup='33333333-3333-4333-8333-333333333333',id='55555555-5555-4555-8555-555555555555',digest='e'.repeat(64),phrase='恢复备份 '+backup
 const data:any={policy:{value:{backupEnabled:false},revision:1,nextBackupAt:'2026-10-16T00:00:00Z'},backups:[{id:backup,state:'ready',origin:'manual',pinned:false,bytes:1024,digest:'d'.repeat(64),manifest:{schema:50},message:'隔离自有备份',createdAt:'2026-10-15T00:00:00Z'}],drills:[],requests:[],releases:[],alerts:[],mode:{paused:false,reason:'',revision:1},toolsReady:true,totalBytes:1024,revision:'a'.repeat(40),schema:50}
 const ticket={id,kind:'restore',state:'preview',payload:{backupId:backup,schema:50},digest,expiresAt:'2027-01-01T00:00:00Z',message:''};let preview:any,approved:any
 await page.route('**/api/v1/ops',r=>r.fulfill({json:{data}}));await page.route('**/api/v1/ops/trends?*',r=>r.fulfill({json:{data:[]}}));await page.route('**/api/v1/ops/requests/preview',r=>{preview=r.request().postDataJSON();return r.fulfill({json:{data:{request:ticket,confirmText:phrase,command:'media-toolctl --root /root/docker/mediatool execute '+id}}})});await page.route(`**/api/v1/ops/requests/${id}/approve`,r=>{approved=r.request().postDataJSON();data.requests=[{...ticket,state:'approved',message:'已审批，等待宿主机受控执行；当前尚未恢复或回滚'}];return r.fulfill({json:{data:{state:'approved'}}})})
 await page.goto('/maintenance');await page.getByRole('button',{name:'恢复与回滚',exact:true}).click();await page.getByRole('combobox',{name:'选择待恢复备份',exact:true}).selectOption(backup);await page.getByRole('button',{name:'预览恢复',exact:true}).click();const previewDialog=page.getByRole('dialog',{name:'恢复与回滚预览',exact:true});await previewDialog.getByRole('button',{name:'审批维护单（不立即执行）',exact:true}).click();const dialog=page.getByRole('dialog',{name:'确认维护审批',exact:true});await expect(dialog).toBeVisible();expect(preview).toEqual({kind:'restore',id:backup});await expect(dialog.getByRole('button',{name:'审批维护单（不立即执行）',exact:true})).toBeDisabled();await dialog.getByPlaceholder('请输入上方确认短语').fill(phrase);await dialog.getByRole('button',{name:'审批维护单（不立即执行）',exact:true}).click();await expect(dialog).not.toBeVisible();expect(approved).toEqual({confirm:true,digest,text:phrase});await expect(page.getByText(/已审批，等待宿主机受控执行；当前尚未恢复或回滚/)).toBeVisible();await expect(page.getByRole('button',{name:'立即执行恢复',exact:true})).toHaveCount(0);await page.setViewportSize({width:375,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,'maintenance-recovery-mobile.png'),fullPage:true,animations:'disabled'})
})

test('maintenance trends use real sample values and alert acknowledgment is explicit (fixture)',async({page})=>{
 await fixtureLogin(page);const data:any={policy:{value:{backupEnabled:false},revision:1,nextBackupAt:'2026-10-16T00:00:00Z'},backups:[],drills:[],requests:[],releases:[],alerts:[{code:'memory',state:'active',hits:3,value:95,acknowledged_at:null,message:'宿主内存使用率过高',last_seen_at:'2026-10-15T00:15:00Z'}],mode:{paused:false,reason:'',revision:1},toolsReady:true,totalBytes:0,revision:'a'.repeat(40),schema:50};await page.route('**/api/v1/ops',r=>r.fulfill({json:{data}}));await page.route('**/api/v1/ops/trends?*',r=>r.fulfill({json:{data:[{at:'2026-10-15T00:00:00Z',memoryPercent:92,diskFreeGB:20,load1:1,loadPerCPU:0.5,processBytes:10485760,jobSeconds:2,pendingJobs:0,failedJobs10m:0,succeededJobs10m:2},{at:'2026-10-15T00:15:00Z',memoryPercent:95,diskFreeGB:19,load1:2,loadPerCPU:1,processBytes:20971520,jobSeconds:3,pendingJobs:0,failedJobs10m:0,succeededJobs10m:3}]}}));let ack:any;await page.route('**/api/v1/ops/alerts/memory/ack',r=>{ack=r.request().postDataJSON();data.alerts[0].acknowledged_at='2026-10-15T00:16:00Z';return r.fulfill({json:{data:{acknowledged:true}}})})
 await page.goto('/maintenance');await page.getByRole('button',{name:'运维告警',exact:true}).click();await page.getByRole('button',{name:'确认告警',exact:true}).click();await page.getByRole('dialog',{name:'确认告警',exact:true}).getByRole('button',{name:'确认继续',exact:true}).click();await expect.poll(()=>ack?.confirm).toBe(true);await expect(page.getByText('已确认',{exact:true})).toBeVisible();await page.getByRole('button',{name:'运行趋势',exact:true}).click();await expect(page.getByRole('img',{name:/内存使用率趋势/})).toBeVisible();const zero=page.locator('.u6-trend').filter({has:page.getByRole('heading',{name:'十分钟失败任务 · 条',exact:true})});await expect(zero).toContainText('最高 0');await expect(page.locator('.u6-trend')).toHaveCount(9)
 for(const width of [1440,375]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`maintenance-trends-${width}.png`),fullPage:true,animations:'disabled'})}
})


test('classification choices pin selections and filter Chinese genres countries and languages without losing saved values (fixture)',async({page})=>{
 await fixtureLogin(page)
 const value={groups:[{id:'choice-movies',name:'电影',directory:'电影',mediaType:'movie',genreIds:[],enabled:true,rules:[{id:'choice-owned',name:'选项筛选',directory:'筛选',enabled:true,genreIds:[99],countries:['US','CN','ZZ'],languages:['zh','yue','xyz']}]}]}
 let saved:any;await page.route('**/api/v1/classification',r=>{if(r.request().method()==='PUT'){saved=r.request().postDataJSON();return r.fulfill({json:{data:saved}})}return r.fulfill({json:{data:{value,revision:1}}})})
 await page.goto('/classification');await page.getByRole('button',{name:'编辑选项筛选',exact:true}).click();const dialog=page.getByRole('dialog',{name:'编辑分流规则'})
 const genre=dialog.getByRole('region',{name:'题材',exact:true}),country=dialog.getByRole('region',{name:'国家或地区',exact:true}),language=dialog.getByRole('region',{name:'原始语言',exact:true})
 await expect(language.getByRole('checkbox',{name:'阿法尔语 aa',exact:true})).toHaveCount(1)
 for(const [picker,count]of [[genre,1],[country,3],[language,3]]as const){expect(await picker.locator('label.rule-choice input').evaluateAll((inputs,n)=>inputs.slice(0,n).every(input=>(input as HTMLInputElement).checked)&&inputs.slice(n).every(input=>!(input as HTMLInputElement).checked),count)).toBe(true)}
 await genre.getByLabel('搜索题材').fill('动画');await genre.getByRole('checkbox',{name:'动画 16',exact:true}).check();await genre.getByLabel('搜索题材').fill('');await expect(genre.locator('label.rule-choice input').nth(1)).toBeChecked()
 await country.getByLabel('搜索国家或地区').fill('日本');await country.getByRole('checkbox',{name:'日本 JP',exact:true}).check();await expect(country.getByRole('status')).toContainText('中国 (CN)');await country.getByLabel('搜索国家或地区').fill(' us ');await expect(country.getByRole('checkbox')).toHaveCount(1);await expect(country.getByRole('checkbox',{name:'美国 US',exact:true})).toBeChecked();await country.getByLabel('搜索国家或地区').fill('')
 await expect(language.getByRole('checkbox',{name:'粤语 yue',exact:true})).toBeChecked();await language.getByLabel('搜索原始语言').fill('英语');await language.getByRole('checkbox',{name:'英语 en',exact:true}).check();await language.getByLabel('搜索原始语言').fill('不存在的语言');await expect(language.getByText(/没有匹配项/)).toBeVisible();await expect(language.getByRole('status')).toContainText('已保存语言 xyz (xyz)');await language.getByLabel('搜索原始语言').fill('CN');await expect(language.getByRole('checkbox',{name:'粤语 cn',exact:true})).toBeVisible();await language.getByLabel('搜索原始语言').fill('')
 await language.getByRole('checkbox',{name:'英语 en',exact:true}).focus();await page.keyboard.press('Space');await expect(language.getByRole('checkbox',{name:'英语 en',exact:true})).not.toBeChecked();await expect(language.getByRole('checkbox',{name:'英语 en',exact:true})).toBeFocused();await page.keyboard.press('Space');await expect(language.getByRole('checkbox',{name:'英语 en',exact:true})).toBeChecked();await expect(language.getByRole('checkbox',{name:'英语 en',exact:true})).toBeFocused()
 await dialog.getByRole('button',{name:'应用到草稿'}).click();await page.getByRole('button',{name:'保存目录分类',exact:true}).click();await expect.poll(()=>saved?.value.groups[0].rules[0].languages).toEqual(['zh','yue','xyz','en']);expect(saved.value.groups[0].rules[0].countries).toEqual(['US','CN','ZZ','JP']);expect(saved.value.groups[0].rules[0].genreIds).toEqual([99,16])
 await page.getByRole('button',{name:'编辑选项筛选',exact:true}).click();for(const label of ['搜索题材','搜索国家或地区','搜索原始语言'])await expect(dialog.getByLabel(label,{exact:true})).toHaveValue('');await expect(dialog.getByRole('checkbox',{name:'英语 en',exact:true})).toBeChecked()
})

test('classification long country labels wrap inside their own choices on desktop and mobile (fixture)',async({page})=>{
 await fixtureLogin(page);const value={groups:[{id:'wrap-movies',name:'电影',directory:'电影',mediaType:'movie',genreIds:[],enabled:true,rules:[{id:'wrap-rule',name:'长名称测试',directory:'测试',enabled:true,genreIds:[],countries:['GS','IO','UM'],languages:['zh']}]}]}
 await page.route('**/api/v1/classification',r=>r.fulfill({json:{data:{value,revision:1}}}));await page.goto('/classification');await page.getByRole('button',{name:'编辑长名称测试',exact:true}).click();const dialog=page.getByRole('dialog',{name:'编辑分流规则'}),country=dialog.getByRole('region',{name:'国家或地区',exact:true})
 for(const width of [1440,768,375,320]){
  await page.setViewportSize({width,height:950});await expect(dialog).toBeVisible();expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  const bounds=await country.locator('label.rule-choice').evaluateAll(labels=>labels.map(label=>{const rect=label.getBoundingClientRect(),text=label.querySelector('span')!,box=label.querySelector('input')!;const range=document.createRange();range.selectNodeContents(text);return{width:label.clientWidth,scroll:label.scrollWidth,height:rect.height,boxWidth:box.getBoundingClientRect().width,contained:[...range.getClientRects()].every(r=>r.left>=rect.left-1&&r.right<=rect.right+1&&r.top>=rect.top-1&&r.bottom<=rect.bottom+1)}}))
  expect(bounds.length).toBeGreaterThan(240);for(const bound of bounds){expect(bound.scroll).toBeLessThanOrEqual(bound.width+1);expect(bound.height).toBeGreaterThanOrEqual(43.99);expect(bound.boxWidth).toBeGreaterThanOrEqual(14);expect(bound.contained).toBe(true)}
  expect(await country.getByRole('group').evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true);await country.scrollIntoViewIfNeeded();await page.screenshot({path:resolve(screenshots,`classification-choice-wrap-${width}.png`),animations:'disabled'})
 }
})

test('settings functional pages are consistent and preserve drafts while switching tabs (isolated)',async({page})=>{
 await fixtureLogin(page);await page.goto('/settings');const nav=page.locator('.settings-workspace > .page-tabs');await expect(nav).toBeVisible()
 for(const width of [1440,768,375]){await page.setViewportSize({width,height:950});for(const tab of ['通用与访问','命名与分类','搜索订阅','外部 API','移动端','账号安全']){await nav.getByRole('button',{name:tab,exact:true}).click();await expect(nav.getByRole('button',{name:tab,exact:true})).toHaveAttribute('aria-pressed','true');expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`settings-${tab}-${width}.png`),animations:'disabled'})}}
 await nav.getByRole('button',{name:'命名与分类',exact:true}).click();await page.getByLabel('电影命名模板',{exact:true}).fill('{title} ({year})/{title} - draft');await nav.getByRole('button',{name:'搜索订阅',exact:true}).click();await nav.getByRole('button',{name:'命名与分类',exact:true}).click();await expect(page.getByLabel('电影命名模板',{exact:true})).toHaveValue('{title} ({year})/{title} - draft')
 await expect(nav.getByRole('button',{name:'TMDB',exact:true})).toHaveCount(0);await page.goto('/settings?section=tmdb');await expect(page).toHaveURL(/\/connections\?.*tab=aux/)
})

test('domain settings validate origins and update generated STRM without touching old output (isolated)',async({page})=>{
 await fixtureLogin(page);await page.goto('/settings?tab=access');const initial=(await(await page.request.get('/api/v1/settings/access')).json()).data
 try{
  await page.getByLabel('后台访问域名',{exact:true}).fill('https://admin.fixture.invalid');await page.getByLabel('播放链接域名',{exact:true}).fill('https://play.fixture.invalid');await page.getByLabel('内置媒体库域名',{exact:true}).fill('https://member.fixture.invalid');await page.getByRole('button',{name:'保存域名设置',exact:true}).click();await expect(page.getByText(/当前分区已保存/)).toBeVisible()
  const current=(await(await page.request.get('/api/v1/settings/access')).json()).data;expect(current.effective.memberUrl).toBe('https://member.fixture.invalid')
  const bad=await page.request.put('/api/v1/settings/access',{headers:{'X-Requested-With':'media-tool'},data:{value:{...current.value,managementUrl:'https://example.com/watch'},revision:current.revision}});expect(bad.status()).toBe(400)
  const preview=await page.request.post('/api/v1/strm/preview',{headers:{'X-Requested-With':'media-tool'},data:{path:((await(await page.request.get('/api/v1/system/status')).json()).data.allowedRoots[0]||'/115')+'/Owned.mkv'}});expect(preview.status()).toBe(200);expect(JSON.stringify((await preview.json()).data)).toContain('https://play.fixture.invalid')
 }finally{const current=(await(await page.request.get('/api/v1/settings/access')).json()).data;expect((await page.request.put('/api/v1/settings/access',{headers:{'X-Requested-With':'media-tool'},data:{value:initial.value,revision:current.revision}})).status()).toBe(200)}
})

test('manual torrent imports support file and magnet input with persistent status and honest unknown progress (fixture)',async({page})=>{
 await fixtureLogin(page);let imports:any[]=[];await page.route('**/api/v1/downloads/manual',async route=>{if(route.request().method()==='POST'){imports.push({contentType:route.request().headers()['content-type'],body:route.request().postData()});return route.fulfill({status:202,json:{data:{id:'fixture-manual'}}})}return route.fulfill({json:{data:[{id:'fixture-manual',title:'Owned Manual.mkv',sourceKind:'file',source:'Owned.torrent',infoHash:'a'.repeat(40),state:'downloading',progress:null,qbState:'',downloadSpeed:0,savePath:'/downloads',endpoint:'http://qb.fixture:8080',category:'media-tool',message:'等待同步',createdAt:'2026-10-15T10:00:00Z'}]}})})
 await page.goto('/downloads?tab=manual');await expect(page.getByRole('heading',{name:'手动种子下载',exact:true})).toBeVisible();await expect(page.getByText('进度待下载器同步，不以0%代替未知进度。',{exact:true})).toBeVisible();await page.getByRole('button',{name:'添加种子',exact:true}).click();const dialog=page.getByRole('dialog',{name:'添加种子下载',exact:true});await dialog.getByLabel('种子文件（.torrent）',{exact:true}).setInputFiles({name:'Owned.torrent',mimeType:'application/x-bittorrent',buffer:Buffer.from('owned-file-fixture')});await dialog.getByRole('button',{name:'确认添加下载',exact:true}).click();await expect(dialog).toBeHidden();expect(imports[0].contentType).toContain('multipart/form-data');expect(imports[0].body).toContain('Owned.torrent')
 await page.getByRole('button',{name:'添加种子',exact:true}).click();await dialog.getByText('种子链接',{exact:true}).click();const magnet='magnet:?xt=urn:btih:'+'a'.repeat(40);await dialog.getByLabel('种子链接或磁力链接',{exact:true}).fill(magnet);await dialog.getByRole('button',{name:'确认添加下载',exact:true}).click();await expect(dialog).toBeHidden();expect(JSON.parse(imports[1].body).url).toBe(magnet)
 for(const width of [1440,375]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`manual-downloads-${width}.png`),fullPage:true,animations:'disabled'})}
})

test('operations panels have real gaps and no repeated cross-module shortcuts (isolated)',async({page})=>{
 await fixtureLogin(page);await page.goto('/jobs');const panels=page.locator('.media-stack>.media-surface');await expect(panels).toHaveCount(2);for(const width of [1440,375]){await page.setViewportSize({width,height:950});const gap=await panels.nth(1).evaluate(el=>el.getBoundingClientRect().top-el.previousElementSibling!.getBoundingClientRect().bottom);expect(gap).toBeGreaterThanOrEqual(16);expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`ops-unified-${width}.png`),fullPage:true})}await expect(page.locator('.media-workspace>.page-tabs')).toBeVisible();await page.locator('.media-workspace>.page-tabs').getByRole('button',{name:'备份与告警',exact:true}).click();await expect(page).toHaveURL(/tab=maintenance/);await expect(page.getByRole('button',{name:'刷新',exact:true})).toHaveCount(1)
})

test('TMDB settings never reveals a stored Token and updates replacements in place (fixture)',async({page})=>{
 await fixtureLogin(page);const token='owned-secret-not-for-echo',row={id:'owned-tmdb-single',kind:'tmdb',name:'TMDB',enabled:true,settings:{language:'zh-CN'},secretFields:['token'],revision:2};let saved:any,reveals=0
 await page.route('**/api/v1/integrations',r=>r.fulfill({json:{data:[row]}}));await page.route('**/secrets/reveal',r=>{reveals++;return r.fulfill({json:{data:{revision:2,values:{token}}}})});await page.route('**/api/v1/integrations/owned-tmdb-single',r=>{saved=r.request().postDataJSON();return r.fulfill({json:{data:{...row,revision:3}}})})
 await page.goto('/settings?tab=tmdb');await page.getByRole('button',{name:'编辑TMDB Token',exact:true}).click();const dialog=page.getByRole('dialog',{name:'编辑连接',exact:true});await expect(dialog.getByLabel('TMDB API Token',{exact:true})).toHaveValue('');expect(reveals).toBe(0);await expect(dialog).not.toContainText(token)
 await dialog.getByLabel('TMDB API Token',{exact:true}).fill('owned-replaced-token');await dialog.getByRole('button',{name:'保存TMDB Token',exact:true}).click();await expect(dialog).toBeHidden();expect(saved).toMatchObject({kind:'tmdb',revision:2,secrets:{token:'owned-replaced-token'}})
 for(const width of [1440,375]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`connection-token-status-${width}.png`),fullPage:true,animations:'disabled'})}
})

test('friendly log details keep raw values and copy controls without redundant URL buttons (fixture)',async({page})=>{
 await fixtureLogin(page);await page.addInitScript(()=>Object.defineProperty(navigator,'clipboard',{value:undefined,configurable:true}));const url='https://owned.example/strm?apiKey=full-value',location='https://cdn.example/video?sign=full-value'
 await page.route('**/api/v1/logs/query?*',route=>route.fulfill({json:{data:{items:[{msg:'HTTP request',level:'INFO',module:'playback',time:'2026-10-15T00:00:00Z',url,location,status:302,durationMs:4,requestId:'owned-full-request'},{msg:'player UA denied',level:'WARN',userAgent:'Owned-UA',time:'2026-10-15T00:00:00Z'}],total:2,page:1,pageSize:200,modules:['playback'],before:'2026-10-15T01:00:00Z',truncated:false}}}))
 await page.goto('/logs');await expect(page.getByText('HTTP request',{exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'完整原始记录',exact:true})).toHaveCount(1);await expect(page.getByRole('button',{name:'查看完整链接',exact:true})).toHaveCount(1);await expect(page.getByRole('button',{name:'查看请求与跳转链接',exact:true})).toHaveCount(0);await page.getByRole('button',{name:'查看完整链接',exact:true}).click();const detail=page.getByRole('dialog',{name:'完整原始日志',exact:true});await expect(detail).toContainText('已返回跳转地址，等待客户端直连');await expect(detail.getByLabel('完整请求URL',{exact:true})).toHaveValue(url);await expect(detail.getByLabel('完整302目标URL',{exact:true})).toHaveValue(location);await expect(detail.locator('pre')).toContainText('owned-full-request');await detail.getByRole('button',{name:'复制跳转地址',exact:true}).click();const copy=page.getByRole('dialog',{name:'复制原始日志',exact:true});await expect(copy.getByLabel('待复制日志')).toHaveValue(location);await copy.getByRole('button',{name:'关闭',exact:true}).click();await detail.getByRole('button',{name:'关闭',exact:true}).click();await page.getByRole('button',{name:'完整原始记录',exact:true}).click();await expect(detail.getByRole('button',{name:'复制跳转地址',exact:true})).toHaveCount(0);await expect(detail).toContainText('白名单');await detail.getByRole('button',{name:'关闭',exact:true}).click()
 for(const width of [1440,375]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`logs-friendly-${width}.png`),animations:'disabled'})}
})

test('native preprocessing controls, statistics, retry confirmation and bounded GC (UI contract)',async({page})=>{
 const c=JSON.parse(readFileSync(resolve(root,'internal/platform/httpapi/testdata/contracts/native-processing.json'),'utf8'))
 let settings={...c.summary.response.data.settings},single=0,batches=0,gcs=0,queued=false
 await page.route('**/api/v1/media-server/native/processing**',async route=>{const r=route.request(),path=new URL(r.url()).pathname;if(path.endsWith('/settings')){expect(r.method()).toBe('PUT');expect(r.headers()['x-requested-with']).toBe('media-tool');expect(r.postDataJSON()).toEqual({...c.settings.request,enabled:false});settings={...c.settings.response.data,enabled:false};return route.fulfill({json:{data:settings}})}if(path.endsWith('/tasks'))return route.fulfill({json:{data:{...c.tasks.response.data,items:queued?c.tasks.response.data.items.map((t:any)=>({...t,state:'pending',attempts:0})):c.tasks.response.data.items}}});if(path.endsWith('/retry')){const body=r.postDataJSON();expect(r.method()).toBe('POST');expect(r.headers()['x-requested-with']).toBe('media-tool');queued=true;if(body.bulk){batches++;expect(body).toEqual(c.batch.request);return route.fulfill({status:202,json:c.batch.response})}single++;expect(body).toEqual(c.retry.request);return route.fulfill({status:202,json:c.retry.response})}if(path.endsWith('/gc')){gcs++;expect(r.method()).toBe('POST');return route.fulfill({json:c.gc.response})}return route.fulfill({json:{data:{...c.summary.response.data,settings,probe:queued?[{state:'failed',count:0},{state:'pending',count:1}]:c.summary.response.data.probe}}})})
 await fixtureLogin(page);const realStatus=await page.request.get('/api/v1/media-server/native/processing');expect(realStatus.status()).toBe(200);expect((await realStatus.json()).data.settings.enabled).toBe(false);const realGC=await page.request.post('/api/v1/media-server/native/processing/gc',{headers:{'X-Requested-With':'media-tool'}});expect(realGC.status()).toBe(200);const report=(await realGC.json()).data;expect(report.scanned).toBeLessThanOrEqual(200);expect(report.reclaimed).toBe(0)
 await page.goto('/playback?tab=processing');await expect(page.getByRole('heading',{name:'探测与截图任务',exact:true})).toBeVisible();await expect(page.getByText(/失败 1/)).toBeVisible();await expect(page.getByText(/上限 2.00 GiB/)).toBeVisible();await expect(page.getByRole('checkbox',{name:'启用播放服务与预处理',exact:true})).toHaveCount(0);await expect(page.getByRole('link',{name:'管理播放服务开关',exact:true})).toHaveAttribute('href','/playback?tab=settings');await page.getByRole('button',{name:'保存预处理设置',exact:true}).click();expect(settings).toEqual({...c.settings.response.data,enabled:false})
 await page.getByRole('button',{name:'重试此任务',exact:true}).click();await page.getByRole('button',{name:'确认执行',exact:true}).click();await expect(page.getByRole('status').filter({hasText:'服务回执：选中 1 条'})).toContainText('已排队 1 条');expect(single).toBe(1)
 await page.getByRole('button',{name:'批量重试当前筛选',exact:true}).click();await page.getByRole('dialog',{name:'确认批量重试',exact:true}).getByRole('button',{name:'取消',exact:true}).click();expect(batches).toBe(0);await page.getByRole('button',{name:'批量重试当前筛选',exact:true}).click();await page.getByRole('button',{name:'确认执行',exact:true}).click();await expect(page.getByRole('status').filter({hasText:'服务回执：选中 500 条'})).toContainText('失败 1 条');expect(batches).toBe(1)
 await page.getByRole('button',{name:'清理截图缓存',exact:true}).click();expect(gcs).toBe(0);await page.getByRole('button',{name:'确认执行',exact:true}).click();await expect(page.getByRole('status').filter({hasText:'服务回执：扫描'})).toContainText('扫描 8 项，回收 2 个文件');expect(gcs).toBe(1)
 for(const width of [1440,390]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`native-processing-${width}.png`),fullPage:true})}
})

test('P4 native settings, media user limits, device revoke and history (isolated)', async ({page}) => {
 page.setDefaultTimeout(5000)
 await fixtureLogin(page)
 const headers={'X-Requested-With':'media-tool'}
 const savedResponse=await page.request.get('/api/v1/media-server/native/settings');expect(savedResponse.status()).toBe(200)
 const saved=(await savedResponse.json()).data
 const username='p4_fixture_member'
 let userId=''
 try {
  await page.goto('/media-server')
  await expect(page.getByRole('heading',{name:'服务设置',exact:true})).toBeVisible()
  await expect(page.getByRole('heading',{name:'Infuse 添加',exact:true})).toBeVisible()
  await expect(page.getByRole('heading',{name:'SenPlayer 添加',exact:true})).toBeVisible()
  await page.getByLabel('内置媒体库服务名称',{exact:true}).fill('P4 Synthetic Living Room')
  const enabled=page.getByRole('checkbox',{name:'启用内置媒体库',exact:true})
  await enabled.check()
  await page.getByRole('button',{name:'保存服务设置',exact:true}).click()
  await expect(page.getByText('已重新读取当前设置',{exact:true})).toBeVisible()
  await expect.poll(async()=>{const info=await page.request.get('http://127.0.0.1:18087/System/Info/Public');expect(info.status()).toBe(200);return(await info.json()).ServerName}).toBe('P4 Synthetic Living Room')
  await page.locator('.u5-workspace .page-tabs').getByRole('button',{name:'在线会话',exact:true}).click();await expect(page.getByRole('heading',{name:'在线会话',exact:true})).toBeVisible()
  await page.goto('/media-users')
  await page.getByRole('button',{name:'新建媒体用户',exact:true}).click()
  let dialog=page.getByRole('dialog',{name:'新建媒体用户',exact:true})
  await dialog.getByLabel('媒体用户名',{exact:true}).fill(username)
  await dialog.getByLabel('初始密码',{exact:true}).fill('Fixture123')
  await dialog.getByLabel('确认初始密码',{exact:true}).fill('Fixture123')
  await dialog.getByRole('spinbutton',{name:'同时播放上限',exact:true}).fill('3')
  await dialog.getByRole('spinbutton',{name:'年龄分级上限',exact:true}).fill('65')
  await dialog.getByLabel('媒体用户有效期',{exact:true}).fill('2030-01-01T12:00')
  await dialog.getByRole('button',{name:'保存媒体用户',exact:true}).click()
  await expect(dialog).not.toBeVisible()
  let users=(await (await page.request.get('/api/v1/media-server/users')).json()).data
  const user=users.find((u:any)=>u.username===username);expect(user).toBeTruthy();userId=user.id
  expect(user.maxStreams).toBe(3);expect(user.maxParentalRating).toBe(65);expect(user.expiresAt).toBeTruthy()
  const login=async(device:string)=>{
   const response=await page.request.post('http://127.0.0.1:18087/Users/AuthenticateByName',{data:{Username:username,Pw:'Fixture123'},headers:{'X-Emby-Authorization':`MediaBrowser Client="Infuse", Device="P4 iPad", DeviceId="${device}", Version="8.0"`}})
   expect(response.status()).toBe(200);return await response.json()
  }
  const token=await login('p4-first'), other=await login('p4-other')
  await page.reload()
  const row=page.locator('.organize-record').filter({hasText:username})
  await row.getByRole('button',{name:'设备与历史',exact:true}).click()
  dialog=page.getByRole('dialog',{name:username+' · 设备与历史',exact:true})
  const deviceRow=dialog.locator('.organize-record').first()
  await expect(deviceRow).toContainText('Infuse')
  const devices=(await (await page.request.get(`/api/v1/media-server/users/${userId}/devices`)).json()).data.items
  const deviceId=devices[0].id
  await deviceRow.getByRole('button',{name:'撤销设备登录',exact:true}).click()
  await page.getByRole('button',{name:'确认撤销',exact:true}).click()
  await expect.poll(async()=>{const r=await page.request.get(`/api/v1/media-server/users/${userId}/devices`);return(await r.json()).data.items.find((d:any)=>d.id===deviceId).activeTokens}).toBe(0)
  const tokens=[token,other]
  // Device session identity is the DTO's SessionInfo.Id, never shown in the UI.
  for(const result of tokens){
   const r=await page.request.get(`http://127.0.0.1:18087/Users/${result.User.Id}`,{headers:{'X-Emby-Token':result.AccessToken}})
   expect(r.status()).toBe(result.SessionInfo.Id===deviceId?401:200)
  }
  await dialog.getByRole('navigation',{name:'用户活动分类',exact:true}).getByRole('button',{name:'观看历史',exact:true}).click()
  await expect(dialog.getByText('暂无记录',{exact:true})).toBeVisible()
  await dialog.getByRole('button',{name:'关闭',exact:true}).click()
  await row.getByRole('button',{name:'编辑用户',exact:true}).click()
  dialog=page.getByRole('dialog',{name:'编辑媒体用户',exact:true})
  await dialog.getByRole('spinbutton',{name:'年龄分级上限',exact:true}).fill('')
  await dialog.getByLabel('媒体用户有效期',{exact:true}).fill('')
  await dialog.getByRole('button',{name:'保存媒体用户',exact:true}).click()
  await expect(dialog).not.toBeVisible()
  users=(await (await page.request.get('/api/v1/media-server/users')).json()).data
  const cleared=users.find((u:any)=>u.id===userId)
  expect(cleared.maxStreams).toBe(3);expect(cleared.maxParentalRating).toBeNull();expect(cleared.expiresAt).toBeNull()
  for(const width of [375,1440]){
   await page.setViewportSize({width,height:1000});await page.goto('/media-server')
   await expect(page.getByRole('heading',{name:'服务设置',exact:true})).toBeVisible()
   await expect(page.getByLabel('内置媒体库服务名称',{exact:true})).toHaveValue('P4 Synthetic Living Room')
   await expect(page.getByRole('checkbox',{name:'启用内置媒体库',exact:true})).toBeChecked()
   await page.locator('.u5-workspace .page-tabs').getByRole('button',{name:'预处理',exact:true}).click();await expect(page.getByRole('button',{name:'保存预处理设置',exact:true})).toBeEnabled()
   await page.locator('.u5-workspace .page-tabs').getByRole('button',{name:'服务设置',exact:true}).click();await expect(page.getByText('服务器不中转流量。',{exact:true})).toBeVisible()
   expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
   mkdirSync(screenshots,{recursive:true});await page.screenshot({path:resolve(screenshots,`native-p4-${width}.png`),fullPage:true,animations:'disabled'})
  }
 } finally {
  if(userId){const u=(await(await page.request.get('/api/v1/media-server/users/'+userId)).json()).data;const removed=await page.request.delete('/api/v1/media-server/users/'+userId,{headers,data:{revision:u.revision}});expect(removed.status()).toBe(200)}
  const current=(await(await page.request.get('/api/v1/media-server/native/settings')).json()).data
  const response=await page.request.put('/api/v1/media-server/native/settings',{headers,data:{serverName:saved.serverName,enabled:saved.enabled,revision:current.revision}});expect(response.status()).toBe(200)
 }
})

test('F2 plugin command confirmation shows durable queued and unknown states (UI contract)',async({page})=>{
 const id='11111111-0000-4000-8000-000000000062';const fixture=JSON.parse(readFileSync(resolve(root,'internal/platform/httpapi/testdata/contracts/plugin-v2-extensions.json'),'utf8'));let submitted:any[]=[]
 await page.route('**/api/v1/plugins',route=>route.fulfill({json:{data:[{id,pluginId:'hello-go',desiredState:'enabled',state:'running',revision:1,reason:''}]}}))
 await page.route('**/api/v1/plugins?*',route=>route.fulfill({json:{data:[{id,pluginId:'hello-go',desiredState:'enabled',state:'running',revision:1,reason:''}]}}))
 await page.route(`**/api/v1/plugins/${id}/extensions`,route=>route.fulfill({json:{data:fixture.snapshot}}))
 await page.route(`**/api/v1/plugins/${id}/runs?*`,route=>route.fulfill({json:{data:fixture.history}}))
 await page.route(`**/api/v1/plugins/${id}/run`,route=>{submitted.push(route.request().postDataJSON());return route.fulfill({status:202,json:{data:{runId:'owned-run'}}})})
 await fixtureLogin(page);await page.goto('/plugins/'+id);await page.getByRole('button',{name:'服务与命令',exact:true}).click();const dialog=page.getByRole('region',{name:'插件服务与命令',exact:true});await expect(dialog).toContainText('结果未知');expect(submitted).toHaveLength(0)
 await dialog.getByRole('button',{name:/^执行 /}).click();await page.getByRole('dialog',{name:'执行插件扩展',exact:true}).getByRole('button',{name:'取消',exact:true}).click();expect(submitted).toHaveLength(0)
 await dialog.getByRole('button',{name:/^执行 /}).click();await page.getByRole('dialog',{name:'执行插件扩展',exact:true}).getByRole('button',{name:'确认执行',exact:true}).click();await expect(dialog).toContainText('不是执行成功');expect(submitted).toHaveLength(1);expect(submitted[0]).toMatchObject({kind:'command',id:'hello',confirm:true});await page.screenshot({path:resolve(screenshots,'plugin-extensions.png'),animations:'disabled'})
})

test('F2 plugin JSON form, shared ESM, multi-key dashboards and sidebar (UI contract)',async({page})=>{
 const id='00000000-0000-4000-8000-000000000123',instance={id,pluginId:'hello-go',desiredState:'enabled',state:'running',revision:2,reason:''},configs:any[]=[]
 const contract=JSON.parse(readFileSync(resolve(root,'internal/platform/httpapi/testdata/contracts/plugin-v2-ui.json'),'utf8'))
 await page.route('**/api/v1/plugins?*',r=>r.fulfill({json:{data:[instance]}}))
 await page.route('**/api/v1/plugins/ui-surfaces',r=>r.fulfill({json:{data:[{instanceId:id,pluginId:'hello-go'}]}}))
 await page.route(`**/api/v1/plugins/${id}?*`,r=>r.fulfill({json:{data:{instance,config:{revision:1,value:{greeting:'Initial',scheduled:true}},entries:[],next:'',fingerprint:'owned-fingerprint'}}}))
 await page.route('**/api/v1/plugins/operations',async r=>{configs.push(r.request().postDataJSON());await r.fulfill({json:{data:{...instance,revision:3}}})})
 await page.route(`**/api/v1/plugins/${id}/ui/*`,r=>{const url=new URL(r.request().url()),kind=url.pathname.split('/').pop()!;return r.fulfill({json:{data:kind==='dashboard'?{content:[{component:'Text',props:{text:'Dashboard '+url.searchParams.get('key')}}]}:contract[kind]}})})
 await fixtureLogin(page);await page.goto('/plugins/'+id)

 const dialog=page.locator('.u6-workspace')
 // Wait for the initial config read before editing; the form and review load independently.
 await expect(dialog.getByRole('textbox',{name:'插件配置 JSON',exact:true})).toHaveValue(/Initial/)
 await expect(dialog.getByRole('textbox',{name:'问候语',exact:true})).toHaveValue('Initial')
 await dialog.getByRole('textbox',{name:'问候语',exact:true}).fill('Browser Owned')
 await expect(dialog.getByRole('textbox',{name:'插件配置 JSON',exact:true})).toHaveValue(/Browser Owned/)
 await expect(dialog).toContainText('定时服务已开启')
 await dialog.getByRole('button',{name:'保存插件配置',exact:true}).click();await page.getByRole('dialog',{name:'确认插件操作',exact:true}).getByRole('button',{name:'确认操作',exact:true}).click()
 expect(configs.at(-1)?.config).toEqual({greeting:'Browser Owned',scheduled:true})
 await page.locator('.u6-workspace').getByRole('link',{name:/打开插件页面/}).click()
 await expect(page.getByRole('button',{name:'交互 0',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'交互 0',exact:true}).click()
 await expect(page.getByRole('button',{name:'交互 1',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'查看 JSON 页面',exact:true}).click()
 await expect(page.getByText('不支持的组件：Unknown',{exact:true})).toBeVisible()
 await page.goto('/dashboard')
 await expect(page.getByRole('heading',{name:'启动次数',exact:true})).toBeVisible()
 await expect(page.getByText('Dashboard starts',{exact:true})).toBeVisible()
 await expect(page.getByText('Dashboard events',{exact:true})).toBeVisible()
 await expect(page.getByRole('navigation',{name:'插件应用',exact:true})).toBeVisible()
 await expect(page.getByRole('link',{name:'Hello Go · Hello 插件',exact:true})).toBeVisible()
})

// Exercise real Vue click handlers without the secure-context-only browser API.
// Backend fixtures isolate layout/error recovery; the next Linux test uses real workers.
test('R-F3 HTTP plugin install, enable and execute recover from failures with bounded confirms (UI contract)',async({page})=>{
 await page.addInitScript(()=>Object.defineProperty(window.crypto,'randomUUID',{value:undefined,configurable:true}))
 const contract=JSON.parse(readFileSync(resolve(root,'internal/platform/httpapi/testdata/contracts/plugin-v2-instances.json'),'utf8'))
 const ext=JSON.parse(readFileSync(resolve(root,'internal/platform/httpapi/testdata/contracts/plugin-v2-extensions.json'),'utf8'))
 const definition={manifest:{id:'hello-go',name:'Hello Go',version:'1.0.0',description:'HTTP regression fixture',permissions:contract.request.scopes.map((scope:string)=>({scope}))},fingerprint:contract.request.fingerprint}
 let instance:any=null,failInstall=true,failRun=true;const operations:any[]=[],runs:any[]=[]
 await page.route('**/api/v1/plugins/catalog?*',r=>r.fulfill({json:{data:{items:[definition],total:1}}}))
 await page.route('**/api/v1/plugins?*',r=>r.fulfill({json:{data:instance?[instance]:[]}}))
 await page.route('**/api/v1/plugins/operations',async r=>{
  const input=r.request().postDataJSON();operations.push(input)
  if(input.kind==='install'&&failInstall){failInstall=false;await r.fulfill({status:503,json:{error:{message:'安装暂不可用'}}});return}
  instance=input.kind==='install'?{...contract.response}:{...instance,desiredState:'enabled',state:'running',revision:2}
  await r.fulfill({json:{data:instance}})
 })
 await page.route(`**/api/v1/plugins/${contract.response.id}/extensions`,r=>r.fulfill({json:{data:ext.snapshot}}))
 await page.route(`**/api/v1/plugins/${contract.response.id}/runs?*`,r=>r.fulfill({json:{data:[]}}))
 await page.route(`**/api/v1/plugins/${contract.response.id}/run`,async r=>{
  runs.push(r.request().postDataJSON())
  if(failRun){failRun=false;await r.fulfill({status:503,json:{error:{message:'执行暂不可用'}}});return}
  await r.fulfill({status:202,json:{data:{runId:'owned-run'}}})
 })
 const bounded=async(title:string,width:number)=>{
  const dialog=page.getByRole('dialog',{name:title,exact:true});await expect(dialog).toBeVisible()
  const panel=dialog.locator('.el-dialog__body').locator('..');const box=await panel.boundingBox();expect(box).not.toBeNull();expect(box!.width).toBeLessThanOrEqual(561);expect(box!.width).toBeLessThanOrEqual(width*0.94+1);expect(box!.x).toBeGreaterThanOrEqual(width*0.03-1)
  expect(await panel.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
  return dialog
 }
 await fixtureLogin(page);await page.goto('/plugins?tab=catalog')
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
 await page.getByRole('region',{name:'内置插件目录'}).locator('article').filter({hasText:'Hello Go'}).getByRole('button',{name:'安装插件',exact:true}).click()
 const consent=page.getByRole('dialog',{name:'插件权限确认',exact:true});await consent.getByRole('checkbox').check()
 await consent.getByRole('button',{name:'确认安装插件',exact:true}).click();await expect(page.getByRole('alert')).toContainText('结果未确认')
 await expect(consent).toBeHidden()
 await page.getByRole('button',{name:'核对原请求',exact:true}).click();await expect(consent).toBeHidden()
 await expect.poll(()=>operations.length).toBe(2);expect(operations[0].operationKey).toBe(operations[1].operationKey)
 await page.goto('/plugins/'+contract.response.id);const card=page.locator('.u6-workspace');await expect(card).toContainText('已停用')
 for(const width of [1440,375]){
  await page.setViewportSize({width,height:1000});await card.getByRole('button',{name:'启用插件',exact:true}).click()
  await bounded('确认插件操作',width);await page.keyboard.press('Escape');await expect(page.getByRole('dialog',{name:'确认插件操作',exact:true})).toBeHidden()
 }
 await card.getByRole('button',{name:'启用插件',exact:true}).click()
 await (await bounded('确认插件操作',375)).getByRole('button',{name:'确认操作',exact:true}).click();await expect(card).toContainText(/启动中|运行中/)
 await page.setViewportSize({width:1440,height:1000});await card.getByRole('button',{name:'服务与命令',exact:true}).click()
 const extensions=page.getByRole('region',{name:'插件服务与命令',exact:true})
 for(const width of [1440,375]){
  await page.setViewportSize({width,height:1000});await extensions.getByRole('button',{name:/^执行 /}).click()
  await bounded('执行插件扩展',width)
  await page.getByRole('dialog',{name:'执行插件扩展',exact:true}).locator('.el-dialog__body').screenshot({path:resolve(screenshots,`plugin-confirm-${width}.png`)})
  await page.keyboard.press('Escape');await expect(page.getByRole('dialog',{name:'执行插件扩展',exact:true})).toBeHidden()
 }
 await page.setViewportSize({width:1440,height:1000})
 for(let attempt=0;attempt<2;attempt++){
  if(!attempt){await extensions.getByRole('button',{name:/^执行 /}).click();await page.getByRole('dialog',{name:'执行插件扩展',exact:true}).getByRole('button',{name:'确认执行',exact:true}).click()}else await extensions.getByRole('button',{name:'核对原执行请求',exact:true}).click()
  if(!attempt)await expect(page.getByRole('alert').filter({hasText:'执行请求结果尚未确认，请先核对。'})).toBeVisible()
  else await expect(extensions).toContainText('不是执行成功')
  await expect(extensions.getByRole('button',{name:/^执行 /})).toBeDisabled()
 }
 await expect.poll(()=>runs.length).toBe(2);expect(runs[0].operationKey).toBe(runs[1].operationKey)
 for(const input of [...operations,...runs])expect(input.operationKey).toMatch(/^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$/)
 await page.getByRole('button',{name:'配置',exact:true}).click();await expect(extensions).toBeHidden()
 expect(errors).toEqual([])
})

test('R-F3 HTTP clicks install, enable, execute and disable the real same-binary plugin (Linux)',async({page})=>{
 test.skip(process.platform!=='linux','Actual worker process accounting requires Linux; native AMD64 CI runs this test')
 await page.addInitScript(()=>Object.defineProperty(window.crypto,'randomUUID',{value:undefined,configurable:true}))
 await fixtureLogin(page);await page.goto('/plugins?tab=catalog')
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
 let id='',completed=false
 try{
  const installation=page.waitForResponse(r=>r.url().endsWith('/api/v1/plugins/operations')&&r.request().postDataJSON().kind==='install')
  await page.getByRole('region',{name:'内置插件目录'}).locator('article').filter({hasText:'Hello Go'}).getByRole('button',{name:'安装插件',exact:true}).click()
  const consent=page.getByRole('dialog',{name:'插件权限确认',exact:true});await consent.getByRole('checkbox').check();await consent.getByRole('button',{name:'确认安装插件',exact:true}).click()
  const installed=await installation;expect(installed.status()).toBe(200);id=(await installed.json()).data.id
  await page.goto('/plugins/'+id);const card=page.locator('.u6-workspace')
  await card.getByRole('button',{name:'启用插件',exact:true}).click();await page.getByRole('dialog',{name:'确认插件操作',exact:true}).getByRole('button',{name:'确认操作',exact:true}).click()
  await expect.poll(async()=>{await page.getByRole('button',{name:'刷新',exact:true}).click();return (await card.getByRole('button',{name:'停用插件',exact:true}).isVisible())&&((await (await page.request.get(`/api/v1/plugins/${id}`)).json()).data.instance.state==='running')},{timeout:20000}).toBe(true)
  await card.getByRole('button',{name:'服务与命令',exact:true}).click()
  const dialog=page.getByRole('region',{name:'插件服务与命令',exact:true})
  await dialog.getByRole('button',{name:/^执行 /}).first().click()
  const execution=page.waitForResponse(r=>r.url().endsWith(`/plugins/${id}/run`))
  await page.getByRole('dialog',{name:'执行插件扩展',exact:true}).getByRole('button',{name:'确认执行',exact:true}).click();expect((await execution).status()).toBe(202)
  await expect.poll(async()=>((await (await page.request.get(`/api/v1/plugins/${id}/runs?page=1&pageSize=50`,{timeout:5000})).json()).data as any[]).some(r=>r.state==='succeeded'),{timeout:20000}).toBe(true)
  await page.getByRole('button',{name:'实例数据',exact:true}).click()
  await card.getByRole('button',{name:'查看 KV',exact:true}).click();await expect(page.getByRole('dialog',{name:'实例 KV 数据',exact:true})).toContainText('starts')
  await page.getByRole('dialog',{name:'实例 KV 数据',exact:true}).getByRole('button',{name:'关闭',exact:true}).click({timeout:5000})
  await card.getByRole('button',{name:'停用插件',exact:true}).click();await page.getByRole('dialog',{name:'确认插件操作',exact:true}).getByRole('button',{name:'确认操作',exact:true}).click();await expect(card).toContainText('已停用')
  expect(errors).toEqual([]);completed=true
 }finally{
  if(id){try{
   const detail=(await (await page.request.get(`/api/v1/plugins/${id}`,{timeout:5000})).json()).data
   const response=await page.request.post('/api/v1/plugins/operations',{timeout:5000,headers:{Origin:'http://localhost:18084','X-Requested-With':'media-tool'},data:{kind:'remove',instanceId:id,fingerprint:detail.fingerprint,expectedRevision:detail.instance.revision,operationKey:`rf3-cleanup-${id}`,confirm:true}})
   expect(response.status()).toBe(200)
  }catch(error){if(completed)throw error;console.info('Failed test cleanup deferred to isolated test-database reset; original error preserved')}}
 }
})

test('F5-alpha built-in metadata, scope upgrade review and missing definition preserve instance data',async({page})=>{
 const plan=JSON.parse(readFileSync(resolve(root,'internal/platform/httpapi/testdata/contracts/plugin-library-review.json'),'utf8'))
 let instance:any={...plan.instance},missing=false,approved=false;const operations:any[]=[]
 const definition={manifest:plan.current,fingerprint:plan.fingerprint,installedCount:1}
 await page.route('**/api/v1/plugins/catalog?*',r=>r.fulfill({json:{data:{items:missing?[]:[definition],total:missing?0:1}}}))
 await page.route('**/api/v1/plugins?*',r=>r.fulfill({json:{data:[instance]}}))
 await page.route(`**/api/v1/plugins/${instance.id}/review`,r=>r.fulfill({json:{data:{...plan,instance,availability:missing?'missing':'available',baselineKnown:!missing,approvedVersion:missing?'':plan.approvedVersion,needsReview:!approved,added:approved?[]:plan.added,removed:approved?[]:plan.removed,changed:approved?[]:plan.changed}}}))
 await page.route(`**/api/v1/plugins/${instance.id}?*`,r=>r.fulfill({json:{data:{instance,config:{value:{retained:'configuration'},revision:1},entries:[{key:'retained',value:'private-instance-data',revision:1}],next:'',fingerprint:plan.fingerprint}}}))
 await page.route('**/api/v1/plugins/operations',async r=>{const input=r.request().postDataJSON();operations.push(input);expect(input.kind).toBe('review');approved=true;instance={...instance,desiredState:'disabled',state:'stopped',reason:'',revision:8};await r.fulfill({json:{data:instance}})})
 await fixtureLogin(page);await page.goto('/plugins?tab=catalog');const directory=page.getByRole('region',{name:'内置插件目录'});await expect(directory).toContainText('Fixture Maintainer');await expect(directory).toContainText('等级 2');await expect(directory).toContainText('已安装 1 个实例');await expect(directory.locator('img')).toHaveAttribute('src','/plugins/hello-go/icon.svg');await directory.locator('summary').click();await expect(directory).toContainText('新增设置权限');await expect(directory).toContainText('settings.write');await page.screenshot({path:resolve(screenshots,'plugin-directory-f5a.png')});await page.goto('/plugins/'+instance.id);const card=page.locator('.u6-workspace');await expect(page.getByRole('button',{name:'启用插件',exact:true})).toBeDisabled();await page.getByRole('button',{name:'重新确认权限',exact:true}).click();const dialog=page.getByRole('dialog',{name:'插件权限确认',exact:true});await expect(dialog).toContainText('legacy.removed');await expect(dialog).toContainText('资源范围变化');await expect(dialog).toContainText('高风险权限');await page.setViewportSize({width:390,height:1000});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await dialog.screenshot({path:resolve(screenshots,'plugin-review-f5a-390.png')});await dialog.getByRole('checkbox').check();await dialog.getByRole('button',{name:'核对结果',exact:true}).click();await expect(dialog.getByRole('checkbox')).not.toBeChecked();await dialog.getByRole('checkbox').check();await dialog.getByRole('button',{name:'确认权限并保持停用',exact:true}).click();await expect(dialog).toBeHidden();await expect(card).toContainText('已停用');expect(operations).toHaveLength(1);expect(operations[0].kind).toBe('review');missing=true;instance={...instance,availability:'missing',reason:'definition_missing'};await page.getByRole('button',{name:'刷新',exact:true}).click();await expect(card).toContainText('Hello Go');await expect(card).toContainText('配置和数据仍保留');await expect(page.getByRole('button',{name:'启用插件',exact:true})).toBeDisabled();await expect(page.getByRole('button',{name:'创建分身',exact:true})).toBeDisabled();await page.getByRole('button',{name:'查看权限',exact:true}).click();await expect(dialog).toContainText('历史授权范围不完整');await dialog.getByRole('checkbox').check();await expect(dialog.getByRole('button',{name:'确认权限并保持停用',exact:true})).toBeDisabled();await dialog.getByRole('button',{name:'取消',exact:true}).click();await page.getByRole('button',{name:'实例数据',exact:true}).click();await page.getByRole('button',{name:'查看 KV',exact:true}).click();await expect(page.getByRole('dialog',{name:'实例 KV 数据',exact:true})).toContainText('private-instance-data');await page.screenshot({path:resolve(screenshots,'plugin-library-f5a.png'),fullPage:true})
})

test('F5-beta real instance snapshot create restore and delete stay disabled',async({page})=>{
 await fixtureLogin(page)
 const d=(await(await page.request.get('/api/v1/plugins/catalog')).json()).data.items[0]
 const op=async(kind:string,extra:any={})=>{const r=await page.request.post('/api/v1/plugins/operations',{headers:{'X-Requested-With':'media-tool'},data:{kind,operationKey:crypto.randomUUID(),pluginId:d.manifest.id,fingerprint:d.fingerprint,expectedRevision:0,confirm:true,scopes:d.manifest.permissions.map((p:any)=>p.scope),config:{snapshotMarker:'original'},...extra}});expect(r.status()).toBe(kind==='configure'?202:200);return(await r.json()).data}
 const instance=await op('install');const inspect=async()=>(await(await page.request.get(`/api/v1/plugins/${instance.id}`)).json()).data
 await page.goto('/plugins/'+instance.id);await page.getByRole('button',{name:'实例数据',exact:true}).click();const card=page.locator('.u6-workspace');await card.getByRole('button',{name:'数据快照',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'实例数据快照',exact:true})
 const confirm=async()=>{await page.getByRole('dialog',{name:'确认实例快照操作',exact:true}).getByRole('button',{name:'确认操作',exact:true}).click()}
 await dialog.getByRole('button',{name:'创建实例快照',exact:true}).click();await confirm();await expect(dialog).toContainText('ready')
 let info=await inspect();await op('configure',{instanceId:instance.id,expectedRevision:info.instance.revision,config:{snapshotMarker:'changed'}})
 await dialog.getByRole('button',{name:'恢复',exact:true}).click();await confirm();await expect.poll(async()=>(await inspect()).config.value.snapshotMarker).toBe('original');info=await inspect();expect(info.instance.desiredState).toBe('disabled')
 await dialog.getByRole('button',{name:'删除',exact:true}).click();await confirm();await expect(dialog).toContainText('尚无实例快照')
 await dialog.screenshot({path:resolve(screenshots,'plugin-snapshots-f5b.png'),animations:'disabled'})
 info=await inspect();await op('remove',{instanceId:instance.id,expectedRevision:info.instance.revision,scopes:[]})
})

test('F5-gamma manual tools workflow schema and auth status stay bounded at 390px (UI contract)',async({page})=>{
 await page.addInitScript(()=>Object.defineProperty(window.crypto,'randomUUID',{value:undefined,configurable:true}))
 const fixture=JSON.parse(readFileSync(resolve(root,'internal/platform/httpapi/testdata/contracts/plugin-v2-instances.json'),'utf8'));const instance={...fixture.response,desiredState:'enabled',state:'running'}
 const manifest={id:'hello-go',name:'Hello Go',version:'1.9.0',description:'panels fixture',permissions:[]}
 await page.route('**/api/v1/plugins/catalog?*',r=>r.fulfill({json:{data:{items:[{manifest,fingerprint:'current'}],total:1}}}))
 await page.route('**/api/v1/plugins?*',r=>r.fulfill({json:{data:[instance]}}))
 await page.route(`**/api/v1/plugins/${instance.id}/tools`,r=>r.fulfill({json:{data:[{kind:'tool',id:'hello.remember',name:'hello.remember',description:'Only own KV',inputSchema:{type:'object'},effectKind:'write',requiredScopes:['agent.tools','plugin.data.write']}]}}))
 const calls:any[]=[];await page.route(`**/api/v1/plugins/${instance.id}/tools/call`,async r=>{calls.push(r.request().postDataJSON());await r.fulfill({json:{data:{saved:true}}})})
 await page.route('**/api/v1/plugins/workflows?*',r=>r.fulfill({json:{data:[{id:'owned',revision:1,fingerprint:'new',approved:'old',steps:1}]}}))
 await page.route('**/api/v1/plugins/workflows?workflowId=owned&view=definition',r=>r.fulfill({json:{data:{dag:{id:'owned',steps:[]},revision:1,fingerprint:'new',approved:'old'}}}))
 await page.route('**/api/v1/plugins/workflows?workflowId=owned&view=runs&*',r=>r.fulfill({json:{data:[{id:'run-fixture',workflowId:'owned',state:'unknown',steps:{one:'unknown'},reasons:{one:'worker_lost'}}]}}))
 await page.route(`**/api/v1/plugins/${instance.id}/auth/status`,r=>r.fulfill({json:{data:[{id:'fake',instanceId:instance.id,bound:0,stale:1,state:'unavailable',error:'provider_inactive_or_scope_denied'}]}}))
 await fixtureLogin(page);await page.goto('/plugins/'+instance.id);await page.getByRole('button',{name:'实例数据',exact:true}).click();await page.setViewportSize({width:390,height:1000})
 const card=page.locator('.u6-workspace');await card.getByRole('button',{name:'工具测试',exact:true}).click();let dialog=page.getByRole('dialog',{name:'人工工具测试',exact:true});await expect(dialog).toContainText('仅管理员可手动调用工具');await dialog.getByLabel('输入 JSON',{exact:true}).fill('{"message":"manual"}');await dialog.getByRole('button',{name:'人工调用工具',exact:true}).click();const consent=page.getByRole('dialog',{name:'确认工具调用',exact:true});await expect(consent).toBeVisible();expect(calls).toHaveLength(0);await consent.getByRole('button',{name:'确认调用',exact:true}).click();await expect(dialog).toContainText('"saved": true');expect(calls[0].confirm).toBe(true);expect(calls[0].operationKey).toMatch(/^[a-f0-9-]{36}$/)
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await dialog.screenshot({path:resolve(screenshots,'plugin-tools-f5g-390.png'),animations:'disabled'});await dialog.getByRole('button',{name:'关闭此对话框',exact:true}).click()
 await page.getByRole('button',{name:'工作流管理',exact:true}).click();dialog=page.getByRole('dialog',{name:'工作流管理',exact:true});await dialog.getByRole('button',{name:'详情',exact:true}).click();await dialog.locator('details').filter({has:page.locator('pre').filter({hasText:'worker_lost'})}).locator('summary').click();await expect(dialog).toContainText('worker_lost');await expect(dialog.getByRole('button',{name:'手动运行工作流',exact:true})).toBeDisabled();await expect(dialog).toContainText('待重新批准');expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await dialog.getByRole('button',{name:'关闭此对话框',exact:true}).click()
 await card.getByRole('button',{name:'认证状态',exact:true}).click();dialog=page.getByRole('dialog',{name:'认证状态',exact:true});await expect(dialog).toContainText('不支持');await dialog.getByText('技术详情',{exact:true}).click();await expect(dialog).toContainText('provider_inactive_or_scope_denied');await expect(dialog.getByRole('button',{name:'绑定账号',exact:true})).toBeDisabled();expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
})

test('R committed install response loss retries the original key against the real host',async({page})=>{
 await page.addInitScript(()=>Object.defineProperty(window.crypto,'randomUUID',{value:undefined,configurable:true}))
 await fixtureLogin(page);await page.goto('/plugins?tab=catalog')
 let id='',lost=false;const keys:string[]=[]
 await page.route('**/api/v1/plugins/operations',async r=>{
  const input=r.request().postDataJSON();if(input.kind!=='install'){await r.continue();return}
  keys.push(input.operationKey)
  if(!lost){lost=true;const response=await r.fetch();expect(response.status()).toBe(200);id=(await response.json()).data.id;await r.abort('connectionreset');return}
  await r.continue()
 })
 try{
  const directory=page.getByRole('region',{name:'内置插件目录'}).locator('article').filter({hasText:'Hello Go'})
  await directory.getByRole('button',{name:/安装插件|新增独立实例/}).click();const consent=page.getByRole('dialog',{name:'插件权限确认',exact:true});await consent.getByRole('checkbox').check();await consent.getByRole('button',{name:'确认安装插件',exact:true}).click();await expect(page.getByRole('alert')).toContainText('结果未确认')
  await page.reload();await page.getByRole('button',{name:'核对原请求',exact:true}).click();await expect.poll(()=>keys.length).toBe(2);expect(keys[0]).toBe(keys[1]);await expect(page.getByRole('status')).toContainText('已安装')
  const detail=(await(await page.request.get(`/api/v1/plugins/${id}`)).json()).data;expect(detail.instance.desiredState).toBe('disabled');expect(detail.instance.revision).toBe(1)
  expect(((await(await page.request.get('/api/v1/plugins')).json()).data as any[]).filter(i=>i.id===id)).toHaveLength(1)
 }finally{if(id){const detail=(await(await page.request.get(`/api/v1/plugins/${id}`)).json()).data;const r=await page.request.post('/api/v1/plugins/operations',{headers:{'X-Requested-With':'media-tool'},data:{kind:'remove',instanceId:id,fingerprint:detail.fingerprint,expectedRevision:detail.instance.revision,operationKey:`r-alpha-cleanup-${id}`,confirm:true}});expect(r.status()).toBe(200)}}
})
