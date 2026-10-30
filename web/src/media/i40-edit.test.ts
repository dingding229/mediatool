import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ref} from 'vue';
import {readFileSync} from 'node:fs';
import {setupComponent,flush} from '../test/setup-component';
import Dialog from './MediaEditDialog.vue';
import MetadataEditor from './MetadataEditor.vue';
import ImageEditor from './ImageEditor.vue';
import Detail from '../views/MediaWorkspaceView.vue';
import SeasonEpisodes from './SeasonEpisodes.vue';
import {seasonLabel} from './season-label';
const confirm=vi.hoisted(()=>vi.fn());
vi.mock('element-plus',async original=>({...await original<typeof import('element-plus')>(),ElMessageBox:{confirm}}));
vi.mock('./useCatalog',()=>({useCatalog:()=>({detail:ref({id:'movie'}),detailOpen:ref(true),refresh:vi.fn(),load:vi.fn()})}));
const metadata={target:{mediaId:'root',kind:'movie',season:-1,episode:-1},targetKey:'root',revision:1,base:{title:'刮削标题'},effective:{title:'刮削标题'},overrides:{},matched:true,unmatched:[],choices:[]};
const images={...metadata,base:{Primary:{url:'/old.jpg'}},effective:{Primary:{url:'/old.jpg'}}};
const views:ReturnType<typeof setupComponent>[]=[];const mount=(c:any,p:any={})=>{const v=setupComponent(c,p);views.push(v);return v.state};
const response=(data:any,status=200)=>({ok:status<400,status,json:async()=>({data,error:status>=400?{code:'conflict',message:'图片版本已变化'}:undefined})});
beforeEach(()=>{vi.clearAllMocks();vi.stubGlobal('fetch',vi.fn(async(url:string)=>response(url.endsWith('/images')?JSON.parse(JSON.stringify(images)):metadata)))});
afterEach(()=>{views.splice(0).forEach(v=>v.unmount());vi.unstubAllGlobals()});
it('keeps both actual editor drafts across tab changes, and defers image mutations until shared save',async()=>{
 const info=mount(MetadataEditor,{itemId:'movie',hideTrigger:true,externalControls:true}),image=mount(ImageEditor,{itemId:'movie',hideTrigger:true,externalControls:true,deferred:true});info.open();await image.openEditor();await flush();info.draft.title='人工片名';await image.selectCandidate('Primary',{path:'/candidate.jpg',url:'/candidate.jpg'});expect(image.dirty).toBe(true);expect(vi.mocked(fetch).mock.calls.every(([,o])=>!o?.method)).toBe(true);
 const dialog=mount(Dialog,{item:{id:'movie',title:'作品'}});dialog.metadata=info;dialog.images=image;dialog.tab='images';dialog.tab='metadata';expect(info.draft.title).toBe('人工片名');expect(image.record.effective.Primary.url).toBe('/candidate.jpg');expect(dialog.dirty).toBe(true);
 vi.mocked(fetch).mockImplementation(async(url,o)=>{if(o?.method==='PATCH')return response({...images,revision:2,effective:{Primary:{url:'/candidate.jpg'}}}) as Response;return response({...metadata,revision:2,effective:{title:'人工片名'}}) as Response});await dialog.save();expect(dialog.dirty).toBe(false);expect(vi.mocked(fetch).mock.calls.filter(([,o])=>o?.method==='PATCH')).toHaveLength(2);
});
it('failed information save never submits images and preserves both drafts; partial image failure is explicit',async()=>{
 const dialog=mount(Dialog,{item:{id:'movie',title:'作品'}}),infoSave=vi.fn(async()=>false),imageSave=vi.fn(async()=>false);
 dialog.metadata={dirty:true,busy:false,save:infoSave};dialog.images={dirty:true,busy:false,saveDraft:imageSave};await dialog.save();expect(imageSave).not.toHaveBeenCalled();expect(dialog.error).toContain('信息未保存');expect(dialog.dirty).toBe(true);
 infoSave.mockResolvedValue(true);await dialog.save();expect(dialog.error).toContain('信息已保存');expect(dialog.error).toContain('图片未全部保存');expect(dialog.dirty).toBe(true);
});
it('requires dirty-close confirmation, retains drafts when cancelled, and discards only on explicit abandonment',async()=>{
 const dialog=mount(Dialog,{item:{id:'movie',title:'作品'}}),discard=vi.fn();dialog.metadata={dirty:true,busy:false,discard};dialog.images={dirty:false,busy:false,discard};const done=vi.fn();confirm.mockRejectedValueOnce('cancel');await dialog.requestClose(done);expect(done).not.toHaveBeenCalled();expect(discard).not.toHaveBeenCalled();confirm.mockResolvedValueOnce('confirm');await dialog.requestClose(done);expect(done).toHaveBeenCalledOnce();expect(discard).toHaveBeenCalledTimes(2);
});
it('an image partial failure preserves only unsaved changes and advances revision for the next action',async()=>{
 const image=mount(ImageEditor,{itemId:'movie',hideTrigger:true,deferred:true});await image.openEditor();await image.selectCandidate('Primary',{path:'/poster.jpg',url:'/poster.jpg'});await image.selectCandidate('Backdrop',{path:'/back.jpg',url:'/back.jpg'});
 vi.mocked(fetch).mockImplementation(async(_url,o)=>{const data=JSON.parse(String(o?.body));return response({...images,revision:2,effective:{Primary:{url:'/poster.jpg'}}},data.kind==='Backdrop'?409:200) as Response});expect(await image.saveDraft()).toBe(false);expect(image.dirty).toBe(true);expect(image.staged('Primary')).toBeUndefined();expect(image.staged('Backdrop')?.path).toBe('/back.jpg');expect(image.blocked).toBe(true);const calls=vi.mocked(fetch).mock.calls.filter(([,o])=>o?.method==='PATCH');expect(JSON.parse(String(calls[1]![1]?.body)).revision).toBe(2);
});
it('read-only sections are hidden by the one editing state and cancellation restores without refetching',async()=>{
 const source=readFileSync(new URL('../views/MediaWorkspaceView.vue',import.meta.url),'utf8');expect(source).toContain('v-show="!editVisible" class="catalog-readonly"');expect(source).toContain('<MediaEditDialog');expect(source).not.toContain('activeEditor');expect(source).toContain('displayRating(detail.rating)!==');const s=mount(Detail);s.editVisible=true;s.editVisible=false;expect(s.load).not.toHaveBeenCalled();
});
it('season display removes duplicate labels and lazily loads an expanded season without a one-page pager',async()=>{
 expect(seasonLabel({title:'第 1 季',season:1} as any,'剧名')).toBe('第 1 季');expect(seasonLabel({title:'剧名',season:1} as any,'剧名')).toBe('第 1 季');
 vi.mocked(fetch).mockResolvedValue(response({items:[{id:'ep1',episode:1,title:'第1集',fileSize:1024}],total:1,page:1,pageSize:25}) as Response);const s=mount(SeasonEpisodes,{seasonId:'season_a_1'});await flush();expect(s.data.data.items).toHaveLength(1);expect(fetch).toHaveBeenCalledOnce();const source=readFileSync(new URL('./SeasonEpisodes.vue',import.meta.url),'utf8');expect(source).toContain('data.data.total>pageSize');expect(source).toContain('episode.episode');
});
