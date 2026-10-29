import {it,expect,beforeEach,afterEach,vi} from 'vitest';
import {setupComponent,flush} from '../test/setup-component';
import Idle from './IdleUpdatePanel.vue';
import {getIdleUpdate,saveIdleUpdate,checkIdleUpdate} from '../api/updates';
vi.mock('../api/updates',()=>({getIdleUpdate:vi.fn(),saveIdleUpdate:vi.fn(),checkIdleUpdate:vi.fn()}));
let view:ReturnType<typeof setupComponent>;
const policy={enabled:false,startTime:'02:00',endTime:'05:00',quietMinutes:60,revision:1,reason:''};
beforeEach(()=>{vi.clearAllMocks();vi.mocked(getIdleUpdate).mockResolvedValue({...policy});view=setupComponent(Idle)});
afterEach(()=>view.unmount());
it('validates time and quiet interval boundaries without submitting invalid values',async()=>{await flush();const s=view.state;for(const v of['14','361','1.5','','abc']){s.minutes=v;expect(s.valid).toBe(false);await s.save()}expect(saveIdleUpdate).not.toHaveBeenCalled();s.minutes='15';expect(s.valid).toBe(true);s.minutes='360';expect(s.valid).toBe(true);s.end='02:00';expect(s.valid).toBe(false);s.end='01:00';expect(s.valid).toBe(true)});
it('requires explicit authorization and clears password even when save fails',async()=>{await flush();const s=view.state;s.enabled=true;await s.save();expect(s.open).toBe(true);await s.save();expect(saveIdleUpdate).not.toHaveBeenCalled();s.password='fixture-private';s.confirm=true;vi.mocked(saveIdleUpdate).mockRejectedValue(new Error('密码错误'));await s.save();expect(s.password).toBe('');expect(s.error).toBe('密码错误');expect(saveIdleUpdate).toHaveBeenCalledWith({...policy,enabled:true},'fixture-private',true)});
it('renders the backend condition model and only calls read-only check',async()=>{await flush();const result={ready:false,timezone:'Asia/Shanghai',checkedAt:'2026-10-26T12:00:00Z',conditions:[{key:'quiet',label:'近期播放请求',passed:false,reason:'安静时长不足'}]};vi.mocked(checkIdleUpdate).mockResolvedValue(result);await view.state.evaluate();expect(view.state.check).toEqual(result);expect(saveIdleUpdate).not.toHaveBeenCalled()});
it('can close without a reusable password and preserves optimistic revision',async()=>{await flush();vi.mocked(saveIdleUpdate).mockResolvedValue({...policy,revision:2});await view.state.save();expect(saveIdleUpdate).toHaveBeenCalledWith(policy,'',false);expect(view.state.policy.revision).toBe(2)});

it('keeps HH:mm validation and cross-midnight save values unchanged',async()=>{
 await flush();const s=view.state;
 for(const value of ['','24:00','12:60','9:00','09:0','09:00:00','no-time']){s.start=value;expect(s.valid).toBe(false);await s.save()}
 expect(saveIdleUpdate).not.toHaveBeenCalled();
 s.start='23:30';s.end='05:15';expect(s.valid).toBe(true);await s.save();
 expect(saveIdleUpdate).toHaveBeenCalledWith({...policy,startTime:'23:30',endTime:'05:15'},'',false);
});

it('styles quiet minutes with the same rounded native input contract as the reference interval',async()=>{
 const {readFileSync}=await import('node:fs');
 const idle=readFileSync(new URL('./IdleUpdatePanel.vue',import.meta.url),'utf8');
 const css=readFileSync(new URL('../ui/consistency.css',import.meta.url),'utf8');
 expect(idle).toContain('<input v-model="minutes" class="mt-native-form-input" inputmode="numeric"');
 expect(idle).toContain('.idle-form .settings-field{width:180px;max-width:100%;min-width:0}');
 expect(css).toContain('height:44px;min-height:44px;box-sizing:border-box;padding:var(--mt-field-padding)');
 expect(css).toContain('border-radius:var(--el-border-radius-base)');
});
