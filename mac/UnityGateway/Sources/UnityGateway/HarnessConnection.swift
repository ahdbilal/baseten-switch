import SwiftUI

struct HarnessConnectionStatus: Decodable {
    let enabled: Bool
    let configured: Bool
}

struct HarnessConnectionSection: View {
    @ObservedObject var state: UnityGatewayState
    let client: String
    private var status: HarnessConnectionStatus? { state.harnessStatuses[client] }
    private var busy: Bool { state.harnessBusy.contains(client) }
    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Toggle("Enable Unity Gateway", isOn: Binding(
                    get: { status?.configured == true },
                    set: { enabled in Task { await state.harnessCommand(client, enabled: enabled) } }
                )).toggleStyle(.switch)
                    .disabled(status == nil || busy || !state.canMutate)
                if busy { ProgressView().controlSize(.small) }
                Button { Task { await state.harnessCommand(client) } } label: {
                    Image(systemName: "arrow.clockwise")
                }.buttonStyle(.borderless).disabled(busy).help("Refresh connection status")
            }
            Text(status == nil ? "Checking connection…" : status?.enabled == true
                 ? "Connected. Disable to restore this client's previous provider settings."
                 : status?.configured == true ? "Configuration needs attention. Disable to remove the gateway setup."
                 : "Disabled. Your own provider settings are in use.")
                .font(.caption).foregroundStyle(.secondary)
            if let notice = state.harnessNotices[client] {
                Text(notice).font(.caption).foregroundStyle(.secondary).textSelection(.enabled)
            }
        }
        .padding(14)
        .background(Color.secondary.opacity(0.05), in: RoundedRectangle(cornerRadius: 10))
        .accessibilityIdentifier("harness-connection-" + client)
        .task { await state.harnessCommand(client) }
    }
}
