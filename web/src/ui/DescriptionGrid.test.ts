import {expect,it} from 'vitest';
import {createSSRApp,h} from 'vue';
import {renderToString} from '@vue/server-renderer';
import Grid from './DescriptionGrid.vue';
it('hides absent values but keeps zero, booleans and meaningful text',async()=>{
 const items=[null,undefined,'','  ','—',0,false,'正常'].map((value,i)=>({key:'k'+i,label:'字段'+i,value}));
 const html=await renderToString(createSSRApp(Grid,{items}));
 for(let i=0;i<5;i++)expect(html).not.toContain('字段'+i);
 for(let i=5;i<8;i++)expect(html).toContain('字段'+i);
 expect(html).toContain('>0<');expect(html).toContain('>false<');
});
it('renders named slots inside label/value pairs, omits the grid when all hidden',async()=>{
 const html=await renderToString(createSSRApp({render:()=>h(Grid,{items:[{key:'state',label:'状态',value:'ready'}]}, {state:()=>h('span',{class:'status-badge'},'连接成功')})}));
 expect(html).toMatch(/<dt[^>]*>状态<\/dt>/);expect(html).toContain('status-badge');expect(html).toContain('连接成功');
 expect(await renderToString(createSSRApp(Grid,{items:[{key:'empty',label:'空值',value:null}]}))).not.toContain('<dl');
});
