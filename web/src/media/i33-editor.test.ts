import {readFileSync} from 'node:fs';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {setupComponent,flush} from '../test/setup-component';
import MetadataEditor from './MetadataEditor.vue';
import Detail from '../views/MediaWorkspaceView.vue';
import {ref} from 'vue';
const confirm=vi.hoisted(()=>vi.fn());
vi.mock('element-plus',async original=>({...await original<typeof import('element-plus')>(),ElMessageBox:{confirm}}));
vi.mock('./useCatalog',()=>({useCatalog:()=>({detail:ref({id:'movie'}),detailOpen:ref(true),refresh:vi.fn(),load:vi.fn()})}));
const read=(file:string)=>readFileSync(new URL(file,import.meta.url),'utf8');
const record={target:{mediaId:'root',kind:'movie',season:-1,episode:-1},targetKey:'root',revision:1,base:{title:'原名'},effective:{title:'原名'},overrides:{},matched:true,unmatched:[],choices:[]};
beforeEach(()=>{vi.clearAllMocks();vi.stubGlobal('fetch',vi.fn(async()=>({ok:true,status:200,json:async()=>({data:record})})));});
afterEach(()=>vi.unstubAllGlobals());
it('places label, input/helper and restore in one row, with an equal-width label column',()=>{
 const source=read('./MetadataEditor.vue'),row=source.slice(source.indexOf('<div v-for="key'),source.indexOf('<div class="mt-save-actions"'));
 expect(row).toMatch(/<label class="metadata-label"[^>]*:for=/);expect(row).toContain('class="metadata-field"');expect(row).toContain('class="metadata-origin" :title=');expect(row).toContain('class="metadata-restore"');expect(row).toContain('metadata-row--overview');
 expect(source).toContain('grid-template-columns:112px minmax(0,1fr) auto');expect(source).toContain('@media(max-width:480px)');
 const images=read('./ImageEditor.vue');expect(images).toMatch(/class="image-label"[\s\S]*class="image-preview"[\s\S]*class="image-actions"[\s\S]*class="image-restore"/);expect(images).not.toContain('PagePagination');
});
it('hides rather than unmounts read-only details on both host pages',()=>{
 const source=read('../views/MediaWorkspaceView.vue');expect(source).toContain('v-show="!editVisible" class="catalog-readonly"');expect(source).toContain('v-show="!editVisible" class="catalog-subtitle"');expect(source).toContain('<DescriptionGrid v-show="!editVisible"');expect(source).toContain('<MediaEditDialog');
 expect(source.indexOf('class="catalog-readonly"')).toBeLessThan(source.indexOf('aria-label="作品简介"'));expect(source.lastIndexOf('<SubtitlePanel')).toBeGreaterThan(source.indexOf('class="catalog-readonly"'));
 const organize=read('../views/OrganizeWorkspaceView.vue');expect(organize).toContain('v-show="!assetImagesOpen"');expect(organize).not.toContain('technical-data');
});
it('confirms dirty collapse, keeps drafts on both cancel and continue, and reopens without a read',async()=>{
 const v=setupComponent(MetadataEditor,{itemId:'movie',hideTrigger:true}),s=v.state;s.open();await flush();s.draft.title='保留草稿';
 confirm.mockRejectedValueOnce('cancel');await s.requestClose();expect(s.isOpen()).toBe(true);expect(s.draft.title).toBe('保留草稿');
 confirm.mockResolvedValueOnce('confirm');await s.requestClose();expect(s.isOpen()).toBe(false);s.open();await flush();expect(s.draft.title).toBe('保留草稿');expect(fetch).toHaveBeenCalledTimes(1);expect(confirm).toHaveBeenCalledTimes(2);v.unmount();
});
