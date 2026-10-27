export const cronModes = [
 ['custom','自定义'],['seconds','每 N 秒'],['minutes','每 N 分钟'],
 ['hours','每 N 小时'],['hourly','每小时第 M 分'],['daily','每天'],['weekly','每周'],['monthly','每月'],
] as const;
export type CronMode = typeof cronModes[number][0];
export interface CronDraft {
 mode:CronMode; custom:string; interval:number|null; minute:number|null;
 clock:string; weekdays:number[]; monthdays:number[]; year:string;
}
const integer=(n:unknown,lo:number,hi:number)=>typeof n==='number'&&Number.isInteger(n)&&n>=lo&&n<=hi;
function yearValid(text:string){
 return text.split(',').every(item=>{
  if(!/^(?:\*|\d{4}(?:-\d{4})?)(?:\/\d{1,3})?$/.test(item))return false;
  const [span,step]=item.split('/');if(step&&!integer(Number(step),1,230))return false;
  if(span==='*')return true;
  const [lo,hi=lo]=span!.split('-').map(Number);return integer(lo,1970,2199)&&integer(hi,lo!,2199);
 });
}
export function draftError(d:CronDraft){
 if(d.mode==='custom')return '';
 if(!yearValid(d.year.trim()||'*'))return '年字段须为1970～2199，支持星号、列表、范围及步长';
 if(['seconds','minutes','hours'].includes(d.mode))return integer(d.interval,1,d.mode==='hours'?23:59)?'':`间隔请输入1～${d.mode==='hours'?23:59}的整数`;
 if(d.mode==='hourly')return integer(d.minute,0,59)?'':'分钟请输入0～59的整数';
 if(!/^([01]\d|2[0-3]):[0-5]\d$/.test(d.clock))return '时刻请输入有效的 HH:mm（00:00～23:59）';
 if(d.mode==='weekly'&&(!d.weekdays.length||d.weekdays.some(n=>!integer(n,0,6))))return '请至少选择一个有效星期';
 if(d.mode==='monthly'&&(!d.monthdays.length||d.monthdays.some(n=>!integer(n,1,31))))return '请至少选择一个有效日期';
 return '';
}
const list=(values:number[])=>[...new Set(values)].sort((a,b)=>a-b).join(',');
export function generateCron(d:CronDraft){
 if(d.mode==='custom')return d.custom;
 if(draftError(d))return '';
 const [h,m]=d.clock.split(':').map(Number);let f:string[];
 switch(d.mode){
  case 'seconds':f=[`*/${d.interval}`,'*','*','*','*','*'];break;
  case 'minutes':f=['0',`*/${d.interval}`,'*','*','*','*'];break;
  case 'hours':f=['0','0',`*/${d.interval}`,'*','*','*'];break;
  case 'hourly':f=['0',String(d.minute),'*','*','*','*'];break;
  default:f=['0',String(m),String(h),d.mode==='monthly'?list(d.monthdays):'*','*',d.mode==='weekly'?list(d.weekdays):'*'];
 }
 return [...f,d.year.trim()||'*'].join(' ');
}
export function decodeCron(expression:string):CronDraft {
 const d:CronDraft={mode:'custom',custom:expression,interval:1,minute:0,clock:'09:00',weekdays:[1],monthdays:[1],year:'*'};
 const raw=expression.trim().split(/\s+/);if(![5,6,7].includes(raw.length))return d;
 const f=raw.length===5?['0',...raw,'*']:raw.length===6?[...raw,'*']:raw;
 const [sec,min,hour,day,month,week,year]=f as [string,string,string,string,string,string,string];
 if(month!=='*'||!yearValid(year))return d;
 const single=(s:string,max:number)=>/^\d{1,2}$/.test(s)&&integer(Number(s),0,max);
 const interval=(s:string,max:number)=>/^\*\/\d{1,2}$/.test(s)&&integer(Number(s.slice(2)),1,max);
 const days=(s:string,lo:number,hi:number)=>/^\d{1,2}(?:,\d{1,2})*$/.test(s)&&s.split(',').every(n=>integer(Number(n),lo,hi));
 if(day==='*'&&week==='*'){
  if(min==='*'&&hour==='*'&&interval(sec,59)){d.mode='seconds';d.interval=Number(sec.slice(2))}
  else if(sec==='0'&&hour==='*'&&interval(min,59)){d.mode='minutes';d.interval=Number(min.slice(2))}
  else if(sec==='0'&&min==='0'&&interval(hour,23)){d.mode='hours';d.interval=Number(hour.slice(2))}
  else if(sec==='0'&&single(min,59)&&hour==='*'){d.mode='hourly';d.minute=Number(min)}
 }
 if(d.mode==='custom'&&sec==='0'&&single(min,59)&&single(hour,23)){
  if(day==='*'&&week==='*')d.mode='daily';
  else if(day==='*'&&days(week,0,6)){d.mode='weekly';d.weekdays=[...new Set(week.split(',').map(Number))].sort((a,b)=>a-b)}
  else if(week==='*'&&days(day,1,31)){d.mode='monthly';d.monthdays=[...new Set(day.split(',').map(Number))].sort((a,b)=>a-b)}
  if(d.mode!=='custom')d.clock=hour.padStart(2,'0')+':'+min.padStart(2,'0');
 }
 if(d.mode!=='custom')d.year=year;
 return d;
}
