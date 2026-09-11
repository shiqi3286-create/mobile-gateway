package com.shiqi3286.mobilegateway

import android.content.Context
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.withContext
import java.io.File
import java.net.HttpURLConnection
import java.net.URL

/**
 * 负责准备 assets 中的 Go ELF 文件和 HTML，并管理 Go 子进程。
 * Android 不允许直接执行 APK assets，因此必须先复制到 filesDir。
 */
class GatewayProcessManager(private val context: Context) {
    companion object {
        private const val HTML_ASSET = "gateway-prototype.html"
        private const val PORT = 8080
        private const val TAG = "GatewayProcess"
    }

    private var process: Process? = null

    suspend fun start(): Boolean = withContext(Dispatchers.IO) {
        if (process?.isAlive == true) return@withContext waitUntilReady()

        val webDir = File(context.filesDir, "web").apply { mkdirs() }
        val dataDir = File(context.filesDir, "data").apply { mkdirs() }

        // 原生库必须真实解压到本机；仅靠压缩包内映射无法物理执行。
        val execFile = File(context.applicationInfo.nativeLibraryDir, "libgateway.so")
        android.util.Log.i(TAG, "nativeLibraryDir=${context.applicationInfo.nativeLibraryDir}")
        android.util.Log.i(TAG, "execFile=${execFile.absolutePath} exists=${execFile.exists()} canExecute=${execFile.canExecute()}")
        if (!execFile.exists() || !execFile.canExecute()) {
            android.util.Log.e(TAG, "原生库网关二进制缺失或不可执行: ${execFile.absolutePath} exists=${execFile.exists()} canExecute=${execFile.canExecute()}")
            throw IllegalStateException(
                "找不到或不可执行 APK 原生库中的网关二进制: ${execFile.absolutePath} " +
                        "(exists=${execFile.exists()}, canExecute=${execFile.canExecute()})"
            )
        }
        copyAssetIfChanged(HTML_ASSET, File(webDir, "index.html"))

        try {
            android.util.Log.i(TAG, "启动网关：workingDir=${context.filesDir.absolutePath} cmd=${execFile.absolutePath}")
            process = ProcessBuilder(
                execFile.absolutePath,
                "-port", PORT.toString(),
                "-data", dataDir.absolutePath,
                "-web", webDir.absolutePath,
                "-demo=false"
            ).directory(context.filesDir)
                .redirectErrorStream(true)
                .start()
        } catch (e: Exception) {
            android.util.Log.e(TAG, "启动网关进程失败: ${e.message}", e)
            throw e
        }

        // 持续消费 Go 日志，避免 stdout 管道缓冲区满导致子进程阻塞。
        Thread {
            process?.inputStream?.bufferedReader()?.useLines { lines ->
                lines.forEach { line -> android.util.Log.i(TAG, line) }
            }
        }.start()

        waitUntilReady()
    }

    fun stop() {
        process?.destroy()
        if (process?.isAlive == true) process?.destroyForcibly()
        process = null
    }

    fun isAlive(): Boolean = process?.isAlive == true

    private suspend fun waitUntilReady(): Boolean {
        repeat(30) {
            if (checkHealth()) return true
            delay(250)
        }
        return false
    }

    private fun checkHealth(): Boolean {
        return try {
            val connection = (URL("http://127.0.0.1:$PORT/api/health").openConnection() as HttpURLConnection)
            connection.connectTimeout = 300
            connection.readTimeout = 300
            connection.requestMethod = "GET"
            val success = connection.responseCode in 200..299
            connection.disconnect()
            success
        } catch (_: Exception) {
            false
        }
    }

    private fun copyAssetIfChanged(assetName: String, destination: File) {
        // 每次启动覆盖，确保 APK 更新后的资源不会继续使用旧版本。
        context.assets.open(assetName).use { input ->
            destination.outputStream().use { output -> input.copyTo(output) }
        }
    }
}
