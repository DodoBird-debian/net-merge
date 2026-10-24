import SwiftUI
import NetworkExtension

@MainActor final class VPNModel: ObservableObject {
    @Published var connections: [Connection] = []
    @Published var connected = false
    @Published var paused = true
    @Published var error = ""
    private var manager: NETunnelProviderManager?
    init() {
        NETunnelProviderManager.loadAllFromPreferences { managers, error in
            Task { @MainActor in
                self.manager = managers?.first { ($0.protocolConfiguration as? NETunnelProviderProtocol)?.providerBundleIdentifier == tunnelID }
                if let error = error { self.error = error.localizedDescription }
                self.refresh()
            }
        }
    }
    func start() {
        let m = manager ?? NETunnelProviderManager()
        let proto = NETunnelProviderProtocol()
        proto.providerBundleIdentifier = tunnelID; proto.serverAddress = "net-merge"
        m.protocolConfiguration = proto; m.localizedDescription = "net-merge · Iraq PC Community"; m.isEnabled = true
        m.saveToPreferences { error in
            if let error = error { Task { @MainActor in self.error = error.localizedDescription }; return }
            m.loadFromPreferences { error in
                Task { @MainActor in
                    do { if let error = error { throw error }; self.manager = m; try m.connection.startVPNTunnel() }
                    catch { self.error = error.localizedDescription }
                }
            }
        }
    }
    func send(_ message: [String:String]) {
        guard let session = manager?.connection as? NETunnelProviderSession, session.status == .connected else { return }
        do {
            try session.sendProviderMessage(JSONSerialization.data(withJSONObject:message)) { data in
                guard let data = data else { return }
                Task { @MainActor in
                    do { let response = try JSONDecoder().decode(Response.self,from:data); self.connections = response.stats; self.paused = response.paused; self.error = response.error }
                    catch { self.error = error.localizedDescription }
                }
            }
        } catch { self.error = error.localizedDescription }
    }
    func refresh() {
        connected = manager?.connection.status == .connected
        if connected { send(["action":"status"]) }
    }
    func toggle() {
        if connected { send(["action":"pause","paused":String(!paused)]) } else { start() }
    }
}

@main struct NetMergeApp: App {
    @StateObject private var model = VPNModel()
    @State private var locale = language
    var body: some Scene { WindowGroup {
        MainView(model:model,locale:$locale)
            .environment(\.locale,Locale(identifier:locale))
            .environment(\.layoutDirection,locale == "ar" ? .rightToLeft : .leftToRight)
    } }
}
struct MainView: View {
    @ObservedObject var model: VPNModel
    @Binding var locale: String
    @State private var editing: Connection?
    @State private var resetting: Connection?
    private let timer = Timer.publish(every:1,on:.main,in:.common).autoconnect()
    var body: some View {
        NavigationStack { List {
            Section {
                Picker(L("language"),selection:$locale) { Text("العربية").tag("ar"); Text("English").tag("en") }.pickerStyle(.segmented)
                    .onChange(of:locale) { language = $0; model.send(["action":"language"]) }
                Text(L(!model.connected || model.paused ? "paused" : "active")).font(.title2)
                Button(L(!model.connected || model.paused ? "resume" : "stop"),action:model.toggle)
                Text(String(format:L("total_used"),ltr(GB(model.connections.reduce(0) {$0+$1.sessionBytes}))))
                if model.connections.contains(where:{$0.present}) && model.connections.filter({$0.present}).allSatisfy({$0.quotaExhausted}) { Text(L("exhausted")).foregroundColor(.red) }
            }
            ForEach(model.connections) { c in
                Section(ltr(c.name)) {
                    Text(L(!c.present ? "disconnected" : c.quotaExhausted ? "exhausted" : c.state == "up" || c.state6 == "up" ? "up" : "checking"))
                    Text(ltr("↓ \(speed(model.connected && !model.paused ? c.rateIn : 0))   ↑ \(speed(model.connected && !model.paused ? c.rateOut : 0))"))
                    Text(String(format:L("used_start"),ltr(GB(c.sessionBytes))))
                    Text(String(format:L("used_allowance"),ltr(GB(c.quotaUsedBytes))))
                    Text(String(format:L("remaining"),c.quotaBytes == 0 ? L("unlimited") : ltr(GB(c.quotaRemainingBytes))))
                    Button(L("limits")) { editing = c }.disabled(!model.connected)
                    Button(L("reset")) { resetting = c }.disabled(!model.connected)
                }
            }
            Section {
                Text(L("scope")); Text(L("experimental"))
                Link(L("community"),destination:URL(string:"https://www.facebook.com/groups/1088749137864406/")!)
                Text(L("credits"))
            }.font(.footnote)
        }.navigationTitle("net-merge") }
        .onReceive(timer) { _ in model.refresh() }
        .sheet(item:$editing) { c in LimitsView(connection:c,model:model) }
        .alert(L("reset"),isPresented:Binding(get:{resetting != nil},set:{if !$0 {resetting=nil}})) {
            Button(L("confirm"),role:.destructive) { if let c = resetting {model.send(["action":"reset","id":c.id])}; resetting=nil }
            Button(L("cancel"),role:.cancel) {resetting=nil}
        } message: { Text(L("reset_confirm")) }
        .alert(L("error"),isPresented:Binding(get:{!model.error.isEmpty},set:{if !$0 {model.error=""}})) { Button(L("cancel"),role:.cancel) {model.error=""} } message: {Text(ltr(model.error))}
    }
}
struct LimitsView: View {
    let connection: Connection
    @ObservedObject var model: VPNModel
    @Environment(\.dismiss) private var dismiss
    @State private var rate = ""
    @State private var quota = ""
    @State private var rateUnlimited = true
    @State private var quotaUnlimited = true
    var body: some View {
        NavigationStack { Form {
            Text(ltr(connection.name))
            Section(L("speed")) { Toggle(L("unlimited"),isOn:$rateUnlimited); if !rateUnlimited {TextField(L("speed"),text:$rate).keyboardType(.decimalPad).environment(\.layoutDirection,.leftToRight)} }
            Section(L("allowance")) { Toggle(L("unlimited"),isOn:$quotaUnlimited); if !quotaUnlimited {TextField(L("allowance"),text:$quota).keyboardType(.decimalPad).environment(\.layoutDirection,.leftToRight)} }
            Button(L("save")) { model.send(["action":"limits","id":connection.id,"speed":rateUnlimited ? "unlimited" : rate,"quota":quotaUnlimited ? "unlimited" : quota]); dismiss() }
            Button(L("cancel")) { dismiss() }
        }.navigationTitle(L("limits")) }
        .onAppear {rateUnlimited=connection.limitMbps==0;quotaUnlimited=connection.quotaBytes==0;rate=String(connection.limitMbps);quota=String(Double(connection.quotaBytes)/1e9)}
    }
}
