import {describe,it,expect} from 'vitest';
import {metadataFields,type MetadataRecord,type MetadataField} from '../api/mediaMetadata';
import {inputValue,metadataValueLabel,parseValue,metadataPatch} from './metadata-form';
describe('metadata editor draft and field locks',()=>{
 const record:MetadataRecord={target:{mediaId:'fixture',kind:'movie',season:-1,episode:-1},targetKey:'fixture:movie:-1:-1',revision:7,base:{title:'刮削标题',overview:'最新简介'},effective:{title:'人工标题',overview:'',communityRating:6.681},overrides:{title:'人工标题',overview:''},matched:true,unmatched:[],choices:[]};
 const draft=()=>Object.fromEntries(metadataFields.map(k=>[k,inputValue(record.effective[k],k)])) as Record<MetadataField,string>;
 it('sends only changed fields and preserves the loaded revision',()=>{const d=draft();d.title=' 新标题 ';const p=metadataPatch(record,d,new Set());expect(p).toEqual({targetKey:record.targetKey,revision:7,set:{title:'新标题'},restore:[]});});
 it('does not confuse explicit empty text with restoring the provider value',()=>{const d=draft();d.title='刮削标题';const p=metadataPatch(record,d,new Set(['title']));expect(p.set).toEqual({});expect(p.restore).toEqual(['title']);expect(record.overrides.overview).toBe('');});
 it('formats provider precision without silently locking an untouched score',()=>{const d=draft();expect(d.communityRating).toBe('6.7');expect(metadataPatch(record,d,new Set()).set).toEqual({});d.communityRating='6.8';expect(metadataPatch(record,d,new Set()).set).toEqual({communityRating:6.8});});
 it('normalizes arrays, nullable numbers and dates',()=>{expect(parseValue('genres',' 剧情、 剧情,动作\n')).toEqual(['剧情','动作']);expect(parseValue('communityRating','')).toBeNull();expect(parseValue('premiereDate','')).toBeNull();expect(parseValue('year','2026')).toBe(2026);expect(inputValue(0)).toBe('0');});
 it('zero is absent only in read-only rating copy, never in numeric drafts or untouched patches',()=>{expect(metadataValueLabel(0,'communityRating')).toBe('—');expect(metadataValueLabel(7.25,'communityRating')).toBe('7.3');expect(inputValue(0,'communityRating')).toBe('0');const zero={...record,effective:{...record.effective,communityRating:0}};const d={...draft(),communityRating:'0'};expect(metadataPatch(zero,d,new Set()).set).toEqual({});});
});
