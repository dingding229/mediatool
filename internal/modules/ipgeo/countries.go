package ipgeo

import "strings"

// Offline display dictionary keyed by ISO 3166-1 alpha-2. The bundled xdb
// currently returns English country names, so aliases resolve to the same code.
var countryNames = map[string]string{
	"CN": "中国", "US": "美国", "NL": "荷兰", "GB": "英国", "JP": "日本", "KR": "韩国", "KP": "朝鲜", "HK": "中国香港", "MO": "中国澳门", "TW": "中国台湾", "SG": "新加坡", "DE": "德国", "FR": "法国", "CA": "加拿大", "AU": "澳大利亚", "NZ": "新西兰", "RU": "俄罗斯", "IN": "印度", "MY": "马来西亚", "TH": "泰国", "VN": "越南", "ID": "印度尼西亚", "PH": "菲律宾", "IT": "意大利", "ES": "西班牙", "CH": "瑞士", "SE": "瑞典", "NO": "挪威", "FI": "芬兰", "DK": "丹麦", "BE": "比利时", "AT": "奥地利", "IE": "爱尔兰", "PT": "葡萄牙", "PL": "波兰", "CZ": "捷克", "UA": "乌克兰", "TR": "土耳其", "IL": "以色列", "AE": "阿联酋", "SA": "沙特阿拉伯", "BR": "巴西", "MX": "墨西哥", "AR": "阿根廷", "ZA": "南非", "EG": "埃及", "IS": "冰岛", "LU": "卢森堡", "RO": "罗马尼亚", "HU": "匈牙利", "GR": "希腊", "CL": "智利", "CO": "哥伦比亚", "PK": "巴基斯坦", "BD": "孟加拉国",
}
var countryAliases = map[string]string{
	"CHINA": "CN", "UNITED STATES": "US", "UNITED STATES OF AMERICA": "US", "USA": "US", "NETHERLANDS": "NL", "UNITED KINGDOM": "GB", "UK": "GB", "JAPAN": "JP", "SOUTH KOREA": "KR", "REPUBLIC OF KOREA": "KR", "NORTH KOREA": "KP", "HONG KONG": "HK", "MACAO": "MO", "MACAU": "MO", "TAIWAN": "TW", "SINGAPORE": "SG", "GERMANY": "DE", "FRANCE": "FR", "CANADA": "CA", "AUSTRALIA": "AU", "NEW ZEALAND": "NZ", "RUSSIA": "RU", "RUSSIAN FEDERATION": "RU", "INDIA": "IN", "MALAYSIA": "MY", "THAILAND": "TH", "VIETNAM": "VN", "INDONESIA": "ID", "PHILIPPINES": "PH", "ITALY": "IT", "SPAIN": "ES", "SWITZERLAND": "CH", "SWEDEN": "SE", "NORWAY": "NO", "FINLAND": "FI", "DENMARK": "DK", "BELGIUM": "BE", "AUSTRIA": "AT", "IRELAND": "IE", "PORTUGAL": "PT", "POLAND": "PL", "CZECHIA": "CZ", "CZECH REPUBLIC": "CZ", "UKRAINE": "UA", "TURKEY": "TR", "TÜRKIYE": "TR", "ISRAEL": "IL", "UNITED ARAB EMIRATES": "AE", "SAUDI ARABIA": "SA", "BRAZIL": "BR", "MEXICO": "MX", "ARGENTINA": "AR", "SOUTH AFRICA": "ZA", "EGYPT": "EG", "ICELAND": "IS", "LUXEMBOURG": "LU", "ROMANIA": "RO", "HUNGARY": "HU", "GREECE": "GR", "CHILE": "CL", "COLOMBIA": "CO", "PAKISTAN": "PK", "BANGLADESH": "BD",
}

func ChineseCountry(raw string) string {
	value := strings.TrimSpace(raw)
	code := strings.ToUpper(value)
	if alias, ok := countryAliases[code]; ok {
		code = alias
	}
	if name, ok := countryNames[code]; ok {
		return name
	}
	return value
}
