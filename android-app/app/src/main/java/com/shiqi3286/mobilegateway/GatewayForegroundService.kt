package com.shiqi3286.mobilegateway

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.Service
import android.content.Intent
import android.os.Binder
import android.os.Build
import android.os.IBinder
import androidx.core.app.NotificationCompat
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.launch

sealed interface GatewayEvent {
    data object Starting : GatewayEvent
    data class Running(val localUrl: String, val lanUrl: String?) : GatewayEvent
    data object Stopping : GatewayEvent
    data object Stopped : GatewayEvent
    data class Error(val message: String) : GatewayEvent
}

class GatewayForegroundService : Service() {
    companion object {
        const val ACTION_START = "com.shiqi3286.mobilegateway.START"
        const val ACTION_STOP = "com.shiqi3286.mobilegateway.STOP"
        private const val CHANNEL_ID = "gateway_runtime"
        private const val NOTIFICATION_ID = 8080
        val events = MutableSharedFlow<GatewayEvent>(replay = 1)
    }

    private val binder = LocalBinder()
    private val serviceScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private var startJob: Job? = null
    private lateinit var processManager: GatewayProcessManager

    inner class LocalBinder : Binder() {
        fun service(): GatewayForegroundService = this@GatewayForegroundService
    }

    override fun onCreate() {
        super.onCreate()
        processManager = GatewayProcessManager(this)
        createNotificationChannel()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_STOP -> stopGateway()
            else -> startGateway()
        }
        return START_STICKY
    }

    private fun startGateway() {
        if (startJob?.isActive == true || processManager.isAlive()) return
        startForeground(NOTIFICATION_ID, buildNotification("网关启动中…"))
        events.tryEmit(GatewayEvent.Starting)
        startJob = serviceScope.launch {
            try {
                val ready = processManager.start()
                if (!ready) throw IllegalStateException("网关端口 8080 未就绪")
                events.emit(
                    GatewayEvent.Running(
                        localUrl = "http://127.0.0.1:8080/admin",
                        lanUrl = null
                    )
                )
                updateNotification("网关运行中 · 端口 8080")
            } catch (error: Exception) {
                events.emit(GatewayEvent.Error(error.message ?: "网关启动失败"))
                updateNotification("网关启动失败")
            }
        }
    }

    private fun stopGateway() {
        serviceScope.launch {
            events.emit(GatewayEvent.Stopping)
            processManager.stop()
            events.emit(GatewayEvent.Stopped)
            stopForeground(STOP_FOREGROUND_REMOVE)
            stopSelf()
        }
    }

    override fun onBind(intent: Intent?): IBinder = binder

    override fun onDestroy() {
        startJob?.cancel()
        processManager.stop()
        serviceScope.coroutineContext[Job]?.cancel()
        super.onDestroy()
    }

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(
                CHANNEL_ID,
                getString(R.string.gateway_channel_name),
                NotificationManager.IMPORTANCE_LOW
            ).apply {
                description = getString(R.string.gateway_channel_description)
            }
            getSystemService(NotificationManager::class.java).createNotificationChannel(channel)
        }
    }

    private fun buildNotification(text: String): Notification =
        NotificationCompat.Builder(this, CHANNEL_ID)
            .setSmallIcon(android.R.drawable.stat_sys_upload)
            .setContentTitle(getString(R.string.app_name))
            .setContentText(text)
            .setOngoing(true)
            .setCategory(NotificationCompat.CATEGORY_SERVICE)
            .build()

    private fun updateNotification(text: String) {
        getSystemService(NotificationManager::class.java)
            .notify(NOTIFICATION_ID, buildNotification(text))
    }
}
