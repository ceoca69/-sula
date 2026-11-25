import Foundation
import SwiftUI
import UIKit
import Zip

struct FairPlayDecryptionOutput {
    let ipaURL: URL
    let decryptedBinaryCount: Int
    let failedBinaryPaths: [String]
}

enum FairPlayDecryptorError: LocalizedError {
    case noBundleIdentifier
    case installedApplicationNotFound(String)
    case fairPlayCannotReadDataContainer(String)
    case invalidApplicationBundle(String)
    case mainExecutableMissing(String)
    case mainExecutableDecryptFailed(String)
    case noEncryptedExecutables
    case archiveCreationFailed(String)

    var errorDescription: String? {
        switch self {
        case .noBundleIdentifier:
            return "No downgraded app is selected. Complete a downgrade first."
        case .installedApplicationNotFound(let bundleID):
            return "Could not resolve the installed application for \(bundleID). Make sure the downgraded app is installed on this device."
        case .fairPlayCannotReadDataContainer(let path):
            return "The app bundle is inside an app data container (\(path)). Apple's FairPlay service cannot read SC_Info from data containers. Install the target as a normal iOS app before retrying."
        case .invalidApplicationBundle(let details):
            return "The installed application bundle is invalid: \(details)"
        case .mainExecutableMissing(let name):
            return "The app's main executable could not be found: \(name)"
        case .mainExecutableDecryptFailed(let details):
            return "FairPlay could not decrypt the app's main executable. \(details)"
        case .noEncryptedExecutables:
            return "No FairPlay-encrypted Mach-O executables were found in this app."
        case .archiveCreationFailed(let details):
            return "Could not create the decrypted IPA: \(details)"
        }
    }
}

enum FairPlayDecryptor {
    private static let workerQueue = DispatchQueue(label: "PancakeStore.FairPlayDecrypt", qos: .utility)

    // Do not run file copying, Mach-O parsing, FairPlay calls or ZIP work on
    // Swift's cooperative executor. LiveContainer hosts the guest app in the
    // same process, so a dedicated queue keeps its UI responsive even if
    // fairplayd stalls or a large binary takes a long time to process.
    static func decryptInstalledAppAsync(bundleID: String) async throws -> FairPlayDecryptionOutput {
        try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<FairPlayDecryptionOutput, Error>) in
            workerQueue.async {
                do {
                    let output = try autoreleasepool {
                        try decryptInstalledApp(bundleID: bundleID)
                    }
                    continuation.resume(returning: output)
                } catch {
                    continuation.resume(throwing: error)
                }
            }
        }
    }

    static func decryptInstalledApp(bundleID: String) throws -> FairPlayDecryptionOutput {
        guard !bundleID.isEmpty else { throw FairPlayDecryptorError.noBundleIdentifier }
        let fileManager = FileManager.default

        guard let installedPath = PSResolveInstalledAppBundlePath(bundleID), !installedPath.isEmpty else {
            throw FairPlayDecryptorError.installedApplicationNotFound(bundleID)
        }

        let installedAppURL = URL(fileURLWithPath: installedPath, isDirectory: true)
        print("[Decrypt] Resolved installed bundle: \(installedAppURL.path)")
        let infoURL = installedAppURL.appendingPathComponent("Info.plist")
        guard let info = NSDictionary(contentsOf: infoURL),
              (info["CFBundleIdentifier"] as? String) == bundleID,
              let executableName = info["CFBundleExecutable"] as? String,
              !executableName.isEmpty else {
            throw FairPlayDecryptorError.invalidApplicationBundle(installedPath)
        }

        let installedMainExecutable = installedAppURL.appendingPathComponent(executableName)
        guard fileManager.fileExists(atPath: installedMainExecutable.path) else {
            throw FairPlayDecryptorError.mainExecutableMissing(executableName)
        }

        let scInfoURL = installedAppURL.appendingPathComponent("SC_Info", isDirectory: true)
        let mainSinf = scInfoURL.appendingPathComponent("\(executableName).sinf")
        let sinfEntries = (try? fileManager.contentsOfDirectory(atPath: scInfoURL.path)) ?? []
        print("[Decrypt] Main executable: \(installedMainExecutable.path)")
        print("[Decrypt] Main SINF: \(fileManager.fileExists(atPath: mainSinf.path) ? "present" : "not found at expected name")")
        print("[Decrypt] SC_Info entries: \(sinfEntries.isEmpty ? "<none or inaccessible>" : sinfEntries.joined(separator: ", "))")
        if installedPath.contains("/Containers/Data/Application/") || installedPath.contains("/Documents/Applications/") {
            print("[Decrypt] Refusing unsupported data-container path: \(installedPath)")
            throw FairPlayDecryptorError.fairPlayCannotReadDataContainer(installedPath)
        }

        let cachesURL = fileManager.urls(for: .cachesDirectory, in: .userDomainMask)[0]
        let workURL = cachesURL.appendingPathComponent("FairPlayDecrypt-\(UUID().uuidString)", isDirectory: true)
        let payloadURL = workURL.appendingPathComponent("Payload", isDirectory: true)
        let copiedAppURL = payloadURL.appendingPathComponent(installedAppURL.lastPathComponent, isDirectory: true)
        try fileManager.createDirectory(at: payloadURL, withIntermediateDirectories: true)

        defer { try? fileManager.removeItem(at: workURL) }

        // Work exclusively on a copy. SC_Info is deliberately omitted: the
        // app sandbox may not read its contents, while fairplayd reads the
        // original installed bundle directly. Watch content is also omitted.
        let copyFailures = copyApplicationTree(from: installedAppURL, to: copiedAppURL)
        if !copyFailures.isEmpty {
            print("[Decrypt] Skipped \(copyFailures.count) unreadable/copied item(s): \(copyFailures.joined(separator: ", "))")
        }

        let copiedMainExecutable = copiedAppURL.appendingPathComponent(executableName)
        guard fileManager.fileExists(atPath: copiedMainExecutable.path) else {
            throw FairPlayDecryptorError.mainExecutableMissing(executableName)
        }

        var candidates: [URL] = []
        if let enumerator = fileManager.enumerator(
            at: copiedAppURL,
            includingPropertiesForKeys: [.isRegularFileKey, .fileSizeKey],
            options: []
        ) {
            for case let fileURL as URL in enumerator {
                let relativePath = String(fileURL.path.dropFirst(copiedAppURL.path.count + 1))
                let values = try? fileURL.resourceValues(forKeys: [.isRegularFileKey, .isSymbolicLinkKey, .fileSizeKey])
                guard values?.isSymbolicLink != true,
                      values?.isRegularFile == true,
                      (values?.fileSize ?? 0) >= 4096,
                      fileURL.path.withCString({ PSIsMachOFile($0) }) else {
                    continue
                }
                candidates.append(fileURL)
            }
        }

        // Process the primary executable first so a failure there prevents a
        // misleading IPA export. Frameworks and extensions are processed next.
        print("[Decrypt] Found \(candidates.count) Mach-O candidate(s)")
        candidates.sort { lhs, rhs in
            let lhsIsMain = lhs.path == copiedMainExecutable.path
            let rhsIsMain = rhs.path == copiedMainExecutable.path
            if lhsIsMain != rhsIsMain { return lhsIsMain }
            return lhs.path < rhs.path
        }
        guard candidates.contains(where: { $0.path == copiedMainExecutable.path }) else {
            throw FairPlayDecryptorError.mainExecutableMissing(executableName)
        }

        var decryptedCount = 0
        var failedPaths: [String] = copyFailures
        for fileURL in candidates {
            let relativePath = String(fileURL.path.dropFirst(copiedAppURL.path.count + 1))
            let sourceURL = installedAppURL.appendingPathComponent(relativePath)
            guard fileManager.fileExists(atPath: sourceURL.path) else {
                if fileURL.path == copiedMainExecutable.path {
                    throw FairPlayDecryptorError.mainExecutableMissing(executableName)
                }
                failedPaths.append(relativePath)
                continue
            }

            let currentAttributes = try fileManager.attributesOfItem(atPath: fileURL.path)
            let currentPermissions = (currentAttributes[.posixPermissions] as? NSNumber)?.intValue ?? 0o644
            try fileManager.setAttributes(
                [.posixPermissions: currentPermissions | 0o200],
                ofItemAtPath: fileURL.path
            )

            var errorBuffer = [CChar](repeating: 0, count: 512)
            print("[Decrypt] Attempting \(relativePath)")
            let status = sourceURL.path.withCString { sourcePointer in
                fileURL.path.withCString { outputPointer in
                    errorBuffer.withUnsafeMutableBufferPointer { buffer in
                        PSFairPlayDecryptMachO(sourcePointer, outputPointer, buffer.baseAddress, buffer.count)
                    }
                }
            }

            if status == 0 {
                decryptedCount += 1
                print("[Decrypt] Decrypted \(fileURL.lastPathComponent)")
            } else if status < 0 {
                let detail = errorBuffer.withUnsafeBufferPointer { buffer -> String in
                    guard let baseAddress = buffer.baseAddress else { return "Unknown FairPlay error" }
                    return String(cString: baseAddress)
                }
                print("[Decrypt] Failed \(fileURL.lastPathComponent): \(detail)")
                if fileURL.path == copiedMainExecutable.path {
                    throw FairPlayDecryptorError.mainExecutableDecryptFailed(detail)
                }
                failedPaths.append(fileURL.path.replacingOccurrences(of: copiedAppURL.path + "/", with: ""))
            }
        }

        guard decryptedCount > 0 else { throw FairPlayDecryptorError.noEncryptedExecutables }

        let documentsURL = fileManager.urls(for: .documentDirectory, in: .userDomainMask)[0]
        let outputDirectory = documentsURL.appendingPathComponent("Decrypted", isDirectory: true)
        try fileManager.createDirectory(at: outputDirectory, withIntermediateDirectories: true)

        let suffix = failedPaths.isEmpty ? "-decrypted" : "-PARTIAL"
        let outputURL = outputDirectory.appendingPathComponent("\(bundleID)\(suffix).ipa")
        try? fileManager.removeItem(at: outputURL)

        do {
            // Zip the Payload folder itself, preserving the IPA's required
            // Payload/<App>.app layout at the archive root.
            try Zip.zipFiles(paths: [payloadURL], zipFilePath: outputURL, password: nil, progress: { progress in
                print("[Decrypt] Archive progress: \(Int(progress * 100))%")
            })
        } catch {
            throw FairPlayDecryptorError.archiveCreationFailed(error.localizedDescription)
        }

        guard fileManager.fileExists(atPath: outputURL.path),
              ((try? outputURL.resourceValues(forKeys: [.fileSizeKey]).fileSize) ?? 0) > 0 else {
            throw FairPlayDecryptorError.archiveCreationFailed("The output file is missing or empty.")
        }

        return FairPlayDecryptionOutput(
            ipaURL: outputURL,
            decryptedBinaryCount: decryptedCount,
            failedBinaryPaths: failedPaths
        )
    }

    private static func copyApplicationTree(from sourceURL: URL, to destinationURL: URL) -> [String] {
        let fileManager = FileManager.default
        var failures: [String] = []
        do {
            try fileManager.createDirectory(at: destinationURL, withIntermediateDirectories: true)
        } catch {
            return ["<app root>: \(error.localizedDescription)"]
        }

        let enumerator = fileManager.enumerator(
            at: sourceURL,
            includingPropertiesForKeys: [.isDirectoryKey, .isSymbolicLinkKey],
            options: []
        ) { url, error in
            let relative = String(url.path.dropFirst(sourceURL.path.count + 1))
            failures.append("\(relative): \(error.localizedDescription)")
            return true
        }

        guard let enumerator else {
            return ["<app root>: could not enumerate installed bundle"]
        }

        for case let sourceItem as URL in enumerator {
            let relativePath = String(sourceItem.path.dropFirst(sourceURL.path.count + 1))
            let components = relativePath.split(separator: "/")

            if components.contains("SC_Info") || components.contains("Watch") {
                // These are package subtrees, never individual files we need
                // in the output IPA. Skipping unconditionally also prevents
                // FileManager from trying to enumerate protected SC_Info data.
                enumerator.skipDescendants()
                continue
            }

            let destinationItem = destinationURL.appendingPathComponent(relativePath)
            do {
                let values = try sourceItem.resourceValues(forKeys: [.isDirectoryKey, .isSymbolicLinkKey])
                if values.isSymbolicLink == true {
                    let linkDestination = try fileManager.destinationOfSymbolicLink(atPath: sourceItem.path)
                    try fileManager.createDirectory(
                        at: destinationItem.deletingLastPathComponent(),
                        withIntermediateDirectories: true
                    )
                    try fileManager.createSymbolicLink(atPath: destinationItem.path, withDestinationPath: linkDestination)
                } else if values.isDirectory == true {
                    try fileManager.createDirectory(at: destinationItem, withIntermediateDirectories: true)
                } else {
                    try fileManager.createDirectory(
                        at: destinationItem.deletingLastPathComponent(),
                        withIntermediateDirectories: true
                    )
                    try fileManager.copyItem(at: sourceItem, to: destinationItem)
                }
            } catch {
                failures.append("\(relativePath): \(error.localizedDescription)")
            }
        }
        return failures
    }
}

struct DecryptedIPAActivitySheet: UIViewControllerRepresentable {
    let url: URL

    func makeUIViewController(context: Context) -> UIActivityViewController {
        UIActivityViewController(activityItems: [url], applicationActivities: nil)
    }

    func updateUIViewController(_ controller: UIActivityViewController, context: Context) {}
}

struct DecryptedIPAShareItem: Identifiable {
    let id = UUID()
    let url: URL
}
