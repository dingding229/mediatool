import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ref} from 'vue';
import {setupComponent,flush} from '../test/setup-component';
import MetadataEditor from './MetadataEditor.vue';
import ImageEditor from './ImageEditor.vue';
import Detail from '../views/MediaWorkspaceView.vue';
vi.mock('./useCatalog',()=>({useCatalog:()=>({detail:ref({id:'movie'}),detailOpen:ref(true),refresh:vi.fn()})}));
const metadata={target:{mediaId:'root',kind:'movie',season:-1,episode:-1},targetKey:'root',revision:1,base:{title:'原名'},effective:{title:'原名'},overrides:{},matched:true,unmatched:[],choices:[]};
let views:ReturnType<typeof setupComponent>[]=[];
const mount=(component:any,props:Record<string,unknown>={})=>{const v=setupComponent(component,props);views.push(v);return v.state;};
beforeEach(()=>vi.stubGlobal('fetch',vi.fn(async()=>({ok:true,status:200,json:async()=>({data:{...metadata,base:{},effective:{},choices:[]}})}))));
afterEach(()=>{views.forEach(v=>v.unmount());views=[];vi.unstubAllGlobals();});
it('external metadata entry is lazy and reopening a mounted editor retains its draft',async()=>{
 vi.mocked(fetch).mockResolvedValue({ok:true,status:200,json:async()=>({data:metadata})} as Response);
 const s=mount(MetadataEditor,{itemId:'movie',hideTrigger:true});expect(fetch).not.toHaveBeenCalled();s.open();await flush();s.draft.title='未保存草稿';s.close();expect(s.isOpen()).toBe(false);s.open();await flush();expect(s.isOpen()).toBe(true);expect(s.draft.title).toBe('未保存草稿');expect(fetch).toHaveBeenCalledTimes(1);
});
it('external image entry reuses the existing lazy read and does not toggle closed',async()=>{
 const s=mount(ImageEditor,{itemId:'movie',hideTrigger:true});expect(fetch).not.toHaveBeenCalled();await s.openEditor();s.close();expect(s.isOpen()).toBe(false);await s.openEditor();expect(s.isOpen()).toBe(true);expect(fetch).toHaveBeenCalledTimes(1);
});
it('detail selects one visible editor, blocks switching during reads and resets on another work',async()=>{
 const s=mount(Detail),metadataOpen=vi.fn(),imageOpen=vi.fn(),metadataClose=vi.fn(),imageClose=vi.fn();s.metadataEditor={open:metadataOpen,close:metadataClose,busy:false,isOpen:()=>true,canClose:vi.fn(async()=>true)};s.imageEditor={open:imageOpen,close:imageClose,busy:false,isOpen:()=>false};
 await s.edit('metadata');expect(s.activeEditor).toBe('metadata');expect(metadataOpen).toHaveBeenCalledTimes(1);expect(s.imageEditor.close).toHaveBeenCalledTimes(1);
 await s.edit('images');expect(s.activeEditor).toBe('images');expect(imageOpen).toHaveBeenCalledTimes(1);expect(s.metadataEditor.close).toHaveBeenCalledTimes(1);
 s.imageEditor.busy=true;await s.edit('metadata');expect(s.activeEditor).toBe('images');expect(metadataOpen).toHaveBeenCalledTimes(1);
 s.detail={id:'series'};await flush();expect(s.activeEditor).toBe('');s.activeEditor='metadata';s.detailOpen=false;await flush();expect(s.activeEditor).toBe('');
});
