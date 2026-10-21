import {it,expect,vi} from 'vitest';
import Proxy from './ProxySettings.vue';
import {request} from '../api/client';
import {setupComponent,flush} from '../test/setup-component';
vi.mock('../api/client',async original=>({...await original<typeof import('../api/client')>(),request:vi.fn()}));
it('page refresh preserves unsaved proxy credentials and routing choices for comparison',async()=>{vi.mocked(request).mockResolvedValue({url:'https://saved.invalid',tmdb:true,sites:false,revision:1});const view=setupComponent(Proxy);await flush();view.state.input='https://typed.invalid';view.state.tmdb=false;vi.mocked(request).mockResolvedValue({url:'https://changed.invalid',tmdb:true,sites:true,revision:2});await view.state.load(true);expect(view.state.input).toBe('https://typed.invalid');expect(view.state.tmdb).toBe(false);expect(view.state.conflict.revision).toBe(2);expect(view.state.showSaved).toBe(false);view.unmount()});
