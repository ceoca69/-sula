//
//  SettingsView.swift
//  PancakeStore
//
//  Created by lunginspector on 1/11/26.
//

import SwiftUI
import PartyUI

struct SettingsView: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(\.openURL) private var openURL
    
    @AppStorage("autoCleanApp") var autoCleanApp: Bool = true
    @StateObject private var store = StoreData.shared
    @State private var isDecrypting = false
    @State private var decryptError: String?
    @State private var shareItem: DecryptedIPAShareItem?

    var body: some View {
        NavigationStack {
            List {
                Section {
                    VStack(alignment: .leading, spacing: 10) {
                        AppInfoCell(build: appBuild)
                        HStack {
                            Button {
                                openURL(URL(string: "https://jailbreak.party/discord")!)
                            } label: {
                                ButtonLabel(text: String(localized: "action.discord"), icon: "discord", useImage: true)
                            }
                            .buttonStyle(TranslucentButtonStyle(color: .discord))

                            Button {
                                openURL(URL(string: "https://github.com/jailbreakdotparty/PancakeStore")!)
                            } label: {
                                ButtonLabel(text: String(localized: "action.github"), icon: "github", useImage: true)
                            }
                            .buttonStyle(TranslucentButtonStyle(color: .github))
                        }

                        Button {
                            openURL(URL(string: "https://jailbreak.party/")!)
                        } label: {
                            ButtonLabel(text: String(localized: "action.website"), icon: "globe")
                        }
                        .buttonStyle(TranslucentButtonStyle())
                    }
                } header: {
                    HeaderLabel(text: String(localized: "section.about.title"), icon: "info.circle")
                }

                Section {
                    Toggle(isOn: $autoCleanApp) {
                        Text("settings.autoClean.title")
                        Text("settings.autoClean.subtitle")
                    }

                    Button("action.cleanDocuments") {
                        cleanUp()
                    }
                } header: {
                    HeaderLabel(text: String(localized: "section.data.title"), icon: "loupe")
                }

                Section {
                    Button {
                        startDecryption()
                    } label: {
                        HStack(spacing: 10) {
                            if isDecrypting {
                                ProgressView()
                            } else {
                                Image(systemName: "lock.open")
                            }
                            Text("Decrypt")
                        }
                    }
                    .disabled(!store.hasServedApp || store.appBID.isEmpty || isDecrypting)
                } header: {
                    HeaderLabel(text: "Decrypt", icon: "lock.open")
                } footer: {
                    Text("Attempts FairPlay decryption of the currently downgraded app. The app must be installed on this device and licensed to the Apple ID signed in here.")
                }

                Section {
                    LinkCreditCell(image: Image("mineek"), name: "mineek", description: String(localized: "credits.mineek"), url: "https://github.com/mineek")
                    LinkCreditCell(image: Image("lunginspector"), name: "lunginspector", description: String(localized: "credits.lunginspector"), url: "https://github.com/lunginspector")
                    LinkCreditCell(image: Image("skadz"), name: "Skadz", description: String(localized: "credits.skadz"), url: "https://github.com/skadz108")
                    
                    NavigationLink {
                        List {
                            TranslatorCreditCell(name: "Isacucho", languageKey: "language.spanish", url: "https://github.com/isacucho")
                            
                            TranslatorCreditCell(name: "gerda", languageKey: "language.russian", url: "https://github.com/gerdaroot")
                            
                            TranslatorCreditCell(name: "roooot", languageKey: "language.german", url: "https://github.com/rooootdev")
                            
                            TranslatorCreditCell(name: "TrollStoreX", languageKey: "language.chineseSimp", url: "https://github.com/TrollStoreX")
                            
                            TranslatorCreditCell(name: "neonmodder123", languageKey: "language.arabic", url: "https://github.com/neonmodder123")
                            
                            TranslatorCreditCell(name: "Jurre", languageKey: "language.dutch", url: "https://github.com/jurre111")
                            
                            TranslatorCreditCell(name: "MineTurtlee", languageKey: "language.vietnamese", url: "https://github.com/MineTurtlee")
                            
                            TranslatorCreditCell(name: "nxtcoreee3", languageKey: "language.swedish, language.romanian, language.norwegian", url: "https://github.com/nxtcoreee3")
                            
                            TranslatorCreditCell(name: "fil", languageKey: "language.italian", url: "https://github.com/tiziodied")
                        }
                        .navigationTitle("credits.translators.title")
                    } label: {
                        Text("credits.translators.title")
                    }
                } header: {
                    HeaderLabel(text: String(localized: "section.credits.title"), icon: "star")
                }
            }
            .navigationTitle("settings.title")
            .sheet(item: $shareItem) { item in
                DecryptedIPAActivitySheet(url: item.url)
            }
            .alert("Decrypt failed", isPresented: Binding(
                get: { decryptError != nil },
                set: { if !$0 { decryptError = nil } }
            )) {
                Button("OK", role: .cancel) { decryptError = nil }
            } message: {
                Text(decryptError ?? "Unknown error")
            }
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button(action: {
                        dismiss()
                    }) {
                        Image(systemName: "xmark")
                    }
                }
            }
        }
    }

    private func startDecryption() {
        guard !isDecrypting, store.hasServedApp, !store.appBID.isEmpty else { return }
        let bundleID = store.appBID
        isDecrypting = true
        decryptError = nil

        print("[Decrypt] Starting FairPlay export for \(bundleID)")
        Task { @MainActor in
            do {
                let result = try await FairPlayDecryptor.decryptInstalledAppAsync(bundleID: bundleID)

                isDecrypting = false
                if result.failedBinaryPaths.isEmpty {
                    print("[Decrypt] Exported \(result.ipaURL.lastPathComponent); decrypted \(result.decryptedBinaryCount) Mach-O files")
                } else {
                    print("[Decrypt] Partial export; decrypted \(result.decryptedBinaryCount) Mach-O files, failed: \(result.failedBinaryPaths.joined(separator: ", "))")
                }
                shareItem = DecryptedIPAShareItem(url: result.ipaURL)
            } catch {
                isDecrypting = false
                decryptError = error.localizedDescription
            }
        }
    }
}

var creditCell: CGFloat {
    if #available(iOS 19.0, *) { return 14 } else { return 16 }
}

// add to partyui?
struct TranslatorCreditCell: View {
    var name: String
    var languageDisplay: String
    var url: String
    @Environment(\.openURL) var openURL
    
    public init(name: String, languageKey: String, url: String = "") {
        self.name = name
        self.url = url
        
        let localizedNames = languageKey
            .split(separator: ",")
            .map { $0.trimmingCharacters(in: .whitespaces) }
            .map { String(localized: String.LocalizationValue($0)) }
        
        let formatter = ListFormatter()
        self.languageDisplay = formatter.string(from: localizedNames) ?? localizedNames.joined(separator: ", ")
    }
    
    public var body: some View {
        Button(action: {
            if !url.isEmpty, let link = URL(string: url) { openURL(link) }
        }) {
            HStack(spacing: creditCell) {
                VStack(alignment: .leading) {
                    Text(name)
                        .fontWeight(.semibold)
                    Text(languageDisplay)
                        .multilineTextAlignment(.leading)
                        .font(.subheadline)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                if !url.isEmpty {
                    Spacer()
                    Image(systemName: "chevron.right")
                        .fontWeight(.semibold)
                        .foregroundStyle(.tertiary)
                        .imageScale(.small)
                }
            }
        }
        .foregroundStyle(Color(.label))
    }
}

#Preview {
    SettingsView()
}
