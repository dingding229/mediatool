// Text entry keeps invalid drafts visible instead of silently clamping time values.
export function integerInputError(value:unknown,min:number,max:number,label:string):string{
 const text=String(value??'').trim();
 if(!/^\+?\d+(?:\.0+)?$/.test(text)||!Number.isSafeInteger(Number(text))||Number(text)<min||Number(text)>max)return `${label}请输入 ${min}～${max} 的整数`;
 return '';
}

const cronBounds=[[0,59],[0,23],[1,31],[1,12],[0,6]] as const;
const months=['jan','feb','mar','apr','may','jun','jul','aug','sep','oct','nov','dec'];
const weekdays=['sun','mon','tue','wed','thu','fri','sat'];
function cronValues(field:string,index:number):number[]{
 const [min,max]=cronBounds[index]!;const out=new Set<number>();
 const integer=(text:string)=>{if(!/^\+?\d+$/.test(text)||BigInt(text)>9223372036854775807n)throw Error();return Number(text)};
 const number=(text:string)=>{const names=index===3?months:index===4?weekdays:[];const named=names.indexOf(text.toLowerCase());return named<0?integer(text):named+(index===3?1:0)};
 for(const item of field.split(',')){
  const parts=item.split('/'),range=parts[0]!.split('-');if(parts.length>2)throw Error();
  let start:number,end:number;
  if(range[0]==='*'||range[0]==='?'){if(range.length!==1)throw Error();start=min;end=max}else{if(range.length>2)throw Error();start=number(range[0]!);end=range.length===2?number(range[1]!):start}
  const step=parts.length===2?integer(parts[1]!):1;if(parts.length===2&&range.length===1)end=max;
  if(start<min||end>max||start>end||step===0)throw Error();
  for(let n=start;n<=end;n+=step)out.add(n);
 }
 return [...out].sort((a,b)=>a-b);
}
// Mirrors the existing five-field parser syntax/ranges, including named months,
// weekdays, lists and steps. Calendar feasibility and minimum interval retain
// the backend's authoritative Asia/Shanghai scheduling.Next validation.
export function subscriptionCronError(value:unknown):string{
 const expression=String(value??'').trim(),parts=expression.split(/\s+/);
 if(new TextEncoder().encode(expression).length>120||parts.length!==5||expression.includes('@'))return '订阅调度时间请输入五字段表达式（分 时 日 月 周）';
 const labels=['分钟（0～59）','小时（0～23）','日期（1～31）','月份（1～12）','星期（0～6）'];const values:number[][]=[];
 for(let i=0;i<5;i++){try{values.push(cronValues(parts[i]!,i))}catch{return `订阅调度时间的${labels[i]}格式或范围无效`}}
 const minutes=values[0]!,hours=values[1]!;
 if(minutes.some((n,i)=>i>0&&n-minutes[i-1]!<5)||hours.some((n,i)=>i>0&&n-hours[i-1]===1&&60-minutes.at(-1)!+minutes[0]!<5))return '订阅调度间隔不能少于 5 分钟';
 return '';
}
