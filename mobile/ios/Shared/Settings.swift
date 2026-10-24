import Foundation

let appGroup = "group.org.iraqpc.netmerge"
let tunnelID = "org.iraqpc.netmerge.ios.tunnel"
let preferences = UserDefaults(suiteName: appGroup)!
var language: String {
    get { preferences.string(forKey: "language") ?? (Locale.preferredLanguages.first?.hasPrefix("ar") == true ? "ar" : "en") }
    set { preferences.set(newValue, forKey: "language") }
}
func L(_ key: String) -> String {
    guard let path = Bundle.main.path(forResource: language, ofType: "lproj"), let bundle = Bundle(path: path) else { return key }
    return bundle.localizedString(forKey: key, value: key, table: nil)
}
func GB(_ bytes: Int64) -> String { String(format: "%.2f GB", locale: Locale(identifier:"en_US_POSIX"), Double(bytes)/1e9) }
func speed(_ bytes: Double) -> String { String(format: "%.2f Mbps", locale: Locale(identifier:"en_US_POSIX"), bytes*8/1e6) }
func ltr(_ text: String) -> String { "\u{2066}\(text)\u{2069}" }
struct Connection: Codable, Identifiable {
    let id, name, state, state6: String
    let present, quotaExhausted: Bool
    let sessionBytes, quotaBytes, quotaUsedBytes, quotaRemainingBytes: Int64
    let rateIn, rateOut, limitMbps: Double
}
struct Response: Codable { let stats: [Connection]; let error: String; let paused: Bool }
