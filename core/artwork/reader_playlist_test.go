package artwork

import (
	"testing"
)

func TestImageWorkerFillRequestJSON(t *testing.T) {
	t.Parallel()

	req := imageWorkerRequest{InputSize: 12, Size: 300, Fill: true, Quality: 75, Format: "png"}
	if !req.Fill || req.Square {
		t.Fatalf("fill request = %#v", req)
	}
}

func TestImageWorkerMosaicRequestJSON(t *testing.T) {
	t.Parallel()

	req := imageWorkerRequest{
		InputSizes: []int{10, 20, 30, 40},
		Mosaic:     true,
		Size:       600,
		Quality:    75,
		Format:     "png",
	}
	if !req.Mosaic || len(req.InputSizes) != 4 || req.InputSize != 0 {
		t.Fatalf("mosaic request = %#v", req)
	}
}
