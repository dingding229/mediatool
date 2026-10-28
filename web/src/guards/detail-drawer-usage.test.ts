import {expect,it} from 'vitest'
import {readFileSync,readdirSync} from 'node:fs'
import {fileURLToPath} from 'node:url'
import {join,relative} from 'node:path'

const root=fileURLToPath(new URL('..',import.meta.url))
function sources(dir:string):string[]{return readdirSync(dir,{withFileTypes:true}).flatMap(e=>e.isDirectory()?sources(join(dir,e.name)):/\.(vue|ts)$/.test(e.name)&&!e.name.includes('.test.')?[join(dir,e.name)]:[])}
function usesDrawer(text:string){return /\bel-drawer\b|\bElDrawer\b/.test(text)}

it('business UI uses DetailDrawer; only mobile navigation retains a direct drawer',()=>{
 const violations:string[]=[]
 for(const file of sources(root)){
  const name=relative(root,file).replaceAll('\\','/'),text=readFileSync(file,'utf8')
  if(name==='layouts/AppLayout.vue'||name==='ui/DetailDrawer.vue')continue
  if(name==='ui/element-components.ts'){
   // The sole lazy global registration supports AppLayout's unchanged mobile menu.
   expect(text.match(/\bElDrawer\b/g)).toHaveLength(2)
   expect(text).toContain("ElDrawer:lazy(()=>import('element-plus/es/components/drawer/index').then(m=>m.ElDrawer)")
   continue
  }
  if(usesDrawer(text))violations.push(name)
 }
 expect(violations).toEqual([])
 const layout=readFileSync(join(root,'layouts/AppLayout.vue'),'utf8')
 expect(layout).toContain('<el-drawer v-model="menuOpen" direction="ltr"')
})

it.each(['<el-drawer />','<ElDrawer />',"import {ElDrawer} from 'element-plus'","const drawer = resolveComponent('el-drawer')"])(
 'detects a new direct drawer: %s',text=>expect(usesDrawer(text)).toBe(true),
)
it('external credential detail preserves the close adapter and supplied footer',()=>{
 const text=readFileSync(join(root,'settings/ExternalAccessPanel.vue'),'utf8')
 expect(text).toContain('<DetailDrawer :visible="!!detail"')
 expect(text).toContain('@update:visible="closeDetail"')
 expect(text).toContain('<template #footer><el-button @click="calls.load">刷新记录</el-button>')
 expect(text).toContain('function closeDetail(value:boolean){if(!value)detail.value=undefined}')
})

it('keeps the audited DetailDrawer size map explicit',()=>{
 const large=new Set([
  'acquisition/ManualWorkspace.vue','files/BrowserPanel.vue','files/ManagedSTRMPanel.vue','files/STRMRepairPanel.vue',
  'operations/CachePanel.vue','operations/LogsPanel.vue','operations/MaintenancePanel.vue','operations/QueuePanel.vue','operations/WebRestorePanel.vue',
  'organize/ArchivePanel.vue','organize/HistoryPanel.vue','organize/ScanPanel.vue',
  'playback/AccessPanel.vue','playback/ProcessingPanel.vue','playback/ServicePanel.vue','playback/SessionsPanel.vue','playback/UsersPanel.vue',
  'settings/ExternalAccessPanel.vue','sites/AccountsPanel.vue','views/DownloadsWorkspaceView.vue','views/MediaWorkspaceView.vue','views/OrganizeWorkspaceView.vue','views/PluginsWorkspaceView.vue','views/SubscriptionWorkspaceView.vue',
 ])
 let fileCount=0,instanceCount=0
 for(const file of sources(root)){
  const name=relative(root,file).replaceAll('\\','/'),text=readFileSync(file,'utf8'),matches=[...text.matchAll(/<DetailDrawer\b[^>]*>/g)]
  if(!matches.length)continue
  fileCount++;instanceCount+=matches.length
  for(const match of matches){
   const tag=match[0]
   if(large.has(name))expect(tag,`${name}: every audited instance must be large`).toContain('size="large"')
   else if(name==='ui/CronInput.vue')expect(tag,'Cron generator uses the shared compact dialog').toContain('size="small"')
   else expect(tag,`${name}: default-sized instance must not opt into a different width`).not.toMatch(/\ssize=/)
  }
 }
 expect(fileCount).toBe(29)
 expect(instanceCount).toBe(31)
})
