package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	minLatitude       = 5.0
	maxLatitude       = 70.0
	maxTympans        = 6
	tympanCHF         = 6_800
	maxDedication     = 120
	maxNotes          = 2000
	maxDossiers       = 10_000
	maxCommissionBody = 32 << 10
)

// Dossier sections, in order. Field errors map back to the section that holds
// the field, so the form reopens where the patron needs to look.
var stageNames = [...]string{1: "Sky", 2: "Complication", 3: "Engraving", 4: "Patron"}

const stageCount = len(stageNames) - 1

var fieldOrder = []struct {
	name  string
	stage int
}{
	{"hemisphere", 1}, {"latDeg", 1}, {"latMin", 1}, {"site", 1},
	{"tier", 2}, {"tympans", 2},
	{"alloy", 3}, {"script", 3}, {"dedication", 3},
	{"name", 4}, {"email", 4}, {"country", 4}, {"contact", 4}, {"phone", 4}, {"notes", 4}, {"discretion", 4},
}

// CommissionInput is the raw form exactly as submitted, kept for re-display.
type CommissionInput struct {
	Hemisphere string
	LatDeg     string
	LatMin     string
	Site       string
	Tier       string
	Tympans    string
	Alloy      string
	Script     string
	Dedication string
	Name       string
	Email      string
	Country    string
	Contact    string
	Phone      string
	Notes      string
	Discretion bool
}

func readCommissionInput(r *http.Request) CommissionInput {
	v := func(k string) string { return strings.TrimSpace(r.PostFormValue(k)) }
	return CommissionInput{
		Hemisphere: v("hemisphere"),
		LatDeg:     v("lat_deg"),
		LatMin:     v("lat_min"),
		Site:       v("site"),
		Tier:       v("tier"),
		Tympans:    v("tympans"),
		Alloy:      v("alloy"),
		Script:     v("script"),
		Dedication: v("dedication"),
		Name:       strings.Join(strings.Fields(v("name")), " "),
		Email:      v("email"),
		Country:    v("country"),
		Contact:    v("contact"),
		Phone:      v("phone"),
		Notes:      v("notes"),
		Discretion: r.PostFormValue("discretion") == "yes",
	}
}

// FieldErrors maps a field name to a message written for the patron.
type FieldErrors map[string]string

func (e FieldErrors) Get(field string) string { return e[field] }

// FieldError is one entry of the error summary.
type FieldError struct {
	Field   string
	Stage   string
	Message string
}

// List returns errors in form order for the summary at the top of the page.
func (e FieldErrors) List() []FieldError {
	var out []FieldError
	for _, f := range fieldOrder {
		if msg, ok := e[f.name]; ok {
			out = append(out, FieldError{f.name, stageNames[f.stage], msg})
		}
	}
	return out
}

// FirstStage is the earliest section holding an error, or 1.
func (e FieldErrors) FirstStage() int {
	for _, f := range fieldOrder {
		if _, ok := e[f.name]; ok {
			return f.stage
		}
	}
	return 1
}

func (e FieldErrors) StageHasError(stage int) bool {
	for _, f := range fieldOrder {
		if _, ok := e[f.name]; ok && f.stage == stage {
			return true
		}
	}
	return false
}

// Commission is a validated dossier.
type Commission struct {
	Ref        string
	Hemisphere Hemisphere
	LatDeg     int
	LatMin     int
	Site       string
	Tier       Tier
	Tympans    int
	Alloy      Alloy
	Script     Script
	Dedication string
	Name       string
	Email      string
	Country    string
	Contact    ContactMethod
	Phone      string
	Notes      string
	Received   time.Time
}

func (c Commission) Latitude() float64 { return float64(c.LatDeg) + float64(c.LatMin)/60 }

func (c Commission) LatitudeLabel() string { return formatLatitude(c.Latitude(), c.Hemisphere) }

// EstimateCHF is the indicative price before the atelier's written quotation.
func (c Commission) EstimateCHF() int {
	return c.Tier.FromCHF + c.Tympans*tympanCHF + c.Alloy.SurchargeCHF
}

var phonePattern = regexp.MustCompile(`^\+?[0-9 ().-]{7,24}$`)

// Validate checks every field and returns either a Commission or the errors.
// The browser's own validation is disabled (novalidate) because hidden
// sections cannot show native bubbles; this is the single source of truth.
func (in CommissionInput) Validate() (Commission, FieldErrors) {
	errs := FieldErrors{}
	var c Commission

	switch Hemisphere(in.Hemisphere) {
	case Northern, Southern:
		c.Hemisphere = Hemisphere(in.Hemisphere)
	default:
		errs["hemisphere"] = "Choose the hemisphere whose sky the plate should show."
	}

	latOK := true
	if in.LatDeg == "" {
		errs["latDeg"] = "Enter the latitude in whole degrees."
		latOK = false
	} else if d, err := strconv.Atoi(in.LatDeg); err != nil || d < 0 || d > 89 {
		errs["latDeg"] = "Degrees must be a whole number from 0 to 89."
		latOK = false
	} else {
		c.LatDeg = d
	}
	if in.LatMin != "" {
		if m, err := strconv.Atoi(in.LatMin); err != nil || m < 0 || m > 59 {
			errs["latMin"] = "Minutes must be a whole number from 0 to 59."
			latOK = false
		} else {
			c.LatMin = m
		}
	}
	if latOK && (c.Latitude() < minLatitude || c.Latitude() > maxLatitude) {
		errs["latDeg"] = fmt.Sprintf("The atelier draws tympans between %.0f° and %.0f°. Nearer the equator the horizon runs off the plate.", minLatitude, maxLatitude)
	}

	if msg := checkText(in.Site, 0, 60, false); msg != "" {
		errs["site"] = "Place name: " + msg
	}
	c.Site = in.Site

	if t, ok := tierByID(in.Tier); ok {
		c.Tier = t
	} else {
		errs["tier"] = "Choose a complication tier."
	}

	if in.Tympans != "" {
		if n, err := strconv.Atoi(in.Tympans); err != nil || n < 0 || n > maxTympans {
			errs["tympans"] = fmt.Sprintf("Additional plates must be a whole number from 0 to %d.", maxTympans)
		} else {
			c.Tympans = n
		}
	}

	if a, ok := alloyByCode(in.Alloy); ok {
		c.Alloy = a
	} else {
		errs["alloy"] = "Choose a plate alloy."
	}
	if s, ok := scriptByID(in.Script); ok {
		c.Script = s
	} else {
		errs["script"] = "Choose an engraving hand."
	}
	if msg := checkText(in.Dedication, 0, maxDedication, false); msg != "" {
		errs["dedication"] = "Dedication: " + msg
	}
	c.Dedication = in.Dedication

	if msg := checkText(in.Name, 2, 100, false); msg != "" {
		errs["name"] = "Name: " + msg
	}
	c.Name = in.Name

	if in.Email == "" {
		errs["email"] = "Enter an email address so the atelier can reply."
	} else if !validEmail(in.Email) {
		errs["email"] = "Enter an email address in the form name@example.com."
	}
	c.Email = in.Email

	if msg := checkText(in.Country, 2, 80, false); msg != "" {
		errs["country"] = "Country of residence: " + msg
	}
	c.Country = in.Country

	if m, ok := contactByID(in.Contact); ok {
		c.Contact = m
	} else {
		errs["contact"] = "Choose how the atelier should reply."
	}

	switch {
	case in.Phone == "" && in.Contact == "telephone":
		errs["phone"] = "Enter a telephone number, or choose another way to reply."
	case in.Phone != "" && !validPhone(in.Phone):
		errs["phone"] = "Enter a telephone number using digits, spaces and an optional leading +."
	}
	c.Phone = in.Phone

	if msg := checkText(in.Notes, 0, maxNotes, true); msg != "" {
		errs["notes"] = "Notes: " + msg
	}
	c.Notes = in.Notes

	if !in.Discretion {
		errs["discretion"] = "Confirm that you have read the discretion policy."
	}

	if len(errs) > 0 {
		return Commission{}, errs
	}
	return c, nil
}

// checkText enforces length in characters and rejects control characters.
// It returns an empty string when the value is acceptable.
func checkText(s string, minLen, maxLen int, multiline bool) string {
	n := utf8.RuneCountInString(s)
	switch {
	case n == 0 && minLen > 0:
		return "this is required."
	case n < minLen:
		return fmt.Sprintf("use at least %d characters.", minLen)
	case n > maxLen:
		return fmt.Sprintf("use %d characters or fewer (currently %d).", maxLen, n)
	}
	for _, r := range s {
		if multiline && (r == '\n' || r == '\r' || r == '\t') {
			continue
		}
		if !unicode.IsPrint(r) {
			return "remove unsupported characters."
		}
	}
	return ""
}

func validEmail(s string) bool {
	if len(s) > 254 {
		return false
	}
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s {
		return false
	}
	at := strings.LastIndexByte(s, '@')
	return at > 0 && strings.Contains(s[at+1:], ".")
}

func validPhone(s string) bool {
	if !phonePattern.MatchString(s) {
		return false
	}
	digits := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			digits++
		}
	}
	return digits >= 7 && digits <= 15
}

// DossierStore keeps lodged dossiers in memory. A production deployment would
// replace it with durable storage behind the same two methods.
type DossierStore struct {
	mu    sync.RWMutex
	items map[string]Commission
}

func NewDossierStore() *DossierStore {
	return &DossierStore{items: make(map[string]Commission)}
}

var errStoreFull = errors.New("dossier store is full")

func (s *DossierStore) Save(c Commission, now time.Time) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.items) >= maxDossiers {
		return "", errStoreFull
	}
	for {
		buf := make([]byte, 4)
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		ref := fmt.Sprintf("DC-%d-%s", now.Year(), strings.ToUpper(hex.EncodeToString(buf)))
		if _, taken := s.items[ref]; taken {
			continue
		}
		c.Ref, c.Received = ref, now
		s.items[ref] = c
		return ref, nil
	}
}

func (s *DossierStore) Get(ref string) (Commission, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.items[ref]
	return c, ok
}

var dossierRefPattern = regexp.MustCompile(`^DC-\d{4}-[0-9A-F]{8}$`)

// CSRF protection uses a double-submit cookie: a random token is set as a
// SameSite=Strict cookie and echoed in a hidden field; both must match.
const csrfCookieName = "ac_csrf"

func csrfToken(w http.ResponseWriter, r *http.Request) (string, error) {
	if c, err := r.Cookie(csrfCookieName); err == nil && len(c.Value) == 43 {
		return c.Value, nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	tok := base64.RawURLEncoding.EncodeToString(buf)
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    tok,
		Path:     "/commission",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int((12 * time.Hour).Seconds()),
	})
	return tok, nil
}

func validCSRF(r *http.Request) bool {
	c, err := r.Cookie(csrfCookieName)
	if err != nil || c.Value == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(r.PostFormValue("csrf"))) == 1
}
