import {expect,it} from 'vitest';
import {integerInputError,subscriptionCronError} from './number-input';
it.each([0,'0',86400,'86400',' 120 ',120,'120.0'])('accepts legal integer seconds without changing units: %s',v=>{expect(integerInputError(v,0,86400,'秒')).toBe('')});
it.each(['',null,undefined,'abc','1e3','-1','1.5','86401',Infinity,NaN])('rejects invalid or out-of-range seconds: %s',v=>{expect(integerInputError(v,0,86400,'秒')).not.toBe('')});
it('preserves the TMDB cache upper boundary and zero-disable meaning',()=>{expect(integerInputError('2592000',0,2592000,'TMDB')).toBe('');expect(integerInputError('2592001',0,2592000,'TMDB')).not.toBe('')});
it.each(['*/5 * * * *','*/30 * * * *','0 */2 * * *','0 0 * JAN MON','0 0 * jan-dec MON-FRI','0,58 0,23 * * MON','0 0 ? * 0','0 0 1/2 * *','+0 0 * * *','0/9223372036854775807 * * * *'])('retains supported cron grammar: %s',s=>{expect(subscriptionCronError(s)).toBe('')});
it.each(['','0 0 * *','60 * * * *','0 24 * * *','0 0 0 * *','0 0 * 13 *','0 0 * * 7','*/0 * * * *','*-10 * * * *','?-5 * * * *','*/-1 * * * *','0 0 * XYZ *','0 0 * * MON-SUN','@hourly','0 0 * * * *','0 0 * * * '.repeat(15)])('rejects bad cron format or bounds: %s',s=>{expect(subscriptionCronError(s)).not.toBe('')});
it.each(['* * * * *','*/2 * * * *','1,3 0 * * *','0,59 0,1 * * MON'])('rejects definitely sub-five-minute scheduling without weakening backend calendar checks: %s',s=>{expect(subscriptionCronError(s)).toContain('5 分钟')});
