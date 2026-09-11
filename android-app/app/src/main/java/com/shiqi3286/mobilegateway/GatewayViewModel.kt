package com.shiqi3286.mobilegateway

import android.app.Application
import android.content.Intent
import androidx.core.content.ContextCompat
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

class GatewayViewModel(application: Application) : AndroidViewModel(application) {
    private val _uiState = MutableStateFlow(GatewayUiState())
    val uiState: StateFlow<GatewayUiState> = _uiState.asStateFlow()

    init {
        GatewayForegroundService.events.tryEmit(GatewayEvent.Stopped)
        viewModelScope.launch {
            GatewayForegroundService.events.collect { event ->
                _uiState.value = when (event) {
                    GatewayEvent.Starting -> _uiState.value.copy(
                        status = GatewayStatus.STARTING,
                        errorMessage = null
                    )
                    is GatewayEvent.Running -> _uiState.value.copy(
                        status = GatewayStatus.RUNNING,
                        localUrl = event.localUrl,
                        lanUrl = event.lanUrl,
                        errorMessage = null
                    )
                    GatewayEvent.Stopping -> _uiState.value.copy(status = GatewayStatus.STOPPING)
                    GatewayEvent.Stopped -> _uiState.value.copy(
                        status = GatewayStatus.STOPPED,
                        errorMessage = null
                    )
                    is GatewayEvent.Error -> _uiState.value.copy(
                        status = GatewayStatus.ERROR,
                        errorMessage = event.message
                    )
                }
            }
        }
    }

    fun startGateway() {
        val application = getApplication<Application>()
        val intent = Intent(application, GatewayForegroundService::class.java)
            .setAction(GatewayForegroundService.ACTION_START)
        ContextCompat.startForegroundService(application, intent)
    }

    fun stopGateway() {
        val application = getApplication<Application>()
        val intent = Intent(application, GatewayForegroundService::class.java)
            .setAction(GatewayForegroundService.ACTION_STOP)
        application.startService(intent)
    }
}
