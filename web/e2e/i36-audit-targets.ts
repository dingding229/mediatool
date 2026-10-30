import {expect,type Page,type Locator} from '@playwright/test';
import {i3Fixture} from './i3-fixture';
type Target={id:string;file:string;path:string;title:string;size?:'default'|'large';open:(p:Page)=>Promise<void>};
const button=(name:string)=>(p:Page)=>p.getByRole('button',{name,exact:true}).first().click();
const summary=async(p:Page,text:string)=>{const s=p.locator('summary').filter({hasText:text}).first();await s.click()};
export const targets:Target[]=[
 {id:'discovery',file:'acquisition/DiscoveryPanel.vue',path:'/search?tab=discover',title:'海边的故事',size:'default',open:p=>p.locator('.discovery-grid .catalog-poster').first().click()},
 {id:'search-history',file:'acquisition/SearchHistory.vue',path:'/search?tab=history',title:'搜索记录详情',size:'default',open:button('查看记录详情')},
 {id:'download',file:'views/DownloadsWorkspaceView.vue',path:'/downloads?tab=tasks',title:'任务详情',open:p=>p.locator('.download-main h3 button').first().click()},
 {id:'manual',file:'acquisition/ManualWorkspace.vue',path:'/downloads?tab=manual',title:'手动下载详情',open:button('查看详情')},
 {id:'browser',file:'files/BrowserPanel.vue',path:'/files?tab=browser',title:'文件操作记录',open:button('操作记录')},
 {id:'managed-strm',file:'files/ManagedSTRMPanel.vue',path:'/files?tab=strm',title:'受管 STRM 与媒体详情',open:button('预览STRM内容')},
 {id:'queue',file:'operations/QueuePanel.vue',path:'/ops?tab=jobs',title:'任务详情',open:button('任务详情')},
 {id:'runtime',file:'operations/RuntimeChecks.vue',path:'/ops?tab=attention',title:'运行检查详情',size:'default',open:async p=>{await summary(p,'运行计划与执行器详情');await button('执行器详情')(p)}},
 {id:'maintenance',file:'operations/MaintenancePanel.vue',path:'/ops?tab=maintenance',title:'维护详情',open:button('备份详情')},
 {id:'restore',file:'operations/WebRestorePanel.vue',path:'/ops?tab=maintenance',title:'网页恢复预览',open:async p=>{await button('恢复与回滚')(p);await button('预览网页恢复')(p)}},
 {id:'log',file:'operations/LogsPanel.vue',path:'/ops?tab=logs',title:'完整原始日志',open:button('查看完整链接')},
 {id:'log-settings',file:'operations/LogsPanel.vue',path:'/ops?tab=logs',title:'日志设置',open:button('日志设置')},
 {id:'cache',file:'operations/CachePanel.vue',path:'/ops?tab=cache',title:'缓存内容',open:button('缓存详情')},
 {id:'asset',file:'views/OrganizeWorkspaceView.vue',path:'/organize?tab=assets',title:'资产与输出详情',open:button('查看资产详情')},
 {id:'scan',file:'organize/ScanPanel.vue',path:'/organize?tab=scans',title:'扫描目录覆盖',open:button('查看目录记录')},
 {id:'history',file:'organize/HistoryPanel.vue',path:'/organize?tab=history',title:'整理步骤详情',open:button('查看步骤详情')},
 {id:'users',file:'playback/UsersPanel.vue',path:'/playback?tab=users',title:'family · 用户详情',open:button('用户详情')},
 {id:'access',file:'playback/AccessPanel.vue',path:'/playback?tab=players',title:'完整 UA 与访问详情',open:button('查看完整 UA')},
 {id:'sessions',file:'playback/SessionsPanel.vue',path:'/playback?tab=sessions',title:'在线会话详情',open:button('会话详情')},
 {id:'accounts',file:'sites/AccountsPanel.vue',path:'/sites?tab=accounts',title:'站点数据与趋势',open:p=>p.locator('.f5-site-name').first().click()},
 {id:'templates',file:'sites/TemplatesPanel.vue',path:'/sites?tab=templates',title:'内置模板详情',size:'default',open:button('模板详情')},
 {id:'media',file:'views/MediaWorkspaceView.vue',path:'/library',title:'作品详情',size:'large',open:p=>p.locator('.catalog-poster').first().click()},
 {id:'plugin',file:'views/PluginsWorkspaceView.vue',path:'/plugins?tab=catalog',title:'插件权限确认',open:async p=>{await button('新增独立实例')(p);const consent=p.getByRole('dialog',{name:'插件权限确认',exact:true});await consent.getByRole('checkbox').check();await consent.getByRole('button',{name:'确认安装插件',exact:true}).click();await expect(consent.locator('details')).toHaveCount(0);await expect(consent.getByText('安装将创建独立实例，默认停用；启用后将以本服务权限运行，请只选择可信插件。',{exact:true})).toBeVisible()}},
 {id:'subscription',file:'views/SubscriptionWorkspaceView.vue',path:'/subscriptions?tab=list',title:'订阅详情',open:p=>p.locator('.subscription-title').first().click()},
 {id:'subscription-run',file:'views/SubscriptionWorkspaceView.vue',path:'/subscriptions?tab=history&run=i3-run',title:'运行详情',open:button('完整运行详情')},
 {id:'external',file:'settings/ExternalAccessPanel.vue',path:'/settings?tab=api',title:'凭据详情',open:button('详情')},
];
export async function navigate(page:Page,t:Target){
 if(new URL(page.url()).pathname!==t.path.split('?')[0])await i3Fixture(page,t.path.split('?')[0]);
 const current=new URL(page.url());if(current.pathname+current.search!==t.path)await page.goto(t.path);await t.open(page);const dialog=page.getByRole('dialog',{name:t.title,exact:true});await expect(dialog).toBeVisible();return dialog;
}
