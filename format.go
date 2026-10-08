package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// formatLatitude renders degrees as the atelier engraves them: 46°12'N.
func formatLatitude(lat float64, h Hemisphere) string {
	suffix := "N"
	if h == Southern {
		suffix = "S"
	}
	d, m := degreesMinutes(lat)
	return fmt.Sprintf("%d°%02d'%s", d, m, suffix)
}

// formatLongitude renders signed degrees east as 6°09'E or 151°12'E.
func formatLongitude(lon float64) string {
	suffix := "E"
	if lon < 0 {
		suffix = "W"
	}
	d, m := degreesMinutes(lon)
	return fmt.Sprintf("%d°%02d'%s", d, m, suffix)
}

func degreesMinutes(v float64) (int, int) {
	v = math.Abs(v)
	d := int(v)
	m := int(math.Round((v - float64(d)) * 60))
	if m == 60 {
		d, m = d+1, 0
	}
	return d, m
}

// formatArc renders degrees as degrees, minutes and seconds of arc.
func formatArc(v float64) string {
	total := int(math.Round(math.Abs(v) * 3600))
	return fmt.Sprintf("%d°%02d'%02d\"", total/3600, total%3600/60, total%60)
}

// groupThousands uses the Swiss apostrophe separator: 186'000.
func groupThousands(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('\'')
		}
		b.WriteRune(r)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func formatCHF(n int) string { return "CHF " + groupThousands(int64(n)) }

func formatYears(v float64) string {
	if math.IsInf(v, 1) || v >= 1e6 {
		return "more than a million years"
	}
	return groupThousands(int64(math.Round(v))) + " years"
}

func formatDrift(secondsPerDay float64) string {
	return fmt.Sprintf("%+.3f s/day", secondsPerDay)
}

// fold lowercases and strips the diacritics that appear in the catalogue, so
// "zurich" finds "Zürich".
var foldReplacer = strings.NewReplacer(
	"à", "a", "á", "a", "â", "a", "ä", "a", "ã", "a", "å", "a",
	"ç", "c", "è", "e", "é", "e", "ê", "e", "ë", "e",
	"ì", "i", "í", "i", "î", "i", "ï", "i", "ñ", "n",
	"ò", "o", "ó", "o", "ô", "o", "ö", "o", "õ", "o", "ø", "o",
	"ù", "u", "ú", "u", "û", "u", "ü", "u", "ý", "y", "ÿ", "y",
	"°", " ", "'", " ",
)

func fold(s string) string { return foldReplacer.Replace(strings.ToLower(s)) }
