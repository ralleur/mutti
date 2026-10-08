// SPDX-License-Identifier: GPL-2.0-or-later
import Foundation

/// Restricts native management navigation to the packaged server page.
enum NativeManagementNavigation {
    static func accepts(frame: URL?, visible: URL?, isMainFrame: Bool) -> Bool {
        guard isMainFrame, trustedPage(frame), trustedPage(visible),
              let fragment = visible?.fragment else { return false }
        return fragment == "/mutti" || fragment.hasPrefix("/mutti/")
    }

    private static func trustedPage(_ url: URL?) -> Bool {
        guard let url, url.scheme == "http", url.host == "127.0.0.1",
              url.port == 18596, url.user == nil, url.password == nil else { return false }
        // Foundation URL.path normalizes /web/ to /web on macOS.
        return ["/web", "/web/", "/web/index.html"].contains(url.path)
    }
}
