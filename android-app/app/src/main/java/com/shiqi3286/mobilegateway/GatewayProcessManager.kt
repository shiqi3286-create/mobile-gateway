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
        private const val BINARY_ASSET = "聚合网关-安卓0.0-arm64"
        private const val LOCAL_BINARY_NAME = "gateway-arm64"
        private const val HTML_ASSET = "gateway-prototype.html"
        private const val PORT = 8080
    }

    private var process: Process? = null

    suspend fun start(): Boolean = withContext(Dispatchers.IO) {
        if (process?.isAlive == true) return@withContext waitUntilReady()

        val gatewayDir = File(context.filesDir, "gateway").apply { mkdirs() }
        val webDir = File(context.filesDir, "web").apply { mkdirs() }
        val dataDir = File(context.filesDir, "data").apply { mkdirs() }
        val binary = File(gatewayDir, LOCAL_BINARY_NAME)

        copyAssetIfChanged(BINARY_ASSET, binary)
        copyAssetIfChanged(HTML_ASSET, File(webDir, "index.html"))
        makeExecutable(binary)

        process = ProcessBuilder(
            binary.absolutePath,
            "-port", PORT.toString(),
            "-data", dataDir.absolutePath,
            "-web", webDir.absolutePath,
            "-demo=false"
        ).directory(context.filesDir)
            .redirectErrorStream(true)
            .start()

        // 持续消费 Go 日志，避免 stdout 管道缓冲区满导致子进程阻塞。
        Thread {
            process?.inputStream?.bufferedReader()?.useLines { lines ->
                lines.forEach { line -> android.util.Log.i("GatewayProcess", line) }
            }
        }.start()

        waitUntilReady()
    }

    private fun makeExecutable(file: File) {
        // assets 解压出的文件默认没有 x 权限；同时使用 Java API 和 chmod，
        // 兼容不同 Android 厂商对 app 私有目录执行权限的处理差异。
        val javaPermission = file.setExecutable(true, false)
        val chmodExit = ProcessBuilder("chmod", "755", file.absolutePath)
            .redirectErrorStream(true)
            .start()
            .also { it.inputStream.close() }
            .waitFor()
        if (!javaPermission && chmodExit != 0) {
            throw IllegalStateException("无法设置网关二进制可执行权限: ${file.absolutePath}")
        }
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
