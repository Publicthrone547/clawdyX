// clawdyx-camera: a tiny AVFoundation camera helper shipped inside
// clawdyX.app (Contents/Helpers). The worker starts it with fixed arguments;
// it prints one JSON object on stdout and exits.
//
//   clawdyx-camera list                              -> {"cameras":[{"id","name","builtin"}]} (no permission needed, nothing captured)
//   clawdyx-camera preflight                         -> {"status":"authorized|denied|restricted|notDetermined"}
//   clawdyx-camera photo <cameraID> <out.jpg>        -> writes one JPEG
//   clawdyx-camera video <cameraID> <seconds> <out>  -> records a clip without sound (H.264, MP4)
//
// photo and video run only when started by the clawdyx worker (its parent
// process), so a shell started by Claude cannot use the camera through it.
// They ask macOS for the Camera permission if it was never asked (the
// dialog appears on the Mac). Nothing is kept: the worker reads the file
// and deletes it.
import AVFoundation
import Darwin
import Foundation

func emit(_ obj: [String: Any]) {
    if let data = try? JSONSerialization.data(withJSONObject: obj), let s = String(data: data, encoding: .utf8) {
        print(s)
    }
}

func fail(_ msg: String, code: Int32 = 1) -> Never {
    emit(["ok": false, "error": msg])
    exit(code)
}

let maxSeconds = 60

func devices() -> [AVCaptureDevice] {
    let types: [AVCaptureDevice.DeviceType] = [.builtInWideAngleCamera, .external, .continuityCamera, .deskViewCamera]
    return AVCaptureDevice.DiscoverySession(deviceTypes: types, mediaType: .video, position: .unspecified).devices
}

func statusName(_ s: AVAuthorizationStatus) -> String {
    switch s {
    case .authorized: return "authorized"
    case .denied: return "denied"
    case .restricted: return "restricted"
    default: return "notDetermined"
    }
}

/// The parent must be the clawdyx worker.
func parentIsWorker() -> Bool {
    var buf = [CChar](repeating: 0, count: Int(MAXPATHLEN) * 4)
    guard proc_pidpath(getppid(), &buf, UInt32(buf.count)) > 0 else { return false }
    return URL(fileURLWithPath: String(cString: buf)).lastPathComponent == "clawdyx"
}

func authorize() async {
    var status = AVCaptureDevice.authorizationStatus(for: .video)
    if status == .notDetermined {
        _ = await AVCaptureDevice.requestAccess(for: .video)
        status = AVCaptureDevice.authorizationStatus(for: .video)
    }
    if status != .authorized {
        fail("camera permission missing (\(statusName(status)))", code: 3)
    }
}

func session(for id: String, preset: AVCaptureSession.Preset) -> (AVCaptureSession, AVCaptureDevice) {
    guard let dev = devices().first(where: { $0.uniqueID == id }) else { fail("no such camera", code: 4) }
    let s = AVCaptureSession()
    s.beginConfiguration()
    if s.canSetSessionPreset(preset) {
        s.sessionPreset = preset
    }
    guard let input = try? AVCaptureDeviceInput(device: dev), s.canAddInput(input) else {
        fail("cannot open the camera (it may be in use by another app)")
    }
    s.addInput(input)
    return (s, dev)
}

final class PhotoDelegate: NSObject, AVCapturePhotoCaptureDelegate {
    var done: ((Data?, Error?) -> Void)?
    func photoOutput(_ output: AVCapturePhotoOutput, didFinishProcessingPhoto photo: AVCapturePhoto, error: Error?) {
        done?(photo.fileDataRepresentation(), error)
        done = nil
    }
}

final class MovieDelegate: NSObject, AVCaptureFileOutputRecordingDelegate {
    var done: ((Error?) -> Void)?
    func fileOutput(_ output: AVCaptureFileOutput, didFinishRecordingTo url: URL, from connections: [AVCaptureConnection], error: Error?) {
        var err = error
        // Reaching maxRecordedDuration is reported as an "error" that still finished the file.
        if let e = error as NSError?, (e.userInfo[AVErrorRecordingSuccessfullyFinishedKey] as? Bool) == true {
            err = nil
        }
        done?(err)
        done = nil
    }
}

@main
struct Helper {
    static func main() async {
        let args = CommandLine.arguments
        guard args.count >= 2 else { fail("usage: clawdyx-camera list|preflight|photo <id> <out>|video <id> <seconds> <out>", code: 2) }
        switch args[1] {
        case "list":
            let cams = devices().map { d -> [String: Any] in
                ["id": d.uniqueID, "name": d.localizedName, "builtin": d.deviceType == .builtInWideAngleCamera]
            }
            emit(["ok": true, "cameras": cams])
        case "preflight":
            emit(["ok": true, "status": statusName(AVCaptureDevice.authorizationStatus(for: .video))])
        case "photo":
            guard args.count == 4 else { fail("usage: photo <cameraID> <out.jpg>", code: 2) }
            guard parentIsWorker() else { fail("only the clawdyx worker may use the camera", code: 5) }
            await authorize()
            await photo(id: args[2], out: URL(fileURLWithPath: args[3]))
        case "video":
            guard args.count == 5, let secs = Int(args[3]), secs >= 1, secs <= maxSeconds else {
                fail("usage: video <cameraID> <seconds 1-\(maxSeconds)> <out.mp4>", code: 2)
            }
            guard parentIsWorker() else { fail("only the clawdyx worker may use the camera", code: 5) }
            await authorize()
            await video(id: args[2], seconds: secs, out: URL(fileURLWithPath: args[4]))
        default:
            fail("unknown command", code: 2)
        }
    }

    static func photo(id: String, out: URL) async {
        let (s, _) = session(for: id, preset: .photo)
        let output = AVCapturePhotoOutput()
        guard s.canAddOutput(output) else { fail("cannot take photos with this camera") }
        s.addOutput(output)
        s.commitConfiguration()
        s.startRunning()
        defer { s.stopRunning() }
        // Give auto exposure and white balance a moment.
        try? await Task.sleep(nanoseconds: 1_200_000_000)
        let delegate = PhotoDelegate()
        let data: Data? = await withCheckedContinuation { cont in
            delegate.done = { d, _ in cont.resume(returning: d) }
            output.capturePhoto(with: AVCapturePhotoSettings(format: [AVVideoCodecKey: AVVideoCodecType.jpeg]), delegate: delegate)
        }
        guard let data, !data.isEmpty else { fail("the camera returned no image") }
        do {
            try data.write(to: out)
        } catch {
            fail("cannot write output")
        }
        emit(["ok": true, "bytes": data.count])
    }

    static func video(id: String, seconds: Int, out: URL) async {
        let (s, _) = session(for: id, preset: .hd1280x720)
        let output = AVCaptureMovieFileOutput()
        guard s.canAddOutput(output) else { fail("cannot record video with this camera") }
        s.addOutput(output)
        output.maxRecordedDuration = CMTime(seconds: Double(seconds), preferredTimescale: 600)
        if let conn = output.connection(with: .video) {
            // H.264 at about 2.5 Mbit/s keeps a 60 s clip near 20 MB (Telegram
            // bots may send up to 50 MB). macOS accepts compression properties here.
            output.setOutputSettings([
                AVVideoCodecKey: AVVideoCodecType.h264,
                AVVideoCompressionPropertiesKey: [AVVideoAverageBitRateKey: 2_500_000],
            ], for: conn)
        }
        s.commitConfiguration()
        s.startRunning()
        defer { s.stopRunning() }
        try? await Task.sleep(nanoseconds: 800_000_000)
        let mov = out.deletingPathExtension().appendingPathExtension("mov")
        let delegate = MovieDelegate()
        let err: Error? = await withCheckedContinuation { cont in
            delegate.done = { e in cont.resume(returning: e) }
            output.startRecording(to: mov, recordingDelegate: delegate)
        }
        if let err { fail("recording failed: \(err.localizedDescription)") }
        // AVCaptureMovieFileOutput writes QuickTime; Telegram plays MP4 inline.
        // Re-wrap without re-encoding.
        defer { try? FileManager.default.removeItem(at: mov) }
        guard let export = AVAssetExportSession(asset: AVURLAsset(url: mov), presetName: AVAssetExportPresetPassthrough) else {
            fail("cannot convert the clip")
        }
        do {
            try await export.export(to: out, as: .mp4)
        } catch {
            fail("cannot convert the clip: \(error.localizedDescription)")
        }
        let size = (try? FileManager.default.attributesOfItem(atPath: out.path)[.size] as? Int) ?? 0
        emit(["ok": true, "bytes": size, "seconds": seconds])
    }
}
