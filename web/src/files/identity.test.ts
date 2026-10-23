import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest';
import {createSSRApp,defineComponent,h,reactive} from 'vue';
import {renderToString} from 'vue/server-renderer';
import {createFileIdentity,filePayload,validateFileIdentity} from './identity';
import {identifyFile} from '../api/fileIdentity';
import {post,request} from '../api/client';
import Fields from './FileIdentityFields.vue';
import Action from './FileActionDialog.vue';
import Batch from '../workflows/FileBatchDialog.vue';
import {setupComponent,deferred,flush} from '../test/setup-component';
vi.mock('../api/fileIdentity',()=>({identifyFile:vi.fn()}));
vi.mock('../api/client',async original=>({...await original<typeof import('../api/client')>(),post:vi.fn(),request:vi.fn()}));
const categories={movie:[{name:'电影 / 欧美电影',directory:'电影/欧美电影'}],series:[{name:'动漫 / 日漫',directory:'动漫/日漫'}]};
const result=(kind='movie')=>({hint:{title:'Original',year:'2026',mediaType:kind,season:3,episode:7,episodes:[7],resolution:'1080p'},matches:[{id:kind==='movie'?301:302,title:'Confirmed',year:'2026',mediaType:kind,posterPath:'/poster.jpg'}],selected:{id:kind==='movie'?301:302,title:'Confirmed',year:'2026',mediaType:kind,posterPath:'/poster.jpg'},classification:categories[kind as keyof typeof categories][0],categories,warning:''});
let views:ReturnType<typeof setupComponent>[]=[];let controllers:ReturnType<typeof createFileIdentity>[]=[];
beforeEach(()=>{vi.mocked(identifyFile).mockReset().mockResolvedValue(result());vi.mocked(post).mockReset().mockResolvedValue({items:[],steps:[]});vi.mocked(request).mockReset().mockResolvedValue({value:{movieTemplate:'{title}',seriesTemplate:'{title}'}});const store=new Map<string,string>();vi.stubGlobal('sessionStorage',{getItem:(k:string)=>store.get(k)||null,setItem:(k:string,v:string)=>store.set(k,v),removeItem:(k:string)=>store.delete(k)});});
afterEach(()=>{controllers.forEach(c=>c.dispose());controllers=[];views.forEach(v=>v.unmount());views=[];vi.unstubAllGlobals();});
async function render(draft:any,identity:any){const app=createSSRApp(Fields,{draft,identity});for(const name of ['el-input','el-input-number','el-select','el-option','el-button'])app.component(name,defineComponent({inheritAttrs:false,setup(_,ctx){return()=>h('div',ctx.attrs,ctx.slots.default?.());}}));return renderToString(app);}
describe.each(['movie','series'])('%s organization identity',kind=>{
 it('fills metadata and category and renders season fields only for episodes',async()=>{
  vi.mocked(identifyFile).mockResolvedValue(result(kind));const draft=reactive<any>({mediaType:'unknown',category:'',title:'old'});const identity=createFileIdentity(draft,'Original.2026.mkv');controllers.push(identity);await identity.recognize();await flush();
  expect(draft).toMatchObject({title:'Confirmed',year:'2026',mediaType:kind,tmdbId:kind==='movie'?301:302,category:categories[kind as keyof typeof categories][0]!.directory});
  const html=await render(draft,identity);expect(html.includes('aria-label="季号"')).toBe(kind==='series');expect(html.includes('aria-label="集号"')).toBe(kind==='series');expect(html).toContain('/api/v1/metadata/image/w185/poster.jpg');
  const payload=filePayload(draft);expect('season'in payload).toBe(kind==='series');expect('episode'in payload).toBe(kind==='series');if(kind==='series')expect(payload).toMatchObject({season:3,episode:7});
 });
 it('single file preview sends the selected TMDB ID and never movie season/episode',async()=>{
  vi.mocked(identifyFile).mockResolvedValue(result(kind));const view=setupComponent(Action);views.push(view);await view.state.open('organize',{id:'root'},'.',{id:'file',name:'Original.2026.mkv',path:'Original.2026.mkv',size:1});await view.state.preview();
  const body=vi.mocked(post).mock.calls.find(([path])=>path==='/files/preview')![1] as any;expect(body.tmdbId).toBe(kind==='movie'?301:302);expect(body.category).toBe(categories[kind as keyof typeof categories][0]!.directory);expect('season'in body).toBe(kind==='series');expect('episode'in body).toBe(kind==='series');
 });
});
it('ambiguous matches remain unselected, allow explicit choice and manual replacement',async()=>{
 const out=result();vi.mocked(identifyFile).mockResolvedValueOnce({...out,selected:undefined,classification:undefined,matches:[...out.matches,{...out.matches[0],id:999}],warning:'未识别，分类需手动选择'});const draft=reactive<any>({});const c=createFileIdentity(draft,'Ambiguous.2026.mkv');controllers.push(c);await c.recognize();expect(draft.tmdbId).toBe(0);expect(draft.category).toBe('');expect(c.matches).toHaveLength(2);expect(c.warning).toContain('未识别');await c.choose(out.matches[0]);expect(identifyFile).toHaveBeenLastCalledWith('Ambiguous.2026.mkv',{tmdbId:301,mediaType:'movie'});expect(draft.tmdbId).toBe(301);c.manual();draft.title='Manual';draft.category=categories.movie[0]!.directory;expect(filePayload(draft)).toMatchObject({tmdbId:0,title:'Manual',category:'电影/欧美电影'});
});
it('manual search does not change an existing choice and missing episode is not invented as 1',async()=>{
 const draft=reactive<any>({mediaType:'unknown'}),c=createFileIdentity(draft,'Unknown');controllers.push(c);vi.mocked(identifyFile).mockResolvedValueOnce({...result('series'),hint:{title:'Show',mediaType:'series',season:0,episode:0},selected:undefined,classification:undefined});await c.recognize();expect(draft.episode).toBeUndefined();expect(draft.episode).not.toBe(1);c.query='Different';c.queryType='movie';await c.search();expect(identifyFile).toHaveBeenLastCalledWith('Unknown',{query:'Different',mediaType:'movie'});expect(draft.tmdbId).toBe(0);
});
it('a late match cannot replace manual input after cancellation or close',async()=>{
 const pending=deferred<any>();vi.mocked(identifyFile).mockReturnValueOnce(pending.promise);const draft=reactive<any>({mediaType:'movie',title:'Manual'}),c=createFileIdentity(draft,'Unknown');controllers.push(c);const read=c.recognize();c.manual();draft.title='Kept';pending.resolve(result());await read;expect(draft.title).toBe('Kept');expect(draft.tmdbId).toBe(0);
});
it('batch preview preserves TMDB IDs, categories and only series season payloads',async()=>{
 vi.mocked(identifyFile).mockImplementation(async name=>result(name.includes('Show')?'series':'movie'));const view=setupComponent(Batch);views.push(view);await view.state.openFiles({id:'root'},[{id:'m',path:'Film.mkv',name:'Film.mkv',size:1},{id:'s',path:'Show.S03E07.mkv',name:'Show.S03E07.mkv',size:2}]);await view.state.preview();const body=vi.mocked(post).mock.calls.find(([p])=>p==='/files/batches/preview')![1] as any;
 expect(body.items[0]).toMatchObject({tmdbId:301,category:'电影/欧美电影'});expect(body.items[0]).not.toHaveProperty('season');expect(body.items[0]).not.toHaveProperty('episode');expect(body.items[0]).not.toHaveProperty('identity');expect(body.items[1]).toMatchObject({tmdbId:302,season:3,episode:7,category:'动漫/日漫'});
});
it('confirmed library selection retains the explicitly chosen identity instead of auto replacing it',async()=>{
 vi.mocked(post).mockResolvedValueOnce({root:{id:'root'},files:[{id:'f',path:'Show.mkv',name:'Show.mkv',size:2}],rows:[{source:'Show.mkv',title:'Known Show',mediaType:'series',tmdbId:302,season:3,episode:7,confirmed:true}]});vi.mocked(identifyFile).mockResolvedValue(result('series'));const view=setupComponent(Batch);views.push(view);await view.state.open('assets',[{id:'owned'}]);expect(identifyFile).toHaveBeenCalledWith('Show.mkv',{tmdbId:302,mediaType:'series'});expect(view.state.items[0].tmdbId).toBe(302);
});

it.each([[undefined,7,true],[3,undefined,true],[0,2,false],[3,7,false],[-1,7,true]])('unknown series coverage is manual, not a fabricated season 1: %s/%s',(season,episode,invalid)=>{expect(!!validateFileIdentity({mediaType:'series',title:'Show',category:'电视剧',season,episode})).toBe(invalid);});
it('whole-directory organization needs no invented per-file episode',()=>expect(validateFileIdentity({mediaType:'series',title:'Show',category:'电视剧'},true)).toBe(''));

it('cannot preview an old identity while replacement details are still loading',async()=>{const view=setupComponent(Action);views.push(view);await view.state.open('organize',{id:'root'},'.',{id:'f',name:'Film.2026.mkv',path:'Film.2026.mkv',size:1});const pending=deferred<any>();vi.mocked(identifyFile).mockReturnValueOnce(pending.promise);const read=view.state.identity.choose({id:999,mediaType:'movie'});await view.state.preview();expect(post).not.toHaveBeenCalled();pending.resolve(result());await read;});
