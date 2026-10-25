import {expect,it} from 'vitest';
import {createSSRApp} from 'vue';
import {renderToString} from 'vue/server-renderer';
import {readFileSync} from 'node:fs';
import PathText from './PathText.vue';
import {splitDisplayPath} from './path-text';
it.each(['','/电影/影片.mkv','/115/媒体/🎬/长路径/'.repeat(30)+'最后的影片.mkv'])('keeps Unicode path ends without changing the original: %s',path=>{const [first,last]=splitDisplayPath(path);expect(first+last).toBe(path);expect(Array.from(first).length-Array.from(last).length).toBeLessThanOrEqual(1)});
it('provides full title and separately hidden accessible text without rendering active HTML',async()=>{const html=await renderToString(createSSRApp(PathText,{path:'/115/<img src=x onerror=bad()>.mkv'}));expect(html).toContain('title="/115/&lt;img src=x onerror=bad()&gt;.mkv"');expect(html).not.toContain('<img');expect(html).toContain('mt-path-full');const css=readFileSync(new URL('./PathText.vue',import.meta.url),'utf8');expect(css).toContain('.mt-path-full{position:absolute;width:1px;height:1px;');expect(css).toContain('text-overflow:ellipsis');expect(css).toContain('justify-content:flex-end')});
it('shows unknown instead of inventing a missing path',async()=>{expect(await renderToString(createSSRApp(PathText))).toContain('title="—"')});
