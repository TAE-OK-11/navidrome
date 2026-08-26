package artwork

import (
	"image"
	"image/color"
	"testing"
)

func TestFillCenterCropsWiderSources(t *testing.T) {
	t.Parallel()

	src := image.NewNRGBA(image.Rect(0, 0, 80, 40))
	for y := range 40 {
		for x := range 80 {
			src.Set(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	got := fillCenter(src, 20, 20)
	if got.Bounds().Dx() != 20 || got.Bounds().Dy() != 20 {
		t.Fatalf("fillCenter bounds = %v", got.Bounds())
	}
	if color.NRGBAModel.Convert(got.At(0, 0)).(color.NRGBA).A == 0 {
		t.Fatal("filled tile should be opaque after center crop")
	}
}

func TestImageWorkerFillRequestJSON(t *testing.T) {
	t.Parallel()

	req := imageWorkerRequest{InputSize: 12, Size: 300, Fill: true, Quality: 75, Format: "png"}
	if !req.Fill || req.Square {
		t.Fatalf("fill request = %#v", req)
	}
}
