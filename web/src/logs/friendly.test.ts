import { describe, it, expect } from "vitest";
import {
  friendlyMessage,
  friendlyFields,
  moduleLabel,
  levelLabel,
  logAdvice,
} from "./friendly";
import { logFields, logText } from "./format";
describe("user-facing log explanations", () => {
  it("explains redirects without claiming playback succeeded", () => {
    const row = {
      msg: "HTTP request",
      status: 302,
      url: "https://owned/strm?apiKey=full",
      location: "https://cdn.owned/video?sign=full",
      durationMs: 12,
    };
    expect(friendlyMessage(row)).toContain("等待客户端直连");
    expect(logAdvice(row)).toContain("不代表客户端已成功播放");
    expect(friendlyFields(row)).toContain("12 ms");
    expect(logFields(row).location).toBe(row.location);
    expect(logText([row], false)).toContain("?sign=full");
  });
  it("uses status and real outcomes rather than inventing success", () => {
    expect(friendlyMessage({ msg: "HTTP request", status: 403 })).toContain(
      "拒绝",
    );
    expect(
      friendlyMessage({
        msg: "Playback resolution stage",
        stage: "resolve",
        outcome: "error",
      }),
    ).toContain("失败");
    expect(friendlyMessage({ msg: "Owned unknown message" })).toBe(
      "Owned unknown message",
    );
  });
  it("offers actionable explanations and retains unknown identifiers", () => {
    expect(moduleLabel("search")).toBe("资源搜索");
    expect(moduleLabel("owned-module")).toBe("owned-module");
    expect(levelLabel("WARN")).toBe("警告");
    expect(logAdvice({ msg: "player UA denied" })).toContain("白名单");
    expect(logAdvice({ msg: "HTTP stream aborted" })).toContain("不等于");
  });
});
