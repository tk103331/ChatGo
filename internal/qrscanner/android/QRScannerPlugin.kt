package dev.mygo

import android.app.Activity
import com.journeyapps.barcodescanner.CaptureActivity
import com.journeyapps.barcodescanner.ScanOptions
import org.json.JSONObject

// CaptureActivity owns the camera permission, preview, decoder and lifecycle.
// The bundled ZXing decoder works offline, without a Play Services download.
class ChatGoScanActivity : CaptureActivity()

class QRScannerPlugin : MyGoPlugin {
    private var scanning = false

    override fun call(method: String, args: JSONObject, reply: PluginReply) {
        if (method != "scan") {
            reply.failure(PluginFailure("NoMethod", method))
            return
        }
        if (scanning) {
            reply.failure(PluginFailure("Busy", "扫码界面已经打开"))
            return
        }
        scanning = true
        try {
            val intent = ScanOptions()
                .setDesiredBarcodeFormats(ScanOptions.QR_CODE)
                .setPrompt("扫描桌面 ChatGo 设置 → 手机中的配对二维码")
                .setBeepEnabled(false)
                .setOrientationLocked(false)
                .setCaptureActivity(ChatGoScanActivity::class.java)
                .createScanIntent(PluginHost.context)
            PluginHost.startForResult(intent) { code, data ->
                scanning = false
                if (data?.getBooleanExtra("MISSING_CAMERA_PERMISSION", false) == true) {
                    reply.failure(PluginFailure("PermissionDenied", "请在系统设置中允许 ChatGo 使用相机"))
                } else {
                    val text = if (code == Activity.RESULT_OK) data?.getStringExtra("SCAN_RESULT") ?: "" else ""
                    reply.success(JSONObject().put("text", text))
                }
            }
        } catch (e: Exception) {
            scanning = false
            reply.failure(PluginFailure("ScanFailed", e.localizedMessage ?: "无法打开相机"))
        }
    }
}
