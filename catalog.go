package main

import (
	"fmt"
	"html/template"
	"slices"
	"strings"
)

// All catalogue content below is mock data for a fictional atelier. Orbital
// mechanics (projection geometry, gear ratios, drift) are computed, not typed.

// Hemisphere names the celestial hemisphere a plate is projected for.
type Hemisphere string

const (
	Northern Hemisphere = "northern"
	Southern Hemisphere = "southern"
)

func (h Hemisphere) Label() string {
	if h == Southern {
		return "Southern"
	}
	return "Northern"
}

// Tier is a complication level offered for private commissions.
type Tier struct {
	ID         string
	Name       string
	Summary    string
	FromCHF    int
	LeadMonths int
}

var tiers = []Tier{
	{"planisphaerium", "Planisphaerium", "Rete, tympan and alidade, set by hand. No movement.", 48_500, 14},
	{"horologium", "Horologium", "Adds a weekly-wound drive that turns the rete with the stars.", 186_000, 26},
	{"uranographia", "Uranographia", "Adds lunar phase and a draconic train for eclipse seasons.", 412_000, 38},
}

func tierByID(id string) (Tier, bool) {
	i := slices.IndexFunc(tiers, func(t Tier) bool { return t.ID == id })
	if i < 0 {
		return Tier{}, false
	}
	return tiers[i], true
}

func tierRank(id string) int {
	return slices.IndexFunc(tiers, func(t Tier) bool { return t.ID == id })
}

// Alloy is a plate alloy identified by its EN material number.
type Alloy struct {
	Code         string
	Formula      string
	Name         string
	Note         string
	SurchargeCHF int
}

var alloys = []Alloy{
	{"CW508L", "CuZn37", "Cartridge brass", "Warm and ductile. Takes the deepest hand-chasing.", 0},
	{"CW612N", "CuZn39Pb2", "Clock brass", "Free-cutting, for the densest scales and numerals.", 0},
	{"CW453K", "CuSn8", "Phosphor bronze", "Darker, slower patina. Hard-wearing at the rete pivot.", 3_900},
	{"CW409J", "CuNi18Zn20", "Nickel silver", "Pale and cool. Resists sea air and handling.", 4_200},
}

func alloyByCode(code string) (Alloy, bool) {
	i := slices.IndexFunc(alloys, func(a Alloy) bool { return a.Code == code })
	if i < 0 {
		return Alloy{}, false
	}
	return alloys[i], true
}

// Script is an engraving hand for the dedication on the back of the mater.
type Script struct {
	ID   string
	Name string
	Note string
}

var scripts = []Script{
	{"roman", "Roman capitals", "Incised, after the inscriptional letter."},
	{"italic", "Humanist italic", "A chancery hand, cut with a fine graver."},
	{"kufic", "Square Kufic", "Geometric, laid out on the plate's own grid."},
}

func scriptByID(id string) (Script, bool) {
	i := slices.IndexFunc(scripts, func(s Script) bool { return s.ID == id })
	if i < 0 {
		return Script{}, false
	}
	return scripts[i], true
}

// ContactMethod is how a patron prefers the atelier to reply.
type ContactMethod struct {
	ID   string
	Name string
}

var contactMethods = []ContactMethod{
	{"email", "Email"},
	{"telephone", "Telephone"},
	{"viewing", "Private viewing in Geneva"},
}

func contactByID(id string) (ContactMethod, bool) {
	i := slices.IndexFunc(contactMethods, func(c ContactMethod) bool { return c.ID == id })
	if i < 0 {
		return ContactMethod{}, false
	}
	return contactMethods[i], true
}

// Spec is one line of a specification sheet.
type Spec struct{ Label, Value string }

// SpecGroup clusters related specifications under one heading.
type SpecGroup struct {
	Title string
	Specs []Spec
}

// Instrument holds the technical definition shared by current editions and
// delivered pieces.
type Instrument struct {
	Code           string // caliber
	Slug           string
	Name           string
	TierID         string
	Hemisphere     Hemisphere
	Latitude       float64 // absolute degrees
	Longitude      float64 // signed degrees east
	Site           string
	DiameterMM     int
	Components     int
	Jewels         int
	ReserveHours   int
	Escapement     string
	MaxTeeth       int // largest wheel the train may use; 0 for no movement
	AlloyCode      string
	Rete           string
	Finish         string
	ToleranceUM    int
	AlmucantarStep int
	AzimuthStep    int

	Trains []Train
	Plate  template.HTML
}

func (in Instrument) Tier() Tier {
	t, _ := tierByID(in.TierID)
	return t
}

func (in Instrument) Alloy() Alloy {
	a, _ := alloyByCode(in.AlloyCode)
	return a
}

func (in Instrument) LatitudeLabel() string  { return formatLatitude(in.Latitude, in.Hemisphere) }
func (in Instrument) LongitudeLabel() string { return formatLongitude(in.Longitude) }

// SignedLatitude is negative south of the equator, for sorting.
func (in Instrument) SignedLatitude() float64 {
	if in.Hemisphere == Southern {
		return -in.Latitude
	}
	return in.Latitude
}

func (in Instrument) Eccentricity() float64 { return eclipticEccentricity(obliquityJ2000) }

func (in Instrument) train(name string) *Train {
	for i := range in.Trains {
		if in.Trains[i].Name == name {
			return &in.Trains[i]
		}
	}
	return nil
}

func (in Instrument) Sidereal() *Train { return in.train("Sidereal") }
func (in Instrument) Synodic() *Train  { return in.train("Synodic") }

func (in Instrument) SpecGroups() []SpecGroup {
	pole := "south"
	if in.Hemisphere == Southern {
		pole = "north"
	}
	geometry := SpecGroup{"Geometry", []Spec{
		{"Projection", "Stereographic, from the " + pole + " celestial pole"},
		{"Latitude", in.LatitudeLabel()},
		{"Plate diameter", fmt.Sprintf("%d mm", in.DiameterMM)},
		{"Ecliptic eccentricity", fmt.Sprintf("%.4f of limb radius", in.Eccentricity())},
		{"Obliquity", formatArc(obliquityJ2000) + " (J2000)"},
		{"Almucantars", fmt.Sprintf("Every %d°", in.AlmucantarStep)},
		{"Azimuths", fmt.Sprintf("Every %d°", in.AzimuthStep)},
	}}

	movement := SpecGroup{Title: "Movement"}
	movement.Specs = append(movement.Specs, Spec{"Caliber", in.Code})
	if in.MaxTeeth == 0 {
		movement.Specs = append(movement.Specs, Spec{"Drive", "None. Rete set by hand"})
	} else {
		movement.Specs = append(movement.Specs,
			Spec{"Escapement", in.Escapement},
			Spec{"Power reserve", fmt.Sprintf("%d hours", in.ReserveHours)},
			Spec{"Jewels", fmt.Sprintf("%d", in.Jewels)},
			Spec{"Largest wheel", fmt.Sprintf("%d teeth", in.MaxTeeth)},
		)
	}
	movement.Specs = append(movement.Specs, Spec{"Components", groupThousands(int64(in.Components))})

	a := in.Alloy()
	materials := SpecGroup{"Materials", []Spec{
		{"Plate and mater", fmt.Sprintf("%s %s (%s)", a.Name, a.Formula, a.Code)},
		{"Rete", in.Rete},
		{"Finish", in.Finish},
		{"Flatness tolerance", fmt.Sprintf("±%d μm", in.ToleranceUM)},
	}}
	return []SpecGroup{geometry, movement, materials}
}

// Edition is an instrument in current limited production.
type Edition struct {
	Instrument
	Epithet     string
	Story       string
	EditionSize int
}

// ProvenanceEvent is one dated line in a piece's history.
type ProvenanceEvent struct {
	Year  int
	Event string
}

// Certificate is the atelier's certificate of authenticity for a piece.
type Certificate struct {
	Number   string
	Issued   string
	Assessor string
}

// LedgerEntry is a delivered piece recorded in the Ephemeris Ledger.
type LedgerEntry struct {
	Instrument
	Ref         string
	Year        int
	Status      string
	Summary     string
	Provenance  []ProvenanceEvent
	Certificate Certificate

	search string // folded haystack for free-text search
}

// Catalog is the read-only, in-memory catalogue built once at start-up.
type Catalog struct {
	Editions []Edition
	Ledger   []LedgerEntry // ordered by year
	bySlug   map[string]int
}

func NewCatalog() *Catalog {
	solver := NewTrainSolver()
	c := &Catalog{Editions: seedEditions(), Ledger: seedLedger(), bySlug: make(map[string]int)}

	for i := range c.Editions {
		e := &c.Editions[i]
		e.Slug = strings.ToLower(e.Code)
		prepareInstrument(&e.Instrument, solver, "plate-"+e.Slug)
	}

	slices.SortStableFunc(c.Ledger, func(a, b LedgerEntry) int { return a.Year - b.Year })
	for i := range c.Ledger {
		e := &c.Ledger[i]
		e.Slug = strings.ToLower(e.Ref)
		prepareInstrument(&e.Instrument, solver, "plate-"+e.Slug)
		e.search = fold(strings.Join([]string{
			e.Ref, e.Name, e.Code, e.Site, e.AlloyCode, e.Alloy().Name, e.Tier().Name,
			e.Status, e.Certificate.Number, e.Certificate.Assessor, e.Hemisphere.Label(),
			e.LatitudeLabel(), fmt.Sprint(e.Year),
		}, " "))
		c.bySlug[e.Slug] = i
	}
	return c
}

func prepareInstrument(in *Instrument, solver *TrainSolver, plateID string) {
	if in.AlmucantarStep == 0 {
		in.AlmucantarStep = 6
	}
	if in.AzimuthStep == 0 {
		in.AzimuthStep = 15
	}
	in.Trains = trainsForTier(in.TierID, in.MaxTeeth, solver)
	in.Plate = RenderPlate(PlateOptions{
		ID:             plateID,
		Title:          fmt.Sprintf("Tympan and rete of %s, drawn for %s", in.Name, in.LatitudeLabel()),
		Latitude:       in.Latitude,
		Southern:       in.Hemisphere == Southern,
		AlmucantarStep: in.AlmucantarStep,
		AzimuthStep:    in.AzimuthStep,
	})
}

// Entry returns a ledger entry with its neighbours in ledger order.
func (c *Catalog) Entry(slug string) (entry LedgerEntry, prev, next *LedgerEntry, ok bool) {
	i, ok := c.bySlug[slug]
	if !ok {
		return LedgerEntry{}, nil, nil, false
	}
	if i > 0 {
		prev = &c.Ledger[i-1]
	}
	if i < len(c.Ledger)-1 {
		next = &c.Ledger[i+1]
	}
	return c.Ledger[i], prev, next, true
}

// Recent returns the n most recently delivered pieces, newest first.
func (c *Catalog) Recent(n int) []LedgerEntry {
	n = min(n, len(c.Ledger))
	out := make([]LedgerEntry, 0, n)
	for i := len(c.Ledger) - 1; i >= len(c.Ledger)-n; i-- {
		out = append(out, c.Ledger[i])
	}
	return out
}

func seedEditions() []Edition {
	return []Edition{
		{
			Instrument: Instrument{
				Code: "AC-09", Name: "Sidereus", TierID: "horologium",
				Hemisphere: Northern, Latitude: 46.2017, Longitude: 6.1469, Site: "Geneva",
				DiameterMM: 212, Components: 238, Jewels: 21, ReserveHours: 192,
				Escapement: "Swiss lever, 18'000 vph", MaxTeeth: 120,
				AlloyCode: "CW508L", Rete: "Sterling silver, pierced by hand", Finish: "Hand-chased, frosted ground",
				ToleranceUM: 2, AlmucantarStep: 6, AzimuthStep: 15,
			},
			Epithet:     "A planispheric astrolabe that keeps sidereal time.",
			Story:       "Sidereus pairs a hand-pierced silver rete with a drive hidden in the mater. Wound once a week, it turns the sky over Geneva at the rate the stars themselves keep.",
			EditionSize: 9,
		},
		{
			Instrument: Instrument{
				Code: "AC-11", Name: "Meridiana", TierID: "uranographia",
				Hemisphere: Northern, Latitude: 41.9028, Longitude: 12.4964, Site: "Rome",
				DiameterMM: 236, Components: 431, Jewels: 33, ReserveHours: 192,
				Escapement: "Swiss lever, 18'000 vph", MaxTeeth: 144,
				AlloyCode: "CW612N", Rete: "Sterling silver, pierced by hand", Finish: "Fire-blued screws, grained mater",
				ToleranceUM: 2, AlmucantarStep: 5, AzimuthStep: 10,
			},
			Epithet:     "An astrolabe with lunar phase and eclipse seasons.",
			Story:       "Drawn for Rome, Meridiana carries two further trains behind the plate: one for the synodic month, one for the draconic month that decides when eclipses can fall.",
			EditionSize: 7,
		},
		{
			Instrument: Instrument{
				Code: "AC-14", Name: "Austral", TierID: "horologium",
				Hemisphere: Southern, Latitude: 33.8688, Longitude: 151.2093, Site: "Sydney",
				DiameterMM: 205, Components: 244, Jewels: 21, ReserveHours: 120,
				Escapement: "Swiss lever, 21'600 vph", MaxTeeth: 96,
				AlloyCode: "CW409J", Rete: "Nickel silver, pierced by hand", Finish: "Satin-brushed, polished bevels",
				ToleranceUM: 3, AlmucantarStep: 6, AzimuthStep: 15,
			},
			Epithet:     "A southern-sky astrolabe for Pacific latitudes.",
			Story:       "Austral is projected from the north celestial pole, so its rete carries Crux, Canopus and Achernar. The plates are nickel silver, which stays pale in sea air.",
			EditionSize: 5,
		},
	}
}

func seedLedger() []LedgerEntry {
	const (
		vauthier = "Odile Vauthier"
		lachat   = "Henri Lachat"
		mottier  = "Aurèle Mottier"
	)
	lever := "Swiss lever, 18'000 vph"
	return []LedgerEntry{
		{
			Instrument: Instrument{Code: "AC-01", Name: "Prima Tabula", TierID: "planisphaerium", Hemisphere: Northern,
				Latitude: 46.2017, Longitude: 6.1469, Site: "Geneva", DiameterMM: 180, Components: 64,
				AlloyCode: "CW612N", Rete: "Clock brass, pierced by hand", Finish: "Hand-chased, oiled", ToleranceUM: 5},
			Ref: "AC-01-01", Year: 1987, Status: "Atelier archive",
			Summary: "The first plate cut in the atelier, drawn for the workshop's own latitude.",
			Provenance: []ProvenanceEvent{
				{1987, "Completed in the workshop on the Rue de la Corraterie."},
				{1988, "Retained by the founder as the atelier's reference plate."},
			},
			Certificate: Certificate{"CA-1987-001", "14 March 1987", lachat},
		},
		{
			Instrument: Instrument{Code: "AC-01", Name: "Lemanus", TierID: "planisphaerium", Hemisphere: Northern,
				Latitude: 46.5197, Longitude: 6.6323, Site: "Lausanne", DiameterMM: 190, Components: 71,
				AlloyCode: "CW508L", Rete: "Cartridge brass, pierced by hand", Finish: "Hand-chased, oiled", ToleranceUM: 5},
			Ref: "AC-01-04", Year: 1989, Status: "Private collection",
			Summary: "A lakeside commission with a second tympan for the patron's family house in the Valais.",
			Provenance: []ProvenanceEvent{
				{1989, "Delivered to the commissioning patron in Lausanne."},
				{2011, "Passed by inheritance to the patron's daughter."},
			},
			Certificate: Certificate{"CA-1989-004", "2 October 1989", lachat},
		},
		{
			Instrument: Instrument{Code: "AC-02", Name: "Hora Sidera", TierID: "horologium", Hemisphere: Northern,
				Latitude: 47.3769, Longitude: 8.5417, Site: "Zürich", DiameterMM: 210, Components: 212, Jewels: 17,
				ReserveHours: 72, Escapement: lever, MaxTeeth: 96,
				AlloyCode: "CW508L", Rete: "Sterling silver, pierced by hand", Finish: "Frosted ground, polished bevels", ToleranceUM: 4},
			Ref: "AC-02-02", Year: 1993, Status: "Private collection",
			Summary: "The atelier's first driven rete, wound every three days.",
			Provenance: []ProvenanceEvent{
				{1993, "Delivered to a private collector in Zürich."},
				{2006, "Returned for a full service and a new mainspring."},
				{2019, "Sold privately to a collection in Basel, with the atelier's knowledge."},
			},
			Certificate: Certificate{"CA-1993-002", "21 June 1993", lachat},
		},
		{
			Instrument: Instrument{Code: "AC-02", Name: "Cancri", TierID: "horologium", Hemisphere: Northern,
				Latitude: 31.6295, Longitude: -7.9811, Site: "Marrakesh", DiameterMM: 200, Components: 224, Jewels: 19,
				ReserveHours: 72, Escapement: lever, MaxTeeth: 96, AlmucantarStep: 3, AzimuthStep: 10,
				AlloyCode: "CW453K", Rete: "Phosphor bronze, pierced by hand", Finish: "Chemically darkened, waxed", ToleranceUM: 4},
			Ref: "AC-02-05", Year: 1995, Status: "Museum loan",
			Summary: "Drawn with almucantars every three degrees, after the Maghrebi instruments that inspired it.",
			Provenance: []ProvenanceEvent{
				{1995, "Delivered to a patron in Marrakesh."},
				{2014, "Placed on long-term loan to a European museum of the history of science."},
			},
			Certificate: Certificate{"CA-1995-005", "9 May 1995", lachat},
		},
		{
			Instrument: Instrument{Code: "AC-03", Name: "Capricornus", TierID: "planisphaerium", Hemisphere: Southern,
				Latitude: 34.6037, Longitude: -58.3816, Site: "Buenos Aires", DiameterMM: 185, Components: 68,
				AlloyCode: "CW508L", Rete: "Cartridge brass, pierced by hand", Finish: "Hand-chased, oiled", ToleranceUM: 4},
			Ref: "AC-03-01", Year: 1998, Status: "Private collection",
			Summary: "The atelier's first southern plate, projected from the north celestial pole.",
			Provenance: []ProvenanceEvent{
				{1998, "Delivered to a patron in Buenos Aires."},
				{2022, "Inspected in Geneva and the rete re-flattened."},
			},
			Certificate: Certificate{"CA-1998-001", "30 November 1998", lachat},
		},
		{
			Instrument: Instrument{Code: "AC-04", Name: "Selene", TierID: "uranographia", Hemisphere: Northern,
				Latitude: 48.2082, Longitude: 16.3738, Site: "Vienna", DiameterMM: 240, Components: 418, Jewels: 31,
				ReserveHours: 96, Escapement: lever, MaxTeeth: 120,
				AlloyCode: "CW612N", Rete: "Sterling silver, pierced by hand", Finish: "Grained mater, blued screws", ToleranceUM: 3},
			Ref: "AC-04-03", Year: 2001, Status: "Private collection",
			Summary: "The first Uranographia: lunar phase and eclipse seasons behind a Viennese plate.",
			Provenance: []ProvenanceEvent{
				{2001, "Delivered to a collector in Vienna."},
				{2015, "Exhibited privately during a horological society meeting."},
			},
			Certificate: Certificate{"CA-2001-003", "17 September 2001", mottier},
		},
		{
			Instrument: Instrument{Code: "AC-05", Name: "Toledana", TierID: "horologium", Hemisphere: Northern,
				Latitude: 39.8628, Longitude: -4.0273, Site: "Toledo", DiameterMM: 215, Components: 229, Jewels: 21,
				ReserveHours: 96, Escapement: lever, MaxTeeth: 120,
				AlloyCode: "CW453K", Rete: "Phosphor bronze, pierced by hand", Finish: "Hand-chased, waxed", ToleranceUM: 3},
			Ref: "AC-05-02", Year: 2004, Status: "Private collection",
			Summary: "A rete laid out in the manner of the Toledan tables, with a modern drive.",
			Provenance: []ProvenanceEvent{
				{2004, "Delivered to a patron in Madrid."},
				{2018, "Serviced in Geneva; original mainspring retained."},
			},
			Certificate: Certificate{"CA-2004-002", "4 April 2004", mottier},
		},
		{
			Instrument: Instrument{Code: "AC-06", Name: "Crux", TierID: "horologium", Hemisphere: Southern,
				Latitude: 37.8136, Longitude: 144.9631, Site: "Melbourne", DiameterMM: 205, Components: 236, Jewels: 21,
				ReserveHours: 80, Escapement: lever, MaxTeeth: 96,
				AlloyCode: "CW409J", Rete: "Nickel silver, pierced by hand", Finish: "Satin-brushed", ToleranceUM: 3},
			Ref: "AC-06-01", Year: 2007, Status: "Private collection",
			Summary: "A driven southern rete for Melbourne, the first in nickel silver.",
			Provenance: []ProvenanceEvent{
				{2007, "Delivered to a patron in Melbourne."},
				{2020, "Returned for service; dial train re-pivoted."},
			},
			Certificate: Certificate{"CA-2007-001", "12 February 2007", vauthier},
		},
		{
			Instrument: Instrument{Code: "AC-07", Name: "Uraniborg", TierID: "uranographia", Hemisphere: Northern,
				Latitude: 55.9078, Longitude: 12.6967, Site: "Ven", DiameterMM: 250, Components: 446, Jewels: 33,
				ReserveHours: 100, Escapement: lever, MaxTeeth: 144, AlmucantarStep: 5, AzimuthStep: 10,
				AlloyCode: "CW508L", Rete: "Sterling silver, pierced by hand", Finish: "Frosted ground, polished bevels", ToleranceUM: 2},
			Ref: "AC-07-04", Year: 2010, Status: "Museum loan",
			Summary: "Drawn for the island where Tycho Brahe built his observatory.",
			Provenance: []ProvenanceEvent{
				{2010, "Delivered to a Scandinavian private foundation."},
				{2016, "Lent for a travelling exhibition on Renaissance astronomy."},
				{2023, "Returned to the foundation's study collection."},
			},
			Certificate: Certificate{"CA-2010-004", "1 December 2010", vauthier},
		},
		{
			Instrument: Instrument{Code: "AC-08", Name: "Kepleriana", TierID: "uranographia", Hemisphere: Northern,
				Latitude: 50.0755, Longitude: 14.4378, Site: "Prague", DiameterMM: 245, Components: 439, Jewels: 33,
				ReserveHours: 100, Escapement: lever, MaxTeeth: 144,
				AlloyCode: "CW612N", Rete: "Sterling silver, pierced by hand", Finish: "Grained mater, blued screws", ToleranceUM: 2},
			Ref: "AC-08-02", Year: 2013, Status: "Private collection",
			Summary: "A Prague plate with a moon disc engraved after early telescopic drawings.",
			Provenance: []ProvenanceEvent{
				{2013, "Delivered to a collector in Prague."},
				{2021, "Moved with its owner to Munich."},
			},
			Certificate: Certificate{"CA-2013-002", "23 August 2013", vauthier},
		},
		{
			Instrument: Instrument{Code: "AC-09", Name: "Sidereus Prima", TierID: "horologium", Hemisphere: Northern,
				Latitude: 46.2017, Longitude: 6.1469, Site: "Geneva", DiameterMM: 212, Components: 238, Jewels: 21,
				ReserveHours: 192, Escapement: lever, MaxTeeth: 120,
				AlloyCode: "CW508L", Rete: "Sterling silver, pierced by hand", Finish: "Hand-chased, frosted ground", ToleranceUM: 2},
			Ref: "AC-09-01", Year: 2016, Status: "Atelier archive",
			Summary: "The prototype of the current Sidereus edition, kept as the caliber's reference.",
			Provenance: []ProvenanceEvent{
				{2016, "Completed as the AC-09 prototype."},
				{2017, "Eight-day reserve verified over a year of continuous running."},
			},
			Certificate: Certificate{"CA-2016-001", "6 January 2016", vauthier},
		},
		{
			Instrument: Instrument{Code: "AC-10", Name: "Magellanica", TierID: "uranographia", Hemisphere: Southern,
				Latitude: 53.1638, Longitude: -70.9171, Site: "Punta Arenas", DiameterMM: 245, Components: 452, Jewels: 33,
				ReserveHours: 100, Escapement: lever, MaxTeeth: 144,
				AlloyCode: "CW409J", Rete: "Nickel silver, pierced by hand", Finish: "Satin-brushed, polished bevels", ToleranceUM: 2},
			Ref: "AC-10-03", Year: 2018, Status: "Private collection",
			Summary: "The atelier's most southerly plate, where Canopus never sets.",
			Provenance: []ProvenanceEvent{
				{2018, "Delivered to a patron in Santiago for a house on the Strait."},
				{2024, "Serviced in Geneva after a sea passage."},
			},
			Certificate: Certificate{"CA-2018-003", "19 July 2018", vauthier},
		},
		{
			Instrument: Instrument{Code: "AC-11", Name: "Meridiana Prima", TierID: "uranographia", Hemisphere: Northern,
				Latitude: 41.9028, Longitude: 12.4964, Site: "Rome", DiameterMM: 236, Components: 431, Jewels: 33,
				ReserveHours: 192, Escapement: lever, MaxTeeth: 144, AlmucantarStep: 5, AzimuthStep: 10,
				AlloyCode: "CW612N", Rete: "Sterling silver, pierced by hand", Finish: "Fire-blued screws, grained mater", ToleranceUM: 2},
			Ref: "AC-11-01", Year: 2021, Status: "Atelier archive",
			Summary: "The prototype of the current Meridiana edition.",
			Provenance: []ProvenanceEvent{
				{2021, "Completed as the AC-11 prototype."},
				{2022, "Lunar train checked against ephemerides for 400 lunations."},
			},
			Certificate: Certificate{"CA-2021-001", "11 March 2021", vauthier},
		},
		{
			Instrument: Instrument{Code: "AC-12", Name: "Tropicus", TierID: "planisphaerium", Hemisphere: Northern,
				Latitude: 23.1136, Longitude: -82.3666, Site: "Havana", DiameterMM: 170, Components: 66,
				AlloyCode: "CW453K", Rete: "Phosphor bronze, pierced by hand", Finish: "Hand-chased, waxed", ToleranceUM: 3},
			Ref: "AC-12-02", Year: 2023, Status: "Private collection",
			Summary: "Drawn almost on the Tropic of Cancer, where the Sun reaches the zenith once a year.",
			Provenance: []ProvenanceEvent{
				{2023, "Delivered to a patron in Havana."},
			},
			Certificate: Certificate{"CA-2023-002", "28 June 2023", vauthier},
		},
	}
}
