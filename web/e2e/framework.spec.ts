import { test,expect } from '@playwright/test'
import { mkdirSync,readFileSync,writeFileSync,existsSync } from 'node:fs'
import { resolve } from 'node:path'
import { navigationItems } from '../src/navigation/items'
const root=resolve(import.meta.dirname,'../..')
const schemaMatch=readFileSync(resolve(root,'internal/platform/buildinfo/buildinfo.go'),'utf8').match(/^const Schema int64 = (\d+)$/m)
if(!schemaMatch)throw new Error('Build schema declaration is missing')
const expectedSchema=Number(schemaMatch[1])
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
 const item=navigationItems.find(item=>item.name===name);if(!item)throw new Error('Unknown navigation target: '+name)
 let menu=page.getByRole('navigation',{name:'功能导航'})
 if(!await menu.isVisible())await page.getByRole('button',{name:'打开导航',exact:true}).click()
 const toggle=menu.getByRole('button',{name:item.group,exact:true})
 if(await toggle.getAttribute('aria-expanded')!=='true')await toggle.click()
 await menu.getByRole('link',{name,exact:true}).click()
}
async function login(page:import('@playwright/test').Page){
 await page.goto('/login')
 await expect(page.getByRole('heading',{name:'进入管理工作台',exact:true})).toBeVisible()
 await page.getByLabel('管理员账号',{exact:true}).fill(username)
 await page.getByLabel('密码',{exact:true}).fill(password)
 await page.getByRole('button',{name:'进入工作台',exact:true}).click()
 await expect(page.getByRole('heading',{name:'运行总览'})).toBeVisible()
 await expect(page.getByText('服务器负载',{exact:true})).toBeVisible()
 await expect(page.locator('.sidebar-note')).toContainText('0.27.0-dev')
}
// Later UI-only fixtures share one valid account session in memory. Login/
// logout/password/rate-limit behavior above remains separately exercised; do
// not weaken production limits just because the suite gained more UI fixtures.
let fixtureCookies:Awaited<ReturnType<import('@playwright/test').BrowserContext['cookies']>>|undefined
async function fixtureLogin(page:import('@playwright/test').Page){
 if(!fixtureCookies){await login(page);fixtureCookies=await page.context().cookies();return}
 await page.context().addCookies(fixtureCookies)
 await page.goto('/dashboard')
 await expect(page.getByRole('heading',{name:'运行总览'})).toBeVisible()
}
test('first registration creates the administrator and closes public registration',async({page,request})=>{
 await page.goto('/login');await expect(page.getByRole('heading',{name:'注册管理员账号',exact:true})).toBeVisible()
 await expect(page.getByText('6–20个字符，支持中文、空格，不会去除密码首尾空格。',{exact:true})).toBeVisible()
 await page.getByLabel('管理员账号',{exact:true}).fill(username);await page.getByLabel('密码',{exact:true}).fill('abc12');await page.getByLabel('确认密码',{exact:true}).fill('abc12');await page.getByRole('button',{name:'注册并进入工作台'}).click();await expect(page.getByText('密码需6–20个字符，可使用空格和中文',{exact:true})).toBeVisible()
 await page.getByLabel('管理员账号',{exact:true}).fill(username);await page.getByLabel('密码',{exact:true}).fill(password);await page.getByLabel('确认密码',{exact:true}).fill(password+'X')
 mkdirSync(screenshots,{recursive:true});await page.screenshot({path:resolve(screenshots,'account-registration-desktop.png'),fullPage:true,mask:[page.getByLabel('密码',{exact:true}),page.getByLabel('确认密码',{exact:true})]})
 await page.getByRole('button',{name:'注册并进入工作台'}).click();await expect(page.getByText('两次输入的密码不一致',{exact:true})).toBeVisible()
 await page.getByLabel('确认密码',{exact:true}).fill(password);const registered=page.waitForResponse(r=>r.url().endsWith('/auth/register'));await page.getByRole('button',{name:'注册并进入工作台'}).click();const reply=await registered;expect(reply.status()).toBe(201);expect(await reply.text()).not.toContain(password)
 await expect(page.getByRole('heading',{name:'运行总览'})).toBeVisible();const setup=await request.get('/api/v1/auth/setup');expect((await setup.json()).data.registrationOpen).toBe(false)
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
 await expect(page.getByRole('navigation',{name:'系统设置分类'})).toBeVisible()
 await expect(page.getByRole('button',{name:'验证 CloudDrive2',exact:true})).toHaveCount(0)
 await expect(page.getByRole('button',{name:'验证 Emby',exact:true})).toHaveCount(0)
 await navigate(page,'PT 站点')
 await expect(page.getByRole('heading',{name:'PT 站点'})).toBeVisible()
 await page.getByRole('button',{name:'退出',exact:true}).click()
 await expect(page.getByRole('heading',{name:'进入管理工作台'})).toBeVisible()
 expect(errors).toEqual([])
})
test('STRM preview encodes cloud path and download has one stable URL',async({page})=>{
 await login(page);await navigate(page,'STRM工具')
 const path='/115/Media/电影/幽灵公主 (1997)/电影 & 特别版#1.mkv'
 await page.getByLabel('CloudDrive2 完整路径',{exact:false}).fill(path)
 await page.getByRole('button',{name:'生成固定链接'}).click()
 await page.getByRole('button',{name:'诊断云端文件',exact:true}).click()
 await expect(page.getByText('请先配置 CloudDrive2 地址及读取凭据',{exact:true})).toBeVisible()
 const textarea=page.getByRole('textbox',{name:'生成的 STRM 固定链接'})
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
 await page.getByRole('button',{name:'运行系统检查'}).click()
 await expect(page.getByText('系统检查已加入持久化队列',{exact:true})).toBeVisible()
 try{await expect(page.getByText('已完成',{exact:true}).first()).toBeVisible({timeout:15000})}catch(error){
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
 await expect(page.getByRole('heading',{name:'STRM目录',exact:true})).toBeVisible()
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
 const row=page.getByRole('row').filter({hasText:name})
 await expect(row).toBeVisible()
 await row.getByRole('button',{name:'编辑',exact:true}).click()
 await expect(page.getByLabel('CloudDrive2 API Token',{exact:true})).toHaveValue('')
 await page.getByLabel('连接名称',{exact:true}).fill(name+' updated')
 await page.getByRole('button',{name:'加密保存',exact:true}).click()
 const updated=page.getByRole('row').filter({hasText:name+' updated'})
 await updated.getByRole('button',{name:'删除',exact:true}).click()
 await page.getByRole('button',{name:'删除连接',exact:true}).click()
 await expect(updated).toHaveCount(0)
})

test('subscriptions and scanner pages expose real configuration gates',async({page})=>{
 await login(page);await navigate(page,'资源订阅')
 await expect(page.getByRole('heading',{name:'资源订阅',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'创建订阅',exact:true}).click()
 await expect(page.getByRole('button',{name:'保存订阅',exact:true})).toBeDisabled()
 await expect(page.getByText('保存后启用自动下载',{exact:true})).toBeVisible()
 await page.keyboard.press('Escape')
 await expect(page.getByRole('button',{name:'保存订阅',exact:true})).not.toBeVisible()
 await navigate(page,'云盘索引')
 await page.getByRole('tab',{name:'已入库资产',exact:true}).click();await expect(page.getByRole('heading',{name:/^已入库文件/})).toBeVisible()
 await page.getByRole('tab',{name:'扫描与计划',exact:true}).click();await page.getByRole('button',{name:'开始扫描',exact:true}).click()
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
 await page.getByRole('tab',{name:'扫描与计划',exact:true}).click();await page.getByRole('button',{name:'新增定时扫描'}).click();const dialog=page.getByRole('dialog');await expect(dialog.getByRole('switch',{name:'启用定时扫描'})).not.toBeChecked();await expect(dialog.getByRole('switch',{name:'启用CloudDrive2文件变化推送'})).not.toBeChecked()
 await dialog.getByRole('button',{name:'保存定时扫描'}).click();await expect(page.getByText('请先在服务连接中配置并启用 CloudDrive2',{exact:true})).toBeVisible()
 await dialog.getByRole('button',{name:'取消',exact:true}).click();await expect(dialog).not.toBeVisible();await page.screenshot({path:resolve(screenshots,'library-desktop.png'),fullPage:true});await page.setViewportSize({width:375,height:812});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
 mkdirSync(screenshots,{recursive:true});await page.screenshot({path:resolve(screenshots,'library-mobile.png'),fullPage:true})
})

test('wrong credentials are rejected and password change revokes captured sessions',async({page,request})=>{
 const wrong=await request.post('/api/v1/auth/login',{headers:{'X-Requested-With':'media-tool'},data:{username,password:'Wrong-strong-password-2026'}});expect(wrong.status()).toBe(401)
 await login(page);const captured=(await page.context().cookies()).find(c=>c.name==='media_tool_session')!;await navigate(page,'系统设置')
 await page.getByRole('navigation',{name:'系统设置分类'}).getByRole('button',{name:'管理员账号',exact:true}).click();const next=password+'-new';await page.getByLabel('当前密码',{exact:true}).fill(password);await page.getByLabel('新密码',{exact:true}).fill(next);await page.getByLabel('确认新密码',{exact:true}).fill(next);await page.getByRole('button',{name:'修改密码并重新登录'}).click();await expect(page.getByRole('heading',{name:'进入管理工作台',exact:true})).toBeVisible()
 const replay=await request.get('/api/v1/auth/session',{headers:{Cookie:'media_tool_session='+captured.value}});expect(replay.status()).toBe(401)
 const old=await request.post('/api/v1/auth/login',{headers:{'X-Requested-With':'media-tool'},data:{username,password}});expect(old.status()).toBe(401)
 await page.getByLabel('管理员账号',{exact:true}).fill(username);await page.getByLabel('密码',{exact:true}).fill(next);await page.getByRole('button',{name:'进入工作台',exact:true}).click();await expect(page.getByRole('heading',{name:'运行总览'})).toBeVisible()
 // Restore only the synthetic test account, never a user's administrator.
 await navigate(page,'系统设置');await page.getByRole('navigation',{name:'系统设置分类'}).getByRole('button',{name:'管理员账号',exact:true}).click();await page.getByLabel('当前密码',{exact:true}).fill(next);await page.getByLabel('新密码',{exact:true}).fill(password);await page.getByLabel('确认新密码',{exact:true}).fill(password);await page.getByRole('button',{name:'修改密码并重新登录'}).click();await expect(page.getByRole('heading',{name:'进入管理工作台',exact:true})).toBeVisible()
})

test('account login form is accessible on mobile and needs no deployment key',async({page})=>{
 await page.setViewportSize({width:375,height:812});await page.emulateMedia({reducedMotion:'reduce'});await page.goto('/login');await expect(page.getByRole('heading',{name:'进入管理工作台',exact:true})).toBeVisible();await expect(page.getByLabel('管理员账号',{exact:true})).toBeVisible();await expect(page.getByLabel('密码',{exact:true})).toHaveAttribute('autocomplete','current-password');expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);expect(await page.evaluate(()=>localStorage.length)).toBe(0);await page.screenshot({path:resolve(screenshots,'account-login-mobile.png'),fullPage:true})
})

test('official 115 is saved disabled and QR authorization is explicit and cancellable',async({page})=>{
 await login(page);await page.goto('/cloud-connections')
 await page.getByRole('button',{name:'添加连接'}).click()
 await page.getByRole('button',{name:'关闭此对话框',exact:true}).click()
 await page.getByRole('button',{name:'115 授权',exact:true}).click()
 await page.getByRole('button',{name:'添加连接',exact:true}).click()
 const name='Official115 fixture '+Date.now()
 await page.getByLabel('连接名称',{exact:true}).fill(name)
 await page.getByLabel('自己的115开放平台 AppID',{exact:true}).fill('12345')
 await expect(page.getByLabel('CloudDrive2中的115挂载前缀',{exact:true})).toHaveValue('/115')
 await page.getByRole('button',{name:'加密保存',exact:true}).click()
 const row=page.getByRole('row').filter({hasText:name});await expect(row).toBeVisible()
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
 await page.route('https://**/favicon.ico',route=>route.abort())
 await fixtureLogin(page);await page.goto('/site-templates',{waitUntil:'domcontentloaded'});await expect(page.getByRole('heading',{name:'站点模板',exact:true})).toBeVisible()
 await expect(page.getByRole('button',{name:'加载所选模板',exact:true})).toBeDisabled()
 await page.getByText('选择 HDSKY',{exact:true}).click();await expect(page.getByRole('checkbox',{name:'选择 HDSKY',exact:true})).toBeChecked()
 await page.getByText('选择 OURBITS',{exact:true}).click();await expect(page.getByRole('checkbox',{name:'选择 OURBITS',exact:true})).toBeChecked()
 await expect(page.getByRole('checkbox',{name:'全选可加载模板',exact:true})).toHaveJSProperty('indeterminate',true)
 await page.getByText('全选可加载模板',{exact:true}).click();await expect(page.getByRole('button',{name:'加载所选模板（6）',exact:true})).toBeEnabled();await page.getByRole('button',{name:'取消全选',exact:true}).click();await expect(page.getByRole('button',{name:'加载所选模板',exact:true})).toBeDisabled()
 await page.getByText('选择 HDSKY',{exact:true}).click();await page.getByText('选择 OURBITS',{exact:true}).click()
 const loadedReply=page.waitForResponse(r=>r.url().endsWith('/pt/templates/load'))
 await page.getByRole('button',{name:'加载所选模板（2）',exact:true}).click()
 expect((await loadedReply).status()).toBe(200)
 const catalog=(await(await page.request.get('/api/v1/pt/templates')).json()).data.templates
 expect(catalog.filter((x:any)=>x.loaded).map((x:any)=>x.id).sort()).toEqual(['hdsky','ourbits'])
 expect(catalog.every((x:any)=>x.version==='1.3.0'&&x.iconUrl===`/api/v1/pt/templates/${x.id}/icon?v=1.3.0`)).toBe(true)
 await page.reload({waitUntil:'domcontentloaded'});await expect(page.locator('article').filter({has:page.getByRole('heading',{name:'HDSKY',exact:true})}).getByRole('checkbox')).toBeDisabled()
 await expect(page.getByRole('checkbox',{name:'选择 SSD',exact:true})).toBeEnabled();await expect.poll(()=>page.locator('.template-card img').evaluateAll(imgs=>imgs.length===6&&imgs.every(img=>(img as HTMLImageElement).complete&&(img as HTMLImageElement).naturalWidth>0))).toBe(true)
 await page.getByText('全选可加载模板',{exact:true}).click();await expect(page.getByRole('button',{name:'加载所选模板（4）',exact:true})).toBeEnabled();await page.getByRole('button',{name:'取消全选',exact:true}).click()
 await expect(page.getByRole('link',{name:'https://hdsky.me',exact:true})).toHaveAttribute('href','https://hdsky.me')
 for(const name of ['HDSKY','OURBITS','CHDBITS','SSD','TTG','UBITS'])await expect(page.getByRole('heading',{name,exact:true})).toBeVisible()
 for(const text of ['来源提交','已导入配置','mediatool-site','天空','我堡','春天'])await expect(page.getByText(text,{exact:true})).toHaveCount(0)
 await page.screenshot({path:resolve(screenshots,'go-templates-desktop.png'),fullPage:true,animations:'disabled'});await page.setViewportSize({width:390,height:844});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true)
 await page.screenshot({path:resolve(screenshots,'go-templates-mobile.png'),fullPage:true,animations:'disabled'})
})

test('strict direct-only policy is explicit, persistent and visible (isolated settings fixture)',async({page})=>{
 await fixtureLogin(page);await page.goto('/mediaLibrary')
 await page.getByRole('button',{name:'Emby 配置',exact:true}).click()
 await page.getByRole('button',{name:'添加连接'}).click()
 const name='Direct-only fixture '+Date.now()
 await page.getByLabel('连接名称',{exact:true}).fill(name)
 await page.getByLabel('Emby 服务地址',{exact:true}).fill('http://127.0.0.1:9')
 await page.getByText('仅直链播放：禁止转码和媒体中转',{exact:true}).click()
 await expect(page.getByRole('checkbox',{name:'仅直链播放：禁止转码和媒体中转'})).toBeChecked()
 await expect(page.getByText(/不支持原始TS\/MKV或音频格式/)).toBeVisible()
 await page.getByRole('button',{name:'加密保存',exact:true}).click()
 const row=page.getByRole('row').filter({hasText:name})
 await row.getByRole('button',{name:'编辑',exact:true}).click()
 await expect(page.getByRole('checkbox',{name:'仅直链播放：禁止转码和媒体中转'})).toBeChecked()
 await page.getByRole('button',{name:'关闭此对话框',exact:true}).click()
 await row.getByRole('button',{name:'删除',exact:true}).click()
 await page.getByRole('button',{name:'删除连接',exact:true}).click()
 await expect(row).toHaveCount(0)
})

test('management Emby page survives a direct reload without entering the playback listener',async({page})=>{
 await fixtureLogin(page)
 await page.goto('/emby')
 await expect(page.getByRole('heading',{name:'Emby 播放网关',exact:true})).toBeVisible()
 await page.reload()
 await expect(page.getByRole('heading',{name:'Emby 播放网关',exact:true})).toBeVisible()
 await expect(page).toHaveURL('http://localhost:18084/emby')
 await expect(page.getByRole('link',{name:'编辑Emby连接',exact:true})).toBeVisible()
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
 const row=page.getByRole('row').filter({hasText:'自有电影'})
 await page.getByRole('tab',{name:'已入库资产',exact:true}).click();await row.getByRole('button',{name:'修正身份 / 季集',exact:true}).click()
 const dialog=page.getByRole('dialog')
 await dialog.getByRole('button',{name:'查询TMDB'}).click()
 await dialog.getByRole('row').filter({hasText:'自有新电影'}).click()
 await expect(dialog.getByRole('checkbox',{name:'同时迁移Movies/TV输出目录（先预览）'})).not.toBeChecked()
 await dialog.getByText('同时迁移Movies/TV输出目录（先预览）',{exact:true}).click()
 await dialog.getByRole('button',{name:'预览输出迁移',exact:true}).click()
 const confirm=page.getByRole('dialog',{name:'确认输出迁移',exact:true})
 await expect(confirm).toBeVisible();await expect(page.getByRole('dialog',{name:'修正媒体身份与季集',exact:true})).toBeHidden();expect(applied).toHaveLength(0)
 await expect(confirm.getByText('归档旧输出',{exact:true})).toBeVisible()
 await expect(confirm.getByText('Movies/自有电影/自有电影.strm',{exact:true})).toBeVisible()
 await page.screenshot({path:resolve(screenshots,'output-migration-preview.png'),fullPage:true,animations:'disabled'})
 await page.setViewportSize({width:375,height:812});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
 await page.screenshot({path:resolve(screenshots,'output-migration-preview-mobile.png'),fullPage:true,animations:'disabled'})
 await page.setViewportSize({width:1440,height:1000})
 await confirm.getByRole('button',{name:'返回检查',exact:true}).click()
 expect(applied).toHaveLength(0)
 await page.getByRole('tab',{name:'已入库资产',exact:true}).click();await row.getByRole('button',{name:'修正身份 / 季集',exact:true}).click()
 await page.getByRole('dialog').getByRole('button',{name:'查询TMDB'}).click()
 await page.getByRole('dialog').getByRole('row').filter({hasText:'自有新电影'}).click()
 await page.getByRole('dialog').getByText('同时迁移Movies/TV输出目录（先预览）',{exact:true}).click()
 await page.getByRole('dialog').getByRole('button',{name:'预览输出迁移',exact:true}).click()
 await confirm.getByRole('button',{name:'确认生成并归档',exact:true}).click()
 await expect.poll(()=>applied.length).toBe(1)
 expect(applied[0]?.planDigest).toBe(preview.planDigest)
 expect(applied[0]?.migrateOutputs).toBe(true)
 await expect(page.getByText('新输出已就绪，旧托管输出已归档；文件播放链接保持不变',{exact:true})).toBeVisible()
})

test('episode metadata preview is read-only, explicit and cleared when season coverage changes (fixture)',async({page})=>{
 const asset='00000000-0000-4000-8000-000000000095'
 let writes=0,previewQueries:string[]=[]
 await page.route('**/api/v1/media/assets?page=*',route=>route.fulfill({json:{data:{items:[{id:asset,title:'自有剧集',path:'/115/owned.mkv',state:'present',revision:1,artifactsState:'ready',fileScoped:true}],total:1,page:1,pageSize:50}}}))
 await page.route('**/api/v1/metadata/search?*',route=>route.fulfill({json:{data:[{id:99,mediaType:'series',title:'自有剧集',year:'2025'}]}}))
 await page.route('**/api/v1/metadata/episodes?*',async route=>{previewQueries.push(route.request().url());await route.fulfill({json:{data:[{id:991,seriesId:99,season:1,number:1,title:'自有单集第一集',overview:'单集简介样本',airDate:'2026-01-01',runtimeMinutes:25},{id:993,seriesId:99,season:1,number:3,title:'自有单集第三集',overview:'第三集简介样本'}]}})})
 await page.route(`**/api/v1/media/assets/${asset}/identity`,async route=>{writes++;await route.fulfill({json:{data:{corrected:true}}})})
 await fixtureLogin(page);await navigate(page,'云盘索引')
 await page.getByRole('tab',{name:'已入库资产',exact:true}).click();await page.getByRole('row').filter({hasText:'自有剧集'}).getByRole('button',{name:'修正身份 / 季集',exact:true}).click()
 const dialog=page.getByRole('dialog')
 await dialog.getByRole('button',{name:'查询TMDB'}).click()
 await dialog.getByRole('row').filter({hasText:'自有剧集'}).click()
 await dialog.getByLabel('修正实际覆盖集',{exact:true}).fill('1,3')
 await dialog.getByRole('button',{name:'预览单集信息',exact:true}).click()
 await expect(dialog.getByText('S01E01 · 自有单集第一集',{exact:true})).toBeVisible()
 await expect(dialog.getByText('S01E03 · 自有单集第三集',{exact:true})).toBeVisible()
 await expect(dialog.getByText('播出日期未知 · 时长未知',{exact:true})).toBeVisible()
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
 await page.getByRole('tab',{name:'输出恢复与归档',exact:true}).click();await expect(page.getByRole('heading',{name:'旧输出归档',exact:false})).toBeVisible()
 const row=page.getByRole('row').filter({hasText:'旧输出.strm'})
 // Element Plus hides native selection checkbox: click its visible label.
 await row.locator('.el-checkbox').click()
 await page.getByRole('button',{name:'预览清理所选 1',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'确认物理清理归档',exact:true})
 await expect(dialog).toBeVisible();expect(confirmations).toHaveLength(0)
 const apply=dialog.getByRole('button',{name:'确认物理清理',exact:true});await expect(apply).toBeDisabled()
 await dialog.getByRole('button',{name:'返回检查',exact:true}).click();expect(confirmations).toHaveLength(0)
 await page.getByRole('button',{name:'预览清理所选 1',exact:true}).click()
 await dialog.getByLabel('输入清理归档确认',{exact:true}).fill('清理归档')
 await expect(apply).toBeEnabled()
 await dialog.screenshot({path:resolve(screenshots,'archive-cleanup-confirmation.png'),animations:'disabled'})
 await page.setViewportSize({width:375,height:812});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
 await expect(apply).toBeInViewport()
 await dialog.screenshot({path:resolve(screenshots,'archive-cleanup-confirmation-mobile.png'),animations:'disabled'})
 await apply.click()
 await expect.poll(()=>confirmations.length).toBe(1)
 expect(confirmations[0]?.archiveIds).toEqual([archive]);expect(confirmations[0]?.planDigest).toBe('b'.repeat(64));expect(confirmations[0]?.requestId).toMatch(/^[a-f0-9-]{36}$/)
 await expect(page.getByText('已清理1项明确批准的过期归档；未操作原媒体',{exact:true})).toBeVisible()
 await expect(row.getByRole('cell',{name:/已清理（审计保留）/})).toBeVisible()
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
 await page.getByRole('tab',{name:'输出恢复与归档',exact:true}).click();await page.getByRole('row').filter({hasText:'待核对.strm'}).locator('.el-checkbox').click()
 await page.getByRole('button',{name:'预览清理所选 1',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'确认物理清理归档',exact:true})
 await dialog.getByLabel('输入清理归档确认',{exact:true}).fill('清理归档')
 const apply=dialog.getByRole('button',{name:'确认物理清理',exact:true})
 await apply.click()
 await expect(page.getByText('清理结果不完整，请保留本次请求ID核对已有请求结果',{exact:true})).toBeVisible()
 await expect(dialog).toBeVisible();expect(confirmations).toHaveLength(1)
 await dialog.getByRole('button',{name:'核对已有请求结果',exact:true}).click()
 await expect(page.getByText('尚未确认批准记录，可保留同一请求ID再确认，不换计划重复提交',{exact:true})).toBeVisible()
 await expect(dialog).toBeVisible();expect(confirmations).toHaveLength(1)
 // A second submit is a new explicit click, never a background retry or a new ID.
 await apply.click()
 await expect.poll(()=>confirmations.length).toBe(2)
 expect(confirmations[1]?.requestId).toBe(confirmations[0]?.requestId)
 expect(confirmations[1]?.planDigest).toBe(confirmations[0]?.planDigest)
 await expect(page.getByText('0项已完成，其余已批准或冲突，请核对记录后明确重试',{exact:true})).toBeVisible()
 await expect(page.getByText('已清理1项明确批准的过期归档；未操作原媒体',{exact:true})).toHaveCount(0)
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
 await page.getByRole('tab',{name:'已入库资产',exact:true}).click();await page.getByRole('row').filter({hasText:'自有非连续合集'}).getByRole('button',{name:'修正身份 / 季集',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'修正媒体身份与季集',exact:true})
 await dialog.getByRole('button',{name:'查询TMDB'}).click();await dialog.getByRole('row').filter({hasText:'自有剧集'}).click()
 await dialog.getByLabel('修正实际覆盖集',{exact:true}).fill('1,3')
 await expect(dialog.getByText(/播放仍读取完整合集，不切片、不按单集跳转/)).toBeVisible()
 await dialog.getByText('同时迁移Movies/TV输出目录（先预览）',{exact:true}).click()
 await dialog.getByRole('button',{name:'预览输出迁移',exact:true}).click()
 const confirm=page.getByRole('dialog',{name:'确认输出迁移',exact:true})
 await expect(confirm.getByText(/每个条目都播放同一个完整源文件/)).toBeVisible()
 await expect(confirm.getByText('TV/自有剧集/Season 01/自有剧集 - S01E01.strm',{exact:true})).toBeVisible()
 await expect(confirm.getByText('TV/自有剧集/Season 01/自有剧集 - S01E03.strm',{exact:true})).toBeVisible()
 expect(applied).toHaveLength(0)
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
 const row=page.getByRole('row').filter({hasText:'Owned.S01E01E03.mkv'})
 await expect(row.getByRole('button',{name:'恢复',exact:true})).toHaveCount(0)
 await row.getByRole('button',{name:'核对入库恢复',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'核对并恢复已有入库',exact:true})
 const resume=dialog.getByRole('button',{name:'明确恢复入库（不重新上传）',exact:true})
 await expect(resume).toBeDisabled();expect(approvals).toHaveLength(0)
 const link=dialog.getByRole('link',{name:'前往云盘索引核对该资产、恢复已有输出并明确预览迁移',exact:true})
 await expect(link).toHaveAttribute('href',`/cloudStorage?assetId=${asset}`)
 await dialog.getByRole('button',{name:'关闭',exact:true}).click();expect(approvals).toHaveLength(0)
 migrated=true;await row.getByRole('button',{name:'核对入库恢复',exact:true}).click()
 await expect(resume).toBeEnabled()
 await dialog.screenshot({path:resolve(screenshots,'output-import-recovery-preview.png'),animations:'disabled'})
 await page.setViewportSize({width:375,height:812});await expect(resume).toBeInViewport({ratio:1})
 await dialog.screenshot({path:resolve(screenshots,'output-import-recovery-preview-mobile.png'),animations:'disabled'})
 await resume.click();await expect.poll(()=>approvals.length).toBe(1)
 expect(approvals[0]).toEqual({controlRevision:1,planDigest:'e'.repeat(64)})
 await expect(page.getByText('入库恢复已明确批准；不重新上传、不改播放链接，请核对后续状态',{exact:true})).toBeVisible()
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
 await dialog.getByRole('cell',{name:'Owned Movie',exact:true}).first().click()
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
 await expect(page.getByText('部分结果',{exact:true})).toHaveCount(3)
 await expect(page.getByText('没有显示不等于站点没有资源。',{exact:false})).toBeVisible()
 await expect(page.getByRole('button',{name:'确认并下载',exact:true})).toBeVisible()
 mkdirSync(screenshots,{recursive:true})
 await page.screenshot({path:resolve(screenshots,'search-provider-partial-desktop.png'),fullPage:true})
 await page.emulateMedia({reducedMotion:'reduce'})
 await page.setViewportSize({width:375,height:812})
 await expect(page.getByRole('button',{name:'打开导航',exact:true})).toBeVisible()
 await expect.poll(()=>page.locator('.sidebar').evaluate(el=>el.getBoundingClientRect().right)).toBeLessThanOrEqual(1)
 await expect(page.getByText('没有显示不等于站点没有资源。',{exact:false})).toBeVisible()
 await page.screenshot({path:resolve(screenshots,'search-provider-partial-mobile.png'),fullPage:true})
})

test('PT account form uses template identity, UA, echo and disallows duplicates (fixture)',async({page})=>{
 await page.route('**/api/v1/pt/templates',route=>route.fulfill({json:{data:{templates:[{id:'hdsky',name:'HDSKY',domain:'https://hdsky.me'},{id:'ourbits',name:'OURBITS',domain:'https://ourbits.club'}].map(x=>({...x,loaded:true,fields:[{key:'cookie',label:'Cookie',required:true},{key:'userAgent',label:'User-Agent',required:true}]}))}}}))
 const accounts=[{id:'owned-pt',definitionId:'hdsky',name:'HDSKY',revision:1,settings:{userAgent:'Owned-UA',rssState:'ready'},values:{cookie:'owned-cookie-e2e',passkey:'owned-mobile-passkey-'.repeat(8),rssUrl:'https://owned.invalid/rss?passkey=owned'}}]
 let saved:any
 await page.route('**/api/v1/pt/accounts',async route=>{if(route.request().method()==='POST'){saved=route.request().postDataJSON();await route.fulfill({json:{data:{saved:true,rssState:'ready',rssMessage:'RSS已自动获取'}}})}else await route.fulfill({json:{data:accounts}})})
 await page.route('**/api/v1/sites/metrics',route=>route.fulfill({json:{data:[{accountId:'owned-pt',name:'HDSKY',enabled:true,metrics:null,state:'not_synced',stale:false}]}}))
 await fixtureLogin(page);await page.goto('/sites');await expect(page.getByText('已保存连接',{exact:true})).toHaveCount(0)
 await page.getByRole('button',{name:'编辑站点',exact:true}).click();const edit=page.getByRole('dialog',{name:'编辑站点',exact:true});await expect(edit.getByLabel('Cookie（必填）',{exact:true})).toHaveValue('owned-cookie-e2e');await expect(edit.getByLabel('User-Agent（必填）',{exact:true})).toHaveValue('Owned-UA');await expect(edit.getByLabel('RSS订阅URL',{exact:true})).toHaveValue('https://owned.invalid/rss?passkey=owned');await expect(edit.getByLabel('Passkey',{exact:true})).toHaveValue('owned-mobile-passkey-'.repeat(8));await page.setViewportSize({width:390,height:844});await expect(edit.locator('pre')).toHaveCount(0);expect(await edit.evaluate(el=>el.scrollWidth<=el.clientWidth+1)).toBe(true);await edit.getByLabel('Passkey',{exact:true}).scrollIntoViewIfNeeded();await page.screenshot({path:resolve(screenshots,'pt-passkey-mobile.png'),animations:'disabled'});await edit.getByRole('button',{name:'关闭此对话框',exact:true}).click();await expect(edit).not.toBeVisible()
 await page.getByRole('button',{name:'添加站点',exact:true}).click();const dialog=page.getByRole('dialog',{name:'添加站点',exact:true});await expect(dialog).toBeVisible();await expect(dialog.getByRole('combobox')).toBeEnabled();await dialog.getByRole('combobox').press('ArrowDown');await expect(dialog.getByRole('combobox')).toHaveAttribute('aria-expanded','true')
 await expect(page.getByRole('option',{name:'HDSKY',exact:true})).toHaveAttribute('aria-disabled','true');await page.getByRole('option',{name:'OURBITS',exact:true}).click()
 await expect(dialog.getByLabel('站点名称',{exact:true})).toHaveValue('OURBITS');await expect(dialog.getByLabel('站点访问地址',{exact:true})).toHaveValue('https://ourbits.club')
 for(const label of ['连接名称','启用为主连接','API Key','Passkey'])await expect(dialog.getByText(label,{exact:true})).toHaveCount(0)
 await dialog.getByLabel('Cookie（必填）',{exact:true}).fill('owned-new-cookie');await dialog.getByLabel('User-Agent（必填）',{exact:true}).fill('Owned-new-UA')
 await dialog.getByRole('button',{name:'保存站点',exact:true}).click();expect(saved.definitionId).toBe('ourbits');expect(saved.values).toEqual({cookie:'owned-new-cookie',userAgent:'Owned-new-UA'})
 await page.screenshot({path:resolve(screenshots,'pt-account-template-form.png'),fullPage:true,animations:'disabled'})
})

test('separate 115 setup saves without tokens and secret echoes are explicit (isolated)',async({page})=>{
 await fixtureLogin(page)
 await page.goto('/cloud-connections');await page.getByRole('button',{name:'115 授权',exact:true}).click()
 await expect(page.getByRole('button',{name:'115 授权',exact:true})).toHaveAttribute('aria-pressed','true')
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
 const draftRow=page.getByRole('row').filter({hasText:name});await expect(draftRow.getByRole('button',{name:'115扫码授权',exact:true})).toBeVisible()
 await page.goto('/mediaLibrary');await page.getByRole('button',{name:'Emby 配置',exact:true}).click()
 await page.getByRole('button',{name:'添加连接',exact:true}).click()
 const embyName='Emby echo fixture '+Date.now(),value='PRIVATE-EMBY-ECHO-FIXTURE'
 await dialog.getByLabel('连接名称',{exact:true}).fill(embyName)
 await dialog.getByLabel('Emby 服务地址',{exact:true}).fill(process.env.E2E_EMBY_ENDPOINT!)
 await dialog.getByLabel('媒体库刷新 API Key',{exact:true}).fill(value)
 await dialog.getByRole('button',{name:'加密保存',exact:true}).click()
 const row=page.getByRole('row').filter({hasText:embyName});await row.getByRole('button',{name:'编辑',exact:true}).click()
 const edit=page.getByRole('dialog',{name:'编辑连接',exact:true}),field=edit.getByLabel('媒体库刷新 API Key',{exact:true})
 await expect(field).toHaveValue(value);await expect(field).toHaveAttribute('type','text')
 await edit.getByRole('button',{name:'隐藏媒体库刷新 API Key',exact:true}).click()
 await expect(field).toHaveAttribute('type','password')
 await edit.getByRole('button',{name:'显示媒体库刷新 API Key',exact:true}).click()
 await expect(field).toHaveAttribute('type','text')
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
 // Four real isolated file operations plus desktop/mobile screenshots; retain
 // explicit dialog-close checks instead of force-clicking through transitions.
 test.setTimeout(60000)
 const directory=process.env.E2E_FILE_ROOT;if(!directory)throw new Error('Owned E2E file root required')
 await fixtureLogin(page)
 await page.goto('/files')
 const old=page.getByRole('row').filter({hasText:'Owned Film.mkv'})
 await expect(old).toBeVisible();await old.getByRole('button',{name:'影视整理',exact:true}).click()
 const edit=page.getByRole('dialog',{name:'影视分类整理',exact:true})
 await test.step('等待0day识别完成并选择电影',async()=>{await expect(edit.getByRole('button',{name:'预览变更',exact:true})).toBeEnabled();await edit.getByText('电影',{exact:true}).click();await expect(edit.getByRole('radio',{name:'电影',exact:true})).toBeChecked()})
 await edit.getByLabel('影视标题',{exact:true}).fill('Fixture Film')
 await edit.getByLabel('年份',{exact:true}).fill('2026')
 await test.step('选择自定义文件名',async()=>{await edit.getByText('自定义文件名',{exact:true}).click();await expect(edit.getByRole('radio',{name:'自定义文件名',exact:true})).toBeChecked()})
 await edit.getByRole('textbox',{name:'自定义文件名',exact:true}).fill('自定义片名.mkv')
 await test.step('获取整理预览',async()=>{const response=page.waitForResponse(r=>r.url().endsWith('/api/v1/files/preview'));await edit.getByRole('button',{name:'预览变更',exact:true}).click();const result=await response;expect(result.status()).toBe(200)})
 const plan=page.getByRole('dialog',{name:'确认文件变更',exact:true})
 await expect(plan.locator('.file-plan li')).toHaveCount(2)
 await expect(plan.getByText('电影/Fixture Film (2026)/自定义片名.mkv',{exact:true})).toBeVisible()
 await expect(plan.getByText('电影/Fixture Film (2026)/自定义片名.zh-CN.srt',{exact:true})).toBeVisible()
 await plan.screenshot({path:resolve(screenshots,'file-organize-preview-desktop.png'),animations:'disabled'})
 await page.setViewportSize({width:375,height:812})
 await expect(plan.getByRole('button',{name:'确认执行文件变更',exact:true})).toBeInViewport({ratio:1})
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
 await plan.screenshot({path:resolve(screenshots,'file-organize-preview-mobile.png'),animations:'disabled'})
 await plan.getByRole('button',{name:'确认执行文件变更',exact:true}).click()
 await expect(plan).not.toBeVisible()
 await page.setViewportSize({width:1440,height:1000})
 await page.getByRole('button',{name:'电影',exact:true}).click()
 await page.getByRole('button',{name:'Fixture Film (2026)',exact:true}).click()
 const video=page.getByRole('row').filter({hasText:'自定义片名.mkv'})
 await expect(video).toBeVisible();await expect(page.getByRole('row').filter({hasText:'自定义片名.zh-CN.srt'})).toBeVisible()
 await page.screenshot({path:resolve(screenshots,'files-managed-desktop.png'),animations:'disabled'})
 await page.setViewportSize({width:375,height:812});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
 await page.screenshot({path:resolve(screenshots,'files-managed-mobile.png'),animations:'disabled'})
 const mobileRow=page.getByLabel('移动端文件列表',{exact:true}).getByRole('listitem').filter({hasText:'自定义片名.mkv'})
 await expect(mobileRow.getByRole('button',{name:'重命名',exact:true})).toBeVisible()
 await mobileRow.getByRole('button',{name:'重命名',exact:true}).click()
 const naming=page.getByRole('dialog',{name:'重命名',exact:true})
 await naming.getByText('命名模板',{exact:true}).click()
 await expect(naming.getByRole('radio',{name:'命名模板',exact:true})).toBeChecked()
 await naming.getByRole('textbox',{name:'命名模板',exact:true}).fill('{original} - {resolution}{ext}')
 await naming.getByRole('textbox',{name:'分辨率（模板需要时）',exact:true}).fill('1080p')
 await naming.getByRole('button',{name:'预览变更',exact:true}).click()
 await expect(plan.locator('.file-plan li')).toHaveCount(2)
 await expect(plan.getByText('电影/Fixture Film (2026)/自定义片名 - 1080p.mkv',{exact:true})).toBeVisible()
 await plan.getByRole('button',{name:'取消，不执行',exact:true}).click()
 await expect(plan).not.toBeVisible()
 await page.setViewportSize({width:1440,height:1000})
 await video.getByRole('button',{name:'删除（可恢复）',exact:true}).click()
 await page.getByRole('dialog',{name:'删除（可恢复）',exact:true}).getByRole('button',{name:'预览变更',exact:true}).click()
 await plan.getByRole('button',{name:'确认执行文件变更',exact:true}).click()
 await expect(video).toHaveCount(0);await expect(plan).not.toBeVisible()
 await page.getByRole('button',{name:'查看回收目录',exact:true}).click()
 const folder=page.locator('.file-name').filter({hasText:/^[0-9a-f-]{36}$/}).first();await folder.click()
 const recycled=page.getByRole('row').filter({hasText:'自定义片名.mkv'});await expect(recycled).toBeVisible()
 await recycled.getByRole('button',{name:'移动',exact:true}).click()
 await page.getByRole('dialog',{name:'移动文件 / 目录',exact:true}).getByLabel('目标目录',{exact:true}).fill('.')
 await page.getByRole('dialog',{name:'移动文件 / 目录',exact:true}).getByRole('button',{name:'预览变更',exact:true}).click()
 await plan.getByRole('button',{name:'确认执行文件变更',exact:true}).click()
 await page.getByRole('button',{name:'根目录',exact:true}).click()
 await expect(page.getByRole('row').filter({hasText:'自定义片名.mkv'})).toBeVisible()
})

test('media catalog browses managed and Emby content without exposing playback credentials (fixture)',async({page})=>{
 await fixtureLogin(page)
 await page.route('**/api/v1/media/library?**',route=>{const query=new URL(route.request().url()).searchParams;const source=query.get('source')??'managed';return route.fulfill({status:200,contentType:'application/json',body:JSON.stringify({data:{items:[{id:'owned-catalog',source,title:'自有影片验证样本',year:'2026',mediaType:'movie',overview:'用于界面验收的明确测试内容，不是实际影片入库记录。',poster:'',state:'present',filePath:'/115/Owned/Owned.mkv'}],total:1,page:1,pageSize:24,source}})})})
 await page.goto('/mediaLibrary')
 await expect(page.getByRole('heading',{name:'媒体库',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'全部媒体',exact:true}).click()
 await page.getByRole('button',{name:/自有影片验证样本/}).click()
 const detail=page.getByRole('dialog',{name:'自有影片验证样本',exact:true});await expect(detail.getByText('用于界面验收的明确测试内容，不是实际影片入库记录。',{exact:true})).toBeVisible()
 await detail.getByRole('button',{name:'关闭',exact:true}).click()
 await expect(detail).not.toBeVisible()
 await page.screenshot({path:resolve(screenshots,'media-catalog-desktop.png'),animations:'disabled'})
 await page.setViewportSize({width:375,height:812});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
 await page.screenshot({path:resolve(screenshots,'media-catalog-mobile.png'),animations:'disabled'})
 await page.getByText('Emby 媒体库',{exact:true}).click()
 await expect(page.getByRole('radio',{name:'Emby 媒体库',exact:true})).toBeChecked()
 await expect(page.getByText('Emby 媒体库 · 1 项',{exact:true})).toBeVisible()
})


test('service overview is read-only and settings own naming and TMDB',async({page})=>{
 await login(page);await page.goto('/integrations');await expect(page.getByRole('heading',{name:'服务连接总览',exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'添加连接',exact:true})).toHaveCount(0)
 await page.goto('/settings?tab=library');await expect(page.getByLabel('电影命名模板',{exact:true})).toHaveValue('{title} ({year})/{title} ({year}) - {videoFormat}');await expect(page.getByLabel('剧集命名模板',{exact:true})).toBeVisible();await page.getByRole('navigation',{name:'系统设置分类'}).getByRole('button',{name:'TMDB',exact:true}).click();await expect(page.getByRole('heading',{name:'TMDB配置',exact:true})).toBeVisible();await page.getByRole('navigation',{name:'系统设置分类'}).getByRole('button',{name:'搜索与订阅',exact:true}).click();await expect(page.getByRole('button',{name:'生成 / 更换恢复码',exact:true})).toHaveCount(0)
 await page.locator('#pt-cache-seconds').fill('600');await page.getByRole('button',{name:'保存系统设置',exact:true}).click();await expect(page.getByText('当前分组已保存，不会自动重命名现有文件或更改其他分组。',{exact:true})).toBeVisible()
 await page.screenshot({path:resolve(screenshots,'workbench-settings-desktop.png'),animations:'disabled',fullPage:true});await page.setViewportSize({width:375,height:812});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,'workbench-settings-mobile.png'),animations:'disabled',fullPage:false})
})

test('log viewer has level, sizes and confirmed cleanup; data tools are formatted read-only',async({page})=>{
 await login(page);await page.goto('/logs');await expect(page.getByRole('heading',{name:'程序日志',exact:true})).toBeVisible();await page.getByRole('button',{name:'日志设置',exact:true}).click();await expect(page.getByRole('heading',{name:'日志大小',exact:true})).toBeVisible();await page.getByRole('dialog',{name:'日志设置',exact:true}).getByRole('button',{name:'关闭此对话框',exact:true}).click();await expect(page.locator('.log-entry').first()).toBeVisible();await page.getByRole('button',{name:'清理日志',exact:true}).click();await page.getByRole('button',{name:'取消',exact:true}).click();await expect(page.locator('.log-entry').first()).toBeVisible();await page.screenshot({path:resolve(screenshots,'workbench-logs-desktop.png'),animations:'disabled'})
 await page.goto('/data');await expect(page.getByRole('heading',{name:'数据库浏览',exact:true})).toBeVisible();await expect(page.locator('.data-row').first()).toBeVisible();await page.locator('.data-row').first().click();await expect(page.getByRole('dialog',{name:'只读格式化数据'})).toBeVisible();await page.getByRole('button',{name:'关闭',exact:true}).click();await page.screenshot({path:resolve(screenshots,'workbench-data-desktop.png'),animations:'disabled'});await page.setViewportSize({width:375,height:812});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,'workbench-data-mobile.png'),animations:'disabled'})
})

test('Emby home follows real category/watch/latest structure without exposing keys (fixture)',async({page})=>{
 await fixtureLogin(page);const sample={id:'owned1',title:'自有分类验证样本',type:'CollectionFolder',image:'',progress:30}
 await page.route('**/api/v1/media/library/emby/home?*',route=>route.fulfill({json:{data:{users:[{id:'u1',name:'自有用户'}],userId:'u1',libraries:[sample],playing:[],resume:[{...sample,type:'Movie',title:'自有继续观看样本'}],recent:[],latest:[{...sample,type:'Episode',title:'自有最新入库样本',subtitle:'第1集'}]}}}))
 await page.goto('/mediaLibrary');for(const name of ['我的媒体库','媒体库分类','正在播放','继续观看','近期观看','最新入库'])await expect(page.getByRole('heading',{name:new RegExp('^'+name+'(?: Emby)?$')})).toBeVisible();await page.screenshot({path:resolve(screenshots,'workbench-emby-home-desktop.png'),animations:'disabled',fullPage:true});await page.setViewportSize({width:375,height:812});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,'workbench-emby-home-mobile.png'),animations:'disabled',fullPage:false})
})

test('TMDB and site-direct search have distinct cache controls (fixture)',async({page})=>{
 await fixtureLogin(page);let calls=0
 await page.route('**/api/v1/search/media?*',route=>{calls++;return route.fulfill({json:{data:{items:[{id:1,mediaType:'movie',title:'自有搜索样本',originalTitle:'Owned Movie',year:'2026',overview:'隔离的搜索界面样本',posterPath:''}],cached:calls>1,cacheSeconds:60}}})})
 await page.goto('/search');await page.getByLabel('TMDB媒体名称',{exact:true}).fill('Owned Movie');await page.getByRole('button',{name:'搜索媒体',exact:true}).click();await expect(page.getByRole('heading',{name:/自有搜索样本/})).toBeVisible();await page.getByRole('button',{name:'搜索媒体',exact:true}).click();await expect(page.getByText(/本次使用TMDB缓存/)).toBeVisible();await expect(page.getByRole('button',{name:'使用标题到站点搜索',exact:true})).toHaveCount(0);await expect(page.getByRole('button',{name:'自有搜索样本',exact:true})).toBeVisible();await page.getByText('站点直搜',{exact:true}).click();await expect(page.getByRole('button',{name:'忽略站点缓存重新搜索',exact:true})).toBeVisible();await expect(page.locator('.account-selection')).toHaveCount(0);await page.screenshot({path:resolve(screenshots,'workbench-search-desktop.png'),animations:'disabled'})
})

test('automatic roots, 0day hints, history and safe plugin manifests',async({page})=>{
 await fixtureLogin(page);await page.goto('/files');await expect(page.getByText(/本地目录/).first()).toBeVisible();const parsed=await page.request.post('/api/v1/media/recognize-name',{headers:{'X-Requested-With':'media-tool'},data:{name:'Owned.Show.2026.S01E03.1080p.WEB-DL.x265-TEAM.mkv'}});expect(parsed.status()).toBe(200);expect((await parsed.json()).data.hint).toMatchObject({title:'Owned Show',year:'2026',mediaType:'series',season:1,episode:3})
 await page.goto('/organize-history');await expect(page.getByRole('heading',{name:'文件与入库历史',exact:true})).toBeVisible();await page.goto('/plugins');await expect(page.getByRole('heading',{name:'内置目录',exact:true})).toBeVisible();await expect(page.getByRole('heading',{name:'0day 文件名识别',exact:true})).toHaveCount(0);await expect(page.getByRole('heading',{name:'电影与剧集命名模板',exact:true})).toHaveCount(0);await expect(page.getByText('兼容入口：导入扩展 Manifest',{exact:true})).toHaveCount(0)
 const rejected=await page.request.post('/api/v1/plugins',{headers:{'X-Requested-With':'media-tool'},data:{manifest:{id:'owned-plugin',name:'Owned',version:'1',contractVersion:1,runtime:'webhook',endpoint:'https://owned.example',capabilities:['media.organized']},enabled:false,consent:false,revision:0}});expect([404,405]).toContain(rejected.status());await page.screenshot({path:resolve(screenshots,'workbench-plugins-desktop.png'),animations:'disabled'})
})

test('temporary-password sessions are routed to compulsory reset without web recovery (fixture)',async({page})=>{
 await page.route('**/api/v1/auth/session',route=>route.fulfill({json:{data:{authenticated:true,account:{id:'owned-reset-fixture',username:'owned_owner',mustChangePassword:true}}}}))
 await page.route('**/api/v1/auth/password',route=>route.fulfill({json:{data:{changed:true,loginRequired:true}}}))
 await page.goto('/logs');await expect(page).toHaveURL('http://localhost:18084/reset-password');await expect(page.getByRole('heading',{name:'必须设置新密码',exact:true})).toBeVisible();await expect(page.getByRole('link',{name:'程序日志',exact:true})).toHaveCount(0)
 await page.getByLabel('当前临时密码',{exact:true}).fill('Temporary-2026');await page.getByLabel('新密码',{exact:true}).fill('Replacement-2026');await page.getByLabel('确认新密码',{exact:true}).fill('Replacement-2026');await page.screenshot({path:resolve(screenshots,'mandatory-reset-desktop.png'),animations:'disabled',mask:[page.locator('input[type=password]')]});await page.setViewportSize({width:375,height:812});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,'mandatory-reset-mobile.png'),animations:'disabled',mask:[page.locator('input[type=password]')]});await page.getByRole('button',{name:'设置新密码并重新登录',exact:true}).click();await expect(page.getByRole('heading',{name:'进入管理工作台',exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'忘记密码？使用恢复码',exact:true})).toHaveCount(0)
})


test('grouped navigation, quick jump and keyboard-only return (UI)',async({page})=>{
 await fixtureLogin(page)
 const navigation=page.getByRole('navigation',{name:'功能导航'})
 for(const name of ['媒体浏览','资源获取','文件与入库','系统运维'])await expect(navigation.getByRole('heading',{name,exact:true})).toBeVisible()
 await expect(page.getByRole('heading',{name:'模块建设进度',exact:true})).toHaveCount(0)
 await expect(page).toHaveTitle('总览 · Media Tool')
 await page.getByRole('button',{name:'快速前往',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'快速前往',exact:true}),input=dialog.getByRole('combobox',{name:'搜索功能入口'})
 await expect(input).toBeFocused();await input.fill('Redis');await expect(dialog.getByRole('option')).toHaveCount(1)
 await input.press('Enter');await expect(page.getByRole('heading',{name:'数据库浏览',exact:true})).toBeVisible();await expect(dialog).toBeHidden()
 await page.keyboard.press('Control+k');await expect(input).toBeFocused();await input.fill('not-a-real-feature');await expect(dialog.getByText('没有匹配的功能，请尝试其他关键词。')).toBeVisible()
 await input.press('Enter');await expect(dialog).toBeVisible();await page.keyboard.press('Escape');await expect(dialog).toBeHidden()
 await page.getByRole('button',{name:'快速前往',exact:true}).focus();await page.keyboard.press('Enter');await expect(input).toBeFocused()
 await input.fill('命名模板');await input.press('ArrowDown');await input.press('ArrowUp');await input.press('Enter');await expect(page).toHaveURL(/\/settings$/)
 await page.getByRole('navigation',{name:'系统设置分类'}).getByRole('button',{name:'命名与刮削',exact:true}).click()
 await expect(page.getByLabel('电影命名模板',{exact:true})).toBeVisible()
})

test('mobile navigation traps focus, closes with Escape and restores its trigger (UI)',async({page})=>{
 await page.setViewportSize({width:375,height:812});await fixtureLogin(page)
 const trigger=page.getByRole('button',{name:'打开导航',exact:true});await trigger.click()
 const drawer=page.getByRole('dialog',{name:'主导航',exact:true});await expect(drawer).toBeVisible();await expect(trigger).toHaveAttribute('aria-expanded','true')
 for(let i=0;i<24;i++){await page.keyboard.press('Tab');expect(await drawer.evaluate(el=>el.contains(document.activeElement))).toBe(true)}
 await page.screenshot({path:resolve(screenshots,'ui-navigation-mobile.png'),animations:'disabled'})
 await page.keyboard.press('Escape');await expect(drawer).toBeHidden();await expect(trigger).toBeFocused()
 await page.keyboard.press('Control+k');await expect(page.getByRole('dialog',{name:'快速前往',exact:true})).toBeVisible();await page.keyboard.press('Escape')
 await trigger.click();await drawer.getByRole('button',{name:'系统运维',exact:true}).click();await drawer.getByRole('link',{name:'程序日志',exact:true}).click();await expect(page.getByRole('heading',{name:'程序日志',exact:true})).toBeVisible();await expect(drawer).toBeHidden()
})

test('TMDB errors and empty results are visible in media search (UI fixture)',async({page})=>{
 await fixtureLogin(page);let fail=true
 await page.route('**/api/v1/search/media?*',route=>route.fulfill(fail?{status:503,json:{error:{code:'provider_unavailable',message:'自有测试：TMDB暂时不可用'}}}:{json:{data:{items:[],cached:false}}}))
 await page.goto('/search');await page.getByLabel('TMDB媒体名称',{exact:true}).fill('Empty fixture');await page.getByRole('button',{name:'搜索媒体',exact:true}).click()
 await expect(page.getByText('自有测试：TMDB暂时不可用',{exact:true})).toBeVisible()
 fail=false;await page.getByRole('button',{name:'搜索媒体',exact:true}).click();await expect(page.getByText('没有找到匹配作品，请换个名称或调整年份。',{exact:true})).toBeVisible()
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
 await fixtureLogin(page);let homeCalls=0
 await page.route('**/api/v1/media/library/emby/home?*',route=>{homeCalls++;return route.fulfill({json:{data:{users:[{id:'owned-user',name:'自有测试用户'}],userId:'owned-user',libraries:[],playing:[],resume:[],recent:[],latest:[]}}})})
 await page.goto('/mediaLibrary');await expect.poll(()=>homeCalls).toBe(1)
 await page.getByRole('button',{name:'刷新媒体库',exact:true}).click();await expect.poll(()=>homeCalls).toBe(2)
 await page.goto('/mediaLibrary?connectionKind=emby');await expect(page.getByRole('heading',{name:'Emby媒体库连接',exact:true})).toBeVisible()
 await expect(page.getByRole('button',{name:'Emby 配置',exact:true})).toHaveAttribute('aria-pressed','true')
 await expect(page.getByRole('heading',{name:/^我的媒体库/})).toHaveCount(0)
})


test('sidebar groups start collapsed and support keyboard disclosure without auto-opening routes',async({page})=>{
 await fixtureLogin(page);const nav=page.getByRole('navigation',{name:'功能导航'})
 for(const name of ['媒体浏览','资源获取','文件与入库','系统运维'])await expect(nav.getByRole('button',{name,exact:true})).toHaveAttribute('aria-expanded','false')
 await expect(nav.getByRole('link')).toHaveCount(0)
 const browse=nav.getByRole('button',{name:'媒体浏览',exact:true});await expect(browse).toHaveClass(/has-current/)
 await browse.focus();await page.keyboard.press('Enter');await expect(browse).toHaveAttribute('aria-expanded','true');await expect(nav.getByRole('link',{name:'媒体库',exact:true})).toBeVisible()
 await page.keyboard.press('Tab');await expect(nav.getByRole('link',{name:'总览',exact:true})).toBeFocused()
 await browse.focus();await page.keyboard.press('Space');await expect(browse).toHaveAttribute('aria-expanded','false');await expect(nav.getByRole('link')).toHaveCount(0)
 await page.keyboard.press('Tab');await expect(nav.getByRole('button',{name:'资源获取',exact:true})).toBeFocused()
 await navigate(page,'程序日志');await expect(page.getByRole('heading',{name:'程序日志',exact:true})).toBeVisible()
 const operate=nav.getByRole('button',{name:'系统运维',exact:true});await expect(operate).toHaveAttribute('aria-expanded','true');await operate.click()
 await expect(operate).toHaveClass(/has-current/);await expect(nav.getByRole('link')).toHaveCount(0)
 await page.reload();await expect(page.getByRole('heading',{name:'程序日志',exact:true})).toBeVisible()
 for(const name of ['媒体浏览','资源获取','文件与入库','系统运维'])await expect(nav.getByRole('button',{name,exact:true})).toHaveAttribute('aria-expanded','false')
 await page.screenshot({path:resolve(screenshots,'navigation-collapsed-desktop.png'),animations:'disabled'})
})

test('desktop and mobile share disclosure state with unique controlled panels',async({page})=>{
 await fixtureLogin(page);const desktop=page.getByRole('complementary',{name:'主导航'})
 await desktop.getByRole('button',{name:'媒体浏览',exact:true}).click();await desktop.getByRole('button',{name:'文件与入库',exact:true}).click()
 await page.setViewportSize({width:375,height:812});await page.getByRole('button',{name:'打开导航',exact:true}).click()
 const drawer=page.getByRole('dialog',{name:'主导航',exact:true});await expect(drawer.getByRole('button',{name:'媒体浏览',exact:true})).toHaveAttribute('aria-expanded','true')
 const organize=drawer.getByRole('button',{name:'文件与入库',exact:true});await expect(organize).toHaveAttribute('aria-expanded','true')
 const target=await organize.getAttribute('aria-controls');expect(target).toBeTruthy();expect(await page.locator(`[id="${target}"]`).count()).toBe(1)
 await organize.click();await expect(drawer.getByRole('link',{name:'文件管理',exact:true})).toHaveCount(0)
 await page.screenshot({path:resolve(screenshots,'navigation-dropdown-mobile.png'),animations:'disabled'})
 await page.keyboard.press('Escape');await page.setViewportSize({width:1440,height:1000})
 await expect(desktop.getByRole('button',{name:'文件与入库',exact:true})).toHaveAttribute('aria-expanded','false')
 await expect(desktop.getByRole('button',{name:'媒体浏览',exact:true})).toHaveAttribute('aria-expanded','true')
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
})


test('console logs use server filtering, bounded pagination, compact mode and explicit details (fixture)',async({page})=>{
 await fixtureLogin(page);await page.addInitScript(()=>Object.defineProperty(navigator,'clipboard',{value:undefined,configurable:true}));const queries:Array<Record<string,string>>=[]
 await page.route('**/api/v1/logs/summary',route=>route.fulfill({json:{data:{totalBytes:4096,limitBytes:52428800,level:'info',files:[{name:'current.jsonl',size:4096,modified:'2026-10-12T07:00:00+08:00'}]}}}))
 await page.route('**/api/v1/logs/query?*',async route=>{
  const params=Object.fromEntries(new URL(route.request().url()).searchParams);queries.push(params)
  const items=params.keyword==='none'?[]:[{time:'2026-10-12T07:41:04.123456789+08:00',level:'INFO',msg:params.page==='2'?'第二页自有日志':'自有控制台样本',module:'search',source:{file:'internal/modules/search/run.go',line:61},requestId:'owned-request',durationMs:26}, {time:'2026-10-12T07:40:53+08:00',level:'ERROR',msg:'自有错误样本',module:'tmdb',error:'[details omitted]'}]
  await route.fulfill({json:{data:{items,total:items.length?201:0,page:Number(params.page),pageSize:Number(params.pageSize),modules:['search','tmdb'],before:'2026-10-12T00:00:00Z',truncated:true,maxFileBytes:2097152,maxRecords:20000}}})
 })
 await page.goto('/logs');await expect(page.getByText('自有控制台样本',{exact:true})).toBeVisible();await expect(page.locator('.log-console')).toHaveCSS('background-color','rgb(21, 27, 35)')
 await page.getByRole('button',{name:'查看第1条日志详情',exact:true}).click({timeout:5000});const logDetail=page.getByRole('dialog',{name:'日志详情',exact:true});await expect(logDetail.locator('.log-details pre')).toContainText('owned-request');await logDetail.getByRole('button',{name:'关闭',exact:true}).click()
 await page.locator('label[for=log-clean-mode]').click({timeout:5000});await expect(page.locator('.log-source')).toHaveCount(0);await expect(page.getByRole('dialog',{name:'日志详情',exact:true})).not.toBeVisible()
 await page.locator('label[for=log-clean-mode]').click({timeout:5000});await page.getByRole('button',{name:'下一页',exact:true}).click({timeout:5000});await expect(page.getByText('第二页自有日志',{exact:true})).toBeVisible();expect(queries.at(-1)).toMatchObject({page:'2',before:'2026-10-12T00:00:00Z'})
 await page.getByLabel('日志关键词',{exact:true}).fill('owned');await expect.poll(()=>queries.at(-1)?.keyword).toBe('owned');expect(queries.at(-1)?.page).toBe('1');expect(queries.at(-1)?.before).toBeUndefined()
 await page.getByRole('combobox',{name:'日志级别筛选'}).focus();await page.keyboard.press('Enter');await page.getByRole('option',{name:'ERROR',exact:true}).click();await expect.poll(()=>queries.at(-1)?.level).toBe('error')
 await page.getByRole('combobox',{name:'日志模块筛选'}).focus();await page.keyboard.press('Enter');await page.getByRole('option',{name:'TMDB元数据 / tmdb',exact:true}).click();await expect.poll(()=>queries.at(-1)?.module).toBe('tmdb')
 await page.getByRole('region',{name:'系统日志控制台',exact:true}).getByPlaceholder('开始时间',{exact:true}).fill('2026-10-12 00:00:00');await page.getByRole('region',{name:'系统日志控制台',exact:true}).getByPlaceholder('结束时间',{exact:true}).fill('2026-10-12 23:59:59');await page.getByRole('region',{name:'系统日志控制台',exact:true}).getByPlaceholder('结束时间',{exact:true}).press('Enter');await page.keyboard.press('Tab');await expect.poll(()=>queries.at(-1)?.from).toBeTruthy();expect(queries.at(-1)?.to).toBeTruthy();await page.keyboard.press('Escape')
 await expect(page.getByText('仅检索每文件末尾 2 MiB、最多 20000 条；不代表全部保留日志。',{exact:true})).toBeVisible()
 await page.getByRole('button',{name:'复制当前页日志',exact:true}).click();const copy=page.getByRole('dialog',{name:'复制原始日志',exact:true});await expect(copy).toBeVisible();await expect(copy.getByLabel('待复制日志')).toHaveValue(/自有控制台样本/);await copy.getByRole('button',{name:'关闭',exact:true}).click();await expect(copy).toBeHidden()
 await page.screenshot({path:resolve(screenshots,'console-log-desktop.png'),animations:'disabled'})
 await page.setViewportSize({width:375,height:812});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,'console-log-mobile.png'),animations:'disabled'})
 await page.getByLabel('日志关键词',{exact:true}).fill('none');await expect(page.getByText('没有符合筛选条件的日志',{exact:true})).toBeVisible()
})

test('log query failures are visible and do not present stale content as current (fixture)',async({page})=>{
 await fixtureLogin(page);let failed=false
 await page.route('**/api/v1/logs/query?*',route=>route.fulfill(failed?{status:503,json:{error:{code:'logs_unavailable',message:'自有日志读取失败'}}}:{json:{data:{items:[{time:'2026-10-12T07:00:00+08:00',level:'INFO',msg:'自有旧快照'}],total:1,modules:['app'],before:'2026-10-12T00:00:00Z',truncated:false,page:1,pageSize:100}}}))
 await page.goto('/logs');await expect(page.getByText('自有旧快照',{exact:true})).toBeVisible();failed=true;await page.getByRole('button',{name:'刷新日志',exact:true}).click()
 await expect(page.getByText('自有日志读取失败',{exact:true})).toBeVisible();await expect(page.getByText('自有旧快照',{exact:true})).toHaveCount(0)
})

test('built-in plugins consent, config, enable, instance logs, uninstall (isolated)',async({page})=>{
 await fixtureLogin(page);await page.goto('/plugins')
 await expect(page.getByLabel('选择ZIP插件包')).toHaveCount(0)
 await page.getByRole('button',{name:'安装插件',exact:true}).click()
 const consent=page.getByRole('dialog',{name:'插件权限确认',exact:true});await expect(consent).toContainText('plugin.data.write')
 const install=consent.getByRole('button',{name:'确认安装或授权',exact:true});await expect(install).toBeDisabled();console.info('plugin-e2e: consent dialog ready')
 await consent.getByText('我已阅读并同意以上权限',{exact:true}).click();await expect(consent.getByRole('checkbox',{name:'我已阅读并同意以上权限',exact:true})).toBeChecked();console.info('plugin-e2e: explicit consent checked');await install.click();await expect(consent).toBeHidden()
 const card=page.locator('.instance-card').first();await expect(card).toContainText('期望停用')
 await card.getByRole('button',{name:'配置',exact:true}).click();await page.getByLabel('配置 JSON',{exact:true}).fill('{"greeting":"Owned fixture"}');await page.getByRole('button',{name:'保存配置',exact:true}).click();await expect(page.getByRole('dialog',{name:'实例配置',exact:true})).toBeHidden()
 await card.getByRole('button',{name:'启用',exact:true}).click();await page.getByRole('dialog',{name:'确认插件操作',exact:true}).getByRole('button',{name:'确认操作',exact:true}).click();await expect(card).toContainText('期望启用')
 await card.getByRole('button',{name:'停用',exact:true}).click();await page.getByRole('dialog',{name:'确认插件操作',exact:true}).getByRole('button',{name:'确认操作',exact:true}).click();await expect(card).toContainText('期望停用')
 await card.getByRole('button',{name:'查看 KV',exact:true}).click();await expect(page.getByRole('dialog',{name:'实例 KV 数据',exact:true})).toBeVisible();await page.getByRole('dialog',{name:'实例 KV 数据',exact:true}).getByRole('button',{name:'关闭此对话框'}).click()
 await card.getByRole('link',{name:'实例日志',exact:true}).click();await expect(page).toHaveURL(/instance=/);await expect(page.getByText(/实例筛选：/)).toBeVisible()
 await page.goto('/plugins');await page.screenshot({path:resolve(screenshots,'builtin-plugins-desktop.png'),animations:'disabled'})
 const items=(await(await page.request.get('/api/v1/plugins')).json()).data
 const catalog=(await(await page.request.get('/api/v1/plugins/catalog')).json()).data.items
 for(const i of items){expect((await page.request.post('/api/v1/plugins/operations',{headers:{'X-Requested-With':'media-tool'},data:{kind:'remove',operationKey:crypto.randomUUID(),instanceId:i.id,fingerprint:catalog.find((d:any)=>d.manifest.id===i.pluginId).fingerprint,expectedRevision:i.revision,confirm:true,scopes:[]}})).status()).toBe(200)}
})

test('PT metrics show actual coverage, zero and unknown separately, with mobile layout (fixture)',async({page})=>{
 await page.route('**/api/v1/sites/metrics',route=>route.fulfill({json:{data:[{accountId:'owned',name:'Owned PT',enabled:true,metrics:{uploaded:1099511627776,downloaded:1073741824,ratio:1024,bonus:1234.5,level:'Owned User',seeding:0,seedingBytes:0},collectedAt:'2026-10-12T08:00:00Z',attemptedAt:'2026-10-12T08:00:00Z',state:'partial',message:'',stale:false},{accountId:'unknown',name:'Unknown PT',enabled:true,metrics:null,collectedAt:null,attemptedAt:null,state:'not_synced',message:'',stale:false}]}}))
 await fixtureLogin(page);await page.goto('/sites');await expect(page.getByRole('heading',{name:'站点最新数据',exact:true})).toBeVisible();await expect(page.locator('.metrics-overview').first()).toContainText('1 / 2');await expect(page.locator('.site-metric-row').filter({hasText:'Owned PT'})).toContainText('1.00 TiB');await expect(page.locator('.site-metric-row').filter({hasText:'Unknown PT'})).toContainText('—');await page.screenshot({path:resolve(screenshots,'pt-metrics-desktop.png'),animations:'disabled'});await page.setViewportSize({width:390,height:844});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);await page.screenshot({path:resolve(screenshots,'pt-metrics-mobile.png'),animations:'disabled'});await page.getByLabel('搜索站点名称',{exact:true}).fill('Unknown');await expect(page.locator('.site-metric-row')).toHaveCount(1)
})

test('database dark console supports simple mode and formatted row detail (fixture)',async({page})=>{
 await page.route('**/api/v1/data/postgres/tables',route=>route.fulfill({json:{data:[{name:'owned_records',estimatedRows:1}]}}));await page.route('**/api/v1/data/postgres/tables/owned_records?*',route=>route.fulfill({json:{data:{items:[{id:'owned',state:'succeeded',payload:{hello:'world',apiKey:'[已隐藏]'}}],total:1}}}))
 await fixtureLogin(page);await page.goto('/data');await expect(page.locator('.data-row')).toHaveCount(1);await page.locator('label[for="data-simple"]').click();await page.locator('.data-row').click();const dialog=page.getByRole('dialog').filter({has:page.locator('pre')});await expect(dialog).toContainText('"hello": "world"');await page.screenshot({path:resolve(screenshots,'database-console-desktop.png'),animations:'disabled'});await dialog.getByRole('button',{name:'关闭此对话框',exact:true}).click();await page.getByLabel('筛选当前页数据',{exact:true}).fill('absent');await expect(page.locator('.data-row')).toHaveCount(0)
})

test('automatic media search retains movie and series with matching numeric IDs (fixture)',async({page})=>{
 const queries:string[]=[];await page.route('**/api/v1/search/media?*',route=>{queries.push(new URL(route.request().url()).searchParams.get('mediaType')||'');return route.fulfill({json:{data:{items:[{id:11,mediaType:'movie',title:'Owned Film',originalTitle:'Owned Film',year:2026},{id:11,mediaType:'series',title:'Owned Series',originalTitle:'Owned Series',year:2026}],cached:false,recognitionHint:{title:'Owned',year:'2026'}}}})})
 await fixtureLogin(page);await page.goto('/search');await page.getByText('TMDB 媒体搜索',{exact:true}).click();await page.getByLabel('TMDB媒体名称',{exact:true}).fill('Owned.2026.1080p.WEB-DL.mkv');await page.getByRole('button',{name:'搜索媒体',exact:true}).click();await expect(page.getByRole('heading',{name:/^Owned Film 2026 · 电影$/})).toBeVisible();await expect(page.getByRole('heading',{name:/^Owned Series 2026 · 剧集$/})).toBeVisible();expect(queries).toContain('auto');await page.screenshot({path:resolve(screenshots,'auto-media-search.png'),animations:'disabled'})
})

test.describe('real PWA lifecycle',()=>{
 test.use({serviceWorkers:'allow'})
test('PWA registration, public-only cache, headers and generic offline page (localhost secure context)',async({page,context})=>{
 await fixtureLogin(page);await page.goto('/settings?tab=mobile');await expect(page.getByRole('heading',{name:'移动端应用 / PWA',exact:true})).toBeVisible()
 await page.evaluate(async()=>{await navigator.serviceWorker.ready});await page.reload();await expect(page.getByRole('heading',{name:'移动端应用 / PWA',exact:true})).toBeVisible()
 const sw=await page.request.get('/sw.js');expect(sw.headers()['cache-control']).toBe('no-cache');expect(sw.headers()['service-worker-allowed']).toBe('/');expect(await sw.text()).not.toContain('__BUILD_ID__')
 const manifest=await page.request.get('/manifest.webmanifest');expect(manifest.status()).toBe(200);const body=await manifest.json();expect(body).toMatchObject({name:'Media Tool 媒体工作台',start_url:'/dashboard',scope:'/',display:'standalone'});expect(new URL(body.start_url,manifest.url()).origin).toBe(new URL(process.env.E2E_BASE_URL!).origin)
 for(const path of ['/manifest.webmanifest','/sw.js','/offline.html','/pwa/icon-192.png','/pwa/icon-512.png']){const reply=await page.request.get('http://localhost:18087'+path);expect(reply.status()).toBe(404);expect(reply.headers()['content-type']).toContain('application/json');expect(reply.headers()['service-worker-allowed']).toBeUndefined();expect(await reply.json()).toEqual({ResponseStatus:{ErrorCode:'NotFound',Message:'Not Found'}})}
 const keys=await page.evaluate(async()=>{const out:string[]=[];for(const name of await caches.keys()){if(name.startsWith('media-tool-public-'))for(const request of await (await caches.open(name)).keys())out.push(new URL(request.url).pathname)}return out});expect(keys).toContain('/offline.html');expect(keys.every(key=>key.startsWith('/assets/')||key.startsWith('/pwa/')||key==='/offline.html')).toBe(true)
 await context.setOffline(true);await page.goto('/data');await expect(page.getByRole('heading',{name:'当前处于离线状态',exact:true})).toBeVisible();await expect(page.getByRole('heading',{name:'数据库浏览',exact:true})).toHaveCount(0);await context.setOffline(false)
})

})

test('site-result filters issue server query and preserve selection binding (fixture)',async({page})=>{
 const seen:string[]=[]
 await page.route('**/api/v1/search',route=>route.fulfill({json:{data:[{id:'owned-filter-run',keyword:'Owned',state:'succeeded',sites:[],resources:[],total:65}]}}))
 await page.route('**/api/v1/search/owned-filter-run?*',route=>{const q=new URL(route.request().url()).searchParams;seen.push(q.toString());return route.fulfill({json:{data:{id:'owned-filter-run',keyword:'Owned',state:'succeeded',sites:[{accountId:'owned-site',name:'Owned PT',state:'succeeded',message:'',count:65}],resources:[{id:'original-resource-id',title:'Owned.Film.2026.2160p.WEB-DL.x265',siteName:'Owned PT',size:1073741824,seeders:11,leechers:0,downloadFactor:0,metadata:{attributes:{resolution:'2160p',codec:'h265',source:'web-dl'}}}],total:q.get('resolution')==='2160p'?12:65}}})})
 await fixtureLogin(page);await page.goto('/search');await page.getByText('站点直搜',{exact:true}).click();await page.getByRole('button',{name:'查看结果',exact:true}).click();await expect(page.getByText('Owned.Film.2026.2160p.WEB-DL.x265',{exact:true})).toBeVisible()
 await page.locator('.el-select').filter({has:page.getByRole('combobox',{name:'资源分辨率',exact:true})}).click();await page.getByRole('option',{name:'2160p',exact:true}).click();await page.getByRole('combobox',{name:'资源分辨率',exact:true}).press('Escape');await expect(page.getByText(/共 12 条匹配/)).toBeVisible();expect(seen.some(q=>new URLSearchParams(q).get('resolution')==='2160p'&&new URLSearchParams(q).get('page')==='1')).toBe(true)
 for(const label of ['最少做种人数','最小资源大小','最大资源大小','仅免费'])await expect(page.getByLabel(label,{exact:true})).toHaveCount(0)
 await page.getByLabel('资源关键字筛选',{exact:true}).fill('film x265')
 await expect.poll(()=>seen.some(q=>new URLSearchParams(q).get('keyword')==='film x265')).toBe(true)
 await expect(page.getByRole('combobox',{name:'资源排序',exact:true})).toHaveCount(0);await page.getByRole('button',{name:'按文件大小排序',exact:true}).click();await expect.poll(()=>seen.some(q=>new URLSearchParams(q).get('sort')==='size_desc')).toBe(true);await page.getByRole('button',{name:'按文件大小排序',exact:true}).click()
 await expect.poll(()=>seen.some(q=>new URLSearchParams(q).get('sort')==='size_asc'&&new URLSearchParams(q).get('page')==='1')).toBe(true)
 await page.getByRole('button',{name:'按做种人数排序',exact:true}).press('Enter');await expect.poll(()=>seen.some(q=>new URLSearchParams(q).get('sort')==='seeders_desc')).toBe(true);await page.getByRole('button',{name:'按做种人数排序',exact:true}).press('Space');await expect.poll(()=>seen.some(q=>new URLSearchParams(q).get('sort')==='seeders_asc')).toBe(true)
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
 await page.goto('/plugins');await expect(page.locator('.instance-card')).toHaveCount(2);await page.setViewportSize({width:390,height:844});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true)
 for(const i of [first,clone])await op('remove',{instanceId:i.id,expectedRevision:i.revision})
})

test('subscription structured filters persist and preview displays individual reasons (fixture)',async({page})=>{
 await page.route('**/api/v1/subscriptions**',route=>route.fulfill({json:{data:[{id:'owned-subscription',title:'Owned Rules',media:{id:1,mediaType:'movie',title:'Owned Film',year:'2026'},enabled:false,source:'search',intervalMinutes:30,rules:{accountIds:['owned-account'],keyword:'Owned',include:[],exclude:[],minSize:0,maxSize:0,minSeeders:1,freeOnly:true,allowUnknownHR:false,season:1,episodes:[],includeHistory:false,resolution:'2160p',source:'web-dl',codec:'h265'},revision:1,lastState:'succeeded',lastMessage:'',nextRunAt:''}]}}))
 await page.route('**/api/v1/subscriptions/owned-subscription/evaluate',route=>route.fulfill({status:202,json:{data:{id:'owned-preview'}}}));await page.route('**/api/v1/subscription-runs/owned-preview',route=>route.fulfill({json:{data:{id:'owned-preview',preview:true,state:'succeeded',decisions:[{id:'owned-decision',title:'Owned Film 2026 1080p WEB-DL x264',action:'skipped',episodes:[],reasons:['分辨率不匹配','编码不匹配'],checks:[{key:'resolution',label:'分辨率',expected:'2160p',actual:'1080p',state:'rejected',message:'分辨率不匹配：需要2160p，识别为1080p'},{key:'source',label:'来源',expected:'web-dl',actual:'web-dl',state:'matched',message:'来源匹配：web-dl'}]}]}}}));await fixtureLogin(page);await page.goto('/subscriptions');await page.getByRole('button',{name:'编辑',exact:true}).click();await expect(page.getByRole('combobox',{name:'订阅分辨率',exact:true})).toBeVisible();await expect(page.getByRole('dialog',{name:'编辑订阅',exact:true})).toContainText('2160p');await page.getByRole('dialog',{name:'编辑订阅',exact:true}).getByRole('button',{name:'关闭此对话框',exact:true}).click();await page.getByRole('button',{name:'预演',exact:true}).click();await expect(page.getByRole('dialog',{name:'订阅评估与决策',exact:true})).toContainText('分辨率 · 拒绝');await expect(page.getByRole('dialog',{name:'订阅评估与决策',exact:true})).toContainText('来源 · 通过');await page.screenshot({path:resolve(screenshots,'subscription-structured-preview.png'),animations:'disabled'})
})

test('raw administrator log URLs have dedicated full request and 302 target view (fixture)',async({page})=>{
 await page.route('**/api/v1/logs/query?*',route=>route.fulfill({json:{data:{items:[{time:'2026-10-12T10:00:00Z',level:'INFO',module:'playback',msg:'HTTP request',url:'https://owned.example/strm?apiKey=owned-fixture-key',location:'https://cdnfhnfile.115cdn.net/owned?sign=owned-fixture-sign',status:302,durationMs:12,requestId:'owned-request'}],total:1,page:1,pageSize:200,modules:['playback'],before:'2026-10-12T11:00:00Z',truncated:false}}}));await fixtureLogin(page);await page.goto('/logs');await page.getByRole('button',{name:'查看第1条日志详情',exact:true}).click();const dialog=page.getByRole('dialog',{name:'日志详情',exact:true});await expect(dialog).toBeVisible();await expect(dialog.locator('textarea').nth(0)).toHaveValue('https://owned.example/strm?apiKey=owned-fixture-key');await expect(dialog.locator('textarea').nth(1)).toHaveValue('https://cdnfhnfile.115cdn.net/owned?sign=owned-fixture-sign');await page.screenshot({path:resolve(screenshots,'raw-playback-url-dialog.png'),animations:'disabled'})
})

test('PT site total counts all added sites regardless of metrics or status (fixture)',async({page})=>{
 await page.route('**/api/v1/sites/metrics',route=>route.fulfill({json:{data:[{accountId:'one',name:'HDSKY',enabled:true,metrics:{uploaded:10,downloaded:1,seeding:50,seedingBytes:100},stale:false,state:'succeeded'},{accountId:'two',name:'SSD',enabled:true,metrics:{uploaded:20,downloaded:2,seeding:4,seedingBytes:20},stale:false,state:'succeeded'}]}}))
 await fixtureLogin(page);await page.goto('/sites');const box=page.locator('.metrics-overview article').filter({hasText:'已添加站点数'});await expect(box.locator('strong')).toHaveText('2');await expect(page.getByText('总做种数',{exact:true})).toHaveCount(0)
 await page.route('**/api/v1/sites/metrics',route=>route.fulfill({json:{data:[{accountId:'one',name:'HDSKY',enabled:true,metrics:{uploaded:10,downloaded:1,seeding:50},stale:true,state:'failed'},{accountId:'disabled',name:'SSD',enabled:false,metrics:null,state:'not_synced'}]}}));await page.reload();await expect(page.locator('.metrics-overview article').filter({hasText:'已添加站点数'}).locator('strong')).toHaveText('2');await page.getByLabel('搜索站点名称',{exact:true}).fill('HDSKY');await expect(page.locator('.metrics-overview article').filter({hasText:'已添加站点数'}).locator('strong')).toHaveText('2')
})

test('site data schedule persists cron and disable settings (isolated)',async({page})=>{
 await fixtureLogin(page);await page.goto('/sites');await page.getByRole('button',{name:'定时刷新',exact:true}).click();const dialog=page.getByRole('dialog',{name:'站点数据定时刷新',exact:true});await dialog.getByText('启用定时刷新',{exact:true}).click();await dialog.getByRole('textbox',{name:'站点刷新Cron',exact:true}).fill('0 */2 * * *');await dialog.getByRole('button',{name:'保存定时刷新',exact:true}).click();await expect(page.getByText('自动刷新 · 0 */2 * * * · Asia/Shanghai',{exact:true})).toBeVisible();await page.reload();await expect(page.getByText('自动刷新 · 0 */2 * * * · Asia/Shanghai',{exact:true})).toBeVisible();await page.getByRole('button',{name:'定时刷新',exact:true}).click();await dialog.getByText('启用定时刷新',{exact:true}).click();await dialog.getByRole('button',{name:'保存定时刷新',exact:true}).click();await expect(page.getByText('自动刷新已关闭',{exact:true})).toBeVisible();await page.setViewportSize({width:390,height:844});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);await page.screenshot({path:resolve(screenshots,'site-schedule-mobile.png'),fullPage:true,animations:'disabled'})
})

test('site action buttons and mixed search-history states align at desktop and mobile (fixture)',async({page})=>{
 await page.route('**/api/v1/sites/metrics',route=>route.fulfill({json:{data:[{accountId:'one',name:'HDSKY',enabled:true,metrics:null,state:'not_synced',stale:false}]}}));await fixtureLogin(page);await page.goto('/sites');const action=page.locator('.metric-state');await expect(action.getByRole('button',{name:'同步此站',exact:true})).toBeVisible();const rects=await action.locator('.el-button').evaluateAll(els=>els.map(el=>({left:el.getBoundingClientRect().left,width:el.getBoundingClientRect().width,margin:getComputedStyle(el).marginLeft})));expect(Math.abs(rects[0].left-rects[1].left)).toBeLessThan(1);expect(Math.abs(rects[0].width-rects[1].width)).toBeLessThan(1);expect(rects.every(x=>x.margin==='0px')).toBe(true);await page.screenshot({path:resolve(screenshots,'site-action-alignment.png'),fullPage:true,animations:'disabled'});
 await page.route('**/api/v1/search',route=>route.fulfill({json:{data:[{id:'one',keyword:'短标题',state:'succeeded'},{id:'two',keyword:'Very Long Original Media Title 2026',state:'partial'},{id:'three',keyword:'第三行',state:'failed'}]}}));await page.goto('/search');await page.getByText('站点直搜',{exact:true}).click();await expect(page.locator('.recent-row .status-badge')).toHaveCount(3);const left=await page.locator('.recent-row .status-badge').evaluateAll(els=>els.map(el=>el.getBoundingClientRect().left));expect(Math.max(...left)-Math.min(...left)).toBeLessThan(1);await page.screenshot({path:resolve(screenshots,'search-history-alignment.png'),fullPage:true,animations:'disabled'});await page.setViewportSize({width:390,height:844});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);await page.screenshot({path:resolve(screenshots,'search-history-mobile.png'),fullPage:true,animations:'disabled'})
})

test('subscription query controls, regex and shared TMDB cache are explicit; removed limits stay absent (fixture)',async({page})=>{
 let calls=0;await page.route('**/api/v1/search/media?*',route=>route.fulfill({json:{data:{items:[{id:42,mediaType:'movie',title:'Owned Film',year:'2026'}],cached:++calls>1,cacheSeconds:86400}}}));await fixtureLogin(page);await page.goto('/subscriptions');await page.getByRole('button',{name:'创建订阅',exact:true}).click();const dialog=page.getByRole('dialog',{name:'创建停用订阅',exact:true});const query=dialog.getByLabel('订阅TMDB关键词',{exact:true});await expect(query).toBeVisible();await expect(dialog.getByRole('button',{name:'查询TMDB',exact:true})).toBeDisabled();await query.fill('Owned Film');await dialog.getByRole('button',{name:'查询TMDB',exact:true}).click();await expect(dialog.getByText(/已查询TMDB · 缓存有效期 86400 秒/)).toBeVisible();await dialog.getByRole('button',{name:'查询TMDB',exact:true}).click();await expect(dialog.getByText(/来自TMDB缓存/)).toBeVisible();for(const label of ['最小体积 GiB','最大体积 GiB（0不限）','最少做种人数','间隔分钟'])await expect(dialog.getByText(label,{exact:true})).toHaveCount(0);await expect(dialog.getByRole('checkbox',{name:'使用正则匹配包含／排除关键词',exact:true})).toBeChecked();await dialog.getByLabel('订阅包含关键词',{exact:true}).fill('(?i)WEB[-.]DL\nS[0-9]{1,3}');await query.scrollIntoViewIfNeeded();const queryWidth=await query.evaluate(el=>el.getBoundingClientRect().width);expect(queryWidth).toBeGreaterThan(200);await page.screenshot({path:resolve(screenshots,'subscription-query-desktop.png'),animations:'disabled'});await page.setViewportSize({width:390,height:844});await query.scrollIntoViewIfNeeded();const dims=await dialog.locator('.subscription-media-query').evaluate(el=>({width:el.clientWidth,scroll:el.scrollWidth}));expect(dims.scroll).toBeLessThanOrEqual(dims.width+1);await page.screenshot({path:resolve(screenshots,'subscription-query-mobile.png'),animations:'disabled'});await dialog.getByRole('button',{name:'关闭此对话框',exact:true}).click();await page.goto('/settings?tab=automation');await expect(page.getByLabel('订阅 Cron 表达式',{exact:true})).toBeVisible();await expect(page.getByLabel('TMDB 搜索缓存有效期（秒）',{exact:true})).toBeVisible()
})

test('template list layout persists and multi-select result facets retain repeated values (fixture)',async({page})=>{
 await fixtureLogin(page);await page.goto('/site-templates');await page.getByRole('button',{name:'列表展示模板',exact:true}).click();await expect(page.locator('.template-list')).toBeVisible();await page.reload();await expect(page.locator('.template-list')).toBeVisible();await page.getByRole('button',{name:'网格展示模板',exact:true}).click();await expect(page.locator('.template-list')).toHaveCount(0)
 const seen:string[]=[];await page.route('**/api/v1/search',route=>route.fulfill({json:{data:[{id:'owned-multiple',keyword:'Owned',state:'succeeded'}]}}));await page.route('**/api/v1/search/owned-multiple?*',route=>{seen.push(new URL(route.request().url()).search);return route.fulfill({json:{data:{id:'owned-multiple',keyword:'Owned',state:'succeeded',sites:[{accountId:'a',name:'Owned A',state:'succeeded'},{accountId:'b',name:'Owned B',state:'succeeded'}],resources:[],total:0}}})});await page.goto('/search');await page.getByText('站点直搜',{exact:true}).click();await page.getByRole('button',{name:'查看结果',exact:true}).click();await page.locator('.el-select').filter({has:page.getByRole('combobox',{name:'资源分辨率',exact:true})}).click();await page.getByRole('option',{name:'1080p',exact:true}).click();await page.getByRole('option',{name:'2160p',exact:true}).click();await page.getByRole('combobox',{name:'资源分辨率',exact:true}).press('Escape');await expect.poll(()=>seen.some(x=>new URLSearchParams(x).getAll('resolution').length===2)).toBe(true)
})

test('independent cache pages expose values and confirmed scoped clear only (fixture)',async({page})=>{
 await page.route('**/api/v1/metadata/image/*/owned.png',r=>r.fulfill({contentType:'image/svg+xml',body:'<svg xmlns="http://www.w3.org/2000/svg" width="80" height="120"><rect width="80" height="120" fill="#c7ded3"/></svg>'}));
 const id='a'.repeat(64),requests:string[]=[];await page.route('**/api/v1/cache/**',route=>{const path=new URL(route.request().url()).pathname;requests.push(path);if(path.endsWith('/clear'))return route.fulfill({json:{data:{deleted:1,remainingMayExist:false}}});if(path.endsWith('/'+id))return route.fulfill({json:{data:{id,key:'media-tool:tmdb-cache:'+id,data:{query:'Owned',items:[{title:'Owned Film'}]}}}});return route.fulfill({json:{data:{items:[{id,key:'media-tool:tmdb-cache:'+id,query:'Owned',state:'cached',bytes:200,ttlSeconds:45,count:1}],total:1,page:1,truncated:false}}})});await fixtureLogin(page);await page.goto('/tmdb-cache');await expect(page.getByRole('heading',{name:'TMDB缓存',exact:true})).toBeVisible();await page.getByRole('button',{name:'查看',exact:true}).click();await expect(page.getByRole('dialog',{name:'缓存内容',exact:true})).toContainText('Owned Film');await page.getByRole('button',{name:'关闭此对话框',exact:true}).click();await page.getByRole('button',{name:'清理此类缓存',exact:true}).click();await page.getByRole('button',{name:'确认清理',exact:true}).click();expect(requests.some(x=>x.endsWith('/tmdb/clear'))).toBe(true);await page.goto('/search-cache');await expect(page.getByRole('heading',{name:'搜索缓存',exact:true})).toBeVisible();expect(requests.some(x=>x.includes('/search'))).toBe(true);await page.setViewportSize({width:390,height:844});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);await page.screenshot({path:resolve(screenshots,'cache-mobile.png'),fullPage:true,animations:'disabled'})
})

test('search media context has verified identity, details and disabled subscription prefill (fixture)',async({page})=>{
 await page.route('**/api/v1/metadata/image/**',route=>route.fulfill({contentType:'image/png',body:readFileSync(resolve(root,'web/public/pwa/icon-192.png'))}))
 await page.route('**/api/v1/search',route=>route.fulfill({json:{data:[{id:'owned-context',keyword:'Owned Film',state:'succeeded'}]}}));await page.route('**/api/v1/search/owned-context?*',route=>route.fulfill({json:{data:{id:'owned-context',keyword:'Owned Film',state:'succeeded',sites:[],resources:[],total:0}}}));await page.route('**/api/v1/search/media?*',route=>route.fulfill({json:{data:{items:[{id:42,mediaType:'movie',title:'Owned Film',originalTitle:'Owned Film',year:'2026'}],cached:false}}}));await page.route('**/api/v1/metadata/details?*',route=>route.fulfill({json:{data:{item:{id:42,mediaType:'movie',title:'Owned Film',originalTitle:'Owned Film',year:'2026',overview:'Owned movie overview',genres:['剧情'],posterPath:'/owned.png',backdropPath:'/owned-backdrop.png'},cached:false}}}));await fixtureLogin(page);await page.goto('/search');await page.getByText('站点直搜',{exact:true}).click();await page.getByRole('button',{name:'查看结果',exact:true}).click();await expect(page.locator('.media-context')).toContainText('Owned Film');await expect(page.locator('.media-context')).toContainText('TMDB ID：42');await expect(page.locator('.context-poster')).toHaveAttribute('src','/api/v1/metadata/image/w500/owned.png');await page.getByRole('button',{name:'查看详情',exact:true}).click();await expect(page.getByRole('dialog',{name:'媒体详情',exact:true})).toContainText('Owned movie overview');await page.getByRole('button',{name:'关闭此对话框',exact:true}).click();await expect(page.getByRole('dialog',{name:'媒体详情',exact:true})).not.toBeVisible();await page.screenshot({path:resolve(screenshots,'search-media-context.png'),fullPage:true,animations:'disabled'});await expect(page.locator('.media-context').getByRole('button',{name:'订阅',exact:true})).toHaveCount(0);await expect(page.locator('.media-context')).not.toHaveCSS('background-image',/url/);await expect(page.locator('.context-poster')).toHaveCSS('width','88px')
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
 await fixtureLogin(page);await page.goto('/search');await expect(page.getByRole('heading',{name:'最近搜索',exact:true})).toBeVisible();await expect(page.getByRole('link',{name:'全部记录',exact:true})).toHaveAttribute('href','/search-cache');await expect(page.locator('.acquire-workspace-nav')).toHaveCount(0);
 await page.getByLabel('TMDB媒体名称',{exact:true}).fill(media.title);await page.getByRole('button',{name:'搜索媒体',exact:true}).click();await page.getByRole('button',{name:media.title,exact:true}).click();expect(started).toBe(true);
 await expect(page).toHaveURL(new RegExp('/search/results/'+id+'$'));await expect(page.getByRole('heading',{name:'搜索结果',exact:true})).toBeVisible();await expect(page.getByLabel('TMDB媒体名称',{exact:true})).toHaveCount(0);
 await expect(page.getByText('HDSKY · H264 · WEB-DL · 1080p',{exact:true})).toBeVisible();await expect(page.getByText('50%DL',{exact:true})).toBeVisible();await expect(page.locator('.media-context').getByRole('button',{name:'订阅',exact:true})).toHaveCount(0);
 await page.screenshot({path:resolve(screenshots,'runtime-inline-search.png'),fullPage:true,animations:'disabled'});
 await page.route('**/api/v1/cache/search?*',r=>r.fulfill({json:{data:{items:[{id,query:media.title,state:'succeeded',count:1,ttlSeconds:45,reusable:true,bytes:100,posterPath:media.posterPath,createdAt:new Date().toISOString()}],total:1}}}));await page.goto('/search-cache');await expect(page.getByRole('button',{name:'查看结果',exact:true})).toBeVisible();await expect(page.locator('.cache-desktop .cache-content img')).toHaveAttribute('src','/api/v1/metadata/image/w500/owned.png');await expect(page.getByText(/media-tool:search-cache:/)).toHaveCount(0);
 await page.setViewportSize({width:390,height:844});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);await page.screenshot({path:resolve(screenshots,'runtime-search-cache-mobile.png'),fullPage:true,animations:'disabled'});
});

test('runtime update: overview metrics, categorized connections and separate downloader panels (fixture)',async({page})=>{
 await page.route('**/api/v1/system/runtime',r=>r.fulfill({json:{data:{server:{loadAverage:[.5,.4,.3],memoryUsed:2147483648,memoryTotal:8589934592,diskAvailable:107374182400,diskTotal:214748364800,uptimeSeconds:90000,sampledAt:new Date().toISOString()},daily:{sites:2,subscriptions:3,downloads:1,media:50,todaySearches:4,failedJobs:0}}}}));
 await fixtureLogin(page);await expect(page.getByText('100.0 GiB',{exact:true})).toBeVisible();for(const old of ['依赖服务','最近任务','资源到播放的路径'])await expect(page.getByRole('heading',{name:old,exact:true})).toHaveCount(0);await page.screenshot({path:resolve(screenshots,'runtime-dashboard.png'),fullPage:true,animations:'disabled'});
 await page.route('**/api/v1/integrations',r=>r.fulfill({json:{data:[{id:'cloud',kind:'open115',name:'115',enabled:true,lastCheckState:'ready'},{id:'media',kind:'emby',name:'Emby',enabled:true,lastCheckState:'ready'}]}}));await page.goto('/integrations');const filter=page.getByRole('combobox',{name:'连接分类',exact:true});await page.locator('.el-select').filter({has:filter}).click();await page.getByRole('option',{name:'云盘 (1)',exact:true}).click();await expect(page.locator('.el-table__body')).toContainText('115');await expect(page.locator('.el-table__body')).not.toContainText('Emby');
 await page.goto('/downloads');await expect(page.getByRole('heading',{name:'下载任务',exact:true})).toBeVisible();await expect(page.locator('.transfer-section')).toBeVisible();const gap=await page.evaluate(()=>document.querySelector('.transfer-section')!.getBoundingClientRect().top-document.querySelector('.download-section')!.getBoundingClientRect().bottom);expect(gap).toBeGreaterThanOrEqual(24);await page.screenshot({path:resolve(screenshots,'runtime-downloads.png'),fullPage:true,animations:'disabled'});
});

test('runtime update: configured proxy origin can log in but spoofed forwarding and missing CSRF fail (isolated)',async({page})=>{
 await fixtureLogin(page);
 const allowed=await page.request.post('/api/v1/auth/login',{headers:{Origin:'https://proxy.fixture.invalid','X-Requested-With':'media-tool'},data:{username,password}});expect(allowed.status()).toBe(200);
 const hostile=await page.request.post('/api/v1/auth/login',{headers:{Origin:'https://hostile.fixture.invalid','X-Requested-With':'media-tool','X-Forwarded-Host':'proxy.fixture.invalid','X-Forwarded-Proto':'https'},data:{username,password}});expect(hostile.status()).toBe(403);
 const missing=await page.request.post('/api/v1/auth/login',{headers:{Origin:'https://proxy.fixture.invalid'},data:{username,password}});expect(missing.status()).toBe(403);
});

test('acquisition: dedicated results survive reload and every history entry navigates there (fixture)',async({page})=>{
 const id='owned-standalone';await page.route('**/api/v1/search',r=>r.fulfill({json:{data:[{id,keyword:'Owned dedicated',state:'succeeded',total:1}]}}));await page.route(`**/api/v1/search/${id}?*`,r=>r.fulfill({json:{data:{id,keyword:'Owned dedicated',state:'succeeded',sites:[],resources:[],total:0}}}));await fixtureLogin(page);await page.goto('/search');await expect(page.getByRole('link',{name:'管理PT站点账号',exact:true})).toHaveCount(0);await page.getByRole('button',{name:'查看结果',exact:true}).click();await expect(page).toHaveURL(/\/search\/results\/owned-standalone$/);await expect(page.locator('h1')).toHaveText('搜索结果');await expect(page.getByRole('radiogroup',{name:'搜索方式'})).toHaveCount(0);await page.reload();await expect(page.locator('h1')).toHaveText('搜索结果');await page.getByRole('button',{name:'返回搜索',exact:true}).click();await expect(page).toHaveURL(/\/search$/);
});

test('acquisition: cache pages omit size and settings, use matching buttons and meaningful TTL (fixture)',async({page})=>{
 const id='22222222-2222-4222-8222-222222222222';let kind='tmdb';await page.route('**/api/v1/cache/**',r=>r.fulfill({json:{data:{items:[{id:kind==='tmdb'?'a'.repeat(64):id,query:'Owned cache',state:'succeeded',count:2,bytes:9999,ttlSeconds:7200,reusable:true}],total:1}}}));await fixtureLogin(page);await page.goto('/tmdb-cache');await expect(page.getByRole('columnheader',{name:'大小',exact:true})).toHaveCount(0);await expect(page.getByText('有效期设置',{exact:true})).toHaveCount(0);await expect(page.locator('.cache-desktop').getByText('2.00 小时',{exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'查看',exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'删除',exact:true})).toBeVisible();kind='search';await page.goto('/search-cache');await expect(page.getByRole('columnheader',{name:'大小',exact:true})).toHaveCount(0);await expect(page.getByRole('button',{name:'查看结果',exact:true})).toBeVisible();await page.setViewportSize({width:390,height:844});await expect(page.locator('.cache-mobile').getByRole('button',{name:'查看结果',exact:true})).toBeVisible();await page.screenshot({path:resolve(screenshots,'acquire-cache-mobile.png'),fullPage:true,animations:'disabled'});
});

test('acquisition: proxy settings are scoped, validated and persisted in isolated database',async({page})=>{
 await fixtureLogin(page);await page.goto('/settings?tab=proxy');await expect(page.getByRole('heading',{name:'网络代理',exact:true})).toBeVisible();await page.getByLabel('代理地址',{exact:true}).fill('http://127.0.0.1:7890');await page.getByRole('button',{name:'保存代理设置',exact:true}).click();await expect(page.getByText('代理设置已保存，新的请求使用所选路由。',{exact:true})).toBeVisible();await page.reload();await expect(page.getByLabel('代理地址',{exact:true})).toHaveValue('http://127.0.0.1:7890');await expect(page.getByRole('checkbox',{name:'TMDB查询与海报使用代理',exact:true})).not.toBeChecked();await page.getByLabel('代理地址',{exact:true}).fill('file:///unsafe');await page.getByRole('button',{name:'保存代理设置',exact:true}).click();await expect(page.getByText(/代理地址须使用/)).toBeVisible();await page.getByLabel('代理地址',{exact:true}).fill('');await page.getByRole('button',{name:'保存代理设置',exact:true}).click();await expect(page.getByText('代理设置已保存，新的请求使用所选路由。',{exact:true})).toBeVisible();
});

test('acquisition: subscriptions reject HR without unknown override and expose history (fixture)',async({page})=>{
 await page.route('**/api/v1/subscriptions**',r=>r.fulfill({json:{data:[]}}));await fixtureLogin(page);await page.goto('/subscriptions');await expect(page.getByText('历史订阅',{exact:true})).toBeVisible();await page.getByText('历史订阅',{exact:true}).click();await expect(page.getByRole('radiogroup',{name:'订阅记录分类',exact:true})).toBeVisible();await page.getByRole('button',{name:'创建订阅',exact:true}).click();const dialog=page.getByRole('dialog',{name:'创建停用订阅',exact:true});await expect(dialog.getByRole('checkbox',{name:'拒绝下载H&R资源',exact:true})).toBeChecked();await expect(dialog.getByText(/明确允许H/)).toHaveCount(0);await dialog.getByRole('button',{name:'关闭此对话框',exact:true}).click();
});

test('files and library readonly capabilities, persistent batch proof, scan controls and stage history (UI fixtures)',async({page})=>{
 await fixtureLogin(page)
 const id='12345678-1234-1234-1234-123456789012'
 const readonly={read:true,write:false,move:false,recycle:false,readOnly:true,role:'download-source',reason:'下载及做种原文件只读保留'}
 await page.route('**/api/v1/files/roots',r=>r.fulfill({json:{data:[{id,name:'自动发现的本地目录',kind:'local',path:'/data',revision:1,capabilities:readonly,connectionState:'accessible'}]}}))
 await page.route('**/api/v1/files?**',r=>r.fulfill({json:{data:{items:[{id:'inode',name:'Owned.Show.S01E01.mkv',path:'downloads/Owned.Show.S01E01.mkv',size:1024,directory:false,capabilities:readonly}],total:1,capabilities:readonly}}}))
 await page.route('**/api/v1/files/operations',r=>r.fulfill({json:{data:[]}}))
 await page.goto('/files');await expect(page.getByRole('button',{name:'新建文件夹',exact:true})).toBeDisabled();await expect(page.locator('.desktop-files').getByRole('button',{name:'影视整理',exact:true})).toBeDisabled()
 await page.setViewportSize({width:390,height:844});await expect(page.getByLabel('选择 Owned.Show.S01E01.mkv')).toBeDisabled();await expect(page.getByRole('button',{name:'上级目录',exact:true})).toBeVisible();expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await page.screenshot({path:resolve(screenshots,'files-library-readonly-mobile.png'),animations:'disabled',fullPage:true})
 await page.route('**/api/v1/library/scans',r=>r.fulfill({json:{data:[{id,root:'/115/Media',state:'paused',files:2,directories:4,completedDirectories:1,generation:2,revision:1,message:'不完整覆盖不用于缺失判断'}]}}))
 await page.goto('/cloudStorage');await page.getByRole('tab',{name:'扫描与计划',exact:true}).click();await expect(page.getByRole('button',{name:'重新完整对账',exact:true})).toBeVisible();expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await page.screenshot({path:resolve(screenshots,'files-library-scans-mobile.png'),animations:'disabled',fullPage:true})
 await page.route('**/api/v1/library/activity?**',r=>r.fulfill({json:{data:{items:[{id,kind:'file',state:'needs_reconcile',source:'Owned.mkv',batchId:'',connectionId:id,message:'只读回核对，不重放',createdAt:new Date().toISOString(),updatedAt:new Date().toISOString(),details:{completedSteps:0,plan:{steps:[{action:'relocate',source:{path:'Owned.mkv'},target:'Owned (2026).mkv'}],items:[{source:{path:'Owned.mkv'},target:'Owned (2026).mkv'}]}}}],total:1}}}))
 await page.goto('/organize-history');await page.locator('.history-action-menu summary').first().click();await page.getByRole('button',{name:'查看步骤详情',exact:true}).click();await expect(page.getByRole('dialog').getByRole('button',{name:'核对结果（不重放）',exact:true})).toBeVisible();await expect(page.getByRole('dialog')).toContainText('Owned (2026).mkv');expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await page.screenshot({path:resolve(screenshots,'files-library-history-mobile.png'),animations:'disabled',fullPage:false})
})


test('isolated local batch preview writes nothing until explicit approval and survives history reload',async({page})=>{
 await fixtureLogin(page)
 const folder=process.env.E2E_FILE_ROOT!
 const names=['Batch.Owned.Show.2026.S01E01.mkv','Batch.Owned.Show.2026.S01E02.mkv']
 for(const name of names)writeFileSync(resolve(folder,name),'Isolated batch fixture, not real media.')
 await page.goto('/files')
 for(const name of names){const row=page.locator('.desktop-files .el-table__row').filter({hasText:name});await row.locator('.el-checkbox').click()}
 await page.getByRole('button',{name:'批量整理预览（2）',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'批次整理工作台',exact:true})
 await expect(dialog.getByLabel('批次文件标题').first()).toHaveValue('Batch Owned Show')
 await dialog.getByRole('button',{name:'预览全部变更',exact:true}).click()
 await expect(dialog.getByRole('button',{name:'明确批准整个批次',exact:true})).toBeEnabled()
 for(const name of names)expect(existsSync(resolve(folder,name))).toBe(true)
 await dialog.screenshot({path:resolve(screenshots,'files-library-batch-preview-desktop.png'),animations:'disabled'})
 await page.setViewportSize({width:390,height:844});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await dialog.screenshot({path:resolve(screenshots,'files-library-batch-preview-mobile.png'),animations:'disabled'})
 await dialog.getByRole('button',{name:'明确批准整个批次',exact:true}).click()
 await expect(dialog.getByRole('button',{name:'刷新执行进度',exact:true})).toBeVisible()
 const batchLink=await dialog.getByRole('link',{name:'查看持久化批次',exact:true}).getAttribute('href');const batchID=new URL('http://localhost:18084'+batchLink).searchParams.get('batchId')!
 await expect.poll(async()=>{const response=await page.request.get(`/api/v1/files/batches/${batchID}`);return (await response.json()).data.state},{timeout:15000}).toBe('succeeded')
 await dialog.getByRole('button',{name:'刷新执行进度',exact:true}).click();await expect(dialog.locator('.batch-status')).toContainText('已完成')
 for(const name of names)expect(existsSync(resolve(folder,name))).toBe(false)
 await dialog.getByRole('link',{name:'查看持久化批次',exact:true}).click();await expect(page.getByRole('heading',{name:'文件与入库历史',exact:true})).toBeVisible();await page.reload();await page.locator('.history-action-menu summary').first().click();await expect(page.getByRole('button',{name:'查看步骤详情',exact:true}).first()).toBeVisible()
})

test('cloud page exposes honest quota and membership while library tabs keep metadata query usable at 390px (UI fixture)',async({page})=>{
 await fixtureLogin(page)
 await page.route('**/api/v1/cloud/accounts/info',r=>r.fulfill({json:{data:{connectionId:'owned',note:'挂载账户范围',info:{path:'/115/Owned',source:'115 官方开放平台 /open/user/info',checkedAt:'2026-10-14T00:00:00Z',space:{total:1099511627776,used:549755813888,free:549755813888},memberships:[{identity:'115VIP',level:'VIP',expiresAt:'2027-01-01T00:00:00Z'}]}}}}))
 await page.goto('/cloud-connections');await expect(page.getByRole('heading',{name:'云盘连接',exact:true})).toBeVisible();await expect(page.locator('.quota-grid')).toContainText('1.00 TiB');await expect(page.locator('.membership-row')).toContainText('2027');await page.screenshot({path:resolve(screenshots,'cloud-account-desktop.png'),animations:'disabled',fullPage:true})
 await page.route('**/api/v1/media/assets?page=*',r=>r.fulfill({json:{data:{items:[{id:'00000000-0000-4000-8000-000000000141',title:'自有电影',mediaType:'movie',path:'/115/Owned/owned.mkv',revision:1,artifactsState:'ready',state:'present',fileScoped:true}],total:1}}}))
 await page.route('**/api/v1/metadata/search?*',r=>r.fulfill({json:{data:[{id:11,title:'自有电影',year:'2026',mediaType:'movie'}]}}))
 await page.goto('/cloudStorage');await expect(page.getByRole('heading',{name:'云盘连接与115授权',exact:true})).toHaveCount(0);await page.getByRole('tab',{name:'已入库资产',exact:true}).click();await page.getByRole('button',{name:'修正身份 / 季集',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'修正媒体身份与季集',exact:true});await expect(dialog.getByLabel('TMDB查询关键词',{exact:true})).toHaveValue('自有电影');await expect(dialog.getByLabel('TMDB查询关键词',{exact:true})).toBeInViewport({ratio:1});await dialog.getByRole('button',{name:'查询TMDB',exact:true}).click();await expect(dialog.getByRole('row').filter({hasText:'自有电影'})).toBeVisible()
 await dialog.screenshot({path:resolve(screenshots,'metadata-query-desktop.png'),animations:'disabled'});await page.setViewportSize({width:390,height:844});await expect(dialog.getByLabel('TMDB查询关键词',{exact:true})).toBeInViewport({ratio:1});await expect(dialog.getByRole('button',{name:'查询TMDB',exact:true})).toBeInViewport({ratio:1});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await dialog.screenshot({path:resolve(screenshots,'metadata-query-mobile.png'),animations:'disabled'})
})

test('folder naming and scraping settings persist while history removal cannot delete media (isolated)',async({page})=>{
 await fixtureLogin(page);await page.goto('/settings?tab=library');const movie='{title} ({year})/{title} ({year}) - {videoFormat}',series='{title} ({year})/Season {season}/{title} - {season_episode} - 第{episode}集'
 await page.getByLabel('电影命名模板',{exact:true}).fill(movie);await page.getByLabel('剧集命名模板',{exact:true}).fill(series);await page.getByRole('button',{name:'预览命名模板',exact:true}).click();await expect(page.locator('.naming-preview')).toContainText('示例剧集 (2026)/Season 01/示例剧集 - S01E04 - 第04集.mkv')
 await page.locator('.scraping-group').filter({has:page.getByRole('heading',{name:'电影刮削项',exact:true})}).getByText('poster（海报）',{exact:true}).click();await page.getByRole('button',{name:'保存系统设置',exact:true}).click();await expect(page.getByText(/当前分组已保存/)).toBeVisible();const prefs=(await(await page.request.get('/api/v1/settings/application')).json()).data;expect(prefs.value.scraping.movie.poster).toBe(true)
 await page.screenshot({path:resolve(screenshots,'scraping-settings-desktop.png'),animations:'disabled',fullPage:true});await page.setViewportSize({width:390,height:844});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,'scraping-settings-mobile.png'),animations:'disabled',fullPage:true})
 // Restore isolated policy before other tests; never change production settings.
 prefs.value.scraping.movie.poster=false;expect((await page.request.put('/api/v1/settings/application',{headers:{'X-Requested-With':'media-tool'},data:prefs})).status()).toBe(200)
 await page.goto('/organize-history');const batch=page.locator('.history-row').filter({hasText:'整理批次'}).filter({hasText:'已完成'}).first();await expect(batch).toBeVisible();await batch.locator('.history-action-menu summary').click();await batch.getByRole('button',{name:'移除历史',exact:true}).click();const confirm=page.getByRole('dialog',{name:'移除整理历史',exact:true});await expect(confirm).toContainText('媒体、STRM、审计和恢复证据保留');await confirm.getByRole('button',{name:'明确移除历史',exact:true}).click();await expect(page.getByText('历史已从列表移除，媒体和恢复证据未删除',{exact:true})).toBeVisible()
})

test('native media boundary is explicit and responsive without changing Emby',async({page})=>{
 await fixtureLogin(page)
 await page.goto('/media-server')
 await expect(page.getByRole('heading',{name:'自研媒体库',exact:true})).toBeVisible()
 await expect(page.getByText('原生播放已接入',{exact:true})).toBeVisible()
 await expect(page.getByRole('link',{name:'继续使用现有 Emby 媒体库'})).toHaveAttribute('href','/mediaLibrary')
 for(const width of [375,768,1440]){
  await page.setViewportSize({width,height:900})
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
  await page.screenshot({path:resolve(screenshots,`native-media-boundary-${width}.png`),fullPage:true})
 }
 await page.goto('/media-users')
 await expect(page.getByRole('heading',{name:'媒体用户',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'添加媒体用户',exact:true}).click()
 await expect(page.getByRole('dialog')).toBeVisible()
 await expect(page.getByRole('checkbox',{name:'要求修改密码（播放器登录将被拒绝）',exact:true})).not.toBeChecked()
 await expect(page.getByRole('dialog')).toContainText('默认可直接在播放器登录')
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
 await page.getByRole('button',{name:'取消',exact:true}).click()
})

test('player UA observations can be approved and enforce whitelist before all playback paths',async({page})=>{
 await fixtureLogin(page)
 const ua='Owned-E2E-Player/1.0'
 const observed=await page.request.get('/api/v1/cloudStorage/strm302?apiKey=owned-invalid-fixture-key&path=/115/OwnedFixture.mkv',{headers:{'User-Agent':ua},maxRedirects:0})
 expect(observed.status()).toBe(401)
 await page.goto('/player-access')
 await expect(page.getByRole('heading',{name:'播放器访问',exact:true})).toBeVisible()
 await expect(page.getByText('当前仅记录访问，不按UA限制现有播放器。',{exact:true})).toBeVisible()
 await page.getByLabel('搜索UA记录',{exact:true}).fill(ua)
 await page.getByRole('button',{name:'搜索记录',exact:true}).click()
 const row=page.locator('.ua-table .el-table__row').filter({hasText:ua})
 await expect(row).toBeVisible()
 await row.getByRole('button',{name:'查看UA',exact:true}).click()
 let dialog=page.getByRole('dialog',{name:'完整UA访问信息',exact:true})
 await expect(dialog.getByLabel('实际收到的完整 UA',{exact:true})).toHaveValue(ua)
 await dialog.getByRole('button',{name:'从此记录加入白名单',exact:true}).click()
 dialog=page.getByRole('dialog',{name:'从访问记录加入白名单',exact:true})
 await expect(dialog.getByLabel('完整 User-Agent',{exact:true})).toHaveValue(ua)
 await expect(dialog.getByLabel('完整 User-Agent',{exact:true})).toHaveAttribute('readonly','')
 await dialog.getByLabel('播放器备注',{exact:true}).fill('Owned E2E player')
 await dialog.getByRole('button',{name:'确认加入白名单',exact:true}).click()
 await expect(page.getByText('Owned E2E player',{exact:true})).toBeVisible()
 await expect(page.getByRole('button',{name:'启用UA限制',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'启用UA限制',exact:true}).click()
 await page.getByRole('dialog',{name:'启用播放器白名单',exact:true}).getByRole('button',{name:'启用限制',exact:true}).click()
 await expect(page.getByRole('button',{name:'关闭UA限制',exact:true})).toBeVisible()
 try{
  const blocked=await page.request.get('/api/v1/cloudStorage/strm302?apiKey=owned-invalid-fixture-key',{headers:{'User-Agent':'Unknown-E2E-Player/2'},maxRedirects:0})
  expect(blocked.status()).toBe(403);expect(blocked.headers().location).toBeUndefined();expect((await blocked.json()).error.code).toBe('player_ua_denied')
  const allowed=await page.request.get('/api/v1/cloudStorage/strm302?apiKey=owned-invalid-fixture-key',{headers:{'User-Agent':ua},maxRedirects:0});expect(allowed.status()).toBe(401)
  for(const path of ['/Videos/11/original.mkv','/Items/11/PlaybackInfo','/emby/VIDEOS/11/stream']){
   const reply=await page.request.get('http://localhost:18085'+path,{headers:{'User-Agent':'Unknown-E2E-Player/2'},maxRedirects:0});expect(reply.status()).toBe(403);expect(reply.headers().location).toBeUndefined()
  }
  await expect(page.locator('.el-message')).toHaveCount(0,{timeout:6000})
  for(const width of [375,768,1440]){
   await page.setViewportSize({width,height:900});await page.getByRole('button',{name:'刷新访问记录',exact:true}).click()
   expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
   await page.screenshot({path:resolve(screenshots,`player-ua-access-${width}.png`),fullPage:true})
  }
 }finally{
  const current=await page.request.get('/api/v1/player-access/policy');const p=(await current.json()).data
  const disabled=await page.request.put('/api/v1/player-access/policy',{headers:{'X-Requested-With':'media-tool'},data:{enabled:false,revision:p.revision,rules:p.rules}});expect(disabled.status()).toBe(200)
 }
 await page.getByRole('button',{name:'刷新访问记录',exact:true}).click()
 await page.getByRole('button',{name:'清理UA记录',exact:true}).click()
 await page.getByRole('dialog',{name:'清理UA访问记录',exact:true}).getByRole('button',{name:'清理记录',exact:true}).click()
 await expect(page.getByText('Owned E2E player',{exact:true})).toBeVisible()
 const p=(await(await page.request.get('/api/v1/player-access/policy')).json()).data
 expect(p.rules).toHaveLength(1);expect(p.enabled).toBe(false)
 expect((await page.request.put('/api/v1/player-access/policy',{headers:{'X-Requested-With':'media-tool'},data:{enabled:false,revision:p.revision,rules:[]}})).status()).toBe(200)
})

test('subscription tabs have separation and PT metrics actions are centered',async({page})=>{
 await fixtureLogin(page);await page.goto('/subscriptions')
 const tabs=page.getByRole('radiogroup',{name:'订阅记录分类',exact:true});const table=page.locator('.el-table').first();await expect(tabs).toBeVisible();await expect(table).toBeVisible()
 const bottom=await tabs.evaluate(el=>el.getBoundingClientRect().bottom);const top=await table.evaluate(el=>el.getBoundingClientRect().top);expect(top-bottom).toBeGreaterThanOrEqual(20)
 await page.route('**/api/v1/sites/metrics',r=>r.fulfill({json:{data:[{accountId:'owned-center',name:'HDSKY',enabled:true,metrics:{uploaded:1,downloaded:0,seeding:2,seedingBytes:1024},state:'succeeded',stale:false}]}}))
 await page.goto('/sites');const metric=page.locator('.site-metric-row').first();await expect(metric).toBeVisible();expect(await metric.evaluate(el=>getComputedStyle(el).alignItems)).toBe('center')
 await page.screenshot({path:resolve(screenshots,'workbench-site-centered.png'),fullPage:true})
})

test('STRM directory has posters and no NFO collection while tools persist repair previews',async({page})=>{
 const asset='00000000-0000-4000-8000-000000000246',repair='00000000-0000-4000-8000-000000000247'
 await page.route('**/api/v1/library/outputs?*',r=>r.fulfill({json:{data:{items:[{id:asset,title:'自有STRM电影',year:'2026',mediaType:'movie',source:'/115/Owned/film.mkv',poster:'/pwa/icon-192.png',cloudState:'present',artifactsState:'ready',revision:1,outputs:[{kind:'strm',path:'Movies/film.strm',owner:'asset:'+asset},{kind:'nfo',path:'Movies/film.nfo',owner:'asset:'+asset}],embyState:'pending',embyPaths:[],revoked:false}],total:1}}}))
 await fixtureLogin(page);await page.goto('/strm');await expect(page.getByRole('heading',{name:'STRM目录',exact:true})).toBeVisible();await expect(page.getByRole('img',{name:'自有STRM电影海报',exact:true})).toBeVisible();await expect(page.getByRole('heading',{name:'生成链接预览',exact:true})).toHaveCount(0)
 await page.getByRole('button',{name:'STRM文件与内容',exact:true}).click();await expect(page.getByRole('dialog')).toContainText('Movies/film.strm');await expect(page.getByRole('dialog')).not.toContainText('Movies/film.nfo');await page.getByRole('dialog').getByRole('button',{name:'关闭此对话框',exact:true}).click();await expect(page.locator('.el-overlay:visible')).toHaveCount(0)
 for(const width of [375,1440]){await page.setViewportSize({width,height:900});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`workbench-strm-directory-${width}.png`),fullPage:true})}
 let approved=false
 const batch=()=>({id:repair,digest:'d'.repeat(64),state:approved?'succeeded':'preview',expiresAt:'2099-01-01T00:00:00Z',items:[{ordinal:0,assetId:asset,path:'Movies/film.strm',digest:'e'.repeat(64),state:approved?'repaired':'missing',message:'只恢复原文'}]})
 await page.route('**/api/v1/strm/repairs/preview',r=>{expect(r.request().postDataJSON()).toEqual({assetIds:[asset]});return r.fulfill({json:{data:batch()}})})
 await page.route(`**/api/v1/strm/repairs/${repair}`,r=>r.fulfill({json:{data:batch()}}))
 await page.route(`**/api/v1/strm/repairs/${repair}/approve`,r=>{expect(r.request().postDataJSON()).toEqual({digest:'d'.repeat(64),confirm:true});approved=true;return r.fulfill({json:{data:batch()}})})
 await page.goto('/strm-tools');await expect(page.getByRole('heading',{name:'STRM工具',exact:true})).toBeVisible();await page.getByRole('button',{name:'全局检查并预览修复',exact:true}).click();const selector=page.locator('.repair-selector');await expect(selector).toBeVisible();await selector.getByRole('checkbox').check();await selector.getByRole('button',{name:'检查已选 1 个资产',exact:true}).click();await expect(page).toHaveURL(/repairId=/);expect(approved).toBe(false)
 await page.reload();await expect(page.getByRole('button',{name:'批准批量修复',exact:true})).toBeVisible();await page.getByRole('button',{name:'批准批量修复',exact:true}).click();await page.getByRole('dialog',{name:'批准全局STRM修复',exact:true}).getByRole('button',{name:'明确批准修复',exact:true}).click();await expect(page.getByRole('heading',{name:'修复完成',exact:true})).toBeVisible();await expect(page.locator('.el-overlay:visible')).toHaveCount(0)
 await page.screenshot({path:resolve(screenshots,'workbench-strm-tools-repair.png'),fullPage:true})
})

test('UA visits expose IP and verified Emby users without inferring unknown callers',async({page})=>{
 await page.route('**/api/v1/player-access/events?*',r=>r.fulfill({json:{data:{items:[{id:'owned-visit',userAgent:'Owned-Player/1',clientIp:'192.0.2.50',entry:'emby',method:'GET',allowed:true,reason:'restriction_disabled',status:302,embyUserId:'actual-user',embyUserName:'已核验用户',embyVerified:true,createdAt:'2026-10-14T00:00:00Z'},{id:'unknown-visit',userAgent:'Owned-Player/1',clientIp:'192.0.2.51',entry:'strm',method:'HEAD',allowed:true,reason:'restriction_disabled',status:401,embyVerified:false,createdAt:'2026-10-14T00:00:01Z'}],total:2}}}))
 await fixtureLogin(page);await page.goto('/player-access');await expect(page.getByText('已核验用户',{exact:true})).toBeVisible();await expect(page.getByText('Emby用户未核验 / 不适用',{exact:true})).toBeVisible();await expect(page.getByText(/192\.0\.2\.50/)).toBeVisible();await expect(page.getByText('已核验用户ID actual-user',{exact:true})).toBeVisible();await page.screenshot({path:resolve(screenshots,'workbench-ua-users.png'),fullPage:true})
})

test('search promotion header sorts full results and history evidence starts folded',async({page})=>{
 let sort='';const run='00000000-0000-4000-8000-000000000248'
 await page.route(`**/api/v1/search/${run}?*`,r=>{sort=new URL(r.request().url()).searchParams.get('sort')||'';return r.fulfill({json:{data:{id:run,keyword:'Owned',state:'succeeded',siteResults:[],resources:[],total:0}}})})
 await fixtureLogin(page);await page.goto('/search/results/'+run);await expect(page.getByRole('button',{name:'促销',exact:true})).toBeVisible();await page.getByRole('button',{name:'促销',exact:true}).click();await expect.poll(()=>sort).toBe('promotion_desc');await page.getByRole('button',{name:'促销',exact:true}).click();await expect.poll(()=>sort).toBe('promotion_asc')
 await page.route('**/api/v1/library/activity?*',r=>r.fulfill({json:{data:{items:[{id:'owned-history',kind:'output',state:'applied',source:'Movies/Owned.strm',message:'输出已写入',createdAt:'2026-10-14T00:00:00Z',updatedAt:'2026-10-14T00:00:00Z',details:{operation:'write',kind:'strm'}}],total:1}}}))
 await page.goto('/organize-history');await expect(page.getByRole('navigation',{name:'历史阶段快捷筛选',exact:true})).toBeVisible();await page.locator('.history-action-menu summary').first().click();await page.getByRole('button',{name:'查看步骤详情',exact:true}).click();await expect(page.locator('.evidence-json')).toBeHidden();await page.getByText('查看完整原始证据（JSON）',{exact:true}).click();await expect(page.locator('.evidence-json')).toContainText('write');await page.getByRole('button',{name:'关闭',exact:true}).click();await expect(page.locator('.el-overlay:visible')).toHaveCount(0);await page.setViewportSize({width:375,height:900});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,'workbench-history-375.png'),fullPage:true})
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
 const dialog=page.getByRole('dialog',{name:'确认媒体身份与下载'});await expect(dialog.getByRole('heading',{name:'已使用搜索中的媒体身份'})).toBeVisible();await expect(dialog.getByText('1381078',{exact:true})).toBeVisible();await expect(dialog.getByRole('button',{name:'查询 TMDB'})).toHaveCount(0)
 for(const width of [1440,375]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`download-search-identity-${width}.png`),fullPage:true,animations:'disabled'})}
 await dialog.getByRole('button',{name:'创建下载任务',exact:true}).click();await expect.poll(()=>submitted?.tmdbId).toBe(1381078);expect(submitted?.searchRunId).toBe(runId);expect(submitted?.correctMediaIdentity).toBe(false);expect(lookup).toBe(0)
})

test('download zero, unknown and sync errors are distinct; refresh observes only (fixture)',async({page})=>{
 await fixtureLogin(page)
 const tasks=[{id:'zero',title:'真实0进度',state:'downloading',seedingState:'pending',progress:0,qbState:'stalledDL',downloadSpeed:0,uploadSpeed:0,seeds:0,peers:0,syncedAt:'2026-10-14T00:00:00Z',infoHash:'owned-hash',media:{title:'电影',year:'2026'}},{id:'unknown',title:'尚未核对的任务',state:'pending',progress:null,media:{}},{id:'stale',title:'保留最后有效进度',state:'downloading',progress:0.42,qbState:'downloading',syncError:'torrent not found in qBittorrent',media:{}}]
 await page.route('**/api/v1/downloads',r=>r.fulfill({json:{data:tasks}}));await page.route('**/api/v1/transfers',r=>r.fulfill({json:{data:[]}}));await page.route('**/api/v1/integrations',r=>r.fulfill({json:{data:[]}}))
 let sync=0;await page.route('**/api/v1/downloads/sync',r=>{expect(r.request().method()).toBe('POST');sync++;return r.fulfill({json:{data:{message:''}}})})
 await page.goto('/downloads');await expect(page.getByText('下载中 · 下载停滞（等待有效连接）',{exact:true})).toBeVisible();await expect(page.getByText('尚未同步',{exact:true})).toBeVisible();await expect(page.getByText('同步失败（保留上次进度）：torrent not found in qBittorrent',{exact:true})).toBeVisible()
 await page.getByRole('button',{name:'刷新进度',exact:true}).click();await expect.poll(()=>sync).toBe(1)
 for(const width of [1440,375]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`download-telemetry-${width}.png`),fullPage:true})}
})

test('Emby user filter uses verified IDs and echoes public address provenance (fixture)',async({page})=>{
 await fixtureLogin(page)
 await page.route('**/api/v1/player-access/policy',r=>r.fulfill({json:{data:{enabled:false,revision:1,rules:[]}}}));await page.route('**/api/v1/player-access/observations?*',r=>r.fulfill({json:{data:{items:[],total:0}}}));await page.route('**/api/v1/player-access/users',r=>r.fulfill({json:{data:[{id:'verified-id',name:'已核验用户'}]}}))
 const seen:string[]=[];await page.route('**/api/v1/player-access/events?*',r=>{seen.push(new URL(r.request().url()).searchParams.get('embyUserId')??'');return r.fulfill({json:{data:{items:[{id:'visit',userAgent:'Owned Player',clientIp:'8.8.8.8',connectionIp:'10.0.0.2',ipSource:'trusted_proxy',ipPublic:true,entry:'emby',method:'GET',status:302,embyVerified:true,embyUserId:'verified-id',embyUserName:'已核验用户',createdAt:'2026-10-14T00:00:00Z'}],total:1}}})})
 await page.goto('/player-access');await expect(page.getByText(/公网访客IP 8\.8\.8\.8/)).toBeVisible();await expect(page.getByText('连接IP 10.0.0.2',{exact:true})).toBeVisible()
 await page.getByRole('combobox',{name:'按Emby用户筛选',exact:true}).press('ArrowDown');await page.getByRole('option',{name:'已核验用户 · verified-id',exact:true}).click();await expect.poll(()=>seen.at(-1)).toBe('verified-id');await page.screenshot({path:resolve(screenshots,'public-ip-user-filter.png'),fullPage:true})
})

test('cloud index bulk organizer echoes saved identity, templates and frozen file proof (fixture)',async({page})=>{
 await fixtureLogin(page)
 const candidateId='00000000-0000-4000-8000-000000000101',assetId='00000000-0000-4000-8000-000000000102',rootId='00000000-0000-4000-8000-000000000103',batchId='owned-batch'
 const source='Show.2020.S01E01.1080p.mkv'
 await page.route('**/api/v1/library/candidates?*',r=>r.fulfill({json:{data:{items:[{id:candidateId,name:source,path:'/115/'+source,size:5,state:'needs_confirmation',revision:1,cloudState:'present'}],total:1}}}));await page.route('**/api/v1/media/assets?*',r=>r.fulfill({json:{data:{items:[],total:0}}}));for(const path of ['scans','schedules'])await page.route(`**/api/v1/library/${path}`,r=>r.fulfill({json:{data:[]}}))
 let selected:Record<string,unknown>|undefined,preview:Record<string,any>|undefined
 await page.route('**/api/v1/library/organize-selection',r=>{selected=r.request().postDataJSON();return r.fulfill({json:{data:{root:{id:rootId,name:'115测试',kind:'cloud',path:'/115',revision:1},files:[{id:'provider',name:source,path:source,size:5,directory:false}],rows:[{source,title:'已确认剧集',year:'2026',mediaType:'series',tmdbId:123,season:0,episode:2,episodes:[2],resolution:'1080p',confirmed:true,warnings:[]}]}}})})
 await page.route('**/api/v1/files/batches/preview',r=>{preview=r.request().postDataJSON();return r.fulfill({json:{data:{id:batchId,state:'preview',digest:'d',expiresAt:'2099-01-01T00:00:00Z',items:[{ordinal:0,source,state:'preview',result:{id:'op',state:'preview',completedSteps:0,plan:{items:[{source:{path:source},target:'电视剧/已确认剧集 (2026)/Season 00/已确认剧集 - S00E02.mkv'}],steps:[]}}}]}}})})
 await page.clock.install();await page.goto('/cloudStorage');await page.getByRole('row').filter({hasText:'/115/'+source}).locator('.el-checkbox').click();await expect(page.getByRole('button',{name:'批量整理115 · 1',exact:true})).toBeEnabled();await page.clock.fastForward(11000);await expect(page.getByRole('button',{name:'批量整理115 · 1',exact:true})).toBeEnabled();await page.getByRole('button',{name:'批量整理115 · 1',exact:true}).click()
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
 await page.goto('/cloudStorage');await expect(page.getByRole('button',{name:'刷新记录',exact:true})).toBeEnabled();await page.getByRole('tab',{name:'已入库资产',exact:true}).click()
 await page.clock.fastForward(11000);await expect.poll(()=>reads).toBe(2)
 await page.getByRole('row').filter({hasText:asset.path}).locator('.el-checkbox').click();await expect(page.getByRole('button',{name:'批量整理115 · 1',exact:true})).toBeEnabled()
 release();await expect(page.getByRole('button',{name:'刷新记录',exact:true})).toBeEnabled();await expect(page.getByRole('button',{name:'批量整理115 · 1',exact:true})).toBeEnabled()
 await page.clock.fastForward(21000);await expect(page.getByRole('button',{name:'批量整理115 · 1',exact:true})).toBeEnabled();expect(reads).toBe(2)
})

test('directory classification has ordered rules and responsive editing (fixture)',async({page})=>{
 await fixtureLogin(page)
 const value={groups:[{id:'movies',name:'电影',directory:'电影',mediaType:'movie',genreIds:[],enabled:true,rules:[{id:'cn',name:'华语电影',directory:'华语电影',enabled:true,genreIds:[],countries:[],languages:['zh']},{id:'other',name:'其他电影',directory:'其他电影',enabled:true,genreIds:[],countries:[],languages:[]}]}]}
 let saved:Record<string,any>|undefined;await page.route('**/api/v1/classification',r=>{if(r.request().method()==='PUT'){saved=r.request().postDataJSON();return r.fulfill({json:{data:saved}})}return r.fulfill({json:{data:{value,revision:1}}})})
 await page.goto('/classification');await expect(page.getByRole('heading',{name:'目录分类',exact:true})).toBeVisible();await page.getByRole('button',{name:'向上移动其他电影',exact:true}).click();await page.getByRole('button',{name:'保存规则',exact:true}).click();await expect.poll(()=>saved?.value.groups[0].rules[0].id).toBe('other')
 await page.getByRole('button',{name:'为电影添加规则',exact:true}).click();const d=page.getByRole('dialog',{name:'子分类规则'});await d.getByLabel('分类名称').fill('纪录片');await d.getByLabel('分类目录').fill('纪录片');await d.getByRole('button',{name:'应用到待保存规则'}).click();await expect(page.getByText('电影/纪录片',{exact:true})).toBeVisible()
 for(const width of [1440,375]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`classification-${width}.png`),fullPage:true,animations:'disabled'})}
})

test('external API has independent scopes, real status verification and revocation (isolated)',async({page})=>{
 await fixtureLogin(page);await page.goto('/external-api');await expect(page.getByRole('heading',{name:'外部API',exact:true})).toBeVisible()
 const response=await page.request.post('/api/v1/external-api/keys',{headers:{'X-Requested-With':'media-tool'},data:{name:'Owned isolated API',scopes:['status.read'],confirm:true}});expect(response.status()).toBe(201);const key=(await response.json()).data;expect(key.expiresAt).toBeNull();await expect(page.getByText('有效天数',{exact:true})).toHaveCount(0)
 const status=await page.request.get('/api/external/v1/status',{headers:{Authorization:'Bearer '+key.token}});expect(status.status()).toBe(200);expect((await status.json()).data.version).toBe('0.27.0-dev')
 expect((await page.request.get('/api/external/v1/downloads',{headers:{Authorization:'Bearer '+key.token}})).status()).toBe(403);expect((await page.request.get('/api/external/v1/status')).status()).toBe(401)
 await page.getByRole('button',{name:'刷新记录',exact:true}).click();await expect(page.getByLabel('完整API凭据')).toHaveValue(key.token)
 for(const width of [1440,375]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`external-api-${width}.png`),fullPage:true,animations:'disabled',mask:[page.getByLabel('完整API凭据')]})}
 expect((await page.request.post(`/api/v1/external-api/keys/${key.id}/revoke`,{headers:{'X-Requested-With':'media-tool'},data:{confirm:true}})).status()).toBe(200);expect((await page.request.get('/api/external/v1/status',{headers:{Authorization:'Bearer '+key.token}})).status()).toBe(401)
})

test('STRM directory filters before pagination and previews original content without manual Emby refresh (fixture)',async({page})=>{
 await fixtureLogin(page);let facet='';const asset='00000000-0000-4000-8000-000000000120'
 await page.route('**/api/v1/library/outputs?*',r=>{facet=new URL(r.request().url()).searchParams.get('mediaType')??'';return r.fulfill({json:{data:{items:[{id:asset,title:'自有媒体',year:'2026',mediaType:'movie',source:'/115/owned.mkv',outputs:[{kind:'strm',path:'电影/华语电影/自有媒体 (2026)/自有媒体 (2026) - 1080p.strm'}],embyState:'pending',cloudState:'present',artifactsState:'ready'}],total:1}}})})
 await page.route(`**/api/v1/library/outputs/${asset}/content?*`,r=>r.fulfill({json:{data:{kind:'strm',path:'owned.strm',digest:'fixture-digest',content:'https://owned.fixture/api/v1/cloudStorage/strm302?apiKey=owned-key\n'}}}))
 await page.goto('/strm');await page.getByRole('combobox',{name:'STRM媒体类型'}).press('ArrowDown');await page.getByRole('option',{name:'电影',exact:true}).click();await expect.poll(()=>facet).toBe('movie');await expect(page.getByRole('button',{name:'定向刷新并确认Emby',exact:true})).toHaveCount(0)
 await page.getByRole('button',{name:'预览STRM内容',exact:true}).click();await expect(page.getByLabel('托管输出原始内容')).toHaveValue('https://owned.fixture/api/v1/cloudStorage/strm302?apiKey=owned-key\n');await page.screenshot({path:resolve(screenshots,'strm-original-content.png'),fullPage:true,animations:'disabled'})
})

test('verified instant upload shows processing completion without inventing sent bytes (fixture)',async({page})=>{
 await fixtureLogin(page);await page.route('**/api/v1/downloads',r=>r.fulfill({json:{data:[]}}));await page.route('**/api/v1/transfers',r=>r.fulfill({json:{data:[{id:'owned',destination:'/115/电影/自有影片.mkv',state:'imported',phase:'verified',bytes:279011,hashBytes:7960117968,total:7960117968,processingProgress:100,processingStage:'入库完成',transferMethod:'instant_or_reused'}]}}));await page.goto('/downloads');await expect(page.getByText('入库完成',{exact:true})).toBeVisible();await expect(page.getByText('秒传或复用；节省字节不计作网络传输',{exact:true})).toBeVisible();await expect(page.getByRole('progressbar')).toHaveAttribute('aria-valuenow','100')
})

test('classification real defaults render without null collection failures (isolated)',async({page})=>{
 await fixtureLogin(page);const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await page.goto('/classification');await expect(page.getByRole('heading',{name:'目录分类',exact:true})).toBeVisible();for(const name of ['动漫','电影','电视剧'])await expect(page.getByRole('rowheader',{name,exact:true})).toBeVisible();await expect(page.getByText('电影/日韩电影',{exact:true})).toBeVisible();expect(errors).toEqual([])
 for(const width of [1440,375]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`classification-real-${width}.png`),fullPage:true,animations:'disabled'})}
})

test('classification checkbox editor preserves original countries and genre IDs (fixture)',async({page})=>{
 await fixtureLogin(page)
 const value={groups:[{id:'movies',name:'电影',directory:'电影',mediaType:'movie',genreIds:[],enabled:true,rules:[{id:'owned',name:'旧值回显',directory:'自定义',enabled:true,genreIds:[999999],countries:['ZZ','CN'],languages:['zh']}]}]}
 let saved:any;await page.route('**/api/v1/classification',r=>{if(r.request().method()==='PUT'){saved=r.request().postDataJSON();return r.fulfill({json:{data:saved}})}return r.fulfill({json:{data:{value,revision:1}}})})
 await page.goto('/classification');await page.getByRole('button',{name:'编辑旧值回显',exact:true}).click();const dialog=page.getByRole('dialog',{name:'子分类规则'})
 await expect(dialog.getByRole('checkbox',{name:'已保存地区 ZZ ZZ',exact:true})).toBeChecked();await expect(dialog.getByRole('checkbox',{name:'中国 CN',exact:true})).toBeChecked();await expect(dialog.getByRole('checkbox',{name:'已保存题材 999999 999999',exact:true})).toBeChecked()
 await dialog.getByRole('checkbox',{name:'纪录片 99',exact:true}).check();await dialog.getByLabel('筛选国家地区').fill('日本');await dialog.getByRole('checkbox',{name:'日本 JP',exact:true}).check();await expect(dialog.getByRole('checkbox',{name:'汉语（普通话） zh',exact:true})).toBeChecked()
 for(const width of [1440,375]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`classification-checkbox-${width}.png`),animations:'disabled'})}
 await dialog.getByRole('button',{name:'应用到待保存规则'}).click();await expect(dialog).not.toBeVisible();await page.getByRole('button',{name:'保存规则',exact:true}).click();await expect.poll(()=>saved?.value.groups[0].rules[0].countries).toEqual(['ZZ','CN','JP']);expect(saved.value.groups[0].rules[0].genreIds).toEqual([999999,99]);expect(saved.value.groups[0].rules[0].languages).toEqual(['zh'])
})

test('system operations has real bounded health, paginated tasks and details (isolated)',async({page})=>{
 await fixtureLogin(page);await page.goto('/jobs');await expect(page.getByRole('heading',{name:'系统运维与后台任务',exact:true})).toBeVisible();await expect(page.getByRole('heading',{name:'Cron 调度',exact:true})).toBeVisible();await expect(page.getByText('连接正常',{exact:true})).toHaveCount(2)
 const health=await page.request.get('/api/v1/system/operations');expect(health.status()).toBe(200);expect(health.headers()['cache-control']).toBe('no-store');const snapshot=(await health.json()).data;expect(snapshot.migration).toBe(expectedSchema);expect(snapshot.unavailable).toEqual([]);expect(snapshot.workers.some((w:any)=>w.online)).toBe(true);expect(snapshot.schedules.some((s:any)=>s.id==='core'&&s.cron==='*/15 * * * * *')).toBe(true)
 const response=await page.request.post('/api/v1/jobs',{headers:{'X-Requested-With':'media-tool'},data:{kind:'system.check'}});expect(response.status()).toBe(201);const job=(await response.json()).data;await page.getByLabel('任务关键词').fill(job.id);await page.getByRole('button',{name:'筛选任务',exact:true}).click();await expect(page.getByRole('button',{name:`查看任务 ${job.id}`,exact:true})).toBeVisible();await page.getByRole('button',{name:`查看任务 ${job.id}`,exact:true}).click();const dialog=page.getByRole('dialog',{name:'后台任务详情',exact:true});await expect(dialog).toContainText(job.id);await expect(dialog).toContainText('不提供通用强制重放');await dialog.getByRole('button',{name:'关闭此对话框'}).click();await expect(dialog).not.toBeVisible()
 expect((await page.request.get('/api/v1/jobs/browse?page=0')).status()).toBe(400);expect((await page.request.get('/api/v1/jobs/not-a-uuid')).status()).toBe(400);const empty=await page.request.get(`/api/v1/jobs/browse?page=2&pageSize=25&q=${job.id}`);expect((await empty.json()).data).toEqual({items:[],total:1})
 for(const width of [1440,375]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`system-operations-${width}.png`),fullPage:true,animations:'disabled'})}
})

test('history entries separate media paths time and safe actions at mobile widths (fixture)',async({page})=>{
 await fixtureLogin(page);const item={id:'owned-history-ui',kind:'transfer',state:'imported',source:'/data/下载目录/Owned.Film.2026.1080p.WEB-DL.mkv',destination:'/115/电影/华语电影/自有影片 (2026)/自有影片 (2026) - 1080p.mkv',origin:'download',method:'transfer',batchId:'owned-batch',media:{title:'自有影片',year:'2026',mediaType:'movie'},category:'华语电影',message:'云端校验、STRM输出均完成，保留做种源',updatedAt:'2026-10-15T01:00:00Z',createdAt:'2026-10-15T00:00:00Z',details:{phase:'verified',bytesTransferred:5,size:5}}
 await page.route('**/api/v1/library/activity?*',r=>r.fulfill({json:{data:{items:[item],total:1}}}));await page.goto('/organize-history');const row=page.locator('.history-row');await expect(row.locator('time')).toHaveCount(1);await expect(row.locator('.history-media strong')).toHaveText('自有影片 (2026)');await expect(row.locator('.history-files')).toContainText(item.source);await expect(row.locator('.history-files')).toContainText(item.destination)
 for(const width of [1440,768,375]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await row.locator('summary').click();await expect(row.getByRole('button',{name:'查看步骤详情',exact:true})).toBeVisible();await page.screenshot({path:resolve(screenshots,`history-compact-${width}.png`),fullPage:true,animations:'disabled'});await row.locator('summary').click()}
 await row.locator('summary').click();await row.getByRole('button',{name:'查看步骤详情',exact:true}).click();await expect(page.getByRole('dialog',{name:'步骤证据与恢复',exact:true})).toContainText('源SHA1')
})

test('classification is one semantic list with shared columns, empty groups and contained mobile scrolling (fixture)',async({page})=>{
 await fixtureLogin(page)
 const value={groups:[{id:'movies-list',name:'列表电影',directory:'电影',mediaType:'movie',genreIds:[99],enabled:false,rules:[{id:'cn-list',name:'华语纪录',directory:'华语纪录',enabled:true,genreIds:[18],countries:['CN','ZZ'],languages:['zh']},{id:'other-list',name:'其他纪录',directory:'其他纪录',enabled:false,genreIds:[],countries:[],languages:[]}]},{id:'series-list',name:'列表剧集',directory:'电视剧',mediaType:'series',genreIds:[],enabled:true,rules:[]}]}
 await page.route('**/api/v1/classification',r=>r.fulfill({json:{data:{value,revision:7}}}))
 await page.goto('/classification');const table=page.getByRole('table',{name:'目录分类规则列表',exact:true});await expect(table).toBeVisible();await expect(page.locator('.classification-list-panel')).toHaveCount(1);await expect(page.locator('.category-group,.category-rule,.classification-grid')).toHaveCount(0)
 await expect(table.getByRole('columnheader')).toHaveText(['分类名称','媒体类型','匹配条件','目标目录','状态','操作']);await expect(table.locator('.classification-group-row')).toHaveCount(2);await expect(table.locator('.classification-rule-row')).toHaveCount(2);await expect(table.getByRole('rowheader',{name:'华语纪录',exact:true})).toBeVisible();await expect(table.getByText('中国 · CN',{exact:true})).toBeVisible();await expect(table.getByText('已保存地区 · ZZ',{exact:true})).toBeVisible();await expect(table.getByText('电影/华语纪录',{exact:true})).toBeVisible();await expect(table.locator('.classification-rule-row').first()).toContainText('组已停用');await expect(table.locator('.classification-rule-row').first()).toContainText('规则启用');await expect(table.locator('.classification-empty-group')).toContainText('暂无子规则');await expect(page.getByText('2 个分类组 · 2 条子规则',{exact:true})).toBeVisible()
 for(const width of [1440,768,375]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`classification-list-${width}.png`),fullPage:true,animations:'disabled'})}
 const region=page.getByRole('region',{name:'目录分类列表横向滚动区',exact:true});expect(await region.evaluate(el=>el.scrollWidth>el.clientWidth)).toBe(true);await region.evaluate(el=>{el.scrollLeft=el.scrollWidth});await expect(table.getByRole('button',{name:'为列表剧集添加规则',exact:true})).toBeInViewport();const touch=await table.getByRole('button',{name:'为列表剧集添加规则',exact:true}).boundingBox();expect(touch!.height).toBeGreaterThanOrEqual(44);await page.screenshot({path:resolve(screenshots,'classification-list-mobile-actions.png'),fullPage:true,animations:'disabled'})
})

test('classification empty list displays an honest empty state without group cards (fixture)',async({page})=>{
 await fixtureLogin(page);await page.route('**/api/v1/classification',r=>r.fulfill({json:{data:{value:{groups:[]},revision:1}}}));await page.goto('/classification');await expect(page.getByRole('table',{name:'目录分类规则列表',exact:true})).toBeVisible();await expect(page.getByText('尚无分类规则，请添加分类组',{exact:true})).toBeVisible();await expect(page.locator('.classification-group-row')).toHaveCount(0);await expect(page.getByRole('button',{name:'添加分类组',exact:true})).toBeEnabled()
})

test('maintenance center preserves policy editing and contains desktop/mobile tables (fixture)',async({page})=>{
 await fixtureLogin(page)
 const policy={backupEnabled:false,backupCron:'0 3 * * *',keepDays:14,keepCount:10,sampleDays:14,alertsEnabled:true,memoryPercent:90,diskFreeGB:5,loadPerCPU:2,failedJobs:3,consecutive:3,cooldownMinutes:60}
 const data:any={policy:{value:policy,revision:1,nextBackupAt:'2026-10-16T03:00:00+08:00'},backups:[],drills:[],requests:[],releases:[],alerts:[],mode:{paused:false,reason:'',revision:1},toolsReady:true,totalBytes:0,revision:'a'.repeat(40),version:'0.27.0-dev',schema:50}
 let saved:any;await page.route('**/api/v1/ops',r=>r.fulfill({json:{data}}));await page.route('**/api/v1/ops/trends?*',r=>r.fulfill({json:{data:[]}}));await page.route('**/api/v1/ops/policy',r=>{saved=r.request().postDataJSON();data.policy={...saved,revision:2};return r.fulfill({json:{data:data.policy}})})
 await page.goto('/maintenance');await expect(page.getByRole('heading',{name:'备份、恢复与运维',exact:true})).toBeVisible();await expect(page.getByText('尚无系统备份',{exact:true})).toBeVisible();await page.getByRole('button',{name:'维护策略',exact:true}).click();await page.locator('.el-switch').filter({has:page.getByRole('switch',{name:'启用定时备份',exact:true})}).click();await expect(page.getByRole('switch',{name:'启用定时备份',exact:true})).toBeChecked();await page.getByLabel('备份Cron',{exact:true}).fill('0 4 * * *');await page.getByRole('button',{name:'保存维护策略',exact:true}).click();await expect.poll(()=>saved?.value.backupCron).toBe('0 4 * * *');expect(saved.value.backupEnabled).toBe(true);expect(saved.revision).toBe(1);expect(saved.value.keepCount).toBe(10)
 for(const width of [1440,768,375]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`maintenance-policy-${width}.png`),fullPage:true,animations:'disabled'})}
 await page.getByRole('button',{name:'运行趋势',exact:true}).click();await expect(page.getByText('暂无有效采样',{exact:true})).toHaveCount(9);await expect(page.locator('.trend-grid').getByRole('img')).toHaveCount(0)
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
 await page.route('**/api/v1/ops',r=>r.fulfill({json:{data}}));await page.route('**/api/v1/ops/trends?*',r=>r.fulfill({json:{data:[]}}));await page.route('**/api/v1/ops/requests/preview',r=>{preview=r.request().postDataJSON();return r.fulfill({json:{data:{request:ticket,confirmText:phrase,command:'python3 scripts/ops-owner.py --root /root/docker/mediatool execute '+id}}})});await page.route(`**/api/v1/ops/requests/${id}/approve`,r=>{approved=r.request().postDataJSON();data.requests=[{...ticket,state:'approved',message:'已审批，等待宿主机受控执行；当前尚未恢复或回滚'}];return r.fulfill({json:{data:{state:'approved'}}})})
 await page.goto('/maintenance');await page.getByRole('button',{name:'恢复与回滚',exact:true}).click();await page.getByRole('combobox',{name:'选择待恢复备份',exact:true}).press('ArrowDown');await expect(page.getByRole('combobox',{name:'选择待恢复备份',exact:true})).toHaveAttribute('aria-expanded','true');await page.getByRole('option').click();await page.getByRole('button',{name:'预览恢复',exact:true}).click();const dialog=page.getByRole('dialog',{name:'确认维护审批',exact:true});await expect(dialog).toBeVisible();expect(preview).toEqual({kind:'restore',id:backup});await expect(dialog.getByRole('button',{name:'审批维护单（不立即执行）',exact:true})).toBeDisabled();await dialog.getByLabel('维护确认短语').fill(phrase);await dialog.getByRole('button',{name:'审批维护单（不立即执行）',exact:true}).click();await expect(dialog).not.toBeVisible();expect(approved).toEqual({confirm:true,digest,text:phrase});await expect(page.getByText('已审批，等待宿主机受控执行；当前尚未恢复或回滚',{exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'立即执行恢复',exact:true})).toHaveCount(0);await page.setViewportSize({width:375,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,'maintenance-recovery-mobile.png'),fullPage:true,animations:'disabled'})
})

test('maintenance trends use real sample values and alert acknowledgment is explicit (fixture)',async({page})=>{
 await fixtureLogin(page);const data:any={policy:{value:{backupEnabled:false},revision:1,nextBackupAt:'2026-10-16T00:00:00Z'},backups:[],drills:[],requests:[],releases:[],alerts:[{code:'memory',state:'active',hits:3,value:95,acknowledged_at:null,message:'宿主内存使用率过高',last_seen_at:'2026-10-15T00:15:00Z'}],mode:{paused:false,reason:'',revision:1},toolsReady:true,totalBytes:0,revision:'a'.repeat(40),schema:50};await page.route('**/api/v1/ops',r=>r.fulfill({json:{data}}));await page.route('**/api/v1/ops/trends?*',r=>r.fulfill({json:{data:[{at:'2026-10-15T00:00:00Z',memoryPercent:92,diskFreeGB:20,load1:1,loadPerCPU:0.5,processBytes:10485760,jobSeconds:2,pendingJobs:0,failedJobs10m:0,succeededJobs10m:2},{at:'2026-10-15T00:15:00Z',memoryPercent:95,diskFreeGB:19,load1:2,loadPerCPU:1,processBytes:20971520,jobSeconds:3,pendingJobs:0,failedJobs10m:0,succeededJobs10m:3}]}}));let ack:any;await page.route('**/api/v1/ops/alerts/memory/ack',r=>{ack=r.request().postDataJSON();data.alerts[0].acknowledged_at='2026-10-15T00:16:00Z';return r.fulfill({json:{data:{acknowledged:true}}})})
 await page.goto('/maintenance');await page.getByRole('button',{name:'运维告警',exact:true}).click();await page.getByRole('button',{name:'确认告警',exact:true}).click();await expect.poll(()=>ack?.confirm).toBe(true);await expect(page.getByText('已确认',{exact:true})).toBeVisible();await page.getByRole('button',{name:'运行趋势',exact:true}).click();await expect(page.getByRole('img',{name:/内存使用率趋势/})).toBeVisible();const zero=page.locator('.metric-trend').filter({has:page.getByRole('heading',{name:'十分钟失败任务 条',exact:true})});await expect(zero).toContainText('最大 0.00');await expect(page.locator('.metric-trend')).toHaveCount(9)
 for(const width of [1440,375]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`maintenance-trends-${width}.png`),fullPage:true,animations:'disabled'})}
})


test('classification choices pin selections and filter Chinese genres countries and languages without losing saved values (fixture)',async({page})=>{
 await fixtureLogin(page)
 const value={groups:[{id:'choice-movies',name:'电影',directory:'电影',mediaType:'movie',genreIds:[],enabled:true,rules:[{id:'choice-owned',name:'选项筛选',directory:'筛选',enabled:true,genreIds:[99],countries:['US','CN','ZZ'],languages:['zh','yue','xyz']}]}]}
 let saved:any;await page.route('**/api/v1/classification',r=>{if(r.request().method()==='PUT'){saved=r.request().postDataJSON();return r.fulfill({json:{data:saved}})}return r.fulfill({json:{data:{value,revision:1}}})})
 await page.goto('/classification');await page.getByRole('button',{name:'编辑选项筛选',exact:true}).click();const dialog=page.getByRole('dialog',{name:'子分类规则'})
 const genre=dialog.locator('[data-choice="题材"]'),country=dialog.locator('[data-choice="国家地区"]'),language=dialog.locator('[data-choice="原始语言"]')
 await expect(language.getByRole('checkbox',{name:'阿法尔语 aa',exact:true})).toHaveCount(1)
 for(const [picker,count]of [[genre,1],[country,3],[language,3]]as const){expect(await picker.locator('.choice-option input').evaluateAll((inputs,n)=>inputs.slice(0,n).every(input=>(input as HTMLInputElement).checked)&&inputs.slice(n).every(input=>!(input as HTMLInputElement).checked),count)).toBe(true)}
 await genre.getByLabel('筛选题材').fill('动画');await genre.getByRole('checkbox',{name:'动画 16',exact:true}).check();await genre.getByLabel('筛选题材').fill('');await expect(genre.locator('.choice-option input').nth(1)).toBeChecked()
 await country.getByLabel('筛选国家地区').fill('日本');await country.getByRole('checkbox',{name:'日本 JP',exact:true}).check();await expect(country.locator('.selection-summary')).toContainText('中国 (CN)');await country.getByLabel('筛选国家地区').fill(' us ');await expect(country.getByRole('checkbox')).toHaveCount(1);await expect(country.getByRole('checkbox',{name:'美国 US',exact:true})).toBeChecked();await country.getByLabel('筛选国家地区').fill('')
 await expect(language.getByRole('checkbox',{name:'粤语 yue',exact:true})).toBeChecked();await language.getByLabel('筛选原始语言').fill('英语');await language.getByRole('checkbox',{name:'英语 en',exact:true}).check();await language.getByLabel('筛选原始语言').fill('不存在的语言');await expect(language.getByRole('status')).toContainText('没有匹配');await expect(language.locator('.selection-summary')).toContainText('已保存语言 xyz (xyz)');await language.getByLabel('筛选原始语言').fill('CN');await expect(language.getByRole('checkbox',{name:'粤语 cn',exact:true})).toBeVisible();await language.getByLabel('筛选原始语言').fill('')
 await language.getByRole('checkbox',{name:'英语 en',exact:true}).focus();await page.keyboard.press('Space');await expect(language.getByRole('checkbox',{name:'英语 en',exact:true})).not.toBeChecked();await expect(language.getByRole('checkbox',{name:'英语 en',exact:true})).toBeFocused();await page.keyboard.press('Space');await expect(language.getByRole('checkbox',{name:'英语 en',exact:true})).toBeChecked();await expect(language.getByRole('checkbox',{name:'英语 en',exact:true})).toBeFocused()
 await dialog.getByRole('button',{name:'应用到待保存规则'}).click();await page.getByRole('button',{name:'保存规则',exact:true}).click();await expect.poll(()=>saved?.value.groups[0].rules[0].languages).toEqual(['zh','yue','xyz','en']);expect(saved.value.groups[0].rules[0].countries).toEqual(['US','CN','ZZ','JP']);expect(saved.value.groups[0].rules[0].genreIds).toEqual([99,16])
 await page.getByRole('button',{name:'编辑选项筛选',exact:true}).click();for(const label of ['筛选题材','筛选国家地区','筛选原始语言'])await expect(dialog.getByLabel(label,{exact:true})).toHaveValue('');await expect(dialog.getByRole('checkbox',{name:'英语 en',exact:true})).toBeChecked()
})

test('classification long country labels wrap inside their own choices on desktop and mobile (fixture)',async({page})=>{
 await fixtureLogin(page);const value={groups:[{id:'wrap-movies',name:'电影',directory:'电影',mediaType:'movie',genreIds:[],enabled:true,rules:[{id:'wrap-rule',name:'长名称测试',directory:'测试',enabled:true,genreIds:[],countries:['GS','IO','UM'],languages:['zh']}]}]}
 await page.route('**/api/v1/classification',r=>r.fulfill({json:{data:{value,revision:1}}}));await page.goto('/classification');await page.getByRole('button',{name:'编辑长名称测试',exact:true}).click();const dialog=page.getByRole('dialog',{name:'子分类规则'}),country=dialog.locator('[data-choice="国家地区"]')
 for(const width of [1440,768,375,320]){
  await page.setViewportSize({width,height:950});await expect(dialog).toBeVisible();expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  const bounds=await country.locator('.choice-option').evaluateAll(labels=>labels.map(label=>{const rect=label.getBoundingClientRect(),text=label.querySelector('.choice-label')!,box=label.querySelector('input')!;const range=document.createRange();range.selectNodeContents(text);return{width:label.clientWidth,scroll:label.scrollWidth,height:rect.height,boxWidth:box.getBoundingClientRect().width,contained:[...range.getClientRects()].every(r=>r.left>=rect.left-1&&r.right<=rect.right+1&&r.top>=rect.top-1&&r.bottom<=rect.bottom+1)}}))
  expect(bounds.length).toBeGreaterThan(240);for(const bound of bounds){expect(bound.scroll).toBeLessThanOrEqual(bound.width+1);expect(bound.height).toBeGreaterThanOrEqual(43.99);expect(bound.boxWidth).toBeCloseTo(16, 2);expect(bound.contained).toBe(true)}
  expect(await country.locator('.choice-options').evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true);await country.scrollIntoViewIfNeeded();await page.screenshot({path:resolve(screenshots,`classification-choice-wrap-${width}.png`),animations:'disabled'})
 }
})

test('settings functional pages are consistent and preserve drafts while switching tabs (isolated)',async({page})=>{
 await fixtureLogin(page);await page.goto('/settings');const nav=page.getByRole('navigation',{name:'系统设置分类'});await expect(nav).toBeVisible()
 for(const width of [1440,768,375]){await page.setViewportSize({width,height:950});for(const tab of ['访问与域名','命名与刮削','搜索与订阅','网络代理','TMDB','管理员账号','移动端应用']){await nav.getByRole('button',{name:tab,exact:true}).click();await expect(nav.getByRole('button',{name:tab,exact:true})).toHaveAttribute('aria-pressed','true');expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`settings-${tab}-${width}.png`),animations:'disabled'})}}
 await nav.getByRole('button',{name:'命名与刮削',exact:true}).click();await page.getByLabel('电影命名模板',{exact:true}).fill('{title} ({year})/{title} - draft');await nav.getByRole('button',{name:'网络代理',exact:true}).click();await nav.getByRole('button',{name:'命名与刮削',exact:true}).click();await expect(page.getByLabel('电影命名模板',{exact:true})).toHaveValue('{title} ({year})/{title} - draft')
 await nav.getByRole('button',{name:'TMDB',exact:true}).click();await expect(page.getByRole('columnheader',{name:'连接验证',exact:true})).toHaveCount(0)
})

test('domain settings validate origins and update generated STRM without touching old output (isolated)',async({page})=>{
 await fixtureLogin(page);await page.goto('/settings?tab=access');const initial=(await(await page.request.get('/api/v1/settings/access')).json()).data
 try{
  await page.getByLabel('后台访问域名',{exact:true}).fill('https://admin.fixture.invalid');await page.getByLabel('STRM播放域名',{exact:true}).fill('https://play.fixture.invalid');await page.getByLabel('内置媒体库域名',{exact:true}).fill('https://member.fixture.invalid');await page.getByRole('button',{name:'保存域名设置',exact:true}).click();await expect(page.getByText(/域名已保存，新生成链接立即使用新地址/)).toBeVisible()
  const current=(await(await page.request.get('/api/v1/settings/access')).json()).data;expect(current.effective.memberUrl).toBe('https://member.fixture.invalid')
  const bad=await page.request.put('/api/v1/settings/access',{headers:{'X-Requested-With':'media-tool'},data:{value:{...current.value,managementUrl:'https://example.com/watch'},revision:current.revision}});expect(bad.status()).toBe(400)
  const preview=await page.request.post('/api/v1/strm/preview',{headers:{'X-Requested-With':'media-tool'},data:{path:((await(await page.request.get('/api/v1/system/status')).json()).data.allowedRoots[0]||'/115')+'/Owned.mkv'}});expect(preview.status()).toBe(200);expect(JSON.stringify((await preview.json()).data)).toContain('https://play.fixture.invalid')
 }finally{const current=(await(await page.request.get('/api/v1/settings/access')).json()).data;expect((await page.request.put('/api/v1/settings/access',{headers:{'X-Requested-With':'media-tool'},data:{value:initial.value,revision:current.revision}})).status()).toBe(200)}
})

test('manual torrent imports support file and magnet input with persistent status and honest unknown progress (fixture)',async({page})=>{
 await fixtureLogin(page);let imports:any[]=[];await page.route('**/api/v1/downloads/manual',async route=>{if(route.request().method()==='POST'){imports.push({contentType:route.request().headers()['content-type'],body:route.request().postData()});return route.fulfill({status:202,json:{data:{id:'fixture-manual'}}})}return route.fulfill({json:{data:[{id:'fixture-manual',title:'Owned Manual.mkv',sourceKind:'file',source:'Owned.torrent',infoHash:'a'.repeat(40),state:'downloading',progress:null,qbState:'',downloadSpeed:0,savePath:'/downloads',endpoint:'http://qb.fixture:8080',category:'media-tool',message:'等待同步',createdAt:'2026-10-15T10:00:00Z'}]}})})
 await page.goto('/downloads');await expect(page.getByRole('heading',{name:'手动种子下载',exact:true})).toBeVisible();await expect(page.getByText('进度待下载器同步，不以0%代替未知进度。',{exact:true})).toBeVisible();await page.getByRole('button',{name:'添加种子',exact:true}).click();const dialog=page.getByRole('dialog',{name:'添加种子下载',exact:true});await dialog.getByLabel('种子文件（.torrent）',{exact:true}).setInputFiles({name:'Owned.torrent',mimeType:'application/x-bittorrent',buffer:Buffer.from('owned-file-fixture')});await dialog.getByRole('button',{name:'确认添加下载',exact:true}).click();await expect(dialog).toBeHidden();expect(imports[0].contentType).toContain('multipart/form-data');expect(imports[0].body).toContain('Owned.torrent')
 await page.getByRole('button',{name:'添加种子',exact:true}).click();await dialog.getByText('种子链接',{exact:true}).click();const magnet='magnet:?xt=urn:btih:'+'a'.repeat(40);await dialog.getByLabel('种子链接或磁力链接',{exact:true}).fill(magnet);await dialog.getByRole('button',{name:'确认添加下载',exact:true}).click();await expect(dialog).toBeHidden();expect(JSON.parse(imports[1].body).url).toBe(magnet)
 for(const width of [1440,375]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`manual-downloads-${width}.png`),fullPage:true,animations:'disabled'})}
})

test('operations panels have real gaps and no repeated cross-module shortcuts (isolated)',async({page})=>{
 await fixtureLogin(page);await page.goto('/jobs');await expect(page.locator('.ops-metrics')).toBeVisible()
 for(const width of [1440,375]){await page.setViewportSize({width,height:950});const gap=await page.locator('.ops-metrics').evaluate(el=>{const next=el.nextElementSibling!;return next.getBoundingClientRect().top-el.getBoundingClientRect().bottom});expect(gap).toBeGreaterThanOrEqual(width===1440?23.5:15.5);expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`ops-unified-${width}.png`),fullPage:true,animations:'disabled'})}
 await expect(page.locator('.ops-links')).toHaveCount(0);await expect(page.getByRole('link',{name:'备份与维护中心',exact:true})).toHaveCount(1);await page.getByRole('link',{name:'备份与维护中心',exact:true}).click();await expect(page.getByRole('link',{name:'任务与运行状态',exact:true})).toHaveCount(0);await expect(page.getByRole('button',{name:'刷新',exact:true})).toHaveCount(1)
})

test('TMDB settings exposes only one full Token and updates it in place (fixture)',async({page})=>{
 await fixtureLogin(page);const token='owned-full-token-for-TMDB-echo';const row={id:'owned-tmdb-single',kind:'tmdb',name:'TMDB',enabled:true,settings:{language:'zh-CN'},secretFields:['token'],revision:2};let saved:any
 await page.route('**/api/v1/integrations',route=>route.fulfill({json:{data:[row]}}));await page.route('**/api/v1/integrations/owned-tmdb-single/secrets/reveal',route=>route.fulfill({json:{data:{revision:2,values:{token}}}}));await page.route('**/api/v1/integrations/owned-tmdb-single',route=>{saved=route.request().postDataJSON();return route.fulfill({json:{data:{...row,revision:3}}})})
 await page.goto('/settings?tab=tmdb');await expect(page.getByLabel('TMDB API Token',{exact:true})).toHaveValue(token);await expect(page.getByRole('button',{name:'添加连接',exact:true})).toHaveCount(0);await expect(page.getByLabel('连接名称',{exact:true})).toHaveCount(0);await expect(page.getByLabel('展示语言',{exact:true})).toHaveCount(0);await expect(page.getByRole('link',{name:'服务连接状态',exact:true})).toHaveCount(0)
 await page.getByLabel('TMDB API Token',{exact:true}).fill('owned-replaced-token');await page.getByRole('button',{name:'保存TMDB Token',exact:true}).click();await expect(page.getByText(/TMDB Token已保存/)).toBeVisible();expect(saved).toMatchObject({kind:'tmdb',revision:2,secrets:{token:'owned-replaced-token'}})
 for(const width of [1440,375]){await page.setViewportSize({width,height:950});expect(await page.getByRole('navigation',{name:'系统设置分类'}).evaluate(el=>getComputedStyle(el).flexDirection)).toBe('row');expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`settings-single-token-${width}.png`),fullPage:true,animations:'disabled'})}
})

test('friendly log details keep raw values and copy controls without redundant URL buttons (fixture)',async({page})=>{
 await fixtureLogin(page);await page.addInitScript(()=>Object.defineProperty(navigator,'clipboard',{value:undefined,configurable:true}));const url='https://owned.example/strm?apiKey=full-value',location='https://cdn.example/video?sign=full-value'
 await page.route('**/api/v1/logs/query?*',route=>route.fulfill({json:{data:{items:[{msg:'HTTP request',level:'INFO',module:'playback',time:'2026-10-15T00:00:00Z',url,location,status:302,durationMs:4,requestId:'owned-full-request'},{msg:'player UA denied',level:'WARN',userAgent:'Owned-UA',time:'2026-10-15T00:00:00Z'}],total:2,page:1,pageSize:200,modules:['playback'],before:'2026-10-15T01:00:00Z',truncated:false}}}))
 await page.goto('/logs');await expect(page.getByText('已返回跳转地址，等待客户端直连',{exact:true})).toBeVisible();await expect(page.locator('.log-row .log-detail-toggle')).toHaveCount(2);await expect(page.getByRole('button',{name:'查看请求与跳转链接',exact:true})).toHaveCount(0);await page.getByRole('button',{name:'查看第1条日志详情',exact:true}).click();const detail=page.getByRole('dialog',{name:'日志详情',exact:true});await expect(detail.getByLabel('完整请求地址',{exact:true})).toHaveValue(url);await expect(detail.getByLabel('302跳转目标',{exact:true})).toHaveValue(location);await expect(detail.locator('pre')).toContainText('owned-full-request');await detail.getByRole('button',{name:'复制跳转地址',exact:true}).click();const copy=page.getByRole('dialog',{name:'复制原始日志',exact:true});await expect(copy.getByLabel('待复制日志')).toHaveValue(location);await copy.getByRole('button',{name:'关闭',exact:true}).click();await detail.getByRole('button',{name:'关闭',exact:true}).click();await page.getByRole('button',{name:'查看第2条日志详情',exact:true}).click();await expect(detail.getByRole('button',{name:'复制跳转地址',exact:true})).toHaveCount(0);await expect(detail).toContainText('白名单');await detail.getByRole('button',{name:'关闭',exact:true}).click()
 for(const width of [1440,375]){await page.setViewportSize({width,height:950});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:resolve(screenshots,`logs-friendly-${width}.png`),animations:'disabled'})}
})

test('native preprocessing controls, statistics, retry confirmation and bounded GC (UI contract)',async({page})=>{
 // Real administrator login; deterministic preprocessing API fixtures isolate
 // browser interactions from 115/ffmpeg. Queue mutations are real in Go tests.
 const c=JSON.parse(readFileSync(resolve(root,'internal/platform/httpapi/testdata/contracts/native-processing.json'),'utf8'))
 let settings={...c.summary.response.data.settings},single=0,batches=0,gcs=0
 await page.route('**/api/v1/media-server/native/processing**',async route=>{
  const request=route.request(),path=new URL(request.url()).pathname
  if(path.endsWith('/settings')){expect(request.method()).toBe('PUT');expect(request.headers()['x-requested-with']).toBe('media-tool');expect(request.postDataJSON()).toEqual(c.settings.request);settings={...c.settings.response.data};return route.fulfill({json:c.settings.response})}
  if(path.endsWith('/tasks'))return route.fulfill({json:c.tasks.response})
  if(path.endsWith('/retry')){const body=request.postDataJSON();expect(request.method()).toBe('POST');expect(request.headers()['x-requested-with']).toBe('media-tool');if(body.bulk){batches++;expect(body).toEqual(c.batch.request);return route.fulfill({status:202,json:c.batch.response})}single++;expect(body).toEqual(c.retry.request);return route.fulfill({status:202,json:c.retry.response})}
  if(path.endsWith('/gc')){gcs++;expect(request.method()).toBe('POST');return route.fulfill({json:c.gc.response})}
  return route.fulfill({json:{data:{...c.summary.response.data,settings}}})
 })
 await fixtureLogin(page)
 // This request is NOT intercepted by page.route: verify real runtime wiring,
 // admin auth/CSRF and one bounded GC in the isolated temporary stills root.
 const realStatus=await page.request.get('/api/v1/media-server/native/processing');expect(realStatus.status()).toBe(200)
 expect((await realStatus.json()).data.settings.enabled).toBe(false)
 const realGC=await page.request.post('/api/v1/media-server/native/processing/gc',{headers:{'X-Requested-With':'media-tool'}});expect(realGC.status()).toBe(200)
 const gcReport=(await realGC.json()).data;expect(gcReport.scanned).toBeLessThanOrEqual(200);expect(gcReport.reclaimed).toBe(0)
 await navigate(page,'自研媒体库')
 await expect(page.getByRole('heading',{name:'媒体预处理',exact:true})).toBeVisible()
 await expect(page.getByRole('region',{name:'探测统计'})).toContainText('失败')
 await expect(page.getByRole('region',{name:'截图容量'})).toContainText('2048.0 MiB')
 await expect(page.getByRole('switch',{name:'原生媒体服务',exact:true})).not.toBeChecked()
 await page.locator('label[for="native-service-enabled"]').click()
 await page.getByRole('button',{name:'保存开关',exact:true}).click()
 await expect(page.getByText('预处理开关已保存',{exact:true})).toBeVisible()
 const row=page.locator('.processing-table tr').filter({hasText:'合成电影'})
 await expect(row).toContainText('115 授权失效')
 await row.getByRole('button',{name:'重试',exact:true}).click()
 await expect(page.getByRole('status').filter({hasText:'本次选中 1 条'})).toContainText('已排队 1 条')
 expect(single).toBe(1)
 await page.getByRole('button',{name:'重试当前筛选结果（最多 500 条）',exact:true}).click()
 await page.getByRole('button',{name:'取消',exact:true}).click();expect(batches).toBe(0)
 await page.getByRole('button',{name:'重试当前筛选结果（最多 500 条）',exact:true}).click()
 await page.getByRole('button',{name:'确认重试',exact:true}).click()
 await expect(page.getByRole('status').filter({hasText:'本次选中 500 条'})).toContainText('失败 1 条');expect(batches).toBe(1)
 await page.getByRole('button',{name:'执行一轮 GC',exact:true}).click()
 await expect(page.getByRole('status').filter({hasText:'本轮 GC'})).toContainText('扫描 8 项，回收 2 个文件');expect(gcs).toBe(1)
 await expect(page.locator('.el-message')).toHaveCount(0,{timeout:6000});await page.getByRole('heading',{name:'媒体预处理',exact:true}).click();await page.evaluate(()=>window.scrollTo(0,0))
 mkdirSync(screenshots,{recursive:true});await page.screenshot({path:resolve(screenshots,'native-processing-desktop.png'),fullPage:true})
 await page.setViewportSize({width:390,height:844});await expect(page.getByRole('heading',{name:'媒体预处理',exact:true})).toBeVisible()
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
 await page.screenshot({path:resolve(screenshots,'native-processing-mobile.png'),fullPage:true})
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
  await expect(page.getByRole('heading',{name:'内置媒体库连接',exact:true})).toBeVisible()
  await expect(page.getByRole('heading',{name:'Infuse 添加',exact:true})).toBeVisible()
  await expect(page.getByRole('heading',{name:'SenPlayer 添加',exact:true})).toBeVisible()
  await page.getByLabel('服务名称（ServerName）',{exact:true}).fill('P4 Synthetic Living Room')
  const enabled=page.getByRole('switch',{name:'启用内置媒体库',exact:true})
  if(await enabled.getAttribute('aria-checked')!=='true')await page.locator('label[for="native-library-enabled"]').click()
  await page.getByRole('button',{name:'保存媒体库设置',exact:true}).click()
  await expect(page.getByText('内置媒体库设置已保存',{exact:true})).toBeVisible()
  await expect.poll(async()=>{const info=await page.request.get('http://127.0.0.1:18087/System/Info/Public');expect(info.status()).toBe(200);return(await info.json()).ServerName}).toBe('P4 Synthetic Living Room')
  await expect(page.getByRole('heading',{name:'在线会话 / 正在播放',exact:true})).toBeVisible()
  await page.goto('/media-users')
  await page.getByRole('button',{name:'添加媒体用户',exact:true}).click()
  let dialog=page.getByRole('dialog',{name:'添加媒体用户',exact:true})
  await dialog.getByLabel('媒体账号',{exact:true}).fill(username)
  await dialog.getByLabel('初始密码',{exact:true}).fill('Fixture123')
  await dialog.getByLabel('确认初始密码',{exact:true}).fill('Fixture123')
  await dialog.getByRole('spinbutton',{name:'同时播放上限',exact:true}).fill('3')
  await dialog.getByRole('spinbutton',{name:'家长分级上限',exact:true}).fill('65')
  await dialog.locator('#media-expiry').fill('2030-01-01T12:00')
  await dialog.getByRole('button',{name:'创建用户',exact:true}).click()
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
  const row=page.locator('.user-table .el-table__row').filter({hasText:username})
  await row.getByRole('button',{name:'设备与历史',exact:true}).click()
  dialog=page.getByRole('dialog',{name:username+' · 设备与历史',exact:true})
  const deviceRow=dialog.locator('.el-table__row').first()
  await expect(deviceRow).toContainText('Infuse')
  const devices=(await (await page.request.get(`/api/v1/media-server/users/${userId}/devices`)).json()).data.items
  const deviceId=devices[0].id
  await deviceRow.getByRole('button',{name:'撤销设备',exact:true}).click()
  await page.getByRole('button',{name:'确认撤销',exact:true}).click()
  await expect(page.getByText('设备登录已撤销',{exact:true})).toBeVisible()
  const tokens=[token,other]
  // Device session identity is the DTO's SessionInfo.Id, never shown in the UI.
  for(const result of tokens){
   const r=await page.request.get(`http://127.0.0.1:18087/Users/${result.User.Id}`,{headers:{'X-Emby-Token':result.AccessToken}})
   expect(r.status()).toBe(result.SessionInfo.Id===deviceId?401:200)
  }
  await dialog.getByRole('tab',{name:'观看历史',exact:true}).click()
  await expect(dialog.getByText('没有观看历史',{exact:true})).toBeVisible()
  await dialog.getByRole('button',{name:'关闭此对话框',exact:true}).click()
  await row.getByRole('button',{name:'编辑',exact:true}).click()
  dialog=page.getByRole('dialog',{name:'编辑媒体用户',exact:true})
  await dialog.getByRole('button',{name:'不限制分级',exact:true}).click()
  await dialog.getByRole('button',{name:'永不过期',exact:true}).click()
  await dialog.getByRole('button',{name:'保存用户',exact:true}).click()
  await expect(dialog).not.toBeVisible()
  users=(await (await page.request.get('/api/v1/media-server/users')).json()).data
  const cleared=users.find((u:any)=>u.id===userId)
  expect(cleared.maxStreams).toBe(3);expect(cleared.maxParentalRating).toBeNull();expect(cleared.expiresAt).toBeNull()
  for(const width of [375,1440]){
   await page.setViewportSize({width,height:1000});await page.goto('/media-server')
   await expect(page.getByRole('heading',{name:'内置媒体库连接',exact:true})).toBeVisible()
   await expect(page.getByLabel('服务名称（ServerName）',{exact:true})).toHaveValue('P4 Synthetic Living Room')
   await expect(page.getByRole('switch',{name:'启用内置媒体库',exact:true})).toBeChecked()
   await expect(page.getByRole('button',{name:'保存开关',exact:true})).toBeEnabled()
   await expect(page.getByText('原生播放已接入',{exact:true})).toBeVisible()
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
 await fixtureLogin(page);await page.goto('/plugins');await page.getByRole('button',{name:'服务与命令',exact:true}).click();const dialog=page.getByRole('dialog',{name:'服务与命令',exact:true});await expect(dialog).toContainText('结果未知');expect(submitted).toHaveLength(0)
 await dialog.getByRole('button',{name:'手动执行',exact:true}).click();await page.getByRole('dialog',{name:'确认执行扩展',exact:true}).getByRole('button',{name:'取消',exact:true}).click();expect(submitted).toHaveLength(0)
 await dialog.getByRole('button',{name:'手动执行',exact:true}).click();await page.getByRole('dialog',{name:'确认执行扩展',exact:true}).getByRole('button',{name:'确认执行',exact:true}).click();await expect(dialog).toContainText('不是执行成功');expect(submitted).toHaveLength(1);expect(submitted[0]).toMatchObject({kind:'command',id:'hello',confirm:true});await page.screenshot({path:resolve(screenshots,'plugin-extensions.png'),animations:'disabled'})
})

test('F2 plugin JSON form, shared ESM, multi-key dashboards and sidebar (UI contract)',async({page})=>{
 const id='00000000-0000-4000-8000-000000000123',instance={id,pluginId:'hello-go',desiredState:'enabled',state:'running',revision:2,reason:''},configs:any[]=[]
 const contract=JSON.parse(readFileSync(resolve(root,'internal/platform/httpapi/testdata/contracts/plugin-v2-ui.json'),'utf8'))
 await page.route('**/api/v1/plugins?*',r=>r.fulfill({json:{data:[instance]}}))
 await page.route('**/api/v1/plugins/ui-surfaces',r=>r.fulfill({json:{data:[{instanceId:id,pluginId:'hello-go'}]}}))
 await page.route(`**/api/v1/plugins/${id}?*`,r=>r.fulfill({json:{data:{instance,config:{revision:1,value:{greeting:'Initial',scheduled:true}},entries:[],next:'',fingerprint:'owned-fingerprint'}}}))
 await page.route('**/api/v1/plugins/operations',async r=>{configs.push(r.request().postDataJSON());await r.fulfill({json:{data:{...instance,revision:3}}})})
 await page.route(`**/api/v1/plugins/${id}/ui/*`,r=>{const url=new URL(r.request().url()),kind=url.pathname.split('/').pop()!;return r.fulfill({json:{data:kind==='dashboard'?{content:[{component:'Text',props:{text:'Dashboard '+url.searchParams.get('key')}}]}:contract[kind]}})})
 await fixtureLogin(page);await page.goto('/plugins')
 await page.locator('.instance-card').getByRole('button',{name:'配置',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'实例配置',exact:true})
 await dialog.getByRole('textbox',{name:'问候语',exact:true}).fill('Browser Owned')
 await expect(dialog).toContainText('定时服务已开启')
 await dialog.getByRole('button',{name:'保存配置',exact:true}).click()
 expect(configs.at(-1)?.config).toEqual({greeting:'Browser Owned',scheduled:true})
 await page.locator('.instance-card').getByRole('link',{name:'插件页面',exact:true}).click()
 await expect(page.getByRole('button',{name:'交互 0',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'交互 0',exact:true}).click()
 await expect(page.getByRole('button',{name:'交互 1',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'查看 JSON 页面',exact:true}).click()
 await expect(page.getByText('不支持的组件：Unknown',{exact:true})).toBeVisible()
 await page.goto('/dashboard')
 await expect(page.getByRole('heading',{name:'启动次数',exact:true})).toBeVisible()
 await expect(page.getByText('Dashboard starts',{exact:true})).toBeVisible()
 await expect(page.getByText('Dashboard events',{exact:true})).toBeVisible()
 await page.getByRole('button',{name:'插件应用',exact:true}).click()
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
  const panel=dialog.locator('.el-message-box.plugin-confirm-dialog');const box=await panel.boundingBox();expect(box).not.toBeNull();expect(box!.width).toBeLessThanOrEqual(420);expect(box!.width).toBeLessThanOrEqual(width-32);expect(box!.x).toBeGreaterThanOrEqual(16)
  expect(await panel.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
  return dialog
 }
 await fixtureLogin(page);await page.goto('/plugins')
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
 await page.getByRole('button',{name:'安装插件',exact:true}).click()
 const consent=page.getByRole('dialog',{name:'插件权限确认',exact:true});await consent.getByRole('checkbox').check()
 await consent.getByRole('button',{name:'确认安装或授权',exact:true}).click();await expect(page.getByRole('alert')).toContainText('安装暂不可用')
 await expect(consent.getByRole('button',{name:'确认安装或授权',exact:true})).toBeEnabled()
 await consent.getByRole('button',{name:'确认安装或授权',exact:true}).click();await expect(consent).toBeHidden()
 expect(operations[0].operationKey).toBe(operations[1].operationKey)
 const card=page.locator('.instance-card');await expect(card).toContainText('期望停用')
 for(const width of [1440,375]){
  await page.setViewportSize({width,height:1000});await card.getByRole('button',{name:'启用',exact:true}).click()
  await bounded('确认插件操作',width);await page.keyboard.press('Escape');await expect(page.getByRole('dialog',{name:'确认插件操作',exact:true})).toBeHidden()
 }
 await card.getByRole('button',{name:'启用',exact:true}).click()
 await (await bounded('确认插件操作',375)).getByRole('button',{name:'确认操作',exact:true}).click();await expect(card).toContainText('期望启用')
 await page.setViewportSize({width:1440,height:1000});await card.getByRole('button',{name:'服务与命令',exact:true}).click()
 const extensions=page.getByRole('dialog',{name:'服务与命令',exact:true})
 for(const width of [1440,375]){
  await page.setViewportSize({width,height:1000});await extensions.getByRole('button',{name:'手动执行',exact:true}).click()
  await bounded('确认执行扩展',width)
  await page.getByRole('dialog',{name:'确认执行扩展',exact:true}).locator('.plugin-confirm-dialog').screenshot({path:resolve(screenshots,`plugin-confirm-${width}.png`)})
  await page.keyboard.press('Escape');await expect(page.getByRole('dialog',{name:'确认执行扩展',exact:true})).toBeHidden()
 }
 await page.setViewportSize({width:1440,height:1000})
 for(let attempt=0;attempt<2;attempt++){
  await extensions.getByRole('button',{name:'手动执行',exact:true}).click()
  await page.getByRole('dialog',{name:'确认执行扩展',exact:true}).getByRole('button',{name:'确认执行',exact:true}).click()
  if(!attempt)await expect(page.getByRole('alert').filter({hasText:'执行暂不可用'})).toBeVisible()
  else await expect(extensions).toContainText('不是执行成功')
  await expect(extensions.getByRole('button',{name:'手动执行',exact:true})).toBeEnabled()
 }
 expect(runs[0].operationKey).toBe(runs[1].operationKey)
 for(const input of [...operations,...runs])expect(input.operationKey).toMatch(/^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$/)
 expect(errors).toEqual([])
})

test('R-F3 HTTP clicks install, enable, execute and disable the real same-binary plugin (Linux)',async({page})=>{
 test.skip(process.platform!=='linux','Actual worker process accounting requires Linux; native AMD64 CI runs this test')
 await page.addInitScript(()=>Object.defineProperty(window.crypto,'randomUUID',{value:undefined,configurable:true}))
 await fixtureLogin(page);await page.goto('/plugins')
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
 let id=''
 try{
  const installation=page.waitForResponse(r=>r.url().endsWith('/api/v1/plugins/operations')&&r.request().postDataJSON().kind==='install')
  await page.getByRole('button',{name:'安装插件',exact:true}).click()
  const consent=page.getByRole('dialog',{name:'插件权限确认',exact:true});await consent.getByRole('checkbox').check();await consent.getByRole('button',{name:'确认安装或授权',exact:true}).click()
  const installed=await installation;expect(installed.status()).toBe(200);id=(await installed.json()).data.id
  const card=page.locator('.instance-card').filter({hasText:id})
  await card.getByRole('button',{name:'启用',exact:true}).click();await page.getByRole('dialog',{name:'确认插件操作',exact:true}).getByRole('button',{name:'确认操作',exact:true}).click()
  await expect.poll(async()=>{await page.getByRole('button',{name:'刷新',exact:true}).click();return card.getByRole('button',{name:'服务与命令',exact:true}).isEnabled()},{timeout:20000}).toBe(true)
  await card.getByRole('button',{name:'服务与命令',exact:true}).click()
  const dialog=page.getByRole('dialog',{name:'服务与命令',exact:true})
  await dialog.getByRole('button',{name:'手动执行',exact:true}).first().click()
  const execution=page.waitForResponse(r=>r.url().endsWith(`/plugins/${id}/run`))
  await page.getByRole('dialog',{name:'确认执行扩展',exact:true}).getByRole('button',{name:'确认执行',exact:true}).click();expect((await execution).status()).toBe(202)
  await expect.poll(async()=>((await (await page.request.get(`/api/v1/plugins/${id}/runs?page=1&pageSize=50`)).json()).data as any[]).some(r=>r.state==='succeeded'),{timeout:20000}).toBe(true)
  await dialog.getByRole('button',{name:'Close this dialog',exact:true}).click()
  await card.getByRole('button',{name:'查看 KV',exact:true}).click();await expect(page.getByRole('dialog',{name:'实例 KV 数据',exact:true})).toContainText('value')
  await page.getByRole('dialog',{name:'实例 KV 数据',exact:true}).getByRole('button',{name:'Close this dialog',exact:true}).click()
  await card.getByRole('button',{name:'停用',exact:true}).click();await page.getByRole('dialog',{name:'确认插件操作',exact:true}).getByRole('button',{name:'确认操作',exact:true}).click();await expect(card).toContainText('期望停用')
  expect(errors).toEqual([])
 }finally{
  if(id){
   const detail=(await (await page.request.get(`/api/v1/plugins/${id}`)).json()).data
   const response=await page.request.post('/api/v1/plugins/operations',{headers:{Origin:'http://localhost:18084','X-Requested-With':'media-tool'},data:{kind:'remove',instanceId:id,fingerprint:detail.fingerprint,expectedRevision:detail.instance.revision,operationKey:`rf3-cleanup-${id}`,confirm:true}})
   expect(response.status()).toBe(200)
  }
 }
})
