// Split by Unicode code point so CSS can shrink the middle, preserving both ends.
export function splitDisplayPath(value:string):[string,string]{
 const points=Array.from(value),middle=Math.ceil(points.length/2);
 return [points.slice(0,middle).join(''),points.slice(middle).join('')];
}
