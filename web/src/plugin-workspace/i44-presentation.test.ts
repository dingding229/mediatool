import {expect,it} from 'vitest';import {readFileSync} from 'node:fs';
import {createSSRApp} from 'vue';import {renderToString} from 'vue/server-renderer';import Json from '../plugins/PluginJson';
import inventory from '../../../internal/modules/plugins/v2/library.json';
import {extensionTitle,extensionDanger,runTitle} from './extension-presentation';
const d={kind:'command',id:'downloader-status',cmd:'downloader-status',handler:'command.hash',requiredScopes:['plugin.data.read']};
it('commands use cmd and missing service names use readable IDs; real names remain intact',()=>{expect(extensionTitle(d)).toBe('downloader status');expect(extensionTitle({id:'some.service_id'})).toBe('some service id');expect(extensionTitle({...d,name:'危险：按已确认配置运行（默认仅预览）'})).toBe('按已确认配置运行（默认仅预览）');expect(extensionTitle({...d,name:'真实.name'})).toBe('真实.name');expect(extensionDanger(d)).toBe(false);expect(extensionDanger({...d,name:'危险：处理'})).toBe(true)});
it('run labels use historical backend names first, matching descriptors second and never raw hashes',()=>{const r={id:'r',kind:'command',handler:'command.hash',state:'succeeded',reason:'',createdAt:'2026-11-03'};expect(runTitle({...r,operationName:'旧时操作名'},[d])).toBe('旧时操作名');expect(runTitle(r,[d])).toBe('downloader status');expect(runTitle({...r,handler:'command.unknown'},[])).toBe('状态查询')});
it('history omits UI reads, empty reason rows and verbose action labels while preserving pagination',()=>{const s=readFileSync(new URL('./ExtensionsPanel.vue',import.meta.url),'utf8');expect(s).toContain("filter(r=>r.kind!=='ui')");expect(s).toContain('v-if="d.description?.trim()"');expect(s).toContain('v-if="r.reason"');expect(s).toContain('>执行</el-button>');expect(s).toContain('>日志</RouterLink>');expect(s).not.toContain('未命名服务');expect(s.match(/<PagePagination /g)).toHaveLength(1)});
it('switch hint is grouped by the shared form token and never becomes an unlabeled control',async()=>{const html=await renderToString(createSSRApp(Json,{nodes:[{component:'VSwitch',model:'write',props:{label:'调整下载设置',hint:'危险操作，请核对'}}],model:{write:false}}));expect(html).toContain('plugin-json-switch');expect(html).toContain('plugin-json-hint');expect(html).toContain('aria-label="调整下载设置"')});

it('common operations and history resolve to the same manifest Chinese title, never a host plugin dictionary',()=>{
 const titles=(inventory.manifests.find(m=>m.id==='torrent-assistant')!.extensions as {titles:Record<string,string>}).titles;
 for(const [kind,id,handler]of [['command','downloader-status','command.status'],['service','assist','service.assist']]){
  const descriptor={kind:kind!,id:id!,handler:handler!,requiredScopes:[],name:''};
  const record={id:'run',kind:kind!,handler:handler!,state:'succeeded',reason:'',createdAt:'2026-11-03',operationName:'历史旧名称'};
  const heading=extensionTitle(descriptor,[record],titles),history=runTitle(record,[descriptor],titles,[record]);
  expect(heading).toBe(titles[`${kind}:${id}`]);expect(history).toBe(heading);expect(heading).toMatch(/\p{Script=Han}/u);
  expect(extensionTitle(descriptor,[record],{})).toBe('历史旧名称');expect(runTitle(record,[descriptor],{},[record])).toBe('历史旧名称');
 }
 const source=readFileSync(new URL('./extension-presentation.ts',import.meta.url),'utf8');expect(source).not.toMatch(/torrent-assistant|downloader-status|下载器状态|下载辅助检查/);
});
