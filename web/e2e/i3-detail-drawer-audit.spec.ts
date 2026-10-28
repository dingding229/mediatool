import {test,expect,type Page,type Locator} from '@playwright/test';
import {mkdirSync,writeFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {i3Fixture} from './i3-fixture';

type Target={id:string;file:string;path:string;title:string;size?:'default'|'large';open:(p:Page)=>Promise<void>};
const button=(name:string)=>(p:Page)=>p.getByRole('button',{name,exact:true}).first().click();
const summary=async(p:Page,text:string)=>{const s=p.locator('summary').filter({hasText:text}).first();await s.click()};
export const targets:Target[]=[
 {id:'discovery',file:'acquisition/DiscoveryPanel.vue',path:'/search?tab=discover',title:'海边的故事',size:'default',open:p=>p.locator('.discovery-grid .catalog-poster').first().click()},
 {id:'search-history',file:'acquisition/SearchHistory.vue',path:'/search?tab=history',title:'搜索记录详情',size:'default',open:button('查看记录详情')},
 {id:'download',file:'views/DownloadsWorkspaceView.vue',path:'/downloads?tab=tasks',title:'任务详情',open:button('查看详情')},
 {id:'manual',file:'acquisition/ManualWorkspace.vue',path:'/downloads?tab=manual',title:'手动下载详情',open:button('查看详情')},
 {id:'browser',file:'files/BrowserPanel.vue',path:'/files?tab=browser',title:'文件操作记录',open:button('操作记录')},
 {id:'managed-strm',file:'files/ManagedSTRMPanel.vue',path:'/files?tab=strm',title:'受管 STRM 与媒体详情',open:button('预览STRM内容')},
 {id:'repair',file:'files/STRMRepairPanel.vue',path:'/files?tab=tools',title:'修复技术详情',open:async p=>{await button('选择受管资产')(p);await p.locator('.files-repair-asset').getByRole('checkbox').first().check();await button('检查已选 1 个资产')(p);await button('技术详情')(p)}},
 {id:'queue',file:'operations/QueuePanel.vue',path:'/ops?tab=jobs',title:'任务与运行技术详情',open:button('任务详情')},
 {id:'runtime',file:'operations/RuntimeChecks.vue',path:'/ops?tab=attention',title:'运行检查详情',size:'default',open:async p=>{await summary(p,'运行计划与执行器详情');await button('执行器详情')(p)}},
 {id:'maintenance',file:'operations/MaintenancePanel.vue',path:'/ops?tab=maintenance',title:'维护技术详情',open:button('备份详情')},
 {id:'restore',file:'operations/WebRestorePanel.vue',path:'/ops?tab=maintenance',title:'网页恢复预览',open:async p=>{await button('恢复与回滚')(p);await button('预览网页恢复')(p)}},
 {id:'log',file:'operations/LogsPanel.vue',path:'/ops?tab=logs',title:'完整原始日志',open:button('查看完整链接')},
 {id:'log-settings',file:'operations/LogsPanel.vue',path:'/ops?tab=logs',title:'日志设置',open:button('日志设置')},
 {id:'cache',file:'operations/CachePanel.vue',path:'/ops?tab=cache',title:'缓存内容',open:button('缓存详情')},
 {id:'asset',file:'views/OrganizeWorkspaceView.vue',path:'/organize?tab=assets',title:'资产与输出详情',open:button('查看资产详情')},
 {id:'scan',file:'organize/ScanPanel.vue',path:'/organize?tab=scans',title:'扫描目录覆盖',open:button('查看目录记录')},
 {id:'history',file:'organize/HistoryPanel.vue',path:'/organize?tab=history',title:'整理步骤详情',open:button('查看步骤详情')},
 {id:'service',file:'playback/ServicePanel.vue',path:'/playback?tab=settings',title:'服务技术详情',open:button('技术详情')},
 {id:'users',file:'playback/UsersPanel.vue',path:'/playback?tab=users',title:'family · 用户详情',open:button('用户详情')},
 {id:'access',file:'playback/AccessPanel.vue',path:'/playback?tab=players',title:'完整 UA 与访问详情',open:button('查看完整 UA')},
 {id:'sessions',file:'playback/SessionsPanel.vue',path:'/playback?tab=sessions',title:'在线会话详情',open:button('会话详情')},
 {id:'processing',file:'playback/ProcessingPanel.vue',path:'/playback?tab=processing',title:'预处理技术详情',open:button('技术详情')},
 {id:'accounts',file:'sites/AccountsPanel.vue',path:'/sites?tab=accounts',title:'站点数据与趋势',open:p=>p.locator('.f5-site-name').first().click()},
 {id:'templates',file:'sites/TemplatesPanel.vue',path:'/sites?tab=templates',title:'内置模板详情',size:'default',open:button('模板详情')},
 {id:'media',file:'views/MediaWorkspaceView.vue',path:'/library',title:'海边的故事',size:'large',open:p=>p.locator('.catalog-poster').first().click()},
 {id:'plugin',file:'views/PluginsWorkspaceView.vue',path:'/plugins?tab=catalog',title:'操作技术详情',open:async p=>{await button('新增独立实例')(p);const consent=p.getByRole('dialog',{name:'插件权限确认',exact:true});await consent.getByRole('checkbox').check();await consent.getByRole('button',{name:'确认安装插件',exact:true}).click();await expect(consent.locator('details')).toBeVisible();await consent.getByRole('button',{name:'取消',exact:true}).click();await button('查看技术详情')(p)}},
 {id:'subscription',file:'views/SubscriptionWorkspaceView.vue',path:'/subscriptions?tab=list',title:'订阅详情',open:p=>p.locator('.subscription-title').first().click()},
 {id:'subscription-run',file:'views/SubscriptionWorkspaceView.vue',path:'/subscriptions?tab=history&run=i3-run',title:'运行详情',open:button('完整运行详情')},
 {id:'external',file:'settings/ExternalAccessPanel.vue',path:'/settings?tab=api',title:'凭据详情',open:button('详情')},
];
const representative=new Set(['media','users','log','accounts','history','external']);
async function navigate(page:Page,t:Target){
 if(new URL(page.url()).pathname!==t.path.split('?')[0])await i3Fixture(page,t.path.split('?')[0]);
 const current=new URL(page.url());if(current.pathname+current.search!==t.path)await page.goto(t.path);await t.open(page);const dialog=page.getByRole('dialog',{name:t.title,exact:true});await expect(dialog).toBeVisible();return dialog;
}
async function geometry(dialog:Locator,width:number,expected:number){
 const panel=dialog.locator('.el-dialog.detail-dialog');
 await expect.poll(()=>panel.evaluate((el,expected)=>{const r=el.getBoundingClientRect();return innerWidth<600?Math.abs(r.x)<1&&Math.abs(r.y)<1&&Math.abs(r.width-innerWidth)<1&&Math.abs(r.height-innerHeight)<1:r.width===expected&&r.height<=innerHeight*.85+1&&Math.abs(r.x-(innerWidth-r.width)/2)<2&&Math.abs(r.y-(innerHeight-r.height)/2)<2},expected)).toBe(true);
 await expect(panel.locator('.el-dialog__body')).toHaveCSS('overflow-y','auto');
 expect(await panel.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true);
 if(width===390)expect(await panel.evaluate(el=>el.getBoundingClientRect().height)).toBe(1000);
}

async function checkFooterAlignment(dialog:Locator){
 const footerRow=dialog.locator('.el-dialog__footer');
 await expect.poll(()=>footerRow.evaluate(el=>{const style=getComputedStyle(el),row=el.getBoundingClientRect(),wrap=el.querySelector('.detail-footer')!.getBoundingClientRect(),button=el.querySelector('.detail-footer button:last-child')!.getBoundingClientRect(),left=row.left+el.clientLeft+parseFloat(style.paddingLeft),right=row.left+el.clientLeft+el.clientWidth-parseFloat(style.paddingRight);return Math.abs(wrap.left-left)<1&&Math.abs(wrap.right-right)<1&&Math.abs(button.right-right)<1})).toBe(true);
}

// One browser context/page and mocked authenticated session for the whole audit; no retries or timeout changes.
let shared:Page;
test.beforeAll(async({browser})=>{shared=await browser.newPage({viewport:{width:1440,height:1000}})});
test.afterAll(async()=>{await shared.close()});
for(const group of ['/search','/downloads','/files','/ops','/organize','/playback','/sites','/library','/plugins','/subscriptions','/settings'])test(`I3 behavior ${group}`,async()=>{
 for(const t of targets.filter(t=>t.path.split('?')[0]===group)){
  await test.step(t.id,async()=>{
   const dialog=await navigate(shared,t);
   if(representative.has(t.id)){
    await geometry(dialog,1440,t.size==='default'?800:1000);
    if(t.id==='external'){await checkFooterAlignment(dialog);await expect(dialog.locator('.el-dialog__footer')).toBeVisible();await expect(dialog.getByRole('button',{name:'刷新记录',exact:true})).toBeVisible();await shared.mouse.click(2,2);await expect(dialog).toBeVisible()}
    else await expect(dialog.locator('.el-dialog__footer')).toHaveCount(0);
   }
   await dialog.getByRole('button',{name:'关闭此对话框',exact:true}).click();await expect(dialog).toBeHidden();
  });
 }
});

test('I3 behavior mobile representatives and footer',async()=>{
 await shared.setViewportSize({width:390,height:1000});
 for(const id of ['media','users','log','accounts','history','external']){
  const t=targets.find(t=>t.id===id)!;
  await test.step(id,async()=>{
   const dialog=await navigate(shared,t);await geometry(dialog,390,t.size==='default'?800:1000);
   if(id==='external'){
    const footer=dialog.locator('.detail-footer');await expect(footer).toHaveCSS('justify-content','flex-end');
    await checkFooterAlignment(dialog);
    const header=dialog.locator('.el-dialog__header'),body=dialog.locator('.el-dialog__body');
    const top=(await header.boundingBox())!.y,bottom=(await footer.boundingBox())!.y;
    expect(await body.evaluate(el=>el.scrollHeight>el.clientHeight)).toBe(true);
    await body.evaluate(el=>{el.scrollTop=el.scrollHeight});
    expect(Math.abs((await header.boundingBox())!.y-top)).toBeLessThan(1);
    expect(Math.abs((await footer.boundingBox())!.y-bottom)).toBeLessThan(1);
    await dialog.getByRole('button',{name:'关闭详情',exact:true}).click();
   }else await shared.keyboard.press('Escape');
   await expect(dialog).toBeHidden();
  });
 }
 await shared.setViewportSize({width:1440,height:1000});
});

const capture=process.env.I3_SCREENSHOTS==='1'&&process.env.CI!=='true'&&process.env.GITHUB_ACTIONS!=='true';
const before=process.env.I3_BEFORE==='1';
const output=resolve(import.meta.dirname,'../../scratchpad/i-phase/i3/screens');
const mobile=new Set(['media','users','log','accounts','history','external']);
test.describe('I3 visual review @screenshots',()=>{
 test.skip(!capture,'Local opt-in only: I3_SCREENSHOTS=1');
 for(const t of targets)for(const width of [1440,...(mobile.has(t.id)?[390]:[])])for(const theme of ['light','dark'])test(`${t.id} ${width} ${theme}`,async({page})=>{
  await page.setViewportSize({width,height:1000});await page.addInitScript(mode=>localStorage.setItem('media-tool:theme-mode:v1',mode),theme);
  await i3Fixture(page,t.path.split('?')[0]);const dialog=await navigate(page,t);
  if(!before)await geometry(dialog,width,t.size==='default'?800:1000);
  mkdirSync(output,{recursive:true});const path=resolve(output,`${before?'before':'after'}-${t.id}-${width}-${theme}.png`);
  await page.screenshot({path,animations:'disabled'});
  writeFileSync(path+'.json',JSON.stringify({file:t.file,title:t.title,size:t.size||'large',width,theme}));
 });
});
