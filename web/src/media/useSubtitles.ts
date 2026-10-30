import {computed,onBeforeUnmount,reactive,ref,watch} from 'vue';
import {requestUUID} from '../utils/request-uuid';
import {catalogSubtitles,identifySubtitle,uploadSubtitle,saveSubtitle,deleteSubtitle,type Subtitle,type SubtitleMetadata,type SubtitleCatalog,type LanguageDetection} from '../api/subtitles';
import {useResource} from '../workflows/useResource';
import {useOperation} from '../workflows/useOperation';
import {errorText} from '../workflows/readiness';
import {languageValue,languageChoice} from './subtitle-language';
export function useSubtitles(item:()=>string){
 const asset=ref(''),data=reactive(useResource(()=>catalogSubtitles(item(),asset.value),{selectedAssetId:'',versions:[],groups:[]} as SubtitleCatalog)),op=reactive(useOperation('subtitles'));
 const file=ref<File>(),uploadId=ref(''),encoding=ref('auto'),choice=ref('auto'),detected=ref<LanguageDetection>(),detecting=ref(false),detectionError=ref(''),notice=ref(''),uploadOpen=ref(false),editing=ref<Subtitle>(),remove=ref<Subtitle>(),confirm=ref(false);
 let originalEditLanguage='und';
 const form=reactive<SubtitleMetadata>({language:'und',languageTags:[],title:'',default:false,forced:false,hidden:false,revision:1}),editLanguage=ref('und');
 const rows=computed(()=>data.data.groups.flatMap(group=>group.items));
 const shownGroups=computed(()=>data.data.groups.filter(group=>group.assetId===asset.value&&group.items.some(row=>!row.hidden)));
 const selectedLanguage=computed(()=>choice.value==='auto'?detected.value:languageValue(choice.value));
 let sequence=0,controller:AbortController|undefined;
 async function load(){await data.load();if(data.error)return;if(!asset.value&&!data.data.selectedAssetId&&data.data.versions.length===1){asset.value=data.data.versions[0]!.id;await data.load();if(data.error)return;}if(!asset.value&&data.data.selectedAssetId)asset.value=data.data.selectedAssetId;if(op.pending){let p;try{p=JSON.parse(op.pending.key)}catch{op.clear();return}if(!data.data.groups.some(group=>group.assetId===p.asset))return;const row=rows.value.find(row=>row.id===p.id&&row.assetId===p.asset);if(p.kind==='delete'&&(!row||row.deleting)||p.kind==='upload'&&row||p.kind==='save'&&row&&row.revision!==p.revision)op.clear()}}
 function resetFile(){sequence++;controller?.abort();file.value=undefined;detected.value=undefined;detecting.value=false;detectionError.value='';choice.value='auto';}
 watch(item,()=>{asset.value='';editing.value=undefined;resetFile();notice.value='';void load()},{immediate:true});
 async function selectAsset(value:string){asset.value=value;editing.value=undefined;resetFile();await load()}
 async function identify(){const selected=file.value;if(!selected||op.pending)return;controller?.abort();controller=new AbortController();const n=++sequence;detecting.value=true;detected.value=undefined;detectionError.value='';try{const result=await identifySubtitle(selected,encoding.value,controller.signal);if(n===sequence)detected.value=result}catch(e){if(n===sequence&&(e as Error).name!=='AbortError')detectionError.value=errorText(e)}finally{if(n===sequence)detecting.value=false}}
 function choose(selected:File){resetFile();notice.value='';if(!/\.(srt|ass|ssa|vtt)$/i.test(selected.name)||selected.size>5*1024*1024){notice.value='仅支持 srt/ass/ssa/vtt，最大 5MiB。';return}file.value=selected;uploadId.value=requestUUID();void identify()}
 watch(encoding,()=>void identify());
 function cancelPending(){op.clear();resetFile();notice.value='已核对列表，请重新选择文件或操作。'}
 async function upload(){if(!file.value||!asset.value||op.pending||detecting.value||!detected.value||!selectedLanguage.value)return;const selected=file.value,target=asset.value,id=uploadId.value,metadata=selectedLanguage.value;const out=await op.run(JSON.stringify({kind:'upload',asset:target,id,name:selected.name}),()=>uploadSubtitle(target,selected,encoding.value,id,metadata));if(asset.value!==target)return;if(out){notice.value=`已上传并转为 UTF-8，识别编码：${out.encoding||'UTF-8'}。`;resetFile();uploadOpen.value=false;await load()}else if(op.pending){await load();notice.value='上传结果待核对，请查看字幕列表；不要重复上传。'}}
 function edit(row:Subtitle){editing.value=row;Object.assign(form,row);editLanguage.value=languageChoice(row);originalEditLanguage=editLanguage.value;op.error=null}
 async function save(row=editing.value,value:SubtitleMetadata={...form,...(editLanguage.value===originalEditLanguage?{}:languageValue(editLanguage.value))}){if(!row||op.pending)return;const out=await op.run(JSON.stringify({kind:'save',asset:row.assetId,id:row.id,revision:row.revision}),()=>saveSubtitle(row.assetId,row.id,{...value,revision:row.revision}));await load();if(out)editing.value=undefined}
 async function applyDelete(){const row=remove.value;if(!row||row.source==='embedded')return;if(row.source==='automatic'){await save(row,{...row,hidden:true})}else{await op.run(JSON.stringify({kind:'delete',asset:row.assetId,id:row.id,revision:row.revision}),()=>deleteSubtitle(row.assetId,row.id,row.revision));await load()}confirm.value=false}
 onBeforeUnmount(()=>{sequence++;controller?.abort()});
 return {asset,data,op,file,uploadId,encoding,choice,detected,detecting,detectionError,notice,uploadOpen,editing,form,editLanguage,remove,confirm,rows,shownGroups,selectedLanguage,load,selectAsset,choose,upload,edit,save,applyDelete,cancelPending};
}
