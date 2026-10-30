import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {createSSRApp} from 'vue';import {renderToString} from 'vue/server-renderer';
import {setupComponent,flush} from '../test/setup-component';
import MiddleEllipsis from './MiddleEllipsis.vue';
import {middleText,displayPath} from './middle-ellipsis';
vi.mock('element-plus',async()=>{const {h}=await import('vue');return {ElTooltip:{props:['content'],setup:(p:any,c:any)=>()=>h('div',{'data-tooltip':p.content},c.slots.default?.())},ElMessage:{success:vi.fn(),warning:vi.fn()}}});
const pixels=(s:string)=>Array.from(s).reduce((n,c)=>n+(/[\u3400-\u9fff]/u.test(c)?14:7),0);
const long='Where.the.Mask.Ends.嫁金钗.2026.S01E01.2160p.WEB-DL.H.265.DDP5.1.Atmos-HHWEB.mkv';
let resize:()=>void,disconnect:ReturnType<typeof vi.fn>;
beforeEach(()=>{
 disconnect=vi.fn();vi.stubGlobal('ResizeObserver',class{constructor(callback:()=>void){resize=callback}observe=vi.fn();disconnect=disconnect;});
 vi.stubGlobal('document',{createElement:()=>({getContext:()=>({font:'',measureText:(text:string)=>({width:pixels(text)})})}),fonts:{ready:Promise.resolve()}});vi.stubGlobal('getComputedStyle',()=>({font:'14px sans-serif'}));
 vi.stubGlobal('navigator',{clipboard:{writeText:vi.fn(async()=>{})}});
});
afterEach(()=>vi.unstubAllGlobals());
it('leaves short names unchanged',()=>expect(middleText('短名.mkv',200,pixels)).toBe('短名.mkv'));
it.each([long,'嫁金钗'.repeat(20)+'.mkv'])('measures long mixed/Chinese names and retains the extension: %s',text=>{
 const shown=middleText(text,210,pixels);expect(shown).toContain('…');expect(shown.endsWith('.mkv')).toBe(true);expect(shown[0]).toBe(text[0]);expect(pixels(shown)).toBeLessThanOrEqual(210);
});
it('handles names without extensions and different glyph widths',()=>{
 for(const text of ['没有扩展名的中文文件名'.repeat(10),'LongUnbrokenReleaseName'.repeat(10)]){const shown=middleText(text,140,pixels);expect(shown).toContain('…');expect(pixels(shown)).toBeLessThanOrEqual(140)}
 expect(middleText('中文'.repeat(20),140,pixels).length).toBeLessThan(middleText('en'.repeat(20),140,pixels).length);
});
it('folds preceding levels but preserves the final directory and file suffix',()=>{
 const shown=displayPath('/115/媒体/电视剧/2026/嫁金钗/'+long,420,pixels);expect(shown).toMatch(/^…\/嫁金钗\//);expect(shown.endsWith('.mkv')).toBe(true);expect(pixels(shown)).toBeLessThanOrEqual(420);
});
it('remeasures on ResizeObserver changes and disconnects when unmounted',async()=>{
 const v=setupComponent(MiddleEllipsis,{text:long});v.state.host={clientWidth:350};resize();const wide=v.state.shown;v.state.host.clientWidth=140;resize();expect(v.state.shown).not.toBe(wide);expect(pixels(v.state.shown)).toBeLessThanOrEqual(140);expect(v.state.shown.endsWith('.mkv')).toBe(true);v.unmount();expect(disconnect).toHaveBeenCalledTimes(1);await flush();
});
it('tooltip and accessible label contain the complete original, and copy writes the original',async()=>{
 const html=await renderToString(createSSRApp(MiddleEllipsis,{text:long}));expect(html).toContain('data-tooltip="'+long+'"');expect(html).toContain('aria-label="'+long+'"');
 const v=setupComponent(MiddleEllipsis,{text:long});v.state.host={clientWidth:140};resize();await v.state.copyFull();expect(navigator.clipboard.writeText).toHaveBeenCalledExactlyOnceWith(long);expect(v.state.notice).toBe('已复制');v.unmount();
});
it('full mode never shortens a deep path',()=>{const path='/115/媒体/电视剧/2026/目录/'+long;const v=setupComponent(MiddleEllipsis,{path,full:true});v.state.host={clientWidth:60};resize();expect(v.state.shown).toBe(path);v.unmount();});
it('remeasures a selected/bold ancestor style even if container width stays unchanged',()=>{
 let changed:()=>void,weight=1;const release=vi.fn();
 vi.stubGlobal('MutationObserver',class{constructor(callback:()=>void){changed=callback}observe=vi.fn();disconnect=release;});
 vi.stubGlobal('document',{createElement:()=>({getContext:()=>({font:'',measureText:(text:string)=>({width:pixels(text)*weight})})}),fonts:{ready:Promise.resolve()}});
 const v=setupComponent(MiddleEllipsis,{text:long});v.state.host={clientWidth:180};resize();const before=v.state.shown;weight=1.2;changed!();expect(v.state.shown).not.toBe(before);expect(pixels(v.state.shown)*weight).toBeLessThanOrEqual(180);expect(v.state.shown.endsWith('.mkv')).toBe(true);v.unmount();expect(release).toHaveBeenCalledTimes(1);
});
it.each([160,240])('retains distinct episode and year markers for eight releases at %ipx',width=>{
 const shown=Array.from({length:8},(_,i)=>middleText(long.replace('S01E01','S01E'+String(i+1).padStart(2,'0')),width,pixels));
 expect(new Set(shown).size).toBe(8);shown.forEach((value,i)=>{expect(value).toContain('S01E'+String(i+1).padStart(2,'0'));expect(value).toContain('2026');expect(value).toMatch(/\.mkv$/);expect(pixels(value)).toBeLessThanOrEqual(width)});
});
it.each(['S01E03-E05','S02E03-05','S03','E04','第十二集','第2季','1x03'])('preserves complete identity marker %s',marker=>{
 const shown=middleText('超长作品Long.Release.Name.2026.'+marker+'.2160p.WEB-DL.DDP-HHWEB.mkv',190,pixels);expect(shown).toContain(marker);expect(shown).toContain('2026');expect(shown).toMatch(/\.mkv$/);expect(pixels(shown)).toBeLessThanOrEqual(190);
});
it('gives the episode and extension priority over a year or directory in a very narrow column',()=>{
 const shown=middleText(long,80,pixels);expect(shown).toContain('S01E01');expect(shown).toMatch(/\.mkv$/);expect(pixels(shown)).toBeLessThanOrEqual(80);
 const path=displayPath('/115/很长的父目录名称/多级目录/'+long,160,pixels);expect(path).toContain('S01E01');expect(path).toMatch(/\.mkv$/);expect(pixels(path)).toBeLessThanOrEqual(160);
});
