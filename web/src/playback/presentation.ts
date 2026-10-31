import type {NativeLibrary} from '../api/mediaServer';
export function libraryGroups(rows:NativeLibrary[]){
 const order=['动漫','电影','电视剧','未分类'];
 const groups=order.map(name=>({name,items:[] as {library:NativeLibrary;name:string}[]}));
 for(const library of rows){
  const groupName=library.groupName?.trim()||'',name=library.ruleName?.trim()||library.name?.trim()||'未命名目录';
  const group=/未分类/.test(name)||library.id.startsWith('f:')?'未分类':/动漫|动画|番剧/.test(groupName)?'动漫':library.collectionType==='tvshows'||/电视|剧集/.test(groupName)?'电视剧':library.collectionType==='movies'||/电影/.test(groupName)?'电影':'未分类';
  const suffix=groupName?' · '+groupName:'';
  groups[order.indexOf(group)]!.items.push({library,name:suffix&&name.endsWith(suffix)?name.slice(0,-suffix.length):name});
 }
 return groups.filter(group=>group.items.length);
}
export function accessAddresses(client?:string,connection?:string){
 if(client&&client===connection)return [{label:'地址',ip:client}];
 return [{label:'访客地址',ip:client},{label:'连接地址',ip:connection}].filter(value=>value.ip);
}
export const playbackIPScope='仅统计已登录内置媒体服务的播放会话公网 IP；直接 STRM 302、登录与浏览请求、被拒绝访问及未核验来源不计入。';
