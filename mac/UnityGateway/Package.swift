// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "UnityGateway",
    platforms: [.macOS(.v13)],
    targets: [
        .executableTarget(name: "UnityGateway", path: "Sources/UnityGateway"),
        .testTarget(name: "UnityGatewayTests", dependencies: ["UnityGateway"])
    ]
)
