import {parse} from '@vue/compiler-sfc';
import {parse as parseTemplate,NodeTypes,type ElementNode,type RootNode,type TemplateChildNode} from '@vue/compiler-dom';
// Parse templates rather than matching comments, quoted strings or tag line breaks.
export function numberControlsViolations(source:string):number[]{
 const template=parse(source).descriptor.template;if(!template)return [];
 const failures:number[]=[];
 const visit=(node:RootNode|TemplateChildNode)=>{
  if(node.type===NodeTypes.ELEMENT){const el=node as ElementNode;
   if(el.tag.replaceAll('-','').toLowerCase()==='elinputnumber'){
    let controls=-1;el.props.forEach((p,i)=>{if(p.type===NodeTypes.DIRECTIVE&&p.name==='bind'&&p.arg?.type===NodeTypes.SIMPLE_EXPRESSION&&p.arg.isStatic&&p.arg.content==='controls'&&p.modifiers.length===0&&p.exp?.type===NodeTypes.SIMPLE_EXPRESSION&&p.exp.content.trim()==='false')controls=i});
    const laterOverride=el.props.some((p,i)=>i>controls&&p.type===NodeTypes.DIRECTIVE&&p.name==='bind'&&(!p.arg||p.arg.type!==NodeTypes.SIMPLE_EXPRESSION||!p.arg.isStatic||p.arg.content==='controls'));
    if(controls<0||laterOverride)failures.push(template.loc.start.line+el.loc.start.line-1);
   }
  }
  if(node.type===NodeTypes.ROOT||node.type===NodeTypes.ELEMENT)for(const child of node.children)visit(child);
 };
 visit(parseTemplate(template.content,{onError:error=>failures.push(template.loc.start.line+(error.loc?.start.line??1)-1)}));return failures;
}
