package sites

func emptySearchEvidence(d document, rule Map) bool {
	evidence := object(object(rule["list"])["empty_evidence"])
	selector, auth := text(evidence["selector"]), text(evidence["authenticated"])
	if d.html == nil || selector == "" || auth == "" {
		return false
	}
	rows, rowErr := d.values(text(object(rule["list"])["selector"]), nil)
	if rowErr != nil || len(rows) > 0 {
		return false
	}
	nodes, err := d.values(selector, nil)
	if err != nil || len(nodes) != 1 {
		return false
	}
	logged, err := d.values(auth, nil)
	return err == nil && len(logged) > 0
}
