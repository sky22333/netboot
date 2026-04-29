package bootmenu

import (
	"crypto/rand"
	"math/big"

	"pxe/internal/storage"
)

func TimeoutSeconds(menu storage.Menu) int {
	timeout := clamp(menu.TimeoutSeconds, 0, 255)
	if !menu.RandomizeTimeout || timeout == 0 {
		return timeout
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(timeout+1)))
	if err != nil {
		return timeout
	}
	return int(n.Int64())
}

func TimeoutMillis(menu storage.Menu) int {
	return TimeoutSeconds(menu) * 1000
}

func clamp(v, minValue, maxValue int) int {
	if v < minValue {
		return minValue
	}
	if v > maxValue {
		return maxValue
	}
	return v
}
