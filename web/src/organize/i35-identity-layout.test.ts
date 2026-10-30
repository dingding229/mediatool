import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {createSSRApp,h} from 'vue';import {renderToString} from 'vue/server-renderer';
import {readFileSync} from 'node:fs';
import {setupComponent,flush} from '../test/setup-component';
import IdentityEditor from './IdentityEditor.vue';import {post,request} from '../api/client';import {migrationPathGroups} from './identity-presentation';
vi.mock('../api/client',async original=>({...await original<typeof import('../api/client')>(),post:vi.fn(),request:vi.fn()}));
let views:ReturnType<typeof setupComponent>[]=[];
const mount=()=>{const v=setupComponent(IdentityEditor);views.push(v);return v.state};
const candidate=(i:number)=>({id:'file-'+i,name:`Where.the.Mask.Ends.S01E${i}.2026.HHWEB.mkv`,path:'/115/long/file-'+i+'.mkv',revision:i+1,state:'needs_confirmation',cloudState:'present'});
beforeEach(()=>{vi.clearAllMocks();const memory=new Map();vi.stubGlobal('sessionStorage',{getItem:(k:string)=>memory.get(k)||null,setItem:(k:string,v:string)=>memory.set(k,v),removeItem:(k:string)=>memory.delete(k)});vi.mocked(post).mockResolvedValue({fileDigest:'a'.repeat(64),hint:{title:'作品',mediaType:'series',season:1,episodes:[1]},matches:[]});vi.mocked(request).mockResolvedValue([]);});
afterEach(()=>{views.forEach(v=>v.unmount());views=[];vi.unstubAllGlobals()});
async function render(state:Record<string,any>){
 const app=createSSRApp({...IdentityEditor,setup:()=>state});
 for(const name of ['el-dialog','el-button','el-select','el-option','el-input-number','el-input','el-checkbox','el-alert'])app.component(name,{setup:(_:unknown,c:any)=>()=>h(name==='el-button'?'button':'div',c.attrs,[c.slots.default?.(),c.slots.footer?.()])});
 return renderToString(app);
}
it('renders three ordered sections for series, only one table header and the original save entry',async()=>{
 const s=mount();await s.open(Array.from({length:8},(_,i)=>candidate(i)));s.selected={id:42,title:'作品',mediaType:'series'};await flush();const html=await render(s);
 const positions=['1 找到正确作品','2 核对季集','3 预览输出'].map(title=>html.indexOf('aria-label="'+title+'"'));expect(positions.every(p=>p>=0)).toBe(true);expect(positions).toEqual([...positions].sort((a,b)=>a-b));expect(html.match(/class="identity-columns"/g)).toHaveLength(1);expect(html.match(/class="organize-file-draft"/g)).toHaveLength(8);expect(html).toContain('确认入库或保存身份');
});
it('does not render the season section for a movie and does not erase its existing values',async()=>{
 const s=mount();await s.open([],{id:'asset',revision:2,title:'电影',mediaType:'movie',path:'/115/movie.mkv'});s.selected={id:42,mediaType:'movie'};s.season=3;s.coverage='1,2';await flush();const html=await render(s);expect(html).not.toContain('aria-label="2 核对季集"');expect(html).not.toContain('aria-label="修正季度"');expect(s.season).toBe(3);expect(s.coverage).toBe('1,2');
});
it('bulk season modifies only existing drafts, retains digest/revision/episodes and obeys locks',async()=>{
 const s=mount();await s.open(Array.from({length:8},(_,i)=>candidate(i)));s.selected={id:42,mediaType:'series'};await flush();const calls=vi.mocked(post).mock.calls.length;s.bulkSeason=4;s.applyBulkSeason();expect(s.batch.map((r:any)=>r.season)).toEqual(Array(8).fill(4));expect(post).toHaveBeenCalledTimes(calls);
 await s.prepare();expect(s.payload.items).toHaveLength(8);expect(s.payload.items[0].confirmation).toEqual({mediaType:'series',tmdbId:42,season:4,episodes:[1],revision:1,fileDigest:'a'.repeat(64)});
 for(const flag of ['saving','recognizing','phase']){s[flag]=flag==='phase'?'reconciling':true;s.bulkSeason=7;s.applyBulkSeason();expect(s.batch[0].season).toBe(4);s[flag]=flag==='phase'?null:false;}
 s.bulkSeason=-1;s.applyBulkSeason();expect(s.batch[0].season).toBe(4);
});
it('groups only API-provided source/target operation sets without inventing a one-to-one mapping',()=>{
 const source=Object.freeze({path:'old.strm',kind:'strm',operation:'archive'}),target=Object.freeze({path:'new.strm',kind:'strm',operation:'write'}),nfo=Object.freeze({path:'new.nfo',kind:'nfo',operation:'write'});const operations=Object.freeze([source,target,nfo]);const groups=migrationPathGroups([...operations]);expect(groups).toEqual([{kind:'strm',source:[source],target:[target]},{kind:'nfo',source:[],target:[nfo]}]);expect(operations).toEqual([source,target,nfo]);
});
it('uses a scoped fixed head/foot layout and keeps preview/candidate scrolling local',()=>{
 const css=readFileSync(new URL('./identity-editor.css',import.meta.url),'utf8');expect(css).toContain('max-height:85vh');expect(css).toContain('flex:0 0 auto');expect(css).toContain('min-height:0;overflow:auto');expect(css).toContain('grid-template-columns:minmax(0,1fr) 88px 160px');expect(css).toContain('max-height:264px;overflow:auto');expect(css).toContain('max-height:320px;overflow:auto');expect(css).toContain('overflow-wrap:anywhere');
});
