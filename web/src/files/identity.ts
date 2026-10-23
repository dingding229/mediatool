import {reactive,watch} from 'vue';
import {identifyFile,type CategoryChoice} from '../api/fileIdentity';
import {errorText} from '../workflows/readiness';

export function filePayload(draft:Record<string,any>){
 const input:Record<string,any>={};
 for(const key of ['operation','source','destination','name','template','title','year','mediaType','category','resolution','includeSidecars','tmdbId'])if(draft[key]!==undefined)input[key]=draft[key];
 if(draft.mediaType==='series')for(const key of ['season','episode'])if(draft[key]!==undefined)input[key]=draft[key];
 return input;
}
export function validateFileIdentity(draft:Record<string,any>,directory=false){
 if(!['movie','series'].includes(draft.mediaType))return '请选择媒体类型。';
 if(!String(draft.title||'').trim())return '请填写媒体标题。';
 if(!draft.category)return '请选择目录分类；未识别时仍可手动填写后整理。';
 if(draft.mediaType==='series'&&!directory&&(!Number.isInteger(draft.season)||draft.season<0||draft.season>999||!Number.isInteger(draft.episode)||draft.episode<1||draft.episode>9999))return '请核对并填写实际季号和集号，未知季集不会自动设为 1。';
 return '';
}
export function createFileIdentity(draft:Record<string,any>,name:string){
 let generation=0,dead=false;
 const state=reactive({loading:false,warning:'',query:'',queryType:'auto',matches:[] as any[],selected:(draft.tmdbId>0?{id:draft.tmdbId,title:draft.title,year:draft.year,mediaType:draft.mediaType}:null) as any,categories:{} as Record<string,CategoryChoice[]>,hint:{} as Record<string,any>,
  get choices(){return this.categories[draft.mediaType]||[];},
  async read(options:{query?:string;mediaType?:string;tmdbId?:number}={}){
   const current=++generation;state.loading=true;
   try{
    const out=await identifyFile(name,options);if(dead||current!==generation)return;
    state.categories=out.categories||{};state.matches=out.matches||[];state.warning=out.warning||'';
    if(!options.query){
     state.hint=out.hint||{};
     if(!options.tmdbId)for(const key of ['title','year','mediaType','season','episode','episodes','resolution'])if(out.hint[key]!==undefined)draft[key]=out.hint[key];
     if(out.selected){state.selected=out.selected;Object.assign(draft,{title:out.selected.title,year:out.selected.year,mediaType:out.selected.mediaType,tmdbId:out.selected.id,category:out.classification?.directory||''});}
     else if(!options.tmdbId||!draft.tmdbId){state.selected=null;draft.tmdbId=0;draft.category='';}else{state.warning='TMDB 暂不可用，保留已确认身份；可重试或改为手动填写。';}
     if(draft.mediaType==='movie'){delete draft.season;delete draft.episode;delete draft.episodes;}
     else if(draft.mediaType==='series'&&!draft.episode){draft.episode=undefined;}
    }
   }catch(e){if(!dead&&current===generation)state.warning='未识别，分类需手动选择。'+errorText(e);}
   finally{if(!dead&&current===generation)state.loading=false;}
  },
  recognize(){return state.read(draft.tmdbId>0?{tmdbId:draft.tmdbId,mediaType:draft.mediaType}:{});},
  search(){if(state.query.trim())return state.read({query:state.query.trim(),mediaType:state.queryType});},
  choose(item:any){return state.read({tmdbId:item.id,mediaType:item.mediaType});},
  manual(){generation++;state.loading=false;state.selected=null;state.warning='未识别，分类需手动选择';draft.tmdbId=0;draft.category='';},
  dispose(){dead=true;generation++;stop();}
 });
 const stop=watch(()=>draft.mediaType,(kind,previous)=>{
  if(!state.choices.some((c:CategoryChoice)=>c.directory===draft.category))draft.category='';
  if(kind==='movie'){delete draft.season;delete draft.episode;delete draft.episodes;}
  else if(kind==='series'&&previous!==kind){draft.season=state.hint.mediaType==='series'?state.hint.season:undefined;draft.episode=state.hint.mediaType==='series'&&state.hint.episode>0?state.hint.episode:undefined;}
 });
 return state;
}
