import {expect,it,vi} from 'vitest';
import TopProgress from './TopProgress.vue';
import {beginActivity} from '../api/activity';
import {setupComponent,flush} from '../test/setup-component';
it('delays short work, tracks concurrency, and never adds document flow',async()=>{
 vi.useFakeTimers();const v=setupComponent(TopProgress),first=beginActivity(),second=beginActivity();await flush();expect(v.state.visible).toBe(false);await vi.advanceTimersByTimeAsync(120);expect(v.state.visible).toBe(true);first();await flush();await vi.advanceTimersByTimeAsync(200);expect(v.state.visible).toBe(true);second();await flush();await vi.advanceTimersByTimeAsync(180);expect(v.state.visible).toBe(false);v.unmount();vi.useRealTimers();
});
