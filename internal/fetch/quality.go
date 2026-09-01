package fetch

// Quality ladder (Cobalt-style height buckets). Never upscale — pick ≤ want.
var qualityBuckets = []int{144, 240, 360, 480, 720, 1080, 1440, 2160, 4320}

// SnapHeight maps a requested max height onto the nearest allowed bucket ≤ want.
// want<=0 defaults to 480 (fast default for Drive).
func SnapHeight(want int) int {
	if want <= 0 {
		return 480
	}
	best := qualityBuckets[0]
	for _, b := range qualityBuckets {
		if b <= want {
			best = b
		}
	}
	return best
}
