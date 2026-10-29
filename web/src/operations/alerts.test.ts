import {expect,it} from 'vitest';
import {alertNames,alertPresentation,type AlertRow} from './alerts';
const row=(extra:Partial<AlertRow>={}):AlertRow=>({code:'disk',state:'normal',value:25.382095336914062,message:'指标已恢复或告警已关闭',unit:'GiB',threshold:5,comparison:'<=',...extra});
it('names match all six real codes, not message text or the nonexistent journal code',()=>{
 expect(alertNames).toEqual({disk:'剩余存储空间',failures:'十分钟失败任务',load:'每 CPU 负载',memory:'内存使用率',plugin_journal:'插件日志占用',workers:'业务队列离线'});
 expect(alertPresentation(row()).name).toBe('剩余存储空间');expect(alertPresentation(row({code:'journal'})).name).toBe('journal');
});
it.each([
 ['disk','GiB',25.382095336914062,5,'<=','25.38 GiB','阈值 ≤ 5.00 GiB'],
 ['memory','percent',68.9797813785616,90,'>=','69.0%','阈值 ≥ 90.0%'],
 ['load','load_per_cpu',0.0575,2,'>=','0.06 /CPU','阈值 ≥ 2.00 /CPU'],
 ['failures','jobs_per_10m',3.7,3,'>=','4 条/10 分钟','阈值 ≥ 3 条/10 分钟'],
 ['workers','queues',0,1,'>=','0 个队列','阈值 ≥ 1 个队列'],
 ['plugin_journal','entries',1488,80000,'>=','1488 条','阈值 ≥ 80000 条'],
])('formats %s from API metadata',(code,unit,value,threshold,comparison,formatted,line)=>{
 const result=alertPresentation(row({code:String(code),unit:String(unit),value,threshold:Number(threshold),comparison:String(comparison)}));
 expect(result.value).toBe(formatted);expect(result.threshold).toBe(line);
});
it.each([{code:'future'}, {unit:undefined},{threshold:undefined},{comparison:undefined},{unit:'toString'},{threshold:NaN},{comparison:'!='}])('missing or unknown metadata falls back to original values: %o',extra=>{
 const result=alertPresentation(row(extra));expect(result.value).toBe('25.382095336914062');expect(result.threshold).toBe('');
});
it('hides only generic normal-state explanation and preserves active original text',()=>{
 expect(alertPresentation(row()).description).toBe('');expect(alertPresentation(row({message:'原始补充说明'})).description).toBe('原始补充说明');
 expect(alertPresentation(row({state:'active'})).description).toBe('指标已恢复或告警已关闭');
});
it('reuses existing badge tones; only active acknowledged rows are orange',()=>{
 expect(alertPresentation(row())).toMatchObject({badgeState:'ready',badgeLabel:'正常'});
 expect(alertPresentation(row({state:'active'}))).toMatchObject({badgeState:'failed',badgeLabel:'告警中'});
 expect(alertPresentation(row({state:'active',acknowledged_at:'2026-10-29T00:00:00Z'}))).toMatchObject({badgeState:'partial',badgeLabel:'已确认'});
 expect(alertPresentation(row({acknowledged_at:'2026-10-29T00:00:00Z'})).badgeState).toBe('ready');
 expect(alertPresentation(row({value:null})).value).toBe('—');
});
