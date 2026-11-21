package rating

import "github.com/kaptree/kapoj/apps/server/internal/ratinglevel"

// Level is the public rating tier returned by the API. It lives in
// ratinglevel so packages that cannot import rating (due to service-layer
// dependencies) can share the exact same type and color definitions.
type Level = ratinglevel.Level

// LevelFor maps a rating to its tier. Negative ratings are treated as zero.
func LevelFor(rating int) Level {
	return ratinglevel.LevelFor(rating)
}
