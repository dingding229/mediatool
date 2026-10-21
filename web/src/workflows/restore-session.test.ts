import {afterEach,expect,it,vi} from 'vitest'
import {pendingRestore,rememberRestore,clearRestore} from './restore-session'
afterEach(()=>vi.unstubAllGlobals())
it('persists only the receipt, never credentials, and clears only the matching result',()=>{
 const values=new Map<string,string>();vi.stubGlobal('sessionStorage',{getItem:(k:string)=>values.get(k),setItem:(k:string,v:string)=>values.set(k,v),removeItem:(k:string)=>values.delete(k)})
 const id='11111111-1111-4111-8111-111111111111';rememberRestore('not-a-receipt');expect(pendingRestore()).toBe('');rememberRestore(id);expect(pendingRestore()).toBe(id);clearRestore('another');expect(pendingRestore()).toBe(id);clearRestore(id);expect(values.size).toBe(0)
})
it('restricted storage does not break login or restore messages',()=>{vi.stubGlobal('sessionStorage',{getItem:()=>{throw Error('blocked')},setItem:()=>{throw Error('blocked')}});expect(pendingRestore()).toBe('');expect(()=>rememberRestore('11111111-1111-4111-8111-111111111111')).not.toThrow()})
