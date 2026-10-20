package tooling

import (
	"encoding/json"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

func policyConstant(t *testing.T, name string) string {
	t.Helper()
	m := regexp.MustCompile("const " + regexp.QuoteMeta(name) + " = `([^`]+)`").FindStringSubmatch(source(t, "internal/modules/emby/browser_policy.go"))
	if len(m) != 2 {
		t.Fatal("missing constant", name)
	}
	return m[1]
}
func TestBootstrapTagRetainsVersion(t *testing.T) {
	tag := policyConstant(t, "revisedLoaderTag")
	m := regexp.MustCompile(`src="([^"]+)"`).FindStringSubmatch(tag)
	if len(m) != 2 {
		t.Fatal(tag)
	}
	u, e := url.Parse(m[1])
	if e != nil || u.Query().Get("v") != "4.10.1.0" || u.Query().Get("mt_hls_startup") != "2" || strings.Contains(tag, "dashboardVersion") {
		t.Fatal(tag)
	}
}
func TestBootstrapExpression(t *testing.T) {
	b, _ := json.Marshal(policyConstant(t, "revisedLoaderCache"))
	cmd := exec.Command("node", "-e", "\nconst expression = JSON.parse(process.argv[1]);\nconst apply = new Function('docElem', 'appMode', expression + ';return globalThis.urlCacheParam;');\nglobalThis.dashboardVersion = '4.10.1.0';\nconst first = apply('4.10.1.0', '');\nconst second = apply('4.10.1.0', '');\nif(first !== 'v=4.10.1.0&mt_hls_startup=2' || second !== first) throw new Error('revision duplicated or version changed');\nif(globalThis.dashboardVersion !== '4.10.1.0') throw new Error('real dashboard version changed');\nconst previous = Date.now; Date.now = () => 12345;\nconst fallback = apply('', ''); Date.now = previous;\nif(fallback !== 'v=12345') throw new Error('unversioned fallback changed');\nglobalThis.urlCacheParam = 'native-preserved';\nif(apply('', 'android') !== 'native-preserved') throw new Error('native fallback changed');\nconsole.log('own-bootstrap-expression-ok');\n", string(b))
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatal(e, string(out))
	}
}
func TestBrowserExpression(t *testing.T) {
	b, _ := json.Marshal(policyConstant(t, "coldBrowserOptions"))
	cmd := exec.Command("node", "-e", "\nconst vm = require('node:vm');\nconst expr = JSON.parse(process.argv[1]);\nconst original = {maxTimeToFirstByteMs:10000,maxLoadTimeMs:120000,timeoutRetry:{maxNumRetry:4,retryDelayMs:0},errorRetry:{maxNumRetry:6,retryDelayMs:1000}};\nconst before = JSON.stringify(original);\nconst sandbox = {Hls:{DefaultConfig:{fragLoadPolicy:{default:original}}}};\nvm.runInNewContext(expr+';this.result=hlsOptions;',sandbox,{timeout:1000});\nif(sandbox.result.fragLoadPolicy.default.maxTimeToFirstByteMs!==60000)throw Error('first-byte budget not changed');\nif(sandbox.result.fragLoadPolicy.default.maxLoadTimeMs!==120000)throw Error('total budget changed');\nif(JSON.stringify(original)!==before)throw Error('global defaults mutated');\nif(JSON.stringify(sandbox.result.fragLoadPolicy.default.timeoutRetry)!==JSON.stringify(original.timeoutRetry))throw Error('retry changed');\nconst old={Hls:{DefaultConfig:{}}};vm.runInNewContext(expr+';this.result=hlsOptions;',old,{timeout:1000});\nif(old.result.fragLoadPolicy!==undefined||old.result.manifestLoadingTimeOut!==20000)throw Error('legacy fallback broke');\n", string(b))
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatal(e, string(out))
	}
}
