import type { PluginNode } from './PluginJson'
/** Validate the full interval, never round or narrow it to a list of presets. */
export function validateForm(nodes:PluginNode[],model:Record<string,unknown>):string {
 for(const node of nodes){
  if(node.component==='VNumberInput'&&node.model){const value=model[node.model],n=typeof value==='number'?value:typeof value==='string'&&value.trim()!==''?Number(value):NaN,p=node.props||{};
   if(!Number.isFinite(n)||(typeof p.min==='number'&&n<p.min)||(typeof p.max==='number'&&n>p.max)||(p.stepStrictly===true&&!Number.isInteger(n)))return `${String(p.label||node.model)}：请填写范围内${p.stepStrictly?'的整数':'的数字'}`
  }
  const error=validateForm(node.children||[],model);if(error)return error
 }
 return ''
}
