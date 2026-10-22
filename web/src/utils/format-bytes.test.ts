import { expect, test } from 'vitest'
import { formatBytes } from './format-bytes'
test.each([[null,'—'],[NaN,'—'],[-1,'—'],[0,'0.00 B'],[999,'999.00 B'],[1000,'0.98 KiB'],[1023,'1.00 KiB'],[1024,'1.00 KiB'],[999*1024**2,'999.00 MiB'],[1000*1024**2,'0.98 GiB'],[1023*1024**2,'1.00 GiB'],[1024**5,'1.00 PiB'],['1023 MiB','1.00 GiB'],['1000MB','0.98 GiB'],['unknown','—']] as const)('capacity %s → %s',(input,expected)=>expect(formatBytes(input)).toBe(expected))
