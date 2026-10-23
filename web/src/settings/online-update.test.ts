import {describe,it,expect,vi} from 'vitest';
import {newerStable,updateDone} from './online-update';
import {executeOnlineUpdate,getOnlineUpdate} from '../api/updates';
describe('F8b execution contract',()=>{
 it('never offers old stable version to a newer bootstrap build',()=>{expect(newerStable('0.28.0-dev+abc','0.28.0')).toBe(false);expect(newerStable('0.28.0-dev+abc','0.28.1')).toBe(true);expect(newerStable('0.28.1','0.28.0')).toBe(false);expect(newerStable('unknown','0.28.1')).toBe(false);expect(updateDone('manual_recovery')).toBe(true);expect(updateDone('applying')).toBe(false)});
 it('uses only registered API paths and never credentials in URL',async()=>{const fetch=vi.fn().mockResolvedValue({ok:true,status:202,headers:new Headers(),json:async()=>({data:{id:'fixture'}})});vi.stubGlobal('fetch',fetch);await executeOnlineUpdate('0.28.1','sha256:approved','test-password',true);const [url,init]=fetch.mock.calls[0];expect(url).toBe('/api/v1/system/online-update');expect(init.method).toBe('POST');expect(JSON.parse(init.body)).toEqual({version:'0.28.1',digest:'sha256:approved',password:'test-password',confirm:true});expect(init.credentials).toBe('same-origin');await getOnlineUpdate();expect(fetch.mock.calls[1][0]).toBe('/api/v1/system/online-update');vi.unstubAllGlobals()});
});
