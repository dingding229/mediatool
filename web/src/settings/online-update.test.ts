import {describe,it,expect,vi} from 'vitest';
import {newerStable,updateDone,updateFailures} from './online-update';
import {executeOnlineUpdate,getOnlineUpdate} from '../api/updates';
describe('F8b execution contract',()=>{
 it('never offers old stable version to a newer bootstrap build',()=>{expect(newerStable('0.28.0-dev+abc','0.28.0')).toBe(false);expect(newerStable('0.28.0-dev+abc','0.28.1')).toBe(true);expect(newerStable('0.28.1','0.28.0')).toBe(false);expect(newerStable('unknown','0.28.1')).toBe(false);expect(updateDone('manual_recovery')).toBe(true);expect(updateDone('applying')).toBe(false)});
 it('uses only registered API paths and never credentials in URL',async()=>{const fetch=vi.fn().mockResolvedValue({ok:true,status:202,headers:new Headers(),json:async()=>({data:{id:'fixture'}})});vi.stubGlobal('fetch',fetch);await executeOnlineUpdate('0.28.1','sha256:approved','test-password',true);const [url,init]=fetch.mock.calls[0];expect(url).toBe('/api/v1/system/online-update');expect(init.method).toBe('POST');expect(JSON.parse(init.body)).toEqual({version:'0.28.1',digest:'sha256:approved',password:'test-password',confirm:true});expect(init.credentials).toBe('same-origin');await getOnlineUpdate();expect(fetch.mock.calls[1][0]).toBe('/api/v1/system/online-update');vi.unstubAllGlobals()});
});

it('execution controls carry password only in JSON body',async()=>{
 const fetch=vi.fn().mockResolvedValue({ok:true,status:200,headers:new Headers(),json:async()=>({data:{ok:true}})});vi.stubGlobal('fetch',fetch);
 const {setOnlineUpdateControl}=await import('../api/updates');await setOnlineUpdateControl('confirm_recovery','fixture-job','fixture-secret',true);
 expect(fetch.mock.calls[0][0]).toBe('/api/v1/system/online-update/control');expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual({action:'confirm_recovery',jobId:'fixture-job',password:'fixture-secret',confirm:true});vi.unstubAllGlobals();
});

it('shows a clear manual-update requirement for changed image startup settings',()=>{expect(updateFailures.image_startup_config_changed).toBe('镜像启动配置已变化，请按文档手动更新')});
