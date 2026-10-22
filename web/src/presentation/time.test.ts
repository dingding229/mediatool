import {expect,it} from 'vitest';
import {beijingTime,beijingISO} from './time';
it('uses Beijing independently of browser/server timezone, including midnight',()=>{expect(beijingTime('2026-10-22T16:00:00Z')).toBe('2026-10-23 00:00:00');expect(beijingTime('2026-10-12T07:41:04.123456789+08:00')).toBe('2026-10-12 07:41:04')});
it('does not invent missing dates',()=>{for(const v of [undefined,null,'','bad'])expect(beijingTime(v)).toBe('—');expect(beijingTime('bad','时间未知')).toBe('时间未知')});
it('interprets time-range inputs as Beijing and preserves explicit offsets',()=>{expect(beijingISO('2026-10-23T00:00:00')).toBe('2026-10-22T16:00:00.000Z');expect(beijingISO('2026-10-23T00:00:00Z')).toBe('2026-10-23T00:00:00.000Z')});
