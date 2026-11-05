package rating

// Level is the public rating tier returned by the API.
type Level struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	AccentColor string `json:"accentColor"`
}

var (
	levelGray     = Level{Key: "gray", Name: "灰名", Color: "#808080"}
	levelGreen    = Level{Key: "green", Name: "绿名", Color: "#008000"}
	levelCyan     = Level{Key: "cyan", Name: "青名", Color: "#03A89E"}
	levelBlue     = Level{Key: "blue", Name: "蓝名", Color: "#0000FF"}
	levelPurple   = Level{Key: "purple", Name: "紫名", Color: "#AA00AA"}
	levelOrange   = Level{Key: "orange", Name: "橙名", Color: "#FF8C00"}
	levelRed      = Level{Key: "red", Name: "红名", Color: "#FF0000"}
	levelBlackRed = Level{Key: "black_red", Name: "黑红名", Color: "#000000", AccentColor: "#FF0000"}
)

// LevelFor maps a rating to its tier. Negative ratings are treated as zero.
func LevelFor(rating int) Level {
	switch {
	case rating >= 2600:
		return levelBlackRed
	case rating >= 2400:
		return levelRed
	case rating >= 2100:
		return levelOrange
	case rating >= 1900:
		return levelPurple
	case rating >= 1600:
		return levelBlue
	case rating >= 1400:
		return levelCyan
	case rating >= 1200:
		return levelGreen
	default:
		return levelGray
	}
}
