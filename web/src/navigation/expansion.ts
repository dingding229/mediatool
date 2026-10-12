// Keep disclosure state in the mounted workbench only. A reload starts closed.
export function toggleNavigationGroup(expanded: readonly string[], groupId: string): string[] {
  return expanded.includes(groupId)
    ? expanded.filter(id => id !== groupId)
    : [...expanded, groupId]
}
