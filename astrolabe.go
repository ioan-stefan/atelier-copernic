package main

import (
	"fmt"
	"html/template"
	"math"
	"strings"
)

// Mean obliquity of the ecliptic at epoch J2000, in degrees.
const obliquityJ2000 = 23.4392911

// Star is a named rete star at epoch J2000.
type Star struct {
	Name string
	RA   float64 // right ascension, degrees
	Dec  float64 // declination, degrees
}

// reteStars are the bright stars traditionally carried on a rete. Each plate
// keeps only those that fall inside its limb.
var reteStars = []Star{
	{"Alpheratz", 2.10, 29.09},
	{"Achernar", 24.43, -57.24},
	{"Aldebaran", 68.98, 16.51},
	{"Rigel", 78.63, -8.20},
	{"Capella", 79.17, 46.00},
	{"Betelgeuse", 88.79, 7.41},
	{"Canopus", 95.99, -52.70},
	{"Sirius", 101.29, -16.72},
	{"Procyon", 114.83, 5.22},
	{"Pollux", 116.33, 28.03},
	{"Alphard", 141.90, -8.66},
	{"Regulus", 152.09, 11.97},
	{"Dubhe", 165.93, 61.75},
	{"Acrux", 186.65, -63.10},
	{"Spica", 201.30, -11.16},
	{"Hadar", 210.96, -60.37},
	{"Arcturus", 213.92, 19.18},
	{"Rigil Kentaurus", 219.90, -60.83},
	{"Antares", 247.35, -26.43},
	{"Vega", 279.23, 38.78},
	{"Altair", 297.70, 8.87},
	{"Peacock", 306.41, -56.74},
	{"Deneb", 310.36, 45.28},
	{"Fomalhaut", 344.41, -29.62},
	{"Markab", 346.19, 15.21},
}

// PlateOptions describes one tympan and its rete.
type PlateOptions struct {
	ID             string // unique per page; prefixes clip-path ids
	Title          string
	Latitude       float64 // degrees; sign ignored, Southern selects the projection
	Southern       bool
	AlmucantarStep int // degrees between altitude circles
	AzimuthStep    int // degrees between azimuth circles
}

func rad(deg float64) float64 { return deg * math.Pi / 180 }
func deg(r float64) float64   { return r * 180 / math.Pi }

// projection is a stereographic projection onto the equatorial plane, scaled
// so the tropic of the far hemisphere sits on the limb.
type projection struct{ rCap, rEq, rCan float64 }

func newProjection(limb, obliquity float64) projection {
	rEq := limb / math.Tan(rad((90+obliquity)/2))
	return projection{rCap: limb, rEq: rEq, rCan: rEq * math.Tan(rad((90-obliquity)/2))}
}

// radius maps an angular distance from the projection's central pole to a
// distance from the plate centre.
func (p projection) radius(polarDistance float64) float64 {
	return p.rEq * math.Tan(rad(polarDistance/2))
}

// sky places a point of the celestial sphere on the plate. The south meridian
// points up the page. Southern plates are projected from the opposite pole,
// which mirrors both declination and the sense of right ascension.
func (p projection) sky(ra, dec float64, southern bool) (x, y, r float64) {
	if southern {
		ra, dec = -ra, -dec
	}
	r = p.radius(90 - dec)
	a := rad(ra)
	return r * math.Sin(a), -r * math.Cos(a), r
}

// almucantar returns the circle of equal altitude alt for latitude lat.
// It is found from the two points where it crosses the meridian.
func (p projection) almucantar(lat, alt float64) (cy, r float64, ok bool) {
	far := 180 - lat - alt
	if far >= 179.5 {
		return 0, 0, false // the circle runs to infinity near the equator
	}
	south := p.radius(far)
	north := p.radius(alt - lat)
	return -(south + north) / 2, math.Abs(south-north) / 2, true
}

// eclipticEccentricity is the offset of the ecliptic ring's centre from the
// plate centre, as a fraction of the limb radius.
func eclipticEccentricity(obliquity float64) float64 {
	p := newProjection(1, obliquity)
	return (p.rCap - p.rCan) / 2 / p.rCap
}

func eclipticToEquatorial(lon, obliquity float64) (ra, dec float64) {
	l, e := rad(lon), rad(obliquity)
	ra = deg(math.Atan2(math.Sin(l)*math.Cos(e), math.Cos(l)))
	dec = deg(math.Asin(math.Sin(e) * math.Sin(l)))
	return ra, dec
}

type svgBuilder struct{ strings.Builder }

func (b *svgBuilder) printf(format string, a ...any) { fmt.Fprintf(&b.Builder, format, a...) }

var romanHours = [12]string{"XII", "I", "II", "III", "IV", "V", "VI", "VII", "VIII", "IX", "X", "XI"}

// RenderPlate draws a tympan for the given latitude beneath its rete, as an
// accessible inline SVG. Every line is computed from the projection; nothing
// is traced from an image.
func RenderPlate(o PlateOptions) template.HTML {
	const limb = 100.0
	p := newProjection(limb, obliquityJ2000)
	lat := math.Abs(o.Latitude)
	almStep := o.AlmucantarStep
	if almStep < 2 {
		almStep = 6
	}
	azStep := o.AzimuthStep
	if azStep < 5 {
		azStep = 15
	}
	hemisphere := Northern
	if o.Southern {
		hemisphere = Southern
	}

	var b svgBuilder
	id := o.ID
	horizonCY, horizonR, _ := p.almucantar(lat, 0)

	// Count stars first so the description is accurate.
	type placed struct {
		Star
		x, y, r float64
	}
	var stars []placed
	for _, s := range reteStars {
		x, y, r := p.sky(s.RA, s.Dec, o.Southern)
		if r > limb-3 || r < 1 {
			continue
		}
		stars = append(stars, placed{s, x, y, r})
	}

	b.printf(`<svg class="plate" data-hemisphere="%s" viewBox="-124 -124 248 248" role="img" aria-labelledby="%s-title %s-desc" xmlns="http://www.w3.org/2000/svg">`,
		hemisphere, id, id)
	b.printf(`<title id="%s-title">%s</title>`, id, template.HTMLEscapeString(o.Title))
	b.printf(`<desc id="%s-desc">Stereographic projection for latitude %s: almucantars every %d degrees, azimuths every %d degrees, the horizon and astronomical twilight, beneath a rete carrying the ecliptic and %d named stars.</desc>`,
		id, template.HTMLEscapeString(formatLatitude(lat, hemisphere)), almStep, azStep, len(stars))
	b.printf(`<defs><clipPath id="%s-limb"><circle r="%.2f"/></clipPath><clipPath id="%s-sky"><circle cy="%.2f" r="%.2f"/></clipPath></defs>`,
		id, limb, id, horizonCY, horizonR)

	// Mater: the fixed limb with degree and hour scales.
	b.printf(`<g class="mater" fill="none" stroke="currentColor">`)
	for _, r := range [...]float64{limb, 103.5, 108, 118} {
		b.printf(`<circle r="%.1f"/>`, r)
	}
	for d := 0; d < 360; d += 5 {
		outer := 103.5
		if d%15 == 0 {
			outer = 108
		}
		s, c := math.Sincos(rad(float64(d)))
		b.printf(`<line x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f"/>`, limb*s, -limb*c, outer*s, -outer*c)
	}
	b.printf(`</g><g class="limb-numerals" fill="currentColor" font-size="5.2" text-anchor="middle" dominant-baseline="central">`)
	for i := 0; i < 24; i++ {
		s, c := math.Sincos(rad(float64(i) * 15))
		b.printf(`<text x="%.2f" y="%.2f">%s</text>`, 113*s, -113*c, romanHours[i%12])
	}
	b.printf(`</g>`)

	// Tympan: reference circles, almucantars, azimuths, latitude engraving.
	b.printf(`<g class="tympan" fill="none" stroke="currentColor" clip-path="url(#%s-limb)">`, id)
	b.printf(`<circle class="tropic" r="%.2f"/><circle class="tropic" r="%.2f"/>`, p.rEq, p.rCan)
	b.printf(`<line class="axis" x1="0" y1="%.1f" x2="0" y2="%.1f"/><line class="axis" x1="%.1f" y1="0" x2="%.1f" y2="0"/>`, -limb, limb, -limb, limb)
	for alt := 0; alt < 90; alt += almStep {
		cy, r, ok := p.almucantar(lat, float64(alt))
		if !ok {
			continue
		}
		class := "almucantar"
		if alt == 0 {
			class = "horizon"
		}
		b.printf(`<circle class="%s" cy="%.2f" r="%.2f"/>`, class, cy, r)
	}
	if cy, r, ok := p.almucantar(lat, -18); ok {
		b.printf(`<circle class="twilight" cy="%.2f" r="%.2f"/>`, cy, r)
	}
	zenith := p.radius(90 - lat)
	nadir := -p.radius(90 + lat)
	half := (zenith - nadir) / 2
	mid := (zenith + nadir) / 2
	b.printf(`<g class="azimuths" clip-path="url(#%s-sky)">`, id)
	for a := azStep; a <= 90; a += azStep {
		cx := half / math.Tan(rad(float64(a)))
		r := half / math.Sin(rad(float64(a)))
		b.printf(`<circle cx="%.2f" cy="%.2f" r="%.2f"/>`, cx, -mid, r)
		if a != 90 {
			b.printf(`<circle cx="%.2f" cy="%.2f" r="%.2f"/>`, -cx, -mid, r)
		}
	}
	b.printf(`</g><path class="zenith" d="M-3 %.2fH3M0 %.2fV%.2f"/>`, -zenith, -zenith-3, -zenith+3)
	b.printf(`<text class="plate-label" x="0" y="86" fill="currentColor" stroke="none" font-size="5" text-anchor="middle">LAT %s</text>`,
		template.HTMLEscapeString(formatLatitude(lat, hemisphere)))
	b.printf(`</g>`)

	// Rete: rim, struts, ecliptic ring with zodiac divisions, star pointers.
	b.printf(`<g class="rete" fill="none" stroke="currentColor">`)
	b.printf(`<circle class="rete-rim" r="%.1f"/>`, limb)
	for _, a := range [...]float64{45, 135, 225, 315} {
		s, c := math.Sincos(rad(a))
		b.printf(`<line class="strut" x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f"/>`, 6*s, -6*c, limb*s, -limb*c)
	}
	sRA, sDec := eclipticToEquatorial(90, obliquityJ2000)
	wRA, wDec := eclipticToEquatorial(270, obliquityJ2000)
	x1, y1, _ := p.sky(sRA, sDec, o.Southern)
	x2, y2, _ := p.sky(wRA, wDec, o.Southern)
	ecx, ecy := (x1+x2)/2, (y1+y2)/2
	eclR := math.Hypot(x1-x2, y1-y2) / 2
	b.printf(`<circle class="ecliptic" cx="%.2f" cy="%.2f" r="%.2f"/><circle class="ecliptic-band" cx="%.2f" cy="%.2f" r="%.2f"/>`,
		ecx, ecy, eclR, ecx, ecy, eclR-5)
	for lon := 0; lon < 360; lon += 10 {
		ra, dec := eclipticToEquatorial(float64(lon), obliquityJ2000)
		x, y, _ := p.sky(ra, dec, o.Southern)
		ux, uy := (ecx-x)/eclR, (ecy-y)/eclR
		length := 2.2
		if lon%30 == 0 {
			length = 5
		}
		b.printf(`<line class="zodiac" x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f"/>`, x, y, x+ux*length, y+uy*length)
	}
	b.printf(`<circle class="boss" r="6"/><circle class="boss" r="2"/>`)
	for _, s := range stars {
		ux, uy := s.x/s.r, s.y/s.r
		px, py := -uy, ux
		base := math.Min(s.r+6, limb)
		bx, by := ux*base, uy*base
		if base-s.r > 1.5 {
			b.printf(`<path class="pointer" fill="currentColor" d="M%.2f %.2fL%.2f %.2fL%.2f %.2fZ"/>`,
				s.x, s.y, bx+px*1.6, by+py*1.6, bx-px*1.6, by-py*1.6)
			b.printf(`<line class="strut" x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f"/>`, bx, by, ux*limb, uy*limb)
		}
		b.printf(`<circle class="star" cx="%.2f" cy="%.2f" r="0.9" fill="currentColor"/>`, s.x, s.y)
		b.printf(`<text class="star-label" x="%.2f" y="%.2f" fill="currentColor" stroke="none" font-size="3.6">%s</text>`,
			s.x+px*2.6, s.y+py*2.6+1.2, template.HTMLEscapeString(s.Name))
	}
	b.printf(`</g>`)

	// Registration marks, as on the workshop drawing.
	b.printf(`<g class="registration" fill="none" stroke="currentColor">`)
	for _, c := range [...][2]float64{{-116, -116}, {116, -116}, {-116, 116}, {116, 116}} {
		b.printf(`<path d="M%.0f %.0fH%.0fM%.0f %.0fV%.0f"/><circle cx="%.0f" cy="%.0f" r="1.6"/>`,
			c[0]-5, c[1], c[0]+5, c[0], c[1]-5, c[1]+5, c[0], c[1])
	}
	b.printf(`</g></svg>`)

	return template.HTML(b.String()) //nolint:gosec // built only from numbers and escaped strings above
}
