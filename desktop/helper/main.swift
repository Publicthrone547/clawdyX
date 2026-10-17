// clawdyx-screenshot: a tiny ScreenCaptureKit helper shipped inside
// clawdyX.app (Contents/Helpers). The worker starts it with fixed arguments;
// it prints one JSON object on stdout and exits.
//
//   clawdyx-screenshot preflight                 -> {"granted":true|false}
//   clawdyx-screenshot request                   -> asks macOS for Screen Recording (shows the system prompt once)
//   clawdyx-screenshot capture <displayID> <out> -> writes a PNG of that display to <out>
//
// It never captures without the capture command and never keeps images.
import CoreGraphics
import Foundation
import ImageIO
import ScreenCaptureKit
import UniformTypeIdentifiers

func emit(_ obj: [String: Any]) {
    if let data = try? JSONSerialization.data(withJSONObject: obj), let s = String(data: data, encoding: .utf8) {
        print(s)
    }
}

func fail(_ msg: String, code: Int32 = 1) -> Never {
    emit(["ok": false, "error": msg])
    exit(code)
}

@main
struct Helper {
    static func main() async {
        let args = CommandLine.arguments
        guard args.count >= 2 else { fail("usage: clawdyx-screenshot preflight|request|capture <displayID> <out.png>", code: 2) }
        switch args[1] {
        case "preflight":
            emit(["ok": true, "granted": CGPreflightScreenCaptureAccess()])
        case "request":
            emit(["ok": true, "granted": CGRequestScreenCaptureAccess()])
        case "capture":
            guard args.count == 4, let id = UInt32(args[2]) else { fail("usage: capture <displayID> <out.png>", code: 2) }
            await capture(displayID: id, out: URL(fileURLWithPath: args[3]))
        default:
            fail("unknown command", code: 2)
        }
    }

    static func capture(displayID: UInt32, out: URL) async {
        if !CGPreflightScreenCaptureAccess() {
            fail("screen recording permission missing", code: 3)
        }
        do {
            let content = try await SCShareableContent.excludingDesktopWindows(false, onScreenWindowsOnly: true)
            guard let display = content.displays.first(where: { $0.displayID == displayID }) else {
                fail("no such display", code: 4)
            }
            let filter = SCContentFilter(display: display, excludingWindows: [])
            let cfg = SCStreamConfiguration()
            var scale: CGFloat = 1
            if let mode = CGDisplayCopyDisplayMode(displayID), display.width > 0 {
                scale = CGFloat(mode.pixelWidth) / CGFloat(display.width)
            }
            cfg.width = Int(CGFloat(display.width) * scale)
            cfg.height = Int(CGFloat(display.height) * scale)
            cfg.showsCursor = false
            let image = try await SCScreenshotManager.captureImage(contentFilter: filter, configuration: cfg)
            guard let dest = CGImageDestinationCreateWithURL(out as CFURL, UTType.png.identifier as CFString, 1, nil) else {
                fail("cannot write output")
            }
            CGImageDestinationAddImage(dest, image, nil)
            guard CGImageDestinationFinalize(dest) else { fail("cannot write output") }
            emit(["ok": true, "width": image.width, "height": image.height])
        } catch {
            fail("capture failed: \(error.localizedDescription)")
        }
    }
}
