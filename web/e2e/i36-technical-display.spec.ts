import {test,expect} from '@playwright/test';
import {mkdirSync,writeFileSync} from 'node:fs';import {resolve} from 'node:path';
import {targets,navigate} from './i36-audit-targets';import {i3Fixture} from './i3-fixture';
import {waitForLayout} from './layout-stability';import {fixture,open} from './i35-identity-fixture';
const output=resolve(import.meta.dirname,'../../scratchpad/i-phase/i36');
const capture=process.env.I36_SCREENSHOTS==='1'&&process.env.CI!=='true'&&process.env.GITHUB_ACTIONS!=='true';
const business:Record<string,string[]>={
 discovery:['海边的故事','搜索资源'],'search-history':['海边的故事','已完成'],download:['海边的故事','文件已入库'],manual:['手动添加的种子','保存路径'],browser:['文件变更','已完成'],'managed-strm':['海边的故事','读取原文'],queue:['系统检查','已完成','默认队列','执行次数'],runtime:['默认队列','在线','最近在线'],maintenance:['备份就绪','备份大小','数据库与受管输出已打包'],restore:['只读核验预览','高风险提示','SHA256'],log:['完整请求 URL','完整 302 URL','复制日志'],'log-settings':['日志大小','日志文件'],cache:['海边的故事'],asset:['海边的故事','当前输出与 STRM'],scan:['/115/媒体','目录覆盖说明'],history:['原位置','目标位置','/电影/核对目录 30'],users:['设备与登录','权限摘要'],access:['实际收到的完整 UA','复制完整 UA'],sessions:['家庭成员','验收播放器'],accounts:['家庭站点','站内用户名','做种体积'],templates:['家庭站点','https://hdsky.me','管理站点账号'],media:['海边的故事','作品简介','版本与文件'],plugin:['确认安装插件','权限'],subscription:['海边的故事','监控站点','等待下次检查'],'subscription-run':['候选决策','已完成'],external:['只读核验凭据','全部权限','最近调用']
};
for(const width of [1440,375])for(const target of targets)test(`I36 business detail ${target.id} ${width}`,async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await page.setViewportSize({width,height:1000});
 await i3Fixture(page,target.path.split('?')[0]);await page.goto(target.path);await page.waitForLoadState('networkidle');await waitForLayout(page);
 // Before I36 every audited dialog measured CLS=0 with these same mocked paths (local evidence).
 await page.evaluate(()=>{(window as any).i36CLS=0;new PerformanceObserver(list=>{for(const entry of list.getEntries())if(!(entry as any).hadRecentInput)(window as any).i36CLS+=(entry as any).value}).observe({type:'layout-shift',buffered:false})});
 await target.open(page);const dialog=page.getByRole('dialog',{name:target.title,exact:true});await expect(dialog).toBeVisible();await waitForLayout(page);await page.waitForTimeout(600);
 await expect(dialog.locator('.technical-section,.technical-json,.technical-toggle')).toHaveCount(0);
 await expect(dialog).not.toContainText(/技术细节|技术详情|复制原始记录/);
 for(const text of business[target.id]||[])await expect(dialog).toContainText(text);
 expect(await page.evaluate(()=>(window as any).i36CLS)).toBe(0);expect(errors).toEqual([]);
 expect(await page.evaluate(()=>document.documentElement.scrollWidth-innerWidth)).toBeLessThanOrEqual(0);
 if(capture){mkdirSync(resolve(output,'screens'),{recursive:true});writeFileSync(resolve(output,`after-${target.id}-${width}.json`),JSON.stringify({cls:await page.evaluate(()=>(window as any).i36CLS)}));if(width===1440&&['search-history','accounts','history'].includes(target.id))await page.screenshot({path:resolve(output,`screens/after-${target.id}-1440-light.png`),animations:'disabled'})}
});
for(const width of [1440,375])test(`I36 episode markers remain visible in each list and identity row ${width}`,async({page})=>{
 await page.setViewportSize({width,height:1000});await fixture(page,'batch');await page.goto('/organize?tab=candidates');await expect(page.locator('.organize-candidate-row')).toHaveCount(8);await waitForLayout(page);
 const check=async(selector:string)=>{const values=await page.locator(selector).allTextContents();expect(values).toHaveLength(8);expect(new Set(values).size).toBe(8);for(const [i,text]of values.entries()){expect(text).toContain('S01E'+String(i+1).padStart(2,'0'));expect(text).toContain('2026');expect(text).toMatch(/\.mkv$/)}};
 await check('.organize-candidate-row .organize-list-identity .middle-ellipsis-text');if(capture)await page.screenshot({path:resolve(output,`screens/after-candidates-${width}-light.png`),animations:'disabled'});
 const dialog=await open(page,'batch');await dialog.locator('[aria-label="2 核对季集"]').scrollIntoViewIfNeeded();await waitForLayout(page);await check('.identity-file-name .middle-ellipsis-text');if(capture)await page.screenshot({path:resolve(output,`screens/after-identity-${width}-light.png`),animations:'disabled'});
});
for(const [path,heading]of [['/playback?tab=settings','播放如何到达你的设备'],['/playback?tab=processing','预处理'],['/trends','运行趋势'],['/files?tab=tools','修复']]as const)test('I36 removes raw-only controls but preserves panel '+path,async({page})=>{
 await i3Fixture(page,new URL(path,'http://local').pathname);await page.goto(path);await expect(page.getByRole('heading').filter({hasText:heading}).first()).toBeVisible();await expect(page.getByRole('button',{name:/技术详情|查看原始采样/})).toHaveCount(0);await expect(page.locator('.technical-json,.technical-section')).toHaveCount(0);
});
test('I36 workflow management keeps editable definitions and readable outcomes, never raw definition/run records',async({page})=>{
 await i3Fixture(page,'/plugins');const mutations:string[]=[];
 await page.route('**/api/v1/plugins/workflows?*',r=>{const query=new URL(r.request().url()).searchParams;const dag={id:'fixture-flow',steps:[{id:'first',instanceId:'owned-instance',action:'notify',dependsOn:[],input:{data:{}}}]};const data=query.get('view')==='definition'?{dag,revision:1,fingerprint:'approved-fixture',approved:'approved-fixture',internalDebug:'i36-private-workflow-object'}:query.get('view')==='runs'?[{id:'internal-run',workflowId:dag.id,state:'failed',steps:{first:'failed'},reasons:{first:'站点连接暂不可用'},internalDebug:'i36-private-run-object'}]:[{id:dag.id,steps:1,fingerprint:'approved-fixture',approved:'approved-fixture'}];return r.fulfill({json:{data}})});
 await page.route('**/api/v1/plugins/workflows',r=>{mutations.push(r.request().method());return r.fulfill({status:500,json:{error:{message:'No mutations in this view test'}}})});
 await page.goto('/plugins');await page.getByRole('button',{name:'工作流管理',exact:true}).click();const dialog=page.getByRole('dialog',{name:'工作流管理',exact:true});await dialog.getByRole('button',{name:'详情',exact:true}).click();
 await expect(dialog.getByLabel('工作流定义（JSON）',{exact:true})).toHaveValue(/"fixture-flow"/);await expect(dialog).toContainText('站点连接暂不可用');await expect(dialog).toContainText('结果未知或失败不会自动重放');await expect(dialog.locator('pre,.technical-json')).toHaveCount(0);await expect(dialog).not.toContainText(/i36-private-workflow-object|i36-private-run-object|技术详情/);expect(mutations).toEqual([]);
});
for(const width of [1440,375])test(`I36 shared compact Cron dialog remains stable ${width}`,async({page})=>{
 await page.setViewportSize({width,height:1000});await i3Fixture(page,'/settings');await page.goto('/settings?tab=automation');await page.waitForLoadState('networkidle');await waitForLayout(page);
 await page.evaluate(()=>{(window as any).i36CLS=0;new PerformanceObserver(list=>{for(const entry of list.getEntries())if(!(entry as any).hadRecentInput)(window as any).i36CLS+=(entry as any).value}).observe({type:'layout-shift',buffered:false})});
 await page.getByRole('button',{name:'订阅调度时间生成',exact:true}).click();const dialog=page.getByRole('dialog',{name:'订阅调度时间生成器',exact:true});await expect(dialog).toBeVisible();await waitForLayout(page);await page.waitForTimeout(600);await expect(dialog.getByRole('button',{name:'确定',exact:true})).toBeVisible();await expect(dialog.locator('.technical-section,.technical-json')).toHaveCount(0);expect(await page.evaluate(()=>(window as any).i36CLS)).toBe(0);
 if(capture)writeFileSync(resolve(output,`after-cron-${width}.json`),JSON.stringify({cls:await page.evaluate(()=>(window as any).i36CLS)}));
});
