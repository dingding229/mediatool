import { afterEach,describe,expect,it,vi } from 'vitest'
import { request,post,APIError } from './client'
afterEach(()=>vi.unstubAllGlobals())
describe('API client',()=>{
 it('uses same-origin credentials and CSRF header without persistent secrets',async()=>{const fetch=vi.fn().mockResolvedValue({ok:true,status:200,json:async()=>({data:{ok:true}})});vi.stubGlobal('fetch',fetch);await post('/jobs',{kind:'system.check'});const [,options]=fetch.mock.calls[0]!;expect(options.credentials).toBe('same-origin');expect(options.headers.get('X-Requested-With')).toBe('media-tool');expect(options.headers.get('Authorization')).toBeNull()})
 it('surfaces backend errors instead of returning simulated records',async()=>{vi.stubGlobal('fetch',vi.fn().mockResolvedValue({ok:false,status:501,json:async()=>({error:{code:'not_implemented',message:'待实现'}})}));await expect(request('/sites')).rejects.toEqual(new APIError(501,'not_implemented','待实现'))})
})
it('keeps the generated requestId on HTTP and malformed response failures',async()=>{
 const requestId='abcdef0123456789abcdef01'
 vi.stubGlobal('fetch',vi.fn().mockResolvedValue({ok:false,status:409,headers:new Headers({'X-Request-ID':requestId}),json:async()=>({error:{code:'plugin_conflict',message:'refused'}})}))
 await expect(post('/plugins/operations',{})).rejects.toMatchObject({status:409,requestId})
 vi.stubGlobal('fetch',vi.fn().mockResolvedValue({ok:true,status:200,headers:new Headers({'X-Request-ID':requestId}),json:async()=>{throw Error('invalid')}}))
 await expect(post('/plugins/operations',{})).rejects.toMatchObject({code:'invalid_response',requestId})
})
it('preserves structured recovery guidance without replaying an unknown effect',async()=>{
 const fetch=vi.fn().mockResolvedValue({ok:false,status:409,json:async()=>({error:{code:'upload_outcome_unknown',message:'先核对',flow:{phase:'organize',retryable:false,nextAction:'reconcile_upload',reason:'upload_outcome_unknown'}}})})
 vi.stubGlobal('fetch',fetch)
 await expect(post('/pipeline/transfers/owned/retry',{})).rejects.toMatchObject({flow:{retryable:false,nextAction:'reconcile_upload'}})
 expect(fetch).toHaveBeenCalledTimes(1)
})
it('closes SSE on error',async()=>{
 const {subscribeJobEvents}=await import('./client')
 const transport=vi.fn().mockResolvedValue({ok:true,status:200,json:async()=>({data:{}}),text:async()=> 'https://example.test/stable'})
 vi.stubGlobal('fetch',transport)
 const close=vi.fn(),tick=vi.fn();let stream:any
 vi.stubGlobal('EventSource',class{onerror:any;close=close;constructor(public path:string,public options:unknown){stream=this}addEventListener(event:string,fn:()=>void){expect(event).toBe('tick');fn()}})
 const subscription=subscribeJobEvents(tick)
 expect(stream.path).toBe('/api/v1/jobs/events');expect(tick).toHaveBeenCalledOnce();stream.onerror();subscription?.close();expect(close).toHaveBeenCalledTimes(2)
})
