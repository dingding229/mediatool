import {afterEach,expect,it,vi} from 'vitest';
import {readFileSync,existsSync} from 'node:fs';
import {resolve} from 'node:path';
import inventory from '../../../internal/modules/plugins/v2/library.json';
import {pluginUI} from '../api/plugins';
import Dashboards from './InstanceDashboards.vue';
import {setupComponent,flush,deferred} from '../test/setup-component';
vi.mock('../api/plugins',()=>({pluginUI:vi.fn()}));
let views:ReturnType<typeof setupComponent>[]=[];
afterEach(()=>{views.forEach(v=>v.unmount());views=[];vi.mocked(pluginUI).mockReset();});
it('home never renders any plugin dashboard even when the pinned manifest declares overview',()=>{
 const plugin=inventory.manifests.find(p=>p.id==='torrent-assistant')!;expect(plugin.extensions.dashboards).toContain('overview');const source=readFileSync(resolve(__dirname,'../views/DashboardWorkspaceView.vue'),'utf8');expect(source).not.toMatch(/PluginDashboards|PluginSurface|plugin-dashboards|dashboard-plugin-container/);expect(existsSync(resolve(__dirname,'../plugins/PluginDashboards.vue'))).toBe(false);expect(readFileSync(resolve(__dirname,'InstanceDetail.vue'),'utf8')).toContain('<InstanceDashboards');
});
it('stopped plugin overview never queries the worker or starts the instance',async()=>{const view=setupComponent(Dashboards,{instanceId:'owned',pluginId:'torrent-assistant',running:false});views.push(view);await flush();expect(pluginUI).not.toHaveBeenCalled();expect(view.state.cards).toEqual([]);});
it('detail loads only its own instance and only declared dashboard keys',async()=>{vi.mocked(pluginUI).mockResolvedValue([{key:'overview',title:'下载器助手'},{key:'not-declared',title:'hidden'}] as any);const view=setupComponent(Dashboards,{instanceId:'owned',pluginId:'torrent-assistant',running:true});views.push(view);await flush();expect(pluginUI).toHaveBeenCalledExactlyOnceWith('owned','meta');expect(view.state.cards).toHaveLength(1);expect(view.state.cards[0].key).toBe('overview');});
it('unmount ignores stale dashboard replies',async()=>{const read=deferred();vi.mocked(pluginUI).mockReturnValueOnce(read.promise);const view=setupComponent(Dashboards,{instanceId:'owned',pluginId:'torrent-assistant',running:true});views.push(view);view.unmount();views=[];read.resolve([{key:'overview',title:'late'}]);await flush();expect(view.state.cards).toEqual([]);});
