// SPDX-License-Identifier: GPL-2.0-or-later
import AppKit
import Foundation
import Security

/// Capability held only by the native parent and its manager child. It never enters
/// the web page, command line, environment, logs, or persistent browser storage.
@MainActor
final class NativeImportClient {
    private let token: String
    private let session: URLSession
    private let origin = URL(string: "http://127.0.0.1:18594/")!
    private var confirming = false

    init() throws {
        var bytes = [UInt8](repeating: 0, count: 32)
        guard SecRandomCopyBytes(kSecRandomDefault, bytes.count, &bytes) == errSecSuccess else {
            throw ServerController.Failure("Die lokale Importfreigabe konnte nicht vorbereitet werden.")
        }
        token = bytes.map { String(format: "%02x", $0) }.joined()
        let configuration = URLSessionConfiguration.ephemeral
        configuration.connectionProxyDictionary = [:]
        configuration.timeoutIntervalForRequest = 10
        session = URLSession(configuration: configuration, delegate: ImportRedirectBlocker(), delegateQueue: nil)
    }

    func attach(to process: Process) throws {
        let pipe = Pipe()
        process.standardInput = pipe
        try pipe.fileHandleForWriting.write(contentsOf: Data(token.utf8))
        try pipe.fileHandleForWriting.close()
    }

    nonisolated static func accepts(_ url: URL?, isMainFrame: Bool) -> Bool {
        guard isMainFrame, let url else { return false }
        return url.scheme == "http" && url.host == "127.0.0.1" && url.port == 18594
            && url.user == nil && url.password == nil && ["/", "/index.html"].contains(url.path)
    }

    private func request(_ path: String, body: Data? = nil, csrf: String? = nil) async throws -> Data {
        var request = URLRequest(url: origin.appending(path: path), cachePolicy: .reloadIgnoringLocalCacheData)
        request.setValue(token, forHTTPHeaderField: "X-Mutti-Native-Owner")
        if let body {
            request.httpMethod = "POST"
            request.httpBody = body
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            request.setValue("http://127.0.0.1:18594", forHTTPHeaderField: "Origin")
            request.setValue(csrf, forHTTPHeaderField: "X-Mutti-CSRF")
        }
        let (data, response) = try await session.data(for: request)
        guard let response = response as? HTTPURLResponse, (200..<300).contains(response.statusCode) else {
            throw ServerController.Failure(String(data: data.prefix(1024), encoding: .utf8) ?? "Die Importfreigabe ist nicht erreichbar.")
        }
        return data
    }

    private struct Session: Decodable { var csrf: String; var nativeOwner: Bool }
    private func authorization() async throws -> Session {
        try JSONDecoder().decode(Session.self, from: await request("api/session"))
    }
    func available() async throws -> Bool { try await authorization().nativeOwner }

    func start(input: [String: Any], window: NSWindow) async throws -> Bool {
        guard !confirming else { throw ServerController.Failure("Bitte die offene Importbestätigung abschließen.") }
        confirming = true
        defer { confirming = false }
        let data = try JSONSerialization.data(withJSONObject: input)
        guard data.count <= 64 * 1024 else { throw ServerController.Failure("Bitte die Importangaben prüfen.") }
        if input["Replace"] as? Bool == true {
            let alert = NSAlert()
            alert.messageText = NSLocalizedString("Zu deiner Jellyfin-Bibliothek wechseln?", comment: "Import confirmation")
            alert.informativeText = NSLocalizedString("Mutti bereitet Jellyfin vor, übernimmt und prüft deine Daten. Falls nötig, startet Mutti deinen Jellyfin-Server kurz neu; laufende Wiedergaben werden dabei unterbrochen. Deine bisherige Mutti-Einrichtung bleibt auf diesem Mac erhalten. Anschließend verwendest du deinen vorhandenen Jellyfin-Zugang.", comment: "Import confirmation")
            alert.addButton(withTitle: NSLocalizedString("Übernehmen", comment: "Import confirmation"))
            alert.addButton(withTitle: NSLocalizedString("Abbrechen", comment: "Import confirmation"))
            let result = await alert.beginSheetModal(for: window)
            guard result == .alertFirstButtonReturn else { return false }
        }
        let authorization = try await authorization()
        guard authorization.nativeOwner else { throw ServerController.Failure("Bitte Mutti erneut öffnen, um den Import in der App zu bestätigen.") }
        _ = try await request("api/import", body: data, csrf: authorization.csrf)
        return true
    }

    /// Rolls back to the pre-update backup; only the native owner may do this
    /// and only while the manager blocks the start.
    func rollback(snapshot: String, window: NSWindow?) async throws -> Bool {
        guard !confirming else { throw ServerController.Failure("Bitte die offene Bestätigung abschließen.") }
        confirming = true
        defer { confirming = false }
        let alert = NSAlert()
        alert.messageText = NSLocalizedString("Datenstand vor dem Update wiederherstellen?", comment: "Rollback confirmation")
        alert.informativeText = NSLocalizedString("Mutti stellt die Sicherung wieder her, die vor dem Update angelegt wurde. Der aktuelle Datenstand wird nicht gelöscht, sondern im Mutti-Ordner beiseitegelegt. Nach der Sicherung entzogene Geräte und Freigaben bleiben entzogen; danach gekoppelte Geräte koppelst du neu.", comment: "Rollback confirmation")
        alert.addButton(withTitle: NSLocalizedString("Wiederherstellen", comment: "Rollback confirmation"))
        alert.addButton(withTitle: NSLocalizedString("Abbrechen", comment: "Rollback confirmation"))
        let result = if let window { await alert.beginSheetModal(for: window) } else { alert.runModal() }
        guard result == .alertFirstButtonReturn else { return false }
        let authorization = try await authorization()
        guard authorization.nativeOwner else { throw ServerController.Failure("Bitte Mutti erneut öffnen, um die Wiederherstellung in der App zu bestätigen.") }
        let body = try JSONSerialization.data(withJSONObject: ["Snapshot": snapshot])
        _ = try await request("api/update/rollback", body: body, csrf: authorization.csrf)
        return true
    }
}

private final class ImportRedirectBlocker: NSObject, URLSessionTaskDelegate {
    func urlSession(_ session: URLSession, task: URLSessionTask, willPerformHTTPRedirection response: HTTPURLResponse,
                    newRequest request: URLRequest, completionHandler: @escaping @Sendable (URLRequest?) -> Void) {
        completionHandler(nil)
    }
}
