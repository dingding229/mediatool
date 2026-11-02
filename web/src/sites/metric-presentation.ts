import {formatBytes} from '../utils/format-bytes';import {formatCompactNumber} from '../presentation/numbers';
export function unknownSeeding(snapshot:any):string{
 if(snapshot?.state==='failed')return '—（采集失败）';
 if(!snapshot?.metrics||snapshot.state==='not_synced')return '—（尚未采集）';
 const m=snapshot.metrics;if((m.seedingPages>0&&!m.seedingComplete)||(m.notes||[]).some((note:string)=>/做种/.test(note)))return '—（采集未完成）';
 return '—（站点未提供）';
}
export function seedingText(snapshot:any,key:'seeding'|'seedingBytes'){
 const value=snapshot?.metrics?.[key];return typeof value==='number'&&Number.isFinite(value)&&value>=0?(key==='seeding'?formatCompactNumber(value):formatBytes(value)):unknownSeeding(snapshot);
}
