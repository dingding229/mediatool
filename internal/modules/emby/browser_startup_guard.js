// Injected only into the hash-verified Emby playback-start callback. The guard
// never calls loadSource/startLoad, changes retry policies, or alters formats.
var mtStartupGuard = (function () {
    var pending = true;
    var deadlineTimer = null;
    var originalResolve = resolve;
    var originalReject = reject;
    var destroyEvent = Hls.Events.DESTROYING;

    function abortError() {
        var error = new Error("Playback interrupted");
        error.name = "AbortError";
        return error;
    }
    function cleanup() {
        if (deadlineTimer !== null) {
            clearTimeout(deadlineTimer);
            deadlineTimer = null;
        }
        if (signal && typeof signal.removeEventListener === "function") {
            signal.removeEventListener("abort", onAbort);
        }
        if (destroyEvent && typeof hls.off === "function") {
            hls.off(destroyEvent, onDestroy);
        }
    }
    function finish(success, result) {
        if (!pending) return;
        pending = false;
        cleanup();
        reject = null;
        var callback = success ? originalResolve : originalReject;
        if (callback) callback(result);
    }
    resolve = function (result) { finish(true, result); };
    reject = function (error) { finish(false, error); };

    function onAbort() {
        if (!pending) return;
        var error = signal && signal.aborted ? getAbortError(signal) : null;
        finish(false, error || abortError());
        destroyHlsPlayer(hls);
    }
    function onDestroy() {
        // Emby's error handler destroys before rejecting with its own error.
        // Defer cancellation so internal 401/fatal/media errors retain their
        // original classification. An external stop still settles promptly.
        Promise.resolve().then(function () {
            if (pending) finish(false, abortError());
        });
    }
    deadlineTimer = setTimeout(function () {
        if (!pending) return;
        var error = new Error("Playback failure: network");
        error.name = "network";
        finish(false, error);
        destroyHlsPlayer(hls);
    }, 90000);
    if (destroyEvent && typeof hls.on === "function") hls.on(destroyEvent, onDestroy);
    if (signal && typeof signal.addEventListener === "function") {
        signal.addEventListener("abort", onAbort, { once: true });
    }
    if (signal && signal.aborted) onAbort();

    function shouldWait(data) {
        if (!pending || !reject || (signal && signal.aborted) || !data ||
            data.fatal !== false || data.type !== Hls.ErrorTypes.NETWORK_ERROR) return false;
        if (["manifestLoadError", "manifestLoadTimeOut", "levelLoadError", "levelLoadTimeOut",
            "audioTrackLoadError", "audioTrackLoadTimeOut", "fragLoadError", "fragLoadTimeOut"]
            .indexOf(data.details) === -1) return false;
        var code = data.response && data.response.code;
        if (code !== undefined && code !== null &&
            [0, 408, 500, 502, 503, 504].indexOf(code) === -1) return false;
        var action = data.errorAction;
        // Exact verified HLS 1.6.0-beta.2 uses action 5 for RetryRequest.
        // A level-switch/do-nothing action may carry retryConfig too; that is
        // not proof a retry was selected and must not delay terminal handling.
        if (Hls.version !== "1.6.0-beta.2" || !action || action.action !== 5) return false;
        var config = action.retryConfig;
        var count = action && action.retryCount;
        if (!config || !Number.isInteger(config.maxNumRetry) || config.maxNumRetry <= 0 ||
            !Number.isInteger(count) || count < 0 || count >= config.maxNumRetry) return false;
        // Let HLS's already-selected, finite retry run. No manual retry, session
        // replacement, 4xx suppression, or bypass of authorization checks.
        console.log("MediaTool HLS startup: configured transient retry pending");
        return true;
    }
    return { shouldWait: shouldWait, stopped: function () { return !pending; } };
})();
if (mtStartupGuard.stopped()) return;
