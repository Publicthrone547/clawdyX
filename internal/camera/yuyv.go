package camera

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
)

// Helpers of the V4L2 provider (camera on Linux), shared so they are tested
// on every OS.

// YUYVToImage converts a packed YUYV 4:2:2 frame (Y0 U Y1 V per two
// pixels; stride bytes per row) to an image.YCbCr without copying colors.
func YUYVToImage(frame []byte, w, h, stride int) (*image.YCbCr, error) {
	if w <= 0 || h <= 0 || w%2 != 0 {
		return nil, errors.New("bad YUYV frame size")
	}
	if stride < 2*w {
		stride = 2 * w
	}
	if len(frame) < stride*(h-1)+2*w {
		return nil, errors.New("the YUYV frame is shorter than its size")
	}
	img := image.NewYCbCr(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio422)
	for y := 0; y < h; y++ {
		row := frame[y*stride : y*stride+2*w]
		yo, co := y*img.YStride, y*img.CStride
		for x := 0; x < w/2; x++ {
			p := row[4*x : 4*x+4]
			img.Y[yo+2*x] = p[0]
			img.Cb[co+x] = p[1]
			img.Y[yo+2*x+1] = p[2]
			img.Cr[co+x] = p[3]
		}
	}
	return img, nil
}

// YUYVToJPEG converts a YUYV frame to a JPEG of the given quality.
func YUYVToJPEG(frame []byte, w, h, stride, quality int) ([]byte, error) {
	img, err := YUYVToImage(frame, w, h, stride)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// CheckJPEG reports whether a camera's MJPEG frame decodes (UVC cameras may
// leave out the Huffman tables, which most decoders then refuse).
func CheckJPEG(b []byte) error {
	if len(b) < 4 || b[0] != 0xff || b[1] != 0xd8 {
		return errors.New("the camera returned no JPEG")
	}
	_, err := jpeg.Decode(bytes.NewReader(b))
	return err
}
