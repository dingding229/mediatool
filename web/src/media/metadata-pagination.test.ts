import {beforeEach,expect,it,vi} from 'vitest';
import {setupComponent,flush} from '../test/setup-component';
import MetadataEditor from './MetadataEditor.vue';
import {readMediaMetadata,saveMediaMetadata,type MetadataRecord} from '../api/mediaMetadata';
vi.mock('../api/mediaMetadata',async original=>({...await original<typeof import('../api/mediaMetadata')>(),readMediaMetadata:vi.fn(),saveMediaMetadata:vi.fn()}));
const record:MetadataRecord={target:{mediaId:'owned',kind:'series',season:-1,episode:-1},targetKey:'series:owned',revision:1,base:{title:'Base'},effective:{title:'Base'},overrides:{},matched:true,unmatched:[],choices:[],unmatchedPage:1,unmatchedTotal:65};
beforeEach(()=>vi.clearAllMocks());
it.each([10,100])('re-reads the selected %s-row page after a save response with the default size',async size=>{
 vi.mocked(readMediaMetadata).mockResolvedValue({...record,unmatched:Array.from({length:Math.min(size,65)},()=>record.target)});
 vi.mocked(saveMediaMetadata).mockResolvedValue({...record,revision:2,effective:{title:'Changed'},unmatched:Array.from({length:50},()=>record.target)});
 const v=setupComponent(MetadataEditor,{itemId:'owned'});v.state.unmatchedSize=size;v.state.open();await flush();v.state.draft.title='Changed';await v.state.save();
 expect(saveMediaMetadata).toHaveBeenCalledTimes(1);expect(readMediaMetadata).toHaveBeenCalledTimes(2);expect(readMediaMetadata).toHaveBeenLastCalledWith('owned',1,size);expect(v.state.record.unmatched).toHaveLength(Math.min(size,65));v.unmount();
});
