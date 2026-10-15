import languageCatalog from "./languages.json";

// TMDB genre identifiers; countries use ISO 3166-1 alpha-2, never display-name matching.
export const genres = [
  [28, "动作", "movie"],
  [12, "冒险", "movie"],
  [16, "动画", "both"],
  [35, "喜剧", "both"],
  [80, "犯罪", "both"],
  [99, "纪录片", "both"],
  [18, "剧情", "both"],
  [10751, "家庭", "both"],
  [14, "奇幻", "movie"],
  [36, "历史", "movie"],
  [27, "恐怖", "movie"],
  [10402, "音乐", "movie"],
  [9648, "悬疑", "both"],
  [10749, "爱情", "movie"],
  [878, "科幻", "movie"],
  [10770, "电视电影", "movie"],
  [53, "惊悚", "movie"],
  [10752, "战争", "movie"],
  [37, "西部", "both"],
  [10759, "动作与冒险", "series"],
  [10762, "儿童", "series"],
  [10763, "新闻", "series"],
  [10764, "真人秀", "series"],
  [10765, "科幻与奇幻", "series"],
  [10766, "肥皂剧", "series"],
  [10767, "脱口秀", "series"],
  [10768, "战争与政治", "series"],
].map(([value, label, type]) => ({
  value: Number(value),
  label: String(label),
  type: String(type),
}));
const codes =
  "AD AE AF AG AI AL AM AO AQ AR AS AT AU AW AX AZ BA BB BD BE BF BG BH BI BJ BL BM BN BO BQ BR BS BT BV BW BY BZ CA CC CD CF CG CH CI CK CL CM CN CO CR CU CV CW CX CY CZ DE DJ DK DM DO DZ EC EE EG EH ER ES ET FI FJ FK FM FO FR GA GB GD GE GF GG GH GI GL GM GN GP GQ GR GS GT GU GW GY HK HM HN HR HT HU ID IE IL IM IN IO IQ IR IS IT JE JM JO JP KE KG KH KI KM KN KP KR KW KY KZ LA LB LC LI LK LR LS LT LU LV LY MA MC MD ME MF MG MH MK ML MM MN MO MP MQ MR MS MT MU MV MW MX MY MZ NA NC NE NF NG NI NL NO NP NR NU NZ OM PA PE PF PG PH PK PL PM PN PR PS PT PW PY QA RE RO RS RU RW SA SB SC SD SE SG SH SI SJ SK SL SM SN SO SR SS ST SV SX SY SZ TC TD TF TG TH TJ TK TL TM TN TO TR TT TV TW TZ UA UG UM US UY UZ VA VC VE VG VI VN VU WF WS YE YT ZA ZM ZW".split(
    " ",
  );
const names = new Intl.DisplayNames(["zh-CN"], { type: "region" });
export const countries = codes.map((value) => ({
  value,
  label: names.of(value) || value,
}));
export function genreChoices(type: string, selected: number[]) {
  const options = genres.filter((g) => g.type === "both" || g.type === type);
  for (const value of selected)
    if (!options.some((g) => g.value === value))
      options.push({
        value,
        label:
          genres.find((g) => g.value === value)?.label || `已保存题材 ${value}`,
        type,
      });
  return options;
}
export function countryChoices(selected: string[]) {
  return [
    ...countries,
    ...selected
      .filter((v) => !countries.some((c) => c.value === v))
      .map((value) => ({ value, label: `已保存地区 ${value}` })),
  ];
}

// ISO 639-1 language identifiers plus TMDB's Cantonese/no-language values and the saved Cantonese ISO 639-3 code.
// Keep the built-in vocabulary independent of browser ICU/locale coverage.
const languageCodes = Object.keys(languageCatalog);
const languageNames = new Intl.DisplayNames(["zh-CN"], {
  type: "language",
  fallback: "none",
});
const languageLabels: Record<string, string> = {
  ...languageCatalog,
  cmn: "汉语（普通话）",
};
export function languageLabel(value: string): string {
  // Recognized saved three-letter identifiers keep their original matching code.
  // Unknown values remain visible and selectable instead of being discarded.
  try {
    return languageLabels[value] || languageNames.of(value) || `已保存语言 ${value}`;
  } catch {
    return `已保存语言 ${value}`;
  }
}
export const languages = languageCodes.map((value) => ({
  value,
  label: languageLabel(value),
}));
export function languageChoices(selected: string[]) {
  return [
    ...languages,
    ...[...new Set(selected)]
      .filter((value) => !languages.some((language) => language.value === value))
      .map((value) => ({ value, label: languageLabel(value) })),
  ];
}
export interface Choice<T extends string | number = string | number> {
  value: T;
  label: string;
}
/** Filtering never changes the model; selected matches precede other matches. */
export function filterChoices<T extends string | number>(
  options: readonly Choice<T>[],
  selected: readonly T[],
  query = "",
): Choice<T>[] {
  const tokens = query.trim().toLowerCase().split(/\s+/).filter(Boolean);
  const values = new Set(selected);
  return options
    .filter((option) => {
      const text = `${option.label} ${option.value}`.toLowerCase();
      return tokens.every((token) => text.includes(token));
    })
    .sort((a, b) => Number(values.has(b.value)) - Number(values.has(a.value)));
}
