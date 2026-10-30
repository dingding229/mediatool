export type TextWidth=(text:string)=>number;
// Binary search measured glyph widths, not a character-count display limit.
function plainMiddleText(text:string,width:number,measure:TextWidth,extension=true){
 if(measure(text)<=width)return text;
 const chars=Array.from(text),ext=extension?(text.match(/\.[a-z\d]{1,10}$/i)?.[0]||''):'';
 const minimum=Array.from(ext).length||1;
 const candidate=(count:number)=>{const tail=Math.max(minimum,Math.floor(count*.45)),head=Math.max(1,count-tail);return chars.slice(0,head).join('')+'…'+chars.slice(-tail).join('')};
 if(measure(candidate(minimum+1))>width)return ext||'…';
 let low=minimum+1,high=chars.length-1;
 while(low<high){const mid=Math.ceil((low+high)/2);if(measure(candidate(mid))<=width)low=mid;else high=mid-1;}
 return candidate(low);
}
// Preserve whole release identity markers, rather than losing the only episode distinction.
export function keyRanges(text:string){
 const pattern=/(?<![a-z\d])(?:S\d+E\d+(?:-E?\d+)?|S\d+|E\d+|\d+x\d+|第[零〇一二三四五六七八九十百千万两\d]+[集季]|(?:19|20)\d{2})(?!\d)/gi;
 return [...text.matchAll(pattern)].map(match=>({start:match.index!,end:match.index!+match[0].length,year:/^(19|20)\d{2}$/.test(match[0])}));
}
function keyedCandidate(text:string,ranges:{start:number;end:number}[],ext:string,count:number){
 const chars=Array.from(text),head=chars.slice(0,Math.ceil(count*.55)).join(''),tail=chars.slice(-Math.floor(count*.45)||chars.length).join('');
 const spans=[...ranges,{start:text.length-ext.length,end:text.length}];
 if(head)spans.push({start:0,end:head.length});if(tail)spans.push({start:text.length-tail.length,end:text.length});
 const merged:{start:number;end:number}[]=[];
 for(const span of spans.filter(r=>r.end>r.start).sort((a,b)=>a.start-b.start)){
  const previous=merged.at(-1);if(previous&&(span.start<=previous.end||/^[. _-]$/.test(text.slice(previous.end,span.start))))previous.end=Math.max(previous.end,span.end);else merged.push({...span});
 }
 return merged.map((range,i)=>(i&&range.start>merged[i-1]!.end?'…':'')+text.slice(range.start,range.end)).join('');
}
function minimumName(text:string,extension=true){
 const ext=extension?(text.match(/\.[a-z\d]{1,10}$/i)?.[0]||''):'';
 return keyedCandidate(text,keyRanges(text),ext,0)||ext||'…';
}
export function middleText(text:string,width:number,measure:TextWidth,extension=true){
 if(measure(text)<=width)return text;
 const ranges=keyRanges(text);if(!ranges.length)return plainMiddleText(text,width,measure,extension);
 const ext=extension?(text.match(/\.[a-z\d]{1,10}$/i)?.[0]||''):'';
 // At extremely small widths, season/episode plus extension takes precedence over a year.
 let required=ranges;
 if(measure(keyedCandidate(text,required,ext,0))>width)required=ranges.filter(r=>!r.year);
 if(!required.length)required=ranges;
 let low=0,high=Array.from(text).length-1;
 while(low<high){const mid=Math.ceil((low+high)/2);if(measure(keyedCandidate(text,required,ext,mid))<=width)low=mid;else high=mid-1;}
 return keyedCandidate(text,required,ext,low);
}
export function displayPath(path:string,width:number,measure:TextWidth){
 const parts=path.split('/').filter(Boolean);if(parts.length<3)return middleText(path,width,measure);
 const file=parts.at(-1)!,parent=parts.at(-2)!,prefix='…/';
 const ext=file.match(/\.[a-z\d]{1,10}$/i)?.[0]||Array.from(file).at(-1)!;
 const fileMinimum=keyRanges(file).length?minimumName(file):'…'+ext;
 const room=width-measure(prefix+'/')-measure(fileMinimum);
 if(room<measure('…'))return measure(prefix+fileMinimum)<=width?prefix+middleText(file,width-measure(prefix),measure):middleText(file,width,measure);
 const letters=Array.from(parent),minimum=measure(letters[0]+'…'+letters.at(-1));
 const parentWidth=Math.min(measure(parent),room,Math.max(minimum,width*.32-measure(prefix+'/')));
 let directory=middleText(parent,parentWidth,measure,false);
 if(directory==='…'&&measure(letters[0]+'…')<=parentWidth)directory=letters[0]+'…';
 const start=prefix+directory+'/';
 return start+middleText(file,Math.max(0,width-measure(start)),measure);
}
