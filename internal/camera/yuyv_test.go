package camera

import (
	"bytes"
	"image/jpeg"
	"testing"
)

func TestYUYVToJPEG(t *testing.T) {
	const w, h, stride = 4, 2, 10 // two padding bytes per row
	frame := make([]byte, stride*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w/2; x++ {
			copy(frame[y*stride+4*x:], []byte{byte(50 + 100*y), 128, byte(60 + 100*y), 128})
		}
	}
	img, err := YUYVToImage(frame, w, h, stride)
	if err != nil {
		t.Fatal(err)
	}
	if img.Y[0] != 50 || img.Y[1] != 60 || img.Y[img.YStride] != 150 || img.Cb[0] != 128 || img.Cr[img.CStride] != 128 {
		t.Fatalf("planes Y=%v Cb=%v Cr=%v", img.Y, img.Cb, img.Cr)
	}
	b, err := YUYVToJPEG(frame, w, h, stride, 85)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(b))
	if err != nil || cfg.Width != w || cfg.Height != h {
		t.Fatalf("jpeg %v %+v", err, cfg)
	}
	if err := CheckJPEG(b); err != nil {
		t.Fatal(err)
	}
	if _, err := YUYVToImage(frame[:15], w, h, stride); err == nil {
		t.Fatal("a short frame was accepted")
	}
	if _, err := YUYVToImage(frame, 3, h, stride); err == nil {
		t.Fatal("an odd width was accepted")
	}
	if CheckJPEG([]byte("not a jpeg")) == nil {
		t.Fatal("garbage passed as JPEG")
	}
}
