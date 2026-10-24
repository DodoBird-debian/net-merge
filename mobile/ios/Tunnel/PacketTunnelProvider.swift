import NetworkExtension
import Netmerge

final class PacketTunnelProvider: NEPacketTunnelProvider {
    private var engine: NetmergePacketCore?
    private var paused = false
    private let output = DispatchQueue(label:"net-merge.packets")

    override func startTunnel(options: [String: NSObject]?, completionHandler: @escaping (Error?) -> Void) {
        let settings = NEPacketTunnelNetworkSettings(tunnelRemoteAddress:"198.18.0.1")
        let v4 = NEIPv4Settings(addresses:["198.18.0.1"],subnetMasks:["255.255.255.0"])
        v4.includedRoutes = [NEIPv4Route.default()]
        let v6 = NEIPv6Settings(addresses:["fd6e:6d65:7267::1"],networkPrefixLengths:[64])
        v6.includedRoutes = [NEIPv6Route.default()]
        settings.ipv4Settings = v4; settings.ipv6Settings = v6
        settings.dnsSettings = NEDNSSettings(servers:["1.1.1.1","2606:4700:4700::1111"])
        settings.mtu = 1500
        setTunnelNetworkSettings(settings) { [weak self] error in
            guard let self = self else { return }
            if let error = error { completionHandler(error); return }
            do {
                guard let dir = FileManager.default.containerURL(forSecurityApplicationGroupIdentifier:appGroup) else { throw NSError(domain:"net-merge",code:1,userInfo:[NSLocalizedDescriptionKey:L("error")]) }
                self.engine = try NetmergeNewPacketCore(dir.path, language)
                guard let engine = self.engine else { throw NSError(domain:"net-merge",code:2) }
                self.readPackets()
                self.output.async { [weak self] in
                    while let data = engine.nextPacket(), !data.isEmpty {
                        let family = (data.first! >> 4) == 6 ? AF_INET6 : AF_INET
                        self?.packetFlow.writePackets([data],withProtocols:[NSNumber(value:family)])
                    }
                }
                completionHandler(nil)
            } catch { completionHandler(error) }
        }
    }
    private func readPackets() {
        packetFlow.readPackets { [weak self] packets, _ in
            guard let self = self, let engine = self.engine else { return }
            for packet in packets { engine.feedPacket(packet) }
            self.readPackets()
        }
    }
    override func stopTunnel(with reason: NEProviderStopReason, completionHandler: @escaping () -> Void) {
        engine?.close(); engine = nil; completionHandler()
    }
    override func handleAppMessage(_ messageData: Data, completionHandler: ((Data?) -> Void)?) {
        var diagnostic = ""
        do {
            let msg = try JSONSerialization.jsonObject(with:messageData) as? [String:String] ?? [:]
            switch msg["action"] {
            case "limits": try NetmergeSetConnectionLimits(msg["id"] ?? "",msg["speed"] ?? "",msg["quota"] ?? "")
            case "reset": try NetmergeResetQuota(msg["id"] ?? "")
            case "pause": paused = msg["paused"] == "true"; NetmergePause(paused)
            case "language": _ = try NetmergeConfigure(FileManager.default.containerURL(forSecurityApplicationGroupIdentifier:appGroup)!.path,language)
            default: break
            }
        } catch { diagnostic = error.localizedDescription }
        if diagnostic.isEmpty { diagnostic = NetmergeStorageError() }
        let stats = (try? JSONSerialization.jsonObject(with:Data(NetmergeStats().utf8))) ?? []
        completionHandler?(try? JSONSerialization.data(withJSONObject:["stats":stats,"error":diagnostic,"paused":paused]))
    }
}
