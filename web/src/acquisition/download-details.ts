import type {DescriptionItem} from '../ui/DescriptionGrid.vue';
import {progress,qbLabel,seedingLabel,size,type DownloadTask} from './model';
import {beijingTime} from '../presentation/time';
import {downloaderName,uploadPercent} from './download-presentation';
type DetailRow=Partial<DownloadTask>&{phase?:string;processingProgress?:number};
type Connection={kind:string;name:string;settings?:{endpoint?:string}};
export const cleanupLabel=(state:string)=>({disabled:'未启用自动清理',pending:'等待清理',succeeded:'已清理',blocked:'清理受阻',deleting:'清理中',uncertain:'结果待核对'}[state]||'清理状态待核对');
export function downloadDetailGroups(row:DetailRow|null,isDownload:boolean,connections:Connection[]=[]){
 if(!row)return [];
 const groups:{key:string;title:string;items:DescriptionItem[]}[]=[{key:'status',title:'状态',items:[
  {key:'state',label:'任务状态',value:row.state},
  {key:'message',label:'说明',value:row.message},
  {key:'progress',label:isDownload?'下载进度':'上传进度',value:isDownload?progress(row.progress):row.state&&uploadPercent({...row,state:row.state})!==null?uploadPercent({...row,state:row.state})+'%':null}
 ]}];
 if(isDownload)groups.push({key:'media',title:'媒体',items:[{key:'media',label:'作品名称',value:row.media?.title},{key:'year',label:'年份',value:row.media?.year}]},
 {key:'downloader',title:'下载器与做种',items:[
  {key:'downloader',label:'下载器',value:downloaderName(row,connections)},
  {key:'qbState',label:'下载器状态',value:row.qbState&&!(['uploading','stalledUP','pausedUP','stoppedUP'].includes(row.qbState)&&['seeding','stopped'].includes(row.seedingState||''))?qbLabel(row.qbState):null},
  {key:'seeding',label:'做种状态',value:seedingLabel(row.seedingState)},
  {key:'syncedAt',label:'同步时间',value:beijingTime(row.syncedAt)},
  {key:'downloadSpeed',label:'下载速度',value:row.syncedAt&&row.downloadSpeed!=null?size(row.downloadSpeed)+'/s':null},
  {key:'uploadSpeed',label:'上传速度',value:row.syncedAt&&row.uploadSpeed!=null?size(row.uploadSpeed)+'/s':null},
  {key:'seeds',label:'做种连接',value:row.seeds},{key:'peers',label:'下载连接',value:row.peers}
 ]},
 {key:'cleanup',title:'清理条件',items:[{key:'cleanup',label:'条件状态',value:row.cleanupState?cleanupLabel(row.cleanupState):null},{key:'cleanupMessage',label:'清理说明',value:row.cleanupMessage}]});
 // Empty groups should not leave orphan headings. DescriptionGrid owns row hiding.
 return groups.filter(group=>group.items.some(item=>item.value!=null&&!['','—'].includes(String(item.value).trim())));
}
