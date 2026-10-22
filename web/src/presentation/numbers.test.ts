import {expect,it} from 'vitest'
import {formatCompactNumber,formatRatio} from './numbers'
it.each([[0,'0'],[9999,'9,999'],[10000,'1.00万'],[79300,'7.93万'],[9999999,'1000.00万'],[10000000,'1.00千万'],[10100000,'1.01千万'],[-10000,'-1.00万'],[null,'—'],[NaN,'—'],[Infinity,'—']])('formats compact count %s without mixed units',(value,expected)=>expect(formatCompactNumber(value)).toBe(expected))
it.each([[0,'0.00'],[1,'1.00'],[1/3,'0.33'],[2086.59,'2086.59'],[null,'—'],[Infinity,'—']])('keeps two ratio decimals %s',(value,expected)=>expect(formatRatio(value)).toBe(expected))
