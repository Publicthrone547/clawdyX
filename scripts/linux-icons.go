//go:build ignore

// linux-icons writes the hicolor icon sizes of the Linux app from
// desktop/build/appicon.png: go run scripts/linux-icons.go
package main

import (
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"golang.org/x/image/draw"
)

func main() {
	f, err := os.Open(filepath.Join("desktop", "build", "appicon.png"))
	if err != nil {
		log.Fatal(err)
	}
	src, err := png.Decode(f)
	f.Close()
	if err != nil {
		log.Fatal(err)
	}
	for _, n := range []int{32, 64, 128, 256, 512} {
		dst := image.NewRGBA(image.Rect(0, 0, n, n))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
		s := strconv.Itoa(n)
		dir := filepath.Join("desktop", "build", "linux", "icons", "hicolor", s+"x"+s, "apps")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatal(err)
		}
		out, err := os.Create(filepath.Join(dir, "clawdyx.png"))
		if err != nil {
			log.Fatal(err)
		}
		if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(out, dst); err != nil {
			log.Fatal(err)
		}
		out.Close()
	}
}
