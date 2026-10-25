import CronInput from '../ui/CronInput.vue'
import { defineComponent, h, type PropType, type Component as VueComponent } from 'vue'
import { ElInput,ElInputNumber,ElSwitch,ElCheckbox,ElSelect,ElOption,ElAlert,ElTag,ElButton,ElDivider } from 'element-plus'
import 'element-plus/es/components/input/style/css'
import 'element-plus/es/components/input-number/style/css'
import 'element-plus/es/components/switch/style/css'
import 'element-plus/es/components/checkbox/style/css'
import 'element-plus/es/components/select/style/css'
import 'element-plus/es/components/option/style/css'
import 'element-plus/es/components/alert/style/css'
import 'element-plus/es/components/tag/style/css'
import 'element-plus/es/components/button/style/css'
import 'element-plus/es/components/divider/style/css'
export interface PluginNode { component:string;key?:string;props?:Record<string,unknown>;model?:string;when?:{model:string;equals:unknown};children?:PluginNode[] }
const safeKey=(k:string)=>/^[A-Za-z0-9_-]{1,64}$/.test(k)&&!['__proto__','constructor','prototype'].includes(k)
const text=(v:unknown,max=4096)=>typeof v==='string'?v.slice(0,max):typeof v==='number'?String(v):''
const containers:Record<string,string>={VRow:'plugin-json-row',VCol:'plugin-json-col',VCard:'panel',VCardTitle:'plugin-json-title',VCardText:'plugin-json-text'}
export default defineComponent({name:'PluginJson',props:{nodes:{type:Array as PropType<PluginNode[]>,required:true},model:{type:Object as PropType<Record<string,unknown>>,default:()=>({})},readonly:Boolean},emits:['update:model','cron-valid'],setup(props,{emit}){
 const validity=new Map<string,boolean>();
 const cronFields=(nodes:PluginNode[]):string[]=>nodes.flatMap(n=>[...(n.component==='VCronInput'&&n.model?[n.model]:[]),...cronFields(n.children||[])]);
 return()=>{let count=0
 const render=(n:PluginNode,depth:number,index:number):ReturnType<typeof h>|null=>{
  if(++count>256||depth>12)return h('p',{role:'status'},'插件界面超过安全上限')
  if(n.when&&(!safeKey(n.when.model)||JSON.stringify(props.model[n.when.model])!==JSON.stringify(n.when.equals)))return null
  const p=n.props??{},key=n.key??index,children=()=>n.children?.map((c,i)=>render(c,depth+1,i))??[]
  if(n.component==='VDetails')return h('details',{key},[h('summary',text(p.label,120)||'更多设置'),...children()])
  if(containers[n.component])return h(n.component==='VCardTitle'?'h3':'div',{key,class:containers[n.component]},children())
  if(n.component==='Text')return h('p',{key},text(p.text??p.count))
  if(n.component==='VDivider')return h(ElDivider,{key})
  if(n.component==='VAlert')return h(ElAlert,{key,title:text(p.title??p.text),type:['success','info','warning','error'].includes(String(p.type))?p.type as 'info':'info',closable:false})
  if(n.component==='VChip')return h(ElTag,{key},()=>text(p.text))
  if(n.component==='VBtn')return h(ElButton,{key,disabled:true,title:'动作请使用已授权的手动执行入口'},()=>text(p.text))
  const models:Record<string,VueComponent>={VCronInput:CronInput,VTextField:ElInput,VTextarea:ElInput,VNumberInput:ElInputNumber,VSwitch:ElSwitch,VCheckbox:ElCheckbox,VSelect:ElSelect}
  const component=models[n.component]
  if(!component)return h('p',{key,role:'status',class:'plugin-json-placeholder'},`不支持的组件：${text(n.component,64)}`)
  const field=n.model??'',valid=safeKey(field),label=text(p.title??p.label,120)
  const input:Record<string,unknown>={modelValue:valid?props.model[field]:undefined,disabled:props.readonly||p.disabled===true||!valid,'aria-label':label||field,'onUpdate:modelValue':(v:unknown)=>{if(valid&&!props.readonly&&(typeof v==='string'||typeof v==='boolean'||typeof v==='number'||v===undefined||v===null))emit('update:model',{...props.model,[field]:typeof v==='string'?v:v})}}
  if(n.component==='VCronInput'){input.purpose='plugin';input.label=label||'插件计划';input.onValid=(valid:boolean)=>{validity.set(field,valid);emit('cron-valid',cronFields(props.nodes).every(f=>validity.get(f)===true))}}
  if(n.component==='VSelect')input.emptyValues=[undefined,null]
  if(n.component==='VNumberInput'){input.modelValue=props.model[field]==null?null:Number(props.model[field]);for(const k of ['min','max','step'])if(typeof p[k]==='number'&&Number.isFinite(p[k]))input[k]=p[k];input.stepStrictly=p.stepStrictly===true;input.controls=false}
  if(n.component==='VTextField'||n.component==='VTextarea'){input.type=n.component==='VTextarea'?'textarea':'text';input.placeholder=text(p.placeholder,120);if(Number.isFinite(p.maxlength))input.maxlength=Math.max(1,Number(p.maxlength));input.readonly=p.readonly===true}
  const hint=[text(p.hint,600),n.component==='VNumberInput'&&typeof p.min==='number'&&typeof p.max==='number'?`范围 ${p.min}–${p.max}，调整步长 ${p.step??1}`:''].filter(Boolean).join(' · ')
  return h('div',{key,class:'plugin-json-field'},[h('label',label),h(component,input,n.component==='VSelect'?()=>Array.isArray(p.items)?p.items.slice(0,100).filter(v=>typeof v==='string').map(v=>h(ElOption,{key:v,label:text((p.itemLabels as Record<string,unknown>|undefined)?.[v],120)||v||'使用默认',value:v})):[]:undefined),hint?h('small',{class:'plugin-json-hint'},hint):null])
 };return h('div',{class:'plugin-json'},props.nodes.map((n,i)=>render(n,0,i)))}
}})
