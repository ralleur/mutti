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
    @Published var error: String?
    @Published var starting = false
    let address = URL(string: "http://127.0.0.1:18596/web/")!
    private var child: Process?
    private var log: FileHandle?
    private var readiness: Task<Void, Never>?
    private var lockFD: Int32 = -1
    private var stopping = false
    private var launchID: UUID?
    private(set) var dataDirectory: URL?

    func start() {
        guard child == nil, !starting else { return }
        error = nil; starting = true; stopping = false
        let identifier = UUID(); launchID = identifier
        do {
            let fm = FileManager.default
            // A separate preview store never opens an existing Jellyfin database.
            let root = try fm.url(for: .applicationSupportDirectory, in: .userDomainMask, appropriateFor: nil, create: true).appending(path: "Mutti Preview", directoryHint: .isDirectory)
            try fm.createDirectory(at: root, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
            dataDirectory = root
            lockFD = open(root.appending(path: "server.lock").path, O_CREAT | O_RDWR, 0o600)
            guard lockFD >= 0, flock(lockFD, LOCK_EX | LOCK_NB) == 0 else { throw Failure("Mutti läuft bereits. Öffne die vorhandene App.") }
            let probe = socket(AF_INET, SOCK_STREAM, 0)
            guard probe >= 0 else { throw Failure("Der lokale Serverzugang konnte nicht vorbereitet werden.") }
            var addr = sockaddr_in(); addr.sin_len = UInt8(MemoryLayout<sockaddr_in>.size); addr.sin_family = sa_family_t(AF_INET); addr.sin_port = UInt16(18596).bigEndian; addr.sin_addr.s_addr = inet_addr("127.0.0.1")
            let available = withUnsafePointer(to: &addr) { p in p.withMemoryRebound(to: sockaddr.self, capacity: 1) { bind(probe, $0, socklen_t(MemoryLayout<sockaddr_in>.size)) == 0 } }
            close(probe)
            guard available else { throw Failure("Der lokale Zugang ist belegt. Beende eine andere Mutti-Instanz und versuche es erneut.") }
            let config = root.appending(path: "config", directoryHint: .isDirectory)
            for dir in [config, root.appending(path: "cache"), root.appending(path: "logs")] { try fm.createDirectory(at: dir, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700]) }
            // Enforce the preview boundary on every start, retaining other preferences.
            let networkURL = config.appending(path: "network.xml")
            let network = fm.fileExists(atPath: networkURL.path) ? try XMLDocument(contentsOf: networkURL) : XMLDocument(rootElement: XMLElement(name: "NetworkConfiguration"))
            guard let element = network.rootElement() else { throw Failure("Die Netzwerkeinstellungen konnten nicht gelesen werden.") }
            for key in ["InternalHttpPort", "PublicHttpPort", "EnableRemoteAccess", "AutoDiscovery", "EnableIPv6", "LocalNetworkAddresses"] { element.elements(forName: key).forEach { $0.detach() } }
            for (key, value) in [("InternalHttpPort", "18596"), ("PublicHttpPort", "18596"), ("EnableRemoteAccess", "false"), ("AutoDiscovery", "false"), ("EnableIPv6", "false")] { element.addChild(XMLElement(name: key, stringValue: value)) }
            let interfaces = XMLElement(name: "LocalNetworkAddresses"); interfaces.addChild(XMLElement(name: "string", stringValue: "127.0.0.1")); element.addChild(interfaces)
            try network.xmlData(options: .nodePrettyPrint).write(to: networkURL, options: .atomic)
            let system = config.appending(path: "system.xml")
            if !fm.fileExists(atPath: system.path) { try Data("<ServerConfiguration><ServerName>Mutti</ServerName></ServerConfiguration>".utf8).write(to: system, options: .atomic) }
            guard let resources = Bundle.main.resourceURL else { throw Failure("Das App-Paket ist unvollständig.") }
            let executable = resources.appending(path: "server/jellyfin")
            guard fm.isExecutableFile(atPath: executable.path), fm.fileExists(atPath: resources.appending(path: "web/index.html").path), fm.isExecutableFile(atPath: resources.appending(path: "ffmpeg/ffmpeg").path) else { throw Failure("Das App-Paket ist unvollständig. Bitte baue oder installiere Mutti erneut.") }
            let logURL = root.appending(path: "logs/launcher.log")
            // Keep one previous launcher log; it is never uploaded automatically.
            if fm.fileExists(atPath: logURL.path) {
                let previous = root.appending(path: "logs/launcher.previous.log")
                try? fm.removeItem(at: previous); try fm.moveItem(at: logURL, to: previous)
            }
            fm.createFile(atPath: logURL.path, contents: nil, attributes: [.posixPermissions: 0o600])
            log = try FileHandle(forWritingTo: logURL)
            let process = Process(); process.executableURL = executable
            process.arguments = ["--datadir", root.appending(path: "data").path, "--configdir", config.path, "--cachedir", root.appending(path: "cache").path, "--logdir", root.appending(path: "logs").path, "--webdir", resources.appending(path: "web").path, "--ffmpeg", resources.appending(path: "ffmpeg/ffmpeg").path, "--package-name", "mutti-preview"]
            var env = ProcessInfo.processInfo.environment; env["DOTNET_CLI_TELEMETRY_OPTOUT"] = "1"; env["MUTTI_LOCAL_ONLY"] = "1"; env["MUTTI_PREVIEW_ORIGIN"] = "http://127.0.0.1:18596"; process.environment = env
            process.standardOutput = log; process.standardError = log
            process.terminationHandler = { [weak self] _ in Task { @MainActor [weak self] in
                guard let self, self.launchID == identifier, !self.stopping else { return }
                self.ready = false; self.starting = false; self.child = nil; self.readiness?.cancel(); self.releaseLock()
                self.error = NSLocalizedString("Mutti wurde beendet. Du kannst den Server erneut starten. Details stehen im lokalen Protokoll.", comment: "Server stopped")
            } }
            try process.run(); child = process
            readiness = Task { [weak self] in
                for _ in 0..<90 {
                    guard let self, !Task.isCancelled, process.isRunning else { return }
                    var request = URLRequest(url: self.address.deletingLastPathComponent().appending(path: "health")); request.timeoutInterval = 2
                    if let (_, response) = try? await URLSession.shared.data(for: request), let http = response as? HTTPURLResponse, http.statusCode == 200, process.isRunning {
                        self.ready = true; self.starting = false; return
                    }
                    try? await Task.sleep(for: .seconds(1))
                }
                guard let self, !Task.isCancelled else { return }
                self.stop(); self.error = NSLocalizedString("Der Start dauert zu lange. Prüfe das Protokoll und versuche es erneut.", comment: "Startup timeout")
            }
        } catch { stop(); self.error = error.localizedDescription }
    }

    func stop() {
        stopping = true; launchID = nil; readiness?.cancel(); readiness = nil
        if let child, child.isRunning {
            child.terminate()
            let deadline = Date().addingTimeInterval(8)
            while child.isRunning && Date() < deadline { Thread.sleep(forTimeInterval: 0.05) }
            if child.isRunning { kill(child.processIdentifier, SIGKILL); child.waitUntilExit() }
        }
        child = nil; try? log?.close(); log = nil; ready = false; starting = false; releaseLock()
    }
    func releaseLock() { if lockFD >= 0 { flock(lockFD, LOCK_UN); close(lockFD); lockFD = -1 } }
    struct Failure: LocalizedError { let message: String; init(_ message: String) { self.message = NSLocalizedString(message, comment: "Mutti server status") }; var errorDescription: String? { message } }
}

struct ContentView: View {
    @ObservedObject var server: ServerController
    var body: some View {
        VStack(spacing: 0) {
            HStack(spacing: 12) {
                Image(nsImage: NSApp.applicationIconImage).resizable().frame(width: 24, height: 24).accessibilityHidden(true)
                Text("Mutti").font(.headline)
                Spacer()
                Label(LocalizedStringKey(server.ready ? "Auf diesem Mac bereit" : "Lokale Vorschau"), systemImage: server.ready ? "checkmark.circle.fill" : "circle").font(.caption)
                Button { if let root = server.dataDirectory { NSWorkspace.shared.open(root.appending(path: "logs")) } } label: { Image(systemName: "doc.text.magnifyingglass") }.help("Lokale Protokolle öffnen")
            }.padding().background(Color(red: 0.12, green: 0.12, blue: 0.12)).foregroundStyle(.white)
            if server.ready { AdminView(address: server.address) }
            else {
                VStack(spacing: 24) {
                    Text("Deine Medien.\nGut zu Hause.").font(.system(size: 38, weight: .semibold)).multilineTextAlignment(.center)
                    if let error = server.error {
                        Text(error).multilineTextAlignment(.center).frame(maxWidth: 480)
                        Button("Erneut starten", action: server.start).buttonStyle(.borderedProminent)
                    } else { ProgressView("Mutti wird gestartet …") }
                }.frame(maxWidth: .infinity, maxHeight: .infinity).padding(32)
            }
        }.tint(Color(red: 0.55, green: 0.49, blue: 0))
    }
}

struct AdminView: NSViewRepresentable {
    let address: URL
    func makeCoordinator() -> Coordinator { Coordinator(address: address) }
    func makeNSView(context: Context) -> WKWebView {
        let configuration = WKWebViewConfiguration()
        configuration.userContentController.addScriptMessageHandler(context.coordinator, contentWorld: .page, name: "muttiFolder")
        let view = WKWebView(frame: .zero, configuration: configuration)
        view.navigationDelegate = context.coordinator; view.load(URLRequest(url: address)); return view
    }
    func updateNSView(_ view: WKWebView, context: Context) {}
    @MainActor final class Coordinator: NSObject, WKNavigationDelegate, WKScriptMessageHandlerWithReply {
        let address: URL
        init(address: URL) { self.address = address }
        func isLocal(_ url: URL?) -> Bool { url?.scheme == address.scheme && url?.host == address.host && url?.port == address.port }
        func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction, decisionHandler: @escaping @MainActor @Sendable (WKNavigationActionPolicy) -> Void) {
            if isLocal(action.request.url) { decisionHandler(.allow) }
            else { decisionHandler(.cancel); if action.navigationType == .linkActivated, let url = action.request.url, url.scheme == "https" { NSWorkspace.shared.open(url) } }
        }
        func userContentController(_ userContentController: WKUserContentController, didReceive message: WKScriptMessage, replyHandler: @escaping @MainActor @Sendable (Any?, String?) -> Void) {
            guard message.frameInfo.isMainFrame, isLocal(message.frameInfo.request.url), message.name == "muttiFolder", let window = message.webView?.window else { replyHandler(nil, "Folder access is unavailable for this page."); return }
            let panel = NSOpenPanel(); panel.canChooseFiles = false; panel.canChooseDirectories = true; panel.allowsMultipleSelection = false; panel.prompt = NSLocalizedString("Medienordner wählen", comment: "Native folder picker")
            panel.beginSheetModal(for: window) { response in replyHandler(response == .OK ? panel.url?.path : nil, nil) }
        }
    }
}
