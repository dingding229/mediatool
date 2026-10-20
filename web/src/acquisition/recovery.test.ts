import {afterEach,expect,it,vi} from 'vitest'
import SourceRecovery from './SourceRecoveryDialog.vue'
import OutputRecovery from '../workflows/OutputRecoveryDialog.vue'
import {request} from '../api/client'
import {setupComponent,deferred} from '../test/setup-component'
vi.mock('../api/client',async original=>({...await original<typeof import('../api/client')>(),request:vi.fn(),post:vi.fn()}))
let view:ReturnType<typeof setupComponent>|undefined
afterEach(()=>{view?.unmount();view=undefined;vi.mocked(request).mockReset()})
it.each([{name:'source',component:SourceRecovery},{name:'output',component:OutputRecovery}])('$name preview cannot cross-bind after another task opens',async({component})=>{
 const old=deferred();vi.mocked(request).mockReturnValueOnce(old.promise).mockResolvedValueOnce({marker:'current',controlRevision:2,planDigest:'current'});view=setupComponent(component);const first=view.state.open('previous-task');await view.state.open('current-task');old.resolve({marker:'stale',controlRevision:1,planDigest:'old'});await first;expect(view.state.preview.marker).toBe('current')
})
it.each([{name:'source',component:SourceRecovery},{name:'output',component:OutputRecovery}])('$name ignores responses after leaving the page',async({component})=>{
 const old=deferred();vi.mocked(request).mockReturnValueOnce(old.promise);view=setupComponent(component);const first=view.state.open('owned-task');view.unmount();old.resolve({marker:'late',controlRevision:2,planDigest:'late'});await first;expect(view.state.preview).toBeNull();view=undefined
})
