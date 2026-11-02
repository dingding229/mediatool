import type{Page}from'@playwright/test';import{fixture}from'./i41b-fixture';import{readFileSync}from'node:fs';
const inventory=JSON.parse(readFileSync(new URL('../../internal/modules/plugins/v2/library.json',import.meta.url),'utf8'));
export async function i42Fixture(page:Page){const f=await fixture(page);const at='2026-11-02T00:00:00Z';
 const instances=inventory.manifests.map((metadata:any,i:number)=>({id:`11111111-1111-4111-8111-${String(i+1).padStart(12,'0')}`,pluginId:metadata.id,metadata,desiredState:metadata.id==='torrent-assistant'?'enabled':'disabled',state:metadata.id==='torrent-assistant'?'running':'stopped',revision:1,reason:'',availability:'available'}));
 const catalog={items:instances.map((i:any)=>({manifest:i.metadata,fingerprint:'i42-'+i.pluginId,installedCount:1})),total:instances.length};
 await page.route('**/api/v1/plugins?*',r=>r.fulfill({json:{data:instances}}));await page.route('**/api/v1/plugins/catalog?*',r=>r.fulfill({json:{data:catalog}}));
 await page.route('**/api/v1/plugins/ui-surfaces',r=>r.fulfill({json:{data:instances.filter((i:any)=>i.state==='running').map((i:any)=>({instanceId:i.id,pluginId:i.pluginId}))}}));
 await page.route('**/api/v1/plugins/*',r=>{const id=new URL(r.request().url()).pathname.split('/').pop(),row=instances.find((i:any)=>i.id===id);return row?r.fulfill({json:{data:{instance:row,config:{value:inventory.forms[row.pluginId]?.defaults||{},revision:1},fingerprint:'i42-'+row.pluginId}}}):r.fallback()});
 await page.route('**/api/v1/plugins/*/extensions',r=>r.fulfill({json:{data:{id:'snapshot',metadata:{descriptors:[]},hash:'fixture'}}}));await page.route('**/api/v1/plugins/*/runs?*',r=>r.fulfill({json:{data:[]}}));
 await page.route('**/api/v1/plugins/*/ui/meta?*',r=>r.fulfill({json:{data:[{key:'overview',title:'只读状态',refresh:60}]}}));
 const content=[{component:'VRow',children:[{component:'Text',props:{label:'状态',text:'尚未运行'}},{component:'Text',props:{label:'任务',text:'0'}},{component:'Text',props:{label:'显示范围',text:'前20项'}}]}];
 await page.route('**/api/v1/plugins/*/ui/{page,dashboard}?*',r=>r.fulfill({json:{data:{content}}}));
 await page.route('**/api/v1/plugins/workflows?*',r=>r.fulfill({json:{data:[]}}));
 const rows=[{ip:'8.8.8.8',location:'中国 浙江省 杭州市',isp:'示例运营商',users:1,devices:1,sessionCount:1,direct302Count:2,count:3,firstSeenAt:at,lastSeenAt:at,accounts:[{id:'i41-user',name:'家人'}],clients:[{id:'device',name:'客厅播放器',client:'测试播放器'}]},{ip:'9.9.9.9',location:'中国 广东省 深圳市',isp:'另一个运营商',users:0,devices:0,sessionCount:0,direct302Count:4,count:4,firstSeenAt:at,lastSeenAt:at,accounts:[],clients:[]}];
 await page.route('**/api/v1/media-server/playback-ips?*',r=>{const url=new URL(r.request().url()),scoped=!!url.searchParams.get('userId')||!!url.searchParams.get('deviceId');const items=scoped?[{...rows[0],direct302Count:0,count:1}]:rows;return r.fulfill({json:{data:{items,total:items.length,summary:{uniqueIPs:items.length,regions:items.length,activeUsers:1,primaryRegion:'中国 浙江省 杭州市',provinces:items.map(row=>({name:row.location,count:1})),cities:items.map(row=>({name:row.location,count:1})),operators:items.map(row=>({name:row.isp,count:1}))}}}})});
 return{instances,catalog,content,f};}
