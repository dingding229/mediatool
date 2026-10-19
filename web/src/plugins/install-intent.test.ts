import {afterEach,beforeEach,expect,it,vi} from 'vitest'
import {installIntent,hasInstallIntent,completeInstallIntent} from './install-intent'
beforeEach(()=>{const entries=new Map<string,string>();vi.stubGlobal('sessionStorage',{getItem:(k:string)=>entries.get(k)??null,setItem:(k:string,v:string)=>entries.set(k,v)})})
afterEach(()=>vi.unstubAllGlobals())
it('retains one public-identity key through reload/retry and releases it only on acknowledgement',()=>{
 const generate=vi.fn().mockReturnValueOnce('opaque-key-one').mockReturnValueOnce('opaque-key-two')
 expect(installIntent('hello-go','fingerprint',generate)).toBe('opaque-key-one')
 expect(hasInstallIntent('hello-go','fingerprint')).toBe(true)
 expect(installIntent('hello-go','fingerprint',generate)).toBe('opaque-key-one');expect(generate).toHaveBeenCalledTimes(1)
 const raw=sessionStorage.getItem('media-tool:plugin-install-intents:v1')!;expect(JSON.parse(raw)).toEqual({'hello-go:fingerprint':'opaque-key-one'})
 completeInstallIntent('hello-go','fingerprint');expect(hasInstallIntent('hello-go','fingerprint')).toBe(false)
 expect(installIntent('hello-go','fingerprint',generate)).toBe('opaque-key-two')
})
it('keeps the view usable when browser storage is disabled',()=>{
 vi.stubGlobal('sessionStorage',{getItem:()=>{throw Error('blocked')},setItem:()=>{throw Error('blocked')}})
 expect(installIntent('hello-go','fp',()=> 'opaque-key')).toBe('opaque-key')
})
it('bounds unresolved intents instead of evicting keys whose outcome is unknown',()=>{
 for(let i=0;i<32;i++)installIntent('plugin-'+i,'fp',()=> 'opaque-key-'+i)
 expect(()=>installIntent('another','fp',()=> 'opaque-key-more')).toThrow('install_intent_limit')
})
