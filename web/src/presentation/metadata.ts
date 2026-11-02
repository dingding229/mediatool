import {toDisplayString} from 'vue';
export const metadataSeparator=' · ';
/** Keep real zeroes, omit unknown fragments and use one delimiter everywhere. The empty separator assembles a labelled fragment without changing Vue interpolation. */
export function joinMetadata(values:readonly unknown[],separator:typeof metadataSeparator|''=metadataSeparator):string{
 if(separator==='')return values.map(toDisplayString).join('');
 return values.map(value=>toDisplayString(value).trim()).filter(value=>value!==''&&value!=='—').join(metadataSeparator);
}
