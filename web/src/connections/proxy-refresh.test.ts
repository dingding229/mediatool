import {it,expect,vi} from 'vitest';
import Proxy from './ProxySettings.vue';
import {request} from '../api/client';
import {setupComponent,flush} from '../test/setup-component';
vi.mock('../api/client',async original=>({...await original<typeof import('../api/client')>(),request:vi.fn()}));
it('page refresh preserves structured proxy fields, unsaved password and scope choices',async()=>{
 const original={protocol:'http',host:'saved.invalid',port:7890,username:'user',configured:true,hasPassword:true,needsInput:false,tmdb:true,sites:false,revision:1};
 vi.mocked(request).mockReset().mockResolvedValue(original);const v=setupComponent(Proxy);await flush();v.state.form.host='typed.invalid';v.state.form.password='typed-secret';v.state.form.tmdb=false;
 vi.mocked(request).mockResolvedValue({...original,host:'changed.invalid',revision:2});await v.state.load(true);expect(v.state.form.host).toBe('typed.invalid');expect(v.state.form.password).toBe('typed-secret');expect(v.state.form.tmdb).toBe(false);expect(v.state.conflict.revision).toBe(2);v.unmount();
});
