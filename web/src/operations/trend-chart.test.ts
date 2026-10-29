import {expect,it} from 'vitest';
import {createSSRApp} from 'vue';
import {renderToString} from 'vue/server-renderer';
import {formatTrendValue,trendPlot,trendRange,trendTimeTicks} from './trend-chart';
import {trendSegments} from './model';
import Chart from './TrendChart.vue';
const rows=(values:(number|null)[],start='2026-10-29T00:00:00Z')=>values.map((v,i)=>({at:new Date(Date.parse(start)+i*60000).toISOString(),x:v}));
it.each([[0,'条'],[5,'GiB'],[.23,''],[-3,'秒']])('constant %s/%s has a finite nonzero range, three ticks and is not on canvas edge',(value,unit)=>{
 const p=trendPlot(rows([Number(value),Number(value),Number(value)]),'x',60000,String(unit)),range=trendRange(p.values,String(unit));
 expect(range.min).toBeLessThan(range.max);expect(p.yTicks).toHaveLength(3);expect(p.xTicks).toHaveLength(3);expect(p.segments).toHaveLength(1);
 for(const point of p.segments[0]!.split(' ')){const [x,y]=point.split(',').map(Number);expect(x).toBeGreaterThan(0);expect(x).toBeLessThan(320);expect(y).toBeGreaterThan(0);expect(y).toBeLessThan(105)}
 expect(p.yTicks.every(t=>Number.isFinite(t.value))).toBe(true);
});
it('one sample is one centered isolated point, with an expanded time domain',()=>{
 const p=trendPlot(rows([2]),'x',60000,'秒');expect(p.segments).toEqual([]);expect(p.dots).toHaveLength(1);expect(p.dots[0]!.x).toBeCloseTo(50);expect(p.xTicks).toHaveLength(3);expect(new Set(p.xTicks.map(t=>t.label)).size).toBe(3);
});
it('preserves original missing-value and gap segmentation, never inserts zero samples',()=>{
 const input=rows([1,2,null,3,null,4,5]);input.push({at:'2026-10-29T02:00:00Z',x:6});
 const p=trendPlot(input,'x',60000,'秒'),original=trendSegments(input,'x',60000);
 expect(p.values).toEqual([1,2,3,4,5,6]);expect(p.segments).toHaveLength(original.filter(s=>s.includes(' ')).length);expect(p.dots).toHaveLength(original.filter(s=>!s.includes(' ')).length);expect(p.segments).toHaveLength(2);expect(p.dots).toHaveLength(2);
 expect(p.segments.reduce((n,s)=>n+s.split(' ').length,0)+p.dots.length).toBe(p.values.length);
});
it('uses all timestamped rows for the domain without inventing values at missing edges',()=>{
 const p=trendPlot(rows([null,2,null]),'x',60000,'秒');expect(p.dots).toHaveLength(1);expect(p.dots[0]!.x).toBeCloseTo(50);expect(p.values).toEqual([2]);
});
it('empty, missing, non-finite and invalid-date samples cannot create NaN points',()=>{
 const p=trendPlot([{at:'bad',x:3},...rows([null,NaN,Infinity])],'x',60000,'');expect(p.values).toEqual([]);expect(p.dots).toEqual([]);expect(p.yTicks).toEqual([]);
});
it('same-day ticks show time and cross-day ticks show date and time in Beijing',()=>{
 expect(trendTimeTicks(Date.parse('2026-10-28T16:00:00Z'),Date.parse('2026-10-28T18:00:00Z')).map(t=>t.label)).toEqual(['00:00','01:00','02:00']);
 const cross=trendTimeTicks(Date.parse('2026-10-28T15:00:00Z'),Date.parse('2026-10-28T17:00:00Z'));
 expect(cross.map(t=>t.label)).toEqual(['10-28 23:00','10-29 00:00','10-29 01:00']);
});
it.each([[5.123,'%','5.1%'],[25.382,'GiB','25.38 GiB'],[65.9,'MiB','66 MiB'],[.234,'秒','0.23 秒'],[3.7,'条','4 条'],[.234,'','0.23']])('formats %s %s',(value,unit,expected)=>expect(formatTrendValue(Number(value),String(unit))).toBe(expected));
it('unknown values remain unknown',()=>{expect(formatTrendValue(undefined,'GiB')).toBe('—');expect(formatTrendValue(NaN,'')).toBe('—')});
it('stretch draws HTML dots, real grid lines and multi-point polylines, never zero-length SVG lines',async()=>{
 const html=await renderToString(createSSRApp(Chart,{title:'每CPU负载',unit:'',rows:rows([1,null,2,3]),metric:'x',gapMs:60000,stretch:true}));
 expect(html).toContain('trend-dot');
 expect((html.match(/class="trend-grid-line"/g)||[])).toHaveLength(3);expect(html).not.toContain('stroke-linecap');expect(html).not.toContain('· —');
});
it('non-stretch playback chart keeps its original coordinates, formatting and SVG-only rendering',async()=>{
 const data=rows([1,null,3]);const html=await renderToString(createSSRApp(Chart,{title:'每日播放',unit:'次',rows:data,metric:'x',gapMs:60000}));
 expect(html).not.toContain('trend-frame');expect(html).not.toContain('trend-dot');expect(html).toContain('preserveAspectRatio="xMidYMid meet"');expect(html).toContain('每日播放');expect(html).toContain('最近 3.00 次');
 for(const point of trendSegments(data,'x',60000))expect(html).toContain(`points="${point}"`);
});
