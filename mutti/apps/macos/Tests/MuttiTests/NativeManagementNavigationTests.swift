// SPDX-License-Identifier: GPL-2.0-or-later
import Foundation
import Testing
@testable import Mutti

struct NativeManagementNavigationTests {
    @Test func acceptsNormalizedWebPathAndManagementDetails() {
        for page in ["http://127.0.0.1:18596/web/#/mutti", "http://127.0.0.1:18596/web/index.html#/mutti/settings"] {
            let url = URL(string: page)!
            #expect(NativeManagementNavigation.accepts(frame: url, visible: url, isMainFrame: true))
        }
    }

    @Test func rejectsOtherFramesPagesAndOrigins() {
        let page = URL(string: "http://127.0.0.1:18596/web/#/mutti/settings")!
        #expect(!NativeManagementNavigation.accepts(frame: page, visible: page, isMainFrame: false))
        for address in ["http://localhost:18596/web/#/mutti", "http://127.0.0.1:18594/web/#/mutti", "https://example.org/web/#/mutti", "http://name@127.0.0.1:18596/web/#/mutti", "http://127.0.0.1:18596/web/#/home", "http://127.0.0.1:18596/web/#/muttievil", "http://127.0.0.1:18596/other/#/mutti"] {
            #expect(!NativeManagementNavigation.accepts(frame: page, visible: URL(string: address), isMainFrame: true))
        }
        #expect(!NativeManagementNavigation.accepts(frame: URL(string: "https://example.org/web/"), visible: page, isMainFrame: true))
    }
}
