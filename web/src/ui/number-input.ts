// Text entry keeps invalid drafts visible instead of silently clamping time values.
export function integerInputError(value:unknown,min:number,max:number,label:string):string{
 const text=String(value??'').trim();
 if(!/^\+?\d+(?:\.0+)?$/.test(text)||!Number.isSafeInteger(Number(text))||Number(text)<min||Number(text)>max)return `${label}请输入 ${min}～${max} 的整数`;
 return '';
}
