import { describe,it,expect } from 'vitest'
import { formatLogTime,logLevel,logSource,logFields,inlineLogFields,logText } from './format'
describe('log presentation',()=>{
 it('formats valid timestamps without inventing absent dates',()=>{
  expect(formatLogTime('2026-10-12T07:41:04.123456789+08:00')).toMatch(/^2026-10-(11|12) \d{2}:41:04$/)
  expect(formatLogTime('bad')).toBe('时间未知');expect(formatLogTime(null)).toBe('时间未知')
 })
 it('keeps severity names and colors independent of messages',()=>{
  expect(logLevel('ERROR')).toBe('error');expect(logLevel('warning')).toBe('warn');expect(logLevel('red')).toBe('unknown')
 })
 it('formats genuine source and fields without manufacturing trace IDs',()=>{
  const row={time:'2026-10-12T07:00:00+08:00',level:'INFO',msg:'Event',module:'search',source:{file:'internal/modules/search/run.go',line:16},durationMs:5,requestId:'owned-fixture'}
  expect(logSource(row)).toBe('internal/modules/search/run.go:16');expect(logSource({})).toBe('')
  expect(logFields(row)).toEqual({durationMs:5,requestId:'owned-fixture'});expect(inlineLogFields(row)).toContain('durationMs=5')
  expect(logText([row],true)).not.toContain('requestId');expect(logText([row],false)).toContain('run.go:16');expect(logText([row],false)).toContain('owned-fixture');expect(inlineLogFields(row)).not.toContain('requestId')
 })
 it('caps inline values but retains the original structured detail',()=>{
  const row={msg:'owned',detail:'x'.repeat(500)};expect(inlineLogFields(row).length).toBeLessThan(160);expect(logFields(row).detail).toHaveLength(500)
 })
})
