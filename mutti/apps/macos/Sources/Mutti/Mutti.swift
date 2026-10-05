// SPDX-License-Identifier: GPL-2.0-or-later
import AppKit
import SwiftUI
import WebKit
import Darwin

@main
struct MuttiApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var delegate
    var body: some Scene {
        Window("Mutti", id: "main") {
            ContentView(server: delegate.server).frame(minWidth: 820, minHeight: 660)
        }.defaultSize(width: 1080, height: 820)
        .commands { CommandGroup(replacing: .newItem) {} }
        MenuBarExtra("Mutti", systemImage: "externaldrive.fill") { MuttiMenu() }
    }
}

struct MuttiMenu: View {
    @Environment(\.openWindow) private var openWindow
    var body: some View {
        Button("Mutti öffnen") { openWindow(id: "main"); NSApp.activate(ignoringOtherApps: true) }
        Divider()
        Button("Mutti beenden") { NSApp.terminate(nil) }
    }
}

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    let server = ServerController()
    func applicationDidFinishLaunching(_ notification: Notification) { server.start() }
    func applicationWillTerminate(_ notification: Notification) { server.stop() }
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { false }
}

@MainActor
final class ServerController: ObservableObject {
    @Published var ready = false
    @Published private(set) var setupCompleted = false
    @Published var showOnboarding = true
    @Published var importEntry = false
    @Published private(set) var importing = false
    @Published var error: String?
    @Published var starting = false
    let address = URL(string: "http://127.0.0.1:18596/web/")!
    let onboardingAddress = URL(string: "http://127.0.0.1:18594/")!
    let connectAddress = URL(string: "http://127.0.0.1:18595/")!
    private var child: Process?
    private var log: FileHandle?
    private var readiness: Task<Void, Never>?
    private var lockFD: Int32 = -1
    private var stopping = false
    private var launchID: UUID?
    private(set) var dataDirectory: URL?
    private(set) var nativeImportClient: NativeImportClient?
    private struct ManagerState: Decodable {
        var ready: Bool
        var setupComplete: Bool
        var newSetup: Bool
        var phase: String
        var message: String
        var active: String
    }
    func start() {
        guard child == nil, !starting else { return }
        error = nil; starting = true; stopping = false; setupCompleted = false; showOnboarding = true
        let identifier = UUID(); launchID = identifier
        do {
            let fm = FileManager.default
            let root = try fm.url(for: .applicationSupportDirectory, in: .userDomainMask, appropriateFor: nil, create: true).appending(path: "Mutti Preview", directoryHint: .isDirectory)
            try fm.createDirectory(at: root, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
            dataDirectory = root
            lockFD = open(root.appending(path: "server.lock").path, O_CREAT | O_RDWR, 0o600)
            guard lockFD >= 0, flock(lockFD, LOCK_EX | LOCK_NB) == 0 else { throw Failure("Mutti läuft bereits. Öffne die vorhandene App.") }
            // Refuse occupied listeners before handing lifecycle ownership to the manager.
            for port in [18594, 18596] {
                let probe = socket(AF_INET, SOCK_STREAM, 0)
                guard probe >= 0 else { throw Failure("Der lokale Serverzugang konnte nicht vorbereitet werden.") }
                var addr = sockaddr_in(); addr.sin_len = UInt8(MemoryLayout<sockaddr_in>.size); addr.sin_family = sa_family_t(AF_INET); addr.sin_port = UInt16(port).bigEndian; addr.sin_addr.s_addr = inet_addr("127.0.0.1")
                let available = withUnsafePointer(to: &addr) { p in p.withMemoryRebound(to: sockaddr.self, capacity: 1) { bind(probe, $0, socklen_t(MemoryLayout<sockaddr_in>.size)) == 0 } }
                close(probe)
                guard available else { throw Failure("Der lokale Zugang ist belegt. Beende eine andere Mutti-Instanz und versuche es erneut.") }
            }
            guard let resources = Bundle.main.resourceURL else { throw Failure("Das App-Paket ist unvollständig.") }
            for path in ["server/jellyfin", "ffmpeg/ffmpeg", "migrate/mutti-migrate", "connect/mutti-connect"] {
                guard fm.isExecutableFile(atPath: resources.appending(path: path).path) else { throw Failure("Das App-Paket ist unvollständig. Bitte Mutti erneut installieren.") }
            }
            try fm.createDirectory(at: root.appending(path: "logs"), withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
            let logURL = root.appending(path: "logs/launcher.log")
            if fm.fileExists(atPath: logURL.path) {
                let previous = root.appending(path: "logs/launcher.previous.log")
                try? fm.removeItem(at: previous); try fm.moveItem(at: logURL, to: previous)
            }
            fm.createFile(atPath: logURL.path, contents: nil, attributes: [.posixPermissions: 0o600])
            log = try FileHandle(forWritingTo: logURL)
            let process = Process(); process.executableURL = resources.appending(path: "migrate/mutti-migrate")
            process.arguments = ["--root", root.path, "--server", resources.appending(path: "server/jellyfin").path, "--web", resources.appending(path: "web").path, "--ffmpeg", resources.appending(path: "ffmpeg/ffmpeg").path, "--connect", resources.appending(path: "connect/mutti-connect").path]
            let importClient = try NativeImportClient()
            try importClient.attach(to: process)
            process.arguments?.append("--native-owner-stdin")
            nativeImportClient = importClient
            process.standardOutput = log; process.standardError = log
            process.terminationHandler = { [weak self] _ in Task { @MainActor [weak self] in
                guard let self, self.launchID == identifier, !self.stopping else { return }
                self.ready = false; self.setupCompleted = false; self.starting = false; self.child = nil; self.nativeImportClient = nil; self.readiness?.cancel(); self.releaseLock()
                self.error = "Mutti wurde beendet. Du kannst den Server erneut starten. Details stehen im lokalen Protokoll."
            } }
            try process.run(); child = process
            readiness = Task { [weak self] in
                var observed = false
                var attempts = 0
                while !Task.isCancelled && process.isRunning {
                    guard let self else { return }
                    var request = URLRequest(url: self.onboardingAddress.appending(path: "api/state"), cachePolicy: .reloadIgnoringLocalCacheData); request.timeoutInterval = 3
                    if let (data, response) = try? await URLSession.shared.data(for: request),
                       (response as? HTTPURLResponse)?.statusCode == 200,
                       let state = try? JSONDecoder().decode(ManagerState.self, from: data) {
                        self.importing = ["checking", "backup", "importing", "verifying", "activating"].contains(state.phase)
                        self.ready = state.ready || self.importing
                        self.setupCompleted = state.setupComplete && !self.importing
                        self.dataDirectory = URL(fileURLWithPath: state.active, isDirectory: true)
                        if !observed && state.ready {
                            self.showOnboarding = !state.setupComplete && !state.newSetup
                            observed = true
                        }
                        if state.newSetup { self.showOnboarding = false }
                        if self.ready { self.starting = false; self.error = nil }
                        else if state.phase == "error" { self.starting = false; self.error = state.message }
                    }
                    attempts += 1
                    if !observed && attempts > 90 {
                        self.stop(); self.error = "Der Start dauert zu lange. Prüfe das lokale Protokoll und versuche es erneut."; return
                    }
                    try? await Task.sleep(for: .seconds(1))
                }
            }
        } catch { stop(); self.error = error.localizedDescription }
    }
    func restart() { stop(); start() }
    func stop() {
        stopping = true; launchID = nil; readiness?.cancel(); readiness = nil
        if let child, child.isRunning {
            child.terminate()
            // The manager first cancels staging, then stops Connect and Jellyfin.
            let deadline = Date().addingTimeInterval(35)
            while child.isRunning && Date() < deadline { Thread.sleep(forTimeInterval: 0.05) }
            if child.isRunning { kill(child.processIdentifier, SIGKILL); child.waitUntilExit() }
        }
        child = nil; nativeImportClient = nil; try? log?.close(); log = nil; ready = false; setupCompleted = false; starting = false; releaseLock()
    }
    func saveConnectSettings(_ settings: ConnectSettings) throws {
        guard ready && setupCompleted else { throw Failure("Bitte zuerst die Einrichtung abschließen.") }
        guard let root = dataDirectory else { return }
        if !settings.broker.isEmpty {
            guard let url = URL(string: settings.broker), url.scheme == "https", url.host != nil,
                  url.user == nil, url.query == nil, url.fragment == nil, url.path.isEmpty || url.path == "/"
            else { throw Failure("Bitte die HTTPS-Adresse des Vermittlungsdiensts eintragen.") }
        }
        if !settings.stun.isEmpty && (!settings.stun.hasPrefix("stun:") || settings.stun.contains("@")) {
            throw Failure("Bitte eine STUN-Adresse verwenden. Relay ist nicht aktiviert.")
        }
        let path = root.appending(path: "connect-settings.json")
        try JSONEncoder().encode(settings).write(to: path, options: .atomic)
        try FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: path.path)
    }
    func releaseLock() { if lockFD >= 0 { flock(lockFD, LOCK_UN); close(lockFD); lockFD = -1 } }
    struct Failure: LocalizedError { let message: String; init(_ message: String) { self.message = NSLocalizedString(message, comment: "Mutti server status") }; var errorDescription: String? { message } }
}

struct ContentView: View {
    @ObservedObject var server: ServerController
    @State private var devices = false
    @State private var settings = false
    var body: some View {
        VStack(spacing: 0) {
            HStack(spacing: 12) {
                Image(nsImage: NSApp.applicationIconImage).resizable().frame(width: 24, height: 24).accessibilityHidden(true)
                Text("Mutti").font(.headline)
                Spacer()
                if server.ready && server.setupCompleted {
                    Button(devices || server.showOnboarding ? "Bibliothek" : "Geräte koppeln") {
                        if server.showOnboarding { server.showOnboarding = false; devices = false } else { devices.toggle() }
                    }
                    if !server.showOnboarding { Button("Jellyfin übernehmen") { devices = false; server.importEntry = true; server.showOnboarding = true } }
                    Button("Fernzugriff") { settings = true }
                }
                Label(LocalizedStringKey(server.ready ? (server.setupCompleted ? "Auf diesem Mac bereit" : "Einrichtung läuft") : "Lokale Vorschau"), systemImage: server.ready && server.setupCompleted ? "checkmark.circle.fill" : "circle").font(.caption)
                Button { if let root = server.dataDirectory { NSWorkspace.shared.open(root.appending(path: "logs")) } } label: { Image(systemName: "doc.text.magnifyingglass") }.help("Lokale Protokolle öffnen")
            }.padding().background(Color(red: 0.12, green: 0.12, blue: 0.12)).foregroundStyle(.white)
            if server.ready {
                if let error = server.error { Text(error).padding().foregroundStyle(.orange) }
                let destination = devices ? server.connectAddress : (server.showOnboarding ? (server.importEntry ? URL(string: "http://127.0.0.1:18594/#import")! : server.onboardingAddress) : server.address)
                AdminView(address: destination, nativeImportClient: server.nativeImportClient).id(destination)
            }
            else {
                VStack(spacing: 24) {
                    Text("Deine Medien.\nGut zu Hause.").font(.system(size: 38, weight: .semibold)).multilineTextAlignment(.center)
                    if let error = server.error {
                        Text(error).multilineTextAlignment(.center).frame(maxWidth: 480)
                        Button("Erneut starten", action: server.restart).buttonStyle(.borderedProminent)
                    } else { ProgressView("Mutti wird gestartet …") }
                }.frame(maxWidth: .infinity, maxHeight: .infinity).padding(32)
            }
        }.tint(Color(red: 0.55, green: 0.49, blue: 0))
        .sheet(isPresented: $settings) { ConnectSettingsView(server: server) }
        .onChange(of: server.setupCompleted) { _, completed in
            if !completed { devices = false; settings = false }
        }
    }
}

struct AdminView: NSViewRepresentable {
    let address: URL
    let nativeImportClient: NativeImportClient?
    func makeCoordinator() -> Coordinator { Coordinator(address: address, nativeImportClient: nativeImportClient) }
    func makeNSView(context: Context) -> WKWebView {
        let configuration = WKWebViewConfiguration()
        configuration.userContentController.addScriptMessageHandler(context.coordinator, contentWorld: .page, name: "muttiFolder")
        configuration.userContentController.addScriptMessageHandler(context.coordinator, contentWorld: .page, name: "muttiImport")
        let view = WKWebView(frame: .zero, configuration: configuration)
        view.navigationDelegate = context.coordinator; view.load(URLRequest(url: address)); return view
    }
    func updateNSView(_ view: WKWebView, context: Context) {}
    @MainActor final class Coordinator: NSObject, WKNavigationDelegate, WKScriptMessageHandlerWithReply {
        let address: URL
        let nativeImportClient: NativeImportClient?
        init(address: URL, nativeImportClient: NativeImportClient?) { self.address = address; self.nativeImportClient = nativeImportClient }
        func isLocal(_ url: URL?) -> Bool {
            guard let url, url.scheme == "http", url.host == "127.0.0.1", url.user == nil else { return false }
            // Onboarding can enter the Jellyfin setup wizard; pairing stays in its own origin.
            return address.port == 18595 ? url.port == 18595 : [18594, 18596].contains(url.port ?? 0)
        }
        func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction, decisionHandler: @escaping @MainActor @Sendable (WKNavigationActionPolicy) -> Void) {
            if isLocal(action.request.url) { decisionHandler(.allow) }
            else { decisionHandler(.cancel); if action.navigationType == .linkActivated, let url = action.request.url, url.scheme == "https" { NSWorkspace.shared.open(url) } }
        }
        func userContentController(_ userContentController: WKUserContentController, didReceive message: WKScriptMessage, replyHandler: @escaping @MainActor @Sendable (Any?, String?) -> Void) {
            if message.name == "muttiImport" {
                guard NativeImportClient.accepts(message.frameInfo.request.url, isMainFrame: message.frameInfo.isMainFrame),
                      NativeImportClient.accepts(message.webView?.url, isMainFrame: true),
                      let client = nativeImportClient, let body = message.body as? [String: Any] else {
                    replyHandler(nil, "Import access is unavailable for this page."); return
                }
                Task { @MainActor in
                    do {
                        if body["action"] as? String == "available" { replyHandler(try await client.available(), nil) }
                        else if body["action"] as? String == "start", let input = body["input"] as? [String: Any], let window = message.webView?.window {
                            let accepted = try await client.start(input: input, window: window)
                            replyHandler(["cancelled": !accepted], nil)
                        } else { replyHandler(nil, "Unknown import action.") }
                    } catch { replyHandler(nil, error.localizedDescription) }
                }
                return
            }
            guard message.frameInfo.isMainFrame, isLocal(message.frameInfo.request.url), message.name == "muttiFolder", let window = message.webView?.window else { replyHandler(nil, "Folder access is unavailable for this page."); return }
            let panel = NSOpenPanel(); panel.canChooseFiles = false; panel.canChooseDirectories = true; panel.allowsMultipleSelection = false; panel.prompt = NSLocalizedString("Medienordner wählen", comment: "Native folder picker")
            panel.beginSheetModal(for: window) { response in replyHandler(response == .OK ? panel.url?.path : nil, nil) }
        }
    }
}

struct ConnectSettings: Codable {
    var broker = ""
    var stun = ""
}

struct ConnectSettingsView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var server: ServerController
    @State private var settings = ConnectSettings()
    @State private var error: String?
    var body: some View {
        VStack(alignment: .leading, spacing: 18) {
            Text("Direkter Fernzugriff").font(.title2.bold())
            Text("Im Heimnetz kannst du sofort koppeln. Für den Zugriff unterwegs benötigt diese Testversion einen erreichbaren Vermittlungsdienst. Er überträgt Verbindungsdaten; deine Medien bleiben direkt zwischen Mutti und kurtz.")
            TextField("HTTPS-Adresse des Vermittlungsdiensts", text: $settings.broker)
            TextField("STUN-Adresse, zum Beispiel stun:connect.example.org:3478", text: $settings.stun)
            Text("Leer lassen für die Heimnetz-Kopplung. Nach einer Änderung bitte die Geräte neu koppeln. Laufende Verbindungen werden beendet.").font(.caption).foregroundStyle(.secondary)
            if let error { Text(error).foregroundStyle(.orange) }
            HStack {
                Button("Abbrechen") { dismiss() }
                Spacer()
                Button("Speichern") {
                    do { try server.saveConnectSettings(settings); dismiss() }
                    catch { self.error = error.localizedDescription }
                }.buttonStyle(.borderedProminent)
            }
        }.padding(28).frame(width: 580)
        .onAppear {
            if let root = server.dataDirectory,
               let data = try? Data(contentsOf: root.appending(path: "connect-settings.json")),
               let saved = try? JSONDecoder().decode(ConnectSettings.self, from: data) { settings = saved }
        }
    }
}
