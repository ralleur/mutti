// swift-tools-version: 6.0
import PackageDescription
let package = Package(name: "Mutti", platforms: [.macOS(.v14)], products: [.executable(name: "Mutti", targets: ["Mutti"])], targets: [.executableTarget(name: "Mutti")])
