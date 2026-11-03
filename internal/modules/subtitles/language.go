package subtitles

import (
	"path"
	"regexp"
	"strings"
	"unicode"
)

// Language remains ISO 639-2/B. Tags preserve Chinese script and additional
// languages without putting a non-ISO string in Emby's Language field.
type LanguageDetection struct {
	Language     string   `json:"language"`
	LanguageTags []string `json:"languageTags"`
	Label        string   `json:"label"`
	Evidence     string   `json:"evidence"`
	Encoding     string   `json:"encoding"`
	Known        bool     `json:"known"`
}

const languageToken = `(?i)(?:^|[^a-z0-9])(%s)(?:$|[^a-z0-9])`

var simpleCharacters = "这为个国汉语简体学书门车东万与云乐乡亲从众会传伤体余儿关兴养军写内农净刘则刚创动务区医华单卖卫发变叶号后听启员团园图圆圣场声处头夹夺奋妇孙实审寻导将层岁岛岭币师带库广应庙开异归录彻忆忧怀态总恋恶惊愿戏户扑扩扫执护报担拥择换据掷数敌无时显术机杀权条来极构枪枫标样树桥检梦楼欢欧气汇没泽洁浅测济浏浓涛润温湾湿满滤灭灯灵灾点炼爱爷牵独狭猎环现电画畅疗症监盖盘着礼离种积称稳穷竞笔简签粮红约级纪纯纱纲纳纵纷纸纹纺线终绍经结给统继绪续维绿编缘缴罗职联聪肃肠肤脑腾舰艰艺节苏获营萧萨蓝虑虚虫虽蚂补装观规觉触誉认让议记讲许论设访证评识译诉试诗诚话该语说请诸诺读课谁调谈谢谱贝财责败货质贩贫费贺资赋赖赚赞赵跃踪转轮软轰轻载较辅辆辉边达过迈运还这进远违连迟适选递逻邻邮郑释里鉴针钟钱铁银错键镜长门闭问闻间闷阁队阳阴阵阶际陆陈险随隐难雾静韵页顶项顺须顾顿预领题颜风飞饭饮饼馆马驳驻驾验骂骑骗鱼鸟鸣鸭鸡鹤麦黄齐龙"
var traditionalCharacters = "這為個國漢語簡體學書門車東萬與雲樂鄉親從眾會傳傷體餘兒關興養軍寫內農淨劉則剛創動務區醫華單賣衛發變葉號後聽啟員團園圖圓聖場聲處頭夾奪奮婦孫實審尋導將層歲島嶺幣師帶庫廣應廟開異歸錄徹憶憂懷態總戀惡驚願戲戶撲擴掃執護報擔擁擇換據擲數敵無時顯術機殺權條來極構槍楓標樣樹橋檢夢樓歡歐氣匯沒澤潔淺測濟瀏濃濤潤溫灣濕滿濾滅燈靈災點煉愛爺牽獨狹獵環現電畫暢療癥監蓋盤著禮離種積稱穩窮競筆簡簽糧紅約級紀純紗綱納縱紛紙紋紡線終紹經結給統繼緒續維綠編緣繳羅職聯聰肅腸膚腦騰艦艱藝節蘇獲營蕭薩藍慮虛蟲雖螞補裝觀規覺觸譽認讓議記講許論設訪證評識譯訴試詩誠話該語說請諸諾讀課誰調談謝譜貝財責敗貨質販貧費賀資賦賴賺贊趙躍蹤轉輪軟轟輕載較輔輛輝邊達過邁運還這進遠違連遲適選遞邏鄰郵鄭釋裡鑒針鐘錢鐵銀錯鍵鏡長門閉問聞間悶閣隊陽陰陣階際陸陳險隨隱難霧靜韻頁頂項順須顧頓預領題顏風飛飯飲餅館馬駁駐駕驗罵騎騙魚鳥鳴鴨雞鶴麥黃齊龍臺"

func CanonicalLanguage(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "zh", "zho", "chi", "chs", "cht", "zh-cn", "zh-tw", "zh-hans", "zh-hant":
		return "chi"
	case "en", "eng":
		return "eng"
	case "ja", "jp", "jpn":
		return "jpn"
	case "ko", "kor":
		return "kor"
	case "es":
		return "spa"
	case "ru":
		return "rus"
	case "it":
		return "ita"
	case "pt":
		return "por"
	case "ar":
		return "ara"
	case "fr", "fra", "fre":
		return "fre"
	case "de", "deu", "ger":
		return "ger"
	case "", "und":
		return "und"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}
func LanguageLabel(language string, tags []string) string {
	if CanonicalLanguage(language) == "chi" {
		for _, tag := range tags {
			if tag == "en" {
				return "中英双语"
			}
		}
		for _, tag := range tags {
			if tag == "zh-Hans" {
				return "简体中文"
			}
			if tag == "zh-Hant" {
				return "繁体中文"
			}
		}
		return "中文"
	}
	switch CanonicalLanguage(language) {
	case "eng":
		return "英语"
	case "jpn":
		return "日语"
	case "kor":
		return "韩语"
	case "und":
		return "未知"
	default:
		return "其他语言"
	}
}

var filenameMatchers = func() map[string]*regexp.Regexp {
	out := map[string]*regexp.Regexp{}
	for _, p := range []string{`chs|sc|zh-hans|zh-cn|gb|gbk|gb18030`, `cht|tc|zh-hant|zh-tw|big5`, `en|eng`, `chi|zh|zho`, `zh|chi|zho`, `ja|jp|jpn`, `ko|kor`} {
		out[p] = regexp.MustCompile(strings.Replace(languageToken, "%s", p, 1))
	}
	return out
}()

func runeSet(value string) map[rune]bool {
	out := map[rune]bool{}
	for _, r := range value {
		out[r] = true
	}
	return out
}

var simpleRunes = runeSet(simpleCharacters)
var traditionalRunes = runeSet(traditionalCharacters)
var commonRunes = runeSet("的一是在不了有和人我你他她它们我們你們这是這是字幕世界你好欢迎歡迎今天明天电影電影看见看見")

func FilenameLanguage(name string) LanguageDetection {
	lower := strings.ToLower(strings.TrimSuffix(path.Base(name), path.Ext(name)))
	has := func(pattern string) bool {
		return filenameMatchers[pattern].MatchString(lower)
	}
	hans := has(`chs|sc|zh-hans|zh-cn|gb|gbk|gb18030`)
	hant := has(`cht|tc|zh-hant|zh-tw|big5`)
	english := has(`en|eng`)
	dual := strings.Contains(lower, "简英") || strings.Contains(lower, "繁英") || strings.Contains(lower, "中英") || (hans || hant || has(`chi|zh|zho`)) && english
	out := LanguageDetection{Language: "und", LanguageTags: []string{}, Evidence: "文件名未包含明确语言标记"}
	switch {
	case dual:
		out.Language = "chi"
		out.LanguageTags = []string{"zh-Hans", "en"}
		if hant || strings.Contains(lower, "繁英") {
			out.LanguageTags[0] = "zh-Hant"
		}
	case hans:
		out.Language = "chi"
		out.LanguageTags = []string{"zh-Hans"}
	case hant:
		out.Language = "chi"
		out.LanguageTags = []string{"zh-Hant"}
	case has(`zh|chi|zho`):
		out.Language = "chi"
	case english:
		out.Language = "eng"
		out.LanguageTags = []string{"en"}
	case has(`ja|jp|jpn`):
		out.Language = "jpn"
		out.LanguageTags = []string{"ja"}
	case has(`ko|kor`):
		out.Language = "kor"
		out.LanguageTags = []string{"ko"}
	}
	out.Known = out.Language != "und"
	out.Label = LanguageLabel(out.Language, out.LanguageTags)
	if out.Known {
		out.Evidence = "来自文件名语言标记"
		for _, pattern := range []string{`chs|sc|zh-hans|zh-cn|gb|gbk|gb18030`, `cht|tc|zh-hant|zh-tw|big5`, `chi|zh|zho`, `en|eng`, `ja|jp|jpn`, `ko|kor`} {
			if match := filenameMatchers[pattern].FindStringSubmatch(lower); len(match) > 1 {
				out.Evidence = "来自文件名 ." + match[1]
				break
			}
		}
		for _, marker := range []string{"简英", "繁英", "中英"} {
			if strings.Contains(lower, marker) {
				out.Evidence = "来自文件名 " + marker
				break
			}
		}

	}
	return out
}

var htmlTag = regexp.MustCompile(`<[^>]*>|\{[^}]*\}`)

func cueText(text string) string {
	if len(text) > 262144 {
		text = text[:262144]
	}
	ass := strings.Contains(strings.ToLower(text), "[script info]") && strings.Contains(strings.ToLower(text), "[events]")
	lines := []string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || timestamp.MatchString(line) || strings.HasPrefix(line, "WEBVTT") || strings.HasPrefix(line, "[") {
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "dialogue:") {
			fields := strings.SplitN(line, ",", 10)
			if len(fields) == 10 {
				line = fields[9]
			}
		} else if ass {
			continue
		}
		lines = append(lines, htmlTag.ReplaceAllString(line, ""))
	}
	return strings.Join(lines, " ")
}

type languageCounts struct{ han, kana, halfKana, hangul, latin, simple, traditional, common, letters int }

func countLanguage(text string) languageCounts {
	var c languageCounts
	for _, r := range cueText(text) {
		if unicode.IsLetter(r) {
			c.letters++
		}
		switch {
		case unicode.In(r, unicode.Hiragana, unicode.Katakana):
			c.kana++
			if r >= 0xff65 && r <= 0xff9f {
				c.halfKana++
			}
		case unicode.In(r, unicode.Hangul):
			c.hangul++
		case unicode.In(r, unicode.Han):
			c.han++
		case unicode.In(r, unicode.Latin):
			c.latin++
		}
		if simpleRunes[r] {
			c.simple++
		}
		if traditionalRunes[r] {
			c.traditional++
		}
		if commonRunes[r] {
			c.common++
		}
	}
	return c
}
func contentLanguage(text string) LanguageDetection {
	c := countLanguage(text)
	out := LanguageDetection{Language: "und", LanguageTags: []string{}, Evidence: "未找到可靠语言特征，请手动选择"}
	switch {
	case c.kana >= 2:
		out.Language = "jpn"
		out.LanguageTags = []string{"ja"}
		out.Evidence = "来自内容中的假名特征"
	case c.hangul >= 2:
		out.Language = "kor"
		out.LanguageTags = []string{"ko"}
		out.Evidence = "来自内容中的谚文特征"
	case c.han >= 2:
		out.Language = "chi"
		if c.traditional > c.simple {
			out.LanguageTags = []string{"zh-Hant"}
		} else if c.simple > 0 {
			out.LanguageTags = []string{"zh-Hans"}
		}
		if c.latin >= 8 && c.han*5 >= c.letters && c.latin*5 >= c.letters {
			out.LanguageTags = append(out.LanguageTags, "en")
		}
		out.Evidence = "来自内容中的汉字和简繁特征"
	case c.latin >= 4 && c.latin*10 >= c.letters*7:
		out.Language = "eng"
		out.LanguageTags = []string{"en"}
		out.Evidence = "来自内容中的拉丁字母占比"
	}
	out.Known = out.Language != "und"
	out.Label = LanguageLabel(out.Language, out.LanguageTags)
	return out
}
func DetectLanguage(name, charset string, raw []byte) (LanguageDetection, error) {
	body, _, encoding, e := Normalize(name, charset, raw)
	if e != nil {
		return LanguageDetection{}, e
	}
	out := FilenameLanguage(name)
	if !out.Known {
		out = contentLanguage(string(body))
	}
	out.Encoding = encoding
	return out, nil
}
func DisplayTitle(m Metadata, ext string) string {
	_ = normalizeLanguage(&m)
	label := LanguageLabel(m.Language, m.LanguageTags)
	title := strings.TrimSpace(m.Title)
	if title == "" {
		return label + " · " + strings.ToUpper(ext)
	}
	if strings.HasPrefix(title, label) {
		return title
	}
	return label + " · " + title
}

var tagPattern = regexp.MustCompile(`^[A-Za-z]{2,3}(?:-[A-Za-z0-9]{2,8}){0,2}$`)

func normalizeLanguage(m *Metadata) error {
	legacy := strings.ToLower(strings.TrimSpace(m.Language))
	if len(m.LanguageTags) == 0 {
		switch legacy {
		case "chs", "zh-cn", "zh-hans":
			m.LanguageTags = []string{"zh-Hans"}
		case "cht", "zh-tw", "zh-hant":
			m.LanguageTags = []string{"zh-Hant"}
		}
	}
	m.Language = CanonicalLanguage(legacy)
	tags := []string{}
	seen := map[string]bool{}
	for _, tag := range m.LanguageTags {
		tag = strings.TrimSpace(tag)
		switch strings.ToLower(tag) {
		case "zh-hans":
			tag = "zh-Hans"
		case "zh-hant":
			tag = "zh-Hant"
		default:
			tag = strings.ToLower(tag)
		}
		if len(tags) >= 8 || len(tag) > 32 || !tagPattern.MatchString(tag) {
			return ErrInvalid
		}
		if !seen[tag] {
			tags = append(tags, tag)
			seen[tag] = true
		}
	}
	m.LanguageTags = tags
	return nil
}
