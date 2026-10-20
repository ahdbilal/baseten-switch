import SwiftUI

struct SavedAPIKeyView: View {
    @ObservedObject var state: BasetenSwitchState
    let isPreview: Bool
    @State private var error: String?
    @State private var confirmsRemoval = false
    @State private var showsEditor = false
    @State private var editingExistingKey = false

    private var hasSavedKey: Bool { state.auth?.savedAPIKey == true }
    private var canEdit: Bool { !isPreview && state.canManageAPIKey }

    @State private var workspace = ""

    var body: some View {
        RoutingSectionCard {
            Label("Databricks workspace", systemImage: "key")
                .font(.headline)
        } content: {
            VStack(alignment: .leading, spacing: 10) {
                if let auth = state.auth, !auth.workspace.isEmpty {
                    Text(auth.workspace).font(.subheadline).textSelection(.enabled)
                    Text(auth.health == "ok" ? "Connected" : "Sign in to verify your connection")
                        .font(.caption).foregroundStyle(.secondary)
                }
                HStack {
                    TextField("Workspace URL (leave blank to reuse)", text: $workspace)
                        .textFieldStyle(.roundedBorder)
                        .accessibilityIdentifier("workspace-url")
                    Button("Sign in…") {
                        Task { await state.reauthenticate(workspace: workspace) }
                    }
                    .disabled(isPreview || state.reauthenticating)
                    .accessibilityIdentifier("workspace-sign-in")
                }
                Text("Opens Databricks authentication in Terminal and your browser.")
                    .font(.caption).foregroundStyle(.secondary)
            }.padding(6)
        }.accessibilityIdentifier("overview-authentication")
    }

    private var statusLabel: String {
        guard let auth = state.auth else { return "Authentication unavailable" }
        if auth.savedAPIKey {
            return auth.signedIn ? "API key saved" : "API key unavailable"
        }
        if auth.health == "refresh_failed" { return "Sign-in required" }
        if auth.signedIn && auth.health != "signed_out" {
            return auth.profile.isEmpty ? "Signed in" : auth.profile
        }
        return auth.fallbackInUse ? "Using API key" : "Not signed in"
    }
}

struct SavedAPIKeyEditor: View {
    @ObservedObject var state: BasetenSwitchState
    let isReplacing: Bool
    @Environment(\.dismiss) private var dismiss
    @FocusState private var keyIsFocused: Bool
    @State private var key = ""
    @State private var error: String?
    @State private var isSubmitting = false

    private var isBusy: Bool { isSubmitting || state.savingAPIKey }

    private var canSave: Bool {
        !isBusy && state.canManageAPIKey
            && !key.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 18) {
            Text(isReplacing ? "Change API Key" : "Add API Key")
                .font(.title3.weight(.semibold))
                .accessibilityAddTraits(.isHeader)
            VStack(alignment: .leading, spacing: 6) {
                SecureField("API key", text: $key)
                    .textFieldStyle(.roundedBorder)
                    .focused($keyIsFocused)
                    .disabled(isBusy)
                    .accessibilityIdentifier("saved-api-key-input")
                    .onSubmit { save() }
                Text("Stored in Keychain.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            if let error {
                Text(error)
                    .font(.callout)
                    .foregroundStyle(.red)
                    .fixedSize(horizontal: false, vertical: true)
            }
            HStack(spacing: 8) {
                if isBusy {
                    ProgressView().controlSize(.small)
                        .accessibilityLabel("Saving API key")
                }
                Spacer()
                Button("Cancel") { dismiss() }
                    .keyboardShortcut(.cancelAction)
                    .disabled(isBusy)
                Button("Save", action: save)
                    .buttonStyle(.borderedProminent)
                    .keyboardShortcut(.defaultAction)
                    .disabled(!canSave)
                    .accessibilityIdentifier("save-api-key")
            }
        }
        .padding(20)
        .frame(width: 440)
        .interactiveDismissDisabled(isBusy)
        .accessibilityIdentifier("saved-api-key-editor")
        .onAppear { keyIsFocused = true }
        .onDisappear { key = "" }
    }

    private func save() {
        guard canSave else { return }
        let value = key.trimmingCharacters(in: .whitespacesAndNewlines)
        isSubmitting = true
        error = nil
        Task {
            defer { isSubmitting = false }
            error = await state.updateSavedAPIKey(value)
            if error == nil {
                key = ""
                dismiss()
            }
        }
    }
}
