import {beforeEach,afterEach,expect,it,vi} from 'vitest';
import * as Vue from 'vue';
import {createRenderer,defineComponent,h,reactive,nextTick,ssrContextKey} from 'vue';
import {parse,compileScript,compileTemplate} from '@vue/compiler-sfc';
import {readFileSync} from 'node:fs';
import Card from './MediaSearchCard.vue';
import {useSearch} from './useSearch';
import {request,post} from '../api/client';
import {flush,deferred} from '../test/setup-component';
vi.mock('vue-router',()=>({useRouter:()=>({push:vi.fn()})}));
vi.mock('../api/client',async original=>({...await original<typeof import('../api/client')>(),request:vi.fn(),post:vi.fn()}));
interface Node{tag:string;text:string;props:Record<string,any>;children:Node[];parent:Node|null}
const make=(tag='',text=''):Node=>({tag,text,props:{},children:[],parent:null});
function detach(n:Node){if(n.parent){n.parent.children.splice(n.parent.children.indexOf(n),1);n.parent=null}}
const renderer=createRenderer<Node,Node>({createElement:t=>make(t),createText:t=>make('#text',t),createComment:t=>make('#comment',t),insert(n,p,anchor){detach(n);const i=anchor?p.children.indexOf(anchor):-1;p.children.splice(i<0?p.children.length:i,0,n);n.parent=p},remove:detach,setText:(n,t)=>{n.text=t},setElementText:(n,t)=>{n.text=t;n.children=[]},patchProp:(n,k,_,v)=>{n.props[k]=v},parentNode:n=>n.parent,nextSibling:n=>n.parent?.children[n.parent.children.indexOf(n)+1]||null});
const cleanups:(()=>void)[]=[];
beforeEach(()=>{const store=new Map<string,string>();vi.stubGlobal('sessionStorage',{getItem:(key:string)=>store.get(key)||null,setItem:(key:string,value:string)=>store.set(key,value),removeItem:(key:string)=>store.delete(key)})});
afterEach(()=>{cleanups.splice(0).forEach(f=>f());vi.unstubAllGlobals()});
const item={id:7,mediaType:'series',title:'作品',year:'2026'};
// Vitest transforms SFCs for SSR. Compile only the real template for the
// client host renderer, retaining the imported component's actual setup/events.
const descriptor=parse(readFileSync(new URL('./MediaSearchCard.vue',import.meta.url),'utf8')).descriptor;
const bindings=compileScript(descriptor,{id:'i31-card'}).bindings;
const output=compileTemplate({source:descriptor.template!.content,filename:'MediaSearchCard.vue',id:'i31-card',compilerOptions:{bindingMetadata:bindings}});
if(output.errors.length)throw output.errors[0];
const clientCode=output.code.replace(/import\s*\{([^}]+)\}\s*from\s*["']vue["']/g,(_,names:string)=>`const {${names.replace(/\bas\b/g,':')}}=Vue;`).replace('export function render','return function render');
const ClientCard={...Card,render:new Function('Vue',clientCode)(Vue)} as typeof Card;

function mount(disabled=false){const select=vi.fn(),props=reactive({item,disabled,loading:false,selected:false}),root=make('root');const app=renderer.createApp(defineComponent({setup:()=>()=>h(ClientCard,{...props,onSelect:select})}));app.provide(ssrContextKey,{modules:new Set()});app.mount(root);cleanups.push(()=>app.unmount());return {select,props,button:root.children[0]!}}
function key(button:Node,value:string){const event={key:value,preventDefault:vi.fn()};for(const fn of [button.props.onKeydown].flat())fn(event);return event}
it('the entire card is an accessible named native button and click emits the media',()=>{const {select,button}=mount();expect(button.tag).toBe('button');expect(button.props.type).toBe('button');expect(button.props['aria-label']).toBe('搜索《作品》(2026) 的站点资源');button.props.onClick();expect(select).toHaveBeenCalledExactlyOnceWith(item)});
it.each(['Enter',' '])('keyboard %s activates exactly once and prevents a second native default action',value=>{const {select,button}=mount();const event=key(button,value);expect(select).toHaveBeenCalledExactlyOnceWith(item);expect(event.preventDefault).toHaveBeenCalledTimes(1)});
it('ready=false/busy disabled cards block pointer and keyboard activation',()=>{const {select,button}=mount(true);expect(button.props.disabled).toBe(true);button.props.onClick();key(button,'Enter');key(button,' ');expect(select).not.toHaveBeenCalled()});
it('the selected in-flight card exposes loading state',async()=>{const {props,button}=mount();props.loading=true;props.selected=true;props.disabled=true;await nextTick();expect(button.props['aria-busy']).toBe(true);expect(button.props['aria-pressed']).toBe(true);const content=(n:Node):string=>n.text+n.children.map(content).join('');expect(content(button)).toContain('正在搜索全部站点…')});
it('the workspace has neither site selector nor second search button',()=>{const view=readFileSync(new URL('../views/SearchWorkspaceView.vue',import.meta.url),'utf8');expect(view).not.toContain('v-model="accountIds"');expect(view).not.toContain('搜索站点资源');expect(view).not.toContain('选择站点，查找资源');expect(view).toContain(':disabled="busy||!!intent||!ready"');expect(view).toContain('@select="selectAndSearch"')});

it.each(['click','Enter'])('real card %s reaches the search API once through useSearch',async action=>{
 vi.mocked(request).mockReset().mockImplementation(async path=>path==='/integrations'?[{id:'site',kind:'ptsite',name:'站点',enabled:true}]:[]);
 const pending=deferred();vi.mocked(post).mockReset().mockReturnValue(pending.promise);
 let flow!:ReturnType<typeof useSearch>;const root=make('root');
 const app=renderer.createApp(defineComponent({setup(){flow=useSearch();return()=>h(ClientCard,{item,disabled:flow.busy.value||!!flow.intent.value||!flow.ready.value,loading:flow.busy.value,selected:flow.selected.value?.id===item.id,onSelect:flow.selectAndSearch})}}));
 app.provide(ssrContextKey,{modules:new Set()});app.mount(root);cleanups.push(()=>app.unmount());await flush();
 const button=root.children[0]!;expect(button.props.disabled).toBe(false);if(action==='click')button.props.onClick();else key(button,'Enter');await flush();
 expect(post).toHaveBeenCalledExactlyOnceWith('/search',{keyword:'作品',refresh:false,accountIds:[],media:{id:7,mediaType:'series'}});
 expect(button.props.disabled).toBe(true);button.props.onClick();expect(post).toHaveBeenCalledTimes(1);pending.resolve({id:'accepted'});await flush();
});
