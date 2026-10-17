import {expect,it} from 'vitest'
import {dangerousScope,pluginPin} from './plugins'
import inventory from '../../../internal/modules/plugins/v2/library.json'
it('uses the exact embedded host pin and visibly classifies dangerous scopes',()=>{
 expect(pluginPin).toBe(inventory.pin)
 for(const value of ['system.exec','network','fs.media:write','settings.write','ui.trusted','events.chain.recognize','module.notification','library.delete'])expect(dangerousScope(value),value).toBe(true)
 for(const value of ['plugin.config.read','plugin.data.read','plugin.state.read','ui.json'])expect(dangerousScope(value),value).toBe(false)
})
