package sites

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"net/url"
	"strings"

	"github.com/antchfx/htmlquery"
	"golang.org/x/net/html"
)

// collectSeeding reports authoritative totals only after proving a terminal
// page. The authenticated HDSKY AJAX "没有记录" response was verified live.
func (c Client) collectSeeding(ctx context.Context, rule Map, vars map[string]string) (int64, int64, int, bool, string) {
	var count, total int64
	seenPages := map[[32]byte]bool{}
	seenRows := map[string]bool{}
	var next *url.URL
	for page := 1; page <= SearchPageLimit; page++ {
		request := rule
		if next != nil {
			request = clone(rule)
			request["path"] = next.String()
			delete(request, "params")
		}
		var target *url.URL
		raw, mode, err := c.callWithTarget(ctx, request, vars, nil, &target)
		if err != nil {
			return 0, 0, page, false, "做种统计请求失败；未将部分页面当总数"
		}
		base, baseErr := c.base(rule)
		if nexusCompatible(c.Config) && page == 1 && baseErr == nil && target != nil && sameRequestOrigin(base, target) && target.Path == "/getusertorrentlistajax.php" && target.Query().Get("userid") == vars["userId"] && target.Query().Get("type") == "seeding" && strings.TrimSpace(string(raw)) == "没有记录" {
			return 0, 0, 1, true, ""
		}
		fingerprint := sha256.Sum256(raw)
		if seenPages[fingerprint] {
			return 0, 0, page, false, "做种列表重复页面，停止累计"
		}
		seenPages[fingerprint] = true
		d, err := parseDocument(raw, mode)
		if err != nil {
			return 0, 0, page, false, "做种列表解析失败"
		}
		list := object(rule["list"])
		selector := text(list["selector"])
		if selector == "" {
			return 0, 0, page, false, "做种列表缺少声明选择器"
		}
		rows, err := d.values(selector, nil)
		if err != nil || len(rows) == 0 {
			return 0, 0, page, false, "未取得有效做种列表或明确空列表证据"
		}
		if count+int64(len(rows)) > SearchResourceLimit {
			return 0, 0, page, false, "做种列表超过采集上限"
		}
		field := clone(object(object(rule["fields"])["size"]))
		if strings.EqualFold(mode, "XPath") {
			field["selector"] = relativeRowSelector(text(field["selector"]))
		}
		for _, row := range rows {
			value, e := mapFields(d, Map{"size": field}, row, vars)
			size := metricBytes(value["size"])
			if e != nil || size == nil || total > math.MaxInt64-*size {
				return 0, 0, page, false, "做种体积字段不完整，未填写猜测总数"
			}
			if node, ok := row.(*html.Node); ok {
				key := strings.Join(strings.Fields(htmlquery.InnerText(node)), " ")
				if seenRows[key] {
					return 0, 0, page, false, "做种列表包含重复记录，无法证明总数"
				}
				seenRows[key] = true
			} else {
				key := fmt.Sprint(row)
				if seenRows[key] {
					return 0, 0, page, false, "做种列表包含重复记录"
				}
				seenRows[key] = true
			}
			count++
			total += *size
		}
		link, declared, end, e := c.linkNavigation(d, rule, vars, target)
		if e != nil {
			return 0, 0, page, false, "做种下一页链接不满足同源同查询边界"
		}
		if declared && end {
			return count, total, page, true, ""
		}
		if !declared {
			return 0, 0, page, false, "做种列表没有明确终页证据"
		}
		if text(rule["method"]) != "" && text(rule["method"]) != "GET" {
			return 0, 0, page, false, "做种列表非GET分页不支持"
		}
		next = link
	}
	return 0, 0, SearchPageLimit, false, "做种采集达到5页上限，未将部分结果当总数"
}
