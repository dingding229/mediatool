import {afterEach,expect,it,vi} from 'vitest';
import {defineComponent,getCurrentInstance,ref} from 'vue';
import {setupComponent,flush} from '../test/setup-component';
import {usePageNumber,validPage} from './page-query';
it.each([undefined,'',0,-1,1.2,'bad',20001])('invalid URL page %s safely starts at 1',v=>expect(validPage(v)).toBe(1));
it('reads URL before the first load, preserves filters and handles back navigation',async()=>{
 vi.useFakeTimers();const route=ref({query:{tab:'jobs',jobsPage:'3',q:'keep'}});const replace=vi.fn(async({query})=>{route.value={query}});
 const v=setupComponent(defineComponent({setup(){getCurrentInstance()!.appContext.config.globalProperties.$router={currentRoute:route,replace} as any;return {page:usePageNumber()}}}));
 expect(v.state.page).toBe(3);v.state.page=4;await flush();await vi.advanceTimersByTimeAsync(1);expect(route.value.query).toMatchObject({q:'keep',jobsPage:'4'});
 route.value={query:{tab:'jobs',jobsPage:'2',q:'keep'}};await flush();expect(v.state.page).toBe(2);v.unmount();vi.useRealTimers();
});
afterEach(()=>vi.useRealTimers());
