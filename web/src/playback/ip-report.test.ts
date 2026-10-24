import {expect,it} from 'vitest';
import IPStatisticsReport from './IPStatisticsReport.vue';
import {setupComponent} from '../test/setup-component';
it('ranks province and city independently with deterministic empty defaults',()=>{
 const view=setupComponent(IPStatisticsReport,{rows:[],summary:{uniqueIPs:2,regions:2,activeUsers:1,provinces:[{name:'浙江省',count:12}],cities:[{name:'杭州市',count:12}],operators:[{name:'阿里',count:12}]}});
 expect(view.state.regions).toEqual([{name:'浙江省',count:12}]);view.state.rank='city';expect(view.state.regions).toEqual([{name:'杭州市',count:12}]);expect(view.state.width({name:'x',count:6},[{name:'max',count:12}])).toBe('50%');view.unmount();
 const empty=setupComponent(IPStatisticsReport,{rows:[]});expect(empty.state.regions).toEqual([]);expect(empty.state.operators).toEqual([]);empty.unmount();
});
