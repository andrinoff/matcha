import Cocoa

// Compilation: swiftc save_file_picker.swift -o save_file_picker
// Usage: ./save_file_picker [initial_path] [suggested_filename]

func saveFilePicker(initialPath: String?, suggestedFilename: String?) {
    let dialog = NSSavePanel()

    dialog.title                   = "Export Email"
    dialog.showsResizeIndicator    = true
    dialog.showsHiddenFiles        = false
    dialog.canCreateDirectories    = true

    if let initialPath = initialPath {
        dialog.directoryURL = URL(fileURLWithPath: (initialPath as NSString).expandingTildeInPath)
    }

    if let suggestedFilename = suggestedFilename, !suggestedFilename.isEmpty {
        dialog.nameFieldStringValue = suggestedFilename
    }

    // Since this is a CLI helper, we need to force it to the front
    NSApp.setActivationPolicy(.accessory)
    NSApp.activate(ignoringOtherApps: true)

    if dialog.runModal() == .OK {
        if let url = dialog.url {
            print(url.path)
        }
    }
}

let args = ProcessInfo.processInfo.arguments
let initialPath = args.count > 1 ? args[1] : nil
let suggestedFilename = args.count > 2 ? args[2] : nil

saveFilePicker(initialPath: initialPath, suggestedFilename: suggestedFilename)
