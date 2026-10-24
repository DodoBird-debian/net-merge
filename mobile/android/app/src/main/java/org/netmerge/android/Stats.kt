package org.netmerge.android

import org.json.JSONArray

/** Parsed form of the engine's per-network stats (netmerge.Netmerge.stats()). */
data class Stats(
    val name: String,
    val id: String,
    val sessionBytes: Long,
    val quotaBytes: Long,
    val quotaUsedBytes: Long,
    val quotaRemainingBytes: Long,
    val quotaExhausted: Boolean,
    val limitMbps: Double,
    val state: String,
    val state6: String,
    val active: Long,
    val total: Long,
    val failed: Long,
    val bytesIn: Long,
    val bytesOut: Long,
    val rateIn: Double,
    val rateOut: Double,
    val weight: Double,
    val present: Boolean,
) {
    /** "up", "checking" or "no internet", folding IPv4 and IPv6 into one word. */
    val status: String
        get() = when {
            state == "up" || state6 == "up" -> "up"
            state == "checking" || state6 == "checking" -> "checking"
            else -> "no internet"
        }

    fun displayName(ctx: android.content.Context): String = when {
        name.startsWith("Mobile data") -> ctx.getString(R.string.mobile_data) + name.removePrefix("Mobile data")
        name.startsWith("Wi-Fi") -> ctx.getString(R.string.wifi) + name.removePrefix("Wi-Fi")
        else -> name
    }
    companion object {
        fun parse(json: String): List<Stats> {
            val arr = runCatching { JSONArray(json) }.getOrNull() ?: return emptyList()
            return (0 until arr.length()).map { i ->
                val o = arr.getJSONObject(i)
                Stats(
                    o.optString("name"), o.optString("id"), o.optLong("sessionBytes"), o.optLong("quotaBytes"),
                    o.optLong("quotaUsedBytes"), o.optLong("quotaRemainingBytes"), o.optBoolean("quotaExhausted"), o.optDouble("limitMbps",0.0), o.optString("state"), o.optString("state6"),
                    o.optLong("active"), o.optLong("total"), o.optLong("failed"),
                    o.optLong("bytesIn"), o.optLong("bytesOut"),
                    o.optDouble("rateIn", 0.0), o.optDouble("rateOut", 0.0),
                    o.optDouble("weight", 1.0), o.optBoolean("present"),
                )
            }
        }

        fun rate(bytesPerSec: Double): String = "%.2f Mbps".format(java.util.Locale.US, bytesPerSec * 8 / 1e6)
        fun gb(bytes: Long): String = "%.2f GB".format(java.util.Locale.US, bytes / 1e9)
    }
}
