import {expect,it} from 'vitest'
import {setupComponent,deferred} from '../test/setup-component'
import {useReadState} from './read-state'
it('read failure is unknown, retry establishes empty; refresh failure keeps successful data',async()=>{
 const v=setupComponent({setup:()=>({read:useReadState()})});const s=v.state.read;let data=['old']
 try{expect(s).toMatchObject({loading:true,loaded:false,error:''});await s.run(()=>Promise.reject(new Error('private-response')),()=>data=[]);expect(s).toMatchObject({loading:false,loaded:false});expect(s.error).not.toContain('private-response');expect(data).toEqual(['old']);await s.run(()=>Promise.resolve([]),(x:string[])=>data=x);expect(s).toMatchObject({loading:false,loaded:true,error:''});expect(data).toEqual([]);await s.run(()=>Promise.reject(new Error()),()=>data=['invalid']);expect(s.loaded).toBe(true);expect(data).toEqual([])}finally{v.unmount()}
})
it('late results, reset and disposed dialog replies cannot overwrite the current value',async()=>{
 const v=setupComponent({setup:()=>({read:useReadState()})});const s=v.state.read;let data='';const old=deferred<string>();const task=s.run(()=>old.promise,(x:string)=>data=x)
 await s.run(()=>Promise.resolve('latest'),(x:string)=>data=x);old.resolve('stale');await task;expect(data).toBe('latest')
 const disposed=deferred<string>();const last=s.run(()=>disposed.promise,(x:string)=>data=x);v.unmount();disposed.resolve('disposed');await last;expect(data).toBe('latest')
})
