// SPDX-License-Identifier: GPL-2.0-or-later
import Foundation
import Testing
@testable import Mutti

struct ManagerStateTests {
    @Test func decodesStateFromOlderManagerWithoutAvailabilityFields() throws {
        let json = #"{"progress":{},"ready":true,"setupComplete":true,"newSetup":false,"phase":"ready","message":"","target":"/t","active":"/a"}"#
        let state = try JSONDecoder().decode(ManagerState.self, from: Data(json.utf8))
        #expect(state.ready == true)
        #expect(state.setupComplete == true)
        #expect(state.phase == "ready")
        #expect(state.active == "/a")
        #expect(state.restarts == nil)
        #expect(state.connectState == nil)
        #expect(state.connectMessage == nil)
        #expect(!state.isRestarting)
        #expect(!state.isError)
        #expect(!state.connectFailed)
    }

    @Test func decodesAvailabilityFields() throws {
        let json = #"{"ready":false,"setupComplete":false,"newSetup":false,"phase":"restarting","message":"Jellyfin wird neu gestartet.","active":"/a","restarts":2,"connectState":"failed","connectMessage":"Connect konnte nicht starten."}"#
        let state = try JSONDecoder().decode(ManagerState.self, from: Data(json.utf8))
        #expect(state.restarts == 2)
        #expect(state.connectState == "failed")
        #expect(state.connectMessage == "Connect konnte nicht starten.")
        #expect(state.isRestarting)
        #expect(!state.isError)
        #expect(state.connectFailed)
    }

    @Test func restartingIsNotAnError() throws {
        let error = #"{"ready":false,"setupComplete":false,"newSetup":false,"phase":"error","message":"Kaputt.","active":"/a"}"#
        let state = try JSONDecoder().decode(ManagerState.self, from: Data(error.utf8))
        #expect(state.isError)
        #expect(!state.isRestarting)
        for connectState in ["", "starting", "running"] {
            let json = #"{"ready":true,"setupComplete":true,"newSetup":false,"phase":"ready","message":"","active":"/a","connectState":""# + connectState + #""}"#
            #expect(try JSONDecoder().decode(ManagerState.self, from: Data(json.utf8)).connectFailed == false)
        }
    }

    @Test func decodesUpdateGuardFields() throws {
        let blocked = #"{"ready":false,"setupComplete":true,"newSetup":false,"phase":"update_blocked","message":"Das Update wurde gestoppt.","active":"/a","update":{"state":"blocked","snapshot":"20261007T090000Z"}}"#
        let state = try JSONDecoder().decode(ManagerState.self, from: Data(blocked.utf8))
        #expect(state.isBlocked)
        #expect(!state.isError)
        #expect(state.update?.snapshot == "20261007T090000Z")
        let replaced = #"{"ready":true,"setupComplete":true,"newSetup":false,"phase":"ready","message":"","active":"/a","serviceMessage":"Mutti wurde ersetzt."}"#
        let running = try JSONDecoder().decode(ManagerState.self, from: Data(replaced.utf8))
        #expect(running.serviceMessage == "Mutti wurde ersetzt.")
        #expect(running.update == nil)
        #expect(!running.isBlocked)
    }

    @Test func requiredFieldsStayRequired() {
        #expect(throws: DecodingError.self) {
            try JSONDecoder().decode(ManagerState.self, from: Data(#"{"ready":true}"#.utf8))
        }
    }
}
