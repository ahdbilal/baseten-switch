import AppKit
import SwiftUI

struct DesktopClientStatus: Decodable {
    let models: [PiModelOption]
    let hidden: [String: Bool]?
    let `default`: String
    let connected: Bool
    let configured: Bool
    let base_url: String
    let directory: String
}

struct DesktopClientView: View {
    @ObservedObject var state: UnityGatewayState
    var client = "claude-desktop"
    private var isCodex: Bool { client == "codex-desktop" }
    private var name: String { isCodex ? "Codex Desktop" : "Claude Desktop" }
    private var status: DesktopClientStatus? { isCodex ? state.codexDesktopStatus : state.desktopStatus }
    private var busy: Bool { isCodex ? state.codexDesktopBusy : state.desktopBusy }
    private var notice: String? { isCodex ? state.codexDesktopNotice : state.desktopNotice }
    private var compassModel: String { isCodex ? "Auto(Compass)" : "claude-unity-compass" }
    private func command(_ args: [String] = ["status", "--json"]) async {
        if isCodex { await state.codexDesktopCommand(args) } else { await state.desktopCommand(args) }
    }
    private var ready: Bool { status?.connected == true && !state.harnessBusy.contains(client) }
    private var models: [PiModelOption] { status?.models ?? [] }
    private var savedSelection: Set<String> {
        Set(models.filter { status?.hidden?[$0.id] != true }.map(\.id))
    }
    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                SwitchClientHeader(state: state, name: name, client: client)
                ModelConfigurationCard {
                    ClientDefaultRow(state: state, client: client, models: models, current: status?.default ?? "")
                    Divider()
                    ClientAutoOptions(state: state, client: client)
                    Divider()
                    ClientVisibleModels(models: models, saved: savedSelection,
                        busy: !ready || busy || !state.canMutate,
                        refresh: { await command(["refresh"]) }, save: { desired in
                            let defaultModel = desired.contains(status?.default ?? "") ? status!.default : models.first(where: { desired.contains($0.id) })?.id ?? ""
                            await command(["selection", defaultModel] + models.filter { desired.contains($0.id) }.map(\.id))
                            await state.loadCompass()
                            return savedSelection == desired && status?.default == defaultModel
                        })
                }.disabled(!ready)
                DisclosureGroup("Connection & setup") {
                    VStack(alignment: .leading, spacing: 10) {
                        Text(status?.base_url ?? (isCodex ? "http://127.0.0.1:45276/v1" : "http://127.0.0.1:45275")).textSelection(.enabled)
                        Text(isCodex ? "Uses ~/.codex/config.toml. Unprofiled CLI launches share these defaults. Disable restores the previous settings and preserves your OpenAI sign-in. Restart Codex and start a new chat to apply." : "Uses a separate Desktop gateway profile. Organization-managed settings take precedence. Requests appear under Claude Desktop in Traffic.")
                        HStack {
                            Button("Refresh status") { Task { await command() } }

                        }.disabled(busy || !state.canMutate)
                    }.font(.caption).foregroundStyle(.secondary).padding(.top, 8)
                }
                if let notice = notice { Text(notice).font(.caption).textSelection(.enabled) }
            }.padding(28).frame(maxWidth: 900, alignment: .leading)
                .frame(maxWidth: .infinity, alignment: .topLeading)
        }.task { await command() }
    }

}
