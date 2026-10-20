import {expect,it} from 'vitest'
import {size,progress,seedingLabel,sameMedia} from './model'
it('keeps real zero distinct from unavailable progress, size and removed seeds',()=>{expect(progress(0)).toBe('0%');expect(progress(null)).toBe('—');expect(size(0)).toBe('0 B');expect(size(undefined)).toBe('—');expect(seedingLabel('removed')).toBe('种子已移除');expect(seedingLabel()).toBe('—')})
it('a numeric TMDB id is not enough to conflate a movie and a series',()=>{expect(sameMedia({id:1,mediaType:'movie',title:'A'},{id:1,mediaType:'series',title:'B'})).toBe(false)})
