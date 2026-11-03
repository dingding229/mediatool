import type {ExtensionDescriptor,PluginRun} from '../api/plugins';
export type ExtensionTitles=Record<string,string>;
type NamedExtension=Pick<ExtensionDescriptor,'id'|'name'|'cmd'>&Partial<Pick<ExtensionDescriptor,'kind'|'handler'>>;
const cleanTitle=(value:string)=>value.trim().replace(/^危险[：:]\s*/, '');
// Both surfaces use the same pinned declaration, then the same history, then ID.
export function extensionTitle(d:NamedExtension,history:PluginRun[]=[],titles:ExtensionTitles={}){
 const declared=titles[`${d.kind}:${d.id}`]?.trim()||(/\p{Script=Han}/u.test(d.name||'')?d.name?.trim():'');
 const recorded=history.find(r=>r.kind===d.kind&&r.handler===d.handler&&r.operationName?.trim())?.operationName;
 const readable=(d.cmd?.trim()||d.id).replace(/[._-]+/g,' ').trim();
 return cleanTitle(declared||recorded||readable)||'扩展操作';
}
export const extensionDanger=(d:ExtensionDescriptor)=>/^危险[：:]/.test(d.name||'')||d.requiredScopes.some(s=>/\.(control|write|delete|execute)$/.test(s));
export function runTitle(r:PluginRun,descriptors:ExtensionDescriptor[],titles:ExtensionTitles={},history:PluginRun[]=[]){
 const d=descriptors.find(d=>d.kind===r.kind&&d.handler===r.handler);
 if(d)return extensionTitle(d,history.length?history:[r],titles);
 return cleanTitle(r.operationName||'')||({service:'后台检查',command:'状态查询',action:'手动操作',event:'事件处理',api:'接口调用',chain:'扩展处理',module:'模块处理',tool:'工具调用'} as Record<string,string>)[r.kind]||'扩展操作';
}
