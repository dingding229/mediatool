import {expect,it} from 'vitest';
import {integerInputError} from './number-input';
it.each([0,'0',86400,'86400',' 120 ',120,'120.0'])('accepts legal integer seconds without changing units: %s',v=>{expect(integerInputError(v,0,86400,'秒')).toBe('')});
it.each(['',null,undefined,'abc','1e3','-1','1.5','86401',Infinity,NaN])('rejects invalid or out-of-range seconds: %s',v=>{expect(integerInputError(v,0,86400,'秒')).not.toBe('')});
it('preserves the TMDB cache upper boundary and zero-disable meaning',()=>{expect(integerInputError('2592000',0,2592000,'TMDB')).toBe('');expect(integerInputError('2592001',0,2592000,'TMDB')).not.toBe('')});
