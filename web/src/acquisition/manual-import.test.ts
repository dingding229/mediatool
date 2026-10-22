import {afterEach,beforeEach,expect,test,vi} from 'vitest'
import {setupComponent,flush} from '../test/setup-component'
import Manual from './ManualWorkspace.vue'
import {post,request} from '../api/client'
vi.mock('vue-router',()=>({useRoute:()=>({query:{}})}))
vi.mock('../api/client',async original=>({...await original<typeof import('../api/client')>(),post:vi.fn(),request:vi.fn(),responseStatus:()=>202}))
let view:ReturnType<typeof setupComponent>,rows:any[]
const id='aaaaaaaa-0000-4000-8000-000000000003'
beforeEach(()=>{const store=new Map<string,string>();vi.stubGlobal('sessionStorage',{getItem:(k:string)=>store.get(k)||null,setItem:(k:string,v:string)=>store.set(k,v),removeItem:(k:string)=>store.delete(k)});rows=[{id,title:'Owned Film 2026',state:'completed',processingState:'download_only',progress:1}];vi.mocked(post).mockReset();vi.mocked(request).mockReset().mockImplementation(async()=>({items:rows,total:rows.length}));view=setupComponent(Manual)})
afterEach(()=>{view.unmount();vi.unstubAllGlobals()})
async function choose(){await flush();view.state.detail=rows[0];await flush();view.state.selectedMedia={id:42,title:'Owned Film',year:'2026',mediaType:'movie'};view.state.importConfirm=true}
test('explicit old-task opt-in queues the selected identity, never another torrent add',async()=>{await choose();vi.mocked(post).mockResolvedValueOnce({queued:true});rows=[{...rows[0],processingState:'identifying'}];await view.state.importTask();expect(post).toHaveBeenCalledExactlyOnceWith(`/downloads/manual/${id}/import`,{confirm:true,mediaType:'movie',tmdbId:42});expect(view.state.importOp.phase).toBe('queued');expect(view.state.importConfirm).toBe(false);rows=[{...rows[0],processingState:'local_complete',managedTaskId:id}];await view.state.readImport();expect(view.state.importOp.pending).toBeNull();expect(view.state.detail.managedTaskId).toBe(id)})
test('an unknown response stays locked until a read proves the original handoff',async()=>{await choose();vi.mocked(post).mockRejectedValueOnce(Error('network'));await view.state.importTask();await view.state.importTask();expect(post).toHaveBeenCalledTimes(1);expect(view.state.importOp.phase).toBe('reconciling');expect(rows[0].processingState).toBe('download_only')})
