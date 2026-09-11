package com.shiqi3286.mobilegateway

/** 当前网关进程状态，供 Compose 界面展示。 */
enum class GatewayStatus {
    STOPPED,
    STARTING,
    RUNNING,
    STOPPING,
    ERROR
}

data class GatewayUiState(
    val status: GatewayStatus = GatewayStatus.STOPPED,
    val port: Int = 8080,
    val localUrl: String = "http://127.0.0.1:8080/admin",
    val lanUrl: String? = null,
    val errorMessage: String? = null
)
