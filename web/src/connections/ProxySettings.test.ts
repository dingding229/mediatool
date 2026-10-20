import {it,expect,vi} from 'vitest'
import Proxy from './ProxySettings.vue'
import { request,APIError } from '../api/client'
import { setupComponent,flush } from '../test/setup-component'
vi.mock('../api/client',async original=>({...await original<typeof import('../api/client')>(),request:vi.fn()}))
it('conflicting proxy save reads latest revision without overwriting user URL or scope inputs',async()=>{vi.mocked(request).mockReset().mockResolvedValueOnce({url:'http://old-secret:pass@proxy.invalid',tmdb:false,sites:false,revision:2});const view=setupComponent(Proxy);await flush();expect(view.state.input).toBe('');view.state.input='http://new-user:private@new.invalid';view.state.tmdb=true;vi.mocked(request).mockRejectedValueOnce(new APIError(409,'proxy_save_failed','revision conflict')).mockResolvedValueOnce({url:'http://server.invalid',tmdb:false,sites:true,revision:3});await view.state.save();expect(view.state.error).toContain('配置已被修改，已重新读取');expect(view.state.input).toBe('http://new-user:private@new.invalid');expect(view.state.tmdb).toBe(true);view.state.adopt();expect(view.state.saved.revision).toBe(3);expect(view.state.input).toContain('new-user');view.unmount()})
