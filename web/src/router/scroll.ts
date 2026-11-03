import type {RouterScrollBehavior} from 'vue-router';
// A new workspace must not inherit the previous long page's scroll offset.
// Back/forward navigation restores the recorded location; same-page filters stay put.
export const navigationScroll:RouterScrollBehavior=(to,from,saved)=>saved||(to.path!==from.path?{left:0,top:0}:false);
