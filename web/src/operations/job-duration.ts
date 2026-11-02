export function jobDuration(row:{startedAt?:string;updatedAt?:string;state?:string},now=Date.now()){
 const start=Date.parse(row.startedAt||''),end=row.state==='running'?now:Date.parse(row.updatedAt||'');
 return Number.isFinite(start)&&Number.isFinite(end)?`耗时 ${Math.max(0,Math.round((end-start)/1000))} 秒`:'耗时 —';
}
