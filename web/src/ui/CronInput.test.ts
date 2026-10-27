import {afterEach,expect,it,vi} from 'vitest';
import {createApp,h,nextTick} from 'vue';
import {renderToString} from 'vue/server-renderer';
import CronInput from './CronInput.vue';
import {setupComponent,flush,deferred} from '../test/setup-component';
import {previewCron} from '../api/scheduling';
import {decodeCron,draftError,generateCron} from './cron-generator';
vi.mock('../api/scheduling',()=>({previewCron:vi.fn()}));
const result={timezone:'Asia/Shanghai',serverNow:'2026-10-26T00:00:00+08:00',fieldCount:7,exhausted:false,minimumIntervalSeconds:300,next:['2026-10-26T09:00:00+08:00']};
afterEach(()=>{vi.useRealTimers();vi.clearAllMocks()});
function render(props:Record<string,unknown>){const app=createApp({render:()=>h(CronInput,props as any)});app.component('el-input',{props:['modelValue'],setup:(p:any)=>()=>h('input',{value:p.modelValue})});app.component('el-button',{setup:(_:unknown,c:any)=>()=>h('button',c.attrs,c.slots.default?.())});app.component('el-dialog',{render:()=>null});return renderToString(app)}

it('uses debounced authoritative validation without rendering future execution rows',async()=>{vi.useFakeTimers();vi.mocked(previewCron).mockResolvedValue(result);const v=setupComponent(CronInput,{modelValue:'0 9 * * *',purpose:'subscription'});await vi.advanceTimersByTimeAsync(250);expect(vi.mocked(previewCron)).toHaveBeenCalledWith('0 9 * * *','subscription',expect.any(AbortSignal));expect(v.state.preview.next).toHaveLength(1);expect(v.state.error).toBe('');const html=await render({modelValue:'0 9 * * *',purpose:'generic'});expect(html).not.toContain('接下来');expect(html).not.toContain('cron-preview');v.unmount()});
it('rejects local field count and preserves authoritative error feedback',async()=>{vi.useFakeTimers();const v=setupComponent(CronInput,{modelValue:'* * * *',purpose:'generic'});await vi.advanceTimersByTimeAsync(300);expect(previewCron).not.toHaveBeenCalled();expect(v.state.error).toContain('5、6或7');v.unmount();vi.mocked(previewCron).mockRejectedValue(Error('年字段无效'));const bad=setupComponent(CronInput,{modelValue:'0 0 0 * * * 2200',purpose:'generic'});await vi.advanceTimersByTimeAsync(300);expect(bad.state.error).toContain('年字段无效');bad.unmount()});
it('invalidates and aborts outstanding preview on unmount',async()=>{vi.useFakeTimers();const d=deferred<any>();vi.mocked(previewCron).mockReturnValue(d.promise);const v=setupComponent(CronInput,{modelValue:'0 9 * * *',purpose:'backup'});await vi.advanceTimersByTimeAsync(250);const signal=vi.mocked(previewCron).mock.calls[0]![2]!;v.unmount();expect(signal.aborted).toBe(true);d.resolve(result);await flush()});
it('renders a generate affordance and escapes manual input',async()=>{const html=await render({modelValue:'<script> * * * *',purpose:'generic'});expect(html).not.toContain('<script>');expect(html).toContain('生成');expect(html).toContain('&lt;script&gt;')});
it('preserves exhausted feedback without treating null next as a validation error',async()=>{vi.useFakeTimers();vi.mocked(previewCron).mockResolvedValue({...result,exhausted:true,next:[]});const v=setupComponent(CronInput,{modelValue:'0 0 0 1 1 * 2020',purpose:'backup'});await vi.advanceTimersByTimeAsync(250);expect(v.state.preview.exhausted).toBe(true);expect(v.state.error).toBe('');expect(v.state.valid).toBe(false);v.unmount()});
it('generates all supported modes and reverse-decodes canonical expressions',()=>{expect(generateCron({...decodeCron('*/15 * * * * *'),mode:'seconds',interval:15})).toBe('*/15 * * * * * *');expect(generateCron({...decodeCron('0 30 10 * * * 2030'),mode:'daily',clock:'10:30',year:'2030'})).toBe('0 30 10 * * * 2030');expect(generateCron({...decodeCron('0 */15 * * * * *'),mode:'minutes',interval:15})).toBe('0 */15 * * * * *');expect(generateCron({...decodeCron('0 5 * * * * *'),mode:'hourly',minute:5})).toBe('0 5 * * * * *');expect(generateCron({...decodeCron('0 20 7 * * 1,5 2030'),mode:'weekly',clock:'07:20',weekdays:[5,1],year:'2030'})).toBe('0 20 7 * * 1,5 2030');expect(generateCron({...decodeCron('0 20 7 2,17 * * 2030'),mode:'monthly',clock:'07:20',monthdays:[17,2],year:'2030'})).toBe('0 20 7 2,17 * * 2030');expect(decodeCron('0 9 * * *').mode).toBe('daily');expect(decodeCron('*/15 * * * * *').mode).toBe('seconds');expect(decodeCron('0 0 0 1 1 * 2020').mode).toBe('custom');expect(draftError({...decodeCron('0 9 * * *'),mode:'daily',clock:'25:61'})).toContain('HH:mm')});
it('custom mode keeps arbitrary 5/6/7 field text for authoritative validation',()=>{const d=decodeCron('0 0 0 1 1 * 2199');expect(d.mode).toBe('custom');expect(d.custom).toBe('0 0 0 1 1 * 2199');});

it('isolates generated/custom drafts, cancels without emitting and confirms only after backend validation',async()=>{
 vi.useFakeTimers();vi.mocked(previewCron).mockResolvedValue(result);const update=vi.fn(),v=setupComponent(CronInput,{modelValue:'0 9 * * *',purpose:'subscription','onUpdate:modelValue':update});
 await vi.advanceTimersByTimeAsync(250);v.state.openGenerator();expect(v.state.draft.mode).toBe('daily');v.state.draft.clock='10:35';await nextTick();expect(v.state.expression).toBe('0 35 10 * * * *');expect(v.state.canConfirm).toBe(false);v.state.confirm();expect(update).not.toHaveBeenCalled();
 await vi.advanceTimersByTimeAsync(250);expect(v.state.canConfirm).toBe(true);v.state.generatorOpen=false;await nextTick();expect(update).not.toHaveBeenCalled();
 v.state.openGenerator();v.state.draft.mode='custom';v.state.draft.custom='0 0 0 1 1 * 2199';await nextTick();await vi.advanceTimersByTimeAsync(250);v.state.confirm();expect(update).toHaveBeenCalledExactlyOnceWith('0 0 0 1 1 * 2199');expect(v.state.generatorOpen).toBe(false);v.unmount();
});
it('blocks dialog confirmation on error or exhausted result and ignores stale draft responses',async()=>{
 vi.useFakeTimers();vi.mocked(previewCron).mockResolvedValue(result);const update=vi.fn(),v=setupComponent(CronInput,{modelValue:'0 9 * * *',purpose:'subscription','onUpdate:modelValue':update});await vi.advanceTimersByTimeAsync(250);
 vi.mocked(previewCron).mockRejectedValueOnce(Error('Cron 执行间隔小于此业务允许的最小间隔'));v.state.openGenerator();await nextTick();await vi.advanceTimersByTimeAsync(250);expect(v.state.draftValidation.error).toContain('最小间隔');expect(v.state.canConfirm).toBe(false);
 vi.mocked(previewCron).mockResolvedValueOnce({...result,exhausted:true,next:[]});v.state.draft.year='2020';await nextTick();await vi.advanceTimersByTimeAsync(250);expect(v.state.draftValidation.preview.exhausted).toBe(true);expect(v.state.canConfirm).toBe(false);
 const late=deferred<any>();vi.mocked(previewCron).mockReturnValueOnce(late.promise);v.state.draft.year='2030';await nextTick();await vi.advanceTimersByTimeAsync(250);v.state.draft.mode='custom';v.state.draft.custom='* *';await nextTick();late.resolve(result);await flush();expect(v.state.canConfirm).toBe(false);expect(v.state.draftValidation.error).toContain('5、6或7');expect(update).not.toHaveBeenCalled();v.unmount();
});
it('retains complete generation and reverse rules without normalizing unknown schedules',()=>{
 for(const [expr,mode] of [['0 */15 * * * * *','minutes'],['0 0 */5 * * * 2030-2040/2','hours'],['0 5 * * * * *','hourly'],['0 20 7 * * 1,5 2030','weekly'],['0 20 7 2,17 * * 2030','monthly']] as const){const d=decodeCron(expr);expect(d.mode).toBe(mode);expect(generateCron(d)).toBe(expr)}
 for(const expr of ['0 0 0 1 1 * 2199','0 5-10 * * * * *','0 0 0 1 * 1 *','0 0 0 * * * 2200']){expect(decodeCron(expr).mode).toBe('custom');expect(generateCron(decodeCron(expr))).toBe(expr)}
 expect(draftError({...decodeCron('0 9 * * *'),mode:'weekly',weekdays:[]})).toContain('星期');expect(draftError({...decodeCron('0 9 * * *'),mode:'monthly',monthdays:[]})).toContain('日期');
});

it('unchanged custom confirmation preserves valid and edited drafts cannot reuse an old verdict',async()=>{
 vi.useFakeTimers();vi.mocked(previewCron).mockResolvedValue(result);const validity=vi.fn(),update=vi.fn(),expression='0 0 0 1 1 * 2199',v=setupComponent(CronInput,{modelValue:expression,purpose:'generic',onValid:validity,'onUpdate:modelValue':update});
 await vi.advanceTimersByTimeAsync(250);v.state.openGenerator();await nextTick();await vi.advanceTimersByTimeAsync(250);expect(v.state.canConfirm).toBe(true);v.state.confirm();expect(update).toHaveBeenLastCalledWith(expression);expect(validity).toHaveBeenLastCalledWith(true);
 v.state.openGenerator();await nextTick();await vi.advanceTimersByTimeAsync(250);v.state.draft.custom='0 0 0 1 1 * 2035';expect(v.state.canConfirm).toBe(false);v.state.confirm();expect(update).toHaveBeenCalledTimes(1);v.unmount();
});
