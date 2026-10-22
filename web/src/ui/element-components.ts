import {ElTooltip} from 'element-plus/es/components/tooltip/index'
import 'element-plus/es/components/tooltip/style/css'
import {ElSwitch} from 'element-plus/es/components/switch/index'
import 'element-plus/es/components/switch/style/css'
import {ElEmpty} from 'element-plus/es/components/empty/index'
import 'element-plus/es/components/empty/style/css'
import {ElInputNumber} from 'element-plus/es/components/input-number/index'
import 'element-plus/es/components/input-number/style/css'
import {ElCheckboxGroup} from 'element-plus/es/components/checkbox/index'
import 'element-plus/es/components/checkbox-group/style/css'
import {ElCheckbox} from 'element-plus/es/components/checkbox/index'
import 'element-plus/es/components/checkbox/style/css'
import {ElOption} from 'element-plus/es/components/select/index'
import 'element-plus/es/components/option/style/css'
import {ElSelect} from 'element-plus/es/components/select/index'
import 'element-plus/es/components/select/style/css'
import {defineAsyncComponent,defineComponent,h,type Component} from 'vue'
import ElAlert from 'element-plus/es/components/alert/index'
import ElButton from 'element-plus/es/components/button/index'
import ElInput from 'element-plus/es/components/input/index'
import ElSkeleton from 'element-plus/es/components/skeleton/index'
import ElProgress from 'element-plus/es/components/progress/index'
import ElTag from 'element-plus/es/components/tag/index'
import {ElLoading} from 'element-plus/es/components/loading/index'
import 'element-plus/es/components/alert/style/css'
import 'element-plus/es/components/button/style/css'
import 'element-plus/es/components/input/style/css'
import 'element-plus/es/components/skeleton/style/css'
import 'element-plus/es/components/progress/style/css'
import 'element-plus/es/components/tag/style/css'
import 'element-plus/es/components/loading/style/css'
const controlPlaceholder=defineComponent({setup:()=>()=>h('span',{class:'mt-control-placeholder','aria-hidden':true})})
const lazy=(component:()=>Promise<Component>,style:()=>Promise<unknown>,reserve=false)=>defineAsyncComponent({loader:async()=>{const [value]=await Promise.all([component(),style()]);return value},delay:0,...(reserve?{loadingComponent:controlPlaceholder}:{})})
export {ElLoading}
// Small ubiquitous controls are eager: an empty async component used to grow
// form rows by 44px after paint. Heavy tables/dialogs/date code stays lazy.
export const elementComponents={ElAlert,ElButton,ElInput,ElSkeleton,ElProgress,ElTag,ElSelect,ElOption,ElCheckbox,ElCheckboxGroup,ElInputNumber,ElEmpty,ElSwitch,ElTooltip,
 ElTable:lazy(()=>import('element-plus/es/components/table/index').then(m=>m.ElTable),()=>import('element-plus/es/components/table/style/css')),
 ElTableColumn:lazy(()=>import('element-plus/es/components/table/index').then(m=>m.ElTableColumn),()=>import('element-plus/es/components/table-column/style/css')),
 ElDialog:lazy(()=>import('element-plus/es/components/dialog/index').then(m=>m.ElDialog),()=>import('element-plus/es/components/dialog/style/css')),
 ElDrawer:lazy(()=>import('element-plus/es/components/drawer/index').then(m=>m.ElDrawer),()=>import('element-plus/es/components/drawer/style/css')),
 ElPagination:lazy(()=>import('element-plus/es/components/pagination/index').then(m=>m.ElPagination),()=>import('element-plus/es/components/pagination/style/css')),
 ElForm:lazy(()=>import('element-plus/es/components/form/index').then(m=>m.ElForm),()=>import('element-plus/es/components/form/style/css')),
 ElFormItem:lazy(()=>import('element-plus/es/components/form/index').then(m=>m.ElFormItem),()=>import('element-plus/es/components/form-item/style/css')),
 ElDatePicker:lazy(()=>import('element-plus/es/components/date-picker/index').then(m=>m.ElDatePicker),()=>import('element-plus/es/components/date-picker/style/css'),true),
 ElRadio:lazy(()=>import('element-plus/es/components/radio/index').then(m=>m.ElRadio),()=>import('element-plus/es/components/radio/style/css')),
 ElRadioGroup:lazy(()=>import('element-plus/es/components/radio/index').then(m=>m.ElRadioGroup),()=>import('element-plus/es/components/radio-group/style/css')),
 ElRadioButton:lazy(()=>import('element-plus/es/components/radio/index').then(m=>m.ElRadioButton),()=>import('element-plus/es/components/radio-button/style/css')),
}
