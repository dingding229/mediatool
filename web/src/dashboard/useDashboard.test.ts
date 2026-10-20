import { afterEach,beforeEach,expect,it,vi } from 'vitest'
import { defineComponent } from 'vue'
import { setupComponent,flush } from '../test/setup-component'
import { request } from '../api/client'
import { useDashboard } from './useDashboard'
vi.mock('../api/client',async original=>({...await original<typeof import('../api/client')>(),request:vi.fn(),post:vi.fn()}))
const storage=new Map<string,string>();let view:ReturnType<typeof setupComponent>
beforeEach(()=>{storage.clear();vi.stubGlobal('localStorage',{getItem:(k:string)=>storage.get(k),setItem:(k:string,v:string)=>storage.set(k,v)});vi.stubGlobal('window',new EventTarget());vi.mocked(request).mockImplementation(async path=>path==='/readiness'?{components:Object.fromEntries(['open115','clouddrive2','emby'].map(k=>[k,{state:'ready'}]))}:path==='/integrations'?[{kind:'ptsite',enabled:true}]:path==='/classification'?{value:{groups:[{enabled:true,directory:'Movies'}]}}:[]);view=setupComponent(defineComponent({setup:useDashboard}))})
afterEach(()=>{view.unmount();vi.unstubAllGlobals()})
it('can collapse a completed guide and reopen it from settings even with a stored hidden preference',async()=>{await flush();expect(view.state.complete).toBe(true);view.state.hideGuide();expect(view.state.guideVisible).toBe(false);window.dispatchEvent(new Event('media-tool:show-setup'));expect(view.state.guideVisible).toBe(true)})
it('one rejected request never prevents other groups loading',async()=>{vi.mocked(request).mockRejectedValueOnce(Error('jobs unavailable'));await view.state.load('jobs');expect(view.state.slots.jobs.error).toBeTruthy();expect(view.state.slots.accounts.data).not.toBeNull()})
