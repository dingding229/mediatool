import { describe, expect, it } from "vitest";
import { resourceSources, sourceLabel } from "./resource-source";
describe("case-sensitive source presentation", () => {
  it("uses the five requested display labels", () =>
    expect(resourceSources.map((x) => x.label)).toEqual([
      "WEB-DL",
      "WEBRip",
      "Blu-ray",
      "Remux",
      "HDTV",
    ]));
  it("keeps Blu-ray paired with the normalized bluray matching key", () =>
    expect(sourceLabel("bluray")).toBe("Blu-ray"));
  it("does not display raw unknown keys", () =>
    expect(sourceLabel("unknown")).toBe("来源未知"));
});
