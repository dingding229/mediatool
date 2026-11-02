export type DurationUnit='minutes'|'hours'|'days';
export const durationUnits: [DurationUnit,string,number][]=[['minutes','分钟',60],['hours','小时',3600],['days','天',86400]];
export function decodeDuration(seconds:number){
 const selected=[...durationUnits].reverse().find(([, ,scale])=>seconds>0&&seconds%scale===0)||durationUnits[0]!;
 return {unit:selected[0],value:String(seconds/selected[2])};
}
export function durationSeconds(value:string,unit:DurationUnit,max:number){
 if(!/^\d+(?:\.\d+)?$/.test(value.trim()))return null;
 const scale=durationUnits.find(([key])=>key===unit)![2],raw=Number(value)*scale,n=Math.round(raw);
 return Number.isSafeInteger(n)&&n>=0&&n<=max&&Math.abs(raw-n)<1e-8?n:null;
}
