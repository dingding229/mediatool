export type CronPreset='half-hour'|'hour'|'six-hours'|'daily'|'twelve-hours'|'weekly'|'custom';
export const cronPresets: [CronPreset,string][]=[['half-hour','每 30 分钟'],['hour','每小时'],['six-hours','每 6 小时'],['daily','每天某个时刻'],['twelve-hours','每 12 小时'],['weekly','每周日 03:00'],['custom','自定义']];
export function presetCron(preset:CronPreset,clock='09:00'){
 const [hour,minute]=clock.split(':').map(Number);
 if(preset==='daily')return /^\d{2}:\d{2}$/.test(clock)&&hour!<24&&minute!<60?`${minute} ${hour} * * *`:'';
 return {'half-hour':'*/30 * * * *',hour:'0 * * * *','six-hours':'0 */6 * * *','twelve-hours':'0 */12 * * *',weekly:'0 3 * * 0',custom:''}[preset];
}
export function cronPreset(expression:string):{preset:CronPreset;clock:string}{
 const fields=expression.trim().split(/\s+/);
 // A zero seconds prefix and unrestricted year are equivalent to five-field presets.
 if(fields.length===7&&fields[6]==='*')fields.pop();
 if(fields.length===6&&fields[0]==='0')fields.shift();
 const text=fields.join(' ');
 for(const preset of ['half-hour','hour','six-hours','twelve-hours','weekly'] as const)if(text===presetCron(preset))return {preset,clock:'09:00'};
 if(fields.length===5&&fields.slice(2).every(f=>f==='*')&&/^\d+$/.test(fields[0]!)&&/^\d+$/.test(fields[1]!)&&Number(fields[0])<60&&Number(fields[1])<24)return {preset:'daily',clock:`${fields[1]!.padStart(2,'0')}:${fields[0]!.padStart(2,'0')}`};
 return {preset:'custom',clock:'09:00'};
}
