export type TextWidth=(text:string)=>number;
// Binary search measured glyph widths, not a character-count display limit.
export function middleText(text:string,width:number,measure:TextWidth,extension=true){
 if(measure(text)<=width)return text;
 const chars=Array.from(text),ext=extension?(text.match(/\.[a-z\d]{1,10}$/i)?.[0]||''):'';
 const minimum=Array.from(ext).length||1;
 const candidate=(count:number)=>{const tail=Math.max(minimum,Math.floor(count*.45)),head=Math.max(1,count-tail);return chars.slice(0,head).join('')+'…'+chars.slice(-tail).join('')};
 if(measure(candidate(minimum+1))>width)return ext||'…';
 let low=minimum+1,high=chars.length-1;
 while(low<high){const mid=Math.ceil((low+high)/2);if(measure(candidate(mid))<=width)low=mid;else high=mid-1;}
 return candidate(low);
}
export function displayPath(path:string,width:number,measure:TextWidth){
 const parts=path.split('/').filter(Boolean);if(parts.length<3)return middleText(path,width,measure);
 const file=parts.at(-1)!,parent=parts.at(-2)!,prefix='…/';
 const ext=file.match(/\.[a-z\d]{1,10}$/i)?.[0]||Array.from(file).at(-1)!;
 const room=width-measure(prefix+'/')-measure('…'+ext);
 if(room<measure('…'))return prefix+middleText(file,Math.max(0,width-measure(prefix)),measure);
 const letters=Array.from(parent),minimum=measure(letters[0]+'…'+letters.at(-1));
 const parentWidth=Math.min(measure(parent),room,Math.max(minimum,width*.32-measure(prefix+'/')));
 let directory=middleText(parent,parentWidth,measure,false);
 if(directory==='…'&&measure(letters[0]+'…')<=parentWidth)directory=letters[0]+'…';
 const start=prefix+directory+'/';
 return start+middleText(file,Math.max(0,width-measure(start)),measure);
}
