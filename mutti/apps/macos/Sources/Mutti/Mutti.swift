// SPDX-License-Identifier: GPL-2.0-or-later
import AppKit
import CoreText
import ServiceManagement
import SwiftUI
import WebKit
import Darwin

@main
struct MuttiApp: App {
    init() {
        if let font = Bundle.main.url(forResource: "Sora-Bold", withExtension: "ttf") {
            CTFontManagerRegisterFontsForURL(font as CFURL, .process, nil)
        }
    }
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var delegate
    var body: some Scene {
        Window("Mutti", id: "main") {
            ContentView(server: delegate.server).frame(minWidth: 820, minHeight: 660)
        }.defaultSize(width: 1080, height: 820)
        .commands { CommandGroup(replacing: .newItem) {} }
        MenuBarExtra("Mutti", systemImage: "externaldrive.fill") { MuttiMenu(server: delegate.server) }
    }
}

struct MuttiMenu: View {
    @ObservedObject var server: ServerController
    @Environment(\.openWindow) private var openWindow
    var body: some View {
        // A plain Text inside a menu renders as a disabled status line.
        Text(LocalizedStringKey(server.statusText))
        Divider()
        Toggle("Bei der Anmeldung starten", isOn: $server.launchAtLoginSetting)
        Toggle("Mac wach halten, solange Mutti läuft", isOn: $server.keepAwake)
        if let settingsError = server.settingsError { Text(settingsError) }
        Divider()
        Button("Mutti öffnen") { openWindow(id: "main"); NSApp.activate(ignoringOtherApps: true) }
        Divider()
        // Quitting goes through the app delegate so the confirmation and the clean stop apply.
        Button("Mutti beenden") { NSApp.terminate(nil) }
    }
}

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    let server = ServerController()
    /// Set when the system announced a logout, restart or shutdown; the quit confirmation is skipped then.
    /// Nothing announces a cancelled logout, so the flag expires on its own after a while.
    private var poweringOff = false
    private var poweringOffExpiry: Task<Void, Never>?
    private static let poweringOffWindow: Duration = .seconds(90)
    func applicationDidFinishLaunching(_ notification: Notification) {
        NSWorkspace.shared.notificationCenter.addObserver(self, selector: #selector(workspaceWillPowerOff(_:)), name: NSWorkspace.willPowerOffNotification, object: nil)
        server.start()
    }
    /// Login items can also be changed in System Settings; pick that up when the user comes back.
    func applicationDidBecomeActive(_ notification: Notification) { server.refreshLaunchAtLogin() }
    @objc private func workspaceWillPowerOff(_ notification: Notification) {
        poweringOff = true
        poweringOffExpiry?.cancel()
        poweringOffExpiry = Task { @MainActor [weak self] in
            try? await Task.sleep(for: AppDelegate.poweringOffWindow)
            guard !Task.isCancelled else { return }
            self?.poweringOff = false
        }
    }
    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        if server.ready && server.setupCompleted && !poweringOff {
            NSApp.activate(ignoringOtherApps: true)
            let alert = NSAlert()
            alert.messageText = ServerController.localized("Mutti beenden?")
            alert.informativeText = ServerController.localized("Gekoppelte Geräte können Mutti nicht erreichen, solange die App beendet ist.")
            alert.addButton(withTitle: ServerController.localized("Beenden"))
            alert.addButton(withTitle: ServerController.localized("Abbrechen"))
            guard alert.runModal() == .alertFirstButtonReturn else { return .terminateCancel }
        }
        // Without a running manager the stop completes synchronously; reply(toApplicationShouldTerminate:)
        // must only be sent after .terminateLater was returned, so answer directly in that case.
        guard server.isManagerRunning else { server.stop(); return .terminateNow }
        server.stop { NSApp.reply(toApplicationShouldTerminate: true) }
        return .terminateLater
    }
    func applicationWillTerminate(_ notification: Notification) { server.stopWithoutWaiting() }
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { false }
}

/// Decoded from the manager's /api/state. The availability fields are optional so that an
/// older manager without them still decodes.
struct ManagerState: Decodable {
    var ready: Bool
    var setupComplete: Bool
    var newSetup: Bool
    var phase: String
    var message: String
    var active: String
    var restarts: Int?
    var connectState: String?
    var connectMessage: String?
    /// Set while the manager refuses a replaced package; Jellyfin is not relaunched then.
    var serviceMessage: String?
    var update: UpdateInfo?
    /// Jellyfin crashed and the manager relaunches it on its own: not ready, but not an error.
    var isRestarting: Bool { phase == "restarting" }
    var isError: Bool { phase == "error" }
    var connectFailed: Bool { connectState == "failed" }
    /// An update failed its checks; the owner may restore the backup taken before it.
    var isBlocked: Bool { phase == "update_blocked" }
}

/// Update guard state from /api/state: `pending` until the first start after an update is verified.
struct UpdateInfo: Decodable {
    var state: String
    var snapshot: String?
}

@MainActor
final class ServerController: ObservableObject {
    @Published var ready = false { didSet { updateActivity() } }
    @Published private(set) var setupCompleted = false
    @Published var showOnboarding = true
    @Published var importEntry = false
    @Published private(set) var importing = false
    @Published var error: String?
    @Published var starting = false
    /// Shown while the manager secures the data before an update.
    @Published private(set) var progressMessage: String?
    /// Set while an update is blocked and a matching pre-update backup exists.
    @Published private(set) var blockedSnapshot: String?
    /// The manager relaunches Jellyfin after a crash. Shown with the manager's message, without a restart button.
    @Published private(set) var restarting = false
    /// Manager message that accompanies a restart; nil otherwise.
    @Published private(set) var notice: String?
    /// Shown under the header while Connect failed to start; the library stays usable meanwhile.
    @Published private(set) var connectMessage: String?
    /// True from stop() until the manager process is gone and the cleanup ran.
    @Published private(set) var isStopping = false
    /// Explains a failed launch-at-login change. Kept apart from `error`, which the readiness poll owns
    /// and clears on every poll while the server is ready.
    @Published private(set) var settingsError: String?
    @Published private(set) var launchAtLogin: Bool
    @Published var keepAwake: Bool {
        didSet {
            UserDefaults.standard.set(keepAwake, forKey: ServerController.keepAwakeKey)
            updateActivity()
        }
    }
    let address = URL(string: "http://127.0.0.1:18596/web/")!
    let managementAddress = URL(string: "http://127.0.0.1:18596/web/#/mutti")!
    let onboardingAddress = URL(string: "http://127.0.0.1:18594/")!
    let connectAddress = URL(string: "http://127.0.0.1:18595/")!
    private static let keepAwakeKey = "MuttiKeepAwake"
    /// Polls (about one per second) after which a slow first start is announced, and after which it is abandoned.
    private static let slowStartPolls = 90
    private static let abandonStartPolls = 900
    private var child: Process?
    private var log: FileHandle?
    private var readiness: Task<Void, Never>?
    private var lockFD: Int32 = -1
    private var stopping = false
    /// Completion of the most recent stop request; runs once the in-flight stop has finished.
    private var pendingStopCompletion: (@MainActor @Sendable () -> Void)?
    private var launchID: UUID?
    private var activity: NSObjectProtocol?
    private(set) var dataDirectory: URL?
    private(set) var nativeImportClient: NativeImportClient?

    init() {
        keepAwake = UserDefaults.standard.object(forKey: ServerController.keepAwakeKey) as? Bool ?? true
        launchAtLogin = SMAppService.mainApp.status == .enabled
    }

    nonisolated static func localized(_ key: String) -> String { NSLocalizedString(key, comment: "Mutti server status") }

    /// True while a manager process exists, including during an asynchronous stop.
    var isManagerRunning: Bool { child?.isRunning == true }

    /// Key for the menu bar status line; localised by the view.
    var statusText: String {
        if isStopping { return "Wird beendet …" }
        if restarting { return "Server wird neu gestartet …" }
        if ready { return setupCompleted ? "Bereit" : "Einrichtung läuft" }
        if error != nil && !starting { return "Fehler" }
        return "Wird gestartet …"
    }

    /// Read-write view of the login item for a Toggle. The getter asks the system directly so the
    /// menu shows changes made in System Settings; writes go through setLaunchAtLogin.
    var launchAtLoginSetting: Bool {
        get { SMAppService.mainApp.status == .enabled }
        set { setLaunchAtLogin(newValue) }
    }

    func start() {
        guard child == nil, !starting else { return }
        error = nil; starting = true; stopping = false; setupCompleted = false; showOnboarding = true; restarting = false; notice = nil; connectMessage = nil
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
                // Match the manager's listener semantics: completed connections
                // in TIME_WAIT must not look like another running server.
                var reuse: Int32 = 1
                guard setsockopt(probe, SOL_SOCKET, SO_REUSEADDR, &reuse, socklen_t(MemoryLayout<Int32>.size)) == 0 else {
                    close(probe)
                    throw Failure("Der lokale Serverzugang konnte nicht vorbereitet werden.")
                }
                var addr = sockaddr_in(); addr.sin_len = UInt8(MemoryLayout<sockaddr_in>.size); addr.sin_family = sa_family_t(AF_INET); addr.sin_port = UInt16(port).bigEndian; addr.sin_addr.s_addr = inet_addr("127.0.0.1")
                let available = withUnsafePointer(to: &addr) { p in p.withMemoryRebound(to: sockaddr.self, capacity: 1) { bind(probe, $0, socklen_t(MemoryLayout<sockaddr_in>.size)) == 0 } }
                close(probe)
                guard available else { throw Failure("Der lokale Zugang ist belegt. Beende eine andere Mutti-Instanz und versuche es erneut.") }
            }
            guard let resources = Bundle.main.resourceURL else { throw Failure("Das App-Paket ist unvollständig.") }
            for path in ["server/jellyfin", "ffmpeg/ffmpeg", "migrate/mutti-migrate", "connect/mutti-connect"] {
                guard fm.isExecutableFile(atPath: resources.appending(path: path).path) else { throw Failure("Das App-Paket ist unvollständig. Bitte baue oder installiere Mutti erneut.") }
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
            process.arguments?.append(contentsOf: ["--intro-skipper", resources.appending(path: "intro-skipper").path])
            // Optional modules: photos, documents and local AI run in their own
            // process. A missing or failing module never blocks the media server.
            let hub = resources.appending(path: "hub/mutti-hub")
            if fm.isExecutableFile(atPath: hub.path) {
                process.arguments?.append(contentsOf: ["--hub", hub.path])
                let engine = resources.appending(path: "ai-engine/ollama")
                if fm.isExecutableFile(atPath: engine.path) { process.arguments?.append(contentsOf: ["--ollama", engine.path]) }
            }
            nativeImportClient = importClient
            process.standardOutput = log; process.standardError = log
            process.terminationHandler = { [weak self] _ in Task { @MainActor [weak self] in
                guard let self, self.launchID == identifier, !self.stopping else { return }
                self.ready = false; self.setupCompleted = false; self.starting = false; self.restarting = false; self.notice = nil; self.connectMessage = nil
                self.child = nil; self.nativeImportClient?.release(); self.nativeImportClient = nil; self.readiness?.cancel(); self.releaseLock()
                self.error = ServerController.localized("Mutti wurde beendet. Du kannst den Server erneut starten. Details stehen im lokalen Protokoll.")
            } }
            try process.run(); child = process
            readiness = Task { [weak self] in
                var observed = false
                // Polls towards the slow-start notice, and towards abandoning the start.
                var slowPolls = 0
                var attempts = 0
                while !Task.isCancelled && process.isRunning {
                    guard let self else { return }
                    // A pre-update backup or a rollback can take long; a blocked update waits for the owner.
                    var busy = false
                    var pending = false
                    var request = URLRequest(url: self.onboardingAddress.appending(path: "api/state"), cachePolicy: .reloadIgnoringLocalCacheData); request.timeoutInterval = 3
                    if let (data, response) = try? await URLSession.shared.data(for: request),
                       (response as? HTTPURLResponse)?.statusCode == 200,
                       let state = try? JSONDecoder().decode(ManagerState.self, from: data) {
                        self.importing = ["checking", "backup", "importing", "verifying", "activating"].contains(state.phase)
                        busy = state.phase == "update" || state.isBlocked
                        // The first start after an update may migrate Jellyfin's data for
                        // longer; the manager itself gives it 20 minutes.
                        pending = state.update?.state == "pending"
                        self.progressMessage = state.phase == "update" && !state.message.isEmpty ? state.message : nil
                        self.blockedSnapshot = state.isBlocked ? state.update?.snapshot : nil
                        self.restarting = state.isRestarting
                        self.ready = (state.ready || self.importing) && !state.isRestarting
                        self.setupCompleted = state.setupComplete && !self.importing
                        self.dataDirectory = URL(fileURLWithPath: state.active, isDirectory: true)
                        if !observed && state.ready {
                            self.showOnboarding = !state.setupComplete && !state.newSetup
                            observed = true
                        }
                        if state.newSetup { self.showOnboarding = false }
                        if self.ready { self.starting = false; self.error = nil }
                        // A backup or rollback runs: its progress replaces an earlier error.
                        else if state.phase == "update" { self.error = nil }
                        else if state.isError || state.isBlocked { self.starting = false; self.error = state.message }
                        else if let message = state.serviceMessage, !message.isEmpty { self.starting = false; self.error = message }
                        else if state.isRestarting { self.error = nil }
                        self.notice = state.isRestarting && !state.message.isEmpty ? state.message : nil
                        if state.connectFailed {
                            let message = state.connectMessage ?? ""
                            self.connectMessage = message.isEmpty ? ServerController.localized("Der Fernzugriff ist nicht verfügbar.") : message
                        } else { self.connectMessage = nil }
                    }
                    // A backup, a rollback or a blocked update does not count against the start. The first
                    // start after an update gets the notice but is never abandoned here.
                    slowPolls = busy ? 0 : slowPolls + 1
                    attempts = busy || pending ? 0 : attempts + 1
                    if !observed && !self.ready {
                        // A first start after an update may run database migrations for minutes:
                        // announce the delay, keep waiting, and only abandon the start much later.
                        if slowPolls >= ServerController.slowStartPolls && self.error == nil {
                            self.error = ServerController.localized("Der Start dauert länger als gewohnt. Nach einem Update kann die Bibliothek einige Minuten aktualisiert werden.")
                        }
                        if attempts > ServerController.abandonStartPolls {
                            self.stop(); self.error = ServerController.localized("Der Start dauert zu lange. Prüfe das lokale Protokoll und versuche es erneut."); return
                        }
                    }
                    try? await Task.sleep(for: .seconds(1))
                }
            }
        } catch { stop(); self.error = error.localizedDescription }
    }

    func restart() { stop { [weak self] in self?.start() } }

    /// Restores the backup taken before the update, after a native confirmation.
    func rollback(window: NSWindow?) {
        guard let snapshot = blockedSnapshot, let client = nativeImportClient else { return }
        Task { @MainActor in
            do {
                if try await client.rollback(snapshot: snapshot, window: window) {
                    self.blockedSnapshot = nil; self.error = nil; self.starting = true
                }
            } catch { self.error = error.localizedDescription }
        }
    }

    /// Stops the manager without blocking the main thread. The lifeline is released first (the
    /// manager treats EOF on stdin as "parent is gone" and shuts down gracefully), then SIGTERM is
    /// sent. A detached task polls the pid for up to 35 seconds; afterwards the manager's whole
    /// process group is killed. Cleanup and `completion` run on the main actor once the process
    /// is gone. Without a running manager the cleanup and `completion` run synchronously.
    ///
    /// A stop is single-flight: while one is in flight, a further call only replaces what happens
    /// afterwards (the latest request wins, so a quit after a restart does not launch a manager that
    /// is cut off right away). The cleanup therefore never runs twice and cannot tear down a manager
    /// that a completion launched in the meantime.
    func stop(completion: (@MainActor @Sendable () -> Void)? = nil) {
        stopping = true; launchID = nil; readiness?.cancel(); readiness = nil
        ready = false; setupCompleted = false; starting = false; restarting = false; notice = nil; connectMessage = nil
        nativeImportClient?.release()
        if isStopping { pendingStopCompletion = completion; return }
        guard let child, child.isRunning else {
            finishStop(); completion?(); return
        }
        isStopping = true
        pendingStopCompletion = completion
        let pid: pid_t = child.processIdentifier
        // The manager first cancels staging, then stops Connect and Jellyfin.
        kill(pid, SIGTERM)
        Task.detached(priority: .userInitiated) { [weak self] in
            var polls = 0
            while polls < 350, kill(pid, 0) == 0 {
                try? await Task.sleep(for: .milliseconds(100))
                polls += 1
            }
            if kill(pid, 0) == 0 {
                // The manager runs in its own process group; take Jellyfin, ffmpeg and Connect down with it
                // so nothing orphaned keeps the loopback ports.
                kill(-pid, SIGKILL)
                kill(pid, SIGKILL)
            }
            await MainActor.run { self?.completeStop() }
        }
    }

    /// Runs once the in-flight stop's process is gone: cleans up and hands over to the latest completion.
    private func completeStop() {
        finishStop()
        isStopping = false
        let completion = pendingStopCompletion
        pendingStopCompletion = nil
        completion?()
    }

    /// Best-effort fallback for applicationWillTerminate: asks the manager to shut down without waiting.
    func stopWithoutWaiting() {
        stopping = true; launchID = nil; readiness?.cancel(); readiness = nil
        nativeImportClient?.release()
        if let child, child.isRunning { kill(child.processIdentifier, SIGTERM) }
        endActivity(); releaseLock()
    }

    private func finishStop() {
        child = nil; nativeImportClient = nil; try? log?.close(); log = nil
        ready = false; setupCompleted = false; starting = false; restarting = false; notice = nil; connectMessage = nil
        endActivity(); releaseLock()
    }

    func setLaunchAtLogin(_ enabled: Bool) {
        settingsError = nil
        do {
            if enabled { try SMAppService.mainApp.register() } else { try SMAppService.mainApp.unregister() }
            if enabled && SMAppService.mainApp.status == .requiresApproval {
                // The user switched Mutti off under Login Items earlier; register() succeeds, but the
                // system keeps the item off until it is allowed there again.
                settingsError = ServerController.localized("Mutti muss in den Systemeinstellungen unter Anmeldeobjekte erlaubt werden.")
                SMAppService.openSystemSettingsLoginItems()
            }
        } catch {
            settingsError = ServerController.localized("Der Start bei der Anmeldung konnte nicht geändert werden. Prüfe die Anmeldeobjekte in den Systemeinstellungen.")
        }
        refreshLaunchAtLogin()
    }

    /// Re-reads the login item status; the user can change it in System Settings as well.
    func refreshLaunchAtLogin() {
        let enabled = SMAppService.mainApp.status == .enabled
        if launchAtLogin != enabled { launchAtLogin = enabled }
    }

    /// Holds a system activity while the server is ready and the user wants the Mac to stay awake.
    private func updateActivity() {
        let wanted = ready && keepAwake
        if wanted {
            guard activity == nil else { return }
            activity = ProcessInfo.processInfo.beginActivity(options: [.idleSystemSleepDisabled], reason: ServerController.localized("Mutti stellt Medien bereit"))
        } else { endActivity() }
    }

    private func endActivity() {
        guard let activity else { return }
        self.activity = nil
        ProcessInfo.processInfo.endActivity(activity)
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
    struct Failure: LocalizedError { let message: String; init(_ message: String) { self.message = ServerController.localized(message) }; var errorDescription: String? { message } }
}

struct ContentView: View {
    @ObservedObject var server: ServerController
    @State private var settings = false
    var body: some View {
        VStack(spacing: 0) {
            if server.showOnboarding || !server.setupCompleted {
            HStack(spacing: 12) {
                Image(nsImage: NSApp.applicationIconImage).resizable().frame(width: 24, height: 24).accessibilityHidden(true)
                if let url = Bundle.main.url(forResource: "wordmark-light", withExtension: "png"), let image = NSImage(contentsOf: url) {
                    Image(nsImage: image).resizable().aspectRatio(contentMode: .fit).frame(width: 64, height: 22).accessibilityLabel("Mutti")
                } else { Text("Mutti").font(.headline) }
                Spacer()
                if server.ready && server.setupCompleted {
                    Button("Zur Übersicht") { server.showOnboarding = false }
                }
                Label(LocalizedStringKey(server.ready ? (server.setupCompleted ? "Auf diesem Mac bereit" : "Einrichtung läuft") : "Lokale Vorschau"), systemImage: server.ready && server.setupCompleted ? "checkmark.circle.fill" : "circle").font(.caption)
                Button { if let root = server.dataDirectory { NSWorkspace.shared.open(root.appending(path: "logs")) } } label: { Image(systemName: "doc.text.magnifyingglass") }.help("Lokale Protokolle öffnen")
            }.padding().background(Color(red: 31/255, green: 31/255, blue: 31/255)).foregroundStyle(Color(red: 250/255, green: 248/255, blue: 241/255))
            }
            if server.ready {
                if let error = server.error { Text(error).padding().foregroundStyle(.orange) }
                if let connectMessage = server.connectMessage { Text(connectMessage).padding().foregroundStyle(.orange) }
                if let settingsError = server.settingsError { Text(settingsError).padding().foregroundStyle(.orange) }
                let destination = server.showOnboarding ? (server.importEntry ? URL(string: "http://127.0.0.1:18594/#import")! : server.onboardingAddress) : (server.setupCompleted ? server.managementAddress : server.address)
                AdminView(address: destination, nativeImportClient: server.nativeImportClient, onNavigate: { action in
                    if action == "import" { server.importEntry = true; server.showOnboarding = true }
                    if action == "remote" { settings = true }
                }).id(destination)
            }
            else {
                VStack(spacing: 24) {
                    Text("Deine Medien.\nGut zu Hause.").font(.custom("Sora-Bold", size: 38)).multilineTextAlignment(.center)
                    if server.restarting {
                        // The manager relaunches Jellyfin on its own; no restart button here.
                        if let notice = server.notice { Text(notice).multilineTextAlignment(.center).frame(maxWidth: 480).foregroundStyle(.orange) }
                        ProgressView("Server wird neu gestartet …")
                    } else if server.isStopping {
                        // Quit or restart in progress: no restart button until the old manager is gone.
                        ProgressView("Wird beendet …")
                    } else if let error = server.error {
                        Text(error).multilineTextAlignment(.center).frame(maxWidth: 480)
                        if server.blockedSnapshot != nil {
                            Button("Sicherung vor dem Update wiederherstellen") { server.rollback(window: NSApp.keyWindow) }.buttonStyle(.borderedProminent)
                        }
                        // During a slow first start the text is only a notice; the start continues.
                        else if server.starting { ProgressView("Mutti wird gestartet …") }
                        else { Button("Erneut starten", action: server.restart).buttonStyle(.borderedProminent) }
                    } else if let message = server.progressMessage {
                        ProgressView(message)
                    } else { ProgressView("Mutti wird gestartet …") }
                    if let settingsError = server.settingsError { Text(settingsError).multilineTextAlignment(.center).frame(maxWidth: 480).foregroundStyle(.orange) }
                }.frame(maxWidth: .infinity, maxHeight: .infinity).padding(32)
            }
        }.background(Color(red: 31/255, green: 31/255, blue: 31/255))
        .tint(Color(red: 1, green: 230/255, blue: 0))
        .preferredColorScheme(.dark)
        .sheet(isPresented: $settings) { ConnectSettingsView(server: server) }
        .onChange(of: server.setupCompleted) { _, completed in
            if !completed { settings = false }
        }
    }
}

struct AdminView: NSViewRepresentable {
    let address: URL
    let nativeImportClient: NativeImportClient?
    var onNavigate: (String) -> Void = { _ in }
    func makeCoordinator() -> Coordinator { Coordinator(address: address, nativeImportClient: nativeImportClient, onNavigate: onNavigate) }
    func makeNSView(context: Context) -> WKWebView {
        let configuration = WKWebViewConfiguration()
        configuration.userContentController.addScriptMessageHandler(context.coordinator, contentWorld: .page, name: "muttiFolder")
        configuration.userContentController.addScriptMessageHandler(context.coordinator, contentWorld: .page, name: "muttiImport")
        configuration.userContentController.addScriptMessageHandler(context.coordinator, contentWorld: .page, name: "muttiNavigate")
        let view = WKWebView(frame: .zero, configuration: configuration)
        view.navigationDelegate = context.coordinator; view.load(URLRequest(url: address)); return view
    }
    func updateNSView(_ view: WKWebView, context: Context) {}
    @MainActor final class Coordinator: NSObject, WKNavigationDelegate, WKScriptMessageHandlerWithReply {
        let address: URL
        let nativeImportClient: NativeImportClient?
        let onNavigate: (String) -> Void
        init(address: URL, nativeImportClient: NativeImportClient?, onNavigate: @escaping (String) -> Void) {
            self.address = address; self.nativeImportClient = nativeImportClient; self.onNavigate = onNavigate
        }
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
            if message.name == "muttiNavigate" {
                guard NativeManagementNavigation.accepts(frame: message.frameInfo.request.url, visible: message.webView?.url, isMainFrame: message.frameInfo.isMainFrame),
                      let body = message.body as? [String: String], let action = body["action"], ["import", "remote"].contains(action)
                else { replyHandler(nil, "Navigation is unavailable for this page."); return }
                onNavigate(action); replyHandler(true, nil); return
            }
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
