import {it,expect,vi} from 'vitest';
import Proxy from './ProxySettings.vue';
import {request,APIError} from '../api/client';
import {setupComponent,flush} from '../test/setup-component';
vi.mock('../api/client',async original=>({...await original<typeof import('../api/client')>(),request:vi.fn()}));
export const initial={protocol:'http',host:'proxy.invalid',port:7890,username:'user',configured:true,hasPassword:true,needsInput:false,tmdb:false,sites:false,revision:2};
it('conflicting proxy save reads latest revision without overwriting typed fields or password',async()=>{
 vi.mocked(request).mockReset().mockResolvedValueOnce(initial).mockRejectedValueOnce(new APIError(409,'proxy_save_failed','changed')).mockResolvedValueOnce({...initial,revision:3});
 const v=setupComponent(Proxy);await flush();v.state.form.host='typed.invalid';v.state.form.password='typed-secret';v.state.form.sites=true;
 await v.state.save();expect(v.state.conflict.revision).toBe(3);expect(v.state.form.host).toBe('typed.invalid');expect(v.state.form.password).toBe('typed-secret');expect(v.state.form.sites).toBe(true);
 v.state.adopt();expect(v.state.saved.revision).toBe(3);expect(v.state.form.password).toBe('typed-secret');v.unmount();expect(v.state.form.password).toBe('');
});
it('requires a new password or explicit clear when identity changes, but ordinary blank means keep',async()=>{
 vi.mocked(request).mockReset().mockResolvedValue(initial);const v=setupComponent(Proxy);await flush();expect(v.state.validation).toBe('');v.state.form.port=1080;expect(v.state.validation).toContain('重填密码');
 await v.state.save();expect(request).toHaveBeenCalledTimes(1);v.state.form.clearPassword=true;expect(v.state.validation).toBe('');v.state.form.port=0;expect(v.state.validation).toContain('端口');v.unmount();
});
it('legacy payload is prominently needsInput, never converted to an empty saved proxy',async()=>{
 vi.mocked(request).mockReset().mockResolvedValue({...initial,host:'',port:0,username:'',hasPassword:false,needsInput:true,tmdb:true});const v=setupComponent(Proxy);await flush();expect(v.state.saved.needsInput).toBe(true);expect(v.state.form.tmdb).toBe(true);expect(v.state.validation).not.toBe('');v.state.form.clear=true;expect(v.state.validation).toContain('关闭两个代理路由');v.unmount();
});
it('an uncertain save stays blocked until a successful read and explicit revision adoption',async()=>{
 vi.mocked(request).mockReset().mockResolvedValueOnce(initial).mockRejectedValueOnce(new Error('network'));
 const v=setupComponent(Proxy);await flush();v.state.form.password='typed-secret';await v.state.save();expect(v.state.blocked).toBe(true);
 vi.mocked(request).mockResolvedValue({...initial,revision:3});await v.state.load(true);expect(v.state.blocked).toBe(true);expect(v.state.form.password).toBe('typed-secret');v.state.adopt();expect(v.state.blocked).toBe(false);expect(v.state.form.password).toBe('typed-secret');v.unmount();
});
