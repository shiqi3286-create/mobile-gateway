package com.shiqi3286.mobilegateway

import android.Manifest
import android.os.Build
import android.os.Bundle
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.viewmodel.compose.viewModel

class MainActivity : ComponentActivity() {
    private val notificationPermission = registerForActivityResult(
        ActivityResultContracts.RequestPermission()
    ) { }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        if (Build.VERSION.SDK_INT >= 33) {
            notificationPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
        }
        setContent { GatewayApp() }
    }
}

@Composable
private fun GatewayApp(vm: GatewayViewModel = viewModel()) {
    val state by vm.uiState.collectAsState()
    val running = state.status == GatewayStatus.RUNNING

    MaterialTheme {
        Column(
            modifier = Modifier.fillMaxSize().padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            Text("聚合网关-安卓0.0", style = MaterialTheme.typography.headlineSmall)
            Card(modifier = Modifier.fillMaxWidth()) {
                Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                    Text("状态：${statusText(state.status)}")
                    Text("监听端口：${state.port}")
                    Text("本机地址：${state.localUrl}")
                    state.lanUrl?.let { Text("局域网地址：$it") }
                    state.errorMessage?.let { Text("错误：$it", color = MaterialTheme.colorScheme.error) }
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        Button(onClick = vm::startGateway, enabled = !running) { Text("启动网关") }
                        OutlinedButton(onClick = vm::stopGateway, enabled = running) { Text("停止网关") }
                    }
                }
            }
            if (running) {
                Column(modifier = Modifier.fillMaxWidth().weight(1f)) {
                    AndroidView(
                        modifier = Modifier.fillMaxSize(),
                        factory = { context ->
                            WebView(context).apply {
                                settings.javaScriptEnabled = true
                                settings.domStorageEnabled = true
                                webViewClient = WebViewClient()
                                loadUrl(state.localUrl)
                            }
                        },
                        update = { webView ->
                            if (webView.url != state.localUrl) webView.loadUrl(state.localUrl)
                        }
                    )
                }
            } else {
                Text("启动网关后，管理界面将在这里打开。", modifier = Modifier.padding(top = 24.dp))
            }
        }
    }
}

private fun statusText(status: GatewayStatus): String = when (status) {
    GatewayStatus.STOPPED -> "已停止"
    GatewayStatus.STARTING -> "启动中"
    GatewayStatus.RUNNING -> "运行中"
    GatewayStatus.STOPPING -> "停止中"
    GatewayStatus.ERROR -> "错误"
}
