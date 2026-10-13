package sites

import (
	"context"
	"net/url"
	"strings"
)

// Summary statistics are explicit totals, never fabricated by counting HTML
// nodes. Only the declared authenticated AJAX identity may produce totals.
func (c Client) collectSeedingSummary(ctx context.Context, rule Map, vars map[string]string) (int64, int64, int, bool, string) {
	fail := func(note string) (int64, int64, int, bool, string) { return 0, 0, 1, false, note }
	summary, valid := rule["summary"].(Map)
	if !valid || len(object(rule["list"])) > 0 || text(summary["selector"]) == "" || !strings.EqualFold(text(rule["parser"]), "XPath") || text(rule["method"]) != "GET" || vars["userId"] == "" {
		return fail("做种汇总声明不完整或与列表模式冲突")
	}
	var target *url.URL
	raw, mode, err := c.callWithTarget(ctx, rule, vars, nil, &target)
	if err != nil {
		return fail("做种汇总请求失败，未填写猜测总数")
	}
	base, e := c.base(rule)
	if e != nil || target == nil || !sameRequestOrigin(base, target) || target.Path != "/getusertorrentlistajax.php" || target.Query().Get("userid") != vars["userId"] || target.Query().Get("type") != "seeding" || len(target.Query()["userid"]) != 1 || len(target.Query()["type"]) != 1 {
		return fail("做种汇总响应没有匹配声明的用户/请求边界")
	}
	// A summary is not a paginated row list. A page-scoped response cannot prove
	// overall totals even when it contains syntactically valid numbers.
	for _, key := range []string{"page", "pagenumber", "page_number", "offset", "start"} {
		if _, exists := target.Query()[key]; exists {
			return fail("分页汇总不能作为完整做种总数")
		}
	}
	if len(target.Query()) != 2 {
		return fail("做种汇总出现未声明的查询范围，不能证明总数")
	}
	if strings.TrimSpace(string(raw)) == "没有记录" {
		return 0, 0, 1, true, ""
	}
	doc, e := parseDocument(raw, mode)
	if e != nil {
		return fail("做种汇总解析失败")
	}
	// A statistics-only AJAX fragment must not be an authentication form,
	// even if a challenge echoes syntactically valid-looking counters.
	forms, e := doc.values("//form | //input[@type='password']", nil)
	if e != nil || len(forms) > 0 {
		return fail("做种汇总响应包含认证/交互表单，未确认总数")
	}
	containers, e := doc.values(text(summary["selector"]), nil)
	if e != nil || len(containers) != 1 {
		return fail("没有取得唯一的做种汇总容器")
	}
	fields := object(rule["fields"])
	if len(object(fields["count"])) == 0 || len(object(fields["size"])) == 0 {
		return fail("做种汇总缺少数量或总体积字段")
	}
	values, e := mapFields(doc, Map{"count": fields["count"], "size": fields["size"]}, containers[0], vars)
	count, ok := checkedInteger(values["count"], 63)
	size := metricBytes(values["size"])
	if e != nil || !ok || size == nil || (count == 0) != (*size == 0) {
		return fail("做种汇总数量/体积缺失、无效或矛盾")
	}
	return count, *size, 1, true, ""
}
