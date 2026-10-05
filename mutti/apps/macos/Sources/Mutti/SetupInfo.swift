// SPDX-License-Identifier: GPL-2.0-or-later
import Foundation

// Jellyfin System/Info/Public uses StartupWizardCompleted (without "Is").
// A missing flag is deliberately not treated as a completed owner setup.
struct SetupInfo: Decodable {
    let startupWizardCompleted: Bool?
    enum CodingKeys: String, CodingKey { case startupWizardCompleted = "StartupWizardCompleted" }
}
