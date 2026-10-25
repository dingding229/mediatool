// F6 removes only the homepage dashboard aggregator; per-instance dashboards reuse PluginSurface.
import {expect,it} from 'vitest'
import {readFileSync} from 'node:fs'
import {resolve} from 'node:path'
import {createHash} from 'node:crypto'
// F5b changes only workflow list URL page state and shared pagination props.
// T9 adds only the explicit CronInput mapping and bounded form-validity propagation; runtime/trust/operation helpers stay pinned.
// T5 changes only VNumberInput presentation: controls=false; ranges and runtime isolation stay pinned.
// F5a approves static forms, full-range numeric controls and simplified metadata.
// Untouched trust/ESM/operation helpers and the shared workflow manager remain pinned.
// B5 allows only explicit component CSS imports outside the runtime hash;
// all executable plugin code still matches the unchanged baseline byte-for-byte.
// Browser E2E uses real CSS; the separate style guard requires those imports.
// B4 renames the read-state callback from fetch to read; transport remains unchanged.
// B3 explicitly adds fixed access guidance; arbitrary provider text remains excluded.
const baseline={
  "PluginMetadata.vue": "122730e4c54c9b2b76b2246841912fe0e08a1f75c59cde36f0058a5cab0496a2",
  "operation-error.ts": "e9420bd2615fa8326d33a0b558fd8a90c538f788fb45b648de2e34665464d6dd",
  "PluginJson.ts": "5d3388aa60d750e4f71e34ea9f7accf081f585efd4d725831d1355357f294efb",
  "PluginSurface.vue": "b4abeb837918d1e8257cce61cd898a8137933f454c4d087da58672866bda0be3",
  "ui-runtime.ts": "3a5fc7b65458ca877eacdc862f27251c2ba7bc61a99830c12b1f8f4d68b1adb2",
  "install-intent.ts": "fa056b05a6532fa495fa8cad9679d447c2738ca4a163ad73e12badf8f17facc2",
  "ReadFeedback.vue": "d0126ad4884bc3e63bbbf8087949c7c45e557efc4cc7398838e8abc41f384d13",
  "PluginAutomation.vue": "cc29b80a6c3e9932b1326f5d09d3609cfa007b27ee6c635099866e0115585467",
  "read-state.ts": "a1511228125ef4f2336b760f18fa336c32a8fc48169da58277d2ebab44fe5067",
  "PluginForm.vue": "3b9a6c79b6a134b01fc5b20bbb4be95f1a5ff9a9b3a0496c7995666df519397f",
  "build-guard.ts": "69e99482dec6ff8726b58a8829856fb8104626db71d86a560a276f0d55fb34e9",
  "esm.ts": "307cc67d6b2298a199668d90fbe0c052c6380d6846058fb56386815af1c97b6e"
}
const scriptBaseline:Record<string,string> = {"PluginAutomation.vue": "cc29b80a6c3e9932b1326f5d09d3609cfa007b27ee6c635099866e0115585467",};
it('preserves untouched plugin security/runtime helpers and pins approved form presentation',()=>{for(const [name,hash] of Object.entries(baseline)){const raw=readFileSync(resolve(__dirname,'../plugins',name),'utf8').replace(/^import 'element-plus\/es\/components\/[a-z-]+\/style\/css'\n/gm,'');const expected=scriptBaseline[name];expect(createHash('sha256').update(expected?raw.match(/<script[^>]*>([\s\S]*?)<\/script>/)![1]:raw).digest('hex'),name).toBe(expected||hash)}})
