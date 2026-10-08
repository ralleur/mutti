// SPDX-License-Identifier: GPL-2.0-or-later
import Foundation
import Testing
@testable import Mutti

struct SetupInfoTests {
    @Test func publicAPIContract() throws {
        let before = Data(#"{"ServerName":"Mutti","Version":"12.1","StartupWizardCompleted":false}"#.utf8)
        let after = Data(#"{"ServerName":"Mutti","Version":"12.1","StartupWizardCompleted":true}"#.utf8)
        #expect(try JSONDecoder().decode(SetupInfo.self, from: before).startupWizardCompleted == false)
        #expect(try JSONDecoder().decode(SetupInfo.self, from: after).startupWizardCompleted == true)
    }

    @Test func unknownCompletionIsNotAccepted() throws {
        for json in [#"{}"#, #"{"StartupWizardCompleted":null}"#, #"{"IsStartupWizardCompleted":true}"#] {
            #expect(try JSONDecoder().decode(SetupInfo.self, from: Data(json.utf8)).startupWizardCompleted != true)
        }
        #expect(throws: DecodingError.self) {
            try JSONDecoder().decode(SetupInfo.self, from: Data(#"{"StartupWizardCompleted":"true"}"#.utf8))
        }
    }
}
