import {afterEach,expect,it,vi} from 'vitest';
import {defineComponent,getCurrentInstance,ref} from 'vue';
import {setupComponent,flush} from '../test/setup-component';
import {usePageNumber,usePageSize,validPage} from './page-query';
it.each([undefined,'',0,-1,1.2,'bad',20001])('invalid URL page %s safely starts at 1',v=>expect(validPage(v)).toBe(1));
it('reads URL before the first load, preserves filters and handles back navigation',async()=>{
 vi.useFakeTimers();const route=ref({query:{tab:'jobs',jobsPage:'3',q:'keep'}});const replace=vi.fn(async({query})=>{route.value={query}});
 const v=setupComponent(defineComponent({setup(){getCurrentInstance()!.appContext.config.globalProperties.$router={currentRoute:route,replace} as any;return {page:usePageNumber()}}}));
 expect(v.state.page).toBe(3);v.state.page=4;await flush();await vi.advanceTimersByTimeAsync(1);expect(route.value.query).toMatchObject({q:'keep',jobsPage:'4'});
 route.value={query:{tab:'jobs',jobsPage:'2',q:'keep'}};await flush();expect(v.state.page).toBe(2);v.unmount();vi.useRealTimers();
});
afterEach(()=>vi.useRealTimers());

it('keeps sizes independent, gives URL priority and merges resize page and size writes',async()=>{
 vi.useFakeTimers();const route=ref({query:{tab:'users',usersPage:'4',usersPageSize:'50',visitPage:'3',visitPageSize:'100',q:'keep'}});
 const replace=vi.fn(async({query})=>{route.value={query}});
 const v=setupComponent(defineComponent({setup(){getCurrentInstance()!.appContext.config.globalProperties.$router={currentRoute:route,replace} as any;const page=usePageNumber();return {page,size:usePageSize(25),visits:usePageSize(25,'visitPage')}}}));
 expect(v.state.size).toBe(50);expect(v.state.visits).toBe(100);
 v.state.size=10;v.state.page=1;await flush();await vi.advanceTimersByTimeAsync(1);
 expect(replace).toHaveBeenCalledTimes(1);expect(route.value.query).toMatchObject({usersPage:'1',usersPageSize:'10',visitPageSize:'100',q:'keep'});
 route.value={query:{...route.value.query,usersPageSize:'invalid'}};await flush();expect(v.state.size).toBe(25);v.unmount();
});
it('retains nonstandard original defaults and page-specific options',()=>{
 for(const initial of [8,20,24,30,200]){const v=setupComponent(defineComponent({setup:()=>({size:usePageSize(initial)})}));expect(v.state.size).toBe(initial);v.unmount()}
});
it('does not write a departing list query onto another page after unmount',async()=>{
 vi.useFakeTimers();const route=ref({query:{tab:'list'}}),replace=vi.fn();const v=setupComponent(defineComponent({setup(){getCurrentInstance()!.appContext.config.globalProperties.$router={currentRoute:route,replace} as any;return {page:usePageNumber(),size:usePageSize(25)}}}));v.state.page=2;v.state.size=10;v.unmount();await vi.advanceTimersByTimeAsync(1);expect(replace).not.toHaveBeenCalled();
});
