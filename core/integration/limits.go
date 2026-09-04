package integration

const (
	maxRequestBodyBytes  = 8 * 1024 * 1024
	maxArtworkBodyBytes  = 20 * 1024 * 1024
	maxResponseBodyBytes = maxRequestBodyBytes
)

func maxRequestBody(dest Destination) int64 {
	if dest == DestArtwork {
		return maxArtworkBodyBytes
	}
	return maxRequestBodyBytes
}

func maxResponseBody(dest Destination) int64 {
	if dest == DestArtwork {
		return maxArtworkBodyBytes
	}
	return maxResponseBodyBytes
}
