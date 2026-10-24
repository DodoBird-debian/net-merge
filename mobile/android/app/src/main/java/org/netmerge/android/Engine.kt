package org.netmerge.android

import android.content.Context
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/**
 * App-wide state shared by the VPN service, the UI and the Quick Settings tile. The service
 * samples the Go engine once a second; the UI only observes.
 */
object Engine {
    enum class Phase { OFF, STARTING, ON }

    /** Seconds of throughput history kept for the live chart. */
    const val HISTORY = 60

    private val _phase = MutableStateFlow(Phase.OFF)
    val phase: StateFlow<Phase> = _phase.asStateFlow()

    private val _stats = MutableStateFlow<List<Stats>>(emptyList())
    val stats: StateFlow<List<Stats>> = _stats.asStateFlow()

    /** Per network: total (down + up) bytes/s for the last [HISTORY] seconds, oldest first. */
    private val _history = MutableStateFlow<Map<String, List<Float>>>(emptyMap())
    val history: StateFlow<Map<String, List<Float>>> = _history.asStateFlow()

    private val _evenSplit = MutableStateFlow(false)
    val evenSplit: StateFlow<Boolean> = _evenSplit.asStateFlow()

    val running: Boolean get() = _phase.value == Phase.ON

    internal fun setPhase(p: Phase) {
        _phase.value = p
        if (p == Phase.OFF) {
            _stats.value = _stats.value.map { it.copy(rateIn = 0.0, rateOut = 0.0) }
            _history.value = emptyMap()
        }
    }

    /** Reads the engine's counters. Called by the service every second while it runs. */
    internal fun sample() {
        val now = Stats.parse(netmerge.Netmerge.stats())
        _stats.value = now
        val old = _history.value
        _history.value = now.associate { s ->
            val prev = old[s.name] ?: List(HISTORY) { 0f }
            s.name to (prev + (s.rateIn + s.rateOut).toFloat()).takeLast(HISTORY)
        }
    }

    // --- settings ---

    private fun prefs(ctx: Context) = ctx.getSharedPreferences("settings", Context.MODE_PRIVATE)

    private val _error = MutableStateFlow("")
    val error: StateFlow<String> = _error.asStateFlow()
    fun setError(e: Exception) { _error.value = e.localizedMessage ?: e.toString() }
    fun clearError() { _error.value = "" }
    fun localized(base: Context): Context {
        val language = prefs(base).getString("language", java.util.Locale.getDefault().language).let { if (it == "ar") "ar" else "en" }
        val config = android.content.res.Configuration(base.resources.configuration)
        val locale = java.util.Locale(language!!)
        config.setLocale(locale); config.setLayoutDirection(locale)
        return base.createConfigurationContext(config)
    }
    fun setLanguage(ctx: Context, language: String) {
        prefs(ctx).edit().putString("language",language).commit()
        netmerge.Netmerge.configure(ctx.filesDir.absolutePath, language)
    }
    fun loadSettings(ctx: Context) {
        netmerge.Netmerge.configure(ctx.filesDir.absolutePath, ctx.resources.configuration.locales[0].language)
        sample()
        _evenSplit.value = prefs(ctx).getBoolean("even_split", false)
        applySettings()
    }

    fun setEvenSplit(ctx: Context, on: Boolean) {
        prefs(ctx).edit().putBoolean("even_split", on).apply()
        _evenSplit.value = on
        applySettings()
    }

    private fun applySettings() {
        runCatching { netmerge.Netmerge.setAutoWeights(!_evenSplit.value) }
    }
}
