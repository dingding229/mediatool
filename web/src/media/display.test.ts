import {expect,it} from 'vitest';
import {displayRating,displayMediaSize,displayResolution} from './model';
it('formats truthful rating and binary media sizes using the approved GB convention',()=>{
 expect(displayRating(6.681)).toBe('6.7');expect(displayRating(0)).toBe('—');expect(displayRating(null)).toBe('—');expect(displayRating(NaN)).toBe('—');
 expect(displayMediaSize(7960117968)).toBe('7.41 GB');expect(displayMediaSize(0)).toBe('0.00 B');expect(displayMediaSize(null)).toBe('—');
});
it.each([[0,'—'],[null,'—'],[7.25,'7.3'],[10,'10.0'],[11,'—']])('rating %s excludes missing/invalid scores',(value,expected)=>expect(displayRating(value)).toBe(expected));
it.each([[3800,'4K'],[3799,'1080p'],[1900,'1080p'],[1899,'720p'],[1260,'720p'],[1259,'SD']])('uses width boundary %i for %s',(width,tier)=>{
 expect(displayResolution([{width:Number(width),height:872}])).toBe(`${tier}（${width}×872）`);
});
it('keeps exact cropped dimensions and handles absent or aggregate facts',()=>{
 expect(displayResolution([{width:1920,height:872}])).toBe('1080p（1920×872）');expect(displayResolution()).toBe('—');
 expect(displayResolution([{width:3840,height:2160},{width:1920,height:1080}])).toBe('4K（3840×2160）、1080p（1920×1080）');
});
