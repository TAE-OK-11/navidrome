package artwork

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"image/png"
)

// createAnimatedGIF creates a minimal animated GIF with the given number of frames.
func createAnimatedGIF(frames int) []byte {
	g := &gif.GIF{
		LoopCount: 0,
	}
	for range frames {
		img := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White})
		g.Image = append(g.Image, img)
		g.Delay = append(g.Delay, 10)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func writeUint32LE(buf *bytes.Buffer, v uint32) {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	buf.Write(b)
}

func writeUint32BE(buf *bytes.Buffer, v uint32) {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	buf.Write(b)
}

func createAnimatedWebPBytes() []byte {
	var buf bytes.Buffer
	buf.WriteString("RIFF")
	writeUint32LE(&buf, 100)
	buf.WriteString("WEBP")
	buf.WriteString("VP8X")
	writeUint32LE(&buf, 10)
	buf.Write(make([]byte, 10))
	buf.WriteString("ANIM")
	writeUint32LE(&buf, 6)
	buf.Write(make([]byte, 6))
	buf.WriteString("ANMF")
	writeUint32LE(&buf, 16)
	buf.Write(make([]byte, 16))
	return buf.Bytes()
}

func createAPNGBytes() []byte {
	staticPNG := createStaticPNGBytes()
	ihdrEnd := 8 + 25
	var buf bytes.Buffer
	buf.Write(staticPNG[:ihdrEnd])
	writeUint32BE(&buf, 8)
	buf.WriteString("acTL")
	writeUint32BE(&buf, 2)
	writeUint32BE(&buf, 0)
	writeUint32BE(&buf, 0)
	buf.Write(staticPNG[ihdrEnd:])
	return buf.Bytes()
}

func createStaticPNGBytes() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
