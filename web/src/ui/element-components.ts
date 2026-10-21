import {defineAsyncComponent,type Component} from 'vue'
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
const lazy=(component:()=>Promise<Component>,style:()=>Promise<unknown>)=>defineAsyncComponent(async()=>{const [value]=await Promise.all([component(),style()]);return value})
export {ElLoading}
// Preserve every global name/prop/event. Only its implementation and CSS move
// behind the first actual render; all workspace routes already lazy-load.
export const elementComponents={ElAlert,ElButton,ElInput,ElSkeleton,ElProgress,ElTag,
 ElTable:lazy(()=>import('element-plus/es/components/table/index').then(m=>m.ElTable),()=>import('element-plus/es/components/table/style/css')),
 ElTableColumn:lazy(()=>import('element-plus/es/components/table/index').then(m=>m.ElTableColumn),()=>import('element-plus/es/components/table-column/style/css')),
 ElDialog:lazy(()=>import('element-plus/es/components/dialog/index').then(m=>m.ElDialog),()=>import('element-plus/es/components/dialog/style/css')),
 ElDrawer:lazy(()=>import('element-plus/es/components/drawer/index').then(m=>m.ElDrawer),()=>import('element-plus/es/components/drawer/style/css')),
 ElEmpty:lazy(()=>import('element-plus/es/components/empty/index').then(m=>m.ElEmpty),()=>import('element-plus/es/components/empty/style/css')),
 ElSelect:lazy(()=>import('element-plus/es/components/select/index').then(m=>m.ElSelect),()=>import('element-plus/es/components/select/style/css')),
 ElOption:lazy(()=>import('element-plus/es/components/select/index').then(m=>m.ElOption),()=>import('element-plus/es/components/option/style/css')),
 ElCheckbox:lazy(()=>import('element-plus/es/components/checkbox/index').then(m=>m.ElCheckbox),()=>import('element-plus/es/components/checkbox/style/css')),
 ElCheckboxGroup:lazy(()=>import('element-plus/es/components/checkbox/index').then(m=>m.ElCheckboxGroup),()=>import('element-plus/es/components/checkbox-group/style/css')),
 ElPagination:lazy(()=>import('element-plus/es/components/pagination/index').then(m=>m.ElPagination),()=>import('element-plus/es/components/pagination/style/css')),
 ElInputNumber:lazy(()=>import('element-plus/es/components/input-number/index').then(m=>m.ElInputNumber),()=>import('element-plus/es/components/input-number/style/css')),
 ElForm:lazy(()=>import('element-plus/es/components/form/index').then(m=>m.ElForm),()=>import('element-plus/es/components/form/style/css')),
 ElFormItem:lazy(()=>import('element-plus/es/components/form/index').then(m=>m.ElFormItem),()=>import('element-plus/es/components/form-item/style/css')),
 ElSwitch:lazy(()=>import('element-plus/es/components/switch/index').then(m=>m.ElSwitch),()=>import('element-plus/es/components/switch/style/css')),
 ElRadio:lazy(()=>import('element-plus/es/components/radio/index').then(m=>m.ElRadio),()=>import('element-plus/es/components/radio/style/css')),
 ElRadioGroup:lazy(()=>import('element-plus/es/components/radio/index').then(m=>m.ElRadioGroup),()=>import('element-plus/es/components/radio-group/style/css')),
 ElRadioButton:lazy(()=>import('element-plus/es/components/radio/index').then(m=>m.ElRadioButton),()=>import('element-plus/es/components/radio-button/style/css')),
}
