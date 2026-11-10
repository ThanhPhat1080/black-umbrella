// Package avatar draws deterministic 5x5 identicons from a username.
package avatar

import (
	"crypto/sha256"
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func hslToRGB(h, s, l float64) (uint8, uint8, uint8) {
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return uint8(math.Round((r + m) * 255)), uint8(math.Round((g + m) * 255)), uint8(math.Round((b + m) * 255))
}

// Grid derives the colour (#rrggbb) and a vertically symmetric 5x5 pattern (15 bits) from SHA-256(username).
func Grid(username string) (grid [5][5]bool, color string) {
	h := sha256.Sum256([]byte(username))
	color = hexHSL(Hue(username), 0.65, 0.6)
	bits := uint16(h[2])<<8 | uint16(h[3])
	for i := 0; i < 15; i++ {
		row, col := i/3, i%3
		on := bits>>uint(i)&1 == 1
		grid[row][col] = on
		grid[row][4-col] = on
	}
	return grid, color
}

// Render returns 3 plain lines of 10 cells (half-block rows (0,1),(2,3),(4,-); each pixel is 2 cells wide).
func Render(username string) []string {
	g, _ := Grid(username)
	lines := make([]string, 3)
	for l := 0; l < 3; l++ {
		var sb strings.Builder
		for c := 0; c < 5; c++ {
			top := g[2*l][c]
			bottom := false
			if 2*l+1 < 5 {
				bottom = g[2*l+1][c]
			}
			ch := " "
			switch {
			case top && bottom:
				ch = "█"
			case top:
				ch = "▀"
			case bottom:
				ch = "▄"
			}
			sb.WriteString(ch + ch)
		}
		lines[l] = sb.String()
	}
	return lines
}

// Color returns the avatar colour as #rrggbb.
func Color(username string) string {
	_, c := Grid(username)
	return c
}

// Dot is a coloured ■ for message lines.
func Dot(username string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(Color(username))).Render("■")
}

// Hue is the user's identicon hue in degrees [0, 360).
func Hue(username string) float64 {
	h := sha256.Sum256([]byte(username))
	return float64(uint16(h[0])<<8|uint16(h[1])) / 65536 * 360
}

func hexHSL(h, s, l float64) string {
	r, g, b := hslToRGB(h, s, l)
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// NameColor is the colour of the user's display name: the avatar hue with clamped saturation
// and lightness, lighter on dark terminals (identical to the avatar colour) and darker on light ones.
func NameColor(username string) lipgloss.AdaptiveColor {
	return lipgloss.AdaptiveColor{Dark: hexHSL(Hue(username), 0.65, 0.6), Light: hexHSL(Hue(username), 0.65, 0.4)}
}

// quadrants maps TL | TR<<1 | BL<<2 | BR<<3 to the matching quadrant block character.
var quadrants = []rune{' ', '▘', '▝', '▀', '▖', '▌', '▞', '▛', '▗', '▚', '▐', '▜', '▄', '▙', '▟', '█'}

// Mini is a one-line, 3-cell identicon (2x2 quadrant blocks per cell) folded from the 5x5 grid.
// It is left/right symmetric like the full identicon. Colour it with Color(username).
func Mini(username string) string {
	g, _ := Grid(username)
	var top, bot [3]int
	for c := 0; c < 3; c++ {
		top[c] = b2i(g[0][c]) ^ b2i(g[1][c])
		bot[c] = b2i(g[2][c]) ^ b2i(g[3][c]) ^ b2i(g[4][c])
	}
	// Sub-columns s0..s5 mirror: s5=s0, s4=s1, s3=s2.
	sub := [6]int{0, 1, 2, 2, 1, 0}
	var out []rune
	for cell := 0; cell < 3; cell++ {
		l, r := sub[cell*2], sub[cell*2+1]
		idx := top[l] | top[r]<<1 | bot[l]<<2 | bot[r]<<3
		out = append(out, quadrants[idx])
	}
	return string(out)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
