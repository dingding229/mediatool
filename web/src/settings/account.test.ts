import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {request} from '../api/client';
import {setupComponent,flush} from '../test/setup-component';
import AccountPanel from './AccountPanel.vue';
const m=vi.hoisted(()=>({push:vi.fn(),replace:vi.fn(),logout:vi.fn()}));
vi.mock('vue-router',()=>({useRouter:()=>m}));
vi.mock('../stores/auth',()=>({useAuthStore:()=>({logout:m.logout})}));
vi.mock('../api/client',async original=>({...await original<typeof import('../api/client')>(),request:vi.fn()}));
let view:ReturnType<typeof setupComponent>,data:Map<string,string>;
beforeEach(async()=>{data=new Map([['media-tool:setup-guide-hidden:v1','1']]);vi.stubGlobal('localStorage',{getItem:(k:string)=>data.get(k)||null,setItem:(k:string,v:string)=>data.set(k,v)});vi.stubGlobal('window',{dispatchEvent:vi.fn()});vi.clearAllMocks();vi.mocked(request).mockResolvedValue({authenticated:true,account:{username:'owned'}});view=setupComponent(AccountPanel);await flush()});
afterEach(()=>{view.unmount();vi.unstubAllGlobals()});
it('reopens the operations guide and navigates to its live progress card',()=>{expect(view.state.hidden).toBe(true);view.state.reopen();expect(data.get('media-tool:setup-guide-hidden:v1')).toBe('0');expect(m.push).toHaveBeenCalledWith('/ops?tab=attention');expect(window.dispatchEvent).toHaveBeenCalled();expect(view.state.hidden).toBe(false)});
it('still opens the guide when browser preferences are unavailable',()=>{vi.stubGlobal('localStorage',{getItem:()=>{throw Error('disabled')},setItem:()=>{throw Error('disabled')}});expect(()=>view.state.reopen()).not.toThrow();expect(m.push).toHaveBeenCalledWith('/ops?tab=attention')});
it('reads the real current session endpoint without inventing session enumeration',()=>{expect(request).toHaveBeenCalledWith('/auth/session');expect(view.state.session.data.value.account.username).toBe('owned')});

it('the visibility switch is a local preference, not a configuration write',()=>{view.state.setGuide(false);expect(data.get('media-tool:setup-guide-hidden:v1')).toBe('1');expect(view.state.hidden).toBe(true);expect(vi.mocked(request).mock.calls.every(([,o])=>!o?.method)).toBe(true)})
