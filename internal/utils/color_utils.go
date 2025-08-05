package utils

import (
	"math/rand"
)

// Available lobby colors - matches Unity client colors
var availableColors = []string{
	"#07a716", "#f8d183", "#b8f0f9", "#c77490", "#0f2af2",
	"#015a87", "#0d72f1", "#15b52b", "#58c93b", "#6611c3",
	"#7cf9de", "#d38ec6", "#daffcb", "#ae48ff", "#e2ce89",
	"#e9f939", "#37fcb5", "#ee4d9d", "#9d93cc", "#c38203",
	"#bb0bc9", "#4e67ff", "#162317", "#ff46d4", "#bb037c",
	"#a76cb2", "#43bb2b", "#ac10cf", "#a2826e", "#7207e4",
	"#b8f5d0", "#5fba27", "#1e4dd5", "#2358cb", "#1e226d",
	"#2b3a8f", "#d08842", "#35970f", "#8cb0d6", "#49e178",
	"#3d08be", "#150d54", "#c6423b", "#37039a", "#7a5e5b",
	"#f95c16", "#b9a01b", "#84eb5e", "#11ec6e", "#da3fe7",
}

func GetAvailableColor(usedColors []string) (string, bool) {
	used := make(map[string]bool)
	for _, color := range usedColors {
		if color != "" {
			used[color] = true
		}
	}

	available := make([]string, 0)
	for _, color := range availableColors {
		if !used[color] {
			available = append(available, color)
		}
	}

	if len(available) == 0 {
		return "", false
	}

	return available[rand.Intn(len(available))], true
}

// GetRandomColor provides a fallback color when no colors are available
func GetRandomColor() string {
	colors := []string{
		"#FF0000", "#00FF00", "#0000FF", "#FFFF00", "#FF00FF", "#00FFFF",
		"#FFA500", "#800080", "#008000", "#000080", "#800000", "#808000",
	}
	return colors[rand.Intn(len(colors))]
}