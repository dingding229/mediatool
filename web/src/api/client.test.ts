import { afterEach,describe,expect,it,vi } from 'vitest'
import { request,post,APIError } from './client'
afterEach(()=>vi.unstubAllGlobals())
describe('API client',()=>{
 it('uses same-origin credentials and CSRF header without persistent secrets',async()=>{const fetch=vi.fn().mockResolvedValue({ok:true,status:200,json:async()=>({data:{ok:true}})});vi.stubGlobal('fetch',fetch);await post('/jobs',{kind:'system.check'});const [,options]=fetch.mock.calls[0]!;expect(options.credentials).toBe('same-origin');expect(options.headers.get('X-Requested-With')).toBe('media-tool');expect(options.headers.get('Authorization')).toBeNull()})
 it('surfaces backend errors instead of returning simulated records',async()=>{vi.stubGlobal('fetch',vi.fn().mockResolvedValue({ok:false,status:501,json:async()=>({error:{code:'not_implemented',message:'待实现'}})}));await expect(request('/sites')).rejects.toEqual(new APIError(501,'not_implemented','待实现'))})
})
