import {expect,it,vi} from 'vitest';
import Picker from './FilePicker.vue';
import {setupComponent} from '../test/setup-component';
it('emits the original File once without uploading and respects disabled selection',()=>{
 const file=new File(['subtitle'],'long.S01E04.srt'),select=vi.fn();
 const v=setupComponent(Picker,{accept:'.srt',label:'选择字幕文件',onSelect:select});v.state.choose({name:file.name,raw:file});expect(select).toHaveBeenCalledExactlyOnceWith(file);v.unmount();
 const disabled=setupComponent(Picker,{accept:'.srt',label:'选择字幕文件',disabled:true,onSelect:select});disabled.state.choose({name:file.name,raw:file});expect(select).toHaveBeenCalledTimes(1);disabled.unmount();
});
