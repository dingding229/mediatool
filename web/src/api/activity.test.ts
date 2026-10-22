import {expect,it,vi,afterEach} from 'vitest';
import {beginActivity,pendingRequests} from './activity';
import {request} from './client';
it('counts parallel activity and idempotent finishes without underflow',()=>{const a=beginActivity(),b=beginActivity();expect(pendingRequests.value).toBe(2);a();a();expect(pendingRequests.value).toBe(1);b();expect(pendingRequests.value).toBe(0)});
it.each(['error','invalid','abort'])('cleans up %s requests without changing transport security',async mode=>{const fetch=vi.fn(async(_path,options)=>{expect(options.credentials).toBe('same-origin');if(mode==='abort')throw new DOMException('aborted','AbortError');return{ok:mode!=='error',status:503,json:async()=>{if(mode==='invalid')throw Error('not json');return{error:{message:'offline'}}}}});vi.stubGlobal('fetch',fetch);await expect(request('/readiness')).rejects.toBeTruthy();expect(pendingRequests.value).toBe(0)});
afterEach(()=>vi.unstubAllGlobals());
