import type {DownloadTask} from './model';
export function remainingTime(task:DownloadTask):string{
 if(task.syncError||!task.syncedAt||task.progress==null||!Number.isFinite(task.progress))return '—';
 if(task.progress>=1)return '0 秒';
 const size=task.resourceSize??0,speed=task.downloadSpeed??0;
 if(!['downloading','forcedDL'].includes(task.qbState||'')||!Number.isFinite(size)||size<=0||!Number.isFinite(speed)||speed<=0)return '—';
 const seconds=Math.ceil(size*(1-Math.max(0,task.progress))/speed);
 return seconds<60?`约 ${seconds} 秒`:seconds<3600?`约 ${Math.ceil(seconds/60)} 分钟`:`约 ${Math.ceil(seconds/3600)} 小时`;
}
