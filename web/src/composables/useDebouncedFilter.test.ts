import {afterEach,expect,it,vi} from 'vitest';
import {defineComponent,ref,nextTick} from 'vue';
import {setupComponent} from '../test/setup-component';
import {useDebouncedFilter} from './useDebouncedFilter';
afterEach(()=>vi.useRealTimers());
it('coalesces typing at >=300ms, does not read on mount, clears on unmount',async()=>{
 vi.useFakeTimers();const value=ref(''),read=vi.fn();const v=setupComponent(defineComponent({setup(){useDebouncedFilter(value,read,1);return{}}}));
 expect(read).not.toHaveBeenCalled();value.value='a';await nextTick();await vi.advanceTimersByTimeAsync(200);value.value='ab';await nextTick();await vi.advanceTimersByTimeAsync(299);expect(read).not.toHaveBeenCalled();await vi.advanceTimersByTimeAsync(1);expect(read).toHaveBeenCalledExactlyOnceWith('ab');
 value.value='';await nextTick();await vi.advanceTimersByTimeAsync(300);expect(read).toHaveBeenLastCalledWith('');value.value='pending';await nextTick();v.unmount();await vi.advanceTimersByTimeAsync(1000);expect(read).toHaveBeenCalledTimes(2);
});
