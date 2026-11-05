package rating_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kaptree/kapoj/apps/server/internal/rating"
)

func TestLevelBoundaries(t *testing.T) {
	cases := []struct {
		rating int
		key    string
	}{
		{-1, "gray"},
		{0, "gray"}, {1199, "gray"},
		{1200, "green"}, {1399, "green"},
		{1400, "cyan"}, {1599, "cyan"},
		{1600, "blue"}, {1899, "blue"},
		{1900, "purple"}, {2099, "purple"},
		{2100, "orange"}, {2399, "orange"},
		{2400, "red"}, {2599, "red"},
		{2600, "black_red"},
	}
	for _, c := range cases {
		assert.Equal(t, c.key, rating.LevelFor(c.rating).Key, "rating %d", c.rating)
	}
}

func TestLevelNamesAndColors(t *testing.T) {
	cases := []struct {
		rating int
		name   string
		color  string
	}{
		{1000, "灰名", "#808080"},
		{1200, "绿名", "#008000"},
		{1400, "青名", "#03A89E"},
		{1600, "蓝名", "#0000FF"},
		{1900, "紫名", "#AA00AA"},
		{2100, "橙名", "#FF8C00"},
		{2400, "红名", "#FF0000"},
		{2600, "黑红名", "#000000"},
	}
	for _, c := range cases {
		level := rating.LevelFor(c.rating)
		assert.Equal(t, c.name, level.Name, "rating %d", c.rating)
		assert.Equal(t, c.color, level.Color, "rating %d", c.rating)
		if c.rating < 2600 {
			assert.Empty(t, level.AccentColor, "only black_red has an accent")
		}
	}
}

func TestBlackRedLevelHasAccent(t *testing.T) {
	l := rating.LevelFor(2600)
	assert.Equal(t, "#000000", l.Color)
	assert.Equal(t, "#FF0000", l.AccentColor)
}

func TestLevelJSONShape(t *testing.T) {
	data, err := json.Marshal(rating.LevelFor(1500))
	assert.NoError(t, err)
	assert.JSONEq(t, `{"key":"cyan","name":"青名","color":"#03A89E","accentColor":""}`, string(data))

	data, err = json.Marshal(rating.LevelFor(3000))
	assert.NoError(t, err)
	assert.JSONEq(t, `{"key":"black_red","name":"黑红名","color":"#000000","accentColor":"#FF0000"}`, string(data))
}
