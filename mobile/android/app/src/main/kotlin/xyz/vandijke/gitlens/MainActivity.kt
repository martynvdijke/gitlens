package xyz.vandijke.gitlens

import android.webkit.CookieManager
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel

class MainActivity : FlutterActivity() {
    private val channelName = "xyz.vandijke.gitlens/cookies"

    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)

        // OIDC runs in a WebView, so make sure its cookie store accepts the
        // signed session cookie issued by the GitLens server.
        CookieManager.getInstance().setAcceptCookie(true)

        // `gitlens_session_oidc` is HttpOnly and therefore invisible to
        // JavaScript. Expose the native WebView cookie store to Dart instead.
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, channelName)
            .setMethodCallHandler { call, result ->
                when (call.method) {
                    "getCookies" -> {
                        val url = call.argument<String>("url")
                        if (url.isNullOrEmpty()) {
                            result.error("invalid_url", "A url argument is required", null)
                        } else {
                            // The WebView commits cookies asynchronously; flush
                            // so the freshly issued session is visible here.
                            CookieManager.getInstance().flush()
                            result.success(CookieManager.getInstance().getCookie(url))
                        }
                    }
                    "clearCookies" -> {
                        // Clear stale cookies before a new OIDC flow so an old
                        // session can never be captured as the new one.
                        CookieManager.getInstance().removeAllCookies {
                            CookieManager.getInstance().flush()
                            result.success(null)
                        }
                    }
                    else -> result.notImplemented()
                }
            }
    }
}
