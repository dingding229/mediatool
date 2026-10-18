import {expect,it,vi} from 'vitest'
import {dangerousScope,pluginPin} from './plugins'
import inventory from '../../../internal/modules/plugins/v2/library.json'
it('uses the exact embedded host pin and visibly classifies dangerous scopes',()=>{
 expect(pluginPin).toBe(inventory.pin)
 for(const value of ['api.public','api.public.write','system.exec','network','fs.media:write','settings.write','ui.trusted','events.chain.recognize','module.notification','library.delete'])expect(dangerousScope(value),value).toBe(true)
 for(const value of ['plugin.config.read','plugin.data.read','plugin.state.read','ui.json'])expect(dangerousScope(value),value).toBe(false)
})

it('uses the shared Go/Vue UI envelope and bounded declared key endpoint',async()=>{
 const fixture=(await import('../../../internal/platform/httpapi/testdata/contracts/plugin-v2-ui.json')).default
 const {pluginUI,uiSurfaces}=await import('./plugins')
 const fetcher=vi.fn().mockResolvedValue(new Response(JSON.stringify({data:fixture.form}),{status:200}))
 vi.stubGlobal('fetch',fetcher)
 try{expect(await pluginUI('owned-instance','form')).toEqual(fixture.form);expect(fetcher.mock.calls[0][0]).toBe('/api/v1/plugins/owned-instance/ui/form?key=');fetcher.mockResolvedValueOnce(new Response(JSON.stringify({data:[]}),{status:200}));expect(await uiSurfaces()).toEqual([]);expect(fetcher.mock.calls[1][0]).toBe('/api/v1/plugins/ui-surfaces')}finally{vi.unstubAllGlobals()}
})

it('classifies every shared F3 dangerous method permission without frontend scope drift',()=>{
 for(const method of inventory.hostMethods)if(method.level==='D')expect(dangerousScope(method.scope),method.name).toBe(true)
 for(const scope of ['downloads.add','downloads.delete','sites.request','sites.templates.select','subscriptions.write'])expect(dangerousScope(scope),scope).toBe(true)
})
