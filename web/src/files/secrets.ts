// Untouched echoes must not overwrite a token refreshed on the server while
// the editor was open. Blank input retains existing credentials as before.
export function changedSecrets(values:Record<string,string>,echoes:Record<string,string>):Record<string,string>{
 return Object.fromEntries(Object.entries(values).filter(([key,value])=>value!==''&&value!==echoes[key]))
}
