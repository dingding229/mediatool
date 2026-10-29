import {trendSegments} from './model';
const digitsFor=(unit:string)=>unit==='%'?1:['MiB','条','次'].includes(unit)?0:2;
export function formatTrendValue(value:number|undefined,unit:string){
 if(value==null||!Number.isFinite(value))return '—';
 return value.toFixed(digitsFor(unit))+(unit?(unit==='%'?'':' ')+unit:'');
}
const timeFormat=new Intl.DateTimeFormat('en-GB',{timeZone:'Asia/Shanghai',hour:'2-digit',minute:'2-digit',hourCycle:'h23'});
const dayFormat=new Intl.DateTimeFormat('en-CA',{timeZone:'Asia/Shanghai',month:'2-digit',day:'2-digit'});
const timeLabel=(at:number,crossDay:boolean)=>crossDay?`${dayFormat.format(at)} ${timeFormat.format(at)}`:timeFormat.format(at);
export function trendTimeTicks(start:number,end:number){
 const crossDay=dayFormat.format(start)!==dayFormat.format(end);
 return [0,.5,1].map(f=>({position:(8+304*f)/320*100,label:timeLabel(start+(end-start)*f,crossDay)}));
}
export function trendRange(values:number[],unit:string){
 if(!values.length)return {min:0,max:1};
 let min=Math.min(...values),max=Math.max(...values);
 if(min===max){const padding=min===0?1:Math.max(10**-digitsFor(unit),Math.abs(min)*.1);min=min>=0?Math.max(0,min-padding):min-padding;max+=padding}
 const half=(max-min)/2,power=10**Math.floor(Math.log10(half));
 const step=Math.max(10**-digitsFor(unit),([1,2,5,10].find(n=>n*power>=half)||10)*power);
 const lower=Math.floor(min/step)*step,upper=lower+Math.max(2,Math.ceil((max-lower)/(2*step))*2)*step;
 return {min:lower,max:upper};
}
export function trendPlot(rows:any[],metric:string,gapMs:number,unit:string){
 const valid=(r:any)=>typeof r[metric]==='number'&&Number.isFinite(r[metric])&&Number.isFinite(Date.parse(r.at));
 const samples=rows.filter(valid).map(r=>({at:Date.parse(r.at),value:r[metric] as number}));
 if(!samples.length)return {values:[],segments:[],dots:[],yTicks:[],xTicks:[]};
 const values=samples.map(s=>s.value),range=trendRange(values,unit);
 const normalized=rows.map(r=>({...r,[metric]:valid(r)?(r[metric]-range.min)/(range.max-range.min):null}));
 const times=rows.map(r=>Date.parse(r.at)).filter(Number.isFinite);
 let start=Math.min(...times),end=Math.max(...times);
 if(start===end){start-=Math.max(gapMs,60000);end+=Math.max(gapMs,60000)}
 const first=samples[0]!.at,last=samples.at(-1)!.at;
 // Keep the original grouping/gap algorithm; only rescale coordinates for readable axes.
 const groups=trendSegments(normalized,metric,gapMs).map(group=>group.split(' ').map(point=>{
  const [x,y]=point.split(',').map(Number),at=first+(x!-8)/304*(last-first);
  return {x:8+(at-start)/(end-start)*304,y:y!};
 }));
 const segments=groups.filter(g=>g.length>1).map(g=>g.map(p=>`${p.x},${p.y}`).join(' '));
 const dots=groups.filter(g=>g.length===1).map(g=>({x:g[0]!.x/320*100,y:g[0]!.y/105*100}));
 const yTicks=[range.max,(range.min+range.max)/2,range.min].map((value,i)=>({value,position:(12+i*40)/105*100,label:formatTrendValue(value,unit)}));
 return {values,segments,dots,yTicks,xTicks:trendTimeTicks(start,end)};
}
