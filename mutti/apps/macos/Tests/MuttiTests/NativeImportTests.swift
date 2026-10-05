// SPDX-License-Identifier: GPL-2.0-or-later
import Foundation
import Testing
@testable import Mutti

struct NativeImportTests {
    @Test func onlyOnboardingMainFrameCanRequestNativeConfirmation() {
        for address in ["http://127.0.0.1:18594/", "http://127.0.0.1:18594/#import", "http://127.0.0.1:18594/index.html"] {
            #expect(NativeImportClient.accepts(URL(string: address), isMainFrame: true))
            #expect(!NativeImportClient.accepts(URL(string: address), isMainFrame: false))
        }
        for address in ["https://attacker.example/", "http://127.0.0.1:18596/web/", "http://127.0.0.1:18595/", "http://127.0.0.1:18594/api/state", "http://localhost:18594/", "http://user@127.0.0.1:18594/", "https://127.0.0.1:18594/"] {
            #expect(!NativeImportClient.accepts(URL(string: address), isMainFrame: true))
        }
        #expect(!NativeImportClient.accepts(nil, isMainFrame: true))
    }
}
