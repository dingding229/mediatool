// Presentation labels are intentionally case-sensitive; values stay normalized
// to the parser's matching keys (e.g. Blu-ray is stored as bluray).
export const resourceSources = [
  { label: "WEB-DL", value: "web-dl" },
  { label: "WEBRip", value: "webrip" },
  { label: "Blu-ray", value: "bluray" },
  { label: "Remux", value: "remux" },
  { label: "HDTV", value: "hdtv" },
] as const;
export function sourceLabel(value?: string): string {
  return (
    resourceSources.find((item) => item.value === value)?.label ?? "来源未知"
  );
}
