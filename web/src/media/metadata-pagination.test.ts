import {readFileSync} from 'node:fs';
import {beforeEach,expect,it,vi} from 'vitest';
import {setupComponent,flush} from '../test/setup-component';
import MetadataEditor from './MetadataEditor.vue';
import {readMediaMetadata,saveMediaMetadata,type MetadataRecord} from '../api/mediaMetadata';
vi.mock('../api/mediaMetadata',async original=>({...await original<typeof import('../api/mediaMetadata')>(),readMediaMetadata:vi.fn(),saveMediaMetadata:vi.fn()}));
const record:MetadataRecord={target:{mediaId:'owned',kind:'series',season:-1,episode:-1},targetKey:'series:owned',revision:1,base:{title:'Base'},effective:{title:'Base'},overrides:{},matched:true,unmatched:[],choices:[],unmatchedPage:1,unmatchedTotal:65};
beforeEach(()=>vi.clearAllMocks());
it('does not render pagination or consume unmatched page URL state',()=>{
 const source=readFileSync(new URL('./MetadataEditor.vue',import.meta.url),'utf8');
 expect(source).not.toContain('PagePagination');expect(source).not.toContain('usePageNumber');expect(source).not.toContain('usePageSize');expect(source).toContain('metadata-unmatched-items');
});
it.each([65,130])('requests one maximum-size page and re-reads a truncated save response (%s total)',async total=>{
 const unmatched=Array.from({length:Math.min(100,total)},(_,i)=>({...record.target,kind:'episode' as const,season:1,episode:i+1}));
 vi.mocked(readMediaMetadata).mockResolvedValue({...record,unmatched,unmatchedTotal:total});
 vi.mocked(saveMediaMetadata).mockResolvedValue({...record,revision:2,effective:{title:'Changed'},unmatched:unmatched.slice(0,50),unmatchedTotal:total});
 const v=setupComponent(MetadataEditor,{itemId:'owned'});v.state.open();await flush();expect(readMediaMetadata).toHaveBeenCalledExactlyOnceWith('owned',1,100);
 v.state.draft.title='Changed';await v.state.save();expect(saveMediaMetadata).toHaveBeenCalledTimes(1);expect(readMediaMetadata).toHaveBeenCalledTimes(2);expect(readMediaMetadata).toHaveBeenLastCalledWith('owned',1,100);expect(v.state.record.unmatched).toHaveLength(Math.min(total,100));v.unmount();
});
