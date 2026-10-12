import { test,expect } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import { resolve } from 'node:path'
import { navigationItems } from '../src/navigation/items'
const root=resolve(import.meta.dirname,'../..')
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
 await expect(page.getByRole('heading',{name:'媒体链路总览'})).toBeVisible()
 await expect(page.getByText('PostgreSQL',{exact:true})).toBeVisible()
}
// Later UI-only fixtures share one valid account session in memory. Login/
// logout/password/rate-limit behavior above remains separately exercised; do
// not weaken production limits just because the suite gained more UI fixtures.
let fixtureCookies:Awaited<ReturnType<import('@playwright/test').BrowserContext['cookies']>>|undefined
async function fixtureLogin(page:import('@playwright/test').Page){
 if(!fixtureCookies){await login(page);fixtureCookies=await page.context().cookies();return}
 await page.context().addCookies(fixtureCookies)
 await page.goto('/dashboard')
 await expect(page.getByRole('heading',{name:'媒体链路总览'})).toBeVisible()
}
test('first registration creates the administrator and closes public registration',async({page,request})=>{
 await page.goto('/login');await expect(page.getByRole('heading',{name:'注册管理员账号',exact:true})).toBeVisible()
 await expect(page.getByText('6–20个字符，支持中文、空格，不会去除密码首尾空格。',{exact:true})).toBeVisible()
 await page.getByLabel('管理员账号',{exact:true}).fill(username);await page.getByLabel('密码',{exact:true}).fill('abc12');await page.getByLabel('确认密码',{exact:true}).fill('abc12');await page.getByRole('button',{name:'注册并进入工作台'}).click();await expect(page.getByText('密码需6–20个字符，可使用空格和中文',{exact:true})).toBeVisible()
 await page.getByLabel('管理员账号',{exact:true}).fill(username);await page.getByLabel('密码',{exact:true}).fill(password);await page.getByLabel('确认密码',{exact:true}).fill(password+'X')
 mkdirSync(screenshots,{recursive:true});await page.screenshot({path:resolve(screenshots,'account-registration-desktop.png'),fullPage:true,mask:[page.getByLabel('密码',{exact:true}),page.getByLabel('确认密码',{exact:true})]})
 await page.getByRole('button',{name:'注册并进入工作台'}).click();await expect(page.getByText('两次输入的密码不一致',{exact:true})).toBeVisible()
 await page.getByLabel('确认密码',{exact:true}).fill(password);const registered=page.waitForResponse(r=>r.url().endsWith('/auth/register'));await page.getByRole('button',{name:'注册并进入工作台'}).click();const reply=await registered;expect(reply.status()).toBe(201);expect(await reply.text()).not.toContain(password)
 await expect(page.getByRole('heading',{name:'媒体链路总览'})).toBeVisible();const setup=await request.get('/api/v1/auth/setup');expect((await setup.json()).data.registrationOpen).toBe(false)
 const second=await request.post('/api/v1/auth/register',{headers:{'X-Requested-With':'media-tool'},data:{username:'second_admin',password,confirmPassword:password}});expect(second.status()).toBe(409)
 await page.getByRole('button',{name:'退出',exact:true}).click();await expect(page.getByRole('heading',{name:'进入管理工作台',exact:true})).toBeVisible();await expect(page.getByLabel('确认密码',{exact:true})).toHaveCount(0)
})
test('login, honest dependency states, module navigation and logout',async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
 await login(page)
 await expect(page.getByText('数据库连接可用',{exact:true})).toBeVisible()
 await expect(page.getByText('请填写地址和接入凭据',{exact:false})).toBeVisible()
 mkdirSync(screenshots,{recursive:true});await page.screenshot({path:resolve(screenshots,'dashboard-desktop.png'),fullPage:true})
 await navigate(page,'系统设置')
 await expect(page.getByRole('heading',{name:'媒体命名与搜索缓存',exact:true})).toBeVisible()
 await expect(page.getByRole('button',{name:'验证 CloudDrive2',exact:true})).toHaveCount(0)
 await expect(page.getByRole('button',{name:'验证 Emby',exact:true})).toHaveCount(0)
 await navigate(page,'PT 站点')
 await expect(page.getByRole('heading',{name:'PT 站点配置'})).toBeVisible()
 await page.getByRole('button',{name:'退出',exact:true}).click()
 await expect(page.getByRole('heading',{name:'进入管理工作台'})).toBeVisible()
 expect(errors).toEqual([])
})
test('STRM preview encodes cloud path and download has one stable URL',async({page})=>{
 await login(page);await navigate(page,'STRM 管理')
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
 await navigate(page,'STRM 管理')
 await expect(page.getByRole('heading',{name:'STRM 固定链接'})).toBeVisible()
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
 await page.screenshot({path:resolve(screenshots,'strm-mobile.png'),fullPage:true})
 await page.setViewportSize({width:812,height:375})
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
})

test('saved connections are editable, redacted, and can be deleted',async({page})=>{
 await login(page);await page.goto('/cloudStorage')
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
 await expect(page.getByRole('heading',{name:/^已入库文件/})).toBeVisible()
 await page.getByRole('button',{name:'开始扫描',exact:true}).click()
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
 await page.getByRole('button',{name:'新增定时扫描'}).click();const dialog=page.getByRole('dialog');await expect(dialog.getByRole('switch',{name:'启用定时扫描'})).not.toBeChecked();await expect(dialog.getByRole('switch',{name:'启用CloudDrive2文件变化推送'})).not.toBeChecked()
 await dialog.getByRole('button',{name:'保存定时扫描'}).click();await expect(page.getByText('请先在服务连接中配置并启用 CloudDrive2',{exact:true})).toBeVisible()
 await dialog.getByRole('button',{name:'取消',exact:true}).click();await expect(dialog).not.toBeVisible();await page.screenshot({path:resolve(screenshots,'library-desktop.png'),fullPage:true});await page.setViewportSize({width:375,height:812});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
 mkdirSync(screenshots,{recursive:true});await page.screenshot({path:resolve(screenshots,'library-mobile.png'),fullPage:true})
})

test('wrong credentials are rejected and password change revokes captured sessions',async({page,request})=>{
 const wrong=await request.post('/api/v1/auth/login',{headers:{'X-Requested-With':'media-tool'},data:{username,password:'Wrong-strong-password-2026'}});expect(wrong.status()).toBe(401)
 await login(page);const captured=(await page.context().cookies()).find(c=>c.name==='media_tool_session')!;await navigate(page,'系统设置')
 const next=password+'-new';await page.getByLabel('当前密码',{exact:true}).fill(password);await page.getByLabel('新密码',{exact:true}).fill(next);await page.getByLabel('确认新密码',{exact:true}).fill(next);await page.getByRole('button',{name:'修改密码并重新登录'}).click();await expect(page.getByRole('heading',{name:'进入管理工作台',exact:true})).toBeVisible()
 const replay=await request.get('/api/v1/auth/session',{headers:{Cookie:'media_tool_session='+captured.value}});expect(replay.status()).toBe(401)
 const old=await request.post('/api/v1/auth/login',{headers:{'X-Requested-With':'media-tool'},data:{username,password}});expect(old.status()).toBe(401)
 await page.getByLabel('管理员账号',{exact:true}).fill(username);await page.getByLabel('密码',{exact:true}).fill(next);await page.getByRole('button',{name:'进入工作台',exact:true}).click();await expect(page.getByRole('heading',{name:'媒体链路总览'})).toBeVisible()
 // Restore only the synthetic test account, never a user's administrator.
 await navigate(page,'系统设置');await page.getByLabel('当前密码',{exact:true}).fill(next);await page.getByLabel('新密码',{exact:true}).fill(password);await page.getByLabel('确认新密码',{exact:true}).fill(password);await page.getByRole('button',{name:'修改密码并重新登录'}).click();await expect(page.getByRole('heading',{name:'进入管理工作台',exact:true})).toBeVisible()
})

test('account login form is accessible on mobile and needs no deployment key',async({page})=>{
 await page.setViewportSize({width:375,height:812});await page.emulateMedia({reducedMotion:'reduce'});await page.goto('/login');await expect(page.getByRole('heading',{name:'进入管理工作台',exact:true})).toBeVisible();await expect(page.getByLabel('管理员账号',{exact:true})).toBeVisible();await expect(page.getByLabel('密码',{exact:true})).toHaveAttribute('autocomplete','current-password');expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);expect(await page.evaluate(()=>localStorage.length)).toBe(0);await page.screenshot({path:resolve(screenshots,'account-login-mobile.png'),fullPage:true})
})

test('official 115 is saved disabled and QR authorization is explicit and cancellable',async({page})=>{
 await login(page);await page.goto('/cloudStorage')
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

test('site snapshot recovery keeps the exact commit, license gate and validation (fixture)',async({page})=>{
 const commit='a'.repeat(40)
 const payloads:Array<{ids:string[];sourceCommit?:string;licenseAccepted:boolean}>=[]
 await page.route('**/api/v1/sites/definitions',route=>route.fulfill({json:{data:[{id:'owned_snapshot',name:'自有快照样本',schema:'NexusPHP',sourceCommit:commit,compatibility:{state:'partial',notes:['原条件优先级缺失，需要同快照重新导入（fixture）'],accountVerified:false}}]}}))
 await page.route('**/api/v1/sites/definitions/import',async route=>{payloads.push(route.request().postDataJSON());await route.fulfill({json:{data:[]}})})
 await fixtureLogin(page);await navigate(page,'PT 站点')
 await expect(page.getByRole('heading',{name:'PT 站点配置'})).toBeVisible()
 const restore=page.getByRole('button',{name:'按此快照重新导入',exact:true})
 await expect(restore).toBeDisabled()
 // Element Plus visually hides its native checkbox; click the visible label,
 // exactly like a user, and verify the native checked state (no force bypass).
 await page.getByText('我已阅读上游许可证，确认具备本次配置使用授权',{exact:true}).click()
 await expect(page.getByRole('checkbox',{name:'我已阅读上游许可证，确认具备本次配置使用授权'})).toBeChecked()
 await expect(restore).toBeEnabled();await restore.click()
 await expect(page.getByText('已原子导入固定快照，账号实测仍需单独验证',{exact:true})).toBeVisible()
 expect(payloads[0]).toEqual({ids:['owned_snapshot'],sourceCommit:commit,licenseAccepted:true})
 await page.getByLabel('来源提交（可选）',{exact:true}).fill('main')
 await expect(page.getByText('来源提交须为完整的 40 位十六进制 SHA，或留空。',{exact:true})).toBeVisible()
 await expect(page.getByRole('button',{name:'同步固定快照',exact:true})).toBeDisabled()
 expect(payloads).toHaveLength(1)
 await page.getByLabel('来源提交（可选）',{exact:true}).fill(commit)
 await page.getByLabel('站点 ID（逗号分隔）',{exact:true}).fill('owned_snapshot')
 await page.getByRole('button',{name:'同步固定快照',exact:true}).click()
 await expect.poll(()=>payloads.length).toBe(2)
 expect(payloads[1]?.sourceCommit).toBe(commit)
 await expect(page.getByText('已原子导入固定快照，账号实测仍需单独验证',{exact:true}).first()).toBeHidden()
 await page.screenshot({path:resolve(screenshots,'sites-snapshot-recovery.png'),fullPage:true,animations:'disabled'})
 await page.setViewportSize({width:375,height:812})
 // Desktop navigation is removed from the mobile accessibility tree.
 await expect(page.getByRole('complementary',{name:'主导航'})).toBeHidden()
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
 await expect(page.getByLabel('来源提交（可选）',{exact:true})).toBeInViewport()
 await expect(page.getByRole('button',{name:'按此快照重新导入',exact:true})).toBeVisible()
 await page.screenshot({path:resolve(screenshots,'sites-snapshot-mobile.png'),fullPage:true,animations:'disabled'})
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
 await row.getByRole('button',{name:'修正身份 / 季集',exact:true}).click()
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
 await row.getByRole('button',{name:'修正身份 / 季集',exact:true}).click()
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
 await page.getByRole('row').filter({hasText:'自有剧集'}).getByRole('button',{name:'修正身份 / 季集',exact:true}).click()
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
 await expect(page.getByRole('heading',{name:'旧输出归档',exact:false})).toBeVisible()
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
 await page.getByRole('row').filter({hasText:'待核对.strm'}).locator('.el-checkbox').click()
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
 await page.getByRole('row').filter({hasText:'自有非连续合集'}).getByRole('button',{name:'修正身份 / 季集',exact:true}).click()
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
 await expect(link).toHaveAttribute('href',`/library?assetId=${asset}`)
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
 await expect(page.getByText('部分结果',{exact:true})).toHaveCount(2)
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

test('PT required-header credentials are masked, validated, encrypted and explicitly clearable',async({page})=>{
 await fixtureLogin(page)
 await page.emulateMedia({reducedMotion:'reduce'})
 await page.goto('/sites')
 await page.route('**/api/v1/integrations',async route=>{if(route.request().method()!=='POST')return route.continue();const inData=route.request().postDataJSON();await route.fulfill({status:201,json:{data:{id:'77777777-7777-4777-8777-777777777777',...inData,secrets:undefined,secretFields:['requestHeaders'],revision:1,lastCheckState:'ready',lastCheckMessage:'明确的隔离验证样本'}}})})
 await page.getByRole('button',{name:'添加连接'}).click()

 const dialog=page.getByRole('dialog',{name:'添加连接',exact:true})
 const name='PT headers fixture '+Date.now()
 await dialog.getByLabel('连接名称',{exact:true}).fill(name)
 await dialog.getByLabel('站点定义 ID',{exact:true}).fill('owned_header_fixture')
 await dialog.getByLabel('站点访问地址',{exact:true}).fill('https://owned-header.example.invalid')
 const headers=dialog.getByLabel('附加鉴权头（JSON，加密保存）',{exact:true})
 await expect(headers).toHaveAttribute('type','password')
 await headers.fill('{"Authorization":12}')
 await dialog.getByRole('button',{name:'加密保存',exact:true}).click()
 await expect(dialog.getByRole('alert').filter({hasText:'每项鉴权头必须是非空字符串'})).toBeVisible()
 await expect(headers).toHaveAttribute('aria-invalid','true')
 await headers.fill('{"Authorization":"one","Authorization":"two"}')
 await dialog.getByRole('button',{name:'加密保存',exact:true}).click()
 await expect(dialog.getByRole('alert').filter({hasText:'鉴权头名称不能重复'})).toBeVisible()
 const secret='PRIVATE-PT-HEADER-E2E'
 await headers.fill(JSON.stringify({Authorization:'Bearer '+secret,'X-Csrf-Token':'PRIVATE-CSRF-E2E'}))
 await expect(dialog.getByText(/只用于当前站点声明的必需头/)).toBeVisible()
 await headers.scrollIntoViewIfNeeded()
 await dialog.screenshot({path:resolve(screenshots,'pt-account-headers-desktop.png'),animations:'disabled',mask:[headers]})
 await page.setViewportSize({width:375,height:812})
 await headers.scrollIntoViewIfNeeded()
 await dialog.getByRole('button',{name:'加密保存',exact:true}).scrollIntoViewIfNeeded()
 await expect(dialog.getByRole('button',{name:'加密保存',exact:true})).toBeInViewport({ratio:1})
 await expect(dialog.getByRole('button',{name:'关闭此对话框',exact:true})).toBeInViewport({ratio:1})
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true)
 await dialog.screenshot({path:resolve(screenshots,'pt-account-headers-mobile.png'),animations:'disabled',mask:[headers]})
 const savedPromise=page.waitForResponse(r=>r.url().endsWith('/api/v1/integrations')&&r.request().method()==='POST')
 await dialog.getByRole('button',{name:'加密保存',exact:true}).click()
 const saved=await savedPromise
 expect(saved.status()).toBe(201)
 const raw=await saved.text();expect(raw).not.toContain(secret);expect(raw).not.toContain('PRIVATE-CSRF-E2E')
 const record=(await saved.json()).data;expect(record.secretFields).toContain('requestHeaders');expect(await saved.text()).not.toContain(secret)
})

test('separate 115 setup saves without tokens and secret echoes are explicit (isolated)',async({page})=>{
 await fixtureLogin(page)
 await page.goto('/cloudStorage');await page.getByRole('button',{name:'115 授权',exact:true}).click()
 await expect(page.getByRole('button',{name:'115 授权',exact:true})).toHaveAttribute('aria-pressed','true')
 await page.getByRole('button',{name:'添加连接',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'添加连接',exact:true})
 const name='115 tokenless fixture '+Date.now()
 await dialog.getByLabel('连接名称',{exact:true}).fill(name)
 await dialog.getByLabel('自己的115开放平台 AppID',{exact:true}).fill('123456')
 await dialog.getByText('启用为主连接',{exact:true}).click()
 await expect(dialog.getByRole('checkbox',{name:'启用为主连接',exact:true})).toBeChecked()
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
 await expect(field).toHaveValue('')
 await edit.getByRole('button',{name:'读取已保存密钥',exact:true}).click()
 await page.getByRole('button',{name:'确认读取',exact:true}).click()
 await expect(field).toHaveValue(value);await expect(field).toHaveAttribute('type','password')
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
 await video.getByRole('button',{name:'移入回收目录',exact:true}).click()
 await page.getByRole('dialog',{name:'移入回收目录',exact:true}).getByRole('button',{name:'预览变更',exact:true}).click()
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
 await page.goto('/settings');await expect(page.getByLabel('电影命名模板',{exact:true})).toHaveValue('{title} ({year}) - {resolution}{ext}');await expect(page.getByLabel('剧集命名模板',{exact:true})).toBeVisible();await expect(page.getByRole('heading',{name:'TMDB配置',exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'生成 / 更换恢复码',exact:true})).toHaveCount(0)
 await page.locator('#pt-cache-seconds').fill('600');await page.getByRole('button',{name:'保存模板与缓存',exact:true}).click();await expect(page.getByText('模板与缓存有效期已保存，新的整理预览默认使用模板。不会自动重命名现有文件。',{exact:true})).toBeVisible()
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
 await page.goto('/search');await page.getByLabel('TMDB媒体名称',{exact:true}).fill('Owned Movie');await page.getByRole('button',{name:'搜索媒体',exact:true}).click();await expect(page.getByRole('heading',{name:/自有搜索样本/})).toBeVisible();await page.getByRole('button',{name:'搜索媒体',exact:true}).click();await expect(page.getByText(/本次使用TMDB缓存/)).toBeVisible();await page.getByRole('button',{name:'使用标题到站点搜索',exact:true}).click();await expect(page.getByLabel('资源关键词',{exact:true})).toHaveValue('Owned Movie');await expect(page.getByRole('button',{name:'忽略站点缓存重新搜索',exact:true})).toBeVisible();await page.screenshot({path:resolve(screenshots,'workbench-search-desktop.png'),animations:'disabled'})
})

test('automatic roots, 0day hints, history and safe plugin manifests',async({page})=>{
 await login(page);await page.goto('/files');await expect(page.getByText(/本地目录/).first()).toBeVisible();const parsed=await page.request.post('/api/v1/media/recognize-name',{headers:{'X-Requested-With':'media-tool'},data:{name:'Owned.Show.2026.S01E03.1080p.WEB-DL.x265-TEAM.mkv'}});expect(parsed.status()).toBe(200);expect((await parsed.json()).data.hint).toMatchObject({title:'Owned Show',year:'2026',mediaType:'series',season:1,episode:3})
 await page.goto('/organize-history');await expect(page.getByRole('heading',{name:'媒体整理历史',exact:true})).toBeVisible();await page.goto('/plugins');await expect(page.getByRole('heading',{name:'0day 文件名识别',exact:true})).toBeVisible();await expect(page.getByRole('heading',{name:'电影与剧集命名模板',exact:true})).toBeVisible();await page.getByText('兼容入口：导入扩展 Manifest',{exact:true}).click();await page.getByLabel('插件Manifest',{exact:true}).fill(JSON.stringify({id:'owned-plugin',name:'自有扩展样本',version:'1',contractVersion:1,endpoint:'http://127.0.0.1/hook',capabilities:['media.organized']}));await page.getByRole('button',{name:'导入为停用插件',exact:true}).click();await expect(page.getByText(/Manifest无效/)).toBeVisible();await page.screenshot({path:resolve(screenshots,'workbench-plugins-desktop.png'),animations:'disabled'})
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
  await page.goto('/dashboard');await expect(page.getByText('已连接依赖',{exact:true})).toBeVisible()
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
 await page.getByRole('button',{name:'查看第1条日志详情',exact:true}).click({timeout:5000});await expect(page.locator('.log-details pre')).toContainText('owned-request')
 await page.locator('label[for=log-clean-mode]').click({timeout:5000});await expect(page.locator('.log-source')).toHaveCount(0);await expect(page.locator('.log-details')).toHaveCount(0)
 await page.locator('label[for=log-clean-mode]').click({timeout:5000});await page.getByRole('button',{name:'下一页',exact:true}).click({timeout:5000});await expect(page.getByText('第二页自有日志',{exact:true})).toBeVisible();expect(queries.at(-1)).toMatchObject({page:'2',before:'2026-10-12T00:00:00Z'})
 await page.getByLabel('日志关键词',{exact:true}).fill('owned');await expect.poll(()=>queries.at(-1)?.keyword).toBe('owned');expect(queries.at(-1)?.page).toBe('1');expect(queries.at(-1)?.before).toBeUndefined()
 await page.getByRole('combobox',{name:'日志级别筛选'}).focus();await page.keyboard.press('Enter');await page.getByRole('option',{name:'ERROR',exact:true}).click();await expect.poll(()=>queries.at(-1)?.level).toBe('error')
 await page.getByRole('combobox',{name:'日志模块筛选'}).focus();await page.keyboard.press('Enter');await page.getByRole('option',{name:'tmdb',exact:true}).click();await expect.poll(()=>queries.at(-1)?.module).toBe('tmdb')
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

test('ZIP plugins install, configure encrypted fields, explicitly run, record and uninstall (isolated)',async({page})=>{
 await fixtureLogin(page);await page.goto('/plugins')
 await page.getByLabel('选择ZIP插件包',{exact:true}).setInputFiles(resolve(root,'examples/plugins/filename-rules.zip'))
 await page.getByRole('button',{name:'预览插件包',exact:true}).click()
 const preview=page.getByRole('dialog',{name:'插件包安装预览',exact:true});await expect(preview).toBeVisible()
 await preview.getByRole('button',{name:'确认安装或升级',exact:true}).click()
 const card=page.locator('.plugin-grid article').filter({has:page.getByRole('heading',{name:'示例文件名规则',exact:true})})
 await expect(card).toBeVisible();await expect(card.getByRole('button',{name:'测试',exact:true})).toBeDisabled()
 await card.getByRole('button',{name:'设置',exact:true}).click()
 await page.getByLabel('标题前缀',{exact:true}).fill('Owned ')
 await page.getByLabel('插件自己的测试密钥',{exact:true}).fill('owned-secret-e2e-only')
 await page.getByRole('button',{name:'保存插件设置',exact:true}).click();await expect(page.getByRole('dialog',{name:'示例文件名规则 插件设置',exact:true})).toBeHidden()
 const config=await page.request.get('/api/v1/plugins/example-filename-rules/settings');expect(config.status()).toBe(200);const data=(await config.json()).data;expect(data.secretFields).toContain('apiKey');expect(data.values.apiKey).toBe('owned-secret-e2e-only')
 await card.getByRole('button',{name:'授权并启用',exact:true}).click()
 await page.getByRole('dialog',{name:'启用插件',exact:true}).getByRole('button',{name:'授权并启用',exact:true}).click()
 await expect(card.getByRole('button',{name:'测试',exact:true})).toBeEnabled();await card.getByRole('button',{name:'测试',exact:true}).click()
 await page.getByLabel('测试文件名',{exact:true}).fill('Example.Show.2026.S01E03.mkv');await page.getByRole('button',{name:'执行测试',exact:true}).click();await page.getByRole('dialog',{name:'测试插件',exact:true}).getByRole('button',{name:'确认测试',exact:true}).click()
 const testDialog=page.getByRole('dialog',{name:'测试插件（不修改媒体）',exact:true});await expect(testDialog.locator('pre')).toContainText('Owned Show');await expect(testDialog.locator('pre')).toContainText('"mediaModified": false')
 await page.screenshot({path:resolve(screenshots,'zip-plugin-test.png'),animations:'disabled'});await testDialog.getByRole('button',{name:'关闭此对话框',exact:true}).click()
 await card.getByRole('button',{name:'记录',exact:true}).click();const history=page.getByRole('dialog',{name:'插件最近执行记录',exact:true});await expect(history).toContainText('succeeded');await history.getByRole('button',{name:'关闭此对话框',exact:true}).click()
 await card.getByRole('button',{name:'卸载',exact:true}).click();await page.getByRole('dialog',{name:'卸载插件',exact:true}).getByRole('button',{name:'确认卸载',exact:true}).click();await expect(card).toHaveCount(0)
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

test('PWA registration, public-only cache, headers and generic offline page (localhost secure context)',async({page,context})=>{
 await fixtureLogin(page);await page.goto('/settings');await expect(page.getByRole('heading',{name:'移动端应用 / PWA',exact:true})).toBeVisible()
 await page.evaluate(async()=>{await navigator.serviceWorker.ready});await page.reload();await expect(page.getByRole('heading',{name:'移动端应用 / PWA',exact:true})).toBeVisible()
 const sw=await page.request.get('/sw.js');expect(sw.headers()['cache-control']).toBe('no-cache');expect(sw.headers()['service-worker-allowed']).toBe('/');expect(await sw.text()).not.toContain('__BUILD_ID__')
 const manifest=await page.request.get('/manifest.webmanifest');expect((await manifest.json()).display).toBe('standalone')
 const keys=await page.evaluate(async()=>{const out:string[]=[];for(const name of await caches.keys()){if(name.startsWith('media-tool-public-'))for(const request of await (await caches.open(name)).keys())out.push(new URL(request.url).pathname)}return out});expect(keys).toContain('/offline.html');expect(keys.every(key=>key.startsWith('/assets/')||key.startsWith('/pwa/')||key==='/offline.html')).toBe(true)
 await context.setOffline(true);await page.goto('/data');await expect(page.getByRole('heading',{name:'当前处于离线状态',exact:true})).toBeVisible();await expect(page.getByRole('heading',{name:'数据库浏览',exact:true})).toHaveCount(0);await context.setOffline(false)
})

test('site-result filters issue server query and preserve selection binding (fixture)',async({page})=>{
 const seen:string[]=[]
 await page.route('**/api/v1/search',route=>route.fulfill({json:{data:[{id:'owned-filter-run',keyword:'Owned',state:'succeeded',sites:[],resources:[],total:65}]}}))
 await page.route('**/api/v1/search/owned-filter-run?*',route=>{const q=new URL(route.request().url()).searchParams;seen.push(q.toString());return route.fulfill({json:{data:{id:'owned-filter-run',keyword:'Owned',state:'succeeded',sites:[{accountId:'owned-site',name:'Owned PT',state:'succeeded',message:'',count:65}],resources:[{id:'original-resource-id',title:'Owned.Film.2026.2160p.WEB-DL.x265',siteName:'Owned PT',size:1073741824,seeders:11,leechers:0,downloadFactor:0,metadata:{attributes:{resolution:'2160p',codec:'h265',source:'web-dl'}}}],total:q.get('resolution')==='2160p'?12:65}}})})
 await fixtureLogin(page);await page.goto('/search');await page.getByText('站点直搜',{exact:true}).click();await page.getByRole('button',{name:'查看结果',exact:true}).click();await expect(page.getByText('Owned.Film.2026.2160p.WEB-DL.x265',{exact:true})).toBeVisible()
 await page.locator('.el-select').filter({has:page.getByRole('combobox',{name:'资源分辨率',exact:true})}).click();await page.getByRole('option',{name:'2160p',exact:true}).click();await expect(page.getByText(/共 12 条匹配/)).toBeVisible();expect(seen.some(q=>new URLSearchParams(q).get('resolution')==='2160p'&&new URLSearchParams(q).get('page')==='1')).toBe(true)
 await page.screenshot({path:resolve(screenshots,'site-resource-filters.png'),animations:'disabled'})
})

test('WASM plugin lifecycle, page, command, API, dashboard, service and isolated clone (isolated)',async({page})=>{
 await fixtureLogin(page);await page.goto('/plugins');await page.getByLabel('选择ZIP插件包',{exact:true}).setInputFiles(resolve(root,'examples/plugins/automation.zip'));await page.getByRole('button',{name:'预览插件包',exact:true}).click();const preview=page.getByRole('dialog',{name:'插件包安装预览',exact:true});await expect(preview).toContainText('自动化插件示例');await preview.getByRole('button',{name:'确认安装或升级',exact:true}).click();const card=page.locator('.plugin-grid article').filter({has:page.getByRole('heading',{name:'自动化插件示例',exact:true})});await expect(card).toBeVisible();await card.getByRole('button',{name:'授权并启用',exact:true}).click();await page.getByRole('dialog',{name:'启用插件',exact:true}).getByRole('button',{name:'授权并启用',exact:true}).click();await expect(card.getByRole('button',{name:'停用扩展',exact:true})).toBeVisible()
 await card.getByRole('link',{name:'示例工作台',exact:true}).click();await expect(page.getByRole('heading',{name:'自动化插件示例',exact:true}).first()).toBeVisible();await page.getByRole('button',{name:'增加计数',exact:true}).click();await page.getByRole('dialog',{name:'执行插件命令',exact:true}).getByRole('button',{name:'确认执行',exact:true}).click();await expect(page.locator('.plugin-metrics').first()).toContainText('1')
 const api=await page.request.get('/api/v1/plugins/example-automation/view/api/status');expect(api.status()).toBe(200);expect((await api.json()).data.count).toBe(1)
 const ui=await page.request.get('/api/v1/plugins/example-automation/ui');expect(ui.headers()['content-security-policy']).toContain("connect-src 'none'");expect(ui.headers()['content-security-policy']).toContain('sandbox allow-scripts');await expect(page.getByTitle('插件独立界面')).toBeVisible();const frame=page.frameLocator('iframe[title="插件独立界面"]');await frame.getByRole('button',{name:'读取本插件数据',exact:true}).click();await expect(frame.locator('#result')).toContainText('"count": 1')
 await page.screenshot({path:resolve(screenshots,'wasm-plugin-page.png'),animations:'disabled'});await page.goto('/dashboard');await expect(page.locator('.plugin-dashboard-grid')).toContainText('执行计数');await page.goto('/plugins');await card.getByRole('button',{name:'创建分身',exact:true}).click();await page.getByLabel('插件分身ID',{exact:true}).fill('owned-clone');await page.getByLabel('插件分身名称',{exact:true}).fill('Owned Clone');await page.getByRole('button',{name:'确认创建停用分身',exact:true}).click();await expect(page.getByRole('heading',{name:'Owned Clone',exact:true})).toBeVisible();const state=await page.request.get('/api/v1/plugins/owned-clone/state');expect((await state.json()).data).toMatchObject({enabled:false,data:{}})
 const services=await page.request.post('/api/v1/plugins/example-automation/services/counter-tick',{headers:{'X-Requested-With':'media-tool'},data:{confirm:true,revision:2}});expect(services.status()).toBe(202)
 // Scope cleanup to these two owned fixture plugins in the isolated database.
 const items=(await(await page.request.get('/api/v1/plugins')).json()).data;for(const id of ['owned-clone','example-automation']){const item=items.find((x:any)=>x.manifest.id===id);expect((await page.request.delete(`/api/v1/plugins/${id}?confirm=true&revision=${item.revision}`,{headers:{'X-Requested-With':'media-tool'}})).status()).toBe(200)}
})

test('subscription structured filters persist and preview displays individual reasons (fixture)',async({page})=>{
 await page.route('**/api/v1/subscriptions',route=>route.fulfill({json:{data:[{id:'owned-subscription',title:'Owned Rules',media:{id:1,mediaType:'movie',title:'Owned Film',year:'2026'},enabled:false,source:'search',intervalMinutes:30,rules:{accountIds:['owned-account'],keyword:'Owned',include:[],exclude:[],minSize:0,maxSize:0,minSeeders:1,freeOnly:true,allowUnknownHR:false,season:1,episodes:[],includeHistory:false,resolution:'2160p',source:'web-dl',codec:'h265'},revision:1,lastState:'succeeded',lastMessage:'',nextRunAt:''}]}}))
 await page.route('**/api/v1/subscriptions/owned-subscription/evaluate',route=>route.fulfill({status:202,json:{data:{id:'owned-preview'}}}));await page.route('**/api/v1/subscription-runs/owned-preview',route=>route.fulfill({json:{data:{id:'owned-preview',preview:true,state:'succeeded',decisions:[{id:'owned-decision',title:'Owned Film 2026 1080p WEB-DL x264',action:'skipped',episodes:[],reasons:['分辨率不匹配','编码不匹配'],checks:[{key:'resolution',label:'分辨率',expected:'2160p',actual:'1080p',state:'rejected',message:'分辨率不匹配：需要2160p，识别为1080p'},{key:'source',label:'来源',expected:'web-dl',actual:'web-dl',state:'matched',message:'来源匹配：web-dl'}]}]}}}));await fixtureLogin(page);await page.goto('/subscriptions');await page.getByRole('button',{name:'编辑',exact:true}).click();await expect(page.getByRole('combobox',{name:'订阅分辨率',exact:true})).toBeVisible();await expect(page.getByRole('dialog',{name:'编辑订阅',exact:true})).toContainText('2160p');await page.getByRole('dialog',{name:'编辑订阅',exact:true}).getByRole('button',{name:'关闭此对话框',exact:true}).click();await page.getByRole('button',{name:'预演',exact:true}).click();await expect(page.getByRole('dialog',{name:'订阅评估与决策',exact:true})).toContainText('分辨率 · 拒绝');await expect(page.getByRole('dialog',{name:'订阅评估与决策',exact:true})).toContainText('来源 · 通过');await page.screenshot({path:resolve(screenshots,'subscription-structured-preview.png'),animations:'disabled'})
})

test('raw administrator log URLs have dedicated full request and 302 target view (fixture)',async({page})=>{
 await page.route('**/api/v1/logs/query?*',route=>route.fulfill({json:{data:{items:[{time:'2026-10-12T10:00:00Z',level:'INFO',module:'playback',msg:'HTTP request',url:'https://owned.example/strm?apiKey=owned-fixture-key',location:'https://cdnfhnfile.115cdn.net/owned?sign=owned-fixture-sign',status:302,durationMs:12,requestId:'owned-request'}],total:1,page:1,pageSize:200,modules:['playback'],before:'2026-10-12T11:00:00Z',truncated:false}}}));await fixtureLogin(page);await page.goto('/logs');await page.getByRole('button',{name:'查看请求与跳转链接',exact:true}).click();const dialog=page.getByRole('dialog',{name:'完整请求与302链接',exact:true});await expect(dialog).toBeVisible();await expect(dialog.locator('textarea').nth(0)).toHaveValue('https://owned.example/strm?apiKey=owned-fixture-key');await expect(dialog.locator('textarea').nth(1)).toHaveValue('https://cdnfhnfile.115cdn.net/owned?sign=owned-fixture-sign');await page.screenshot({path:resolve(screenshots,'raw-playback-url-dialog.png'),animations:'disabled'})
})
