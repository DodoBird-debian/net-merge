package org.netmerge.android.ui

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.Image
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.foundation.shape.RoundedCornerShape
import org.netmerge.android.Engine
import org.netmerge.android.Stats
import org.netmerge.android.R

@Composable
fun MainScreen(phase: Engine.Phase, stats: List<Stats>, history: Map<String,List<Float>>,
               evenSplit: Boolean, onToggle: ()->Unit, onEvenSplit: (Boolean)->Unit, onLanguage: (String)->Unit,
               onResetAll: ()->Unit) {
    val ctx = LocalContext.current
    val error by Engine.error.collectAsState()
    var editing by remember { mutableStateOf<Stats?>(null) }
    var resetting by remember { mutableStateOf<Stats?>(null) }
    var showResetAll by remember { mutableStateOf(false) }
    LazyColumn(Modifier.fillMaxSize().safeDrawingPadding().padding(16.dp), verticalArrangement=Arrangement.spacedBy(12.dp)) {
        item {
            Image(
                painter = painterResource(R.drawable.community_banner),
                contentDescription = stringResource(R.string.community_banner),
                contentScale = ContentScale.Crop,
                modifier = Modifier.fillMaxWidth().aspectRatio(16f / 9f).clip(RoundedCornerShape(16.dp)),
            )
            Text(ltr(stringResource(R.string.app_name)), style=MaterialTheme.typography.headlineSmall)
            Row {
                TextButton(onClick={onLanguage("ar")}) { Text("العربية") }
                TextButton(onClick={onLanguage("en")}) { Text("English") }
            }
            Text(stringResource(when(phase) { Engine.Phase.OFF->R.string.paused; Engine.Phase.STARTING->R.string.starting; else->R.string.active }), style=MaterialTheme.typography.titleLarge)
            Button(onClick=onToggle) { Text(stringResource(if(phase==Engine.Phase.OFF) R.string.resume else R.string.stop)) }
            if(stats.any { it.present } && stats.filter { it.present }.all { it.quotaExhausted }) Text(stringResource(R.string.exhausted),color=MaterialTheme.colorScheme.error)
            Text(stringResource(R.string.total_used, ltr(Stats.gb(stats.sumOf {it.sessionBytes}))))
            TextButton(onClick={showResetAll=true}) { Text(stringResource(R.string.reset_all)) }
        }
        items(stats.filter { it.present },key={it.id}) { s ->
            Card(Modifier.fillMaxWidth()) { Column(Modifier.padding(12.dp)) {
                Text(s.displayName(ctx),style=MaterialTheme.typography.titleMedium)
                Text(stringResource(when { !s.present->R.string.disconnected; s.quotaExhausted->R.string.exhausted; s.status=="up"->R.string.up; s.status=="checking"->R.string.checking; else->R.string.no_internet }))
                Text(ltr("↓ ${Stats.rate(s.rateIn)}   ↑ ${Stats.rate(s.rateOut)}"))
                Text(stringResource(R.string.used_start,ltr(Stats.gb(s.sessionBytes))))
                Text(stringResource(R.string.used_allowance,ltr(Stats.gb(s.quotaUsedBytes))))
                Text(stringResource(R.string.remaining,if(s.quotaBytes==0L) stringResource(R.string.unlimited) else ltr(Stats.gb(s.quotaRemainingBytes))))
                Row {
                    TextButton(onClick={editing=s}) { Text(stringResource(R.string.limits)) }
                    TextButton(onClick={resetting=s}) { Text(stringResource(R.string.reset)) }
                }
            } }
        }
        item {
            Text(stringResource(R.string.scope),style=MaterialTheme.typography.bodySmall)
            TextButton(onClick={ctx.startActivity(android.content.Intent(android.content.Intent.ACTION_VIEW,android.net.Uri.parse(ctx.getString(R.string.link))))}) { Text(ltr(stringResource(R.string.community))) }
            Text(stringResource(R.string.credits),style=MaterialTheme.typography.bodySmall)
        }
    }
    editing?.let { s ->
        var speed by remember(s.id) { mutableStateOf(if(s.limitMbps==0.0) "" else s.limitMbps.toString()) }
        var allowance by remember(s.id) { mutableStateOf(if(s.quotaBytes==0L) "" else (s.quotaBytes/1e9).toString()) }
        var speedUnlimited by remember(s.id) { mutableStateOf(s.limitMbps==0.0) }
        var dataUnlimited by remember(s.id) { mutableStateOf(s.quotaBytes==0L) }
        AlertDialog(onDismissRequest={editing=null}, title={Text(stringResource(R.string.limits))},text={Column {
            Text(s.displayName(ctx))
            OutlinedTextField(value=speed,onValueChange={speed=it},enabled=!speedUnlimited,label={Text(stringResource(R.string.speed))},keyboardOptions=KeyboardOptions(keyboardType=KeyboardType.Decimal))
            Row { Checkbox(speedUnlimited,{speedUnlimited=it}); Text(stringResource(R.string.unlimited)) }
            OutlinedTextField(value=allowance,onValueChange={allowance=it},enabled=!dataUnlimited,label={Text(stringResource(R.string.allowance))},keyboardOptions=KeyboardOptions(keyboardType=KeyboardType.Decimal))
            Row { Checkbox(dataUnlimited,{dataUnlimited=it}); Text(stringResource(R.string.unlimited)) }
        }},confirmButton={TextButton(onClick={
            try { netmerge.Netmerge.setConnectionLimits(s.id,if(speedUnlimited) "unlimited" else speed,if(dataUnlimited) "unlimited" else allowance); Engine.sample(); editing=null } catch(e:Exception){Engine.setError(e)}
        }) { Text(stringResource(R.string.save)) }},dismissButton={TextButton(onClick={editing=null}){Text(stringResource(R.string.cancel))}})
    }
    resetting?.let { s -> AlertDialog(onDismissRequest={resetting=null},title={Text(stringResource(R.string.reset))},text={Text(stringResource(R.string.reset_confirm))},confirmButton={TextButton(onClick={
        try {netmerge.Netmerge.resetQuota(s.id);Engine.sample();resetting=null} catch(e:Exception){Engine.setError(e)}
    }){Text(stringResource(R.string.confirm))}},dismissButton={TextButton(onClick={resetting=null}){Text(stringResource(R.string.cancel))}}) }
    if(showResetAll) AlertDialog(onDismissRequest={showResetAll=false},title={Text(stringResource(R.string.reset_all))},text={Text(stringResource(R.string.reset_all_confirm))},confirmButton={TextButton(onClick={
        onResetAll(); showResetAll=false
    }) { Text(stringResource(R.string.confirm)) }},dismissButton={TextButton(onClick={showResetAll=false}){Text(stringResource(R.string.cancel))}})
    val storage = netmerge.Netmerge.storageError()
    if(error.isNotEmpty() || storage.isNotEmpty()) AlertDialog(onDismissRequest={Engine.clearError()},title={Text(stringResource(R.string.error))},text={Text(ltr(if(error.isNotEmpty()) error else storage))},confirmButton={TextButton(onClick={Engine.clearError()}){Text(stringResource(R.string.cancel))}})
}
private fun ltr(value:String) = "\u2066$value\u2069"
