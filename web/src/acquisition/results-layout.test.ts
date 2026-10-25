import {expect,it} from 'vitest';
import {readFileSync} from 'node:fs';
const view=readFileSync(new URL('../views/ResourceResultsView.vue',import.meta.url),'utf8');
const css=readFileSync(new URL('./acquisition.css',import.meta.url),'utf8');
const stability=readFileSync(new URL('../ui/stability.css',import.meta.url),'utf8');
it('status summary opts out of the generic surface minimum without removing the expanded list bound',()=>{expect(stability).toContain('.acquire-surface.f3-site-summary{min-height:0;');expect(stability).toContain('max-height:240px');expect(view).toContain('v-show="sitesOpen"')});
it('header and rows share responsive tracks and account for sort icon space',()=>{expect(view).toContain('f3-stable-scroll resource-results-table');expect(css).toContain('.resource-columns,.resource-entry {display:grid;grid-template-columns:var(--resource-result-columns);gap:var(--resource-result-gap)');expect(css).toContain('--resource-result-columns:repeat(3,minmax(0,1fr))');expect(css).toContain('margin-inline-end:var(--resource-sort-space)');expect(view.match(/class="resource-sort-label"/g)).toHaveLength(3)});
it('seeder cell has no redundant label, preserves zero and unknown values, and keeps accessible sorting',()=>{expect(view).toContain('<span class="resource-stat">{{ item.seeders??\'—\' }}</span>');expect(view).not.toContain('>做种 {{');expect(view).toContain('aria-label="按做种人数排序"');expect(view).toContain('@keydown.space.prevent="sort(\'seeders\')"')});
