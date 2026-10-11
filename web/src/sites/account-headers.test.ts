import {describe,expect,it} from 'vitest'
import {accountHeadersError} from './account-headers'
describe('encrypted PT account header feedback',()=>{
 it('accepts empty preservation, explicit clear and literal auth values',()=>{
  for(const value of ['', '{}', ' { } ', '{"Authorization":"Bearer literal{passkey}","X-Csrf-Token":"owned"}'])expect(accountHeadersError(value)).toBe('')
 })
 it('rejects invalid JSON and non-string or empty values without echoing them',()=>{
  for(const raw of ['not PRIVATE-JSON','null','[]','{"Authorization":12}','{"Authorization":""}','{"Authorization":"PRIVATE-HEADER\\r\\nInjected: secret"}']){
   const message=accountHeadersError(raw);expect(message).not.toBe('');expect(message).not.toContain('PRIVATE-HEADER')
  }
 })
 it('rejects transport, cookie and forwarding headers and duplicate case names',()=>{
  for(const name of ['Host','Cookie','Content-Length','Proxy-Authorization','X-Forwarded-For','Connection'])expect(accountHeadersError(JSON.stringify({[name]:'owned'}))).not.toBe('')
  expect(accountHeadersError('{"Authorization":"one","authorization":"two"}')).not.toBe('')
  expect(accountHeadersError('{"Authorization":"one","Authorization":"two"}')).not.toBe('')
  expect(accountHeadersError('{"Authorizatio\\u006e":"one","Authorization":"two"}')).not.toBe('')
 })
 it('does not treat quoted JSON-like text inside literal credentials as header keys',()=>{
  expect(accountHeadersError(JSON.stringify({Authorization:'literal \"Authorization\": value : {passkey}', 'X-Owned':'text \\\"}'}))).toBe('')
 })
 it('bounds header count and total length',()=>{
  expect(accountHeadersError(JSON.stringify(Object.fromEntries(Array.from({length:17},(_,i)=>[`X-Owned-${i}`,'owned']))))).not.toBe('')
  expect(accountHeadersError(' '.repeat(16385))).not.toBe('')
  expect(accountHeadersError(JSON.stringify({Authorization:'中'.repeat(8193)}))).not.toBe('')
 })
})
