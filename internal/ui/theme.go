package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Palette: Voi violet with a cyan accent, on the terminal's own (dark) background.
var (
	cViolet  = lipgloss.Color("#A78BFA")
	cDeep    = lipgloss.Color("#7C3AED")
	cCyan    = lipgloss.Color("#22D3EE")
	cOK      = lipgloss.Color("#34D399")
	cWarn    = lipgloss.Color("#FBBF24")
	cBad     = lipgloss.Color("#F87171")
	cText    = lipgloss.Color("#E5E7EB")
	cMuted   = lipgloss.Color("#9CA3AF")
	cDim     = lipgloss.Color("#4B5563")
	cTrack   = lipgloss.Color("#374151")
	cSelBg   = lipgloss.Color("#2E1065")
	cPanel   = lipgloss.Color("#5B21B6")
	cOnColor = lipgloss.Color("#111827")
)

var (
	sText     = lipgloss.NewStyle().Foreground(cText)
	sMuted    = lipgloss.NewStyle().Foreground(cMuted)
	sDim      = lipgloss.NewStyle().Foreground(cDim)
	sBold     = lipgloss.NewStyle().Foreground(cText).Bold(true)
	sAccent   = lipgloss.NewStyle().Foreground(cViolet)
	sCyan     = lipgloss.NewStyle().Foreground(cCyan)
	sOK       = lipgloss.NewStyle().Foreground(cOK)
	sWarn     = lipgloss.NewStyle().Foreground(cWarn)
	sBad      = lipgloss.NewStyle().Foreground(cBad)
	sSection  = lipgloss.NewStyle().Foreground(cViolet).Bold(true)
	sColHead  = lipgloss.NewStyle().Foreground(cDim).Bold(true)
	sKeycap   = lipgloss.NewStyle().Foreground(cText).Background(cTrack).Padding(0, 1)
	sKeyLabel = lipgloss.NewStyle().Foreground(cMuted)
)

func pill(text string, bg lipgloss.Color) string {
	return lipgloss.NewStyle().Foreground(cOnColor).Background(bg).Bold(true).Padding(0, 1).Render(text)
}

func panel(border lipgloss.Color, width int) lipgloss.Style {
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).Padding(0, 1).Width(max(1, width-2))
}

// gradient colours each rune along a fixed violet-to-cyan ramp.
func gradient(s string) string {
	ramp := []string{"#C4B5FD", "#A78BFA", "#8B5CF6", "#7C8CF8", "#60A5FA", "#38BDF8", "#22D3EE"}
	runes := []rune(s)
	var b strings.Builder
	for i, r := range runes {
		c := ramp[min(len(ramp)-1, i*len(ramp)/max(1, len(runes)))]
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(c)).Bold(true).Render(string(r)))
	}
	return b.String()
}

func commas(n uint64) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// compactVOI renders microVOI as 6.52M / 348.4K / 912.33.
func compactVOI(micro uint64) string {
	v := float64(micro) / 1e6
	switch {
	case v >= 1e9:
		return fmt.Sprintf("%.2fB", v/1e9)
	case v >= 1e6:
		return fmt.Sprintf("%.2fM", v/1e6)
	case v >= 1e3:
		return fmt.Sprintf("%.1fK", v/1e3)
	}
	return fmt.Sprintf("%.2f", v)
}

func exactVOI(micro uint64) string {
	return fmt.Sprintf("%s.%06d", commas(micro/1e6), micro%1e6)
}

// span renders a duration at the precision a human wants for it.
func span(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	switch {
	case d < time.Second:
		return "now"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// bar is a smooth (eighth-block) gauge of frac in [0,1].
func bar(frac float64, width int, color lipgloss.Color) string {
	return barOn(frac, width, color, lipgloss.NoColor{})
}

// barOn is bar drawn over a background, so it can sit inside a highlighted row.
func barOn(frac float64, width int, color lipgloss.Color, bg lipgloss.TerminalColor) string {
	if width <= 0 {
		return ""
	}
	if math.IsNaN(frac) {
		frac = 0
	}
	frac = math.Max(0, math.Min(1, frac))
	eighths := []string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}
	cells := frac * float64(width)
	full := int(cells)
	part := eighths[int((cells-float64(full))*8)]
	filled := strings.Repeat("█", full) + part
	rest := width - full
	if part != "" {
		rest--
	}
	return lipgloss.NewStyle().Foreground(color).Background(bg).Render(filled) + lipgloss.NewStyle().Foreground(cTrack).Background(bg).Render(strings.Repeat("━", max(0, rest)))
}

// spark draws recent block times; taller is slower.
func spark(samples []time.Duration, n int) string {
	if len(samples) == 0 {
		return ""
	}
	if len(samples) > n {
		samples = samples[len(samples)-n:]
	}
	lo, hi := samples[0], samples[0]
	for _, s := range samples {
		lo, hi = min(lo, s), max(hi, s)
	}
	levels := []rune("▁▂▃▄▅▆▇█")
	var b strings.Builder
	for _, s := range samples {
		i := 0
		if hi > lo {
			i = int(float64(s-lo) / float64(hi-lo) * float64(len(levels)-1))
		}
		b.WriteRune(levels[i])
	}
	return sCyan.Render(b.String())
}

// lifeColor grades the time a key has left.
func lifeColor(left time.Duration, known bool) lipgloss.Color {
	switch {
	case !known:
		return cViolet
	case left < 14*24*time.Hour:
		return cBad
	case left < 30*24*time.Hour:
		return cWarn
	}
	return cOK
}

func keycaps(pairs ...string) string {
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, sKeycap.Render(pairs[i])+" "+sKeyLabel.Render(pairs[i+1]))
	}
	return strings.Join(parts, "  ")
}

// spread puts left and right on one line of the given width.
func spread(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return left + " " + right
	}
	return left + strings.Repeat(" ", gap) + right
}

// flow joins segments with sep, breaking lines only between segments.
func flow(segments []string, sep string, width int) string {
	var lines []string
	line := ""
	for _, seg := range segments {
		switch {
		case line == "":
			line = seg
		case lipgloss.Width(line)+lipgloss.Width(sep)+lipgloss.Width(seg) <= width:
			line += sep + seg
		default:
			lines = append(lines, line)
			line = seg
		}
	}
	return strings.Join(append(lines, line), "\n")
}
