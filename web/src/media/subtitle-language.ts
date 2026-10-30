import type {SubtitleLanguage} from '../api/subtitles';
export const subtitleLanguages=[{value:'auto',label:'自动识别'},{value:'hans',label:'简体中文'},{value:'hant',label:'繁体中文'},{value:'dual',label:'中英双语'},{value:'eng',label:'英语'},{value:'jpn',label:'日语'},{value:'kor',label:'韩语'},{value:'und',label:'其他'}];
// UI choices only. Filename/content recognition is exclusively server-side.
export function languageValue(choice:string):SubtitleLanguage{
 if(['hans','hant','dual'].includes(choice))return {language:'chi',languageTags:choice==='dual'?['zh-Hans','en']:[choice==='hans'?'zh-Hans':'zh-Hant']};
 return {language:choice,languageTags:({eng:['en'],jpn:['ja'],kor:['ko']} as Record<string,string[]>)[choice]||[]};
}
export function languageChoice(value:SubtitleLanguage){const code=value.language.toLowerCase();const tags=value.languageTags||[];if(['chi','zho','zh','chs','cht','zh-cn','zh-tw'].includes(code)){if(tags.includes('en'))return 'dual';if(tags.includes('zh-Hant')||['cht','zh-tw'].includes(code))return 'hant';if(tags.includes('zh-Hans')||['chs','zh-cn'].includes(code))return 'hans';return 'chi'}return ({en:'eng',ja:'jpn',jp:'jpn',ko:'kor'} as Record<string,string>)[code]||code||'und'}
export function subtitleLanguageLabel(value:SubtitleLanguage){const choice=languageChoice(value);return choice==='chi'?'中文（简繁待核对）':choice==='und'?'未知':subtitleLanguages.find(v=>v.value===choice)?.label||'其他语言'}
