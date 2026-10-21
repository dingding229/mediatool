import {expect,it} from 'vitest'
import {readFileSync} from 'node:fs'
import {resolve} from 'node:path'
import {createHash} from 'node:crypto'
// B5 allows only explicit component CSS imports outside the runtime hash;
// all executable plugin code still matches the unchanged baseline byte-for-byte.
// Browser E2E uses real CSS; the separate style guard requires those imports.
// B4 renames the read-state callback from fetch to read; transport remains unchanged.
// B3 explicitly adds fixed access guidance; arbitrary provider text remains excluded.
const baseline={
  "PluginMetadata.vue": "8808b39eecb2ed02eaed906877c38da79e147e39415329048ffee52fb76f6abe",
  "operation-error.ts": "e9420bd2615fa8326d33a0b558fd8a90c538f788fb45b648de2e34665464d6dd",
  "PluginAuth.vue": "10d93e8b37ca1d52f3738d0ae489bf7f58e2b24c5080456151792d03359ef124",
  "PluginJson.ts": "a6a24b2335b501ebe9e6866f218da5115298cbf2281d2d632943893327d7d1e7",
  "PluginSurface.vue": "ca0efd7a95b8d1440dc85038332bbdb938141808309f3b893295d03d74f152ec",
  "ui-runtime.ts": "3a5fc7b65458ca877eacdc862f27251c2ba7bc61a99830c12b1f8f4d68b1adb2",
  "install-intent.ts": "fa056b05a6532fa495fa8cad9679d447c2738ca4a163ad73e12badf8f17facc2",
  "ReadFeedback.vue": "887a2a1c450442fa2b8ba4c1ffc39f1ed9f195367673beb49e3b1d86a2a28995",
  "PluginAutomation.vue": "7fac2321f8f4858459b31d5967efc242aa911f2591793ae7fe83bc0c6c1cde41",
  "PluginSnapshots.vue": "c0e9f0007ede42e7ea5fe975d246ac1ef833c94273445f17abb0d012431e24d6",
  "read-state.ts": "a1511228125ef4f2336b760f18fa336c32a8fc48169da58277d2ebab44fe5067",
  "PluginTools.vue": "5e84eca57b155f4b12e8c14c679b57c70e6a7f31917a90000c080f3c0eb94fed",
  "PluginForm.vue": "5e94c38a89011ac58f875abeee718a3b0741602b9568d74b181d1760cfa00667",
  "PluginDashboards.vue": "eb4366cd83ac074bd391252e15b65da572278a320e25013826f3c463046dba48",
  "build-guard.ts": "69e99482dec6ff8726b58a8829856fb8104626db71d86a560a276f0d55fb34e9",
  "esm.ts": "307cc67d6b2298a199668d90fbe0c052c6380d6846058fb56386815af1c97b6e"
}
const scriptBaseline:Record<string,string> = {"PluginAuth.vue":"a56aceff5f2e1484fe54a6cf3f9f8bff6457cafa075939d827d47f3d7fbe15da","PluginAutomation.vue": "74fdcf114ba6179f95e12e49f759fe7fbb975db41db7d990d8b190af4420942d", "PluginSnapshots.vue": "e7a4dc6338e2c9bc1faa699274c078053ccb7b9a026dab30ddbf6db380a617c4", "PluginTools.vue": "c272ad022bd96f5d6204eee38439b73a846d8382e7c549b86eced79a999de50d"};
it('preserves every plugin runtime script and untouched security helper while allowing copy-only templates',()=>{for(const [name,hash] of Object.entries(baseline)){const raw=readFileSync(resolve(__dirname,'../plugins',name),'utf8').replace(/^import 'element-plus\/es\/components\/[a-z-]+\/style\/css'\n/gm,'');const expected=scriptBaseline[name];expect(createHash('sha256').update(expected?raw.match(/<script[^>]*>([\s\S]*?)<\/script>/)![1]:raw).digest('hex'),name).toBe(expected||hash)}})
