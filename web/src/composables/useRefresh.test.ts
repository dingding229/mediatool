import {afterEach,expect,test,vi} from 'vitest'
import {defineComponent} from 'vue'
import {setupComponent,deferred,flush} from '../test/setup-component'
import {useRefresh} from './useRefresh'
import {useResource} from '../workflows/useResource'
let cleanup=()=>{}
afterEach(()=>{cleanup();vi.useRealTimers();vi.unstubAllGlobals()})
test('single flight, completion cadence, hidden pause, disposal and stale data',async()=>{
 vi.useFakeTimers();const events=new EventTarget(),document=Object.assign(events,{hidden:false});vi.stubGlobal('document',document)
 const read=vi.fn().mockResolvedValue({items:[{id:'same'}]})
 const view=setupComponent(defineComponent({setup(){const resource=useResource(read);const loop=useRefresh(resource.load,{interval:15000});return{resource,loop}}}));cleanup=view.unmount;await flush()
 const s=view.state;expect(read).toHaveBeenCalledTimes(1);expect(s.resource.loading.value).toBe(false)
 const pending=deferred();read.mockReturnValueOnce(pending.promise);const a=s.loop.refresh(),b=s.loop.refresh();expect(a).toBe(b);await flush()
 expect(s.resource.loading.value).toBe(false);expect(s.resource.refreshing.value).toBe(true);expect(s.resource.data.value.items[0].id).toBe('same')
 await vi.advanceTimersByTimeAsync(60000);expect(read).toHaveBeenCalledTimes(2)
 pending.resolve({items:[{id:'updated'}]});await a
 document.hidden=true;events.dispatchEvent(new Event('visibilitychange'));await vi.advanceTimersByTimeAsync(60000);expect(read).toHaveBeenCalledTimes(2)
 document.hidden=false;events.dispatchEvent(new Event('visibilitychange'));await flush();expect(read).toHaveBeenCalledTimes(3)
 read.mockRejectedValueOnce(Error('offline'));await s.loop.refresh();expect(s.resource.data.value.items[0].id).toBe('same');expect(s.resource.error.value.message).toBe('offline')
 cleanup();await vi.advanceTimersByTimeAsync(60000);expect(read).toHaveBeenCalledTimes(4)
})
