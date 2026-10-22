const clock=new Intl.DateTimeFormat('zh-CN',{timeZone:'Asia/Shanghai',year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit',hourCycle:'h23'});
export function beijingTime(value?:unknown,fallback='—'){
 if(typeof value!=='string'||!value)return fallback;
 const date=new Date(value.replace(/(\.\d{3})\d+(?=Z|[+-])/,'$1'));if(!Number.isFinite(date.getTime()))return fallback;
 const p=Object.fromEntries(clock.formatToParts(date).map(part=>[part.type,part.value]));return `${p.year}-${p.month}-${p.day} ${p.hour}:${p.minute}:${p.second}`;
}
export function beijingISO(value:string){return new Date(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(?::\d{2})?$/.test(value)?value+'+08:00':value).toISOString()}
