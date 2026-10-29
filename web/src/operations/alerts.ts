import {stateLabel} from './model';
export const alertNames:Record<string,string>={disk:'剩余存储空间',failures:'十分钟失败任务',load:'每 CPU 负载',memory:'内存使用率',plugin_journal:'插件日志占用',workers:'业务队列离线'};
const units:Record<string,{digits:number;suffix:string}>={GiB:{digits:2,suffix:' GiB'},percent:{digits:1,suffix:'%'},load_per_cpu:{digits:2,suffix:' /CPU'},jobs_per_10m:{digits:0,suffix:' 条/10 分钟'},queues:{digits:0,suffix:' 个队列'},entries:{digits:0,suffix:' 条'}};
export interface AlertRow {code:string;state:string;value:unknown;message?:string;acknowledged_at?:string|null;threshold?:number|null;unit?:string;comparison?:string}
export function alertPresentation(a:AlertRow){
 const unit=a.unit&&Object.hasOwn(units,a.unit)?units[a.unit]:undefined;
 const known=Object.hasOwn(alertNames,a.code),metadata=known&&unit&&typeof a.threshold==='number'&&Number.isFinite(a.threshold)&&['<=','>='].includes(a.comparison||'');
 const format=(v:number)=>v.toFixed(unit!.digits)+unit!.suffix;
 const raw=a.value==null?'—':String(a.value);
 const acknowledged=a.state==='active'&&!!a.acknowledged_at;
 return {
  name:known?alertNames[a.code]:a.code,
  description:a.state==='normal'&&a.message==='指标已恢复或告警已关闭'?'':a.message||'',
  value:metadata&&typeof a.value==='number'&&Number.isFinite(a.value)?format(a.value):raw,
  threshold:metadata?`阈值 ${a.comparison==='<='?'≤':'≥'} ${format(a.threshold!)}`:'',
  badgeState:acknowledged?'partial':a.state==='active'?'failed':a.state==='normal'?'ready':a.state,
  badgeLabel:acknowledged?'已确认':stateLabel(a.state),
 };
}
