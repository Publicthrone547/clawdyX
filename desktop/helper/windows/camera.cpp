// clawdyx-camera.exe: the Windows camera helper of clawdyX (Media
// Foundation). It has the same command line and output as the macOS helper
// (desktop/helper/camera.swift): one JSON object on stdout, then exit.
//
//   clawdyx-camera list                              -> {"ok":true,"cameras":[{"id","name","builtin"}]} (nothing captured)
//   clawdyx-camera preflight                         -> {"ok":true,"status":"authorized|denied"}
//   clawdyx-camera photo <cameraID> <out.jpg>        -> writes one JPEG
//   clawdyx-camera video <cameraID> <seconds> <out>  -> records a clip without sound (H.264, MP4, about 2.5 Mbit/s)
//
// photo and video run only when the parent process is clawdyx.exe (the
// worker), so a shell started by Claude cannot use the camera through it.
// Windows does not ask desktop apps for the camera; the switch "Let desktop
// apps access your camera" in Windows Settings decides (preflight reads it).
//
// Build: build.ps1 (MSVC, the C runtime linked statically).

#define WIN32_LEAN_AND_MEAN
#define NOMINMAX
#include <windows.h>
#include <mfapi.h>
#include <mfidl.h>
#include <mfreadwrite.h>
#include <mferror.h>
#include <wincodec.h>
#include <tlhelp32.h>
#include <shlwapi.h>

#include <cstdio>
#include <cstdlib>
#include <string>
#include <vector>

#pragma comment(lib, "mfplat.lib")
#pragma comment(lib, "mf.lib")
#pragma comment(lib, "mfreadwrite.lib")
#pragma comment(lib, "mfuuid.lib")
#pragma comment(lib, "ole32.lib")
#pragma comment(lib, "windowscodecs.lib")
#pragma comment(lib, "shlwapi.lib")
#pragma comment(lib, "advapi32.lib")
#pragma comment(lib, "oleaut32.lib")

static const int kMaxSeconds = 60;
static const UINT32 kBitrate = 2500000;

// ---- output ----

static std::string utf8(const std::wstring &w) {
    if (w.empty()) return std::string();
    int n = WideCharToMultiByte(CP_UTF8, 0, w.data(), (int)w.size(), nullptr, 0, nullptr, nullptr);
    std::string s(n, '\0');
    WideCharToMultiByte(CP_UTF8, 0, w.data(), (int)w.size(), &s[0], n, nullptr, nullptr);
    return s;
}

static std::string quote(const std::string &s) {
    std::string out = "\"";
    for (unsigned char c : s) {
        switch (c) {
        case '"': out += "\\\""; break;
        case '\\': out += "\\\\"; break;
        case '\n': out += "\\n"; break;
        case '\r': out += "\\r"; break;
        case '\t': out += "\\t"; break;
        default:
            if (c < 0x20) {
                char b[8];
                snprintf(b, sizeof b, "\\u%04x", c);
                out += b;
            } else {
                out += (char)c;
            }
        }
    }
    return out + "\"";
}

static void emit(const std::string &json) {
    fputs(json.c_str(), stdout);
    fputs("\n", stdout);
    fflush(stdout);
}

[[noreturn]] static void fail(const std::string &msg, int code = 1) {
    emit("{\"ok\":false,\"error\":" + quote(msg) + "}");
    exit(code);
}

static std::string hr(HRESULT h) {
    char b[16];
    snprintf(b, sizeof b, "0x%08lX", (unsigned long)h);
    return b;
}

// ---- COM helpers ----

template <class T> struct Ptr {
    T *p = nullptr;
    ~Ptr() { if (p) p->Release(); }
    T **operator&() { return &p; }
    T *operator->() const { return p; }
    operator T *() const { return p; }
};

struct Session {
    Session() {
        if (FAILED(CoInitializeEx(nullptr, COINIT_MULTITHREADED))) fail("COM could not start");
        if (FAILED(MFStartup(MF_VERSION))) fail("Media Foundation could not start");
    }
    ~Session() {
        MFShutdown();
        CoUninitialize();
    }
};

// ---- devices ----

struct Camera {
    std::wstring id, name;
};

static std::vector<Camera> cameras(std::vector<IMFActivate *> *keep = nullptr) {
    Ptr<IMFAttributes> attr;
    if (FAILED(MFCreateAttributes(&attr, 1))) fail("cannot list cameras");
    attr->SetGUID(MF_DEVSOURCE_ATTRIBUTE_SOURCE_TYPE, MF_DEVSOURCE_ATTRIBUTE_SOURCE_TYPE_VIDCAP_GUID);
    IMFActivate **devs = nullptr;
    UINT32 n = 0;
    HRESULT h = MFEnumDeviceSources(attr, &devs, &n);
    if (FAILED(h)) fail("cannot list cameras (" + hr(h) + ")");
    std::vector<Camera> out;
    for (UINT32 i = 0; i < n; i++) {
        WCHAR *name = nullptr, *link = nullptr;
        UINT32 len = 0;
        Camera c;
        if (SUCCEEDED(devs[i]->GetAllocatedString(MF_DEVSOURCE_ATTRIBUTE_FRIENDLY_NAME, &name, &len))) {
            c.name = name;
            CoTaskMemFree(name);
        }
        if (SUCCEEDED(devs[i]->GetAllocatedString(MF_DEVSOURCE_ATTRIBUTE_SOURCE_TYPE_VIDCAP_SYMBOLIC_LINK, &link, &len))) {
            c.id = link;
            CoTaskMemFree(link);
        }
        out.push_back(c);
        if (keep) keep->push_back(devs[i]);
        else devs[i]->Release();
    }
    CoTaskMemFree(devs);
    return out;
}

// openCamera activates the camera whose symbolic link is id.
static IMFMediaSource *openCamera(const std::wstring &id) {
    std::vector<IMFActivate *> acts;
    std::vector<Camera> cs = cameras(&acts);
    IMFMediaSource *src = nullptr;
    HRESULT h = E_FAIL;
    bool found = false;
    for (size_t i = 0; i < cs.size(); i++) {
        if (!found && _wcsicmp(cs[i].id.c_str(), id.c_str()) == 0) {
            found = true;
            h = acts[i]->ActivateObject(IID_PPV_ARGS(&src));
        }
        acts[i]->Release();
    }
    if (!found) fail("no such camera", 4);
    if (h == E_ACCESSDENIED) fail("camera permission missing (denied)", 3);
    if (FAILED(h)) fail("cannot open the camera (it may be in use by another app; " + hr(h) + ")");
    return src;
}

// ---- consent (Windows Settings → Privacy & security → Camera) ----

static std::wstring regValue(HKEY root, const wchar_t *path) {
    wchar_t buf[64];
    DWORD size = sizeof buf;
    if (RegGetValueW(root, path, L"Value", RRF_RT_REG_SZ, nullptr, buf, &size) != ERROR_SUCCESS) return L"";
    return buf;
}

static std::string consent() {
    const wchar_t *store = L"Software\\Microsoft\\Windows\\CurrentVersion\\CapabilityAccessManager\\ConsentStore\\webcam";
    const wchar_t *desktop = L"Software\\Microsoft\\Windows\\CurrentVersion\\CapabilityAccessManager\\ConsentStore\\webcam\\NonPackaged";
    // A policy (HKLM) or the user's two switches can deny desktop apps.
    if (regValue(HKEY_LOCAL_MACHINE, store) == L"Deny" || regValue(HKEY_CURRENT_USER, store) == L"Deny" ||
        regValue(HKEY_CURRENT_USER, desktop) == L"Deny")
        return "denied";
    return "authorized";
}

// ---- the parent must be the worker ----

static bool parentIsWorker() {
    DWORD self = GetCurrentProcessId(), parent = 0;
    HANDLE snap = CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0);
    if (snap == INVALID_HANDLE_VALUE) return false;
    PROCESSENTRY32W e = {sizeof e};
    for (BOOL ok = Process32FirstW(snap, &e); ok; ok = Process32NextW(snap, &e)) {
        if (e.th32ProcessID == self) {
            parent = e.th32ParentProcessID;
            break;
        }
    }
    CloseHandle(snap);
    if (!parent) return false;
    HANDLE p = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, FALSE, parent);
    if (!p) return false;
    wchar_t path[MAX_PATH * 4];
    DWORD n = MAX_PATH * 4;
    BOOL ok = QueryFullProcessImageNameW(p, 0, path, &n);
    CloseHandle(p);
    return ok && _wcsicmp(PathFindFileNameW(path), L"clawdyx.exe") == 0;
}

// ---- reading frames ----

// pickSize chooses the camera's frame size: the largest at most 1920×1080,
// preferring 16:9, and the frame rate of that mode.
static void pickSize(IMFSourceReader *r, UINT32 *w, UINT32 *h, UINT32 *num, UINT32 *den) {
    *w = 0, *h = 0, *num = 30, *den = 1;
    for (DWORD i = 0;; i++) {
        Ptr<IMFMediaType> t;
        if (FAILED(r->GetNativeMediaType((DWORD)MF_SOURCE_READER_FIRST_VIDEO_STREAM, i, &t))) break;
        UINT32 tw = 0, th = 0, fn = 0, fd = 0;
        if (FAILED(MFGetAttributeSize(t, MF_MT_FRAME_SIZE, &tw, &th))) continue;
        if (tw > 1920 || th > 1080) continue;
        bool wide = tw * 9 == th * 16, curWide = *w * 9 == *h * 16;
        if (tw * th > *w * *h || (tw * th == *w * *h && wide && !curWide)) {
            *w = tw, *h = th;
            if (SUCCEEDED(MFGetAttributeRatio(t, MF_MT_FRAME_RATE, &fn, &fd)) && fn && fd) *num = fn, *den = fd;
        }
    }
    if (!*w) *w = 1280, *h = 720;
}

static IMFSourceReader *reader(IMFMediaSource *src, const GUID &subtype, UINT32 *w, UINT32 *h, UINT32 *num, UINT32 *den) {
    Ptr<IMFAttributes> attr;
    MFCreateAttributes(&attr, 1);
    attr->SetUINT32(MF_SOURCE_READER_ENABLE_VIDEO_PROCESSING, TRUE);
    IMFSourceReader *r = nullptr;
    HRESULT h0 = MFCreateSourceReaderFromMediaSource(src, attr, &r);
    if (FAILED(h0)) fail("cannot read from the camera (" + hr(h0) + ")");
    pickSize(r, w, h, num, den);
    Ptr<IMFMediaType> t;
    MFCreateMediaType(&t);
    t->SetGUID(MF_MT_MAJOR_TYPE, MFMediaType_Video);
    t->SetGUID(MF_MT_SUBTYPE, subtype);
    MFSetAttributeSize(t, MF_MT_FRAME_SIZE, *w, *h);
    HRESULT h1 = r->SetCurrentMediaType((DWORD)MF_SOURCE_READER_FIRST_VIDEO_STREAM, nullptr, t);
    if (FAILED(h1)) {
        // Some cameras only convert at their default size.
        Ptr<IMFMediaType> t2;
        MFCreateMediaType(&t2);
        t2->SetGUID(MF_MT_MAJOR_TYPE, MFMediaType_Video);
        t2->SetGUID(MF_MT_SUBTYPE, subtype);
        h1 = r->SetCurrentMediaType((DWORD)MF_SOURCE_READER_FIRST_VIDEO_STREAM, nullptr, t2);
        if (FAILED(h1)) fail("the camera's picture format cannot be converted (" + hr(h1) + ")");
        Ptr<IMFMediaType> cur;
        if (SUCCEEDED(r->GetCurrentMediaType((DWORD)MF_SOURCE_READER_FIRST_VIDEO_STREAM, &cur))) MFGetAttributeSize(cur, MF_MT_FRAME_SIZE, w, h);
    }
    return r;
}

// readSample returns the next video sample (skipping stream ticks).
static IMFSample *readSample(IMFSourceReader *r, LONGLONG *ts) {
    for (int tries = 0; tries < 200; tries++) {
        DWORD flags = 0;
        IMFSample *s = nullptr;
        HRESULT h = r->ReadSample((DWORD)MF_SOURCE_READER_FIRST_VIDEO_STREAM, 0, nullptr, &flags, ts, &s);
        if (h == E_ACCESSDENIED) fail("camera permission missing (denied)", 3);
        if (FAILED(h)) fail("the camera stopped (" + hr(h) + ")");
        if (flags & MF_SOURCE_READERF_ENDOFSTREAM) fail("the camera stopped sending pictures");
        if (s) return s;
    }
    fail("the camera sent no picture");
}

// ---- photo ----

static void photo(const std::wstring &id, const std::wstring &out) {
    Ptr<IMFMediaSource> src;
    src.p = openCamera(id);
    UINT32 w, h, num, den;
    Ptr<IMFSourceReader> r;
    r.p = reader(src, MFVideoFormat_RGB32, &w, &h, &num, &den);
    LONGLONG ts = 0;
    // Let auto exposure and white balance settle (about a second).
    for (int i = 0; i < 12; i++) {
        IMFSample *s = readSample(r, &ts);
        s->Release();
    }
    Ptr<IMFSample> s;
    s.p = readSample(r, &ts);
    Ptr<IMFMediaBuffer> buf;
    if (FAILED(s->ConvertToContiguousBuffer(&buf))) fail("the camera returned no image");
    BYTE *data = nullptr;
    DWORD len = 0;
    if (FAILED(buf->Lock(&data, nullptr, &len)) || len < (DWORD)(w * h * 4)) fail("the camera returned no image");
    // RGB32 is BGRX; the default stride says whether rows run bottom-up.
    LONG stride = (LONG)(w * 4);
    Ptr<IMFMediaType> cur;
    if (SUCCEEDED(r->GetCurrentMediaType((DWORD)MF_SOURCE_READER_FIRST_VIDEO_STREAM, &cur))) {
        UINT32 st = 0;
        if (SUCCEEDED(cur->GetUINT32(MF_MT_DEFAULT_STRIDE, &st))) stride = (LONG)st;
    }
    std::vector<BYTE> bgr((size_t)w * h * 3);
    for (UINT32 y = 0; y < h; y++) {
        const BYTE *row = stride >= 0 ? data + (size_t)y * stride : data + (size_t)(h - 1 - y) * (size_t)(-stride);
        BYTE *dst = &bgr[(size_t)y * w * 3];
        for (UINT32 x = 0; x < w; x++) {
            dst[x * 3] = row[x * 4], dst[x * 3 + 1] = row[x * 4 + 1], dst[x * 3 + 2] = row[x * 4 + 2];
        }
    }
    buf->Unlock();

    Ptr<IWICImagingFactory> wic;
    if (FAILED(CoCreateInstance(CLSID_WICImagingFactory, nullptr, CLSCTX_INPROC_SERVER, IID_PPV_ARGS(&wic)))) fail("cannot encode the photo");
    Ptr<IWICStream> stream;
    wic->CreateStream(&stream);
    if (FAILED(stream->InitializeFromFilename(out.c_str(), GENERIC_WRITE))) fail("cannot write output");
    Ptr<IWICBitmapEncoder> enc;
    wic->CreateEncoder(GUID_ContainerFormatJpeg, nullptr, &enc);
    enc->Initialize(stream, WICBitmapEncoderNoCache);
    Ptr<IWICBitmapFrameEncode> frame;
    Ptr<IPropertyBag2> props;
    enc->CreateNewFrame(&frame, &props);
    PROPBAG2 opt = {};
    opt.pstrName = (LPOLESTR)L"ImageQuality";
    VARIANT v;
    VariantInit(&v);
    v.vt = VT_R4;
    v.fltVal = 0.85f;
    props->Write(1, &opt, &v);
    frame->Initialize(props);
    frame->SetSize(w, h);
    WICPixelFormatGUID fmt = GUID_WICPixelFormat24bppBGR;
    frame->SetPixelFormat(&fmt);
    if (fmt != GUID_WICPixelFormat24bppBGR) fail("cannot encode the photo");
    if (FAILED(frame->WritePixels(h, w * 3, (UINT)bgr.size(), bgr.data())) || FAILED(frame->Commit()) || FAILED(enc->Commit()))
        fail("cannot encode the photo");
    LARGE_INTEGER zero = {};
    ULARGE_INTEGER size = {};
    stream->Seek(zero, STREAM_SEEK_END, &size);
    emit("{\"ok\":true,\"bytes\":" + std::to_string(size.QuadPart) + "}");
}

// ---- video ----

static void video(const std::wstring &id, int seconds, const std::wstring &out) {
    Ptr<IMFMediaSource> src;
    src.p = openCamera(id);
    UINT32 w, h, num, den;
    Ptr<IMFSourceReader> r;
    r.p = reader(src, MFVideoFormat_NV12, &w, &h, &num, &den);
    w &= ~1u, h &= ~1u;

    Ptr<IMFAttributes> wattr;
    MFCreateAttributes(&wattr, 2);
    wattr->SetUINT32(MF_READWRITE_ENABLE_HARDWARE_TRANSFORMS, TRUE);
    wattr->SetGUID(MF_TRANSCODE_CONTAINERTYPE, MFTranscodeContainerType_MPEG4);
    Ptr<IMFSinkWriter> sw;
    HRESULT h0 = MFCreateSinkWriterFromURL(out.c_str(), nullptr, wattr, &sw);
    if (FAILED(h0)) fail("cannot write output (" + hr(h0) + ")");

    Ptr<IMFMediaType> outT, inT;
    MFCreateMediaType(&outT);
    outT->SetGUID(MF_MT_MAJOR_TYPE, MFMediaType_Video);
    outT->SetGUID(MF_MT_SUBTYPE, MFVideoFormat_H264);
    outT->SetUINT32(MF_MT_AVG_BITRATE, kBitrate);
    outT->SetUINT32(MF_MT_INTERLACE_MODE, MFVideoInterlace_Progressive);
    MFSetAttributeSize(outT, MF_MT_FRAME_SIZE, w, h);
    MFSetAttributeRatio(outT, MF_MT_FRAME_RATE, num, den);
    MFSetAttributeRatio(outT, MF_MT_PIXEL_ASPECT_RATIO, 1, 1);
    DWORD stream = 0;
    HRESULT h1 = sw->AddStream(outT, &stream);
    if (FAILED(h1)) fail("H.264 encoder not available (Windows N editions need the Media Feature Pack; " + hr(h1) + ")");
    MFCreateMediaType(&inT);
    inT->SetGUID(MF_MT_MAJOR_TYPE, MFMediaType_Video);
    inT->SetGUID(MF_MT_SUBTYPE, MFVideoFormat_NV12);
    inT->SetUINT32(MF_MT_INTERLACE_MODE, MFVideoInterlace_Progressive);
    MFSetAttributeSize(inT, MF_MT_FRAME_SIZE, w, h);
    MFSetAttributeRatio(inT, MF_MT_FRAME_RATE, num, den);
    MFSetAttributeRatio(inT, MF_MT_PIXEL_ASPECT_RATIO, 1, 1);
    HRESULT h2 = sw->SetInputMediaType(stream, inT, nullptr);
    if (FAILED(h2)) fail("H.264 encoder not available (Windows N editions need the Media Feature Pack; " + hr(h2) + ")");
    if (FAILED(sw->BeginWriting())) fail("cannot start recording");

    LONGLONG start = -1, ts = 0, limit = (LONGLONG)seconds * 10000000;
    for (int i = 0; i < 5; i++) { // settle
        IMFSample *s = readSample(r, &ts);
        s->Release();
    }
    for (;;) {
        Ptr<IMFSample> s;
        s.p = readSample(r, &ts);
        if (start < 0) start = ts;
        LONGLONG t = ts - start;
        if (t >= limit) break;
        s->SetSampleTime(t);
        HRESULT hw = sw->WriteSample(stream, s);
        if (FAILED(hw)) fail("recording failed (" + hr(hw) + ")");
    }
    if (FAILED(sw->Finalize())) fail("cannot finish the clip");
    WIN32_FILE_ATTRIBUTE_DATA fa;
    ULONGLONG size = 0;
    if (GetFileAttributesExW(out.c_str(), GetFileExInfoStandard, &fa)) size = ((ULONGLONG)fa.nFileSizeHigh << 32) | fa.nFileSizeLow;
    emit("{\"ok\":true,\"bytes\":" + std::to_string(size) + ",\"seconds\":" + std::to_string(seconds) + "}");
}

// ---- main ----

int wmain(int argc, wchar_t **argv) {
    if (argc < 2) fail("usage: clawdyx-camera list|preflight|photo <id> <out>|video <id> <seconds> <out>", 2);
    std::wstring cmd = argv[1];
    if (cmd == L"preflight") {
        emit("{\"ok\":true,\"status\":" + quote(consent()) + "}");
        return 0;
    }
    Session session;
    if (cmd == L"list") {
        std::vector<Camera> cs = cameras();
        std::string json = "{\"ok\":true,\"cameras\":[";
        for (size_t i = 0; i < cs.size(); i++) {
            if (i) json += ",";
            // Windows does not say which camera is built in; with a single
            // camera, that one is.
            json += "{\"id\":" + quote(utf8(cs[i].id)) + ",\"name\":" + quote(utf8(cs[i].name)) + ",\"builtin\":" + (cs.size() == 1 ? "true" : "false") + "}";
        }
        emit(json + "]}");
        return 0;
    }
    if (cmd == L"photo") {
        if (argc != 4) fail("usage: photo <cameraID> <out.jpg>", 2);
        if (!parentIsWorker()) fail("only the clawdyx worker may use the camera", 5);
        if (consent() == "denied") fail("camera permission missing (denied)", 3);
        photo(argv[2], argv[3]);
        return 0;
    }
    if (cmd == L"video") {
        int secs = argc == 5 ? _wtoi(argv[3]) : 0;
        if (secs < 1 || secs > kMaxSeconds) fail("usage: video <cameraID> <seconds 1-60> <out.mp4>", 2);
        if (!parentIsWorker()) fail("only the clawdyx worker may use the camera", 5);
        if (consent() == "denied") fail("camera permission missing (denied)", 3);
        video(argv[2], secs, argv[4]);
        return 0;
    }
    fail("unknown command", 2);
}
