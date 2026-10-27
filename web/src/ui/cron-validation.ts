import {computed,onBeforeUnmount,ref,watch} from 'vue';
import {previewCron,type CronPreview,type CronPurpose} from '../api/scheduling';
import {errorText} from '../workflows/readiness';
// The same authoritative validation drives the saved field and the isolated dialog draft.
// No client-side policy map and no rendering/validation dependency on the future-time list.
export function useCronValidation(source:()=>string,purpose:()=>CronPurpose,localError:()=>string=()=>'',active:()=>boolean=()=>true){
 const accepted=ref(false),checkedText=ref(''),checkedPurpose=ref<CronPurpose>(purpose()),pending=ref(false),error=ref(''),preview=ref<CronPreview|null>(null);
 const valid=computed(()=>accepted.value&&active()&&!localError()&&checkedText.value===source().trim()&&checkedPurpose.value===purpose());
 let timer:ReturnType<typeof setTimeout>|undefined,controller:AbortController|undefined,sequence=0;
 function cancel(){sequence++;clearTimeout(timer);controller?.abort()}
 watch(()=>[source(),purpose(),localError(),active()] as const,()=>{
  cancel();const current=sequence;accepted.value=false;pending.value=false;error.value='';preview.value=null;
  if(!active())return;
  const text=source().trim(),kind=purpose();
  error.value=localError()||(new TextEncoder().encode(text).length>128||![5,6,7].includes(text.split(/\s+/).length)?'请输入最多128字节的5、6或7字段 Cron 表达式':'');
  if(error.value)return;
  pending.value=true;
  timer=setTimeout(async()=>{
   controller=new AbortController();
   try{
    const value=await previewCron(text,kind,controller.signal);if(current!==sequence)return;
    if(value.timezone!=='Asia/Shanghai'||![5,6,7].includes(value.fieldCount)||typeof value.exhausted!=='boolean'||!Number.isFinite(value.minimumIntervalSeconds)||value.minimumIntervalSeconds<0){error.value='计划校验回执无效，请重试';return}
    checkedText.value=text;checkedPurpose.value=kind;preview.value=value;accepted.value=!value.exhausted;
   }catch(e){if(current===sequence)error.value=errorText(e)}
   finally{if(current===sequence)pending.value=false}
  },250);
 },{immediate:true});
 onBeforeUnmount(cancel);
 return {valid,pending,error,preview};
}
