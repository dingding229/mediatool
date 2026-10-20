import {expect,it} from 'vitest'
import {readFileSync} from 'node:fs'
import {resolve} from 'node:path'
import {createHash} from 'node:crypto'
const baseline={
  "PluginMetadata.vue": "8808b39eecb2ed02eaed906877c38da79e147e39415329048ffee52fb76f6abe",
  "operation-error.ts": "018bc358abf257b49a0e89009e413d8c24ae7a396849fe6aa5545c5f465062fc",
  "PluginAuth.vue": "10d93e8b37ca1d52f3738d0ae489bf7f58e2b24c5080456151792d03359ef124",
  "PluginJson.ts": "a6a24b2335b501ebe9e6866f218da5115298cbf2281d2d632943893327d7d1e7",
  "PluginSurface.vue": "ca0efd7a95b8d1440dc85038332bbdb938141808309f3b893295d03d74f152ec",
  "ui-runtime.ts": "3a5fc7b65458ca877eacdc862f27251c2ba7bc61a99830c12b1f8f4d68b1adb2",
  "install-intent.ts": "fa056b05a6532fa495fa8cad9679d447c2738ca4a163ad73e12badf8f17facc2",
  "ReadFeedback.vue": "887a2a1c450442fa2b8ba4c1ffc39f1ed9f195367673beb49e3b1d86a2a28995",
  "PluginAutomation.vue": "7fac2321f8f4858459b31d5967efc242aa911f2591793ae7fe83bc0c6c1cde41",
  "PluginSnapshots.vue": "c0e9f0007ede42e7ea5fe975d246ac1ef833c94273445f17abb0d012431e24d6",
  "read-state.ts": "a0012acaaa43b53ce7b501570bb5c56c98633d9ae29bc12aa8213f81aa8e4bbe",
  "PluginTools.vue": "5e84eca57b155f4b12e8c14c679b57c70e6a7f31917a90000c080f3c0eb94fed",
  "PluginForm.vue": "5e94c38a89011ac58f875abeee718a3b0741602b9568d74b181d1760cfa00667",
  "PluginDashboards.vue": "eb4366cd83ac074bd391252e15b65da572278a320e25013826f3c463046dba48",
  "build-guard.ts": "69e99482dec6ff8726b58a8829856fb8104626db71d86a560a276f0d55fb34e9",
  "esm.ts": "307cc67d6b2298a199668d90fbe0c052c6380d6846058fb56386815af1c97b6e"
}
it('preserves the complete existing plugin runtime and security helpers byte for byte',()=>{for(const [name,hash] of Object.entries(baseline))expect(createHash('sha256').update(readFileSync(resolve(__dirname,'../plugins',name))).digest('hex'),name).toBe(hash)})
