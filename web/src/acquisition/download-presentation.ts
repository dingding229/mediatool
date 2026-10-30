import type {DownloadTask} from './model';
export function remainingTime(task:DownloadTask):string{
 if(task.syncError||!task.syncedAt||task.progress==null||!Number.isFinite(task.progress))return '—';
 if(task.progress>=1)return '0 秒';
 const size=task.resourceSize??0,speed=task.downloadSpeed??0;
 if(!['downloading','forcedDL'].includes(task.qbState||'')||!Number.isFinite(size)||size<=0||!Number.isFinite(speed)||speed<=0)return '—';
 const seconds=Math.ceil(size*(1-Math.max(0,task.progress))/speed);
 return seconds<60?`约 ${seconds} 秒`:seconds<3600?`约 ${Math.ceil(seconds/60)} 分钟`:`约 ${Math.ceil(seconds/3600)} 小时`;
}

export function uploadPercent(transfer:{state:string;processingProgress?:number;phase?:string;transferMethod?:string}):number|null{
 if(['verified','imported','strm_ready','refresh_requested'].includes(transfer.state)||transfer.phase==='verified')return 100;
 return typeof transfer.processingProgress==='number'&&Number.isFinite(transfer.processingProgress)?Math.max(0,Math.min(100,transfer.processingProgress)):null;
}
export function uploadInProgress(state:string){return ['starting','uploading','hashing','running'].includes(state)}
/** Resolve only the task's frozen endpoint, never misattribute it to today's default. */
export function downloaderName(task:{endpoint?:string},connections:{kind:string;name:string;settings?:{endpoint?:string}}[]):string{
 const endpoint=task.endpoint?.replace(/\/+$/,'');
 const matches=endpoint?connections.filter(c=>c.kind==='qbittorrent'&&c.settings?.endpoint?.replace(/\/+$/,'')===endpoint):[];
 return matches.length===1?matches[0]!.name:'qBittorrent';
}
