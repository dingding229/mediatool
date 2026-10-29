import {metadataFields,type MetadataField,type MetadataValue,type MetadataValues,type MetadataRecord,type MetadataPatch} from '../api/mediaMetadata';
import {displayRating} from './model';
export const metadataLabels:Record<MetadataField,string>={title:'标题',originalTitle:'原始标题',sortName:'排序标题',year:'年份',premiereDate:'首播日期',overview:'简介',tagline:'标语',genres:'类型',tags:'标签',communityRating:'评分',officialRating:'分级'};
export function inputValue(value?:MetadataValue,field?:MetadataField){if(field==='communityRating'&&typeof value==='number'&&value>0&&value<=10)return displayRating(value);return Array.isArray(value)?value.join('、'):value==null?'':String(value);}
export function metadataValueLabel(value:MetadataValue|undefined,field:MetadataField){return field==='communityRating'?displayRating(typeof value==='number'?value:undefined):inputValue(value,field)||'暂无刮削值';}
export function parseValue(key:MetadataField,value:string):MetadataValue{
 const trimmed=value.trim();
 if(key==='genres'||key==='tags')return [...new Set(value.split(/[、,，\n]/).map(v=>v.trim()).filter(Boolean))];
 if(key==='year'||key==='communityRating')return trimmed===''?null:Number(trimmed);
 return key==='premiereDate'&&trimmed===''?null:trimmed;
}
export function metadataPatch(record:MetadataRecord,draft:Record<MetadataField,string>,restore:Set<MetadataField>):MetadataPatch{
 const set:MetadataValues={};
 for(const key of metadataFields){if(restore.has(key))continue;const value=parseValue(key,draft[key]);if(inputValue(value,key)!==inputValue(record.effective[key],key))set[key]=value;}
 return {targetKey:record.targetKey,revision:record.revision,set,restore:[...restore]};
}
