package main

import (
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestSolveTwoStageStaysInRangeAndIsAccurate(t *testing.T) {
	cases := []struct {
		name    string
		target  float64
		maxRel  float64
		maxTeet int
	}{
		{"sidereal", siderealRate, 1e-6, 120},
		{"synodic", 1 / synodicMonth, 1e-5, 144},
		{"draconic", 1 / draconicMonth, 1e-5, 144},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stages := solveTwoStage(tc.target, minTeeth, tc.maxTeet)
			if len(stages) != 2 {
				t.Fatalf("want 2 stages, got %d", len(stages))
			}
			for _, s := range stages {
				for _, n := range []int{s.Driver, s.Driven} {
					if n < minTeeth || n > tc.maxTeet {
						t.Errorf("tooth count %d outside [%d, %d]", n, minTeeth, tc.maxTeet)
					}
				}
			}
			tr := Train{Target: tc.target, Stages: stages}
			if rel := math.Abs(tr.RelativeError()); rel > tc.maxRel {
				t.Errorf("relative error %.3g exceeds %.3g (train %s)", rel, tc.maxRel, tr.Notation())
			}
		})
	}
}

func TestEclipticEccentricityMatchesClosedForm(t *testing.T) {
	// (1 - tan((90-e)/2) / tan((90+e)/2)) / 2 for e = 23.44 degrees.
	got := eclipticEccentricity(obliquityJ2000)
	if math.Abs(got-0.2846) > 0.0005 {
		t.Fatalf("eccentricity = %.4f, want about 0.2846", got)
	}
}

func TestFormatting(t *testing.T) {
	if got := formatLatitude(46.2017, Northern); got != "46°12'N" {
		t.Errorf("formatLatitude = %q", got)
	}
	if got := formatLatitude(33.9999, Southern); got != "34°00'S" {
		t.Errorf("formatLatitude rounding = %q", got)
	}
	if got := formatCHF(186000); got != "CHF 186'000" {
		t.Errorf("formatCHF = %q", got)
	}
	if got := formatArc(obliquityJ2000); got != "23°26'21\"" {
		t.Errorf("formatArc = %q", got)
	}
}

func validInput() CommissionInput {
	return CommissionInput{
		Hemisphere: "northern", LatDeg: "46", LatMin: "12", Tier: "horologium", Tympans: "1",
		Alloy: "CW508L", Script: "roman", Name: "Livia Marchetti", Email: "livia@example.ch",
		Country: "Switzerland", Contact: "email", Discretion: true,
	}
}

func TestValidateAcceptsCompleteDossier(t *testing.T) {
	c, errs := validInput().Validate()
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if want := 186_000 + tympanCHF; c.EstimateCHF() != want {
		t.Errorf("estimate = %d, want %d", c.EstimateCHF(), want)
	}
}

func TestValidateRejectsBadFields(t *testing.T) {
	cases := map[string]func(*CommissionInput){
		"hemisphere": func(in *CommissionInput) { in.Hemisphere = "eastern" },
		"latDeg":     func(in *CommissionInput) { in.LatDeg = "2"; in.LatMin = "0" },
		"latMin":     func(in *CommissionInput) { in.LatMin = "75" },
		"tier":       func(in *CommissionInput) { in.Tier = "" },
		"tympans":    func(in *CommissionInput) { in.Tympans = "9" },
		"alloy":      func(in *CommissionInput) { in.Alloy = "CW000X" },
		"script":     func(in *CommissionInput) { in.Script = "gothic" },
		"dedication": func(in *CommissionInput) { in.Dedication = strings.Repeat("a", maxDedication+1) },
		"name":       func(in *CommissionInput) { in.Name = "A" },
		"email":      func(in *CommissionInput) { in.Email = "Livia <livia@example.ch>" },
		"phone":      func(in *CommissionInput) { in.Contact = "telephone"; in.Phone = "" },
		"discretion": func(in *CommissionInput) { in.Discretion = false },
	}
	for field, mutate := range cases {
		t.Run(field, func(t *testing.T) {
			in := validInput()
			mutate(&in)
			_, errs := in.Validate()
			if errs.Get(field) == "" {
				t.Fatalf("expected an error on %q, got %v", field, errs)
			}
		})
	}
}

func TestErrorsOpenTheEarliestSection(t *testing.T) {
	errs := FieldErrors{"email": "x", "alloy": "y"}
	if got := errs.FirstStage(); got != 3 {
		t.Fatalf("FirstStage = %d, want 3", got)
	}
}

func TestLedgerSearchFoldsDiacriticsAndSorts(t *testing.T) {
	c := NewCatalog()
	got := c.SearchLedger(ParseLedgerQuery(url.Values{"q": {"zurich"}}))
	if len(got) != 1 || got[0].Ref != "AC-02-02" {
		t.Fatalf("search for zurich returned %d entries", len(got))
	}
	desc := c.SearchLedger(ParseLedgerQuery(url.Values{"sort": {"year"}, "dir": {"desc"}}))
	for i := 1; i < len(desc); i++ {
		if desc[i-1].Year < desc[i].Year {
			t.Fatalf("not sorted descending at %d", i)
		}
	}
	south := c.SearchLedger(ParseLedgerQuery(url.Values{"hemisphere": {"southern"}}))
	for _, e := range south {
		if e.Hemisphere != Southern {
			t.Fatalf("%s is not southern", e.Ref)
		}
	}
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	assets, err := NewAssetStore(embedded)
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewRenderer(embedded, assets, log)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{catalog: NewCatalog(), dossiers: NewDossierStore(), render: renderer, assets: assets, log: log, now: time.Now}
	srv := httptest.NewServer(app.routes())
	t.Cleanup(srv.Close)
	return srv
}

var scriptTag = regexp.MustCompile(`(?i)<script`)

func TestPagesRenderWithoutJavaScript(t *testing.T) {
	srv := newTestServer(t)
	paths := map[string]int{
		"/":       200,
		"/ledger": 200,
		"/ledger?q=geneva&sort=latitude&dir=desc": 200,
		"/ledger?q=nothing-matches":               200,
		"/ledger/ac-07-04":                        200,
		"/ledger/ac-99-99":                        404,
		"/commission":                             200,
		"/does-not-exist":                         404,
		"/static/styles.css":                      200,
		"/healthz":                                200,
	}
	for p, want := range paths {
		resp, err := http.Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("%s: status %d, want %d", p, resp.StatusCode, want)
		}
		if scriptTag.Match(body) {
			t.Errorf("%s: response contains a script tag", p)
		}
		if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'none'") {
			t.Errorf("%s: CSP missing script-src 'none': %q", p, csp)
		}
	}
}

func TestCommissionFlowEndToEnd(t *testing.T) {
	srv := newTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	resp, err := client.Get(srv.URL + "/commission")
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	m := regexp.MustCompile(`name="csrf" value="([^"]+)"`).FindSubmatch(page)
	if m == nil {
		t.Fatal("no csrf token in form")
	}
	token := string(m[1])

	form := url.Values{
		"csrf": {token}, "hemisphere": {"southern"}, "lat_deg": {"33"}, "lat_min": {"52"},
		"tier": {"uranographia"}, "tympans": {"0"}, "alloy": {"CW409J"}, "script": {"italic"},
		"name": {"Tomás Ibarra"}, "email": {"tomas@example.cl"}, "country": {"Chile"},
		"contact": {"email"}, "discretion": {"yes"},
	}

	// Missing CSRF is refused.
	bad := url.Values{}
	for k, v := range form {
		bad[k] = v
	}
	bad.Del("csrf")
	resp, _ = client.PostForm(srv.URL+"/commission", withAction(bad, "review"))
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("missing csrf: status %d", resp.StatusCode)
	}

	// Invalid input reopens the form with 422.
	invalid := url.Values{}
	for k, v := range form {
		invalid[k] = v
	}
	invalid.Set("email", "not-an-address")
	resp, _ = client.PostForm(srv.URL+"/commission", withAction(invalid, "review"))
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(body), `id="email-error"`) {
		t.Fatalf("invalid email: status %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), `id="stage-4" value="4" checked`) {
		t.Error("error response should reopen the Patron section")
	}

	// Review, then confirm.
	resp, _ = client.PostForm(srv.URL+"/commission", withAction(form, "review"))
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "CHF 416&#39;200") {
		t.Fatalf("review: status %d, estimate missing", resp.StatusCode)
	}

	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, _ = client.PostForm(srv.URL+"/commission", withAction(form, "confirm"))
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(loc, "/commission/received/DC-") {
		t.Fatalf("confirm: status %d, location %q", resp.StatusCode, loc)
	}
	resp, _ = client.Get(srv.URL + loc)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || strings.Contains(string(body), "tomas@example.cl") {
		t.Fatalf("receipt: status %d or leaks the email address", resp.StatusCode)
	}
}

func withAction(v url.Values, action string) url.Values {
	out := url.Values{}
	for k, vals := range v {
		out[k] = vals
	}
	out.Set("action", action)
	return out
}
